package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"EpicScoreBot/internal/config"
	"EpicScoreBot/internal/models/domain"
	"EpicScoreBot/internal/transport/httpServer/middleware"
	"log/slog"

	"github.com/google/uuid"
)

func superadminCtx(req *http.Request, telegramID, username string) *http.Request {
	session := &middleware.UserSession{TelegramID: telegramID, Username: username}
	return req.WithContext(context.WithValue(req.Context(), middleware.UserSessionKey, session))
}

func nonSuperadminCtx(req *http.Request, telegramID, username string) *http.Request {
	session := &middleware.UserSession{TelegramID: telegramID, Username: username}
	return req.WithContext(context.WithValue(req.Context(), middleware.UserSessionKey, session))
}

func TestGetTeamAdmins(t *testing.T) {
	teamID := uuid.New()
	cfg := config.BotConfig{SuperAdmins: []string{"root"}}

	t.Run("happy_path", func(t *testing.T) {
		repo := &mockRepository{
			teamAdmins: []domain.User{
				{ID: uuid.New(), TelegramID: "111", FirstName: "Иван", LastName: "Иванов"},
			},
		}
		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})

		req := httptest.NewRequest("GET", "/api/gantt/admin/team-admins?team_id="+teamID.String(), nil)
		req = superadminCtx(req, "1", "root")
		rr := httptest.NewRecorder()

		handler.GetTeamAdmins(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var resp struct {
			TeamID string `json:"team_id"`
			Admins []struct {
				ID         string `json:"id"`
				TelegramID string `json:"telegram_id"`
			} `json:"admins"`
		}
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.TeamID != teamID.String() {
			t.Errorf("expected team_id %s, got %s", teamID.String(), resp.TeamID)
		}
		if len(resp.Admins) != 1 || resp.Admins[0].TelegramID != "111" {
			t.Errorf("unexpected admins list: %+v", resp.Admins)
		}
	})

	t.Run("invalid_team_id", func(t *testing.T) {
		repo := &mockRepository{}
		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})

		req := httptest.NewRequest("GET", "/api/gantt/admin/team-admins?team_id=not-a-uuid", nil)
		req = superadminCtx(req, "1", "root")
		rr := httptest.NewRecorder()

		handler.GetTeamAdmins(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rr.Code)
		}
	})
}

func TestAssignTeamAdmin(t *testing.T) {
	userID := uuid.New()
	teamID := uuid.New()
	cfg := config.BotConfig{SuperAdmins: []string{"root"}}

	t.Run("happy_path", func(t *testing.T) {
		superadminUser := &domain.User{ID: uuid.New(), TelegramID: "1"}
		repo := &mockRepository{
			users: map[string]*domain.User{
				"target": {ID: userID, TelegramID: "target"},
				"1":      superadminUser,
			},
			team: &domain.Team{ID: teamID, Name: "Team A"},
		}
		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})

		body := `{"user_id":"` + userID.String() + `","team_id":"` + teamID.String() + `"}`
		req := httptest.NewRequest("POST", "/api/gantt/admin/team-admins", strings.NewReader(body))
		req = superadminCtx(req, "1", "root")
		rr := httptest.NewRecorder()

		handler.AssignTeamAdmin(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		if !repo.assignTeamAdminCalled {
			t.Fatal("expected AssignTeamAdmin to be called")
		}
		if repo.assignTeamAdminArgs.userID != userID || repo.assignTeamAdminArgs.teamID != teamID {
			t.Errorf("unexpected assign args: %+v", repo.assignTeamAdminArgs)
		}
		if repo.assignTeamAdminArgs.assignedBy != superadminUser.ID {
			t.Errorf("expected assigned_by %v, got %v", superadminUser.ID, repo.assignTeamAdminArgs.assignedBy)
		}
	})

	t.Run("user_not_found", func(t *testing.T) {
		repo := &mockRepository{team: &domain.Team{ID: teamID}}
		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})

		body := `{"user_id":"` + userID.String() + `","team_id":"` + teamID.String() + `"}`
		req := httptest.NewRequest("POST", "/api/gantt/admin/team-admins", strings.NewReader(body))
		req = superadminCtx(req, "1", "root")
		rr := httptest.NewRecorder()

		handler.AssignTeamAdmin(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", rr.Code)
		}
		if repo.assignTeamAdminCalled {
			t.Error("expected AssignTeamAdmin not to be called")
		}
	})

	t.Run("invalid_ids", func(t *testing.T) {
		repo := &mockRepository{}
		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})

		body := `{"user_id":"not-a-uuid","team_id":"` + teamID.String() + `"}`
		req := httptest.NewRequest("POST", "/api/gantt/admin/team-admins", strings.NewReader(body))
		req = superadminCtx(req, "1", "root")
		rr := httptest.NewRecorder()

		handler.AssignTeamAdmin(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rr.Code)
		}
	})
}

func TestRemoveTeamAdmin(t *testing.T) {
	userID := uuid.New()
	teamID := uuid.New()
	cfg := config.BotConfig{SuperAdmins: []string{"root"}}

	t.Run("happy_path", func(t *testing.T) {
		repo := &mockRepository{}
		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})

		body := `{"user_id":"` + userID.String() + `","team_id":"` + teamID.String() + `"}`
		req := httptest.NewRequest("DELETE", "/api/gantt/admin/team-admins", strings.NewReader(body))
		req = superadminCtx(req, "1", "root")
		rr := httptest.NewRecorder()

		handler.RemoveTeamAdmin(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		if !repo.removeTeamAdminCalled {
			t.Fatal("expected RemoveTeamAdmin to be called")
		}
		if repo.removeTeamAdminArgs.userID != userID || repo.removeTeamAdminArgs.teamID != teamID {
			t.Errorf("unexpected remove args: %+v", repo.removeTeamAdminArgs)
		}
	})

	t.Run("invalid_ids", func(t *testing.T) {
		repo := &mockRepository{}
		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})

		body := `{"user_id":"not-a-uuid","team_id":"` + teamID.String() + `"}`
		req := httptest.NewRequest("DELETE", "/api/gantt/admin/team-admins", strings.NewReader(body))
		req = superadminCtx(req, "1", "root")
		rr := httptest.NewRecorder()

		handler.RemoveTeamAdmin(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rr.Code)
		}
		if repo.removeTeamAdminCalled {
			t.Error("expected RemoveTeamAdmin not to be called")
		}
	})
}

// TestTeamAdminRoutes_RequireSuperAdmin — контроль, что RoleAuth("superadmin")
// (реальная реализация middleware, не мок) действительно отклоняет
// не-superadmin сессии для управления team_admins — эквивалент 403 для
// эндпоинтов GET/POST/DELETE /api/gantt/admin/team-admins, замыкаемых на
// RoleAuth("superadmin") в routers.go.
func TestTeamAdminRoutes_RequireSuperAdmin(t *testing.T) {
	cfg := config.BotConfig{SuperAdmins: []string{"root"}}
	finder := &mockRepository{}
	teamAdminChecker := &mockRepository{teamAdminOfAny: true} // даже team-admin не должен пройти

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := middleware.RoleAuth(finder, teamAdminChecker, cfg, "superadmin")(next)

	req := httptest.NewRequest("POST", "/api/gantt/admin/team-admins", nil)
	req = nonSuperadminCtx(req, "2", "team_admin_user")
	rr := httptest.NewRecorder()

	mw.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-superadmin, got %d", rr.Code)
	}
}
