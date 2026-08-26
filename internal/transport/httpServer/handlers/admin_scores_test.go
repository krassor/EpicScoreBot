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
	"EpicScoreBot/internal/scoring"
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
	// denyTeamAdmin переключает мок team-admin проверок (IsTeamAdminOfAny/
	// IsTeamAdminOf) в состояние "не admin" — по умолчанию (false) сессия
	// считается team-admin, чтобы не трогать существующие "success"-тесты.
	denyTeamAdmin bool
	// allowedTeamID, если задан, делает IsTeamAdminOf team-specific: true
	// только для этой конкретной команды (для тестов "team-admin команды A
	// не может работать с командой B"). Если nil — используется denyTeamAdmin
	// без учёта конкретной команды.
	allowedTeamID *uuid.UUID
}

func (m *mockAdminScoresRepo) IsTeamAdminOfAny(ctx context.Context, telegramID string) (bool, error) {
	return !m.denyTeamAdmin, nil
}

func (m *mockAdminScoresRepo) IsTeamAdminOf(ctx context.Context, telegramID string, teamID uuid.UUID) (bool, error) {
	if m.allowedTeamID != nil {
		return *m.allowedTeamID == teamID, nil
	}
	return !m.denyTeamAdmin, nil
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

	manualFinalScoreEpic *domain.Epic
	manualFinalScoreErr  error
	manualFinalScoreCall struct {
		called     bool
		epicID     uuid.UUID
		finalScore float64
	}
}

func (s *mockAdminScoresSvc) TryCompleteEpicScoring(ctx context.Context, epicID uuid.UUID) error {
	s.repo.completedEpicScoring = true
	return nil
}

func (s *mockAdminScoresSvc) TryCompleteRiskScoring(ctx context.Context, riskID uuid.UUID) error {
	s.repo.completedRiskScoring = true
	return nil
}

func (s *mockAdminScoresSvc) SetManualFinalScore(ctx context.Context, epicID uuid.UUID, finalScore float64) (*domain.Epic, error) {
	s.manualFinalScoreCall.called = true
	s.manualFinalScoreCall.epicID = epicID
	s.manualFinalScoreCall.finalScore = finalScore

	if s.manualFinalScoreErr != nil {
		return nil, s.manualFinalScoreErr
	}
	return s.manualFinalScoreEpic, nil
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
		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, svc, &mockAIClient{}, cfg, &mockNotifier{})

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

	t.Run("cross_team_forbidden", func(t *testing.T) {
		// team-admin команды A не может проставлять оценку эпика команды B.
		teamA := uuid.New()
		teamB := uuid.New()
		repo := &mockAdminScoresRepo{
			user:          &domain.User{ID: userID, TelegramID: "user_tg"},
			role:          &domain.Role{ID: roleID, Name: "BE разработчик"},
			epic:          &domain.Epic{ID: epicID, Status: domain.StatusScoring, TeamID: teamB},
			allowedTeamID: &teamA,
		}
		svc := &mockAdminScoresSvc{repo: repo}
		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, svc, &mockAIClient{}, cfg, &mockNotifier{})

		body := `{"epic_id":"` + epicID.String() + `","user_id":"` + userID.String() + `","score":8}`
		req := httptest.NewRequest("POST", "/api/gantt/admin/scores/epic", strings.NewReader(body))
		session := &middleware.UserSession{TelegramID: "999", Username: "admin_user"}
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserSessionKey, session))
		rr := httptest.NewRecorder()

		handler.AdminSubmitEpicScore(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden for cross-team epic, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		if repo.createdEpicScore {
			t.Error("expected epic score not to be created for cross-team epic")
		}
	})

	t.Run("forbidden_for_member", func(t *testing.T) {
		repo := &mockAdminScoresRepo{
			user:          &domain.User{ID: userID, TelegramID: "user_tg"},
			role:          &domain.Role{ID: roleID, Name: "BE разработчик"},
			epic:          &domain.Epic{ID: epicID, Status: domain.StatusScoring, TeamID: uuid.New()},
			denyTeamAdmin: true,
		}
		svc := &mockAdminScoresSvc{repo: repo}
		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, svc, &mockAIClient{}, cfg, &mockNotifier{})

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
				handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, svc, &mockAIClient{}, cfg, &mockNotifier{})

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
		riskEpicID := uuid.New()
		repo := &mockAdminScoresRepo{
			user: &domain.User{ID: userID, TelegramID: "user_tg"},
			risk: &domain.Risk{ID: riskID, Status: domain.StatusScoring, EpicID: riskEpicID},
			epic: &domain.Epic{ID: riskEpicID, TeamID: uuid.New()},
		}
		svc := &mockAdminScoresSvc{repo: repo}
		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, svc, &mockAIClient{}, cfg, &mockNotifier{})

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
			user:          &domain.User{ID: userID, TelegramID: "user_tg"},
			risk:          &domain.Risk{ID: riskID, Status: domain.StatusScoring, EpicID: uuid.New()},
			denyTeamAdmin: true,
		}
		svc := &mockAdminScoresSvc{repo: repo}
		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, svc, &mockAIClient{}, cfg, &mockNotifier{})

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
		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, svc, &mockAIClient{}, cfg, &mockNotifier{})

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

func TestAdminOverrideFinalScore(t *testing.T) {
	epicID := uuid.New()
	parentID := uuid.New()

	cfg := config.BotConfig{
		Admins: []string{"admin_user"},
	}

	t.Run("forbidden_for_member", func(t *testing.T) {
		repo := &mockAdminScoresRepo{denyTeamAdmin: true}
		svc := &mockAdminScoresSvc{repo: repo}
		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, svc, &mockAIClient{}, cfg, &mockNotifier{})

		body := fmt.Sprintf(`{"epic_id":"%s","final_score":42}`, epicID.String())
		req := httptest.NewRequest("POST", "/api/gantt/admin/scores/final", strings.NewReader(body))
		session := &middleware.UserSession{TelegramID: "111", Username: "regular_user"}
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserSessionKey, session))
		rr := httptest.NewRecorder()

		handler.AdminOverrideFinalScore(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden, got %d", rr.Code)
		}
		if svc.manualFinalScoreCall.called {
			t.Error("expected SetManualFinalScore not to be called")
		}
	})

	t.Run("bad_request_when_scoring_not_complete", func(t *testing.T) {
		repo := &mockAdminScoresRepo{epic: &domain.Epic{ID: epicID, TeamID: uuid.New()}}
		svc := &mockAdminScoresSvc{repo: repo, manualFinalScoreErr: scoring.ErrScoringNotComplete}
		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, svc, &mockAIClient{}, cfg, &mockNotifier{})

		body := fmt.Sprintf(`{"epic_id":"%s","final_score":42}`, epicID.String())
		req := httptest.NewRequest("POST", "/api/gantt/admin/scores/final", strings.NewReader(body))
		session := &middleware.UserSession{TelegramID: "999", Username: "admin_user"}
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserSessionKey, session))
		rr := httptest.NewRecorder()

		handler.AdminOverrideFinalScore(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.NewDecoder(rr.Body).Decode(&errResp); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}
		if errResp.Error != "story scoring is not completed yet" {
			t.Errorf("expected error message 'story scoring is not completed yet', got '%s'", errResp.Error)
		}
	})

	t.Run("success_with_parent_epic_id", func(t *testing.T) {
		finalScore := 42.0
		repo := &mockAdminScoresRepo{epic: &domain.Epic{ID: epicID, TeamID: uuid.New()}}
		svc := &mockAdminScoresSvc{
			repo: repo,
			manualFinalScoreEpic: &domain.Epic{
				ID:           epicID,
				Status:       domain.StatusScored,
				ParentEpicID: &parentID,
				FinalScore:   &finalScore,
			},
		}
		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, svc, &mockAIClient{}, cfg, &mockNotifier{})

		body := fmt.Sprintf(`{"epic_id":"%s","final_score":42}`, epicID.String())
		req := httptest.NewRequest("POST", "/api/gantt/admin/scores/final", strings.NewReader(body))
		session := &middleware.UserSession{TelegramID: "999", Username: "admin_user"}
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserSessionKey, session))
		rr := httptest.NewRecorder()

		handler.AdminOverrideFinalScore(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		if !svc.manualFinalScoreCall.called {
			t.Fatal("expected SetManualFinalScore to be called")
		}
		if svc.manualFinalScoreCall.epicID != epicID {
			t.Errorf("expected SetManualFinalScore called with epicID %v, got %v", epicID, svc.manualFinalScoreCall.epicID)
		}
		if svc.manualFinalScoreCall.finalScore != 42.0 {
			t.Errorf("expected SetManualFinalScore called with finalScore 42.0, got %v", svc.manualFinalScoreCall.finalScore)
		}

		var resp struct {
			Status       string  `json:"status"`
			FinalScore   float64 `json:"final_score"`
			EpicID       string  `json:"epic_id"`
			ParentEpicID *string `json:"parent_epic_id"`
		}
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.Status != "ok" {
			t.Errorf("expected status 'ok', got '%s'", resp.Status)
		}
		if resp.FinalScore != 42.0 {
			t.Errorf("expected final_score 42.0, got %v", resp.FinalScore)
		}
		if resp.EpicID != epicID.String() {
			t.Errorf("expected epic_id %s, got %s", epicID.String(), resp.EpicID)
		}
		if resp.ParentEpicID == nil || *resp.ParentEpicID != parentID.String() {
			t.Errorf("expected parent_epic_id %s, got %v", parentID.String(), resp.ParentEpicID)
		}
	})

	t.Run("success_without_parent_epic_id", func(t *testing.T) {
		finalScore := 15.0
		repo := &mockAdminScoresRepo{epic: &domain.Epic{ID: epicID, TeamID: uuid.New()}}
		svc := &mockAdminScoresSvc{
			repo: repo,
			manualFinalScoreEpic: &domain.Epic{
				ID:         epicID,
				Status:     domain.StatusScored,
				FinalScore: &finalScore,
			},
		}
		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, svc, &mockAIClient{}, cfg, &mockNotifier{})

		body := fmt.Sprintf(`{"epic_id":"%s","final_score":15}`, epicID.String())
		req := httptest.NewRequest("POST", "/api/gantt/admin/scores/final", strings.NewReader(body))
		session := &middleware.UserSession{TelegramID: "999", Username: "admin_user"}
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserSessionKey, session))
		rr := httptest.NewRecorder()

		handler.AdminOverrideFinalScore(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var resp struct {
			ParentEpicID *string `json:"parent_epic_id"`
		}
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.ParentEpicID != nil {
			t.Errorf("expected parent_epic_id null, got %v", *resp.ParentEpicID)
		}
	})

	t.Run("bad_request_invalid_final_score", func(t *testing.T) {
		repo := &mockAdminScoresRepo{}
		svc := &mockAdminScoresSvc{repo: repo}
		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, svc, &mockAIClient{}, cfg, &mockNotifier{})

		body := fmt.Sprintf(`{"epic_id":"%s","final_score":-1}`, epicID.String())
		req := httptest.NewRequest("POST", "/api/gantt/admin/scores/final", strings.NewReader(body))
		session := &middleware.UserSession{TelegramID: "999", Username: "admin_user"}
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserSessionKey, session))
		rr := httptest.NewRecorder()

		handler.AdminOverrideFinalScore(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request, got %d", rr.Code)
		}
		if svc.manualFinalScoreCall.called {
			t.Error("expected SetManualFinalScore not to be called for invalid final_score")
		}
	})

	t.Run("bad_request_invalid_epic_id", func(t *testing.T) {
		repo := &mockAdminScoresRepo{}
		svc := &mockAdminScoresSvc{repo: repo}
		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, svc, &mockAIClient{}, cfg, &mockNotifier{})

		body := `{"epic_id":"not-a-uuid","final_score":42}`
		req := httptest.NewRequest("POST", "/api/gantt/admin/scores/final", strings.NewReader(body))
		session := &middleware.UserSession{TelegramID: "999", Username: "admin_user"}
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserSessionKey, session))
		rr := httptest.NewRecorder()

		handler.AdminOverrideFinalScore(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request, got %d", rr.Code)
		}
	})
}
