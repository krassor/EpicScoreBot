package scoring

import (
	"context"

	"EpicScoreBot/internal/models/domain"

	"github.com/google/uuid"
)

// Repository defines the data-access contract required by the scoring service.
type Repository interface {
	GetEpicScoresByEpicIDAndRoleID(ctx context.Context, epicID, roleID uuid.UUID) ([]domain.EpicScore, error)
	GetUserByID(ctx context.Context, userID uuid.UUID) (*domain.User, error)
	GetRiskScoresByRiskID(ctx context.Context, riskID uuid.UUID) ([]domain.RiskScore, error)
	GetRiskByID(ctx context.Context, riskID uuid.UUID) (*domain.Risk, error)
	GetEpicByID(ctx context.Context, epicID uuid.UUID) (*domain.Epic, error)
	CountTeamMembers(ctx context.Context, teamID uuid.UUID) (int, error)
	CountRiskScores(ctx context.Context, riskID uuid.UUID) (int, error)
	SetRiskWeightedScore(ctx context.Context, riskID uuid.UUID, score float64) error
	CountEpicScores(ctx context.Context, epicID uuid.UUID) (int, error)
	GetDistinctRoleIDsForEpicScores(ctx context.Context, epicID uuid.UUID) ([]uuid.UUID, error)
	UpsertEpicRoleScore(ctx context.Context, epicID, roleID uuid.UUID, weightedAvg float64) error
	GetRisksByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.Risk, error)
	SetEpicFinalScore(ctx context.Context, epicID uuid.UUID, score float64) error
}
