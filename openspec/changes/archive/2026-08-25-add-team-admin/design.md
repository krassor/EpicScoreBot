## Context

См. proposal.md - Why. Ключевые ограничения текущей реализации:

- `users` не хранит Telegram-username (только `telegram_id`, `first_name`,
  `last_name`) — только Telegram-бот получает username из `msg.From`/
  `callback.From` в рантайме, веб-сессия — из `middleware.UserSession.Username`
  (OAuth-виджет). Значит команду `/addadmin <username>` нельзя один в один
  перевести на БД: username нигде не хранится как устойчивый ключ.
- Проверки admin разбросаны в двух независимых местах с одинаковой логикой
  чтения `cfg.BotConfig.Admins`: Telegram-бот (`internal/telegram/auth.go`)
  и HTTP API (`middleware/role_auth.go`, `handlers/admin_scores.go`,
  `handlers/gantt.go`). Оба нужно перевести на БД независимо, интерфейсы
  разные (Telegram username/ID vs. `UserSession`).
  - Бот уже использует интерактивные пикеры через inline-кнопки и сессии
  callback'ов (`showTeamPickerInitial`, `showEpicPickerInitial`,
  `showUserPickerInitial` — см. `handleChangeRate`) — того же паттерна
  естественно ожидать и для назначения team-admin.
- Часть admin-flow в боте не привязана к конкретной команде на момент
  первой проверки (`isAdmin(msg)` вызывается до выбора команды/эпика,
  например `handleAddEpic` → `showTeamPickerInitial`), часть — привязана
  сразу (например `/deleteepic`, `/addrisk` работают с уже существующим
  эпиком, у которого есть `TeamID`).
- В HTTP API аналогично: `RoleAuth` монтируется на группу роутов без
  ресурса (`/admin/users`, `/admin/scores/*`, `PUT /epics/{id}` и т.д.),
  значит team-scoping нельзя целиком реализовать в middleware — часть
  проверки/фильтрации неизбежно уходит на уровень хендлера.

## Goals / Non-Goals

**Goals:**
- Хранить привязку admin↔team в БД (many-to-many), управляемую только
  superadmin.
- Ограничить действия admin (Telegram-бот и HTTP API) его командой(ами):
  не видит и не может изменять чужие эпики/пользователей/оценки.
- Дать superadmin два равноценных способа управления привязкой: команды
  бота и раздел веб-панели.

**Non-Goals:**
- Не переносить SuperAdmins в БД — остаётся конфиг-based (согласовано с
  пользователем).
- Не мигрировать текущие записи `BotConfig.Admins` — после выката список
  считается пустым, superadmin назначает заново.
- Не исправлять существующий, не связанный с этой задачей пробел: роуты
  `POST /teams`, `POST /epics`, `POST /risks`, `POST /users/bulk` сейчас
  не обёрнуты `RoleAuth` вообще (доступны любому аутентифицированному
  Telegram-пользователю) — вне рамок этой задачи, отдельное решение при
  необходимости.
- Не переделывать формат `/addadmin <username>` в текстовый
  `/addadmin <команда> <username>` — вместо этого меняем UX на
  интерактивный пикер (см. Decisions), т.к. username нигде не хранится.

## Decisions

### 1. Схема данных: таблица `team_admins`

```sql
CREATE TABLE IF NOT EXISTS team_admins (
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    assigned_by UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, team_id)
);
```

Миграция — следующий номер после `009_task_start_offset.sql`:
`010_team_admins.sql`.

`user_id` — FK на `users.id`, то есть team-admin обязан быть уже
зарегистрированным пользователем (как и назначение роли/веса сейчас).
`assigned_by` — кто из superadmin назначил (для аудита); superadmin не
обязан быть строкой в `users`, поэтому `ON DELETE SET NULL`, а не
NOT NULL — если такой строки нет, поле остаётся `NULL`.

**Альтернатива (отклонена)**: хранить Telegram username в отдельном
столбце `team_admins.username` без FK на `users`, как раньше в конфиге.
Отклонено — рассинхронизируется с `users`, теряет проверяемость (можно
назначить несуществующего пользователя), усложняет джойны для фильтрации
списков по team-admin.

### 2. UX назначения: интерактивный пикер вместо свободного текста

`/addadmin` и `/removeadmin` (только superadmin) переходят на тот же
паттерн, что уже используют `/changerate`, `/addepic` и др.:
`/addadmin` → inline-клавиатура выбора команды → inline-клавиатура выбора
пользователя (из уже зарегистрированных, `GetUsersByTeamID` или
`GetAllUsers`) → подтверждение → запись в `team_admins`.
`/removeadmin` → выбор команды → список её текущих team-admin'ов (уже
самодостаточный "список admin'ов команды", отдельная `/listadmins` не
нужна) → снятие.

**Альтернатива (отклонена)**: `/addadmin <команда> <username>` текстом.
Отклонена, т.к. `username` нигде не хранится персистентно — потребовался
бы вызов Telegram Bot API `getChat` по username в рантайме (новая
внешняя зависимость поведения, ненадёжно для приватных чатов) либо
хранение username в БД (см. решение 1).

### 3. Двухуровневая проверка admin в Telegram-боте

`internal/telegram/auth.go` получает два новых метода поверх БД
(через существующий репозиторий, доступный `Bot`):
- `isTeamAdminAny(msg/callback) bool` — true, если superadmin ИЛИ
  пользователь состоит в `team_admins` хотя бы для одной команды.
  Заменяет текущий `isAdmin`/`isAdminCallback` там, где команда ещё не
  выбрана (грубый гейт входа в flow, например `handleAddEpic`,
  `handleReport`, `handleList` — все, что начинаются с
  `showTeamPickerInitial`/`showEpicPickerInitial` без заранее известной
  команды).
- `isTeamAdminFor(msg/callback, teamID) bool` — true, если superadmin
  ИЛИ пользователь — team-admin именно этой команды. Используется в
  момент, когда команда уже известна: при финальном подтверждении внутри
  callback-флоу (создание эпика/риска/старт скоринга и т.п.) и там, где
  действие изначально привязано к существующему эпику
  (`TeamID` эпика известен сразу).
- `showTeamPickerInitial`/`showEpicPickerInitial` фильтруют выдаваемый
  список команд/эпиков по командам вызывающего admin (superadmin видит
  всё) — иначе admin увидит в списке чужие команды, хотя выбрать их не
  сможет (несогласованный UX).
- `isSuperAdmin`/`isSuperAdminCallback` не меняются (config-based).

### 4. Двухуровневая проверка admin в HTTP API

`middleware.RoleAuth`: роль `"admin"` перестаёт читать
`cfg.BotConfig.Admins`, вместо этого — новый интерфейс (по аналогии с
`UserFinder`):

```go
type TeamAdminChecker interface {
    IsTeamAdminOfAny(ctx context.Context, telegramID string) (bool, error)
}
```

`RoleAuth` остаётся грубым гейтом ("admin хоть какой-то команды" против
`requiredRole == "admin"`), т.к. на уровне middleware группы роутов
ресурс (`team_id`/`epic_id`) не всегда известен без парсинга тела
запроса. Точечная фильтрация/проверка конкретной команды — на уровне
хендлера через отдельный интерфейс:

```go
type TeamAdminScoper interface {
    IsTeamAdminOf(ctx context.Context, telegramID string, teamID uuid.UUID) (bool, error)
    AdminTeamIDs(ctx context.Context, telegramID string) ([]uuid.UUID, error)
}
```

Затрагиваемые хендлеры (`admin.go`, `admin_scores.go`, `gantt.go`):
- `AdminSubmitEpicScore`/`AdminSubmitRiskScore`/`AdminOverrideFinalScore`:
  после декодирования `epic_id`/`risk_id` — резолвить его `team_id` и
  проверить `IsTeamAdminOf`; superadmin — пропускать проверку.
  `isAdminOrSuperAdmin` в `admin_scores.go` заменяется на вызов новой
  проверки через `TeamAdminChecker`/`TeamAdminScoper` вместо
  `cfg.BotConfig.Admins`.
- `GetUsersList`/`GetUserDetails`: для admin (не superadmin) — фильтровать
  выдачу пользователями из команд `AdminTeamIDs`.
- `CreateSingleUser`/`UpdateUser`: admin может назначать только команды
  из своего списка (`team_ids` в запросе — подмножество `AdminTeamIDs`,
  иначе 403/400).
- `UpdateEpic`/`UpdateStory`/`UpdateRisk`: проверка `IsTeamAdminOf`
  команды целевого эпика/стори/риска.
- `gantt.go` (места, ранее читавшие `cfg.BotConfig.Admins` напрямую,
  строки ~77-103, ~677-697): переводятся на ту же проверку.

Формат ошибок не меняется — `403` с телом
`{ "error": { "code": "forbidden", "message": "..." } }`.

### 5. Веб-панель: новые эндпоинты (только superadmin)

```
GET    /api/gantt/admin/team-admins?team_id=<uuid>   — список team-admin'ов команды
POST   /api/gantt/admin/team-admins   {user_id, team_id}  — назначить
DELETE /api/gantt/admin/team-admins   {user_id, team_id}  — снять
```

Монтируются в новую подгруппу с `RoleAuth(..., "superadmin")`
(в `routers.go`, по аналогии с существующей admin-подгруппой). Раздел в
`web/gantt/js/admin-panel.js`/`index.html`: выбор команды (переиспользует
существующий `team-select`), таблица текущих team-admin'ов, форма
назначения (выбор пользователя из уже загруженного списка `loadUsers`).
Виден только если сессия — superadmin (уже определяется на фронте по
ответу `/profile`, см. текущую логику показа admin-функций).

## Risks / Trade-offs

- [Admin, потерявший все команды] → после снятия последней привязки
  admin теряет доступ полностью (не superadmin) — ожидаемое поведение,
  соответствует "не мигрировать": явно требует, чтобы у каждого
  действующего admin была хотя бы одна команда.
- [Рассинхронизация пикеров и финальной проверки] → между выбором команды
  в пикере и подтверждением callback'а права могли измениться (superadmin
  снял admin с команды) → финальная проверка `isTeamAdminFor`/
  `IsTeamAdminOf` выполняется заново при выполнении действия, не только на
  входе в flow.
- [Рост числа проверок в hot-path хендлеров] → каждый admin-хендлер
  получает дополнительный запрос в БД для проверки/фильтрации; объём
  данных мал (команды десятки, не тысячи), не ожидается влияния на
  производительность.
- [Двойная реализация проверки в боте и HTTP API] → неизбежно, т.к. это
  два независимых транспорта с разными механизмами идентификации
  пользователя (Telegram username/ID в боте, `UserSession` в HTTP);
  общая точка — репозиторий/сервис поверх `team_admins`.

## Migration Plan

1. Накатить миграцию `010_team_admins.sql` (создание таблицы, без данных).
2. Задеплоить backend с новой логикой проверки — начиная с этого момента
   `BotConfig.Admins` перестаёт давать реальные права admin (роль
   фактически пустая, пока superadmin не назначит team-admin'ов).
3. Superadmin вручную назначает team-admin'ов через бот или веб-панель.
4. Откат: миграция обратима (`DROP TABLE team_admins`), код проверки —
   через обычный revert коммита/деплоя предыдущей версии; на время между
   выкатом нового бэкенда и назначением admin'ов доступ к admin-функциям
   есть только у superadmin — деплой стоит планировать вместе с
   немедленным назначением team-admin'ов, чтобы не блокировать текущих
   пользователей.
