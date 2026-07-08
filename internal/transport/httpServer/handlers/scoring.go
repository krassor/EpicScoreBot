package handlers

import (
	"EpicScoreBot/internal/models/domain"
	"EpicScoreBot/internal/transport/httpServer/middleware"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
)

// SubmitEpicScore submits a user's score for an epic.
func (h *GanttHandler) SubmitEpicScore(w http.ResponseWriter, r *http.Request) {
	op := "handlers.SubmitEpicScore"
	sessionData := r.Context().Value(middleware.UserSessionKey)
	if sessionData == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	session, ok := sessionData.(*middleware.UserSession)
	if !ok || session.TelegramID == "" {
		writeError(w, http.StatusUnauthorized, "invalid session")
		return
	}

	var req struct {
		EpicID string `json:"epic_id"`
		Score  int    `json:"score"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	epicUUID, err := uuid.Parse(req.EpicID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid epic_id")
		return
	}

	if req.Score <= 0 {
		writeError(w, http.StatusBadRequest, "score must be greater than 0")
		return
	}

	// 1. Find user by Telegram ID
	user, err := h.repo.FindUserByTelegramID(r.Context(), session.TelegramID)
	if err != nil || user == nil {
		h.log.Error("failed to find user by telegram id", slog.String("op", op), slog.String("tg_id", session.TelegramID))
		writeError(w, http.StatusForbidden, "user not registered in system")
		return
	}

	// 2. Find user's role
	role, err := h.repo.GetRoleByUserID(r.Context(), user.ID)
	if err != nil || role == nil {
		h.log.Error("user has no role assigned", slog.String("op", op), slog.String("user_id", user.ID.String()))
		writeError(w, http.StatusForbidden, "user has no role assigned")
		return
	}

	// 3. Create/update score
	if err := h.repo.CreateEpicScore(r.Context(), epicUUID, user.ID, role.ID, req.Score); err != nil {
		h.log.Error("failed to create epic score", slog.String("op", op), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, fmt.Errorf("%s: %w", op, err).Error())
		return
	}

	// 4. Try complete scoring
	if err := h.scoring.TryCompleteEpicScoring(r.Context(), epicUUID); err != nil {
		h.log.Error("failed to run TryCompleteEpicScoring", slog.String("op", op), slog.String("error", err.Error()))
		// Do not fail the request if cascade complete fails, but log it
	}

	// 5. Gather statistics
	epic, err := h.repo.GetEpicByID(r.Context(), epicUUID)
	var totalMembers int
	var receivedScores int
	if err == nil && epic != nil {
		totalMembers, _ = h.repo.CountTeamMembers(r.Context(), epic.TeamID)
		receivedScores, _ = h.repo.CountEpicScores(r.Context(), epicUUID)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":           "ok",
		"scoring_complete": epic != nil && epic.Status == domain.StatusScored,
		"scores_received":  receivedScores,
		"scores_expected":  totalMembers,
	})
}

// SubmitRiskScore submits a user's assessment of a risk.
func (h *GanttHandler) SubmitRiskScore(w http.ResponseWriter, r *http.Request) {
	op := "handlers.SubmitRiskScore"
	sessionData := r.Context().Value(middleware.UserSessionKey)
	if sessionData == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	session, ok := sessionData.(*middleware.UserSession)
	if !ok || session.TelegramID == "" {
		writeError(w, http.StatusUnauthorized, "invalid session")
		return
	}

	var req struct {
		RiskID      string `json:"risk_id"`
		Probability int    `json:"probability"`
		Impact      int    `json:"impact"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	riskUUID, err := uuid.Parse(req.RiskID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid risk_id")
		return
	}

	if req.Probability < 1 || req.Probability > 4 || req.Impact < 1 || req.Impact > 4 {
		writeError(w, http.StatusBadRequest, "probability and impact must be between 1 and 4")
		return
	}

	// 1. Find user by Telegram ID
	user, err := h.repo.FindUserByTelegramID(r.Context(), session.TelegramID)
	if err != nil || user == nil {
		h.log.Error("failed to find user by telegram id", slog.String("op", op), slog.String("tg_id", session.TelegramID))
		writeError(w, http.StatusForbidden, "user not registered in system")
		return
	}

	// 2. Create/update score
	if err := h.repo.CreateRiskScore(r.Context(), riskUUID, user.ID, req.Probability, req.Impact); err != nil {
		h.log.Error("failed to create risk score", slog.String("op", op), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, fmt.Errorf("%s: %w", op, err).Error())
		return
	}

	// 3. Try complete scoring
	if err := h.scoring.TryCompleteRiskScoring(r.Context(), riskUUID); err != nil {
		h.log.Error("failed to run TryCompleteRiskScoring", slog.String("op", op), slog.String("error", err.Error()))
		// Do not fail the request but log it
	}

	// 4. Gather statistics
	risk, err := h.repo.GetRiskByID(r.Context(), riskUUID)
	var totalMembers int
	var receivedScores int
	if err == nil && risk != nil {
		epic, errEpic := h.repo.GetEpicByID(r.Context(), risk.EpicID)
		if errEpic == nil && epic != nil {
			totalMembers, _ = h.repo.CountTeamMembers(r.Context(), epic.TeamID)
		}
		receivedScores, _ = h.repo.CountRiskScores(r.Context(), riskUUID)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":           "ok",
		"scoring_complete": risk != nil && risk.Status == domain.StatusScored,
		"scores_received":  receivedScores,
		"scores_expected":  totalMembers,
	})
}

// GetMyScores returns all scores submitted by the currently logged-in user.
func (h *GanttHandler) GetMyScores(w http.ResponseWriter, r *http.Request) {
	op := "handlers.GetMyScores"
	sessionData := r.Context().Value(middleware.UserSessionKey)
	if sessionData == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	session, ok := sessionData.(*middleware.UserSession)
	if !ok || session.TelegramID == "" {
		writeError(w, http.StatusUnauthorized, "invalid session")
		return
	}

	// Find user by Telegram ID
	user, err := h.repo.FindUserByTelegramID(r.Context(), session.TelegramID)
	if err != nil || user == nil {
		writeError(w, http.StatusForbidden, "user not registered")
		return
	}

	epicScores, err := h.repo.GetEpicScoresByUserID(r.Context(), user.ID)
	if err != nil {
		h.log.Error("failed to get user epic scores", slog.String("op", op), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to get epic scores")
		return
	}

	riskScores, err := h.repo.GetRiskScoresByUserID(r.Context(), user.ID)
	if err != nil {
		h.log.Error("failed to get user risk scores", slog.String("op", op), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to get risk scores")
		return
	}

	type epicScoreResp struct {
		EpicID    string `json:"epic_id"`
		Score     int    `json:"score"`
		RoleID    string `json:"role_id"`
		CreatedAt string `json:"created_at"`
	}

	type riskScoreResp struct {
		RiskID      string `json:"risk_id"`
		Probability int    `json:"probability"`
		Impact      int    `json:"impact"`
		CreatedAt   string `json:"created_at"`
	}

	var epics []epicScoreResp
	for _, es := range epicScores {
		epics = append(epics, epicScoreResp{
			EpicID:    es.EpicID.String(),
			Score:     es.Score,
			RoleID:    es.RoleID.String(),
			CreatedAt: es.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	var risks []riskScoreResp
	for _, rs := range riskScores {
		risks = append(risks, riskScoreResp{
			RiskID:      rs.RiskID.String(),
			Probability: rs.Probability,
			Impact:      rs.Impact,
			CreatedAt:   rs.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":     user.ID.String(),
		"epic_scores": epics,
		"risk_scores": risks,
	})
}
