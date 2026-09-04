## 1. Backend: сервисный слой

- [x] 1.1 Реализовать `Service.SetManualRoleScore(ctx, epicID, roleID uuid.UUID, score float64) (*domain.EpicRoleScore, error)` в `internal/scoring/scoring.go`, симметрично `SetManualFinalScore`: проверка `epic.Status == domain.StatusScored` (иначе `ErrScoringNotComplete`), `repo.UpsertEpicRoleScore(ctx, epicID, roleID, score)`, без вызова `recalcParentEpic`. Проверить: `go build ./...` проходит, юнит-тест на happy path и на `ErrScoringNotComplete` при незавершённом скоринге.
- [x] 1.2 Реализовать `Service.PreviewFinalScore(ctx, epicID uuid.UUID) (float64, error)` в `internal/scoring/scoring.go`: проверка `epic.Status == domain.StatusScored`, сумма `weighted_avg` из `repo.GetEpicRoleScoresByEpicID`, умножение на произведение `RiskCoefficient(*risk.WeightedScore)` по рискам стори (`repo.GetRisksByEpicID`, только с `WeightedScore != nil`), `math.Round`. Проверить: юнит-тест воспроизводит формулу `TryCompleteEpicScoring` (сравнить на fake-репозитории с уже известным по продовому кейсу набором ролей/рисков, включая сценарий с переопределённой ролью).

## 2. Backend: HTTP-слой

- [x] 2.1 Добавить хендлер `AdminOverrideRoleScore` в `internal/transport/httpServer/handlers/admin_scores.go` (`POST /admin/scores/role`, body `{epic_id, role_id, score}`), скопировать структуру прав доступа и валидации из `AdminOverrideFinalScore` (session → superadmin/`IsTeamAdminOfAny` → точечный `IsTeamAdminOf` по team эпика → `score >= 0`), вызвать `SetManualRoleScore`, ответ `{"status":"ok","epic_id":...,"role_id":...,"weighted_avg":...}`, ошибки — стандартный формат `{"error":{"code":"...","message":"..."}}`. Проверить: запрос curl/httptest возвращает 200 и корректно обновляет `epic_role_scores`; 400 при `ErrScoringNotComplete`; 403 для team-admin чужой команды.
- [x] 2.2 Добавить хендлер `GetFinalScorePreview` (`GET /admin/scores/{epic_id}/recalc-preview`), та же проверка прав (без изменения данных), вызвать `PreviewFinalScore`, ответ `{"epic_id":...,"final_score":...}`. Проверить: httptest-тест возвращает ожидаемое расчётное значение и не изменяет `epics.final_score`.
- [x] 2.3 Зарегистрировать оба роута в `internal/transport/httpServer/routers/routers.go` в существующей admin-only группе (рядом с `/admin/scores/final`). Проверить: `go build ./...`, маршруты видны в списке роутов chi (или ручной smoke-тест через curl с валидным admin-токеном).

## 3. Frontend: переопределение оценки роли

- [x] 3.1 В `web/gantt/js/scoring-panel.js` добавить в `renderRoleScoresTableRows` третью колонку с полем ввода и кнопкой «Переопределить», видимую только при тех же admin-правах, что и остальные элементы переопределения на этой панели. Проверить: `node --input-type=module --check` проходит, кнопка не рендерится для обычного участника.
- [x] 3.2 Реализовать обработчик клика: `apiPost('/admin/scores/role', {epic_id, role_id, score})`, после успеха — перезапросить `GET /epics/{id}/role-scores` и перерисовать только таблицу ролей (без полной перезагрузки панели). Проверить вручную в браузере: переопределение роли отражается в таблице сразу после сохранения, без обновления страницы.

## 4. Frontend: пересчёт по формуле

- [x] 4.1 Добавить кнопку «Пересчитать по формуле» рядом с существующим блоком переопределения финальной оценки стори (`scoring-panel.js`, ~строки 624-628), видимую при тех же условиях, что и сама кнопка «Переопределить». Проверить: `node --input-type=module --check` проходит.
- [x] 4.2 Реализовать обработчик: `apiGet('/admin/scores/{epic_id}/recalc-preview')`, результат `final_score` подставляется в `#input-override-final-score` без автоматической отправки. Проверить вручную в браузере: после нажатия кнопки поле заполняется расчётным значением, финальная оценка стори не меняется, пока не нажата «Переопределить».

## 5. QA

- [x] 5.1 Прогнать `go test ./...` (включая новые тесты 1.1/1.2/2.1/2.2) и убедиться, что покрытие `internal/scoring` и `internal/transport/httpServer/handlers` не упало.
- [x] 5.2 Ручная проверка сценариев из specs/role-score-override/spec.md на реальных данных: переопределение роли, попытка переопределения на незавершённом скоринге, пересчёт по формуле с учётом переопределённой роли, права team-admin чужой команды.
