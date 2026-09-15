package handlers

import (
	"EpicScoreBot/internal/config"
	"EpicScoreBot/internal/gantt"
	"EpicScoreBot/internal/models/domain"
	"EpicScoreBot/internal/transport/httpServer/middleware"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Тесты HTTP-слоя ограничения "Начать не ранее" (openspec/changes/
// add-task-start-constraints, backend §3.1-3.4). Используют те же
// mockGanttRepo/mockGanttSvc и newAssigneeRequest, что и assignee_test.go —
// установленная в этом пакете конвенция для хендлер-тестов, вызывающих
// функцию хендлера напрямую, в обход роутера/middleware (маршрутизация и
// закрытость middleware отдельно проверяются в routers_mount_test.go).

// --- PUT /api/gantt/tasks/{id}/start-constraint (backend §3.2) ---

func TestSetTaskStartConstraint_Unauthorized(t *testing.T) {
	repo := &mockGanttRepo{}
	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	taskID := uuid.New()
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/tasks/"+taskID.String()+"/start-constraint", "id", taskID.String(), `{}`, nil)
	w := httptest.NewRecorder()

	handler.SetTaskStartConstraint(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d, body: %s", w.Code, w.Body.String())
	}
	decodeError(t, w.Body.Bytes())
}

func TestSetTaskStartConstraint_RejectsMember(t *testing.T) {
	repo := &mockGanttRepo{
		isTeamAdminOfAny: false,
		findUserByTelegramIDFunc: func(ctx context.Context, telegramID string) (*domain.User, error) {
			return &domain.User{ID: uuid.New(), TelegramID: telegramID}, nil
		},
	}
	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	taskID := uuid.New()
	session := &middleware.UserSession{TelegramID: "member-1", Username: "plain_member"}
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/tasks/"+taskID.String()+"/start-constraint", "id", taskID.String(), `{}`, session)
	w := httptest.NewRecorder()

	handler.SetTaskStartConstraint(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for member role, got %d, body: %s", w.Code, w.Body.String())
	}
	decodeError(t, w.Body.Bytes())
	if svc.setTaskStartConstraintCalled {
		t.Errorf("service must not be called when the request is rejected for role")
	}
}

func TestSetTaskStartConstraint_InvalidTaskID(t *testing.T) {
	repo := &mockGanttRepo{}
	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	req := newAssigneeRequest(http.MethodPut, "/api/gantt/tasks/not-a-uuid/start-constraint", "id", "not-a-uuid", `{}`, nil)
	w := httptest.NewRecorder()

	handler.SetTaskStartConstraint(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body: %s", w.Code, w.Body.String())
	}
	e := decodeError(t, w.Body.Bytes())
	if e.Error.Code != "INVALID_TASK_ID" {
		t.Errorf("expected error code INVALID_TASK_ID, got %q", e.Error.Code)
	}
}

func TestSetTaskStartConstraint_RejectsParentTask(t *testing.T) {
	taskID := uuid.New()
	repo := &mockGanttRepo{
		isTeamAdminOfAny: true,
		getTaskFunc: func(ctx context.Context, id uuid.UUID) (*domain.GanttTask, error) {
			return &domain.GanttTask{ID: taskID, IsParent: true}, nil
		},
	}
	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	session := &middleware.UserSession{TelegramID: "admin-1", Username: "team_admin"}
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/tasks/"+taskID.String()+"/start-constraint", "id", taskID.String(), `{}`, session)
	w := httptest.NewRecorder()

	handler.SetTaskStartConstraint(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body: %s", w.Code, w.Body.String())
	}
	e := decodeError(t, w.Body.Bytes())
	if e.Error.Code != "START_CONSTRAINT_NOT_ALLOWED_ON_PARENT" {
		t.Errorf("expected error code START_CONSTRAINT_NOT_ALLOWED_ON_PARENT, got %q", e.Error.Code)
	}
	if svc.setTaskStartConstraintCalled {
		t.Errorf("service must not be called for a parent task")
	}
}

func TestSetTaskStartConstraint_TaskNotFound(t *testing.T) {
	taskID := uuid.New()
	repo := &mockGanttRepo{
		isTeamAdminOfAny: true,
		getTaskFunc: func(ctx context.Context, id uuid.UUID) (*domain.GanttTask, error) {
			return nil, errors.New("not found")
		},
	}
	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	session := &middleware.UserSession{TelegramID: "admin-1", Username: "team_admin"}
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/tasks/"+taskID.String()+"/start-constraint", "id", taskID.String(), `{}`, session)
	w := httptest.NewRecorder()

	handler.SetTaskStartConstraint(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body: %s", w.Code, w.Body.String())
	}
	e := decodeError(t, w.Body.Bytes())
	if e.Error.Code != "TASK_NOT_FOUND" {
		t.Errorf("expected error code TASK_NOT_FOUND, got %q", e.Error.Code)
	}
}

func TestSetTaskStartConstraint_InvalidRequestBody(t *testing.T) {
	taskID := uuid.New()
	repo := &mockGanttRepo{isTeamAdminOfAny: true}
	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	session := &middleware.UserSession{TelegramID: "admin-1", Username: "team_admin"}
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/tasks/"+taskID.String()+"/start-constraint", "id", taskID.String(), `not json`, session)
	w := httptest.NewRecorder()

	handler.SetTaskStartConstraint(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body: %s", w.Code, w.Body.String())
	}
	e := decodeError(t, w.Body.Bytes())
	if e.Error.Code != "INVALID_REQUEST_BODY" {
		t.Errorf("expected error code INVALID_REQUEST_BODY, got %q", e.Error.Code)
	}
}

func TestSetTaskStartConstraint_InvalidNotBeforeDate(t *testing.T) {
	taskID := uuid.New()
	repo := &mockGanttRepo{isTeamAdminOfAny: true}
	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	session := &middleware.UserSession{TelegramID: "admin-1", Username: "team_admin"}
	body := `{"not_before_date":"13-07-2026"}`
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/tasks/"+taskID.String()+"/start-constraint", "id", taskID.String(), body, session)
	w := httptest.NewRecorder()

	handler.SetTaskStartConstraint(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body: %s", w.Code, w.Body.String())
	}
	e := decodeError(t, w.Body.Bytes())
	if e.Error.Code != "INVALID_NOT_BEFORE_DATE" {
		t.Errorf("expected error code INVALID_NOT_BEFORE_DATE, got %q", e.Error.Code)
	}
}

func TestSetTaskStartConstraint_InvalidWaitForStoryID(t *testing.T) {
	taskID := uuid.New()
	repo := &mockGanttRepo{isTeamAdminOfAny: true}
	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	session := &middleware.UserSession{TelegramID: "admin-1", Username: "team_admin"}
	body := `{"wait_for_story_id":"not-a-uuid","wait_for_role_id":"` + uuid.New().String() + `"}`
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/tasks/"+taskID.String()+"/start-constraint", "id", taskID.String(), body, session)
	w := httptest.NewRecorder()

	handler.SetTaskStartConstraint(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body: %s", w.Code, w.Body.String())
	}
	e := decodeError(t, w.Body.Bytes())
	if e.Error.Code != "INVALID_WAIT_FOR_STORY_ID" {
		t.Errorf("expected error code INVALID_WAIT_FOR_STORY_ID, got %q", e.Error.Code)
	}
}

func TestSetTaskStartConstraint_InvalidWaitForRoleID(t *testing.T) {
	taskID := uuid.New()
	repo := &mockGanttRepo{isTeamAdminOfAny: true}
	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	session := &middleware.UserSession{TelegramID: "admin-1", Username: "team_admin"}
	body := `{"wait_for_story_id":"` + uuid.New().String() + `","wait_for_role_id":"not-a-uuid"}`
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/tasks/"+taskID.String()+"/start-constraint", "id", taskID.String(), body, session)
	w := httptest.NewRecorder()

	handler.SetTaskStartConstraint(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body: %s", w.Code, w.Body.String())
	}
	e := decodeError(t, w.Body.Bytes())
	if e.Error.Code != "INVALID_WAIT_FOR_ROLE_ID" {
		t.Errorf("expected error code INVALID_WAIT_FOR_ROLE_ID, got %q", e.Error.Code)
	}
}

// TestSetTaskStartConstraint_RejectsCrossTeamAdmin — тот же приём, что и
// SetTaskAssignee (см. assignee_test.go): admin команды A получает 403,
// если задача принадлежит команде B, где он не team-admin.
func TestSetTaskStartConstraint_RejectsCrossTeamAdmin(t *testing.T) {
	ownTeam := uuid.New()
	foreignTeam := uuid.New()
	roleID := uuid.New()
	epicID := uuid.New()
	taskID := uuid.New()

	repo := &mockGanttRepo{
		isTeamAdminOfAny: true,
		getTaskFunc: func(ctx context.Context, id uuid.UUID) (*domain.GanttTask, error) {
			return &domain.GanttTask{ID: taskID, EpicID: epicID, RoleID: &roleID, IsParent: false}, nil
		},
		getEpicByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Epic, error) {
			return &domain.Epic{ID: epicID, TeamID: foreignTeam}, nil
		},
		isTeamAdminOfFunc: func(ctx context.Context, telegramID string, tID uuid.UUID) (bool, error) {
			return tID == ownTeam, nil
		},
	}
	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	session := &middleware.UserSession{TelegramID: "admin-a", Username: "team_admin_a"}
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/tasks/"+taskID.String()+"/start-constraint", "id", taskID.String(), `{}`, session)
	w := httptest.NewRecorder()

	handler.SetTaskStartConstraint(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-team admin, got %d, body: %s", w.Code, w.Body.String())
	}
	e := decodeError(t, w.Body.Bytes())
	if e.Error.Code != "FORBIDDEN" {
		t.Errorf("expected error code FORBIDDEN, got %q", e.Error.Code)
	}
	if svc.setTaskStartConstraintCalled {
		t.Errorf("service must not be called when the admin is out of scope for the task's team")
	}
}

func TestSetTaskStartConstraint_SuperAdminBypassesTeamScope(t *testing.T) {
	teamID := uuid.New()
	roleID := uuid.New()
	epicID := uuid.New()
	taskID := uuid.New()

	repo := &mockGanttRepo{
		isTeamAdminOfAny: false,
		getTaskFunc: func(ctx context.Context, id uuid.UUID) (*domain.GanttTask, error) {
			return &domain.GanttTask{ID: taskID, EpicID: epicID, RoleID: &roleID, IsParent: false}, nil
		},
		getEpicByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Epic, error) {
			return &domain.Epic{ID: epicID, TeamID: teamID}, nil
		},
		isTeamAdminOfFunc: func(ctx context.Context, telegramID string, tID uuid.UUID) (bool, error) {
			return false, nil
		},
	}
	svc := &mockGanttSvc{}
	cfg := config.BotConfig{SuperAdmins: []string{"root"}}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})

	session := &middleware.UserSession{TelegramID: "super-1", Username: "root"}
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/tasks/"+taskID.String()+"/start-constraint", "id", taskID.String(), `{}`, session)
	w := httptest.NewRecorder()

	handler.SetTaskStartConstraint(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for superadmin, got %d, body: %s", w.Code, w.Body.String())
	}
	if !svc.setTaskStartConstraintCalled {
		t.Errorf("expected service to be called for superadmin")
	}
}

// TestSetTaskStartConstraint_SetsDateAndReference проверяет успешный путь:
// оба поля разбираются и передаются в сервис как есть, ответ содержит
// пересчитанное число задач.
func TestSetTaskStartConstraint_SetsDateAndReference(t *testing.T) {
	teamID := uuid.New()
	roleID := uuid.New()
	epicID := uuid.New()
	taskID := uuid.New()
	waitForStoryID := uuid.New()
	waitForRoleID := uuid.New()

	repo := &mockGanttRepo{
		isTeamAdminOfAny: true,
		getTaskFunc: func(ctx context.Context, id uuid.UUID) (*domain.GanttTask, error) {
			return &domain.GanttTask{ID: taskID, EpicID: epicID, RoleID: &roleID, IsParent: false}, nil
		},
		getEpicByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Epic, error) {
			return &domain.Epic{ID: epicID, TeamID: teamID}, nil
		},
		isTeamAdminOfFunc: func(ctx context.Context, telegramID string, tID uuid.UUID) (bool, error) {
			return tID == teamID, nil
		},
	}
	svc := &mockGanttSvc{
		setTaskStartConstraintFunc: func(ctx context.Context, tID uuid.UUID, notBefore *time.Time, storyID, roleIDArg *uuid.UUID) ([]domain.GanttTask, error) {
			return []domain.GanttTask{{ID: taskID}, {ID: uuid.New()}}, nil
		},
	}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	session := &middleware.UserSession{TelegramID: "admin-1", Username: "team_admin"}
	body := `{"not_before_date":"2026-08-10","wait_for_story_id":"` + waitForStoryID.String() + `","wait_for_role_id":"` + waitForRoleID.String() + `"}`
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/tasks/"+taskID.String()+"/start-constraint", "id", taskID.String(), body, session)
	w := httptest.NewRecorder()

	handler.SetTaskStartConstraint(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}
	if !svc.setTaskStartConstraintCalled {
		t.Fatalf("expected service.SetTaskStartConstraint to be called")
	}
	if svc.setTaskStartConstraintArgs.taskID != taskID {
		t.Errorf("taskID = %v, want %v", svc.setTaskStartConstraintArgs.taskID, taskID)
	}
	wantDate := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	if svc.setTaskStartConstraintArgs.notBeforeDate == nil || !svc.setTaskStartConstraintArgs.notBeforeDate.Equal(wantDate) {
		t.Errorf("notBeforeDate = %v, want %v", svc.setTaskStartConstraintArgs.notBeforeDate, wantDate)
	}
	if svc.setTaskStartConstraintArgs.waitForStoryID == nil || *svc.setTaskStartConstraintArgs.waitForStoryID != waitForStoryID {
		t.Errorf("waitForStoryID = %v, want %v", svc.setTaskStartConstraintArgs.waitForStoryID, waitForStoryID)
	}
	if svc.setTaskStartConstraintArgs.waitForRoleID == nil || *svc.setTaskStartConstraintArgs.waitForRoleID != waitForRoleID {
		t.Errorf("waitForRoleID = %v, want %v", svc.setTaskStartConstraintArgs.waitForRoleID, waitForRoleID)
	}

	var resp struct {
		Message string `json:"message"`
		Count   int    `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Count != 2 {
		t.Errorf("count = %d, want 2", resp.Count)
	}
}

// TestSetTaskStartConstraint_ClearsViaNulls проверяет снятие ограничения:
// поле отсутствует/равно null в теле запроса -> nil передаётся в сервис.
func TestSetTaskStartConstraint_ClearsViaNulls(t *testing.T) {
	teamID := uuid.New()
	roleID := uuid.New()
	epicID := uuid.New()
	taskID := uuid.New()

	repo := &mockGanttRepo{
		isTeamAdminOfAny: true,
		getTaskFunc: func(ctx context.Context, id uuid.UUID) (*domain.GanttTask, error) {
			return &domain.GanttTask{ID: taskID, EpicID: epicID, RoleID: &roleID, IsParent: false}, nil
		},
		getEpicByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Epic, error) {
			return &domain.Epic{ID: epicID, TeamID: teamID}, nil
		},
		isTeamAdminOfFunc: func(ctx context.Context, telegramID string, tID uuid.UUID) (bool, error) {
			return tID == teamID, nil
		},
	}
	svc := &mockGanttSvc{
		setTaskStartConstraintFunc: func(ctx context.Context, tID uuid.UUID, notBefore *time.Time, storyID, roleIDArg *uuid.UUID) ([]domain.GanttTask, error) {
			return nil, nil
		},
	}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	session := &middleware.UserSession{TelegramID: "admin-1", Username: "team_admin"}
	body := `{"not_before_date":null,"wait_for_story_id":null,"wait_for_role_id":null}`
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/tasks/"+taskID.String()+"/start-constraint", "id", taskID.String(), body, session)
	w := httptest.NewRecorder()

	handler.SetTaskStartConstraint(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}
	if svc.setTaskStartConstraintArgs.notBeforeDate != nil {
		t.Errorf("notBeforeDate = %v, want nil", svc.setTaskStartConstraintArgs.notBeforeDate)
	}
	if svc.setTaskStartConstraintArgs.waitForStoryID != nil {
		t.Errorf("waitForStoryID = %v, want nil", svc.setTaskStartConstraintArgs.waitForStoryID)
	}
	if svc.setTaskStartConstraintArgs.waitForRoleID != nil {
		t.Errorf("waitForRoleID = %v, want nil", svc.setTaskStartConstraintArgs.waitForRoleID)
	}
}

// --- Сопоставление сентинел-ошибок сервиса с кодами ответа (backend §3.2) ---

func TestSetTaskStartConstraint_MapsIncompleteRefError(t *testing.T) {
	teamID := uuid.New()
	roleID := uuid.New()
	epicID := uuid.New()
	taskID := uuid.New()

	repo := &mockGanttRepo{
		isTeamAdminOfAny: true,
		getTaskFunc: func(ctx context.Context, id uuid.UUID) (*domain.GanttTask, error) {
			return &domain.GanttTask{ID: taskID, EpicID: epicID, RoleID: &roleID, IsParent: false}, nil
		},
		getEpicByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Epic, error) {
			return &domain.Epic{ID: epicID, TeamID: teamID}, nil
		},
		isTeamAdminOfFunc: func(ctx context.Context, telegramID string, tID uuid.UUID) (bool, error) {
			return tID == teamID, nil
		},
	}
	svc := &mockGanttSvc{
		setTaskStartConstraintFunc: func(ctx context.Context, tID uuid.UUID, notBefore *time.Time, storyID, roleIDArg *uuid.UUID) ([]domain.GanttTask, error) {
			return nil, gantt.ErrStartConstraintIncompleteRef
		},
	}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	session := &middleware.UserSession{TelegramID: "admin-1", Username: "team_admin"}
	body := `{"wait_for_story_id":"` + uuid.New().String() + `"}`
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/tasks/"+taskID.String()+"/start-constraint", "id", taskID.String(), body, session)
	w := httptest.NewRecorder()

	handler.SetTaskStartConstraint(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body: %s", w.Code, w.Body.String())
	}
	e := decodeError(t, w.Body.Bytes())
	if e.Error.Code != "INCOMPLETE_TASK_REFERENCE" {
		t.Errorf("expected error code INCOMPLETE_TASK_REFERENCE, got %q", e.Error.Code)
	}
}

func TestSetTaskStartConstraint_MapsCrossTeamRefError(t *testing.T) {
	teamID := uuid.New()
	roleID := uuid.New()
	epicID := uuid.New()
	taskID := uuid.New()

	repo := &mockGanttRepo{
		isTeamAdminOfAny: true,
		getTaskFunc: func(ctx context.Context, id uuid.UUID) (*domain.GanttTask, error) {
			return &domain.GanttTask{ID: taskID, EpicID: epicID, RoleID: &roleID, IsParent: false}, nil
		},
		getEpicByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Epic, error) {
			return &domain.Epic{ID: epicID, TeamID: teamID}, nil
		},
		isTeamAdminOfFunc: func(ctx context.Context, telegramID string, tID uuid.UUID) (bool, error) {
			return tID == teamID, nil
		},
	}
	svc := &mockGanttSvc{
		setTaskStartConstraintFunc: func(ctx context.Context, tID uuid.UUID, notBefore *time.Time, storyID, roleIDArg *uuid.UUID) ([]domain.GanttTask, error) {
			return nil, gantt.ErrStartConstraintCrossTeam
		},
	}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	session := &middleware.UserSession{TelegramID: "admin-1", Username: "team_admin"}
	body := `{"wait_for_story_id":"` + uuid.New().String() + `","wait_for_role_id":"` + uuid.New().String() + `"}`
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/tasks/"+taskID.String()+"/start-constraint", "id", taskID.String(), body, session)
	w := httptest.NewRecorder()

	handler.SetTaskStartConstraint(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body: %s", w.Code, w.Body.String())
	}
	e := decodeError(t, w.Body.Bytes())
	if e.Error.Code != "TASK_REFERENCE_CROSS_TEAM" {
		t.Errorf("expected error code TASK_REFERENCE_CROSS_TEAM, got %q", e.Error.Code)
	}
}

func TestSetTaskStartConstraint_MapsCycleError(t *testing.T) {
	teamID := uuid.New()
	roleID := uuid.New()
	epicID := uuid.New()
	taskID := uuid.New()

	repo := &mockGanttRepo{
		isTeamAdminOfAny: true,
		getTaskFunc: func(ctx context.Context, id uuid.UUID) (*domain.GanttTask, error) {
			return &domain.GanttTask{ID: taskID, EpicID: epicID, RoleID: &roleID, IsParent: false}, nil
		},
		getEpicByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Epic, error) {
			return &domain.Epic{ID: epicID, TeamID: teamID}, nil
		},
		isTeamAdminOfFunc: func(ctx context.Context, telegramID string, tID uuid.UUID) (bool, error) {
			return tID == teamID, nil
		},
	}
	svc := &mockGanttSvc{
		setTaskStartConstraintFunc: func(ctx context.Context, tID uuid.UUID, notBefore *time.Time, storyID, roleIDArg *uuid.UUID) ([]domain.GanttTask, error) {
			return nil, gantt.ErrStartConstraintCycle
		},
	}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	session := &middleware.UserSession{TelegramID: "admin-1", Username: "team_admin"}
	body := `{"wait_for_story_id":"` + uuid.New().String() + `","wait_for_role_id":"` + uuid.New().String() + `"}`
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/tasks/"+taskID.String()+"/start-constraint", "id", taskID.String(), body, session)
	w := httptest.NewRecorder()

	handler.SetTaskStartConstraint(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body: %s", w.Code, w.Body.String())
	}
	e := decodeError(t, w.Body.Bytes())
	if e.Error.Code != "START_CONSTRAINT_CYCLE" {
		t.Errorf("expected error code START_CONSTRAINT_CYCLE, got %q", e.Error.Code)
	}
}

func TestSetTaskStartConstraint_MapsUnknownErrorToRescheduleFailed(t *testing.T) {
	teamID := uuid.New()
	roleID := uuid.New()
	epicID := uuid.New()
	taskID := uuid.New()

	repo := &mockGanttRepo{
		isTeamAdminOfAny: true,
		getTaskFunc: func(ctx context.Context, id uuid.UUID) (*domain.GanttTask, error) {
			return &domain.GanttTask{ID: taskID, EpicID: epicID, RoleID: &roleID, IsParent: false}, nil
		},
		getEpicByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Epic, error) {
			return &domain.Epic{ID: epicID, TeamID: teamID}, nil
		},
		isTeamAdminOfFunc: func(ctx context.Context, telegramID string, tID uuid.UUID) (bool, error) {
			return tID == teamID, nil
		},
	}
	svc := &mockGanttSvc{
		setTaskStartConstraintFunc: func(ctx context.Context, tID uuid.UUID, notBefore *time.Time, storyID, roleIDArg *uuid.UUID) ([]domain.GanttTask, error) {
			return nil, errors.New("db exploded")
		},
	}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	session := &middleware.UserSession{TelegramID: "admin-1", Username: "team_admin"}
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/tasks/"+taskID.String()+"/start-constraint", "id", taskID.String(), `{}`, session)
	w := httptest.NewRecorder()

	handler.SetTaskStartConstraint(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d, body: %s", w.Code, w.Body.String())
	}
	e := decodeError(t, w.Body.Bytes())
	if e.Error.Code != "RESCHEDULE_FAILED" {
		t.Errorf("expected error code RESCHEDULE_FAILED, got %q", e.Error.Code)
	}
}

// --- GET /api/gantt/tasks/{id}/start-constraint/options (backend §3.3) ---

func TestGetTaskStartConstraintOptions_InvalidTaskID(t *testing.T) {
	repo := &mockGanttRepo{}
	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	req := newAssigneeRequest(http.MethodGet, "/api/gantt/tasks/not-a-uuid/start-constraint/options", "id", "not-a-uuid", "", nil)
	w := httptest.NewRecorder()

	handler.GetTaskStartConstraintOptions(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body: %s", w.Code, w.Body.String())
	}
	e := decodeError(t, w.Body.Bytes())
	if e.Error.Code != "INVALID_TASK_ID" {
		t.Errorf("expected error code INVALID_TASK_ID, got %q", e.Error.Code)
	}
}

func TestGetTaskStartConstraintOptions_MapsOnParentError(t *testing.T) {
	taskID := uuid.New()
	repo := &mockGanttRepo{}
	svc := &mockGanttSvc{
		getTeamTaskOptionsForFunc: func(ctx context.Context, tID uuid.UUID) ([]gantt.TeamTaskOption, error) {
			return nil, gantt.ErrStartConstraintOnParent
		},
	}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	req := newAssigneeRequest(http.MethodGet, "/api/gantt/tasks/"+taskID.String()+"/start-constraint/options", "id", taskID.String(), "", nil)
	w := httptest.NewRecorder()

	handler.GetTaskStartConstraintOptions(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body: %s", w.Code, w.Body.String())
	}
	e := decodeError(t, w.Body.Bytes())
	if e.Error.Code != "START_CONSTRAINT_NOT_ALLOWED_ON_PARENT" {
		t.Errorf("expected error code START_CONSTRAINT_NOT_ALLOWED_ON_PARENT, got %q", e.Error.Code)
	}
}

func TestGetTaskStartConstraintOptions_InternalError(t *testing.T) {
	taskID := uuid.New()
	repo := &mockGanttRepo{}
	svc := &mockGanttSvc{
		getTeamTaskOptionsForFunc: func(ctx context.Context, tID uuid.UUID) ([]gantt.TeamTaskOption, error) {
			return nil, errors.New("db exploded")
		},
	}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	req := newAssigneeRequest(http.MethodGet, "/api/gantt/tasks/"+taskID.String()+"/start-constraint/options", "id", taskID.String(), "", nil)
	w := httptest.NewRecorder()

	handler.GetTaskStartConstraintOptions(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d, body: %s", w.Code, w.Body.String())
	}
	e := decodeError(t, w.Body.Bytes())
	if e.Error.Code != "TASK_OPTIONS_LOOKUP_FAILED" {
		t.Errorf("expected error code TASK_OPTIONS_LOOKUP_FAILED, got %q", e.Error.Code)
	}
}

// TestGetTaskStartConstraintOptions_GroupsByStoryAndMarksUnavailable
// проверяет JSON-контракт: плоский список TeamTaskOption группируется по
// стори (первое появление StoryID задаёт порядок стори в ответе — сервис
// уже отдаёт список, отсортированный по StoryName/RoleName), недоступные
// варианты помечены, а не исключены (design.md Решение 6).
func TestGetTaskStartConstraintOptions_GroupsByStoryAndMarksUnavailable(t *testing.T) {
	taskID := uuid.New()
	story1 := uuid.New()
	story2 := uuid.New()
	roleA := uuid.New()
	roleB := uuid.New()
	roleC := uuid.New()

	repo := &mockGanttRepo{}
	svc := &mockGanttSvc{
		getTeamTaskOptionsForFunc: func(ctx context.Context, tID uuid.UUID) ([]gantt.TeamTaskOption, error) {
			return []gantt.TeamTaskOption{
				{StoryID: story1, StoryName: "E-1-S1: Story One", RoleID: roleA, RoleName: "BE разработчик"},
				{StoryID: story1, StoryName: "E-1-S1: Story One", RoleID: roleB, RoleName: "FE разработчик", Unavailable: true, UnavailableReason: "это и есть текущая задача"},
				{StoryID: story2, StoryName: "E-1-S2: Story Two", RoleID: roleC, RoleName: "Тестировщик", Unavailable: true, UnavailableReason: "выбор замкнёт цикл ожидания"},
			}, nil
		},
	}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	req := newAssigneeRequest(http.MethodGet, "/api/gantt/tasks/"+taskID.String()+"/start-constraint/options", "id", taskID.String(), "", nil)
	w := httptest.NewRecorder()

	handler.GetTaskStartConstraintOptions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Stories []struct {
			StoryID   string `json:"story_id"`
			StoryName string `json:"story_name"`
			Roles     []struct {
				RoleID            string `json:"role_id"`
				RoleName          string `json:"role_name"`
				Unavailable       bool   `json:"unavailable"`
				UnavailableReason string `json:"unavailable_reason,omitempty"`
			} `json:"roles"`
		} `json:"stories"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Stories) != 2 {
		t.Fatalf("expected 2 story groups, got %d: %+v", len(resp.Stories), resp.Stories)
	}
	s1 := resp.Stories[0]
	if s1.StoryID != story1.String() || s1.StoryName != "E-1-S1: Story One" {
		t.Errorf("unexpected first story group: %+v", s1)
	}
	if len(s1.Roles) != 2 {
		t.Fatalf("expected 2 roles under story1, got %d", len(s1.Roles))
	}
	if s1.Roles[0].Unavailable {
		t.Errorf("expected first role (roleA) to be available")
	}
	if !s1.Roles[1].Unavailable || s1.Roles[1].UnavailableReason == "" {
		t.Errorf("expected second role (roleB, self) to be unavailable with a reason, got %+v", s1.Roles[1])
	}

	s2 := resp.Stories[1]
	if s2.StoryID != story2.String() {
		t.Errorf("unexpected second story group: %+v", s2)
	}
	if len(s2.Roles) != 1 || !s2.Roles[0].Unavailable {
		t.Errorf("expected story2's single role to be unavailable (cycle), got %+v", s2.Roles)
	}
}

// --- GET /api/gantt/tasks — not_before_date/wait_for_*/
// start_constraint_ref_invalid (backend §3.1) ---

// TestGetTasks_StartConstraintFields проверяет три состояния ограничения
// "Начать не ранее" в ответе GetTasks одновременно: заданы оба поля и
// ссылка действует; ссылка задана, но недействует (её цель не входит в
// validRefs — например, исчезнувшая задача, design.md Решение 4);
// ограничение не задано вовсе.
func TestGetTasks_StartConstraintFields(t *testing.T) {
	teamID := uuid.New()
	roleID := uuid.New()

	bothValidID, invalidRefID, noneID := uuid.New(), uuid.New(), uuid.New()
	waitForStory, waitForRole := uuid.New(), uuid.New()
	danglingStory, danglingRole := uuid.New(), uuid.New()

	tasks := []domain.GanttTask{
		{ID: bothValidID, RoleID: &roleID, IsParent: false},
		{ID: invalidRefID, RoleID: &roleID, IsParent: false},
		{ID: noneID, RoleID: &roleID, IsParent: false},
	}
	notBefore := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	assignments := map[uuid.UUID]domain.TaskAssignment{
		bothValidID: {
			RoleID: roleID, NotBeforeDate: &notBefore,
			WaitForStoryID: &waitForStory, WaitForRoleID: &waitForRole,
		},
		invalidRefID: {
			RoleID:         roleID,
			WaitForStoryID: &danglingStory, WaitForRoleID: &danglingRole,
		},
	}
	// Только waitForStory/waitForRole (цель ссылки bothValidID) сейчас
	// существуют — danglingStory/danglingRole отсутствуют (исчезнувшая
	// задача).
	validRefs := map[gantt.TaskRef]bool{
		{StoryID: waitForStory, RoleID: waitForRole}: true,
	}

	repo := &mockGanttRepo{}
	svc := &mockGanttSvc{
		getTeamTasksWithAssignmentsFunc: func(ctx context.Context, tID uuid.UUID) ([]domain.GanttTask, map[uuid.UUID]domain.TaskAssignment, map[gantt.TaskRef]bool, error) {
			return tasks, assignments, validRefs, nil
		},
	}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	req := httptest.NewRequest(http.MethodGet, "/api/gantt/tasks?team_id="+teamID.String(), nil)
	w := httptest.NewRecorder()

	handler.GetTasks(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Tasks []taskRespDecode `json:"tasks"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	byID := make(map[string]taskRespDecode, len(resp.Tasks))
	for _, item := range resp.Tasks {
		byID[item.ID] = item
	}

	bothValid := byID[bothValidID.String()]
	if bothValid.NotBeforeDate == nil || *bothValid.NotBeforeDate != "2026-08-10" {
		t.Errorf("bothValid not_before_date = %v, want 2026-08-10", bothValid.NotBeforeDate)
	}
	if bothValid.WaitForStoryID == nil || *bothValid.WaitForStoryID != waitForStory.String() {
		t.Errorf("bothValid wait_for_story_id = %v, want %v", bothValid.WaitForStoryID, waitForStory)
	}
	if bothValid.WaitForRoleID == nil || *bothValid.WaitForRoleID != waitForRole.String() {
		t.Errorf("bothValid wait_for_role_id = %v, want %v", bothValid.WaitForRoleID, waitForRole)
	}
	if bothValid.StartConstraintRefInvalid {
		t.Errorf("bothValid start_constraint_ref_invalid = true, want false (target exists)")
	}

	invalidRef := byID[invalidRefID.String()]
	if invalidRef.NotBeforeDate != nil {
		t.Errorf("invalidRef not_before_date = %v, want nil (date was never set)", invalidRef.NotBeforeDate)
	}
	if invalidRef.WaitForStoryID == nil || *invalidRef.WaitForStoryID != danglingStory.String() {
		t.Errorf("invalidRef wait_for_story_id = %v, want %v (ref kept even though invalid)", invalidRef.WaitForStoryID, danglingStory)
	}
	if !invalidRef.StartConstraintRefInvalid {
		t.Errorf("invalidRef start_constraint_ref_invalid = false, want true (target no longer exists)")
	}

	none := byID[noneID.String()]
	if none.NotBeforeDate != nil || none.WaitForStoryID != nil || none.WaitForRoleID != nil {
		t.Errorf("none task should have no start-constraint fields, got %+v", none)
	}
	if none.StartConstraintRefInvalid {
		t.Errorf("none task start_constraint_ref_invalid = true, want false (no reference at all)")
	}
}
