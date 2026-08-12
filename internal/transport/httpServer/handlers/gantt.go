package handlers

import (
	"EpicScoreBot/internal/config"
	"EpicScoreBot/internal/models/domain"
	"EpicScoreBot/internal/transport/httpServer/middleware"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// GanttHandler provides HTTP handlers for the Gantt API.
type GanttHandler struct {
	svc     GanttService
	repo    Repository
	scoring ScoringService
	ai      AIClient
	cfg     config.BotConfig
	log     *slog.Logger
}

// NewGanttHandler creates a new GanttHandler.
func NewGanttHandler(
	log *slog.Logger,
	svc GanttService,
	repo Repository,
	scoring ScoringService,
	ai AIClient,
	cfg config.BotConfig,
) *GanttHandler {
	return &GanttHandler{
		svc:     svc,
		repo:    repo,
		scoring: scoring,
		ai:      ai,
		cfg:     cfg,
		log:     log.With(slog.String("component", "gantt-handler")),
	}
}

// Repo возвращает Repository этого обработчика.
func (h *GanttHandler) Repo() Repository {
	return h.repo
}

// Config возвращает config.BotConfig этого обработчика.
func (h *GanttHandler) Config() config.BotConfig {
	return h.cfg
}

// ── API Handlers ──────────────────────────────────────────────────────────

// GetTeams returns teams based on user's role.
func (h *GanttHandler) GetTeams(w http.ResponseWriter, r *http.Request) {
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

	var teams []domain.Team
	var err error
	var role string

	// 1. Is SuperAdmin?
	isSuperAdmin := false
	for _, sa := range h.cfg.SuperAdmins {
		if strings.EqualFold(session.Username, sa) {
			isSuperAdmin = true
			break
		}
	}

	h.log.Info("RBAC Check In GetTeams",
		slog.String("username", session.Username),
		slog.Bool("isSuperAdmin", isSuperAdmin),
		slog.Any("superAdminsConfig", h.cfg.SuperAdmins))

	if isSuperAdmin {
		teams, err = h.repo.GetAllTeams(r.Context())
		role = "superadmin"
	} else {
		// 2. Is Admin?
		isAdmin := false
		for _, ad := range h.cfg.Admins {
			if strings.EqualFold(session.Username, ad) {
				isAdmin = true
				break
			}
		}

		if isAdmin {
			teams, err = h.repo.GetTeamsByUserTelegramID(r.Context(), session.TelegramID)
			role = "admin"
		} else {
			// 3. Regular member?
			user, errDb := h.repo.FindUserByTelegramID(r.Context(), session.TelegramID)
			if errDb != nil || user == nil {
				// 4. Access Denied
				writeError(w, http.StatusForbidden, "access denied")
				return
			}
			teams, err = h.repo.GetTeamsByUserTelegramID(r.Context(), session.TelegramID)
			role = "member"
		}
	}

	if err != nil {
		h.log.Error("failed to get teams", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to get teams")
		return
	}

	type teamResp struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	var resp []teamResp
	for _, t := range teams {
		resp = append(resp, teamResp{
			ID:   t.ID.String(),
			Name: t.Name,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"teams": resp,
		"role":  role,
	})
}

// GetEpics returns epics for a team (scored only by default, or all if all=true).
func (h *GanttHandler) GetEpics(w http.ResponseWriter, r *http.Request) {
	teamIDStr := r.URL.Query().Get("team_id")
	teamID, err := uuid.Parse(teamIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid team_id")
		return
	}

	var epics []domain.Epic
	allStr := r.URL.Query().Get("all")
	if allStr == "true" {
		var allEpics []domain.Epic
		allEpics, err = h.repo.GetAllEpics(r.Context())
		if err == nil {
			for _, e := range allEpics {
				if e.TeamID == teamID {
					epics = append(epics, e)
				}
			}
		}
	} else {
		epics, err = h.repo.GetEpicsByTeamIDAndStatus(
			r.Context(), teamID, "SCORED",
		)
	}
	
	if err != nil {
		h.log.Error("failed to get epics", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to get epics")
		return
	}

	type epicResp struct {
		ID                string   `json:"id"`
		Number            string   `json:"number"`
		Name              string   `json:"name"`
		Status            string   `json:"status"`
		Description       string   `json:"description"`
		FinalScore        *float64 `json:"final_score"`
		TeamID            string   `json:"team_id"`
		Year              int      `json:"year"`
		Quarter           int      `json:"quarter"`
		Type              string   `json:"type"`
		EvaluatingRoleIDs []string `json:"evaluating_role_ids,omitempty"`
		ParentEpicID      *string  `json:"parent_epic_id,omitempty"`
	}
	var resp []epicResp
	for _, e := range epics {
		var evalRoles []string
		for _, rID := range e.EvaluatingRoleIDs {
			evalRoles = append(evalRoles, rID.String())
		}
		var parentStr *string
		if e.ParentEpicID != nil {
			s := e.ParentEpicID.String()
			parentStr = &s
		}
		resp = append(resp, epicResp{
			ID:                e.ID.String(),
			Number:            e.Number,
			Name:              e.Name,
			Status:            string(e.Status),
			Description:       e.Description,
			FinalScore:        e.FinalScore,
			TeamID:            e.TeamID.String(),
			Year:              e.Year,
			Quarter:           e.Quarter,
			Type:              e.Type,
			EvaluatingRoleIDs: evalRoles,
			ParentEpicID:      parentStr,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"epics": resp})
}

// ganttTaskResp is the JSON representation of a Gantt task
// compatible with Frappe Gantt.
type ganttTaskResp struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Start        string  `json:"start"`
	End          string  `json:"end"`
	Progress     float64 `json:"progress"`
	Dependencies string  `json:"dependencies"`
	CustomClass  string  `json:"custom_class"`
	IsParent     bool    `json:"is_parent"`
	ParentID     string  `json:"parent_id,omitempty"`
	SortOrder    int     `json:"sort_order"`
	RoleID       string  `json:"role_id,omitempty"`
}

// roleToCSS maps role names to CSS class names.
var roleToCSS = map[string]string{
	"BE разработчик":     "gantt-be",
	"FE разработчик":     "gantt-fe",
	"Mobile разработчик": "gantt-mobile",
	"Аналитик":           "gantt-analyst",
	"Тестировщик":        "gantt-qa",
	"IT-лидер":           "gantt-leader",
}

// GetTasks returns Gantt tasks for a team.
func (h *GanttHandler) GetTasks(w http.ResponseWriter, r *http.Request) {
	teamIDStr := r.URL.Query().Get("team_id")
	teamID, err := uuid.Parse(teamIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid team_id")
		return
	}

	tasks, err := h.repo.GetGanttTasksByTeamID(r.Context(), teamID)
	if err != nil {
		h.log.Error("failed to get tasks", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to get tasks")
		return
	}

	// Build dependency strings: child tasks depend on the previous
	// sort_order group's tasks within the same epic.
	type epicGroup struct {
		parentID string
		groups   map[int][]string // sort_order -> task IDs
	}
	epicGroups := make(map[string]*epicGroup)
	for _, t := range tasks {
		if t.IsParent {
			epicGroups[t.ID.String()] = &epicGroup{
				parentID: t.ID.String(),
				groups:   make(map[int][]string),
			}
		}
	}
	for _, t := range tasks {
		if !t.IsParent && t.ParentTaskID != nil {
			pid := t.ParentTaskID.String()
			if eg, ok := epicGroups[pid]; ok {
				eg.groups[t.SortOrder] = append(
					eg.groups[t.SortOrder], t.ID.String(),
				)
			}
		}
	}

	var resp []ganttTaskResp
	for _, t := range tasks {
		var css string
		if t.IsParent {
			if t.ParentTaskID != nil {
				css = "gantt-story"
			} else {
				css = "gantt-epic"
			}
		} else {
			css = roleToCSS[t.Name]
			if css == "" {
				css = "gantt-default"
			}
		}

		name := t.Name
		if !t.IsParent {
			name = "  " + name
		}

		// Build dependencies from previous sort_order group.
		var deps string
		if !t.IsParent && t.ParentTaskID != nil {
			pid := t.ParentTaskID.String()
			if eg, ok := epicGroups[pid]; ok {
				prevOrder := t.SortOrder - 1
				if prevIDs, has := eg.groups[prevOrder]; has {
					deps = strings.Join(prevIDs, ", ")
				}
			}
		}

		item := ganttTaskResp{
			ID:           t.ID.String(),
			Name:         name,
			Start:        t.StartDate.Format("2006-01-02"),
			End:          t.EndDate.Format("2006-01-02"),
			Progress:     t.Progress,
			Dependencies: deps,
			CustomClass:  css,
			IsParent:     t.IsParent,
			SortOrder:    t.SortOrder,
		}
		if t.ParentTaskID != nil {
			item.ParentID = t.ParentTaskID.String()
		}
		if t.RoleID != nil {
			item.RoleID = t.RoleID.String()
		}
		resp = append(resp, item)
	}

	writeJSON(w, http.StatusOK, map[string]any{"tasks": resp})
}

// GenerateTasks generates Gantt tasks for an epic.
func (h *GanttHandler) GenerateTasks(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EpicID    string `json:"epic_id"`
		StartDate string `json:"start_date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	epicID, err := uuid.Parse(req.EpicID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid epic_id")
		return
	}

	startDate, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		writeError(w, http.StatusBadRequest,
			"invalid start_date, expected YYYY-MM-DD")
		return
	}

	tasks, err := h.svc.GenerateTasksForEpic(
		r.Context(), epicID, startDate,
	)
	if err != nil {
		h.log.Error("failed to generate tasks",
			slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError,
			fmt.Sprintf("failed to generate: %s", err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message": "tasks generated",
		"count":   len(tasks),
	})
}

// UpdateTask updates a task's dates and/or progress.
func (h *GanttHandler) UpdateTask(w http.ResponseWriter, r *http.Request) {
	taskIDStr := chi.URLParam(r, "id")
	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid task id")
		return
	}

	var req struct {
		Start    *string  `json:"start"`
		End      *string  `json:"end"`
		Progress *float64 `json:"progress"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Start != nil && req.End != nil {
		startDate, err := time.Parse("2006-01-02", *req.Start)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid start date")
			return
		}
		endDate, err := time.Parse("2006-01-02", *req.End)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid end date")
			return
		}
		if err := h.svc.UpdateTaskDates(
			r.Context(), taskID, startDate, endDate,
		); err != nil {
			h.log.Error("failed to update dates",
				slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError,
				"failed to update dates")
			return
		}
	}

	if req.Progress != nil {
		if err := h.repo.UpdateGanttTaskProgress(
			r.Context(), taskID, *req.Progress,
		); err != nil {
			h.log.Error("failed to update progress",
				slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError,
				"failed to update progress")
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "task updated",
	})
}

// ReorderTask changes a task's sort order and recalculates dates.
func (h *GanttHandler) ReorderTask(w http.ResponseWriter, r *http.Request) {
	taskIDStr := chi.URLParam(r, "id")
	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid task id")
		return
	}

	var req struct {
		SortOrder int `json:"sort_order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	tasks, err := h.svc.ReorderTask(r.Context(), taskID, req.SortOrder)
	if err != nil {
		h.log.Error("failed to reorder task",
			slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError,
			"failed to reorder task")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message": "task reordered",
		"count":   len(tasks),
	})
}

// DeleteTask deletes Gantt tasks for an epic.
func (h *GanttHandler) DeleteTask(w http.ResponseWriter, r *http.Request) {
	taskIDStr := chi.URLParam(r, "id")
	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid task id")
		return
	}

	// Get task to find epic_id, then delete all tasks for that epic.
	task, err := h.repo.GetGanttTaskByID(r.Context(), taskID)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	if err := h.repo.DeleteGanttTasksByEpicID(
		r.Context(), task.EpicID,
	); err != nil {
		h.log.Error("failed to delete tasks",
			slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError,
			"failed to delete tasks")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "tasks deleted",
	})
}

// TelegramAuth handles the Telegram Login Widget callback.
func (h *GanttHandler) TelegramAuth(w http.ResponseWriter, r *http.Request) {
	// Verify query params (initial login redirect).
	if middleware.VerifyTelegramAuth(r, h.cfg.TgbotApiToken) {
		query := r.URL.Query()
		session := middleware.UserSession{
			TelegramID: query.Get("id"),
			Username:   query.Get("username"),
			FirstName:  query.Get("first_name"),
		}
		token, err := middleware.CreateSessionToken(session, h.cfg.TgbotApiToken)
		if err == nil {
			http.SetCookie(w, &http.Cookie{
				Name:     "tg_sys_auth",
				Value:    token,
				Path:     "/",
				MaxAge:   86400 * 7, // 7 days
				HttpOnly: false,     // Frontend JS needs to read this to know auth state
				SameSite: http.SameSiteLaxMode,
			})
			// Clean up old tg_auth cookie if it still exists
			http.SetCookie(w, &http.Cookie{
				Name:   "tg_auth",
				Value:  "",
				Path:   "/",
				MaxAge: -1,
			})
			http.Redirect(w, r, "/gantt/", http.StatusFound)
			return
		}
	}

	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte("Unauthorized telegram login"))
}

// TelegramWebAppAuth handles authorization from Telegram Mini App.
func (h *GanttHandler) TelegramWebAppAuth(w http.ResponseWriter, r *http.Request) {
	var req struct {
		InitData string `json:"initData"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if session, ok := middleware.VerifyTelegramWebAppData(req.InitData, h.cfg.TgbotApiToken); ok {
		token, err := middleware.CreateSessionToken(*session, h.cfg.TgbotApiToken)
		if err == nil {
			http.SetCookie(w, &http.Cookie{
				Name:     "tg_sys_auth",
				Value:    token,
				Path:     "/",
				MaxAge:   86400 * 7, // 7 days
				HttpOnly: false,
				SameSite: http.SameSiteLaxMode,
			})
			writeJSON(w, http.StatusOK, map[string]string{"message": "authenticated"})
			return
		}
	}

	writeError(w, http.StatusUnauthorized, "invalid init data")
}

// GetProfile returns the profile of the currently authenticated user including their role.
func (h *GanttHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
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

	var role string

	// 1. Is SuperAdmin?
	isSuperAdmin := false
	for _, sa := range h.cfg.SuperAdmins {
		if strings.EqualFold(session.Username, sa) {
			isSuperAdmin = true
			break
		}
	}

	if isSuperAdmin {
		role = "superadmin"
	} else {
		// 2. Is Admin?
		isAdmin := false
		for _, ad := range h.cfg.Admins {
			if strings.EqualFold(session.Username, ad) {
				isAdmin = true
				break
			}
		}

		if isAdmin {
			role = "admin"
		} else {
			// 3. Regular member?
			user, errDb := h.repo.FindUserByTelegramID(r.Context(), session.TelegramID)
			if errDb != nil || user == nil {
				writeError(w, http.StatusForbidden, "access denied")
				return
			}
			role = "member"
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"telegram_id": session.TelegramID,
		"username":    session.Username,
		"first_name":  session.FirstName,
		"role":        role,
	})
}

// GetRoles returns all system roles.
func (h *GanttHandler) GetRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := h.repo.GetAllRoles(r.Context())
	if err != nil {
		h.log.Error("failed to get roles", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to get roles")
		return
	}

	type roleResp struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	var resp []roleResp
	for _, role := range roles {
		resp = append(resp, roleResp{
			ID:   role.ID.String(),
			Name: role.Name,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"roles": resp})
}

// CreateTeam inserts a new team.
func (h *GanttHandler) CreateTeam(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	team, err := h.repo.CreateTeam(r.Context(), req.Name, req.Description)
	if err != nil {
		h.log.Error("failed to create team", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to create team: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, team)
}

// GetEpicScores returns voting progress and raw scores.
func (h *GanttHandler) GetEpicScores(w http.ResponseWriter, r *http.Request) {
	epicIDStr := chi.URLParam(r, "epic_id")
	epicID, err := uuid.Parse(epicIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid epic_id")
		return
	}

	scores, err := h.repo.GetEpicScoresByEpicID(r.Context(), epicID)
	if err != nil {
		h.log.Error("failed to get scores", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to get scores")
		return
	}

	type MemberInfo struct {
		ID         string `json:"id"`
		FirstName  string `json:"first_name"`
		LastName   string `json:"last_name"`
		TelegramID string `json:"telegram_id"`
		Weight     int    `json:"weight"`
		RoleID     string `json:"role_id"`
		RoleName   string `json:"role_name"`
	}

	epic, err := h.repo.GetEpicByID(r.Context(), epicID)
	var expected int
	var membersResp []MemberInfo
	if err == nil && epic != nil {
		members, errMem := h.repo.GetUsersByTeamID(r.Context(), epic.TeamID)
		if errMem == nil {
			expected = len(members)
			for _, m := range members {
				var roleID string
				var roleName string
				role, errR := h.repo.GetRoleByUserID(r.Context(), m.ID)
				if errR == nil && role != nil {
					roleID = role.ID.String()
					roleName = role.Name
				}
				membersResp = append(membersResp, MemberInfo{
					ID:         m.ID.String(),
					FirstName:  m.FirstName,
					LastName:   m.LastName,
					TelegramID: m.TelegramID,
					Weight:     m.Weight,
					RoleID:     roleID,
					RoleName:   roleName,
				})
			}
		}
	}

	type UserInfo struct {
		ID         string `json:"id"`
		FirstName  string `json:"first_name"`
		LastName   string `json:"last_name"`
		TelegramID string `json:"telegram_id"`
		Weight     int    `json:"weight"`
	}

	type RoleInfo struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}

	type EnrichedEpicScore struct {
		ID        uuid.UUID `json:"id"`
		EpicID    uuid.UUID `json:"epic_id"`
		UserID    uuid.UUID `json:"user_id"`
		RoleID    uuid.UUID `json:"role_id"`
		Score     int       `json:"score"`
		CreatedAt time.Time `json:"created_at"`
		User      UserInfo  `json:"user"`
		Role      RoleInfo  `json:"role"`
	}

	var enrichedScores []EnrichedEpicScore
	for _, s := range scores {
		var uInfo UserInfo
		user, errU := h.repo.GetUserByID(r.Context(), s.UserID)
		if errU == nil && user != nil {
			uInfo = UserInfo{
				ID:         user.ID.String(),
				FirstName:  user.FirstName,
				LastName:   user.LastName,
				TelegramID: user.TelegramID,
				Weight:     user.Weight,
			}
		} else {
			uInfo.ID = s.UserID.String()
		}

		var rInfo RoleInfo
		role, errR := h.repo.GetRoleByID(r.Context(), s.RoleID)
		if errR == nil && role != nil {
			rInfo = RoleInfo{
				ID:   role.ID.String(),
				Name: role.Name,
			}
		} else {
			rInfo.ID = s.RoleID.String()
		}

		enrichedScores = append(enrichedScores, EnrichedEpicScore{
			ID:        s.ID,
			EpicID:    s.EpicID,
			UserID:    s.UserID,
			RoleID:    s.RoleID,
			Score:     s.Score,
			CreatedAt: s.CreatedAt,
			User:      uInfo,
			Role:      rInfo,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"scores":          enrichedScores,
		"scores_received": len(scores),
		"scores_expected": expected,
		"members":         membersResp,
	})
}

// GetEpicRoleScores returns aggregated weighted scores per role.
func (h *GanttHandler) GetEpicRoleScores(w http.ResponseWriter, r *http.Request) {
	epicIDStr := chi.URLParam(r, "epic_id")
	epicID, err := uuid.Parse(epicIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid epic_id")
		return
	}

	roleScores, err := h.repo.GetEpicRoleScoresByEpicID(r.Context(), epicID)
	if err != nil {
		h.log.Error("failed to get role scores", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to get role scores")
		return
	}

	type roleScoreResp struct {
		RoleID      string  `json:"role_id"`
		RoleName    string  `json:"role_name"`
		WeightedAvg float64 `json:"weighted_avg"`
	}

	var resp []roleScoreResp
	for _, rs := range roleScores {
		roleName := rs.RoleID.String()
		role, errR := h.repo.GetRoleByID(r.Context(), rs.RoleID)
		if errR == nil && role != nil {
			roleName = role.Name
		}
		resp = append(resp, roleScoreResp{
			RoleID:      rs.RoleID.String(),
			RoleName:    roleName,
			WeightedAvg: rs.WeightedAvg,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetEpicRisks returns risks of an epic.
func (h *GanttHandler) GetEpicRisks(w http.ResponseWriter, r *http.Request) {
	epicIDStr := chi.URLParam(r, "epic_id")
	epicID, err := uuid.Parse(epicIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid epic_id")
		return
	}

	risks, err := h.repo.GetRisksByEpicID(r.Context(), epicID)
	if err != nil {
		h.log.Error("failed to get risks", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to get risks")
		return
	}

	type UserInfo struct {
		ID         string `json:"id"`
		FirstName  string `json:"first_name"`
		LastName   string `json:"last_name"`
		TelegramID string `json:"telegram_id"`
		Weight     int    `json:"weight"`
	}

	type EnrichedRiskScore struct {
		ID          uuid.UUID `json:"id"`
		RiskID      uuid.UUID `json:"risk_id"`
		UserID      uuid.UUID `json:"user_id"`
		Probability int       `json:"probability"`
		Impact      int       `json:"impact"`
		CreatedAt   time.Time `json:"created_at"`
		User        UserInfo  `json:"user"`
	}

	type riskResp struct {
		ID            string              `json:"id"`
		Description   string              `json:"description"`
		WeightedScore *float64            `json:"weighted_score"`
		Scores        []EnrichedRiskScore `json:"scores"`
	}

	var resp []riskResp
	for _, risk := range risks {
		var enrichedRiskScores []EnrichedRiskScore
		riskScores, errRS := h.repo.GetRiskScoresByRiskID(r.Context(), risk.ID)
		if errRS == nil {
			for _, rs := range riskScores {
				var uInfo UserInfo
				user, errU := h.repo.GetUserByID(r.Context(), rs.UserID)
				if errU == nil && user != nil {
					uInfo = UserInfo{
						ID:         user.ID.String(),
						FirstName:  user.FirstName,
						LastName:   user.LastName,
						TelegramID: user.TelegramID,
						Weight:     user.Weight,
					}
				} else {
					uInfo.ID = rs.UserID.String()
				}
				enrichedRiskScores = append(enrichedRiskScores, EnrichedRiskScore{
					ID:          rs.ID,
					RiskID:      rs.RiskID,
					UserID:      rs.UserID,
					Probability: rs.Probability,
					Impact:      rs.Impact,
					CreatedAt:   rs.CreatedAt,
					User:        uInfo,
				})
			}
		}

		resp = append(resp, riskResp{
			ID:            risk.ID.String(),
			Description:   risk.Description,
			WeightedScore: risk.WeightedScore,
			Scores:        enrichedRiskScores,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"risks": resp})
}



