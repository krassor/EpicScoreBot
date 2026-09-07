## 1. Backend: сервисный слой

- [x] 1.1 Реализовать `Service.SubmitExpertRoleScore(ctx, epicID, roleID uuid.UUID, score int) (scoredCount int, err error)` в `internal/scoring/scoring.go`: проверка `epic.Status == domain.StatusScoring` (иначе новая ошибка вроде `ErrScoringAlreadyComplete`, по аналогии с `ErrScoringNotComplete`), `repo.GetUsersByTeamIDAndRoleID(ctx, epic.TeamID, roleID)` (пустой список → ошибка "нет участников с этой ролью в команде"), цикл `repo.CreateEpicScore(ctx, epicID, user.ID, roleID, score)` по каждому участнику, затем один раз `s.TryCompleteEpicScoring(ctx, epicID)`. Проверить: `go build ./...`, юнит-тесты — happy path (несколько участников роли), ошибка при пустом списке участников, ошибка при статусе не `SCORING`, сценарий "это была последняя недостающая роль → эпик переходит в SCORED".

## 2. Backend: HTTP-слой

- [x] 2.1 Добавить хендлер `AdminSubmitExpertRoleScore` в `internal/transport/httpServer/handlers/admin_scores.go` (`POST /admin/scores/role/expert`, body `{epic_id, role_id, score}`), права доступа/team-scoping — копия структуры `AdminOverrideRoleScore`. Ответ: `{"status":"ok","epic_id":...,"role_id":...,"scored_count":N,"epic_status":"SCORING"|"SCORED"}` (актуальный статус эпика после вызова сервиса). Ошибки — стандартный формат `{"error":{"code":"...","message":"..."}}`. Проверить: httptest — 200 с корректным `scored_count`/`epic_status`; 400 при статусе не `SCORING` и при отсутствии участников роли; 403 для team-admin чужой команды.
- [x] 2.2 Зарегистрировать роут в `internal/transport/httpServer/routers/routers.go` в существующей admin-only группе (путь `/admin/scores/role/expert`, отдельно от уже существующего `/admin/scores/role`). Проверить: `go build ./...`.

## 3. Frontend: массовое голосование по роли

- [x] 3.1 В `web/gantt/js/scoring-panel.js` изменить источник данных таблицы «Оценки по ролям» при статусе `SCORING`: строить список строк из `evaluating_role_ids` выбранной стори/эпика (имена ролей — через уже используемый на этой же странице справочник ролей, без нового запроса), вместо пустого результата `GET /epics/{id}/role-scores`. При статусе `SCORED` источник данных и поведение не меняются (используется уже существующий `role-scores` + кнопка переопределения). Проверить: `node --input-type=module --check web/gantt/js/scoring-panel.js` проходит.
- [x] 3.2 Для каждой строки роли при статусе `SCORING` добавить поле ввода и кнопку «Оценить всей ролью» (видна только администратору). По клику — модалка подтверждения с текстом о перезаписи личных голосов участников этой роли, затем `apiPost('/admin/scores/role/expert', {epic_id, role_id, score})`, при успехе — `loadEpicData()` для обновления панели (прогресс голосов, таблицы, возможный переход в `SCORED`). При отмене подтверждения — запрос не отправляется. Проверить вручную в браузере: подтверждение работает, отмена ничего не меняет, после применения прогресс/статус обновляются.

## 4. QA

- [x] 4.1 Прогнать `go test ./...` (включая новые тесты 1.1/2.1) и убедиться, что покрытие `internal/scoring` и `internal/transport/httpServer/handlers` не упало, регрессий в существующих тестах завершения скоринга нет.
- [ ] 4.2 Ручная проверка сценариев из `specs/role-expert-vote/spec.md`: экспертная оценка роли без личных голосов, перезапись уже поданных личных голосов, ошибка при пустой роли, отказ для уже `SCORED` эпика, переход в `SCORED` после проставления последней недостающей роли, отмена подтверждения не меняет данные.
