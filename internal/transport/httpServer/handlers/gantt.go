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
	svc  GanttService
	repo Repository
	cfg  config.BotConfig
	log  *slog.Logger
}

// NewGanttHandler creates a new GanttHandler.
func NewGanttHandler(
	log *slog.Logger,
	svc GanttService,
	repo Repository,
	cfg config.BotConfig,
) *GanttHandler {
	return &GanttHandler{
		svc:  svc,
		repo: repo,
		cfg:  cfg,
		log:  log.With(slog.String("component", "gantt-handler")),
	}
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

// GetEpics returns scored epics for a team.
func (h *GanttHandler) GetEpics(w http.ResponseWriter, r *http.Request) {
	teamIDStr := r.URL.Query().Get("team_id")
	teamID, err := uuid.Parse(teamIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid team_id")
		return
	}

	epics, err := h.repo.GetEpicsByTeamIDAndStatus(
		r.Context(), teamID, "SCORED",
	)
	if err != nil {
		h.log.Error("failed to get epics", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to get epics")
		return
	}

	type epicResp struct {
		ID         string   `json:"id"`
		Number     string   `json:"number"`
		Name       string   `json:"name"`
		FinalScore *float64 `json:"final_score"`
	}
	var resp []epicResp
	for _, e := range epics {
		resp = append(resp, epicResp{
			ID:         e.ID.String(),
			Number:     e.Number,
			Name:       e.Name,
			FinalScore: e.FinalScore,
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
	"Аналитик":           "gantt-analyst",
	"BE разработчик":     "gantt-developer",
	"FE разработчик":     "gantt-developer",
	"Mobile разработчик": "gantt-developer",
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
		css := "gantt-epic"
		if !t.IsParent {
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

