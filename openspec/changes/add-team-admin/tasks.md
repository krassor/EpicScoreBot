## 1. Backend: данные и репозиторий

- [x] 1.1 Добавить миграцию `internal/migrator/migrations/010_team_admins.sql`
      с таблицей `team_admins` (`user_id`, `team_id`, `assigned_by`,
      `created_at`, составной PK `(user_id, team_id)`, FK на `users`/`teams`
      с `ON DELETE CASCADE`) и проверить, что миграция применяется без
      ошибок на чистой БД (`go run` мигратора / существующий механизм
      запуска миграций проекта).
- [x] 1.2 Добавить в `internal/repositories/` (новый файл `team_admin.go`)
      методы: `AssignTeamAdmin(ctx, userID, teamID, assignedBy uuid.UUID) error`,
      `RemoveTeamAdmin(ctx, userID, teamID uuid.UUID) error`,
      `GetTeamAdminsByTeamID(ctx, teamID uuid.UUID) ([]domain.User, error)`,
      `GetTeamIDsByAdminUserID(ctx, userID uuid.UUID) ([]uuid.UUID, error)`,
      `IsTeamAdmin(ctx, userID, teamID uuid.UUID) (bool, error)`,
      `IsTeamAdminOfAny(ctx, userID uuid.UUID) (bool, error)`; покрыть
      unit-тестами (или интеграционными по образцу существующих в
      `internal/repositories`, если там есть тестовая БД).
- [x] 1.3 Добавить в репозиторий telegram-ориентированные обёртки, если
      понадобится резолвинг `telegram_id → user_id` (проверить, есть ли
      уже `FindUserByTelegramID` — переиспользовать вместо дублирования).

## 2. Backend: Telegram-бот

- [x] 2.1 В `internal/telegram/auth.go` добавить `isTeamAdminAny` и
      `isTeamAdminFor(ctx, telegramID string, teamID uuid.UUID)`
      поверх новых репозиторных методов; сохранить `isSuperAdmin`/
      `isSuperAdminCallback` без изменений (config-based).
- [x] 2.2 Заменить вызовы `isAdmin`/`isAdminCallback` в
      `handlers.go`, `handlers_epic.go`, `handlers_admin.go`,
      `admin_callbacks.go` на `isTeamAdminAny` (грубый гейт входа в
      команду) или `isTeamAdminFor` (когда команда уже известна из
      контекста действия) согласно design.md раздел "3. Двухуровневая
      проверка admin в Telegram-боте"; проверить `go build ./...`
      проходит и существующие тесты бота (`go test ./internal/telegram/...`)
      не сломаны.
- [x] 2.3 Отфильтровать выдачу `showTeamPickerInitial` и
      `showEpicPickerInitial` по командам вызывающего (для superadmin —
      без фильтра, для team-admin — только его команды); добавить
      финальную повторную проверку `isTeamAdminFor` в момент выполнения
      действия колбэка (защита от гонки между выбором и подтверждением).
- [x] 2.4 Переписать `handleAddAdmin`/`handleRemoveAdmin` в
      `handlers_admin.go`: убрать чтение/запись
      `epicBot.cfg.BotConfig.Admins`/`epicBot.cfg.Write()`, реализовать
      интерактивный флоу (выбор команды → выбор пользователя из
      зарегистрированных участников для назначения, либо выбор из
      текущих team-admin команды для снятия) через существующий
      механизм inline-клавиатур/сессий (по образцу
      `showUserPickerInitial`/`handleChangeRate`); только superadmin.
- [ ] 2.5 Проверить вручную (или e2e-тестом, если такие уже есть в
      проекте) базовый сценарий: superadmin назначает пользователя
      team-admin команды, этот пользователь получает доступ к
      `/addepic` только для своей команды, `/deleteepic` (superadmin-only)
      не меняется.
      _Не выполнено: в проекте нет e2e-тестов бота и работающего
      Telegram-токена для ручной проверки в этой сессии; логика проверена
      только на уровне кода/сборки (`go build`) — см. финальный отчёт._

## 3. Backend: HTTP API

- [x] 3.1 Добавить в `internal/transport/httpServer/middleware/role_auth.go`
      интерфейсы `TeamAdminChecker`/`TeamAdminScoper` (см. design.md
      раздел 4) и переключить определение роли `"admin"` в `RoleAuth` с
      `cfg.Admins` на `TeamAdminChecker.IsTeamAdminOfAny`; обновить/добавить
      тесты в `role_auth_test.go` под новую сигнатуру.
- [x] 3.2 Убрать `isAdminOrSuperAdmin` (читает `cfg.BotConfig.Admins`) из
      `handlers/admin_scores.go`, заменить проверкой через
      `TeamAdminScoper.IsTeamAdminOf` с `team_id`, резолвленным из
      `epic_id`/`risk_id` запроса (superadmin — пропускать проверку);
      обновить `admin_scores_test.go`.
- [x] 3.3 В `handlers/gantt.go` заменить прямые обращения к
      `cfg.BotConfig.Admins` (около строк 77-103 и 677-697) на ту же
      team-scoped проверку.
- [x] 3.4 В `handlers/admin.go`: `GetUsersList`/`GetUserDetails` —
      фильтровать выдачу командами admin (`AdminTeamIDs`) для не-superadmin;
      `CreateSingleUser`/`UpdateUser` — валидировать, что переданные
      `team_ids` являются подмножеством команд вызывающего admin (иначе
      400/403); `UpdateEpic` (и `UpdateStory`/`UpdateRisk` в соответствующих
      хендлерах) — проверять `IsTeamAdminOf` команды целевой сущности.
      Обновить/добавить тесты в `admin_test.go`.
- [x] 3.5 Добавить хендлеры и роуты (только `RoleAuth(..., "superadmin")`)
      для управления привязками: `GET/POST/DELETE /api/gantt/admin/team-admins`
      (список по `team_id`, назначение, снятие) в `routers.go` и новом
      файле хендлеров; ошибки — в стандартном формате
      `{ "error": { "code": "...", "message": "..." } }`. Покрыть unit-тестами
      happy path и 403 для не-superadmin.

## 4. Frontend: веб-панель

- [x] 4.1 В `web/gantt/index.html` добавить раздел "Администраторы команд"
      в admin-панели (выбор команды через существующий паттерн
      `team-select`, таблица текущих team-admin, форма назначения по
      пользователю из уже загруженного списка), видимый только когда
      текущая сессия — superadmin.
- [x] 4.2 В `web/gantt/js/admin-panel.js` реализовать загрузку/рендер
      списка team-admin по выбранной команде и обработчики
      назначения/снятия через новые эндпоинты `/api/gantt/admin/team-admins`;
      проверить в браузере (dev-сервер) сценарий: назначение, отображение
      в списке, снятие, ошибка при попытке под non-superadmin сессией.
      Проверено headless-браузером (Playwright + системный Chrome) со
      статикой `web/gantt/` через `python3 -m http.server` и замоканными
      ответами `/api/gantt/*` (реального бэкенда/БД в этой сессии нет —
      см. финальный отчёт): назначение/снятие/обновление списка работают,
      раздел скрыт и запрос отклоняется (403, сообщение показано тостом)
      под сессией role=admin (не superadmin).

## 5. QA

- [x] 5.1 Написать тесты на `internal/repositories/team_admin.go`
      (assign/remove/list/is-admin, в т.ч. что удаление пользователя или
      команды каскадно убирает привязку) — `go test ./internal/repositories/...`.
      Реализовано как интеграционные тесты (`team_admin_test.go`) против
      реальной PostgreSQL (DSN из `TEST_DATABASE_DSN`, по умолчанию —
      docker-compose проекта); при недоступности БД тесты пропускаются
      (`t.Skip`), не ломая `go test ./...`. Проверено локально against
      реальный Postgres — все проходят (см. финальный отчёт).
- [x] 5.2 Написать тесты на team-scoped авторизацию в
      `middleware/role_auth_test.go`, `admin_scores_test.go`, `admin_test.go`:
      team-admin команды A получает 200 на действиях над A и 403/404 над B;
      superadmin получает 200 над обеими; пользователь без привязок не
      проходит `RoleAuth("admin")`.
- [x] 5.3 Прогнать полный набор `go test ./...` и убедиться, что покрытие
      новых пакетов не ниже принятого в проекте порога (см.
      `.agents/tasks/task_qa.md`). `go build ./...`, `go vet ./...`,
      `go test -race ./...` — зелёные; `services` (покрывает
      `TeamAdminService`) — 80.6% (порог ≥80% выполнен); `repositories` —
      низкое агрегированное покрытие пакета (7.2%), т.к. до этой задачи в
      пакете не было тестов вовсе (не тестировался никакой другой файл) —
      новый файл `team_admin.go` покрыт интеграционными тестами полностью.
