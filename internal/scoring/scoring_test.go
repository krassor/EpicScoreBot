package scoring

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"math"
	"testing"

	"EpicScoreBot/internal/models/domain"

	"github.com/google/uuid"
)

// MockRepository implements the Repository interface for testing.
type MockRepository struct {
	GetEpicScoresByEpicIDAndRoleIDFunc  func(ctx context.Context, epicID, roleID uuid.UUID) ([]domain.EpicScore, error)
	GetUserByIDFunc                     func(ctx context.Context, userID uuid.UUID) (*domain.User, error)
	GetRiskScoresByRiskIDFunc           func(ctx context.Context, riskID uuid.UUID) ([]domain.RiskScore, error)
	GetRiskByIDFunc                     func(ctx context.Context, riskID uuid.UUID) (*domain.Risk, error)
	GetEpicByIDFunc                     func(ctx context.Context, epicID uuid.UUID) (*domain.Epic, error)
	CountTeamMembersFunc                func(ctx context.Context, teamID uuid.UUID) (int, error)
	CountRiskScoresFunc                 func(ctx context.Context, riskID uuid.UUID) (int, error)
	SetRiskWeightedScoreFunc            func(ctx context.Context, riskID uuid.UUID, score float64) error
	CountEpicScoresFunc                 func(ctx context.Context, epicID uuid.UUID) (int, error)
	GetExpectedScorersCountFunc         func(ctx context.Context, epicID uuid.UUID, teamID uuid.UUID) (int, error)
	GetSubmittedEpicScorersCountFunc    func(ctx context.Context, epicID uuid.UUID, teamID uuid.UUID) (int, error)
	GetSubmittedRiskScorersCountFunc    func(ctx context.Context, riskID uuid.UUID, epicID uuid.UUID, teamID uuid.UUID) (int, error)
	GetDistinctRoleIDsForEpicScoresFunc func(ctx context.Context, epicID uuid.UUID) ([]uuid.UUID, error)
	UpsertEpicRoleScoreFunc             func(ctx context.Context, epicID, roleID uuid.UUID, weightedAvg float64) error
	GetRisksByEpicIDFunc                func(ctx context.Context, epicID uuid.UUID) ([]domain.Risk, error)
	SetEpicFinalScoreFunc               func(ctx context.Context, epicID uuid.UUID, score float64) error
	GetStoriesByEpicIDFunc              func(ctx context.Context, epicID uuid.UUID) ([]domain.Epic, error)
	GetEpicRoleScoresByEpicIDFunc       func(ctx context.Context, epicID uuid.UUID) ([]domain.EpicRoleScore, error)
}

func (m *MockRepository) GetEpicScoresByEpicIDAndRoleID(ctx context.Context, epicID, roleID uuid.UUID) ([]domain.EpicScore, error) {
	if m.GetEpicScoresByEpicIDAndRoleIDFunc != nil {
		return m.GetEpicScoresByEpicIDAndRoleIDFunc(ctx, epicID, roleID)
	}
	return nil, nil
}

func (m *MockRepository) GetUserByID(ctx context.Context, userID uuid.UUID) (*domain.User, error) {
	if m.GetUserByIDFunc != nil {
		return m.GetUserByIDFunc(ctx, userID)
	}
	return nil, nil
}

func (m *MockRepository) GetRiskScoresByRiskID(ctx context.Context, riskID uuid.UUID) ([]domain.RiskScore, error) {
	if m.GetRiskScoresByRiskIDFunc != nil {
		return m.GetRiskScoresByRiskIDFunc(ctx, riskID)
	}
	return nil, nil
}

func (m *MockRepository) GetRiskByID(ctx context.Context, riskID uuid.UUID) (*domain.Risk, error) {
	if m.GetRiskByIDFunc != nil {
		return m.GetRiskByIDFunc(ctx, riskID)
	}
	return nil, nil
}

func (m *MockRepository) GetEpicByID(ctx context.Context, epicID uuid.UUID) (*domain.Epic, error) {
	if m.GetEpicByIDFunc != nil {
		return m.GetEpicByIDFunc(ctx, epicID)
	}
	return nil, nil
}

func (m *MockRepository) CountTeamMembers(ctx context.Context, teamID uuid.UUID) (int, error) {
	if m.CountTeamMembersFunc != nil {
		return m.CountTeamMembersFunc(ctx, teamID)
	}
	return 0, nil
}

func (m *MockRepository) CountRiskScores(ctx context.Context, riskID uuid.UUID) (int, error) {
	if m.CountRiskScoresFunc != nil {
		return m.CountRiskScoresFunc(ctx, riskID)
	}
	return 0, nil
}

func (m *MockRepository) SetRiskWeightedScore(ctx context.Context, riskID uuid.UUID, score float64) error {
	if m.SetRiskWeightedScoreFunc != nil {
		return m.SetRiskWeightedScoreFunc(ctx, riskID, score)
	}
	return nil
}

func (m *MockRepository) CountEpicScores(ctx context.Context, epicID uuid.UUID) (int, error) {
	if m.CountEpicScoresFunc != nil {
		return m.CountEpicScoresFunc(ctx, epicID)
	}
	return 0, nil
}

func (m *MockRepository) GetExpectedScorersCount(ctx context.Context, epicID uuid.UUID, teamID uuid.UUID) (int, error) {
	if m.GetExpectedScorersCountFunc != nil {
		return m.GetExpectedScorersCountFunc(ctx, epicID, teamID)
	}
	return 0, nil
}

func (m *MockRepository) GetSubmittedEpicScorersCount(ctx context.Context, epicID uuid.UUID, teamID uuid.UUID) (int, error) {
	if m.GetSubmittedEpicScorersCountFunc != nil {
		return m.GetSubmittedEpicScorersCountFunc(ctx, epicID, teamID)
	}
	return 0, nil
}

func (m *MockRepository) GetSubmittedRiskScorersCount(ctx context.Context, riskID uuid.UUID, epicID uuid.UUID, teamID uuid.UUID) (int, error) {
	if m.GetSubmittedRiskScorersCountFunc != nil {
		return m.GetSubmittedRiskScorersCountFunc(ctx, riskID, epicID, teamID)
	}
	return 0, nil
}

func (m *MockRepository) GetDistinctRoleIDsForEpicScores(ctx context.Context, epicID uuid.UUID) ([]uuid.UUID, error) {
	if m.GetDistinctRoleIDsForEpicScoresFunc != nil {
		return m.GetDistinctRoleIDsForEpicScoresFunc(ctx, epicID)
	}
	return nil, nil
}

func (m *MockRepository) UpsertEpicRoleScore(ctx context.Context, epicID, roleID uuid.UUID, weightedAvg float64) error {
	if m.UpsertEpicRoleScoreFunc != nil {
		return m.UpsertEpicRoleScoreFunc(ctx, epicID, roleID, weightedAvg)
	}
	return nil
}

func (m *MockRepository) GetRisksByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.Risk, error) {
	if m.GetRisksByEpicIDFunc != nil {
		return m.GetRisksByEpicIDFunc(ctx, epicID)
	}
	return nil, nil
}

func (m *MockRepository) SetEpicFinalScore(ctx context.Context, epicID uuid.UUID, score float64) error {
	if m.SetEpicFinalScoreFunc != nil {
		return m.SetEpicFinalScoreFunc(ctx, epicID, score)
	}
	return nil
}

func (m *MockRepository) GetStoriesByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.Epic, error) {
	if m.GetStoriesByEpicIDFunc != nil {
		return m.GetStoriesByEpicIDFunc(ctx, epicID)
	}
	return nil, nil
}

func (m *MockRepository) GetEpicRoleScoresByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.EpicRoleScore, error) {
	if m.GetEpicRoleScoresByEpicIDFunc != nil {
		return m.GetEpicRoleScoresByEpicIDFunc(ctx, epicID)
	}
	return nil, nil
}


func TestCalculateEpicRoleAvg(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()
	epicID := uuid.New()
	roleID := uuid.New()
	userID1 := uuid.New()
	userID2 := uuid.New()

	tests := []struct {
		name      string
		mockRepo  *MockRepository
		wantScore float64
		wantErr   bool
	}{
		{
			name: "successful weighted average calculation",
			mockRepo: &MockRepository{
				GetEpicScoresByEpicIDAndRoleIDFunc: func(ctx context.Context, eID, rID uuid.UUID) ([]domain.EpicScore, error) {
					return []domain.EpicScore{
						{UserID: userID1, Score: 5},
						{UserID: userID2, Score: 10},
					}, nil
				},
				GetUserByIDFunc: func(ctx context.Context, uID uuid.UUID) (*domain.User, error) {
					if uID == userID1 {
						return &domain.User{ID: userID1, Weight: 20}, nil
					}
					if uID == userID2 {
						return &domain.User{ID: userID2, Weight: 80}, nil
					}
					return nil, errors.New("not found")
				},
			},
			wantScore: 9.0, // (5*20 + 10*80) / (20+80) = 900 / 100 = 9.0
			wantErr:   false,
		},
		{
			name: "no scores found",
			mockRepo: &MockRepository{
				GetEpicScoresByEpicIDAndRoleIDFunc: func(ctx context.Context, eID, rID uuid.UUID) ([]domain.EpicScore, error) {
					return []domain.EpicScore{}, nil
				},
			},
			wantScore: 0.0,
			wantErr:   false,
		},
		{
			name: "total weight is zero",
			mockRepo: &MockRepository{
				GetEpicScoresByEpicIDAndRoleIDFunc: func(ctx context.Context, eID, rID uuid.UUID) ([]domain.EpicScore, error) {
					return []domain.EpicScore{
						{UserID: userID1, Score: 5},
					}, nil
				},
				GetUserByIDFunc: func(ctx context.Context, uID uuid.UUID) (*domain.User, error) {
					return &domain.User{ID: userID1, Weight: 0}, nil
				},
			},
			wantScore: 0.0,
			wantErr:   false,
		},
		{
			name: "repository GetEpicScores error",
			mockRepo: &MockRepository{
				GetEpicScoresByEpicIDAndRoleIDFunc: func(ctx context.Context, eID, rID uuid.UUID) ([]domain.EpicScore, error) {
					return nil, errors.New("db error")
				},
			},
			wantScore: 0.0,
			wantErr:   true,
		},
		{
			name: "repository GetUserByID error",
			mockRepo: &MockRepository{
				GetEpicScoresByEpicIDAndRoleIDFunc: func(ctx context.Context, eID, rID uuid.UUID) ([]domain.EpicScore, error) {
					return []domain.EpicScore{
						{UserID: userID1, Score: 5},
					}, nil
				},
				GetUserByIDFunc: func(ctx context.Context, uID uuid.UUID) (*domain.User, error) {
					return nil, errors.New("db error")
				},
			},
			wantScore: 0.0,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(logger, tt.mockRepo)
			got, err := s.CalculateEpicRoleAvg(ctx, epicID, roleID)
			if (err != nil) != tt.wantErr {
				t.Errorf("CalculateEpicRoleAvg() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if math.Abs(got-tt.wantScore) > 1e-9 {
				t.Errorf("CalculateEpicRoleAvg() got = %v, want %v", got, tt.wantScore)
			}
		})
	}
}

func TestCalculateRiskWeightedScore(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()
	riskID := uuid.New()
	userID1 := uuid.New()
	userID2 := uuid.New()

	tests := []struct {
		name      string
		mockRepo  *MockRepository
		wantScore float64
		wantErr   bool
	}{
		{
			name: "successful risk weighted average calculation",
			mockRepo: &MockRepository{
				GetRiskScoresByRiskIDFunc: func(ctx context.Context, rID uuid.UUID) ([]domain.RiskScore, error) {
					return []domain.RiskScore{
						{UserID: userID1, Probability: 2, Impact: 3}, // Score = 6
						{UserID: userID2, Probability: 4, Impact: 4}, // Score = 16
					}, nil
				},
				GetUserByIDFunc: func(ctx context.Context, uID uuid.UUID) (*domain.User, error) {
					if uID == userID1 {
						return &domain.User{ID: userID1, Weight: 25}, nil
					}
					if uID == userID2 {
						return &domain.User{ID: userID2, Weight: 75}, nil
					}
					return nil, errors.New("not found")
				},
			},
			wantScore: 13.5, // (6*25 + 16*75) / (25+75) = (150 + 1200) / 100 = 13.5
			wantErr:   false,
		},
		{
			name: "no risk scores found",
			mockRepo: &MockRepository{
				GetRiskScoresByRiskIDFunc: func(ctx context.Context, rID uuid.UUID) ([]domain.RiskScore, error) {
					return []domain.RiskScore{}, nil
				},
			},
			wantScore: 0.0,
			wantErr:   false,
		},
		{
			name: "total weight is zero",
			mockRepo: &MockRepository{
				GetRiskScoresByRiskIDFunc: func(ctx context.Context, rID uuid.UUID) ([]domain.RiskScore, error) {
					return []domain.RiskScore{
						{UserID: userID1, Probability: 3, Impact: 3},
					}, nil
				},
				GetUserByIDFunc: func(ctx context.Context, uID uuid.UUID) (*domain.User, error) {
					return &domain.User{ID: userID1, Weight: 0}, nil
				},
			},
			wantScore: 0.0,
			wantErr:   false,
		},
		{
			name: "repository GetRiskScores error",
			mockRepo: &MockRepository{
				GetRiskScoresByRiskIDFunc: func(ctx context.Context, rID uuid.UUID) ([]domain.RiskScore, error) {
					return nil, errors.New("db error")
				},
			},
			wantScore: 0.0,
			wantErr:   true,
		},
		{
			name: "repository GetUserByID error",
			mockRepo: &MockRepository{
				GetRiskScoresByRiskIDFunc: func(ctx context.Context, rID uuid.UUID) ([]domain.RiskScore, error) {
					return []domain.RiskScore{
						{UserID: userID1, Probability: 3, Impact: 3},
					}, nil
				},
				GetUserByIDFunc: func(ctx context.Context, uID uuid.UUID) (*domain.User, error) {
					return nil, errors.New("db error")
				},
			},
			wantScore: 0.0,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(logger, tt.mockRepo)
			got, err := s.CalculateRiskWeightedScore(ctx, riskID)
			if (err != nil) != tt.wantErr {
				t.Errorf("CalculateRiskWeightedScore() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if math.Abs(got-tt.wantScore) > 1e-9 {
				t.Errorf("CalculateRiskWeightedScore() got = %v, want %v", got, tt.wantScore)
			}
		})
	}
}

func TestTryCompleteRiskScoring(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()
	riskID := uuid.New()
	epicID := uuid.New()
	teamID := uuid.New()
	userID := uuid.New()

	tests := []struct {
		name     string
		mockRepo *MockRepository
		wantErr  bool
	}{
		{
			name: "scoring not complete yet",
			mockRepo: &MockRepository{
				GetRiskByIDFunc: func(ctx context.Context, rID uuid.UUID) (*domain.Risk, error) {
					return &domain.Risk{ID: riskID, EpicID: epicID}, nil
				},
				GetEpicByIDFunc: func(ctx context.Context, eID uuid.UUID) (*domain.Epic, error) {
					return &domain.Epic{ID: epicID, TeamID: teamID}, nil
				},
				GetExpectedScorersCountFunc: func(ctx context.Context, eID, tID uuid.UUID) (int, error) {
					return 5, nil
				},
				GetSubmittedRiskScorersCountFunc: func(ctx context.Context, rID, eID, tID uuid.UUID) (int, error) {
					return 3, nil
				},
			},
			wantErr: false,
		},
		{
			name: "scoring complete and cascades to epic scoring",
			mockRepo: &MockRepository{
				GetRiskByIDFunc: func(ctx context.Context, rID uuid.UUID) (*domain.Risk, error) {
					return &domain.Risk{ID: riskID, EpicID: epicID}, nil
				},
				GetEpicByIDFunc: func(ctx context.Context, eID uuid.UUID) (*domain.Epic, error) {
					return &domain.Epic{ID: epicID, TeamID: teamID, Status: domain.StatusScored}, nil
				},
				GetExpectedScorersCountFunc: func(ctx context.Context, eID, tID uuid.UUID) (int, error) {
					return 3, nil
				},
				GetSubmittedRiskScorersCountFunc: func(ctx context.Context, rID, eID, tID uuid.UUID) (int, error) {
					return 3, nil
				},
				GetRiskScoresByRiskIDFunc: func(ctx context.Context, rID uuid.UUID) ([]domain.RiskScore, error) {
					return []domain.RiskScore{
						{UserID: userID, Probability: 3, Impact: 4}, // Score = 12
					}, nil
				},
				GetUserByIDFunc: func(ctx context.Context, uID uuid.UUID) (*domain.User, error) {
					return &domain.User{ID: userID, Weight: 100}, nil
				},
				SetRiskWeightedScoreFunc: func(ctx context.Context, rID uuid.UUID, score float64) error {
					if rID != riskID {
						t.Errorf("SetRiskWeightedScore called with riskID %v, want %v", rID, riskID)
					}
					if math.Abs(score-12.0) > 1e-9 {
						t.Errorf("SetRiskWeightedScore called with score %v, want 12.0", score)
					}
					return nil
				},
			},
			wantErr: false,
		},
		{
			name: "repository GetRiskByID error",
			mockRepo: &MockRepository{
				GetRiskByIDFunc: func(ctx context.Context, rID uuid.UUID) (*domain.Risk, error) {
					return nil, errors.New("db error")
				},
			},
			wantErr: true,
		},
		{
			name: "repository GetEpicByID error",
			mockRepo: &MockRepository{
				GetRiskByIDFunc: func(ctx context.Context, rID uuid.UUID) (*domain.Risk, error) {
					return &domain.Risk{ID: riskID, EpicID: epicID}, nil
				},
				GetEpicByIDFunc: func(ctx context.Context, eID uuid.UUID) (*domain.Epic, error) {
					return nil, errors.New("db error")
				},
			},
			wantErr: true,
		},
		{
			name: "repository GetExpectedScorersCount error",
			mockRepo: &MockRepository{
				GetRiskByIDFunc: func(ctx context.Context, rID uuid.UUID) (*domain.Risk, error) {
					return &domain.Risk{ID: riskID, EpicID: epicID}, nil
				},
				GetEpicByIDFunc: func(ctx context.Context, eID uuid.UUID) (*domain.Epic, error) {
					return &domain.Epic{ID: epicID, TeamID: teamID}, nil
				},
				GetExpectedScorersCountFunc: func(ctx context.Context, eID, tID uuid.UUID) (int, error) {
					return 0, errors.New("db error")
				},
			},
			wantErr: true,
		},
		{
			name: "repository GetSubmittedRiskScorersCount error",
			mockRepo: &MockRepository{
				GetRiskByIDFunc: func(ctx context.Context, rID uuid.UUID) (*domain.Risk, error) {
					return &domain.Risk{ID: riskID, EpicID: epicID}, nil
				},
				GetEpicByIDFunc: func(ctx context.Context, eID uuid.UUID) (*domain.Epic, error) {
					return &domain.Epic{ID: epicID, TeamID: teamID}, nil
				},
				GetExpectedScorersCountFunc: func(ctx context.Context, eID, tID uuid.UUID) (int, error) {
					return 3, nil
				},
				GetSubmittedRiskScorersCountFunc: func(ctx context.Context, rID, eID, tID uuid.UUID) (int, error) {
					return 0, errors.New("db error")
				},
			},
			wantErr: true,
		},
		{
			name: "CalculateRiskWeightedScore error in completion flow",
			mockRepo: &MockRepository{
				GetRiskByIDFunc: func(ctx context.Context, rID uuid.UUID) (*domain.Risk, error) {
					return &domain.Risk{ID: riskID, EpicID: epicID}, nil
				},
				GetEpicByIDFunc: func(ctx context.Context, eID uuid.UUID) (*domain.Epic, error) {
					return &domain.Epic{ID: epicID, TeamID: teamID}, nil
				},
				GetExpectedScorersCountFunc: func(ctx context.Context, eID, tID uuid.UUID) (int, error) {
					return 3, nil
				},
				GetSubmittedRiskScorersCountFunc: func(ctx context.Context, rID, eID, tID uuid.UUID) (int, error) {
					return 3, nil
				},
				GetRiskScoresByRiskIDFunc: func(ctx context.Context, rID uuid.UUID) ([]domain.RiskScore, error) {
					return nil, errors.New("db error")
				},
			},
			wantErr: true,
		},
		{
			name: "repository SetRiskWeightedScore error",
			mockRepo: &MockRepository{
				GetRiskByIDFunc: func(ctx context.Context, rID uuid.UUID) (*domain.Risk, error) {
					return &domain.Risk{ID: riskID, EpicID: epicID}, nil
				},
				GetEpicByIDFunc: func(ctx context.Context, eID uuid.UUID) (*domain.Epic, error) {
					return &domain.Epic{ID: epicID, TeamID: teamID}, nil
				},
				GetExpectedScorersCountFunc: func(ctx context.Context, eID, tID uuid.UUID) (int, error) {
					return 3, nil
				},
				GetSubmittedRiskScorersCountFunc: func(ctx context.Context, rID, eID, tID uuid.UUID) (int, error) {
					return 3, nil
				},
				GetRiskScoresByRiskIDFunc: func(ctx context.Context, rID uuid.UUID) ([]domain.RiskScore, error) {
					return []domain.RiskScore{
						{UserID: userID, Probability: 3, Impact: 4},
					}, nil
				},
				GetUserByIDFunc: func(ctx context.Context, uID uuid.UUID) (*domain.User, error) {
					return &domain.User{ID: userID, Weight: 100}, nil
				},
				SetRiskWeightedScoreFunc: func(ctx context.Context, rID uuid.UUID, score float64) error {
					return errors.New("db error")
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(logger, tt.mockRepo)
			err := s.TryCompleteRiskScoring(ctx, riskID)
			if (err != nil) != tt.wantErr {
				t.Errorf("TryCompleteRiskScoring() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTryCompleteEpicScoring(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()
	epicID := uuid.New()
	teamID := uuid.New()
	roleID1 := uuid.New()
	roleID2 := uuid.New()
	userID := uuid.New()
	riskID1 := uuid.New()
	riskID2 := uuid.New()

	riskScore1 := 13.0
	riskScore2 := 4.0

	tests := []struct {
		name     string
		mockRepo *MockRepository
		wantErr  bool
	}{
		{
			name: "epic already scored",
			mockRepo: &MockRepository{
				GetEpicByIDFunc: func(ctx context.Context, eID uuid.UUID) (*domain.Epic, error) {
					return &domain.Epic{ID: epicID, Status: domain.StatusScored}, nil
				},
			},
			wantErr: false,
		},
		{
			name: "epic scoring not complete yet",
			mockRepo: &MockRepository{
				GetEpicByIDFunc: func(ctx context.Context, eID uuid.UUID) (*domain.Epic, error) {
					return &domain.Epic{ID: epicID, TeamID: teamID, Status: domain.StatusScoring}, nil
				},
				GetExpectedScorersCountFunc: func(ctx context.Context, eID, tID uuid.UUID) (int, error) {
					return 5, nil
				},
				GetSubmittedEpicScorersCountFunc: func(ctx context.Context, eID, tID uuid.UUID) (int, error) {
					return 3, nil
				},
			},
			wantErr: false,
		},
		{
			name: "epic scores collected but risks are not completed",
			mockRepo: &MockRepository{
				GetEpicByIDFunc: func(ctx context.Context, eID uuid.UUID) (*domain.Epic, error) {
					return &domain.Epic{ID: epicID, TeamID: teamID, Status: domain.StatusScoring}, nil
				},
				GetExpectedScorersCountFunc: func(ctx context.Context, eID, tID uuid.UUID) (int, error) {
					return 3, nil
				},
				GetSubmittedEpicScorersCountFunc: func(ctx context.Context, eID, tID uuid.UUID) (int, error) {
					return 3, nil
				},
				GetDistinctRoleIDsForEpicScoresFunc: func(ctx context.Context, eID uuid.UUID) ([]uuid.UUID, error) {
					return []uuid.UUID{roleID1}, nil
				},
				GetEpicScoresByEpicIDAndRoleIDFunc: func(ctx context.Context, eID, rID uuid.UUID) ([]domain.EpicScore, error) {
					return []domain.EpicScore{{UserID: userID, Score: 10}}, nil
				},
				GetUserByIDFunc: func(ctx context.Context, uID uuid.UUID) (*domain.User, error) {
					return &domain.User{ID: userID, Weight: 100}, nil
				},
				UpsertEpicRoleScoreFunc: func(ctx context.Context, eID, rID uuid.UUID, score float64) error {
					return nil
				},
				GetRisksByEpicIDFunc: func(ctx context.Context, eID uuid.UUID) ([]domain.Risk, error) {
					return []domain.Risk{
						{ID: riskID1, Status: domain.StatusScored},
						{ID: riskID2, Status: domain.StatusScoring}, // Not complete!
					}, nil
				},
			},
			wantErr: false,
		},
		{
			name: "successful complete scoring (calculates and updates final score with coefficients)",
			mockRepo: &MockRepository{
				GetEpicByIDFunc: func(ctx context.Context, eID uuid.UUID) (*domain.Epic, error) {
					return &domain.Epic{ID: epicID, TeamID: teamID, Status: domain.StatusScoring}, nil
				},
				GetExpectedScorersCountFunc: func(ctx context.Context, eID, tID uuid.UUID) (int, error) {
					return 3, nil
				},
				GetSubmittedEpicScorersCountFunc: func(ctx context.Context, eID, tID uuid.UUID) (int, error) {
					return 3, nil
				},
				GetDistinctRoleIDsForEpicScoresFunc: func(ctx context.Context, eID uuid.UUID) ([]uuid.UUID, error) {
					return []uuid.UUID{roleID1, roleID2}, nil
				},
				GetEpicScoresByEpicIDAndRoleIDFunc: func(ctx context.Context, eID, rID uuid.UUID) ([]domain.EpicScore, error) {
					if rID == roleID1 {
						return []domain.EpicScore{{UserID: userID, Score: 10}}, nil
					}
					return []domain.EpicScore{{UserID: userID, Score: 15}}, nil
				},
				GetUserByIDFunc: func(ctx context.Context, uID uuid.UUID) (*domain.User, error) {
					return &domain.User{ID: userID, Weight: 100}, nil
				},
				UpsertEpicRoleScoreFunc: func(ctx context.Context, eID, rID uuid.UUID, score float64) error {
					return nil
				},
				GetRisksByEpicIDFunc: func(ctx context.Context, eID uuid.UUID) ([]domain.Risk, error) {
					return []domain.Risk{
						{ID: riskID1, Status: domain.StatusScored, WeightedScore: &riskScore1}, // Coefficient for 13.0 = 1.20
						{ID: riskID2, Status: domain.StatusScored, WeightedScore: &riskScore2}, // Coefficient for 4.0 = 1.03
					}, nil
				},
				SetEpicFinalScoreFunc: func(ctx context.Context, eID uuid.UUID, score float64) error {
					// Base score = 10.0 + 15.0 = 25.0
					// Coefficients: 1.20 and 1.03
					// Final score = round(25.0 * 1.20 * 1.03) = round(30.0 * 1.03) = round(30.9) = 31.0
					if math.Abs(score-31.0) > 1e-9 {
						t.Errorf("SetEpicFinalScore called with score %v, want 31.0", score)
					}
					return nil
				},
			},
			wantErr: false,
		},
		{
			name: "successful complete scoring without evaluating roles",
			mockRepo: &MockRepository{
				GetEpicByIDFunc: func(ctx context.Context, eID uuid.UUID) (*domain.Epic, error) {
					return &domain.Epic{ID: epicID, TeamID: teamID, Status: domain.StatusScoring}, nil
				},
				GetExpectedScorersCountFunc: func(ctx context.Context, eID, tID uuid.UUID) (int, error) {
					return 3, nil
				},
				GetSubmittedEpicScorersCountFunc: func(ctx context.Context, eID, tID uuid.UUID) (int, error) {
					return 3, nil
				},
				GetDistinctRoleIDsForEpicScoresFunc: func(ctx context.Context, eID uuid.UUID) ([]uuid.UUID, error) {
					return []uuid.UUID{}, nil
				},
				GetRisksByEpicIDFunc: func(ctx context.Context, eID uuid.UUID) ([]domain.Risk, error) {
					return []domain.Risk{}, nil
				},
				SetEpicFinalScoreFunc: func(ctx context.Context, eID uuid.UUID, score float64) error {
					if math.Abs(score-0.0) > 1e-9 {
						t.Errorf("SetEpicFinalScore called with score %v, want 0.0", score)
					}
					return nil
				},
			},
			wantErr: false,
		},
		{
			name: "repository GetEpicByID error",
			mockRepo: &MockRepository{
				GetEpicByIDFunc: func(ctx context.Context, eID uuid.UUID) (*domain.Epic, error) {
					return nil, errors.New("db error")
				},
			},
			wantErr: true,
		},
		{
			name: "repository GetExpectedScorersCount error",
			mockRepo: &MockRepository{
				GetEpicByIDFunc: func(ctx context.Context, eID uuid.UUID) (*domain.Epic, error) {
					return &domain.Epic{ID: epicID, TeamID: teamID, Status: domain.StatusScoring}, nil
				},
				GetExpectedScorersCountFunc: func(ctx context.Context, eID, tID uuid.UUID) (int, error) {
					return 0, errors.New("db error")
				},
			},
			wantErr: true,
		},
		{
			name: "repository GetSubmittedEpicScorersCount error",
			mockRepo: &MockRepository{
				GetEpicByIDFunc: func(ctx context.Context, eID uuid.UUID) (*domain.Epic, error) {
					return &domain.Epic{ID: epicID, TeamID: teamID, Status: domain.StatusScoring}, nil
				},
				GetExpectedScorersCountFunc: func(ctx context.Context, eID, tID uuid.UUID) (int, error) {
					return 3, nil
				},
				GetSubmittedEpicScorersCountFunc: func(ctx context.Context, eID, tID uuid.UUID) (int, error) {
					return 0, errors.New("db error")
				},
			},
			wantErr: true,
		},
		{
			name: "repository GetDistinctRoleIDsForEpicScores error",
			mockRepo: &MockRepository{
				GetEpicByIDFunc: func(ctx context.Context, eID uuid.UUID) (*domain.Epic, error) {
					return &domain.Epic{ID: epicID, TeamID: teamID, Status: domain.StatusScoring}, nil
				},
				GetExpectedScorersCountFunc: func(ctx context.Context, eID, tID uuid.UUID) (int, error) {
					return 3, nil
				},
				GetSubmittedEpicScorersCountFunc: func(ctx context.Context, eID, tID uuid.UUID) (int, error) {
					return 3, nil
				},
				GetDistinctRoleIDsForEpicScoresFunc: func(ctx context.Context, eID uuid.UUID) ([]uuid.UUID, error) {
					return nil, errors.New("db error")
				},
			},
			wantErr: true,
		},
		{
			name: "CalculateEpicRoleAvg error in flow",
			mockRepo: &MockRepository{
				GetEpicByIDFunc: func(ctx context.Context, eID uuid.UUID) (*domain.Epic, error) {
					return &domain.Epic{ID: epicID, TeamID: teamID, Status: domain.StatusScoring}, nil
				},
				GetExpectedScorersCountFunc: func(ctx context.Context, eID, tID uuid.UUID) (int, error) {
					return 3, nil
				},
				GetSubmittedEpicScorersCountFunc: func(ctx context.Context, eID, tID uuid.UUID) (int, error) {
					return 3, nil
				},
				GetDistinctRoleIDsForEpicScoresFunc: func(ctx context.Context, eID uuid.UUID) ([]uuid.UUID, error) {
					return []uuid.UUID{roleID1}, nil
				},
				GetEpicScoresByEpicIDAndRoleIDFunc: func(ctx context.Context, eID, rID uuid.UUID) ([]domain.EpicScore, error) {
					return nil, errors.New("db error")
				},
			},
			wantErr: true,
		},
		{
			name: "repository UpsertEpicRoleScore error",
			mockRepo: &MockRepository{
				GetEpicByIDFunc: func(ctx context.Context, eID uuid.UUID) (*domain.Epic, error) {
					return &domain.Epic{ID: epicID, TeamID: teamID, Status: domain.StatusScoring}, nil
				},
				GetExpectedScorersCountFunc: func(ctx context.Context, eID, tID uuid.UUID) (int, error) {
					return 3, nil
				},
				GetSubmittedEpicScorersCountFunc: func(ctx context.Context, eID, tID uuid.UUID) (int, error) {
					return 3, nil
				},
				GetDistinctRoleIDsForEpicScoresFunc: func(ctx context.Context, eID uuid.UUID) ([]uuid.UUID, error) {
					return []uuid.UUID{roleID1}, nil
				},
				GetEpicScoresByEpicIDAndRoleIDFunc: func(ctx context.Context, eID, rID uuid.UUID) ([]domain.EpicScore, error) {
					return []domain.EpicScore{{UserID: userID, Score: 10}}, nil
				},
				GetUserByIDFunc: func(ctx context.Context, uID uuid.UUID) (*domain.User, error) {
					return &domain.User{ID: userID, Weight: 100}, nil
				},
				UpsertEpicRoleScoreFunc: func(ctx context.Context, eID, rID uuid.UUID, score float64) error {
					return errors.New("db error")
				},
			},
			wantErr: true,
		},
		{
			name: "repository GetRisksByEpicID error",
			mockRepo: &MockRepository{
				GetEpicByIDFunc: func(ctx context.Context, eID uuid.UUID) (*domain.Epic, error) {
					return &domain.Epic{ID: epicID, TeamID: teamID, Status: domain.StatusScoring}, nil
				},
				GetExpectedScorersCountFunc: func(ctx context.Context, eID, tID uuid.UUID) (int, error) {
					return 3, nil
				},
				GetSubmittedEpicScorersCountFunc: func(ctx context.Context, eID, tID uuid.UUID) (int, error) {
					return 3, nil
				},
				GetDistinctRoleIDsForEpicScoresFunc: func(ctx context.Context, eID uuid.UUID) ([]uuid.UUID, error) {
					return []uuid.UUID{roleID1}, nil
				},
				GetEpicScoresByEpicIDAndRoleIDFunc: func(ctx context.Context, eID, rID uuid.UUID) ([]domain.EpicScore, error) {
					return []domain.EpicScore{{UserID: userID, Score: 10}}, nil
				},
				GetUserByIDFunc: func(ctx context.Context, uID uuid.UUID) (*domain.User, error) {
					return &domain.User{ID: userID, Weight: 100}, nil
				},
				UpsertEpicRoleScoreFunc: func(ctx context.Context, eID, rID uuid.UUID, score float64) error {
					return nil
				},
				GetRisksByEpicIDFunc: func(ctx context.Context, eID uuid.UUID) ([]domain.Risk, error) {
					return nil, errors.New("db error")
				},
			},
			wantErr: true,
		},
		{
			name: "repository SetEpicFinalScore error",
			mockRepo: &MockRepository{
				GetEpicByIDFunc: func(ctx context.Context, eID uuid.UUID) (*domain.Epic, error) {
					return &domain.Epic{ID: epicID, TeamID: teamID, Status: domain.StatusScoring}, nil
				},
				GetExpectedScorersCountFunc: func(ctx context.Context, eID, tID uuid.UUID) (int, error) {
					return 3, nil
				},
				GetSubmittedEpicScorersCountFunc: func(ctx context.Context, eID, tID uuid.UUID) (int, error) {
					return 3, nil
				},
				GetDistinctRoleIDsForEpicScoresFunc: func(ctx context.Context, eID uuid.UUID) ([]uuid.UUID, error) {
					return []uuid.UUID{roleID1}, nil
				},
				GetEpicScoresByEpicIDAndRoleIDFunc: func(ctx context.Context, eID, rID uuid.UUID) ([]domain.EpicScore, error) {
					return []domain.EpicScore{{UserID: userID, Score: 10}}, nil
				},
				GetUserByIDFunc: func(ctx context.Context, uID uuid.UUID) (*domain.User, error) {
					return &domain.User{ID: userID, Weight: 100}, nil
				},
				UpsertEpicRoleScoreFunc: func(ctx context.Context, eID, rID uuid.UUID, score float64) error {
					return nil
				},
				GetRisksByEpicIDFunc: func(ctx context.Context, eID uuid.UUID) ([]domain.Risk, error) {
					return []domain.Risk{
						{ID: riskID1, Status: domain.StatusScored, WeightedScore: &riskScore1},
					}, nil
				},
				SetEpicFinalScoreFunc: func(ctx context.Context, eID uuid.UUID, score float64) error {
					return errors.New("db error")
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(logger, tt.mockRepo)
			err := s.TryCompleteEpicScoring(ctx, epicID)
			if (err != nil) != tt.wantErr {
				t.Errorf("TryCompleteEpicScoring() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRiskCoefficient(t *testing.T) {
	tests := []struct {
		score float64
		want  float64
	}{
		{13.5, 1.20},
		{13.0, 1.20},
		{12.5, 1.20}, // rounds to 13
		{12.4, 1.10}, // rounds to 12
		{9.0, 1.10},
		{8.5, 1.10},  // rounds to 9
		{8.4, 1.05},  // rounds to 8
		{5.0, 1.05},
		{4.5, 1.05},  // rounds to 5
		{4.4, 1.03},  // rounds to 4
		{0.0, 1.03},
		{-1.0, 1.03},
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.score)), func(t *testing.T) {
			if got := RiskCoefficient(tt.score); got != tt.want {
				t.Errorf("RiskCoefficient(%v) = %v, want %v", tt.score, got, tt.want)
			}
		})
	}
}

func TestTryCompleteEpicScoring_StoryCascade(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()

	parentID := uuid.New()
	storyID1 := uuid.New()
	storyID2 := uuid.New()
	teamID := uuid.New()
	roleID := uuid.New()

	epicFinalScoreCalled := false
	upsertRoleScoreCalled := false

	mockRepo := &MockRepository{
		GetEpicByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Epic, error) {
			if id == storyID1 {
				return &domain.Epic{
					ID:           storyID1,
					TeamID:       teamID,
					Status:       domain.StatusScoring,
					ParentEpicID: &parentID,
				}, nil
			}
			if id == storyID2 {
				s2Score := 20.0
				return &domain.Epic{
					ID:         storyID2,
					TeamID:     teamID,
					Status:     domain.StatusScored,
					FinalScore: &s2Score,
				}, nil
			}
			if id == parentID {
				return &domain.Epic{
					ID:     parentID,
					TeamID: teamID,
					Status: domain.StatusScoring,
				}, nil
			}
			return nil, sql.ErrNoRows
		},
		GetExpectedScorersCountFunc: func(ctx context.Context, eID, tID uuid.UUID) (int, error) {
			return 1, nil
		},
		GetSubmittedEpicScorersCountFunc: func(ctx context.Context, eID, tID uuid.UUID) (int, error) {
			return 1, nil
		},
		GetDistinctRoleIDsForEpicScoresFunc: func(ctx context.Context, eID uuid.UUID) ([]uuid.UUID, error) {
			return []uuid.UUID{roleID}, nil
		},
		GetEpicScoresByEpicIDAndRoleIDFunc: func(ctx context.Context, eID, rID uuid.UUID) ([]domain.EpicScore, error) {
			return []domain.EpicScore{{UserID: uuid.New(), Score: 10}}, nil
		},
		GetUserByIDFunc: func(ctx context.Context, uID uuid.UUID) (*domain.User, error) {
			return &domain.User{ID: uID, Weight: 100}, nil
		},
		UpsertEpicRoleScoreFunc: func(ctx context.Context, eID, rID uuid.UUID, score float64) error {
			if eID == parentID {
				upsertRoleScoreCalled = true
				if score != 25.0 {
					t.Errorf("parent role score aggregation mismatch, got %v, want 25.0", score)
				}
			}
			return nil
		},
		GetRisksByEpicIDFunc: func(ctx context.Context, eID uuid.UUID) ([]domain.Risk, error) {
			return []domain.Risk{}, nil
		},
		SetEpicFinalScoreFunc: func(ctx context.Context, eID uuid.UUID, score float64) error {
			if eID == parentID {
				epicFinalScoreCalled = true
				if score != 30.0 {
					t.Errorf("parent final score aggregation mismatch, got %v, want 30.0", score)
				}
			}
			return nil
		},
		GetStoriesByEpicIDFunc: func(ctx context.Context, pID uuid.UUID) ([]domain.Epic, error) {
			if pID != parentID {
				return nil, nil
			}
			s2Score := 20.0
			return []domain.Epic{
				{ID: storyID1, Status: domain.StatusScoring, ParentEpicID: &parentID},
				{ID: storyID2, Status: domain.StatusScored, FinalScore: &s2Score, ParentEpicID: &parentID},
			}, nil
		},
		GetEpicRoleScoresByEpicIDFunc: func(ctx context.Context, eID uuid.UUID) ([]domain.EpicRoleScore, error) {
			if eID == storyID1 {
				return []domain.EpicRoleScore{{EpicID: storyID1, RoleID: roleID, WeightedAvg: 10.0}}, nil
			}
			if eID == storyID2 {
				return []domain.EpicRoleScore{{EpicID: storyID2, RoleID: roleID, WeightedAvg: 15.0}}, nil
			}
			return nil, nil
		},
	}

	s := New(logger, mockRepo)
	err := s.TryCompleteEpicScoring(ctx, storyID1)
	if err != nil {
		t.Fatalf("TryCompleteEpicScoring failed: %v", err)
	}

	if !epicFinalScoreCalled {
		t.Error("expected parent SetEpicFinalScore to be called")
	}
	if !upsertRoleScoreCalled {
		t.Error("expected parent UpsertEpicRoleScore to be called")
	}
}

