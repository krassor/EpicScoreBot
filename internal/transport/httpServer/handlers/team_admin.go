package handlers

import (
	"EpicScoreBot/internal/transport/httpServer/middleware"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
)

// Хендлеры управления привязками team_admins (many-to-many user↔team).
// Монтируются в отдельную подгруппу с middleware.RoleAuth(..., "superadmin")
// (см. routers.go) — доступны только superadmin.

// GetTeamAdmins возвращает список team-admin указанной команды.
func (h *GanttHandler) GetTeamAdmins(w http.ResponseWriter, r *http.Request) {
	op := "handlers.GetTeamAdmins"

	teamIDStr := r.URL.Query().Get("team_id")
	teamID, err := uuid.Parse(teamIDStr)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_team_id", "invalid team_id")
		return
	}

	admins, err := h.repo.GetTeamAdminsByTeamID(r.Context(), teamID)
	if err != nil {
		h.log.Error("failed to get team admins", slog.String("op", op), slog.String("error", err.Error()))
		writeErrorCode(w, http.StatusInternalServerError, "internal_error", "failed to get team admins")
		return
	}

	type adminResp struct {
		ID         string `json:"id"`
		TelegramID string `json:"telegram_id"`
		FirstName  string `json:"first_name"`
		LastName   string `json:"last_name"`
	}
	resp := make([]adminResp, 0, len(admins))
	for _, a := range admins {
		resp = append(resp, adminResp{
			ID:         a.ID.String(),
			TelegramID: a.TelegramID,
			FirstName:  a.FirstName,
			LastName:   a.LastName,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"team_id": teamID.String(),
		"admins":  resp,
	})
}

// AssignTeamAdmin назначает пользователя администратором команды.
func (h *GanttHandler) AssignTeamAdmin(w http.ResponseWriter, r *http.Request) {
	op := "handlers.AssignTeamAdmin"

	var req struct {
		UserID string `json:"user_id"`
		TeamID string `json:"team_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_body", "invalid request body")
		return
	}

	userUUID, err1 := uuid.Parse(req.UserID)
	teamUUID, err2 := uuid.Parse(req.TeamID)
	if err1 != nil || err2 != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_id", "invalid user_id or team_id")
		return
	}

	if u, err := h.repo.GetUserByID(r.Context(), userUUID); err != nil || u == nil {
		writeErrorCode(w, http.StatusNotFound, "user_not_found", "user not found")
		return
	}
	if t, err := h.repo.GetTeamByID(r.Context(), teamUUID); err != nil || t == nil {
		writeErrorCode(w, http.StatusNotFound, "team_not_found", "team not found")
		return
	}

	// assigned_by — вызывающий superadmin; superadmin не обязан быть строкой
	// в users, поэтому uuid.Nil (не найден) сохраняется как NULL, см.
	// Repository.AssignTeamAdmin.
	var assignedBy uuid.UUID
	if session := userSessionFrom(r); session != nil {
		if superadmin, err := h.repo.FindUserByTelegramID(r.Context(), session.TelegramID); err == nil && superadmin != nil {
			assignedBy = superadmin.ID
		}
	}

	if err := h.repo.AssignTeamAdmin(r.Context(), userUUID, teamUUID, assignedBy); err != nil {
		h.log.Error("failed to assign team admin", slog.String("op", op), slog.String("error", err.Error()))
		writeErrorCode(w, http.StatusInternalServerError, "internal_error", "failed to assign team admin")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// RemoveTeamAdmin снимает привязку team-admin пользователя к команде.
func (h *GanttHandler) RemoveTeamAdmin(w http.ResponseWriter, r *http.Request) {
	op := "handlers.RemoveTeamAdmin"

	var req struct {
		UserID string `json:"user_id"`
		TeamID string `json:"team_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_body", "invalid request body")
		return
	}

	userUUID, err1 := uuid.Parse(req.UserID)
	teamUUID, err2 := uuid.Parse(req.TeamID)
	if err1 != nil || err2 != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_id", "invalid user_id or team_id")
		return
	}

	if err := h.repo.RemoveTeamAdmin(r.Context(), userUUID, teamUUID); err != nil {
		h.log.Error("failed to remove team admin", slog.String("op", op), slog.String("error", err.Error()))
		writeErrorCode(w, http.StatusInternalServerError, "internal_error", "failed to remove team admin")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// userSessionFrom извлекает UserSession из контекста запроса, если он там
// есть (защитно — эти хендлеры уже стоят за middleware.TelegramAuth +
// RoleAuth("superadmin"), сессия гарантированно валидна).
func userSessionFrom(r *http.Request) *middleware.UserSession {
	sessionData := r.Context().Value(middleware.UserSessionKey)
	if sessionData == nil {
		return nil
	}
	session, ok := sessionData.(*middleware.UserSession)
	if !ok {
		return nil
	}
	return session
}
