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
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// newAssigneeRequest builds a PUT/GET request with taskID/teamID injected as
// a chi URL param (matching how the router extracts "id") and, unless
// session is nil, a UserSession in the request context (matching how
// middleware.TelegramAuth/RoleAuth would have populated it upstream — these
// handler-level tests call the handler function directly, bypassing the
// router's middleware chain, following this package's established
// convention, see e.g. admin_team_scope_test.go).
func newAssigneeRequest(method, path, param, paramValue, body string, session *middleware.UserSession) *http.Request {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(param, paramValue)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	if session != nil {
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserSessionKey, session))
	}
	return req
}

type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// decodeError сообщает, что тело ответа — стандартный формат проекта
// {"error":{"code":"...","message":"..."}}, и возвращает код/сообщение.
func decodeError(t *testing.T, body []byte) errorEnvelope {
	t.Helper()
	var e errorEnvelope
	if err := json.Unmarshal(body, &e); err != nil {
		t.Fatalf("response is not the standard {error:{code,message}} envelope: %v; body=%s", err, body)
	}
	if e.Error.Code == "" {
		t.Fatalf("expected non-empty error.code, body=%s", body)
	}
	if e.Error.Message == "" {
		t.Fatalf("expected non-empty error.message, body=%s", body)
	}
	return e
}

// --- PUT /api/gantt/tasks/{id}/assignee (backend §3.2) ---

func TestSetTaskAssignee_Unauthorized(t *testing.T) {
	repo := &mockGanttRepo{}
	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	taskID := uuid.New()
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/tasks/"+taskID.String()+"/assignee", "id", taskID.String(), `{"user_id":null}`, nil)
	w := httptest.NewRecorder()

	handler.SetTaskAssignee(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d, body: %s", w.Code, w.Body.String())
	}
	decodeError(t, w.Body.Bytes())
}

func TestSetTaskAssignee_RejectsMember(t *testing.T) {
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
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/tasks/"+taskID.String()+"/assignee", "id", taskID.String(), `{"user_id":null}`, session)
	w := httptest.NewRecorder()

	handler.SetTaskAssignee(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for member role, got %d, body: %s", w.Code, w.Body.String())
	}
	decodeError(t, w.Body.Bytes())
	if svc.setTaskAssigneeCalled {
		t.Errorf("service must not be called when the request is rejected for role")
	}
}

func TestSetTaskAssignee_PinsCandidateSuccessfully(t *testing.T) {
	teamID := uuid.New()
	roleID := uuid.New()
	epicID := uuid.New()
	taskID := uuid.New()
	candidateID := uuid.New()

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
		getUsersByTeamIDAndRoleFunc: func(ctx context.Context, tID, rID uuid.UUID) ([]domain.User, error) {
			return []domain.User{{ID: candidateID}}, nil
		},
	}
	svc := &mockGanttSvc{
		setTaskAssigneeFunc: func(ctx context.Context, tID uuid.UUID, userID *uuid.UUID) ([]domain.GanttTask, error) {
			return []domain.GanttTask{{ID: taskID}}, nil
		},
	}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	session := &middleware.UserSession{TelegramID: "admin-1", Username: "team_admin"}
	body := `{"user_id":"` + candidateID.String() + `"}`
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/tasks/"+taskID.String()+"/assignee", "id", taskID.String(), body, session)
	w := httptest.NewRecorder()

	handler.SetTaskAssignee(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}
	if !svc.setTaskAssigneeCalled {
		t.Fatalf("expected service.SetTaskAssignee to be called")
	}
	if svc.setTaskAssigneeArgs.taskID != taskID {
		t.Errorf("taskID = %v, want %v", svc.setTaskAssigneeArgs.taskID, taskID)
	}
	if svc.setTaskAssigneeArgs.userID == nil || *svc.setTaskAssigneeArgs.userID != candidateID {
		t.Errorf("userID = %v, want %v", svc.setTaskAssigneeArgs.userID, candidateID)
	}
}

func TestSetTaskAssignee_NullUserIDUnpinsAutomatically(t *testing.T) {
	teamID := uuid.New()
	epicID := uuid.New()
	roleID := uuid.New()
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
		setTaskAssigneeFunc: func(ctx context.Context, tID uuid.UUID, userID *uuid.UUID) ([]domain.GanttTask, error) {
			return nil, nil
		},
	}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	session := &middleware.UserSession{TelegramID: "admin-1", Username: "team_admin"}
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/tasks/"+taskID.String()+"/assignee", "id", taskID.String(), `{"user_id":null}`, session)
	w := httptest.NewRecorder()

	handler.SetTaskAssignee(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}
	if !svc.setTaskAssigneeCalled {
		t.Fatalf("expected service.SetTaskAssignee to be called")
	}
	if svc.setTaskAssigneeArgs.userID != nil {
		t.Errorf("expected nil userID (return to automatic), got %v", svc.setTaskAssigneeArgs.userID)
	}
	// GetUsersByTeamIDAndRoleID/GetEpicByID candidate validation must not
	// even be attempted when unpinning — there's no candidate to validate.
}

func TestSetTaskAssignee_RejectsForeignCandidate(t *testing.T) {
	teamID := uuid.New()
	roleID := uuid.New()
	epicID := uuid.New()
	taskID := uuid.New()
	outsiderID := uuid.New() // not in the role's pool

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
		getUsersByTeamIDAndRoleFunc: func(ctx context.Context, tID, rID uuid.UUID) ([]domain.User, error) {
			return []domain.User{{ID: uuid.New()}}, nil // outsiderID deliberately absent
		},
	}
	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	session := &middleware.UserSession{TelegramID: "admin-1", Username: "team_admin"}
	body := `{"user_id":"` + outsiderID.String() + `"}`
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/tasks/"+taskID.String()+"/assignee", "id", taskID.String(), body, session)
	w := httptest.NewRecorder()

	handler.SetTaskAssignee(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body: %s", w.Code, w.Body.String())
	}
	e := decodeError(t, w.Body.Bytes())
	if e.Error.Code != "USER_NOT_ROLE_CANDIDATE" {
		t.Errorf("expected error code USER_NOT_ROLE_CANDIDATE, got %q", e.Error.Code)
	}
	if svc.setTaskAssigneeCalled {
		t.Errorf("service must not be called when the candidate is rejected")
	}
}

// --- backend §3.6: team-scoped проверка admin-а ---

// TestSetTaskAssignee_RejectsCrossTeamAdmin проверяет, что admin команды A
// (isTeamAdminOfAny=true — грубый гейт RoleAuth("admin") этот случай уже
// пропустил бы) получает 403, если задача принадлежит команде B, где он не
// team-admin — та же точечная проверка, что уже применяют admin.go
// (UpdateEpic) и admin_scores.go (AdminSubmitEpicScore).
func TestSetTaskAssignee_RejectsCrossTeamAdmin(t *testing.T) {
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
		// admin команды A team-admin ТОЛЬКО ownTeam, не foreignTeam.
		isTeamAdminOfFunc: func(ctx context.Context, telegramID string, tID uuid.UUID) (bool, error) {
			return tID == ownTeam, nil
		},
	}
	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	session := &middleware.UserSession{TelegramID: "admin-a", Username: "team_admin_a"}
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/tasks/"+taskID.String()+"/assignee", "id", taskID.String(), `{"user_id":null}`, session)
	w := httptest.NewRecorder()

	handler.SetTaskAssignee(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-team admin, got %d, body: %s", w.Code, w.Body.String())
	}
	e := decodeError(t, w.Body.Bytes())
	if e.Error.Code != "FORBIDDEN" {
		t.Errorf("expected error code FORBIDDEN, got %q", e.Error.Code)
	}
	if svc.setTaskAssigneeCalled {
		t.Errorf("service must not be called when the admin is out of scope for the task's team")
	}
}

// TestSetTaskAssignee_AllowsSameTeamAdmin проверяет положительный случай:
// admin, являющийся team-admin именно той команды, которой принадлежит
// задача, проходит проверку.
func TestSetTaskAssignee_AllowsSameTeamAdmin(t *testing.T) {
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
	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	session := &middleware.UserSession{TelegramID: "admin-1", Username: "team_admin"}
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/tasks/"+taskID.String()+"/assignee", "id", taskID.String(), `{"user_id":null}`, session)
	w := httptest.NewRecorder()

	handler.SetTaskAssignee(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for same-team admin, got %d, body: %s", w.Code, w.Body.String())
	}
	if !svc.setTaskAssigneeCalled {
		t.Errorf("expected service to be called for a same-team admin")
	}
}

// TestSetTaskAssignee_SuperAdminBypassesTeamScope проверяет, что superadmin
// проходит независимо от IsTeamAdminOf (не team-admin ни одной команды) —
// та же семантика, что и в admin.go/admin_scores.go: isSuper пропускает
// точечную team-scoped проверку целиком.
func TestSetTaskAssignee_SuperAdminBypassesTeamScope(t *testing.T) {
	teamID := uuid.New()
	roleID := uuid.New()
	epicID := uuid.New()
	taskID := uuid.New()

	repo := &mockGanttRepo{
		// Ни team-admin какой-либо команды, ни team-admin конкретно этой —
		// superadmin должен пройти невзирая на это.
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
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/tasks/"+taskID.String()+"/assignee", "id", taskID.String(), `{"user_id":null}`, session)
	w := httptest.NewRecorder()

	handler.SetTaskAssignee(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for superadmin, got %d, body: %s", w.Code, w.Body.String())
	}
	if !svc.setTaskAssigneeCalled {
		t.Errorf("expected service to be called for superadmin")
	}
}

func TestSetTaskAssignee_RejectsParentTask(t *testing.T) {
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
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/tasks/"+taskID.String()+"/assignee", "id", taskID.String(), `{"user_id":null}`, session)
	w := httptest.NewRecorder()

	handler.SetTaskAssignee(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body: %s", w.Code, w.Body.String())
	}
	e := decodeError(t, w.Body.Bytes())
	if e.Error.Code != "ASSIGNEE_NOT_ALLOWED_ON_PARENT" {
		t.Errorf("expected error code ASSIGNEE_NOT_ALLOWED_ON_PARENT, got %q", e.Error.Code)
	}
}

func TestSetTaskAssignee_TaskNotFound(t *testing.T) {
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
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/tasks/"+taskID.String()+"/assignee", "id", taskID.String(), `{"user_id":null}`, session)
	w := httptest.NewRecorder()

	handler.SetTaskAssignee(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body: %s", w.Code, w.Body.String())
	}
	e := decodeError(t, w.Body.Bytes())
	if e.Error.Code != "TASK_NOT_FOUND" {
		t.Errorf("expected error code TASK_NOT_FOUND, got %q", e.Error.Code)
	}
}

func TestSetTaskAssignee_InvalidUserID(t *testing.T) {
	teamID := uuid.New()
	epicID := uuid.New()
	roleID := uuid.New()
	taskID := uuid.New()
	repo := &mockGanttRepo{
		isTeamAdminOfAny: true,
		getTaskFunc: func(ctx context.Context, id uuid.UUID) (*domain.GanttTask, error) {
			return &domain.GanttTask{ID: taskID, EpicID: epicID, RoleID: &roleID}, nil
		},
		getEpicByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Epic, error) {
			return &domain.Epic{ID: epicID, TeamID: teamID}, nil
		},
		isTeamAdminOfFunc: func(ctx context.Context, telegramID string, tID uuid.UUID) (bool, error) {
			return tID == teamID, nil
		},
	}
	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	session := &middleware.UserSession{TelegramID: "admin-1", Username: "team_admin"}
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/tasks/"+taskID.String()+"/assignee", "id", taskID.String(), `{"user_id":"not-a-uuid"}`, session)
	w := httptest.NewRecorder()

	handler.SetTaskAssignee(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body: %s", w.Code, w.Body.String())
	}
	e := decodeError(t, w.Body.Bytes())
	if e.Error.Code != "INVALID_USER_ID" {
		t.Errorf("expected error code INVALID_USER_ID, got %q", e.Error.Code)
	}
}

// --- GET /api/gantt/teams/{id}/members (backend §3.3) ---

func TestGetTeamMembers_ReturnsAllRoles(t *testing.T) {
	teamID := uuid.New()
	memberID := uuid.New()
	role1, role2 := uuid.New(), uuid.New()

	repo := &mockGanttRepo{
		getTeamByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
			return &domain.Team{ID: teamID}, nil
		},
	}
	svc := &mockGanttSvc{
		getTeamMembersFunc: func(ctx context.Context, tID uuid.UUID) ([]domain.TeamMember, error) {
			return []domain.TeamMember{
				{ID: memberID, FirstName: "Ivan", LastName: "Petrov", RoleIDs: []uuid.UUID{role1, role2}},
			}, nil
		},
	}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	req := newAssigneeRequest(http.MethodGet, "/api/gantt/teams/"+teamID.String()+"/members", "id", teamID.String(), "", nil)
	w := httptest.NewRecorder()

	handler.GetTeamMembers(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Members []struct {
			ID        string   `json:"id"`
			FirstName string   `json:"first_name"`
			LastName  string   `json:"last_name"`
			RoleIDs   []string `json:"role_ids"`
		} `json:"members"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(resp.Members))
	}
	m := resp.Members[0]
	if m.ID != memberID.String() || m.FirstName != "Ivan" || m.LastName != "Petrov" {
		t.Errorf("unexpected member: %+v", m)
	}
	if len(m.RoleIDs) != 2 {
		t.Fatalf("expected 2 role_ids, got %v", m.RoleIDs)
	}
	roleSet := map[string]bool{m.RoleIDs[0]: true, m.RoleIDs[1]: true}
	if !roleSet[role1.String()] || !roleSet[role2.String()] {
		t.Errorf("expected both roles present, got %v", m.RoleIDs)
	}
}

func TestGetTeamMembers_UnknownTeam(t *testing.T) {
	teamID := uuid.New()
	repo := &mockGanttRepo{
		getTeamByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
			return nil, errors.New("not found")
		},
	}
	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	req := newAssigneeRequest(http.MethodGet, "/api/gantt/teams/"+teamID.String()+"/members", "id", teamID.String(), "", nil)
	w := httptest.NewRecorder()

	handler.GetTeamMembers(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body: %s", w.Code, w.Body.String())
	}
	e := decodeError(t, w.Body.Bytes())
	if e.Error.Code != "TEAM_NOT_FOUND" {
		t.Errorf("expected error code TEAM_NOT_FOUND, got %q", e.Error.Code)
	}
}

// --- GET /api/gantt/tasks — assignee_* fields and start_offset_days
// (backend §3.1, §3.5) ---

type taskRespDecode struct {
	ID                 string  `json:"id"`
	IsParent           bool    `json:"is_parent"`
	RoleID             string  `json:"role_id"`
	StartOffsetDays    int     `json:"start_offset_days"`
	AssigneeID         *string `json:"assignee_id"`
	AssigneeName       *string `json:"assignee_name"`
	AssigneeIsManual   bool    `json:"assignee_is_manual"`
	AssigneePinInvalid bool    `json:"assignee_pin_invalid"`
	// Поля ограничения "Начать не ранее" (backend §3.1, add-task-start-constraints).
	NotBeforeDate             *string `json:"not_before_date"`
	WaitForStoryID            *string `json:"wait_for_story_id"`
	WaitForRoleID             *string `json:"wait_for_role_id"`
	StartConstraintRefInvalid bool    `json:"start_constraint_ref_invalid"`
}

// TestGetTasks_AssigneeFieldsAndStartOffset — комплексный тест на четыре
// состояния исполнителя ролевой задачи одновременно (design.md, спека
// gantt-task-assignees): действующее закрепление, автоматический выбор,
// недействующее закрепление и отсутствие исполнителя (пустой пул роли).
// Также проверяет §3.5: start_offset_days в ответе берётся из
// task_assignments, а не остаётся нулём.
func TestGetTasks_AssigneeFieldsAndStartOffset(t *testing.T) {
	teamID := uuid.New()
	roleID := uuid.New()

	manualTaskID, autoTaskID, invalidPinTaskID, noAssigneeTaskID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	pinnedUser := uuid.New()   // действующее закрепление — сам и есть фактический исполнитель
	autoUser := uuid.New()     // автоматически назначенный, без закрепления
	fallbackUser := uuid.New() // фактический исполнитель после недействующего закрепления
	staleUser := uuid.New()    // закреплён, но больше не в команде (недействует)

	tasks := []domain.GanttTask{
		{ID: manualTaskID, RoleID: &roleID, IsParent: false, AssigneeID: &pinnedUser},
		{ID: autoTaskID, RoleID: &roleID, IsParent: false, AssigneeID: &autoUser},
		{ID: invalidPinTaskID, RoleID: &roleID, IsParent: false, AssigneeID: &fallbackUser},
		{ID: noAssigneeTaskID, RoleID: &roleID, IsParent: false, AssigneeID: nil},
	}
	assignments := map[uuid.UUID]domain.TaskAssignment{
		manualTaskID:     {RoleID: roleID, UserID: &pinnedUser, StartOffsetDays: 3},
		invalidPinTaskID: {RoleID: roleID, UserID: &staleUser, StartOffsetDays: 0},
	}

	repo := &mockGanttRepo{
		getUsersByTeamIDFunc: func(ctx context.Context, tID uuid.UUID) ([]domain.User, error) {
			// staleUser намеренно отсутствует — выбыл из команды
			// (design.md Решение 6/7), поэтому его имя резолвится точечным
			// фоллбэком через GetUserByID.
			return []domain.User{
				{ID: pinnedUser, FirstName: "Pinned", LastName: "Person"},
				{ID: autoUser, FirstName: "Auto", LastName: "Person"},
				{ID: fallbackUser, FirstName: "Fallback", LastName: "Person"},
			}, nil
		},
		getUserByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
			if id == staleUser {
				return &domain.User{ID: staleUser, FirstName: "Stale", LastName: "Person"}, nil
			}
			return nil, errors.New("not found")
		},
	}
	svc := &mockGanttSvc{
		getTeamTasksWithAssignmentsFunc: func(ctx context.Context, tID uuid.UUID) ([]domain.GanttTask, map[uuid.UUID]domain.TaskAssignment, map[gantt.TaskRef]bool, error) {
			return tasks, assignments, nil, nil
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

	manual := byID[manualTaskID.String()]
	if manual.AssigneeID == nil || *manual.AssigneeID != pinnedUser.String() {
		t.Errorf("manual task assignee_id = %v, want %v", manual.AssigneeID, pinnedUser)
	}
	if manual.AssigneeName == nil || *manual.AssigneeName != "Pinned Person" {
		t.Errorf("manual task assignee_name = %v, want %q", manual.AssigneeName, "Pinned Person")
	}
	if !manual.AssigneeIsManual {
		t.Errorf("manual task assignee_is_manual = false, want true")
	}
	if manual.AssigneePinInvalid {
		t.Errorf("manual task assignee_pin_invalid = true, want false")
	}
	if manual.StartOffsetDays != 3 {
		t.Errorf("manual task start_offset_days = %d, want 3 (from task_assignments)", manual.StartOffsetDays)
	}

	auto := byID[autoTaskID.String()]
	if auto.AssigneeID == nil || *auto.AssigneeID != autoUser.String() {
		t.Errorf("auto task assignee_id = %v, want %v", auto.AssigneeID, autoUser)
	}
	if auto.AssigneeIsManual {
		t.Errorf("auto task assignee_is_manual = true, want false (no pin at all)")
	}
	if auto.AssigneePinInvalid {
		t.Errorf("auto task assignee_pin_invalid = true, want false (no pin at all)")
	}
	if auto.StartOffsetDays != 0 {
		t.Errorf("auto task start_offset_days = %d, want 0", auto.StartOffsetDays)
	}

	invalidPin := byID[invalidPinTaskID.String()]
	if invalidPin.AssigneeID == nil || *invalidPin.AssigneeID != fallbackUser.String() {
		t.Errorf("invalid-pin task assignee_id = %v, want fallback %v", invalidPin.AssigneeID, fallbackUser)
	}
	if invalidPin.AssigneeName == nil || *invalidPin.AssigneeName != "Fallback Person" {
		t.Errorf("invalid-pin task assignee_name = %v, want %q", invalidPin.AssigneeName, "Fallback Person")
	}
	if invalidPin.AssigneeIsManual {
		t.Errorf("invalid-pin task assignee_is_manual = true, want false (pin no longer effective)")
	}
	if !invalidPin.AssigneePinInvalid {
		t.Errorf("invalid-pin task assignee_pin_invalid = false, want true")
	}

	noAssignee := byID[noAssigneeTaskID.String()]
	if noAssignee.AssigneeID != nil {
		t.Errorf("no-assignee task assignee_id = %v, want nil (empty role pool)", noAssignee.AssigneeID)
	}
	if noAssignee.AssigneeName != nil {
		t.Errorf("no-assignee task assignee_name = %v, want nil", noAssignee.AssigneeName)
	}
	if noAssignee.AssigneeIsManual || noAssignee.AssigneePinInvalid {
		t.Errorf("no-assignee task manual/pin_invalid should both be false, got manual=%v pin_invalid=%v",
			noAssignee.AssigneeIsManual, noAssignee.AssigneePinInvalid)
	}
	if noAssignee.StartOffsetDays != 0 {
		t.Errorf("no-assignee task start_offset_days = %d, want 0", noAssignee.StartOffsetDays)
	}
}
