package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// GetTeamScheduleSettings возвращает текущее значение настройки команды «не
// занимать промежутки расписания, оставшиеся в прошлом» (backend §3.1,
// openspec/changes/backfill-idle-gaps-in-schedule) — источник состояния
// переключателя на вкладке «Гант». Доступен любому аутентифицированному
// пользователю, как и соседний GetTeamMembers: само чтение ничего не
// меняет, редактирование гейтится отдельно в SetTeamScheduleSettings.
func (h *GanttHandler) GetTeamScheduleSettings(w http.ResponseWriter, r *http.Request) {
	teamIDStr := chi.URLParam(r, "id")
	teamID, err := uuid.Parse(teamIDStr)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_TEAM_ID", "invalid team id")
		return
	}

	team, err := h.repo.GetTeamByID(r.Context(), teamID)
	if err != nil {
		writeErrorCode(w, http.StatusNotFound, "TEAM_NOT_FOUND", "team not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"backfill_block_past": team.BackfillBlockPast,
	})
}

// SetTeamScheduleSettings переключает настройку команды «не занимать
// промежутки расписания, оставшиеся в прошлом» (backend §3.2) и
// пересчитывает расписание команды по новому правилу — того же требует
// спека (Requirement "Команда может запретить занимать промежутки в
// прошлом", Scenario "Администратор переключает настройку": "значение
// сохраняется и расписание команды пересчитывается по новому правилу").
//
// Доступ — только администратор команды, которой принадлежит настройка
// (или superadmin); участник с ролью member получает отказ, значение не
// меняется (Scenario "Пользователь без права редактирования"). Тот же
// приём двухуровневой проверки, что и SetTaskAssignee (admin.go:372,
// admin_scores.go:81): RoleAuth("admin") на уровне группы маршрутов
// (routers.go) — грубый гейт "admin хотя бы одной команды", и здесь же,
// внутри хендлера, — team-scoped IsTeamAdminOf, чтобы admin команды A не
// мог переключить настройку команды B.
func (h *GanttHandler) SetTeamScheduleSettings(w http.ResponseWriter, r *http.Request) {
	teamIDStr := chi.URLParam(r, "id")
	teamID, err := uuid.Parse(teamIDStr)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_TEAM_ID", "invalid team id")
		return
	}

	session, role, ok := h.sessionRole(w, r)
	if !ok {
		return
	}
	if role == "member" {
		writeErrorCode(w, http.StatusForbidden, "FORBIDDEN",
			"только администратор команды может изменять настройки расписания")
		return
	}

	var req struct {
		BackfillBlockPast bool `json:"backfill_block_past"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_REQUEST_BODY", "invalid request body")
		return
	}

	if _, err := h.repo.GetTeamByID(r.Context(), teamID); err != nil {
		writeErrorCode(w, http.StatusNotFound, "TEAM_NOT_FOUND", "team not found")
		return
	}

	if role != "superadmin" {
		isAdminOf, err := h.repo.IsTeamAdminOf(r.Context(), session.TelegramID, teamID)
		if err != nil || !isAdminOf {
			writeErrorCode(w, http.StatusForbidden, "FORBIDDEN",
				"вы не администратор этой команды")
			return
		}
	}

	tasks, err := h.svc.SetTeamBackfillBlockPast(r.Context(), teamID, req.BackfillBlockPast)
	if err != nil {
		h.log.Error("failed to set team schedule settings", slog.String("error", err.Error()))
		writeErrorCode(w, http.StatusInternalServerError, "RESCHEDULE_FAILED",
			"failed to update schedule settings")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message":             "schedule settings updated",
		"backfill_block_past": req.BackfillBlockPast,
		"count":               len(tasks),
	})
}
