## Context

См. proposal.md - Why. Технический контекст (изучен напрямую, без предположений):

- Финальная оценка стори считается в `internal/scoring/scoring.go:176-276` (`TryCompleteEpicScoring`): для каждой роли — взвешенное среднее оценок участников (`CalculateEpicRoleAvg`, upsert в `epic_role_scores.weighted_avg` через `UpsertEpicRoleScore` — таблица `epic_role_scores`, уникальный ключ `(epic_id, role_id)`, upsert уже реализован); `epicBaseScore = Σ(weighted_avg по всем ролям)`; `finalScore = epicBaseScore × Π(RiskCoefficient(risk.WeightedScore))` по всем рискам стори; округление до целого (`math.Round`). Это и есть формула «как в отчёте».
- Переопределение финальной оценки стори (`SetManualFinalScore`, `scoring.go:355-390`) требует `epic.Status == StatusScored`, напрямую делает `UPDATE epics.final_score` и каскадно пересчитывает родителя (`recalcParentEpic`). Никакого отдельного флага «переопределено» нет — это прямая перезапись поля, тот же паттерн будет использован для роли.
- `GET /epics/{epic_id}/role-scores` (`GetEpicRoleScores`, `internal/transport/httpServer/handlers/gantt.go:920-957`) читает ровно то, что есть в `epic_role_scores` для эпика (`SELECT ... WHERE epic_id = $1`, без JOIN с ролями команды) — строка появляется в таблице только если хотя бы один участник этой роли уже проголосовал. У полностью оценённой (`SCORED`) стори строки есть для всех ролей, участвовавших в скоринге.
- Admin-only эндпоинты переопределения (`POST /admin/scores/final`, `POST /admin/scores/epic`, `POST /admin/scores/risk`) зарегистрированы в одной chi-группе `routers.go` (~строки 61-70) под `RoleAuth(..., "admin")`; сам хендлер `AdminOverrideFinalScore` (`admin_scores.go:248-342`) дополнительно точечно проверяет `IsTeamAdminOf` для team-admin (не superadmin) — эту же двухуровневую проверку прав повторяем для новых хендлеров.

## Goals / Non-Goals

**Goals:**
- Переопределение оценки роли — простая прямая перезапись `epic_role_scores.weighted_avg`, без нового флага/колонки и без побочных пересчётов.
- Предпросмотр по формуле — чистый расчёт без записи в БД, переиспользующий ту же формулу, что и `TryCompleteEpicScoring`, но по уже сохранённым `epic_role_scores` (а не пересчитывая их заново из `epic_scores` участников).

**Non-Goals:**
- Не переопределяем/не защищаем oт последующего перезатирания: если после переопределения роли или финальной оценки кто-то ещё раз вызовет `AdminSubmitEpicScore` (правка оценки участника), `TryCompleteEpicScoring` пересчитает и `epic_role_scores`, и `epics.final_score` заново, затерев оба переопределения. Это уже существующее поведение `SetManualFinalScore` (не new regression), сознательно не меняем в рамках этого change.
- Не кешируем/не versioning переопределений — как и у `SetManualFinalScore`, нет истории/аудита кто и когда переопределил.
- Не меняем формат ошибок API (уже стандартизирован, см. context).

## Decisions

**1. Переопределение роли — новый метод `Service.SetManualRoleScore`, симметричный `SetManualFinalScore`.**
Требует `epic.Status == domain.StatusScored` (иначе `scoring.ErrScoringNotComplete`, как у финальной оценки), затем `repo.UpsertEpicRoleScore(ctx, epicID, roleID, score)` — тот же repo-метод, что уже использует `TryCompleteEpicScoring`. Не вызывает `recalcParentEpic` и не трогает `epics.final_score` — переопределение роли и переопределение финальной оценки стори остаются независимыми явными действиями (см. proposal.md).
Альтернатива (отклонена): автоматически пересчитывать финальную оценку стори при переопределении роли. Отклонено, т.к. пользователь явно хочет двухшаговый процесс — переопределить роль(и), затем через «Пересчитать по формуле» проверить и осознанно сохранить итог.

**2. Предпросмотр по формуле — новый метод `Service.PreviewFinalScore(ctx, epicID) (float64, error)`.**
Читает `repo.GetEpicRoleScoresByEpicID` (сумма `weighted_avg`, уже включает переопределения ролей) + `repo.GetRisksByEpicID` (произведение `RiskCoefficient(*risk.WeightedScore)` по рискам с ненулевым `WeightedScore`), `math.Round(baseScore × riskCoeffProduct)`. Требует `epic.Status == domain.StatusScored` (иначе `ErrScoringNotComplete`) — предпросмотр на незавершённом скоринге не имеет смысла и данные могут быть неполными.
Альтернатива (отклонена): переиспользовать код `capacity_report.go` (`BuildCapacityReport`). Отклонена — формула отчёта распределяет уже известную `FinalScore` по ролям пропорционально (`riskFactor = FinalScore/baseScore`), это обратная задача; для предпросмотра нужна прямая формула из `TryCompleteEpicScoring`, а не производная от текущего (возможно, уже переопределённого) `FinalScore`.

**3. Два новых хендлера в `admin_scores.go`, два новых роута в существующей admin-only группе `routers.go`.**
`AdminOverrideRoleScore` (`POST /admin/scores/role`, body `{epic_id, role_id, score}`) — структура прав/валидации копирует `AdminOverrideFinalScore` (session → isSuper/IsTeamAdminOfAny → точечный `IsTeamAdminOf` по `epicForScope.TeamID` → `score >= 0`). Ответ: `{"status":"ok","epic_id":...,"role_id":...,"weighted_avg":...}`.
`GetFinalScorePreview` (`GET /admin/scores/{epic_id}/recalc-preview`) — та же проверка прав (без модификации данных, но эндпоинт всё равно admin-only группа, т.к. видим его только из UI-блока переопределения, доступного лишь админам). Ответ: `{"epic_id":...,"final_score":...}`.
Оба используют стандартный формат ошибок `{"error":{"code":"...","message":"..."}}`.

**4. Frontend: строка роли получает inline input + кнопку, видимые только при том же условии admin-прав, что и у существующей кнопки «Изменить»/«Переопределить» на этой панели.**
`renderRoleScoresTableRows` (`scoring-panel.js:415-425`) — добавить 3-ю колонку. Обработчик по клику — `apiPost('/admin/scores/role', {epic_id, role_id, score})`, затем перерисовать только таблицу ролей (повторный `GET /epics/{id}/role-scores`), без полной перезагрузки панели.
Кнопка «Пересчитать по формуле» — рядом с существующим блоком переопределения финальной оценки (`scoring-panel.js:624-628`), обработчик — `apiGet('/admin/scores/{epic_id}/recalc-preview')` → устанавливает `value` инпута `#input-override-final-score`, не вызывая сохранение.

## Risks / Trade-offs

- [Риск] Переопределение роли молча теряется при следующей правке оценки участника (см. Non-Goals) → Митигация: не new regression (симметрично уже существующему поведению `final_score`), но стоит явно упомянуть в тексте/тултипе кнопки на фронте, что переопределение может быть пересчитано, если оценки участников изменятся повторно (мелкая деталь, на усмотрение frontend-агента при реализации).
- [Риск] Расхождение между `PreviewFinalScore` и фактическим следующим вызовом `TryCompleteEpicScoring` (если оценки участников поменяются между предпросмотром и сохранением) → Митигация: предпросмотр — это значение здесь и сейчас по текущим данным, не гарантия на будущее; пользователь явно нажимает «Переопределить» осознанно, как и раньше.

## Open Questions

(нет — все технические решения приняты выше)
