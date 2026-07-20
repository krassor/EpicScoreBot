package handlers

import (
	"EpicScoreBot/internal/models/domain"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// BulkCreateUsers imports multiple users.
func (h *GanttHandler) BulkCreateUsers(w http.ResponseWriter, r *http.Request) {
	op := "handlers.BulkCreateUsers"
	h.log.Info("executing bulk user import", slog.String("op", op))

	var req struct {
		Users []struct {
			TelegramID string `json:"telegram_id"`
			FirstName  string `json:"first_name"`
			LastName   string `json:"last_name"`
			Weight     int    `json:"weight"`
		} `json:"users"`
		CSV    string `json:"csv"`
		TeamID string `json:"team_id"`
		RoleID string `json:"role_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Error("failed to decode request body", slog.String("op", op), slog.String("error", err.Error()))
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var teamUUID *uuid.UUID
	if req.TeamID != "" {
		tid, err := uuid.Parse(req.TeamID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid team_id")
			return
		}
		teamUUID = &tid
	}

	var roleUUID *uuid.UUID
	if req.RoleID != "" {
		rid, err := uuid.Parse(req.RoleID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid role_id")
			return
		}
		roleUUID = &rid
	}

	var domainUsers []domain.User

	// Process JSON users
	for _, u := range req.Users {
		if u.TelegramID == "" || u.FirstName == "" {
			continue
		}
		weight := u.Weight
		if weight <= 0 {
			weight = 100 // default weight
		}
		domainUsers = append(domainUsers, domain.User{
			ID:         uuid.New(),
			TelegramID: u.TelegramID,
			FirstName:  u.FirstName,
			LastName:   u.LastName,
			Weight:     weight,
		})
	}

	// Process CSV users (format: telegram_id;first_name;last_name;weight)
	if req.CSV != "" {
		lines := strings.Split(req.CSV, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.Split(line, ";")
			if len(parts) < 2 {
				// try comma if semicolon not found
				parts = strings.Split(line, ",")
			}
			if len(parts) < 2 {
				continue
			}

			telegramID := strings.TrimSpace(parts[0])
			firstName := strings.TrimSpace(parts[1])
			lastName := ""
			if len(parts) > 2 {
				lastName = strings.TrimSpace(parts[2])
			}
			weight := 100
			if len(parts) > 3 {
				wVal, err := strconv.Atoi(strings.TrimSpace(parts[3]))
				if err == nil && wVal > 0 {
					weight = wVal
				}
			}

			if telegramID != "" && firstName != "" {
				domainUsers = append(domainUsers, domain.User{
					ID:         uuid.New(),
					TelegramID: telegramID,
					FirstName:  firstName,
					LastName:   lastName,
					Weight:     weight,
				})
			}
		}
	}

	if len(domainUsers) == 0 {
		writeError(w, http.StatusBadRequest, "no valid users provided")
		return
	}

	if err := h.repo.BulkCreateUsers(r.Context(), domainUsers, teamUUID, roleUUID); err != nil {
		h.log.Error("failed to bulk create users", slog.String("op", op), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, fmt.Errorf("%s: %w", op, err).Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"status": "ok",
		"count":  len(domainUsers),
	})
}

// AddTeam creates a new team.
func (h *GanttHandler) AddTeam(w http.ResponseWriter, r *http.Request) {
	op := "handlers.AddTeam"
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "team name is required")
		return
	}

	team, err := h.repo.CreateTeam(r.Context(), req.Name, req.Description)
	if err != nil {
		h.log.Error("failed to create team", slog.String("op", op), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, fmt.Errorf("%s: %w", op, err).Error())
		return
	}

	writeJSON(w, http.StatusCreated, team)
}

// AssignUserTeam assigns a user to a team.
func (h *GanttHandler) AssignUserTeam(w http.ResponseWriter, r *http.Request) {
	op := "handlers.AssignUserTeam"
	var req struct {
		UserID string `json:"user_id"`
		TeamID string `json:"team_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	userUUID, err1 := uuid.Parse(req.UserID)
	teamUUID, err2 := uuid.Parse(req.TeamID)
	if err1 != nil || err2 != nil {
		writeError(w, http.StatusBadRequest, "invalid user_id or team_id")
		return
	}

	if err := h.repo.AssignUserTeam(r.Context(), userUUID, teamUUID); err != nil {
		h.log.Error("failed to assign user to team", slog.String("op", op), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, fmt.Errorf("%s: %w", op, err).Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// RemoveUserTeam removes a user from a team.
func (h *GanttHandler) RemoveUserTeam(w http.ResponseWriter, r *http.Request) {
	op := "handlers.RemoveUserTeam"
	var req struct {
		UserID string `json:"user_id"`
		TeamID string `json:"team_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	userUUID, err1 := uuid.Parse(req.UserID)
	teamUUID, err2 := uuid.Parse(req.TeamID)
	if err1 != nil || err2 != nil {
		writeError(w, http.StatusBadRequest, "invalid user_id or team_id")
		return
	}

	if err := h.repo.RemoveUserTeam(r.Context(), userUUID, teamUUID); err != nil {
		h.log.Error("failed to remove user from team", slog.String("op", op), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, fmt.Errorf("%s: %w", op, err).Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// AssignUserRole assigns a role to a user.
func (h *GanttHandler) AssignUserRole(w http.ResponseWriter, r *http.Request) {
	op := "handlers.AssignUserRole"
	var req struct {
		UserID string `json:"user_id"`
		RoleID string `json:"role_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	userUUID, err1 := uuid.Parse(req.UserID)
	roleUUID, err2 := uuid.Parse(req.RoleID)
	if err1 != nil || err2 != nil {
		writeError(w, http.StatusBadRequest, "invalid user_id or role_id")
		return
	}

	if err := h.repo.AssignUserRole(r.Context(), userUUID, roleUUID); err != nil {
		h.log.Error("failed to assign user role", slog.String("op", op), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, fmt.Errorf("%s: %w", op, err).Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// RemoveUserRole removes a role from a user.
func (h *GanttHandler) RemoveUserRole(w http.ResponseWriter, r *http.Request) {
	op := "handlers.RemoveUserRole"
	var req struct {
		UserID string `json:"user_id"`
		RoleID string `json:"role_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	userUUID, err1 := uuid.Parse(req.UserID)
	roleUUID, err2 := uuid.Parse(req.RoleID)
	if err1 != nil || err2 != nil {
		writeError(w, http.StatusBadRequest, "invalid user_id or role_id")
		return
	}

	if err := h.repo.RemoveUserRole(r.Context(), userUUID, roleUUID); err != nil {
		h.log.Error("failed to remove user role", slog.String("op", op), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, fmt.Errorf("%s: %w", op, err).Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// AddEpic creates a new epic.
func (h *GanttHandler) AddEpic(w http.ResponseWriter, r *http.Request) {
	op := "handlers.AddEpic"
	var req struct {
		Number            string   `json:"number"`
		Name              string   `json:"name"`
		Description       string   `json:"description"`
		TeamID            string   `json:"team_id"`
		Year              *int     `json:"year"`
		Quarter           *int     `json:"quarter"`
		Type              *string  `json:"type"`
		EvaluatingRoleIDs []string `json:"evaluating_role_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	teamUUID, err := uuid.Parse(req.TeamID)
	if err != nil || req.Number == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "invalid epic fields or team_id")
		return
	}

	yearVal := 2026
	if req.Year != nil {
		yearVal = *req.Year
	}
	quarterVal := 3
	if req.Quarter != nil {
		quarterVal = *req.Quarter
	}
	typeVal := "feature"
	if req.Type != nil {
		typeVal = *req.Type
	}

	var evalUUIDs []uuid.UUID
	for _, idStr := range req.EvaluatingRoleIDs {
		u, err := uuid.Parse(idStr)
		if err == nil {
			evalUUIDs = append(evalUUIDs, u)
		}
	}

	epic, err := h.repo.CreateEpic(r.Context(), req.Number, req.Name, req.Description, teamUUID, yearVal, quarterVal, typeVal, evalUUIDs)
	if err != nil {
		h.log.Error("failed to create epic", slog.String("op", op), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, fmt.Errorf("%s: %w", op, err).Error())
		return
	}

	writeJSON(w, http.StatusCreated, epic)
}

// StartEpicScoring starts the scoring stage for an epic and its risks/stories.
func (h *GanttHandler) StartEpicScoring(w http.ResponseWriter, r *http.Request) {
	op := "handlers.StartEpicScoring"
	var req struct {
		EpicID string `json:"epic_id"`
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

	epic, err := h.repo.GetEpicByID(r.Context(), epicUUID)
	if err != nil {
		writeError(w, http.StatusNotFound, "epic not found")
		return
	}

	if epic.Status != domain.StatusNew {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("epic is already in status %s", epic.Status))
		return
	}

	if epic.ParentEpicID != nil {
		writeError(w, http.StatusBadRequest, "cannot start scoring of a story directly")
		return
	}

	// 1. Проверяем наличие сторей
	stories, err := h.repo.GetStoriesByEpicID(r.Context(), epicUUID)
	if err != nil {
		h.log.Error("failed to get stories", slog.String("op", op), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to check stories")
		return
	}

	if len(stories) == 0 {
		writeError(w, http.StatusBadRequest, "epic must have at least one story")
		return
	}

	// 2. Обновляем статус родительского эпика
	if err := h.repo.UpdateEpicStatus(r.Context(), epicUUID, domain.StatusScoring); err != nil {
		h.log.Error("failed to update parent status", slog.String("op", op), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to start scoring")
		return
	}

	// 3. Запускаем скоринг для всех дочерних сторей
	for _, story := range stories {
		if err := h.repo.StartEpicScoring(r.Context(), story.ID); err != nil {
			h.log.Error("failed to start story scoring", slog.String("op", op), slog.String("story_id", story.ID.String()), slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "failed to start scoring for one of the stories")
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": "scoring started"})
}

// GetEpicScoringStatus returns details about the scoring status of an epic.
func (h *GanttHandler) GetEpicScoringStatus(w http.ResponseWriter, r *http.Request) {
	op := "handlers.GetEpicScoringStatus"
	epicIDStr := chi.URLParam(r, "id")
	epicUUID, err := uuid.Parse(epicIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid epic id")
		return
	}

	epic, err := h.repo.GetEpicByID(r.Context(), epicUUID)
	if err != nil {
		writeError(w, http.StatusNotFound, "epic not found")
		return
	}

	members, err := h.repo.GetUsersByTeamID(r.Context(), epic.TeamID)
	if err != nil {
		h.log.Error("failed to get team members", slog.String("op", op), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to get team members")
		return
	}

	epicScores, err := h.repo.GetEpicScoresByEpicID(r.Context(), epicUUID)
	if err != nil {
		h.log.Error("failed to get epic scores", slog.String("op", op), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to get epic scores")
		return
	}

	risks, err := h.repo.GetRisksByEpicID(r.Context(), epicUUID)
	if err != nil {
		h.log.Error("failed to get epic risks", slog.String("op", op), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to get epic risks")
		return
	}

	// Map score details per user
	type unscoredRiskResp struct {
		RiskID      string `json:"risk_id"`
		Description string `json:"description"`
	}

	type memberStatusResp struct {
		UserID        string             `json:"user_id"`
		FirstName     string             `json:"first_name"`
		LastName      string             `json:"last_name"`
		TelegramID    string             `json:"telegram_id"`
		Weight        int                `json:"weight"`
		RoleName      string             `json:"role_name"`
		HasScoredEpic bool               `json:"has_scored_epic"`
		EpicScore     *int               `json:"epic_score,omitempty"`
		UnscoredRisks []unscoredRiskResp `json:"unscored_risks"`
	}

	var membersResp []memberStatusResp

	for _, m := range members {
		// Find user role
		roleName := ""
		if role, err := h.repo.GetRoleByUserID(r.Context(), m.ID); err == nil && role != nil {
			roleName = role.Name
		}

		// Find user's epic score
		var scoreVal *int
		hasScoredEpic := false
		for _, es := range epicScores {
			if es.UserID == m.ID {
				val := es.Score
				scoreVal = &val
				hasScoredEpic = true
				break
			}
		}

		// Find which risks this user hasn't scored
		var unscored []unscoredRiskResp
		for _, rk := range risks {
			if rk.Status == domain.StatusScoring {
				riskScores, err := h.repo.GetRiskScoresByRiskID(r.Context(), rk.ID)
				if err != nil {
					continue
				}
				hasScoredRisk := false
				for _, rs := range riskScores {
					if rs.UserID == m.ID {
						hasScoredRisk = true
						break
					}
				}
				if !hasScoredRisk {
					unscored = append(unscored, unscoredRiskResp{
						RiskID:      rk.ID.String(),
						Description: rk.Description,
					})
				}
			}
		}

		membersResp = append(membersResp, memberStatusResp{
			UserID:        m.ID.String(),
			FirstName:     m.FirstName,
			LastName:      m.LastName,
			TelegramID:    m.TelegramID,
			Weight:        m.Weight,
			RoleName:      roleName,
			HasScoredEpic: hasScoredEpic,
			EpicScore:     scoreVal,
			UnscoredRisks: unscored,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"epic_id":      epic.ID.String(),
		"epic_number":  epic.Number,
		"epic_name":    epic.Name,
		"status":       epic.Status,
		"final_score":  epic.FinalScore,
		"team_members": membersResp,
	})
}

// AddRisk adds a risk associated with an epic.
func (h *GanttHandler) AddRisk(w http.ResponseWriter, r *http.Request) {
	op := "handlers.AddRisk"
	var req struct {
		Description string `json:"description"`
		EpicID      string `json:"epic_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	epicUUID, err := uuid.Parse(req.EpicID)
	if err != nil || req.Description == "" {
		writeError(w, http.StatusBadRequest, "invalid description or epic_id")
		return
	}

	risk, err := h.repo.CreateRisk(r.Context(), req.Description, epicUUID)
	if err != nil {
		h.log.Error("failed to create risk", slog.String("op", op), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, fmt.Errorf("%s: %w", op, err).Error())
		return
	}

	writeJSON(w, http.StatusCreated, risk)
}

// SetUserWeight updates a user's scoring weight.
func (h *GanttHandler) SetUserWeight(w http.ResponseWriter, r *http.Request) {
	op := "handlers.SetUserWeight"
	userIDStr := chi.URLParam(r, "id")
	userUUID, err := uuid.Parse(userIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	var req struct {
		Weight int `json:"weight"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Weight < 0 || req.Weight > 100 {
		writeError(w, http.StatusBadRequest, "weight must be between 0 and 100")
		return
	}

	if err := h.repo.UpdateUserWeight(r.Context(), userUUID, req.Weight); err != nil {
		h.log.Error("failed to update user weight", slog.String("op", op), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, fmt.Errorf("%s: %w", op, err).Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DeleteUser deletes a user. (SuperAdmin only)
func (h *GanttHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	op := "handlers.DeleteUser"
	userIDStr := chi.URLParam(r, "id")
	userUUID, err := uuid.Parse(userIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	if err := h.repo.DeleteUser(r.Context(), userUUID); err != nil {
		h.log.Error("failed to delete user", slog.String("op", op), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, fmt.Errorf("%s: %w", op, err).Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DeleteEpic deletes an epic. (SuperAdmin only)
func (h *GanttHandler) DeleteEpic(w http.ResponseWriter, r *http.Request) {
	op := "handlers.DeleteEpic"
	epicIDStr := chi.URLParam(r, "id")
	epicUUID, err := uuid.Parse(epicIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid epic id")
		return
	}

	if err := h.repo.DeleteEpic(r.Context(), epicUUID); err != nil {
		h.log.Error("failed to delete epic", slog.String("op", op), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, fmt.Errorf("%s: %w", op, err).Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DeleteRisk deletes a risk. (SuperAdmin only)
func (h *GanttHandler) DeleteRisk(w http.ResponseWriter, r *http.Request) {
	op := "handlers.DeleteRisk"
	riskIDStr := chi.URLParam(r, "id")
	riskUUID, err := uuid.Parse(riskIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid risk id")
		return
	}

	if err := h.repo.DeleteRisk(r.Context(), riskUUID); err != nil {
		h.log.Error("failed to delete risk", slog.String("op", op), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, fmt.Errorf("%s: %w", op, err).Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GetUsersList возвращает список всех зарегистрированных пользователей со всеми связями.
func (h *GanttHandler) GetUsersList(w http.ResponseWriter, r *http.Request) {
	op := "handlers.GetUsersList"
	h.log.Info("executing get users list", slog.String("op", op))

	users, err := h.repo.GetAllUsers(r.Context())
	if err != nil {
		h.log.Error("failed to get all users", slog.String("op", op), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to get users")
		return
	}

	type teamResp struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	type roleResp struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	type userWithRelations struct {
		ID         string     `json:"id"`
		TelegramID string     `json:"telegram_id"`
		FirstName  string     `json:"first_name"`
		LastName   string     `json:"last_name"`
		Weight     int        `json:"weight"`
		UserTeams  []teamResp `json:"user_teams"`
		UserRoles  []roleResp `json:"user_roles"`
	}

	resp := make([]userWithRelations, 0, len(users))

	for _, u := range users {
		// Получение команд пользователя
		userTeams, err := h.repo.GetUserTeams(r.Context(), u.ID)
		if err != nil {
			h.log.Error("failed to get teams for user", slog.String("op", op), slog.String("user_id", u.ID.String()), slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "failed to load user relations")
			return
		}
		teams := make([]teamResp, len(userTeams))
		for i, t := range userTeams {
			teams[i] = teamResp{ID: t.ID.String(), Name: t.Name}
		}

		// Получение ролей пользователя
		userRoles, err := h.repo.GetUserRoles(r.Context(), u.ID)
		if err != nil {
			h.log.Error("failed to get roles for user", slog.String("op", op), slog.String("user_id", u.ID.String()), slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "failed to load user relations")
			return
		}
		roles := make([]roleResp, len(userRoles))
		for i, r := range userRoles {
			roles[i] = roleResp{ID: r.ID.String(), Name: r.Name}
		}

		resp = append(resp, userWithRelations{
			ID:         u.ID.String(),
			TelegramID: u.TelegramID,
			FirstName:  u.FirstName,
			LastName:   u.LastName,
			Weight:     u.Weight,
			UserTeams:  teams,
			UserRoles:  roles,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetUserDetails возвращает детальную информацию о пользователе, включая списки ID привязанных команд и ролей.
func (h *GanttHandler) GetUserDetails(w http.ResponseWriter, r *http.Request) {
	op := "handlers.GetUserDetails"
	userIDStr := chi.URLParam(r, "id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	h.log.Info("executing get user details", slog.String("op", op), slog.String("user_id", userIDStr))

	user, err := h.repo.GetUserByID(r.Context(), userID)
	if err != nil {
		h.log.Error("failed to get user by id", slog.String("op", op), slog.String("user_id", userIDStr), slog.String("error", err.Error()))
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	teamIDs, roleIDs, err := h.repo.GetUserRelations(r.Context(), userID)
	if err != nil {
		h.log.Error("failed to get user relations", slog.String("op", op), slog.String("user_id", userIDStr), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to load user relations")
		return
	}

	teamIDsStr := make([]string, len(teamIDs))
	for i, tid := range teamIDs {
		teamIDsStr[i] = tid.String()
	}

	roleIDsStr := make([]string, len(roleIDs))
	for i, rid := range roleIDs {
		roleIDsStr[i] = rid.String()
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":          user.ID.String(),
		"telegram_id": user.TelegramID,
		"first_name":  user.FirstName,
		"last_name":   user.LastName,
		"weight":      user.Weight,
		"team_ids":    teamIDsStr,
		"role_ids":    roleIDsStr,
	})
}

// CreateSingleUser создает нового пользователя со всеми связями с командами и ролями в одной транзакции.
func (h *GanttHandler) CreateSingleUser(w http.ResponseWriter, r *http.Request) {
	op := "handlers.CreateSingleUser"
	h.log.Info("executing single user creation", slog.String("op", op))

	var req struct {
		TelegramID string   `json:"telegram_id"`
		FirstName  string   `json:"first_name"`
		LastName   string   `json:"last_name"`
		Weight     int      `json:"weight"`
		TeamIDs    []string `json:"team_ids"`
		RoleIDs    []string `json:"role_ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Error("failed to decode request body", slog.String("op", op), slog.String("error", err.Error()))
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.TelegramID == "" || req.FirstName == "" {
		writeError(w, http.StatusBadRequest, "telegram_id and first_name are required")
		return
	}

	if req.Weight <= 0 {
		req.Weight = 100 // вес по умолчанию
	}

	var teamUUIDs []uuid.UUID
	for _, tidStr := range req.TeamIDs {
		tid, err := uuid.Parse(tidStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid team_id in list")
			return
		}
		teamUUIDs = append(teamUUIDs, tid)
	}

	var roleUUIDs []uuid.UUID
	for _, ridStr := range req.RoleIDs {
		rid, err := uuid.Parse(ridStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid role_id in list")
			return
		}
		roleUUIDs = append(roleUUIDs, rid)
	}

	user := &domain.User{
		ID:         uuid.New(),
		TelegramID: req.TelegramID,
		FirstName:  req.FirstName,
		LastName:   req.LastName,
		Weight:     req.Weight,
	}

	err := h.repo.CreateUserWithRelations(r.Context(), user, teamUUIDs, roleUUIDs)
	if err != nil {
		h.log.Error("failed to create user with relations", slog.String("op", op), slog.String("error", err.Error()))
		if strings.Contains(err.Error(), "unique_telegram_id") || strings.Contains(err.Error(), "telegram_id") {
			writeError(w, http.StatusConflict, "пользователь с таким telegram_id уже зарегистрирован")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	writeJSON(w, http.StatusCreated, user)
}

// UpdateUser обновляет данные пользователя и его связи с командами и ролями.
func (h *GanttHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	op := "handlers.UpdateUser"
	userIDStr := chi.URLParam(r, "id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	h.log.Info("executing user update", slog.String("op", op), slog.String("user_id", userIDStr))

	var req struct {
		FirstName string   `json:"first_name"`
		LastName  string   `json:"last_name"`
		Weight    int      `json:"weight"`
		TeamIDs   []string `json:"team_ids"`
		RoleIDs   []string `json:"role_ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Error("failed to decode request body", slog.String("op", op), slog.String("error", err.Error()))
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.FirstName == "" {
		writeError(w, http.StatusBadRequest, "first_name is required")
		return
	}

	if req.Weight < 0 || req.Weight > 100 {
		writeError(w, http.StatusBadRequest, "weight must be between 0 and 100")
		return
	}

	var teamUUIDs []uuid.UUID
	for _, tidStr := range req.TeamIDs {
		tid, err := uuid.Parse(tidStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid team_id in list")
			return
		}
		teamUUIDs = append(teamUUIDs, tid)
	}

	var roleUUIDs []uuid.UUID
	for _, ridStr := range req.RoleIDs {
		rid, err := uuid.Parse(ridStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid role_id in list")
			return
		}
		roleUUIDs = append(roleUUIDs, rid)
	}

	err = h.repo.UpdateUserWithRelations(r.Context(), userID, req.FirstName, req.LastName, req.Weight, teamUUIDs, roleUUIDs)
	if err != nil {
		h.log.Error("failed to update user with relations", slog.String("op", op), slog.String("user_id", userIDStr), slog.String("error", err.Error()))
		if strings.Contains(err.Error(), "user not found") {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update user")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
