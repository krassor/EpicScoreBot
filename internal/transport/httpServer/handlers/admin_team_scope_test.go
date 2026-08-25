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

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// TestGetUsersList_TeamScoped проверяет, что team-admin команды A видит в
// списке только пользователей своей команды, а superadmin — всех.
func TestGetUsersList_TeamScoped(t *testing.T) {
	teamA := uuid.New()
	teamB := uuid.New()
	userInA := domain.User{ID: uuid.New(), TelegramID: "user_a", FirstName: "A"}
	userInB := domain.User{ID: uuid.New(), TelegramID: "user_b", FirstName: "B"}

	cfg := config.BotConfig{SuperAdmins: []string{"root"}}

	newRepo := func() *mockRepository {
		return &mockRepository{
			getAllUsersFunc: func(ctx context.Context) ([]domain.User, error) {
				return []domain.User{userInA, userInB}, nil
			},
			getUserTeamsFunc: func(ctx context.Context, userID uuid.UUID) ([]domain.Team, error) {
				if userID == userInA.ID {
					return []domain.Team{{ID: teamA}}, nil
				}
				return []domain.Team{{ID: teamB}}, nil
			},
		}
	}

	t.Run("team_admin_sees_only_own_team", func(t *testing.T) {
		repo := newRepo()
		repo.teamAdminOfAny = true
		repo.adminTeamIDs = []uuid.UUID{teamA}
		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, &mockScoringService{}, &mockAIClient{}, cfg)

		req := httptest.NewRequest("GET", "/api/gantt/admin/users", nil)
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserSessionKey,
			&middleware.UserSession{TelegramID: "1", Username: "team_admin_a"}))
		rr := httptest.NewRecorder()

		handler.GetUsersList(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var resp []struct {
			TelegramID string `json:"telegram_id"`
		}
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(resp) != 1 || resp[0].TelegramID != "user_a" {
			t.Errorf("expected only user_a in scope, got %+v", resp)
		}
	})

	t.Run("superadmin_sees_all", func(t *testing.T) {
		repo := newRepo()
		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, &mockScoringService{}, &mockAIClient{}, cfg)

		req := httptest.NewRequest("GET", "/api/gantt/admin/users", nil)
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserSessionKey,
			&middleware.UserSession{TelegramID: "2", Username: "root"}))
		rr := httptest.NewRecorder()

		handler.GetUsersList(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var resp []struct {
			TelegramID string `json:"telegram_id"`
		}
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(resp) != 2 {
			t.Errorf("expected both users for superadmin, got %+v", resp)
		}
	})
}

// TestCreateSingleUser_TeamScopeViolation проверяет, что team-admin не может
// назначить пользователя в команду, где он сам не является team-admin.
func TestCreateSingleUser_TeamScopeViolation(t *testing.T) {
	ownTeam := uuid.New()
	foreignTeam := uuid.New()
	cfg := config.BotConfig{SuperAdmins: []string{"root"}}

	repo := &mockRepository{
		users:          make(map[string]*domain.User),
		teamAdminOfAny: true,
		adminTeamIDs:   []uuid.UUID{ownTeam},
	}
	handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, &mockScoringService{}, &mockAIClient{}, cfg)

	body := `{"telegram_id":"newbie","first_name":"New","team_ids":["` + foreignTeam.String() + `"]}`
	req := httptest.NewRequest("POST", "/api/gantt/admin/users", strings.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserSessionKey,
		&middleware.UserSession{TelegramID: "1", Username: "team_admin_a"}))
	rr := httptest.NewRecorder()

	handler.CreateSingleUser(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 for foreign team_id, got %d. Body: %s", rr.Code, rr.Body.String())
	}
}

// TestUpdateEpic_CrossTeamForbidden проверяет, что team-admin команды A не
// может редактировать эпик команды B.
func TestUpdateEpic_CrossTeamForbidden(t *testing.T) {
	teamB := uuid.New()
	epicID := uuid.New()
	cfg := config.BotConfig{SuperAdmins: []string{"root"}}

	repo := &mockRepository{
		teamAdminOfAny: true,
		teamAdminOf:    false, // team-admin команды A не является team-admin команды B
	}

	// Мок GetEpicByID напрямую не настроен через func-поле в mockRepository —
	// оборачиваем его отдельным типом, реализующим нужный метод.
	epicRepo := &epicScopeRepo{mockRepository: repo, epic: &domain.Epic{ID: epicID, TeamID: teamB}}
	handler := NewGanttHandler(slog.Default(), &mockGanttService{}, epicRepo, &mockScoringService{}, &mockAIClient{}, cfg)

	body := `{"number":"EP-1","name":"Epic","team_id":"` + teamB.String() + `"}`
	req := httptest.NewRequest("PUT", "/api/gantt/epics/"+epicID.String(), strings.NewReader(body))
	chiCtx := chi.NewRouteContext()
	chiCtx.URLParams.Add("epic_id", epicID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiCtx))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserSessionKey,
		&middleware.UserSession{TelegramID: "1", Username: "team_admin_a"}))
	rr := httptest.NewRecorder()

	handler.UpdateEpic(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 for cross-team epic update, got %d. Body: %s", rr.Code, rr.Body.String())
	}
}

// epicScopeRepo расширяет mockRepository, возвращая фиксированный эпик из
// GetEpicByID — нужно для теста team-scope на UpdateEpic.
type epicScopeRepo struct {
	*mockRepository
	epic *domain.Epic
}

func (r *epicScopeRepo) GetEpicByID(ctx context.Context, epicID uuid.UUID) (*domain.Epic, error) {
	return r.epic, nil
}
