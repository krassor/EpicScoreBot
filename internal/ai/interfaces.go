package ai

import (
	"context"

	"EpicScoreBot/internal/models/domain"

	"github.com/google/uuid"
)

// Repository defines the data-access contract required by the AI client.
type Repository interface {
	// Users
	FindUserByTelegramID(ctx context.Context, telegramID string) (*domain.User, error)
	GetUserByID(ctx context.Context, userID uuid.UUID) (*domain.User, error)
	GetUsersByTeamID(ctx context.Context, teamID uuid.UUID) ([]domain.User, error)
	GetUsersByTeamIDAndRoleID(ctx context.Context, teamID, roleID uuid.UUID) ([]domain.User, error)
	GetAllUsers(ctx context.Context) ([]domain.User, error)

	// Roles
	GetAllRoles(ctx context.Context) ([]domain.Role, error)
	GetRoleByID(ctx context.Context, roleID uuid.UUID) (*domain.Role, error)
	GetRoleByName(ctx context.Context, name string) (*domain.Role, error)
	GetRoleByUserID(ctx context.Context, userID uuid.UUID) (*domain.Role, error)

	// Teams
	GetTeamByName(ctx context.Context, name string) (*domain.Team, error)
	GetTeamByID(ctx context.Context, teamID uuid.UUID) (*domain.Team, error)
	GetAllTeams(ctx context.Context) ([]domain.Team, error)
	GetTeamsByUserTelegramID(ctx context.Context, telegramID string) ([]domain.Team, error)

	// Epics
	GetEpicByID(ctx context.Context, epicID uuid.UUID) (*domain.Epic, error)
	GetEpicByNumber(ctx context.Context, number string) (*domain.Epic, error)
	GetEpicsByTeamIDAndStatus(ctx context.Context, teamID uuid.UUID, status domain.Status) ([]domain.Epic, error)
	GetEpicsByStatus(ctx context.Context, status domain.Status) ([]domain.Epic, error)
	GetAllEpics(ctx context.Context) ([]domain.Epic, error)
	GetUnscoredEpicsByUser(ctx context.Context, userID, teamID uuid.UUID) ([]domain.Epic, error)

	// Risks
	GetRisksByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.Risk, error)
	GetRiskByID(ctx context.Context, riskID uuid.UUID) (*domain.Risk, error)
	GetUnscoredRisksByUser(ctx context.Context, userID, epicID uuid.UUID) ([]domain.Risk, error)

	// Scoring data
	GetEpicScoresByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.EpicScore, error)
	GetEpicScoresByEpicIDAndRoleID(ctx context.Context, epicID, roleID uuid.UUID) ([]domain.EpicScore, error)
	GetEpicRoleScoresByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.EpicRoleScore, error)
	HasUserScoredEpic(ctx context.Context, epicID, userID uuid.UUID) (bool, error)
	HasUserScoredRisk(ctx context.Context, riskID, userID uuid.UUID) (bool, error)
	GetUsersWhoScoredEpic(ctx context.Context, epicID uuid.UUID) ([]domain.User, error)
	GetUsersWhoScoredRisk(ctx context.Context, riskID uuid.UUID) ([]domain.User, error)
	GetRiskScoresByRiskID(ctx context.Context, riskID uuid.UUID) ([]domain.RiskScore, error)
}
