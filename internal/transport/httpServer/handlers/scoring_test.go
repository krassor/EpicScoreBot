package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"EpicScoreBot/internal/config"
	"EpicScoreBot/internal/models/domain"
	"EpicScoreBot/internal/transport/httpServer/middleware"
	"log/slog"

	"github.com/google/uuid"
)

type mockScoringRepo struct {
	Repository
	user               *domain.User
	role               *domain.Role
	epic               *domain.Epic
	risk               *domain.Risk
	createdEpicScore   bool
	createdRiskScore   bool
	completedEpicScoring bool
	completedRiskScoring bool
	epicScores         []domain.EpicScore
	riskScores         []domain.RiskScore
}

func (m *mockScoringRepo) FindUserByTelegramID(ctx context.Context, telegramID string) (*domain.User, error) {
	return m.user, nil
}

func (m *mockScoringRepo) GetRoleByUserID(ctx context.Context, userID uuid.UUID) (*domain.Role, error) {
	return m.role, nil
}

func (m *mockScoringRepo) CreateEpicScore(ctx context.Context, epicID, userID, roleID uuid.UUID, score int) error {
	m.createdEpicScore = true
	return nil
}

func (m *mockScoringRepo) CreateRiskScore(ctx context.Context, riskID, userID uuid.UUID, probability, impact int) error {
	m.createdRiskScore = true
	return nil
}

func (m *mockScoringRepo) GetEpicByID(ctx context.Context, epicID uuid.UUID) (*domain.Epic, error) {
	return m.epic, nil
}

func (m *mockScoringRepo) GetRiskByID(ctx context.Context, riskID uuid.UUID) (*domain.Risk, error) {
	return m.risk, nil
}

func (m *mockScoringRepo) CountTeamMembers(ctx context.Context, teamID uuid.UUID) (int, error) {
	return 5, nil
}

func (m *mockScoringRepo) CountEpicScores(ctx context.Context, epicID uuid.UUID) (int, error) {
	return 3, nil
}

func (m *mockScoringRepo) CountRiskScores(ctx context.Context, riskID uuid.UUID) (int, error) {
	return 4, nil
}

func (m *mockScoringRepo) GetEpicScoresByUserID(ctx context.Context, userID uuid.UUID) ([]domain.EpicScore, error) {
	return m.epicScores, nil
}

func (m *mockScoringRepo) GetRiskScoresByUserID(ctx context.Context, userID uuid.UUID) ([]domain.RiskScore, error) {
	return m.riskScores, nil
}

type mockScoringSvc struct {
	ScoringService
	repo *mockScoringRepo
}

func (s *mockScoringSvc) TryCompleteEpicScoring(ctx context.Context, epicID uuid.UUID) error {
	s.repo.completedEpicScoring = true
	return nil
}

func (s *mockScoringSvc) TryCompleteRiskScoring(ctx context.Context, riskID uuid.UUID) error {
	s.repo.completedRiskScoring = true
	return nil
}

func TestSubmitEpicScore(t *testing.T) {
	userID := uuid.New()
	roleID := uuid.New()
	epicID := uuid.New()

	repo := &mockScoringRepo{
		user: &domain.User{ID: userID, TelegramID: "12345"},
		role: &domain.Role{ID: roleID, Name: "BE разработчик"},
		epic: &domain.Epic{ID: epicID, Status: domain.StatusScoring, TeamID: uuid.New()},
	}
	svc := &mockScoringSvc{repo: repo}

	handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, svc, &mockAIClient{}, config.BotConfig{})

	body := `{"epic_id":"` + epicID.String() + `","score":13}`
	req := httptest.NewRequest("POST", "/api/gantt/scores/epic", strings.NewReader(body))
	session := &middleware.UserSession{TelegramID: "12345", Username: "test"}
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserSessionKey, session))
	rr := httptest.NewRecorder()

	handler.SubmitEpicScore(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	if !repo.createdEpicScore {
		t.Error("expected epic score to be created")
	}
	if !repo.completedEpicScoring {
		t.Error("expected TryCompleteEpicScoring to be called")
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["scores_received"].(float64) != 3 {
		t.Errorf("expected 3 scores_received, got %v", resp["scores_received"])
	}
}

func TestSubmitEpicScoreValidation(t *testing.T) {
	userID := uuid.New()
	roleID := uuid.New()
	epicID := uuid.New()

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
			repo := &mockScoringRepo{
				user: &domain.User{ID: userID, TelegramID: "12345"},
				role: &domain.Role{ID: roleID, Name: "BE разработчик"},
				epic: &domain.Epic{ID: epicID, Status: domain.StatusScoring, TeamID: uuid.New()},
			}
			svc := &mockScoringSvc{repo: repo}
			handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, svc, &mockAIClient{}, config.BotConfig{})

			body := fmt.Sprintf(`{"epic_id":"%s","score":%d}`, epicID.String(), tc.score)
			req := httptest.NewRequest("POST", "/api/gantt/scores/epic", strings.NewReader(body))
			session := &middleware.UserSession{TelegramID: "12345", Username: "test"}
			req = req.WithContext(context.WithValue(req.Context(), middleware.UserSessionKey, session))
			rr := httptest.NewRecorder()

			handler.SubmitEpicScore(rr, req)
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
}

func TestSubmitRiskScore(t *testing.T) {
	userID := uuid.New()
	riskID := uuid.New()

	repo := &mockScoringRepo{
		user: &domain.User{ID: userID, TelegramID: "12345"},
		risk: &domain.Risk{ID: riskID, Status: domain.StatusScoring, EpicID: uuid.New()},
	}
	svc := &mockScoringSvc{repo: repo}

	handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, svc, &mockAIClient{}, config.BotConfig{})

	body := `{"risk_id":"` + riskID.String() + `","probability":3,"impact":4}`
	req := httptest.NewRequest("POST", "/api/gantt/scores/risk", strings.NewReader(body))
	session := &middleware.UserSession{TelegramID: "12345", Username: "test"}
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserSessionKey, session))
	rr := httptest.NewRecorder()

	handler.SubmitRiskScore(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	if !repo.createdRiskScore {
		t.Error("expected risk score to be created")
	}
	if !repo.completedRiskScoring {
		t.Error("expected TryCompleteRiskScoring to be called")
	}
}

func TestGetMyScores(t *testing.T) {
	userID := uuid.New()
	epicID := uuid.New()
	roleID := uuid.New()
	riskID := uuid.New()

	repo := &mockScoringRepo{
		user: &domain.User{ID: userID, TelegramID: "12345"},
		epicScores: []domain.EpicScore{
			{ID: uuid.New(), EpicID: epicID, UserID: userID, RoleID: roleID, Score: 8, CreatedAt: time.Now()},
		},
		riskScores: []domain.RiskScore{
			{ID: uuid.New(), RiskID: riskID, UserID: userID, Probability: 2, Impact: 3, CreatedAt: time.Now()},
		},
	}

	handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, &mockScoringSvc{}, &mockAIClient{}, config.BotConfig{})

	req := httptest.NewRequest("GET", "/api/gantt/scores/my", nil)
	session := &middleware.UserSession{TelegramID: "12345", Username: "test"}
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserSessionKey, session))
	rr := httptest.NewRecorder()

	handler.GetMyScores(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	epics := resp["epic_scores"].([]any)
	if len(epics) != 1 {
		t.Errorf("expected 1 epic score, got %d", len(epics))
	}
	if epics[0].(map[string]any)["score"].(float64) != 8 {
		t.Errorf("expected score 8, got %v", epics[0].(map[string]any)["score"])
	}
}
