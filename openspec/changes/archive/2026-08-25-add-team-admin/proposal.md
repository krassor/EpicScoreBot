## Why

Сейчас роль admin — глобальный список Telegram-username в `config.yaml`
(`BotConfig.Admins`), не привязанный к командам: любой admin видит и
управляет эпиками/пользователями/оценками всех команд сразу, а список
меняется через `/addadmin`/`/removeadmin` (только superadmin) с перезаписью
конфиг-файла. По мере роста числа команд это не масштабируется и не
позволяет делегировать управление конкретной командой её собственному
админу без выдачи полного доступа. Нужна возможность назначать
администратора(ов) на конкретную команду, храня привязку в БД, а не в
конфиге, при этом права admin ограничиваются его командой(ами).

## What Changes

- Новая таблица `team_admins` (many-to-many `users` ↔ `teams`): у одного
  пользователя может быть несколько команд, у одной команды — несколько
  админов.
- Роль superadmin остаётся как сейчас — глобальный список username в
  `config.yaml`, не привязана к командам; только superadmin назначает и
  снимает team-admin.
- **BREAKING**: авторизационная логика admin меняется с глобальной
  (`BotConfig.Admins`) на team-scoped через БД:
  - Telegram-бот: `isAdmin`/`isAdminCallback` перестают читать
    `BotConfig.Admins` и проверяют членство пользователя в `team_admins`
    (глобально — "есть ли хотя бы одна команда", либо для конкретной
    команды — см. design.md); команды `/addadmin`, `/removeadmin` меняют
    сигнатуру на `/addadmin <команда> <username>`,
    `/removeadmin <команда> <username>` и пишут/удаляют строку в БД вместо
    правки `config.yaml`.
  - HTTP API: `middleware.RoleAuth` и точечные проверки admin/superadmin в
    `handlers/admin_scores.go`, `handlers/gantt.go` перестают читать
    `cfg.BotConfig.Admins` для роли admin и проверяют team-admin в БД,
    ограничивая действие/выдачу данных командой(ами) вошедшего admin (не
    видит и не может редактировать чужие эпики/пользователей/оценки).
    Роль superadmin в HTTP API по-прежнему проверяется по `config.yaml`.
  - Старые записи `BotConfig.Admins` в `config.yaml` не мигрируются:
    список считается неактуальным после выката, superadmin назначает
    team-admin'ов заново новыми командами/через веб-панель.
- Веб-панель (Gantt admin-panel): новый раздел управления привязками
  admin↔команда (список текущих team-admin'ов по команде, назначение и
  снятие), доступен только superadmin.

## Capabilities

### New Capabilities
- `team-admin`: назначение и снятие администратора команды (many-to-many
  user↔team), список team-admin'ов команды, доступ к назначению только у
  superadmin, ограничение прав admin рамками его команд в Telegram-боте и
  HTTP API.

### Modified Capabilities
(нет существующих спек в проекте — задача полностью описывается новой
capability `team-admin`)

## Impact

- **backend** (затронутые слои: repositories → services → transport):
  - `internal/migrator/migrations/`: новая миграция — таблица
    `team_admins` (`user_id`, `team_id`, FK на `users`/`teams`,
    составной PK).
  - `internal/models/domain/domain.go`: при необходимости — вспомогательные
    типы для team-admin связи.
  - `internal/repositories/team.go` (или новый `team_admin.go`): CRUD для
    `team_admins` (assign/remove/list по команде/по пользователю,
    is-team-admin проверка).
  - `internal/services/team.go` или новый сервис: бизнес-логика
    назначения/снятия team-admin (только вызывается из superadmin-контекста).
  - `internal/telegram/auth.go`, `handlers_admin.go`, `handlers_epic.go`,
    `admin_callbacks.go`: замена глобальной проверки на team-scoped,
    новый формат команд `/addadmin`, `/removeadmin`.
  - `internal/transport/httpServer/middleware/role_auth.go`: team-scoped
    admin-проверка (нужен способ определить team_id целевого ресурса на
    уровне хендлера/роута).
  - `internal/transport/httpServer/handlers/admin.go`,
    `admin_scores.go`, `gantt.go`: фильтрация выдачи и проверка доступа
    по команде admin'а вместо `cfg.BotConfig.Admins`.
  - `internal/config/configModels.go`: `BotConfig.Admins` перестаёт
    использоваться для роли admin (SuperAdmins остаётся).
- **frontend** (`web/gantt/`): новый раздел в админ-панели — управление
  team-admin (только для superadmin), использует новые HTTP-эндпоинты.
- **qa**: тесты на team-scoped авторизацию (admin одной команды не видит/не
  управляет чужой), тесты миграции и репозитория `team_admins`.
- Формат ошибок API не меняется (стандартный
  `{ "error": { "code": "...", "message": "..." } }`).
