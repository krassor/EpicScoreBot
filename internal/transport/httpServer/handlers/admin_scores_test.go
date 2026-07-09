package handlers

import (
	"context"
	"encoding/json"
	"fmt"
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

type mockAdminScoresRepo struct {
	Repository
	user                 *domain.User
	role                 *domain.Role
	epic                 *domain.Epic
	risk                 *domain.Risk
	createdEpicScore     bool
	createdRiskScore     bool
	completedEpicScoring bool
	completedRiskScoring bool
}

func (m *mockAdminScoresRepo) GetUserByID(ctx context.Context, userID uuid.UUID) (*domain.User, error) {
	if m.user != nil && m.user.ID == userID {
		return m.user, nil
	}
	return nil, nil
}

func (m *mockAdminScoresRepo) GetRoleByUserID(ctx context.Context, userID uuid.UUID) (*domain.Role, error) {
	if m.role != nil {
		return m.role, nil
	}
	return nil, nil
}

func (m *mockAdminScoresRepo) CreateEpicScore(ctx context.Context, epicID, userID, roleID uuid.UUID, score int) error {
	m.createdEpicScore = true
	return nil
}

func (m *mockAdminScoresRepo) CreateRiskScore(ctx context.Context, riskID, userID uuid.UUID, probability, impact int) error {
	m.createdRiskScore = true
	return nil
}

func (m *mockAdminScoresRepo) GetEpicByID(ctx context.Context, epicID uuid.UUID) (*domain.Epic, error) {
	return m.epic, nil
}

func (m *mockAdminScoresRepo) GetRiskByID(ctx context.Context, riskID uuid.UUID) (*domain.Risk, error) {
	return m.risk, nil
}

func (m *mockAdminScoresRepo) CountTeamMembers(ctx context.Context, teamID uuid.UUID) (int, error) {
	return 3, nil
}

func (m *mockAdminScoresRepo) CountEpicScores(ctx context.Context, epicID uuid.UUID) (int, error) {
	return 2, nil
}

func (m *mockAdminScoresRepo) CountRiskScores(ctx context.Context, riskID uuid.UUID) (int, error) {
	return 2, nil
}

type mockAdminScoresSvc struct {
	ScoringService
	repo *mockAdminScoresRepo
}

func (s *mockAdminScoresSvc) TryCompleteEpicScoring(ctx context.Context, epicID uuid.UUID) error {
	s.repo.completedEpicScoring = true
	return nil
}

func (s *mockAdminScoresSvc) TryCompleteRiskScoring(ctx context.Context, riskID uuid.UUID) error {
	s.repo.completedRiskScoring = true
	return nil
}

func TestAdminSubmitEpicScore(t *testing.T) {
	userID := uuid.New()
	roleID := uuid.New()
	epicID := uuid.New()

	cfg := config.BotConfig{
		Admins: []string{"admin_user"},
	}

	t.Run("success_as_admin", func(t *testing.T) {
		repo := &mockAdminScoresRepo{
			user: &domain.User{ID: userID, TelegramID: "user_tg"},
			role: &domain.Role{ID: roleID, Name: "BE разработчик"},
			epic: &domain.Epic{ID: epicID, Status: domain.StatusScoring, TeamID: uuid.New()},
		}
		svc := &mockAdminScoresSvc{repo: repo}
		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, svc, &mockAIClient{}, cfg)

		body := `{"epic_id":"` + epicID.String() + `","user_id":"` + userID.String() + `","score":8}`
		req := httptest.NewRequest("POST", "/api/gantt/admin/scores/epic", strings.NewReader(body))
		session := &middleware.UserSession{TelegramID: "999", Username: "admin_user"}
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserSessionKey, session))
		rr := httptest.NewRecorder()

		handler.AdminSubmitEpicScore(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		if !repo.createdEpicScore {
			t.Error("expected epic score to be created")
		}
		if !repo.completedEpicScoring {
			t.Error("expected TryCompleteEpicScoring to be called")
		}
	})

	t.Run("forbidden_for_member", func(t *testing.T) {
		repo := &mockAdminScoresRepo{
			user: &domain.User{ID: userID, TelegramID: "user_tg"},
			role: &domain.Role{ID: roleID, Name: "BE разработчик"},
			epic: &domain.Epic{ID: epicID, Status: domain.StatusScoring, TeamID: uuid.New()},
		}
		svc := &mockAdminScoresSvc{repo: repo}
		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, svc, &mockAIClient{}, cfg)

		body := `{"epic_id":"` + epicID.String() + `","user_id":"` + userID.String() + `","score":8}`
		req := httptest.NewRequest("POST", "/api/gantt/admin/scores/epic", strings.NewReader(body))
		session := &middleware.UserSession{TelegramID: "111", Username: "regular_user"}
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserSessionKey, session))
		rr := httptest.NewRecorder()

		handler.AdminSubmitEpicScore(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden, got %d", rr.Code)
		}
		if repo.createdEpicScore {
			t.Error("expected epic score not to be created")
		}
	})

	t.Run("score_validation", func(t *testing.T) {
		tests := []struct {
			name         string
			score        int
			expectedCode int
		}{
			{"zero_is_valid", 0, http.StatusOK},
			{"five_hundred_is_valid", 500, http.StatusOK},
			{"negative_is_invalid", -1, http.StatusBadRequest},
			{"over_five_hundred_is_invalid", 501, http.StatusBadRequest},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				repo := &mockAdminScoresRepo{
					user: &domain.User{ID: userID, TelegramID: "user_tg"},
					role: &domain.Role{ID: roleID, Name: "BE разработчик"},
					epic: &domain.Epic{ID: epicID, Status: domain.StatusScoring, TeamID: uuid.New()},
				}
				svc := &mockAdminScoresSvc{repo: repo}
				handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, svc, &mockAIClient{}, cfg)

				body := fmt.Sprintf(`{"epic_id":"%s","user_id":"%s","score":%d}`, epicID.String(), userID.String(), tc.score)
				req := httptest.NewRequest("POST", "/api/gantt/admin/scores/epic", strings.NewReader(body))
				session := &middleware.UserSession{TelegramID: "999", Username: "admin_user"}
				req = req.WithContext(context.WithValue(req.Context(), middleware.UserSessionKey, session))
				rr := httptest.NewRecorder()

				handler.AdminSubmitEpicScore(rr, req)
				if rr.Code != tc.expectedCode {
					t.Errorf("for score %d expected %d, got %d. Body: %s", tc.score, tc.expectedCode, rr.Code, rr.Body.String())
				}

				if tc.expectedCode == http.StatusBadRequest {
					var errResp struct {
						Error string `json:"error"`
					}
					if err := json.NewDecoder(rr.Body).Decode(&errResp); err != nil {
						t.Fatalf("failed to decode error response: %v", err)
					}
					if errResp.Error != "score must be between 0 and 500" {
						t.Errorf("expected error message 'score must be between 0 and 500', got '%s'", errResp.Error)
					}
				}
			})
		}
	})
}

func TestAdminSubmitRiskScore(t *testing.T) {
	userID := uuid.New()
	riskID := uuid.New()

	cfg := config.BotConfig{
		Admins: []string{"admin_user"},
	}

	t.Run("success_as_admin", func(t *testing.T) {
		repo := &mockAdminScoresRepo{
			user: &domain.User{ID: userID, TelegramID: "user_tg"},
			risk: &domain.Risk{ID: riskID, Status: domain.StatusScoring, EpicID: uuid.New()},
		}
		svc := &mockAdminScoresSvc{repo: repo}
		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, svc, &mockAIClient{}, cfg)

		body := `{"risk_id":"` + riskID.String() + `","user_id":"` + userID.String() + `","probability":3,"impact":2}`
		req := httptest.NewRequest("POST", "/api/gantt/admin/scores/risk", strings.NewReader(body))
		session := &middleware.UserSession{TelegramID: "999", Username: "admin_user"}
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserSessionKey, session))
		rr := httptest.NewRecorder()

		handler.AdminSubmitRiskScore(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		if !repo.createdRiskScore {
			t.Error("expected risk score to be created")
		}
		if !repo.completedRiskScoring {
			t.Error("expected TryCompleteRiskScoring to be called")
		}
	})

	t.Run("forbidden_for_member", func(t *testing.T) {
		repo := &mockAdminScoresRepo{
			user: &domain.User{ID: userID, TelegramID: "user_tg"},
			risk: &domain.Risk{ID: riskID, Status: domain.StatusScoring, EpicID: uuid.New()},
		}
		svc := &mockAdminScoresSvc{repo: repo}
		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, svc, &mockAIClient{}, cfg)

		body := `{"risk_id":"` + riskID.String() + `","user_id":"` + userID.String() + `","probability":3,"impact":2}`
		req := httptest.NewRequest("POST", "/api/gantt/admin/scores/risk", strings.NewReader(body))
		session := &middleware.UserSession{TelegramID: "111", Username: "regular_user"}
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserSessionKey, session))
		rr := httptest.NewRecorder()

		handler.AdminSubmitRiskScore(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden, got %d", rr.Code)
		}
		if repo.createdRiskScore {
			t.Error("expected risk score not to be created")
		}
	})

	t.Run("bad_request_invalid_values", func(t *testing.T) {
		repo := &mockAdminScoresRepo{
			user: &domain.User{ID: userID, TelegramID: "user_tg"},
			risk: &domain.Risk{ID: riskID, Status: domain.StatusScoring, EpicID: uuid.New()},
		}
		svc := &mockAdminScoresSvc{repo: repo}
		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, svc, &mockAIClient{}, cfg)

		body := `{"risk_id":"` + riskID.String() + `","user_id":"` + userID.String() + `","probability":5,"impact":2}`
		req := httptest.NewRequest("POST", "/api/gantt/admin/scores/risk", strings.NewReader(body))
		session := &middleware.UserSession{TelegramID: "999", Username: "admin_user"}
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserSessionKey, session))
		rr := httptest.NewRecorder()

		handler.AdminSubmitRiskScore(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request, got %d", rr.Code)
		}
	})
}
