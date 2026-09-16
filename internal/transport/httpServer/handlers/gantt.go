package handlers

import (
	"EpicScoreBot/internal/config"
	"EpicScoreBot/internal/gantt"
	"EpicScoreBot/internal/models/domain"
	"EpicScoreBot/internal/transport/httpServer/middleware"
	"encoding/json"
	"errors"
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
	svc        GanttService
	repo       Repository
	scoring    ScoringService
	ai         AIClient
	notifier   TelegramNotifier
	docSender  DocumentSender
	reportData ReportDataProvider
	reportGen  PDFReportGenerator
	cfg        config.BotConfig
	log        *slog.Logger
}

// NewGanttHandler creates a new GanttHandler.
func NewGanttHandler(
	log *slog.Logger,
	svc GanttService,
	repo Repository,
	scoring ScoringService,
	ai AIClient,
	cfg config.BotConfig,
	notifier TelegramNotifier,
) *GanttHandler {
	return &GanttHandler{
		svc:      svc,
		repo:     repo,
		scoring:  scoring,
		ai:       ai,
		notifier: notifier,
		cfg:      cfg,
		log:      log.With(slog.String("component", "gantt-handler")),
	}
}

// WithReportServices устанавливает зависимости, нужные для файловой выгрузки
// отчёта команды (см. ExportTeamReport, format=pdf) — источник данных PDF
// (ReportDataProvider) и генератор PDF (PDFReportGenerator). Вынесено
// отдельным методом, а не обязательными параметрами NewGanttHandler, чтобы
// не менять сигнатуру конструктора и не задевать существующие тесты
// (см. gantt_test.go и др., которые не используют экспорт отчётов файлом).
func (h *GanttHandler) WithReportServices(reportData ReportDataProvider, reportGen PDFReportGenerator) *GanttHandler {
	h.reportData = reportData
	h.reportGen = reportGen
	return h
}

// WithDocumentSender устанавливает отправитель документов в личный чат
// Telegram, используемый ExportGanttImage (см. handlers/gantt_export.go,
// design.md Решение 7 заявки export-gantt-chart-image). Тот же приём, что и
// WithReportServices выше: отдельный метод, а не обязательный параметр
// NewGanttHandler, чтобы не менять сигнатуру конструктора и не задевать
// существующие тесты, которые его не используют (см. комментарий у
// WithReportServices).
func (h *GanttHandler) WithDocumentSender(sender DocumentSender) *GanttHandler {
	h.docSender = sender
	return h
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
		// 2. Is team-admin (team_admins в БД, team-scoped) хотя бы одной команды?
		isAdmin, errAdmin := h.repo.IsTeamAdminOfAny(r.Context(), session.TelegramID)
		if errAdmin != nil {
			isAdmin = false
		}

		if isAdmin {
			// Team-admin видит команды, где он назначен team-admin (не
			// обязательно совпадает с командами членства через user_teams).
			var teamIDs []uuid.UUID
			teamIDs, err = h.repo.AdminTeamIDs(r.Context(), session.TelegramID)
			if err == nil {
				for _, tid := range teamIDs {
					if t, errT := h.repo.GetTeamByID(r.Context(), tid); errT == nil && t != nil {
						teams = append(teams, *t)
					}
				}
			}
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
		// SortOrder — позиция эпика в очереди конвейерного планировщика
		// (см. epics.sort_order); nil, если ещё не назначена.
		SortOrder *int `json:"sort_order,omitempty"`
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
			SortOrder:         e.SortOrder,
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
	// EpicID — реальный Epic.ID, к которому относится gantt-задача (не путать
	// с ID самой строки gantt_tasks). Нужен фронтенду, чтобы drag-reorder на
	// диаграмме подставлял в PUT /epics/{id}/reorder и PUT /stories/{id}/reorder
	// корректный идентификатор — см. design.md change enable-gantt-drag-reorder,
	// Decision 5. Присутствует всегда (без omitempty), т.к. любая gantt-задача
	// принадлежит какому-то эпику.
	EpicID string `json:"epic_id"`
	// ActualEndDate — фактическая дата завершения задачи (проставляется
	// автоматически при 100% прогресса), отсутствует пока задача не завершена.
	ActualEndDate *string `json:"actual_end_date,omitempty"`
	// ActualEffortDays — фактическая трудоёмкость в рабочих днях.
	ActualEffortDays *int `json:"actual_effort_days,omitempty"`
	// StartOffsetDays — смещение (lead/lag, в днях) планового старта листовой
	// задачи относительно окончания предыдущей ролевой группы внутри стори.
	// Без omitempty — 0 такое же значимое значение, как и любое другое.
	// Источник — task_assignments (см. GetTeamTasksWithAssignments), а не
	// gantt_tasks.start_offset_days: эта колонка перестала писаться
	// планировщиком начиная с add-gantt-task-assignees (backend §2.3), но
	// продолжала бы отдаваться нулём в этом ответе, если бы не §3.5.
	StartOffsetDays int `json:"start_offset_days"`
	// AssigneeID — исполнитель ролевой задачи (см. domain.GanttTask.AssigneeID).
	// Отсутствует у родительских (стори/эпик) задач и у ролевых задач без
	// исполнителя (пул роли пуст в команде).
	AssigneeID *string `json:"assignee_id,omitempty"`
	// AssigneeName — отображаемое имя исполнителя ("Имя Фамилия"). Те же
	// случаи отсутствия, что и у AssigneeID.
	AssigneeName *string `json:"assignee_name,omitempty"`
	// AssigneeIsManual — true, если текущий исполнитель определён
	// ДЕЙСТВУЮЩИМ ручным закреплением (task_assignments.user_id), а не
	// автоматическим распределением планировщика.
	AssigneeIsManual bool `json:"assignee_is_manual"`
	// AssigneePinInvalid — ручное закрепление есть (task_assignments.user_id
	// задан), но не действует: закреплённый человек больше не кандидат роли
	// этой задачи (вышел из команды/лишился роли), поэтому фактический
	// исполнитель — результат автоматического распределения, отличный от
	// закреплённого (design.md Решение 7). Позволяет фронтенду отличить
	// «закреплён и работает» от «закрепление есть, но не действует».
	AssigneePinInvalid bool `json:"assignee_pin_invalid"`
	// NotBeforeDate — календарная дата ограничения "Начать не ранее"
	// (openspec/changes/add-task-start-constraints, backend §3.1), формат
	// YYYY-MM-DD. Отсутствует, если ограничение по дате не задано.
	NotBeforeDate *string `json:"not_before_date,omitempty"`
	// WaitForStoryID/WaitForRoleID — ссылка на другую задачу команды,
	// раньше завершения которой эта не может начаться, парой "стори (или
	// эпик без сторей) + роль" — тот же ключ, которым уже адресуется
	// пользовательский ввод (design.md Решение 1). Отсутствуют, если
	// ссылка не задана.
	WaitForStoryID *string `json:"wait_for_story_id,omitempty"`
	WaitForRoleID  *string `json:"wait_for_role_id,omitempty"`
	// StartConstraintRefInvalid — true, если WaitForStoryID/WaitForRoleID
	// заданы, но сейчас не указывают ни на одну существующую листовую
	// задачу (стори удалена, роль перестала оцениваться, эпик вне
	// периода) — design.md Решение 4. Ограничение по дате (если задано)
	// при этом продолжает действовать независимо от этого признака.
	StartConstraintRefInvalid bool `json:"start_constraint_ref_invalid"`
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

	tasks, assignments, validRefs, err := h.svc.GetTeamTasksWithAssignments(r.Context(), teamID)
	if err != nil {
		h.log.Error("failed to get tasks", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to get tasks")
		return
	}

	// Резолвим отображаемые имена исполнителей ПАЧКОЙ одним запросом
	// (GetTasks дёргается на каждое изменение прогресса — см. design.md
	// Решение 9), а не по пользователю на задачу. usersByID дополняется
	// точечно только для исполнителей замороженных задач, уже выбывших из
	// команды (design.md Решение 6) и потому отсутствующих в выборке по
	// team_id — редкий случай, а не путь по каждой задаче.
	usersByID := make(map[uuid.UUID]domain.User)
	if members, errU := h.repo.GetUsersByTeamID(r.Context(), teamID); errU == nil {
		for _, u := range members {
			usersByID[u.ID] = u
		}
	}
	assigneeName := func(id uuid.UUID) string {
		if u, ok := usersByID[id]; ok {
			return strings.TrimSpace(u.FirstName + " " + u.LastName)
		}
		if u, errU := h.repo.GetUserByID(r.Context(), id); errU == nil && u != nil {
			usersByID[id] = *u
			return strings.TrimSpace(u.FirstName + " " + u.LastName)
		}
		return ""
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
			EpicID:       t.EpicID.String(),
		}
		if t.ParentTaskID != nil {
			item.ParentID = t.ParentTaskID.String()
		}
		if t.RoleID != nil {
			item.RoleID = t.RoleID.String()
		}
		if t.ActualEndDate != nil {
			s := t.ActualEndDate.Format("2006-01-02")
			item.ActualEndDate = &s
		}
		if t.ActualEffortDays != nil {
			item.ActualEffortDays = t.ActualEffortDays
		}

		// task_assignments (закрепление/смещение) — единый источник и для
		// StartOffsetDays (backend §3.5), и для полей assignee_* ниже
		// (backend §3.1), см. gantt.Service.GetTeamTasksWithAssignments.
		assignment, hasAssignment := assignments[t.ID]
		if hasAssignment {
			item.StartOffsetDays = assignment.StartOffsetDays
		}
		if t.AssigneeID != nil {
			idStr := t.AssigneeID.String()
			item.AssigneeID = &idStr
			if displayName := assigneeName(*t.AssigneeID); displayName != "" {
				item.AssigneeName = &displayName
			}
		}
		if hasAssignment && assignment.UserID != nil {
			item.AssigneeIsManual = t.AssigneeID != nil && *t.AssigneeID == *assignment.UserID
			item.AssigneePinInvalid = !item.AssigneeIsManual
		}

		// Ограничение "Начать не ранее" (backend §3.1,
		// openspec/changes/add-task-start-constraints) — тот же
		// hasAssignment/assignment, что уже читались выше для
		// закрепления/смещения.
		if hasAssignment {
			if assignment.NotBeforeDate != nil {
				s := assignment.NotBeforeDate.Format("2006-01-02")
				item.NotBeforeDate = &s
			}
			if assignment.WaitForStoryID != nil && assignment.WaitForRoleID != nil {
				storyIDStr := assignment.WaitForStoryID.String()
				roleIDStr := assignment.WaitForRoleID.String()
				item.WaitForStoryID = &storyIDStr
				item.WaitForRoleID = &roleIDStr
				ref := gantt.TaskRef{StoryID: *assignment.WaitForStoryID, RoleID: *assignment.WaitForRoleID}
				item.StartConstraintRefInvalid = !validRefs[ref]
			}
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

// minEpicYear/maxEpicYear ограничивают допустимый диапазон года эпика —
// совпадает с диапазоном, принятым на фронтенде для полей года эпика
// (см. web/gantt/index.html, min/max у #epic-year).
const (
	minEpicYear = 2000
	maxEpicYear = 2100
)

// GenerateQuarterTasks (пере)генерирует задачи Ганта для всех заскоренных
// топ-эпиков команды за указанные год и квартал одним запросом — вместо
// повторных вызовов GenerateTasks по каждому эпику, каждый из которых сам
// по себе запускает дорогой полный пересчёт расписания команды
// (см. openspec/changes/add-gantt-quarter-regenerate).
func (h *GanttHandler) GenerateQuarterTasks(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TeamID    string `json:"team_id"`
		Year      int    `json:"year"`
		Quarter   int    `json:"quarter"`
		StartDate string `json:"start_date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_REQUEST_BODY", "invalid request body")
		return
	}

	teamID, err := uuid.Parse(req.TeamID)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_TEAM_ID", "invalid team_id")
		return
	}

	if req.Quarter < 1 || req.Quarter > 4 {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_QUARTER", "quarter must be between 1 and 4")
		return
	}

	if req.Year < minEpicYear || req.Year > maxEpicYear {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_YEAR",
			fmt.Sprintf("year must be between %d and %d", minEpicYear, maxEpicYear))
		return
	}

	startDate, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_START_DATE",
			"invalid start_date, expected YYYY-MM-DD")
		return
	}

	result, err := h.svc.GenerateTasksForQuarter(r.Context(), teamID, req.Year, req.Quarter, startDate)
	if err != nil {
		h.log.Error("failed to generate quarter tasks",
			slog.String("teamID", teamID.String()),
			slog.Int("year", req.Year),
			slog.Int("quarter", req.Quarter),
			slog.String("error", err.Error()))
		writeErrorCode(w, http.StatusInternalServerError, "GENERATE_QUARTER_FAILED",
			fmt.Sprintf("failed to generate: %s", err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message":           "quarter tasks generated",
		"epics_total":       result.EpicsTotal,
		"epics_regenerated": result.EpicsRegenerated,
		"epics_failed":      result.EpicsFailed,
		"tasks_count":       result.TasksCount,
	})
}

// UpdateTask updates a task's progress. Dates are no longer settable
// manually — the pipeline scheduler (epic/story/role order + progress)
// is the only way to move a task on the Gantt chart.
func (h *GanttHandler) UpdateTask(w http.ResponseWriter, r *http.Request) {
	taskIDStr := chi.URLParam(r, "id")
	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid task id")
		return
	}

	var req struct {
		Start           *string  `json:"start"`
		End             *string  `json:"end"`
		Progress        *float64 `json:"progress"`
		StartOffsetDays *int     `json:"start_offset_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Start != nil || req.End != nil {
		writeErrorCode(w, http.StatusBadRequest, "SCHEDULE_MANAGED_AUTOMATICALLY",
			"даты задач рассчитываются автоматически конвейерным планировщиком; "+
				"управляйте расписанием через порядок эпиков/сторей/ролей и прогресс")
		return
	}

	if req.Progress != nil {
		if _, err := h.svc.SetTaskProgress(
			r.Context(), taskID, *req.Progress,
		); err != nil {
			h.log.Error("failed to update progress",
				slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError,
				"failed to update progress")
			return
		}
	}

	if req.StartOffsetDays != nil {
		if _, err := h.svc.SetTaskStartOffset(
			r.Context(), taskID, *req.StartOffsetDays,
		); err != nil {
			h.log.Error("failed to update start offset",
				slog.String("error", err.Error()))
			writeErrorCode(w, http.StatusBadRequest, "OFFSET_NOT_ALLOWED_ON_PARENT",
				"смещение старта можно задать только для листовой (ролевой) задачи")
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "task updated",
	})
}

// sessionRole extracts the UserSession from the request context and
// classifies it into the project's three-tier role (superadmin/admin/
// member) — the same classification GetProfile/GetTeams already do inline.
// Duplicated here rather than refactored into a single shared helper used
// everywhere (out of scope for this change — see openspec/changes/
// add-gantt-task-assignees) so that SetTaskAssignee can gate on role
// without depending on middleware.RoleAuth, which unit tests calling the
// handler function directly (as the rest of this package's handler tests
// do) don't go through. Writes a 401 response and returns ok=false if
// there's no valid session.
func (h *GanttHandler) sessionRole(w http.ResponseWriter, r *http.Request) (*middleware.UserSession, string, bool) {
	sessionData := r.Context().Value(middleware.UserSessionKey)
	if sessionData == nil {
		writeErrorCode(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return nil, "", false
	}
	session, ok := sessionData.(*middleware.UserSession)
	if !ok || session.TelegramID == "" {
		writeErrorCode(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid session")
		return nil, "", false
	}

	if isSuperAdminSession(session, &h.cfg) {
		return session, "superadmin", true
	}
	if isAdmin, _ := h.repo.IsTeamAdminOfAny(r.Context(), session.TelegramID); isAdmin {
		return session, "admin", true
	}
	// Regular member — same DB-existence check GetProfile/GetTeams already
	// perform: an unrecognized Telegram user is denied outright, not
	// silently treated as a member.
	user, err := h.repo.FindUserByTelegramID(r.Context(), session.TelegramID)
	if err != nil || user == nil {
		writeErrorCode(w, http.StatusForbidden, "FORBIDDEN", "access denied")
		return nil, "", false
	}
	return session, "member", true
}

// SetTaskAssignee pins (user_id set) or unpins (user_id null — "Автоматически")
// the executor of a leaf (role) Gantt task. Only admin/superadmin sessions
// may call this — a plain "member" is rejected (design.md, сценарий
// "Пользователь без права редактирования не может закрепить исполнителя").
// A non-null user_id must be a current candidate of the task's role (in the
// team AND holding the role) — otherwise the request is rejected outright
// rather than silently accepted as an inactive pin (design.md Решение 7
// covers a candidate BECOMING invalid after being pinned, not being pinned
// while already invalid). Recalculates the team's schedule and returns the
// updated task count, same response shape as the other task mutation
// endpoints (UpdateTask/ReorderTask).
func (h *GanttHandler) SetTaskAssignee(w http.ResponseWriter, r *http.Request) {
	taskIDStr := chi.URLParam(r, "id")
	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_TASK_ID", "invalid task id")
		return
	}

	session, role, ok := h.sessionRole(w, r)
	if !ok {
		return
	}
	if role == "member" {
		writeErrorCode(w, http.StatusForbidden, "FORBIDDEN",
			"только администратор команды может назначать исполнителя задачи")
		return
	}

	var req struct {
		UserID *string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_REQUEST_BODY", "invalid request body")
		return
	}

	task, err := h.repo.GetGanttTaskByID(r.Context(), taskID)
	if err != nil {
		writeErrorCode(w, http.StatusNotFound, "TASK_NOT_FOUND", "task not found")
		return
	}
	if task.IsParent || task.RoleID == nil {
		writeErrorCode(w, http.StatusBadRequest, "ASSIGNEE_NOT_ALLOWED_ON_PARENT",
			"исполнителя можно назначить только листовой (ролевой) задаче")
		return
	}

	epic, err := h.repo.GetEpicByID(r.Context(), task.EpicID)
	if err != nil {
		h.log.Error("failed to resolve task's epic", slog.String("error", err.Error()))
		writeErrorCode(w, http.StatusInternalServerError, "EPIC_LOOKUP_FAILED",
			"failed to resolve task's team")
		return
	}

	// Точечная team-scoped проверка: RoleAuth("admin") на уровне группы
	// маршрутов — грубый гейт "admin хотя бы одной команды" (см.
	// middleware.RoleAuth), поэтому admin команды A без этой проверки мог
	// бы назначать исполнителя в задачах команды B. Тот же приём, что уже
	// применяют admin.go (UpdateEpic) и admin_scores.go
	// (AdminSubmitEpicScore) — superadmin проверку проходит без
	// ограничения.
	if role != "superadmin" {
		isAdminOf, err := h.repo.IsTeamAdminOf(r.Context(), session.TelegramID, epic.TeamID)
		if err != nil || !isAdminOf {
			writeErrorCode(w, http.StatusForbidden, "FORBIDDEN",
				"вы не администратор команды, которой принадлежит эта задача")
			return
		}
	}

	var userID *uuid.UUID
	if req.UserID != nil {
		parsed, err := uuid.Parse(*req.UserID)
		if err != nil {
			writeErrorCode(w, http.StatusBadRequest, "INVALID_USER_ID", "invalid user_id")
			return
		}

		candidates, err := h.repo.GetUsersByTeamIDAndRoleID(r.Context(), epic.TeamID, *task.RoleID)
		if err != nil {
			h.log.Error("failed to resolve role candidates", slog.String("error", err.Error()))
			writeErrorCode(w, http.StatusInternalServerError, "CANDIDATES_LOOKUP_FAILED",
				"failed to resolve role candidates")
			return
		}
		isCandidate := false
		for _, c := range candidates {
			if c.ID == parsed {
				isCandidate = true
				break
			}
		}
		if !isCandidate {
			writeErrorCode(w, http.StatusBadRequest, "USER_NOT_ROLE_CANDIDATE",
				"пользователь не входит в число кандидатов роли этой задачи")
			return
		}
		userID = &parsed
	}

	tasks, err := h.svc.SetTaskAssignee(r.Context(), taskID, userID)
	if err != nil {
		h.log.Error("failed to set task assignee", slog.String("error", err.Error()))
		writeErrorCode(w, http.StatusInternalServerError, "RESCHEDULE_FAILED",
			"failed to set task assignee")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message": "assignee updated",
		"count":   len(tasks),
	})
}

// SetTaskStartConstraint задаёт (либо снимает, через null) ограничение
// «Начать не ранее» листовой (ролевой) задачи: дату и/или ссылку на другую
// задачу той же команды парой "стори + роль" (backend §3.2,
// openspec/changes/add-task-start-constraints). Оба поля пишутся вместе,
// как единый блок формы (design.md Решение 6) — null в любом из трёх полей
// тела запроса снимает соответствующую часть ограничения, отсутствующее
// поле трактуется так же (см. domain.TaskAssignment.NotBeforeDate/
// WaitForStoryID/WaitForRoleID).
//
// Доступ — та же двухуровневая проверка, что и SetTaskAssignee: грубый
// гейт RoleAuth("admin") на группе маршрутов (routers.go) плюс точечная
// team-scoped IsTeamAdminOf здесь. Отклоняет все виды цикла (прямой, через
// цепочку, ссылку на саму задачу) и ссылку на задачу другой команды —
// проверки выполняет gantt.Service.SetTaskStartConstraint, хендлер только
// сопоставляет сентинел-ошибки с кодами ответа.
func (h *GanttHandler) SetTaskStartConstraint(w http.ResponseWriter, r *http.Request) {
	taskIDStr := chi.URLParam(r, "id")
	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_TASK_ID", "invalid task id")
		return
	}

	session, role, ok := h.sessionRole(w, r)
	if !ok {
		return
	}
	if role == "member" {
		writeErrorCode(w, http.StatusForbidden, "FORBIDDEN",
			"только администратор команды может изменять ограничение «Начать не ранее»")
		return
	}

	var req struct {
		NotBeforeDate  *string `json:"not_before_date"`
		WaitForStoryID *string `json:"wait_for_story_id"`
		WaitForRoleID  *string `json:"wait_for_role_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_REQUEST_BODY", "invalid request body")
		return
	}

	var notBeforeDate *time.Time
	if req.NotBeforeDate != nil {
		parsed, err := time.Parse("2006-01-02", *req.NotBeforeDate)
		if err != nil {
			writeErrorCode(w, http.StatusBadRequest, "INVALID_NOT_BEFORE_DATE",
				"invalid not_before_date, expected YYYY-MM-DD")
			return
		}
		notBeforeDate = &parsed
	}

	var waitForStoryID *uuid.UUID
	if req.WaitForStoryID != nil {
		parsed, err := uuid.Parse(*req.WaitForStoryID)
		if err != nil {
			writeErrorCode(w, http.StatusBadRequest, "INVALID_WAIT_FOR_STORY_ID", "invalid wait_for_story_id")
			return
		}
		waitForStoryID = &parsed
	}
	var waitForRoleID *uuid.UUID
	if req.WaitForRoleID != nil {
		parsed, err := uuid.Parse(*req.WaitForRoleID)
		if err != nil {
			writeErrorCode(w, http.StatusBadRequest, "INVALID_WAIT_FOR_ROLE_ID", "invalid wait_for_role_id")
			return
		}
		waitForRoleID = &parsed
	}

	task, err := h.repo.GetGanttTaskByID(r.Context(), taskID)
	if err != nil {
		writeErrorCode(w, http.StatusNotFound, "TASK_NOT_FOUND", "task not found")
		return
	}
	if task.IsParent {
		writeErrorCode(w, http.StatusBadRequest, "START_CONSTRAINT_NOT_ALLOWED_ON_PARENT",
			"ограничение «Начать не ранее» можно задать только листовой (ролевой) задаче")
		return
	}

	epic, err := h.repo.GetEpicByID(r.Context(), task.EpicID)
	if err != nil {
		h.log.Error("failed to resolve task's epic", slog.String("error", err.Error()))
		writeErrorCode(w, http.StatusInternalServerError, "EPIC_LOOKUP_FAILED",
			"failed to resolve task's team")
		return
	}

	// Точечная team-scoped проверка — см. комментарий у SetTaskAssignee:
	// RoleAuth("admin") на уровне группы маршрутов пропускает admin ЛЮБОЙ
	// команды, эта проверка не даёт admin-у команды A менять ограничение
	// задачи команды B.
	if role != "superadmin" {
		isAdminOf, err := h.repo.IsTeamAdminOf(r.Context(), session.TelegramID, epic.TeamID)
		if err != nil || !isAdminOf {
			writeErrorCode(w, http.StatusForbidden, "FORBIDDEN",
				"вы не администратор команды, которой принадлежит эта задача")
			return
		}
	}

	tasks, err := h.svc.SetTaskStartConstraint(r.Context(), taskID, notBeforeDate, waitForStoryID, waitForRoleID)
	if err != nil {
		switch {
		case errors.Is(err, gantt.ErrStartConstraintIncompleteRef):
			writeErrorCode(w, http.StatusBadRequest, "INCOMPLETE_TASK_REFERENCE",
				"нужно указать и стори, и роль выбранной задачи, либо не указывать ни одно из полей")
		case errors.Is(err, gantt.ErrStartConstraintCrossTeam):
			writeErrorCode(w, http.StatusBadRequest, "TASK_REFERENCE_CROSS_TEAM",
				"выбранная задача принадлежит другой команде")
		case errors.Is(err, gantt.ErrStartConstraintCycle):
			writeErrorCode(w, http.StatusBadRequest, "START_CONSTRAINT_CYCLE",
				"выбранная задача уже (прямо или через цепочку) ожидает текущую — это замкнуло бы цикл")
		case errors.Is(err, gantt.ErrStartConstraintOnParent):
			// Защитный случай — уже отсечён проверкой task.IsParent выше,
			// но сопоставляем на случай расхождения с сервисом.
			writeErrorCode(w, http.StatusBadRequest, "START_CONSTRAINT_NOT_ALLOWED_ON_PARENT",
				"ограничение «Начать не ранее» можно задать только листовой (ролевой) задаче")
		default:
			h.log.Error("failed to set task start constraint", slog.String("error", err.Error()))
			writeErrorCode(w, http.StatusInternalServerError, "RESCHEDULE_FAILED",
				"failed to set task start constraint")
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message": "start constraint updated",
		"count":   len(tasks),
	})
}

// GetTaskStartConstraintOptions отдаёт состав задач команды для
// двухшагового выбора цели ограничения «Начать не ранее» (backend §3.3,
// design.md Решение 6): стори, внутри каждой — роли. Недоступные для выбора
// варианты (сама задача и всё, что замкнуло бы цикл ожидания) ПОМЕЧЕНЫ, а
// не исключены из выдачи — иначе пользователь искал бы пропавшую строку.
// Доступен любому аутентифицированному пользователю, как и GetTeamMembers —
// само чтение ничего не меняет, редактирование гейтится в
// SetTaskStartConstraint.
func (h *GanttHandler) GetTaskStartConstraintOptions(w http.ResponseWriter, r *http.Request) {
	taskIDStr := chi.URLParam(r, "id")
	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_TASK_ID", "invalid task id")
		return
	}

	options, err := h.svc.GetTeamTaskOptionsFor(r.Context(), taskID)
	if err != nil {
		if errors.Is(err, gantt.ErrStartConstraintOnParent) {
			writeErrorCode(w, http.StatusBadRequest, "START_CONSTRAINT_NOT_ALLOWED_ON_PARENT",
				"ограничение «Начать не ранее» можно задать только листовой (ролевой) задаче")
			return
		}
		h.log.Error("failed to get task start constraint options", slog.String("error", err.Error()))
		writeErrorCode(w, http.StatusInternalServerError, "TASK_OPTIONS_LOOKUP_FAILED",
			"failed to get task options")
		return
	}

	type roleOptionResp struct {
		RoleID            string `json:"role_id"`
		RoleName          string `json:"role_name"`
		Unavailable       bool   `json:"unavailable"`
		UnavailableReason string `json:"unavailable_reason,omitempty"`
	}
	type storyOptionResp struct {
		StoryID   string           `json:"story_id"`
		StoryName string           `json:"story_name"`
		Roles     []roleOptionResp `json:"roles"`
	}

	// Группировка по стори с сохранением порядка первого появления —
	// GetTeamTaskOptionsFor уже отдаёт список, отсортированный по
	// (StoryName, RoleName), поэтому первое появление StoryID и есть
	// правильный порядок стори (design.md Решение 6: та же иерархия,
	// что на диаграмме).
	var stories []storyOptionResp
	indexByStoryID := make(map[string]int)
	for _, o := range options {
		storyIDStr := o.StoryID.String()
		idx, ok := indexByStoryID[storyIDStr]
		if !ok {
			idx = len(stories)
			indexByStoryID[storyIDStr] = idx
			stories = append(stories, storyOptionResp{StoryID: storyIDStr, StoryName: o.StoryName})
		}
		stories[idx].Roles = append(stories[idx].Roles, roleOptionResp{
			RoleID:            o.RoleID.String(),
			RoleName:          o.RoleName,
			Unavailable:       o.Unavailable,
			UnavailableReason: o.UnavailableReason,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"stories": stories})
}

// GetTeamMembers returns a team's roster with each member's full set of
// roles — the candidate source for the assignee dropdown (design.md
// Решение 9). Unlike Repository.GetRoleByUserID (one role per user), this
// correctly reflects user_roles as an M:N relation.
func (h *GanttHandler) GetTeamMembers(w http.ResponseWriter, r *http.Request) {
	teamIDStr := chi.URLParam(r, "id")
	teamID, err := uuid.Parse(teamIDStr)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_TEAM_ID", "invalid team id")
		return
	}

	if _, err := h.repo.GetTeamByID(r.Context(), teamID); err != nil {
		writeErrorCode(w, http.StatusNotFound, "TEAM_NOT_FOUND", "team not found")
		return
	}

	members, err := h.svc.GetTeamMembers(r.Context(), teamID)
	if err != nil {
		h.log.Error("failed to get team members", slog.String("error", err.Error()))
		writeErrorCode(w, http.StatusInternalServerError, "TEAM_MEMBERS_LOOKUP_FAILED",
			"failed to get team members")
		return
	}

	type memberResp struct {
		ID        string   `json:"id"`
		FirstName string   `json:"first_name"`
		LastName  string   `json:"last_name"`
		RoleIDs   []string `json:"role_ids"`
	}
	resp := make([]memberResp, 0, len(members))
	for _, m := range members {
		roleIDs := make([]string, len(m.RoleIDs))
		for i, rid := range m.RoleIDs {
			roleIDs[i] = rid.String()
		}
		resp = append(resp, memberResp{
			ID:        m.ID.String(),
			FirstName: m.FirstName,
			LastName:  m.LastName,
			RoleIDs:   roleIDs,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"members": resp})
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
		SortOrder int `json:"new_sort_order"`
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

// ReorderEpic changes a top-level epic's position in its team's pipeline
// queue and recalculates the whole team's schedule.
func (h *GanttHandler) ReorderEpic(w http.ResponseWriter, r *http.Request) {
	epicIDStr := chi.URLParam(r, "epic_id")
	epicID, err := uuid.Parse(epicIDStr)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_EPIC_ID", "invalid epic id")
		return
	}

	var req struct {
		SortOrder int `json:"new_sort_order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_REQUEST_BODY", "invalid request body")
		return
	}

	tasks, err := h.svc.ReorderEpic(r.Context(), epicID, req.SortOrder)
	if err != nil {
		h.log.Error("failed to reorder epic",
			slog.String("error", err.Error()))
		// Сервис не возвращает типизированные sentinel-ошибки (не найден
		// эпик / стори вместо эпика / ошибка пересчёта расписания), поэтому
		// все ошибки уровня сервиса возвращаются одним общим кодом.
		writeErrorCode(w, http.StatusInternalServerError, "RESCHEDULE_FAILED",
			"failed to reorder epic")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message": "epic reordered",
		"count":   len(tasks),
	})
}

// ReorderStory changes a story's position in its parent epic's pipeline
// queue and recalculates the whole team's schedule.
func (h *GanttHandler) ReorderStory(w http.ResponseWriter, r *http.Request) {
	storyIDStr := chi.URLParam(r, "story_id")
	storyID, err := uuid.Parse(storyIDStr)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_STORY_ID", "invalid story id")
		return
	}

	var req struct {
		SortOrder int `json:"new_sort_order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_REQUEST_BODY", "invalid request body")
		return
	}

	tasks, err := h.svc.ReorderStory(r.Context(), storyID, req.SortOrder)
	if err != nil {
		h.log.Error("failed to reorder story",
			slog.String("error", err.Error()))
		// См. комментарий в ReorderEpic — сервис не возвращает
		// типизированные sentinel-ошибки, поэтому один общий код.
		writeErrorCode(w, http.StatusInternalServerError, "RESCHEDULE_FAILED",
			"failed to reorder story")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message": "story reordered",
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
		// 2. Is team-admin (team_admins в БД, team-scoped) хотя бы одной команды?
		isAdmin, errAdmin := h.repo.IsTeamAdminOfAny(r.Context(), session.TelegramID)
		if errAdmin != nil {
			isAdmin = false
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
