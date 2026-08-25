package handlers

import (
	"EpicScoreBot/internal/models/domain"
	"EpicScoreBot/internal/scoring"
	"EpicScoreBot/internal/transport/httpServer/middleware"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
)

// AdminSubmitEpicScore позволяет администратору проставить оценку эпика вместо участника.
func (h *GanttHandler) AdminSubmitEpicScore(w http.ResponseWriter, r *http.Request) {
	op := "handlers.AdminSubmitEpicScore"
	h.log.Info("admin submitting epic score", slog.String("op", op))

	// 1. Проверка роли admin/superadmin инициатора запроса
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

	isSuper := isSuperAdminSession(session, &h.cfg)
	if !isSuper {
		// Грубый гейт: superadmin ИЛИ team-admin хотя бы одной команды.
		isAdminAny, err := h.repo.IsTeamAdminOfAny(r.Context(), session.TelegramID)
		if err != nil || !isAdminAny {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
	}

	// 2. Декодирование тела запроса
	var req struct {
		EpicID string `json:"epic_id"`
		UserID string `json:"user_id"`
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

	userUUID, err := uuid.Parse(req.UserID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user_id")
		return
	}

	if req.Score < 0 || req.Score > 500 {
		writeError(w, http.StatusBadRequest, "score must be between 0 and 500")
		return
	}

	// Точечная проверка: team-admin может проставлять оценку только в
	// эпиках своей команды (superadmin — без ограничения).
	epicForScope, err := h.repo.GetEpicByID(r.Context(), epicUUID)
	if err != nil || epicForScope == nil {
		writeError(w, http.StatusNotFound, "epic not found")
		return
	}
	if !isSuper {
		isAdminOf, err := h.repo.IsTeamAdminOf(r.Context(), session.TelegramID, epicForScope.TeamID)
		if err != nil || !isAdminOf {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
	}

	// 3. Получение целевого пользователя
	user, err := h.repo.GetUserByID(r.Context(), userUUID)
	if err != nil || user == nil {
		h.log.Error("failed to find user by id", slog.String("op", op), slog.String("user_id", req.UserID))
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	// 4. Получение роли целевого пользователя
	role, err := h.repo.GetRoleByUserID(r.Context(), user.ID)
	if err != nil || role == nil {
		h.log.Error("target user has no role assigned", slog.String("op", op), slog.String("user_id", user.ID.String()))
		writeError(w, http.StatusBadRequest, "target user has no role assigned")
		return
	}

	// 5. Создание/обновление оценки в репозитории
	if err := h.repo.CreateEpicScore(r.Context(), epicUUID, user.ID, role.ID, req.Score); err != nil {
		h.log.Error("failed to create epic score", slog.String("op", op), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, fmt.Errorf("%s: %w", op, err).Error())
		return
	}

	// 6. Попытка завершения оценки эпика
	if err := h.scoring.TryCompleteEpicScoring(r.Context(), epicUUID); err != nil {
		h.log.Error("failed to run TryCompleteEpicScoring", slog.String("op", op), slog.String("error", err.Error()))
	}

	// 7. Сбор статистики для ответа
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

// AdminSubmitRiskScore позволяет администратору проставить оценку риска вместо участника.
func (h *GanttHandler) AdminSubmitRiskScore(w http.ResponseWriter, r *http.Request) {
	op := "handlers.AdminSubmitRiskScore"
	h.log.Info("admin submitting risk score", slog.String("op", op))

	// 1. Проверка роли admin/superadmin инициатора запроса
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

	isSuper := isSuperAdminSession(session, &h.cfg)
	if !isSuper {
		isAdminAny, err := h.repo.IsTeamAdminOfAny(r.Context(), session.TelegramID)
		if err != nil || !isAdminAny {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
	}

	// 2. Декодирование тела запроса
	var req struct {
		RiskID      string `json:"risk_id"`
		UserID      string `json:"user_id"`
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

	userUUID, err := uuid.Parse(req.UserID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user_id")
		return
	}

	if req.Probability < 1 || req.Probability > 4 || req.Impact < 1 || req.Impact > 4 {
		writeError(w, http.StatusBadRequest, "probability and impact must be between 1 and 4")
		return
	}

	// Точечная проверка: team-admin может проставлять оценку риска только в
	// эпиках своей команды (superadmin — без ограничения).
	riskForScope, err := h.repo.GetRiskByID(r.Context(), riskUUID)
	if err != nil || riskForScope == nil {
		writeError(w, http.StatusNotFound, "risk not found")
		return
	}
	if !isSuper {
		riskEpic, err := h.repo.GetEpicByID(r.Context(), riskForScope.EpicID)
		if err != nil || riskEpic == nil {
			writeError(w, http.StatusNotFound, "epic not found")
			return
		}
		isAdminOf, err := h.repo.IsTeamAdminOf(r.Context(), session.TelegramID, riskEpic.TeamID)
		if err != nil || !isAdminOf {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
	}

	// 3. Получение целевого пользователя
	user, err := h.repo.GetUserByID(r.Context(), userUUID)
	if err != nil || user == nil {
		h.log.Error("failed to find user by id", slog.String("op", op), slog.String("user_id", req.UserID))
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	// 4. Создание/обновление оценки риска в репозитории
	if err := h.repo.CreateRiskScore(r.Context(), riskUUID, user.ID, req.Probability, req.Impact); err != nil {
		h.log.Error("failed to create risk score", slog.String("op", op), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, fmt.Errorf("%s: %w", op, err).Error())
		return
	}

	// 5. Попытка завершения оценки риска
	if err := h.scoring.TryCompleteRiskScoring(r.Context(), riskUUID); err != nil {
		h.log.Error("failed to run TryCompleteRiskScoring", slog.String("op", op), slog.String("error", err.Error()))
	}

	// 6. Сбор статистики для ответа
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

// AdminOverrideFinalScore позволяет администратору вручную переопределить итоговую
// оценку (final_score) уже полностью оцененного эпика/стори (статус SCORED).
// Если у эпика есть родитель (это стори), каскадно пересчитывает агрегированную
// оценку родительского эпика.
func (h *GanttHandler) AdminOverrideFinalScore(w http.ResponseWriter, r *http.Request) {
	op := "handlers.AdminOverrideFinalScore"
	h.log.Info("admin overriding final score", slog.String("op", op))

	// 1. Проверка роли admin/superadmin инициатора запроса
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

	isSuper := isSuperAdminSession(session, &h.cfg)
	if !isSuper {
		isAdminAny, err := h.repo.IsTeamAdminOfAny(r.Context(), session.TelegramID)
		if err != nil || !isAdminAny {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
	}

	// 2. Декодирование тела запроса
	var req struct {
		EpicID     string  `json:"epic_id"`
		FinalScore float64 `json:"final_score"`
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

	if req.FinalScore < 0 {
		writeError(w, http.StatusBadRequest, "final_score must be >= 0")
		return
	}

	// Точечная проверка: team-admin может переопределять итоговую оценку
	// только в эпиках своей команды (superadmin — без ограничения).
	if !isSuper {
		epicForScope, err := h.repo.GetEpicByID(r.Context(), epicUUID)
		if err != nil || epicForScope == nil {
			writeError(w, http.StatusNotFound, "epic not found")
			return
		}
		isAdminOf, err := h.repo.IsTeamAdminOf(r.Context(), session.TelegramID, epicForScope.TeamID)
		if err != nil || !isAdminOf {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
	}

	// 3. Применение ручного переопределения итоговой оценки
	epic, err := h.scoring.SetManualFinalScore(r.Context(), epicUUID, req.FinalScore)
	if err != nil {
		if errors.Is(err, scoring.ErrScoringNotComplete) {
			writeError(w, http.StatusBadRequest, "story scoring is not completed yet")
			return
		}
		h.log.Error("failed to set manual final score", slog.String("op", op), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, fmt.Errorf("%s: %w", op, err).Error())
		return
	}

	var parentEpicID any
	if epic.ParentEpicID != nil {
		parentEpicID = epic.ParentEpicID.String()
	}

	finalScore := req.FinalScore
	if epic.FinalScore != nil {
		finalScore = *epic.FinalScore
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "ok",
		"final_score":    finalScore,
		"epic_id":        epicUUID.String(),
		"parent_epic_id": parentEpicID,
	})
}
