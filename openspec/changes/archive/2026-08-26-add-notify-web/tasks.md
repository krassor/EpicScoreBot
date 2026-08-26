## 1. Backend: общий пакет напоминаний

- [x] 1.1 Создать `internal/notify` с интерфейсом `ReminderRepository`,
      типом `EpicReminder` и функцией
      `BuildEpicScoringReminders(ctx, repo, epicID)`, перенеся в неё
      логику вычисления непроголосовавших участников и текста
      напоминания из `internal/telegram/admin_callbacks.go:sendEpicNotifications`
      (см. design.md, решение 1). Покрыть unit-тестами кейсы: участник не
      оценил ничего, участник оценил только эпик, участник полностью
      завершил оценку, эпик без участников.
- [x] 1.2 Добавить в `internal/notify` функцию
      `DeliverReminders(ctx, reminders, send)` (счётчик успех/неудача) и
      unit-тест на неё (в т.ч. кейс ошибки отправки одному из адресатов —
      рассылка остальным не прерывается).

## 2. Backend: Telegram-бот (рефакторинг)

- [x] 2.1 Добавить в `internal/telegram` адаптер, реализующий
      `notify.ReminderRepository` поверх существующих
      `epicService`/`userService`/`riskService` бота.
- [x] 2.2 Добавить экспортируемый метод
      `func (epicBot *Bot) SendDirectMessage(ctx, chatID int64, text string) error`
      (обёртка над `epicBot.b.SendMessage`).
- [x] 2.3 Переписать `sendEpicNotifications` на вызов
      `notify.BuildEpicScoringReminders` + `notify.DeliverReminders`,
      сохранив прежний текст итоговой сводки в чате. Проверить
      `go test ./internal/telegram/...` и вручную командой `/epicnotify`,
      что видимое поведение не изменилось.

## 3. Backend: HTTP API

- [x] 3.1 Добавить в `internal/transport/httpServer/handlers/interfaces.go`
      методы `HasUserScoredEpic`, `GetUnscoredRisksByUser` в интерфейс
      `Repository` (реализация уже есть в `internal/repositories`, новый
      код репозитория не требуется) и новый интерфейс `TelegramNotifier`
      с методом `SendDirectMessage`.
- [x] 3.2 Добавить поле `notifier TelegramNotifier` и параметр в
      `NewGanttHandler` (`internal/transport/httpServer/handlers/gantt.go`).
- [x] 3.3 Реализовать хендлер `POST /epics/notify` (body `{epic_id}`):
      `requireSession`, team-scoped проверка admin по образцу
      `AdminSubmitEpicScore` (`isSuperAdminSession` ИЛИ
      `h.repo.IsTeamAdminOf`), валидация статуса эпика (SCORING → иначе
      400), вызов `notify.BuildEpicScoringReminders` +
      `notify.DeliverReminders(ctx, reminders, h.notifier.SendDirectMessage)`,
      ответ `{ "sent_count": N, "failed_telegram_ids": [...] }`. Покрыть
      тестами: happy path, 403 для team-admin чужой команды, 403 для
      участника без admin-прав, 400 для эпика не в статусе SCORING.
- [x] 3.4 Смонтировать роут в `internal/transport/httpServer/routers.go`
      внутри существующей защищённой admin-подгруппы
      (`RoleAuth(..., "admin")`), рядом с `/admin/scores/*`.
- [x] 3.5 В `app/main.go` передать существующий `tgBot` в
      `handlers.NewGanttHandler(...)` как `TelegramNotifier`; проверить
      `go build ./...` проходит.

## 4. Frontend: веб-панель

- [x] 4.1 В `web/gantt/js/scoring-panel.js` (`renderDetails`) добавить
      кнопку "🔔 Напомнить непроголосовавшим" в заголовке карточки эпика
      рядом с "✏️ Редактировать", видимую при `isAdmin &&
      selectedEpic.status === 'SCORING'`.
- [x] 4.2 В `bindEvents` добавить обработчик клика, вызывающий
      `apiPost('/epics/notify', { epic_id: selectedEpic.id })`, и по
      результату показывать `showToast` с числом отправленных/неудачных
      напоминаний (по аналогии с `startEpicScoring`).
- [x] 4.3 Проверить в браузере (dev-сервер): кнопка видна только
      admin/superadmin и только для эпика в статусе SCORING; клик
      вызывает эндпоинт и показывает тост с результатом; кнопка
      отсутствует для обычного участника и для эпика в статусе NEW/SCORED.

## 5. QA

- [x] 5.1 Прогнать `go test ./...` и убедиться, что новые пакеты
      (`internal/notify`) и изменённые (`internal/telegram`,
      `internal/transport/httpServer/handlers`) покрыты тестами не хуже
      принятого в проекте порога (см. `.agents/tasks/task_qa.md`).
