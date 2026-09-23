## 1. Backend — ключ опознания сессии (субагент `backend`)

- [x] 1.1 Добавить `middleware.UserSession.DirectoryKey()` (Решение 2): нормализация `Username` — trim, срез ведущего `@`, нижний регистр; пустая строка при незаданном username. Проверка: юнит-тест в `middleware/telegram_auth_test.go` на `@Ivan`, `ivan`, ` IVAN `, `""`.
- [x] 1.2 Нормализовать сравнение в `repositories.FindUserByTelegramID` и `GetTeamsByUserTelegramID` — `WHERE lower(ltrim(telegram_id, '@')) = $1` (Решение 3), сигнатуры не менять. Проверка: `go build ./...` и существующие тесты репозитория проходят.
- [x] 1.3 Перевести `middleware.RoleAuth` на `session.DirectoryKey()` в ветках team-admin и `member`. Проверка: `role_auth_test.go` — сессия с числовым `TelegramID` и заданным `Username` опознаёт `member`.
- [x] 1.4 Перевести на `session.DirectoryKey()` остальные обращения к справочнику: `handlers/gantt.go`, `scoring.go`, `admin_scores.go`, `admin.go`, `stories.go`, `team_admin.go`, `notify.go`, `schedule_settings.go`. Проверка: `grep -rn "session.TelegramID" internal/ | grep -v _test` оставляет только сравнения `== ""`, адресата в `gantt_export.go` и поле ответа в `GetProfile`.

## 2. Backend — доставка картинки (субагент `backend`)

- [x] 2.1 В `handlers/gantt_export.go` заменить резолвинг получателя на `strconv.ParseInt(session.TelegramID, 10, 64)` и убрать обращение `FindUserByTelegramID` вместе с проверкой `user.ChatID == 0` (Решение 1). Проверка: `BOT_CHAT_NOT_STARTED` остаётся только в ветке `errors.Is(err, tgbot.ErrorForbidden)`.
- [x] 2.2 Неразбираемый `session.TelegramID` отдаёт 500 `internal_error` с записью в лог, а не `BOT_CHAT_NOT_STARTED` (Решение 1). Проверка: тест на сессию с нечисловым `TelegramID`.
- [x] 2.3 Переписать `gantt_export_test.go` под новый контракт: успех без записи в справочнике, адресат равен числовому ID из сессии, 403 от Telegram → 409 `BOT_CHAT_NOT_STARTED`, нечисловой ID → 500. Проверка: `go test ./internal/transport/httpServer/handlers/... -run Export`.

## 3. Backend — объяснимый отказ (субагент `backend`)

- [x] 3.1 В `RoleAuth` и `GetProfile` отличать пустой `DirectoryKey()` (не задан @username) от «не найден в справочнике» (Решение 4); `GetProfile` отвечает `writeErrorCode` с кодами `USERNAME_REQUIRED` и `USER_NOT_REGISTERED`, статус в обоих случаях 403. Проверка: тест на оба кода в ответе `/api/gantt/profile`.
- [x] 3.2 Убедиться, что формат ответов middleware (`{"error":"..."}`) не меняется — приведение к общему формату вынесено в «Дальнейшие шаги» design.md. Проверка: `role_auth_test.go` на прежнюю форму тела.

## 4. UX-ревью (субагент `ux`)

- [x] 4.1 Сформировать постановку для `denied-overlay` (`index.html:51`): как подать причину отказа, полученную от сервера, чтобы «не задан @username» читалось как исправимое действие, а не как окончательный запрет; что показывать, когда сервер причины не прислал. Проверка: постановка приложена к заявке отдельным файлом `ux-brief.md`.

## 5. Frontend (субагент `frontend`)

- [x] 5.1 Реализовать постановку 4.1: первый `<p>` оверлея получает `id` и заполняется сообщением из ответа, при отсутствии сообщения остаётся прежний текст. Проверка: оверлей открывается с текстом сервера и без него.
- [x] 5.2 В `api.js` в ветке 403 не терять `errData` — передавать сообщение и код в оверлей (Решение 4). Проверка: в консоли у брошенной ошибки заполнены `code` и `message`.

## 6. QA (субагент `qa`)

- [x] 6.1 Прогнать `go test ./... -count=1` и `go vet ./...`. Проверка: сборка и тесты зелёные.
- [x] 6.2 Проверить покрытие новых веток `gantt_export.go` и `role_auth.go` — `go test -cover` по затронутым пакетам не ниже прежнего.

## 7. Проверка на проде (после деплоя)

- [ ] 7.1 Выгрузить диаграмму в PNG из мобильного Telegram WebApp — файл приходит в личный чат, 409 больше нет.
- [ ] 7.2 То же в SVG.
- [ ] 7.3 Выгрузка пользователем, у которого личный чат с ботом не начат, — 409 с подсказкой открыть чат; после `/start` повторная выгрузка проходит.
- [ ] 7.4 Участник без роли admin открывает веб — видит свои команды и вкладку «Гант» (проверка Решения 2 на реальных данных).
- [ ] 7.5 Администратор команды открывает веб — видит административные действия своей команды и не видит чужих.
- [ ] 7.6 Пользователь без @username открывает веб — видит объяснение про @username, а не общий отказ.
