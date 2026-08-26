## Context

См. proposal.md - Why. Технический контекст, влияющий на реализацию:

- Правило "кто ещё не проголосовал по эпику" сейчас существует только
  внутри `sendEpicNotifications` (`internal/telegram/admin_callbacks.go`,
  вызывается из `/epicnotify`): для каждого участника команды эпика
  проверяется `epicService.HasUserScoredEpic` и
  `riskService.GetUnscoredRisksByUser`, строится текст напоминания,
  сообщение шлётся личным сообщением через `epicBot.b.SendMessage`
  (`ChatID` пользователя).
- Бот и HTTP-хендлеры используют независимые слои доступа к данным:
  `internal/telegram.Bot` держит набор узких сервис-интерфейсов
  (`epicService services.EpicService`, `userService services.UserService`,
  `riskService services.RiskService`, ...), а
  `internal/transport/httpServer/handlers.GanttHandler` работает напрямую
  с БД-репозиторием через собственный локально объявленный интерфейс
  `handlers.Repository` (см. `interfaces.go`) — `handlers` пакет нигде не
  импортирует `internal/services` (проверено — импортов нет). Это
  существующая, а не вводимая этой задачей граница.
- Оба типа (`services.Repository`, `handlers.Repository`) уже содержат
  почти всё нужное для вычисления списка непроголосовавших:
  `GetEpicByID`, `GetUsersByTeamID` есть в обоих; `HasUserScoredEpic`,
  `GetUnscoredRisksByUser` реализованы в конкретном
  `internal/repositories.Repository`, но в локальный интерфейс
  `handlers.Repository` пока не включены.
- `GanttHandler` не имеет способа отправить Telegram-сообщение — этой
  возможностью обладает только `internal/telegram.Bot` (обёртка над
  `*bot.Bot` из go-telegram/bot). Раньше HTTP-слой в отправке сообщений
  не нуждался.
- `/epics/start` (аналогичный по форме "admin-действие над уже выбранным
  эпиком" эндпоинт) сейчас смонтирован ВНЕ защищённой admin-группы
  роутов (доступен любому аутентифицированному пользователю, скрыт
  только на фронте) — существующий, не связанный с этой задачей пробел.

## Goals / Non-Goals

**Goals:**
- Единственный источник истины для "кто ещё не проголосовал по эпику и
  какой текст напоминания" — не дублировать этот расчёт между ботом и
  HTTP-хендлером.
- Не переносить `handlers` на зависимость от `internal/services` (сохранить
  существующую границу пакетов) и не переносить `internal/telegram` на
  прямую работу с `internal/repositories` в обход сервисного слоя.
- Новый HTTP-эндпоинт защищён server-side (team-scoped admin), в отличие
  от соседнего `/epics/start`.

**Non-Goals:**
- Не чинить отсутствие server-side защиты у `/epics/start` и других
  соседних эндпоинтов вне admin-группы — отдельная задача.
- Не переводить существующие HTTP-хендлеры на использование
  `internal/services` — только для этой фичи вводится нейтральный пакет.
- Не менять текст/логику напоминания как таковые — только источник
  вызова (веб-панель в дополнение к боту).

## Decisions

### 1. Новый нейтральный пакет `internal/notify` для общей бизнес-логики

Вместо дублирования цикла "участник → проверить голос → собрать текст"
в HTTP-хендлере или переноса `handlers` на `internal/services`, логика
переносится в новый пакет без зависимостей от Telegram/HTTP:

```go
package notify

type ReminderRepository interface {
    GetEpicByID(ctx context.Context, epicID uuid.UUID) (*domain.Epic, error)
    GetUsersByTeamID(ctx context.Context, teamID uuid.UUID) ([]domain.User, error)
    HasUserScoredEpic(ctx context.Context, epicID, userID uuid.UUID) (bool, error)
    GetUnscoredRisksByUser(ctx context.Context, userID, epicID uuid.UUID) ([]domain.Risk, error)
}

type EpicReminder struct {
    ChatID     int64
    TelegramID string // для отчёта о неудачных отправках
    Text       string
}

// BuildEpicScoringReminders возвращает эпик и список напоминаний для
// участников его команды, у кого есть неоконченные оценки (трудоёмкость
// эпика и/или риски). Единственное место, определяющее критерий
// "непроголосовавший" и текст сообщения.
func BuildEpicScoringReminders(ctx context.Context, repo ReminderRepository, epicID uuid.UUID) (*domain.Epic, []EpicReminder, error)

// DeliverReminders вызывает send для каждого напоминания и считает
// успехи/неудачи; send ничего не знает о Telegram API напрямую — просто
// "отправь текст в chatID".
func DeliverReminders(ctx context.Context, reminders []EpicReminder, send func(ctx context.Context, chatID int64, text string) error) (sentCount int, failedTelegramIDs []string)
```

`handlers.Repository` (уже структурно совместим по 2 из 4 методов)
дополняется `HasUserScoredEpic`/`GetUnscoredRisksByUser` — оба уже
реализованы в конкретном репозитории, новый код в репозитории не нужен.
Для бота, чьи `epicService`/`userService`/`riskService` — три отдельных
узких интерфейса, добавляется маленький адаптер в `internal/telegram`
(~15 строк), реализующий `notify.ReminderRepository` через вызовы этих
трёх сервисов — без изменения самих сервисных интерфейсов.

**Альтернатива А (отклонена)**: продублировать цикл в HTTP-хендлере.
Отклонена — прямое нарушение "не дублируй", риск расхождения критерия
"непроголосовавший" и текста между ботом и веб-панелью со временем.

**Альтернатива Б (отклонена)**: перевести `handlers` на прямую
зависимость от `internal/services.EpicService`/`UserService`/`RiskService`.
Отклонена — ломает существующую, последовательно соблюдаемую во всём
пакете `handlers` границу (`handlers` нигде не импортирует `services`,
работает напрямую с репозиторием); эта задача — не повод её пересматривать.

**Альтернатива В (отклонена)**: положить общую функцию в
`internal/services` и завести в `handlers` только для неё этот один
импорт. Отклонена — тот же аргумент, что и Б: сама функция не требует
сервисного слоя (ей нужен только репозиторий), поэтому нейтральный
пакет без зависимости от `services` — более узкая и точная граница.

### 2. Отправка сообщения: интерфейс в `handlers`, реализация в `telegram.Bot`

`GanttHandler` получает новую зависимость через локально объявленный (по
уже принятой в проекте конвенции — см. `middleware.UserFinder`,
`handlers.Repository`) интерфейс:

```go
// handlers/interfaces.go
type TelegramNotifier interface {
    SendDirectMessage(ctx context.Context, chatID int64, text string) error
}
```

`internal/telegram.Bot` получает новый экспортируемый метод
`SendDirectMessage` (тонкая обёртка над уже используемым
`epicBot.b.SendMessage`) — структурно реализует `handlers.TelegramNotifier`
без явной ссылки/импорта. `app/main.go` передаёт уже существующий `tgBot`
в `handlers.NewGanttHandler(..., tgBot)` — `tgBot` создаётся раньше
`ganttHandler` в текущем порядке инициализации, порядок не меняется.

### 3. `sendEpicNotifications` в боте — рефакторинг, не переписывание

`internal/telegram/admin_callbacks.go:sendEpicNotifications` заменяет
внутренний цикл на `notify.BuildEpicScoringReminders` +
`notify.DeliverReminders(ctx, reminders, epicBot.SendDirectMessage)`,
затем как и раньше редактирует сообщение в чате итоговым отчётом
(`✅ Уведомления по эпику %s разосланы...`). Наблюдаемое поведение команды
`/epicnotify` не меняется — это чистый рефакторинг с целью убрать
единственный источник логики из бот-специфичного файла.

### 4. Новый HTTP-эндпоинт защищается server-side, в отличие от `/epics/start`

`POST /api/gantt/epics/notify` монтируется в уже существующую
защищённую подгруппу роутов (`RoleAuth(..., "admin")`, рядом с
`/admin/scores/*`), а не в общую группу, где сейчас живёт `/epics/start`.
Внутри хендлера — team-scoped проверка по образцу
`AdminSubmitEpicScore`/`admin_scores.go`: `isSuperAdminSession` ИЛИ
`h.repo.IsTeamAdminOf(ctx, session.TelegramID, epic.TeamID)`. Причина
расхождения с соседним `/epics/start` — рассылка напоминаний имеет
видимый внешний побочный эффект (реальные сообщения участникам команды в
Telegram), в то время как непринятая защита `/epics/start` — отдельный,
не связанный с этой задачей пробел (см. Non-Goals); новые эндпоинты по
умолчанию должны быть защищены (правило проекта).

Валидация статуса эпика (`SCORING`) — как и в боте
(`showEpicPickerInitial(..., "epicnotify", string(domain.StatusScoring))`,
только эпики этого статуса предлагались к выбору) — переносится в
хендлер explicit-проверкой, иначе `400`.

### 5. Формат ответа и запроса

По аналогии с `/epics/start` (`POST` + body `{epic_id}`, а не URL path
param) — тот же паттерн для однотипного "admin-действие над уже выбранным
эпиком":

```
POST /api/gantt/epics/notify
{ "epic_id": "<uuid>" }

200 OK
{ "sent_count": 3, "failed_telegram_ids": ["ivanov"] }
```

Ошибки — стандартный формат `{ "error": { "code": "...", "message": "..." } }`.

## Risks / Trade-offs

- [Рефакторинг рабочей команды `/epicnotify`] → регресс поведения бота
  для существующих пользователей → покрыть новый пакет `notify` unit-тестами
  на тот же набор кейсов, что уже неявно проверялся вручную (частично
  оценённый эпик, эпик без неоценивших, пользователь без ChatID).
- [Второй источник отправки Telegram-сообщений (HTTP-хендлер) в
  дополнение к боту] → в проекте один Bot API токен/клиент — оба пути
  используют один и тот же `*bot.Bot` через один и тот же `tgBot`,
  никакого нового клиента/токена не создаётся, риска рассинхронизации
  нет.
- [Пользователь без `ChatID` (не начинал диалог с ботом)] → как и в
  боте, такие пользователи попадают в `failed_telegram_ids`, ответ
  хендлера — `200` с непустым `failed_telegram_ids`, не ошибка (это не
  сбой запроса, а частичный результат рассылки) — совпадает с текущим
  поведением бота (аналогично сообщающего "не удалось отправить" в
  сводке, не прерывая её).
