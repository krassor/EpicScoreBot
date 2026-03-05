package telegram

import (
	"context"

	"EpicScoreBot/internal/models/domain"

	"github.com/google/uuid"
)

// Repository defines the data-access contract required by the telegram bot.
type Repository interface {
	// Users
	CreateUser(ctx context.Context, firstName, lastName, telegramID string, weight int) (*domain.User, error)
	FindUserByTelegramID(ctx context.Context, telegramID string) (*domain.User, error)
	GetUserByID(ctx context.Context, userID uuid.UUID) (*domain.User, error)
	GetUsersByTeamID(ctx context.Context, teamID uuid.UUID) ([]domain.User, error)
	GetAllUsers(ctx context.Context) ([]domain.User, error)
	DeleteUser(ctx context.Context, userID uuid.UUID) error
	UpdateUserName(ctx context.Context, userID uuid.UUID, firstName, lastName string) error
	UpdateUserWeight(ctx context.Context, userID uuid.UUID, weight int) error

	// Roles
	GetAllRoles(ctx context.Context) ([]domain.Role, error)
	GetRoleByID(ctx context.Context, roleID uuid.UUID) (*domain.Role, error)
	GetRoleByUserID(ctx context.Context, userID uuid.UUID) (*domain.Role, error)
	AssignUserRole(ctx context.Context, userID, roleID uuid.UUID) error
	RemoveUserRole(ctx context.Context, userID, roleID uuid.UUID) error

	// Teams
	CreateTeam(ctx context.Context, name, description string) (*domain.Team, error)
	GetTeamByName(ctx context.Context, name string) (*domain.Team, error)
	GetTeamByID(ctx context.Context, teamID uuid.UUID) (*domain.Team, error)
	GetAllTeams(ctx context.Context) ([]domain.Team, error)
	GetTeamsByUserTelegramID(ctx context.Context, telegramID string) ([]domain.Team, error)
	AssignUserTeam(ctx context.Context, userID, teamID uuid.UUID) error
	RemoveUserTeam(ctx context.Context, userID, teamID uuid.UUID) error

	// Epics
	CreateEpic(ctx context.Context, number, name, description string, teamID uuid.UUID) (*domain.Epic, error)
	GetEpicByID(ctx context.Context, epicID uuid.UUID) (*domain.Epic, error)
	GetEpicByNumber(ctx context.Context, number string) (*domain.Epic, error)
	GetEpicsByStatus(ctx context.Context, status domain.Status) ([]domain.Epic, error)
	GetAllEpics(ctx context.Context) ([]domain.Epic, error)
	GetUnscoredEpicsByUser(ctx context.Context, userID, teamID uuid.UUID) ([]domain.Epic, error)
	UpdateEpicStatus(ctx context.Context, epicID uuid.UUID, status domain.Status) error
	DeleteEpic(ctx context.Context, epicID uuid.UUID) error

	// Risks
	CreateRisk(ctx context.Context, description string, epicID uuid.UUID) (*domain.Risk, error)
	GetRisksByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.Risk, error)
	GetRiskByID(ctx context.Context, riskID uuid.UUID) (*domain.Risk, error)
	GetUnscoredRisksByUser(ctx context.Context, userID, epicID uuid.UUID) ([]domain.Risk, error)
	UpdateRiskStatus(ctx context.Context, riskID uuid.UUID, status domain.Status) error
	DeleteRisk(ctx context.Context, riskID uuid.UUID) error

	// Scoring data
	CreateEpicScore(ctx context.Context, epicID, userID, roleID uuid.UUID, score int) error
	HasUserScoredEpic(ctx context.Context, epicID, userID uuid.UUID) (bool, error)
	GetUsersWhoScoredEpic(ctx context.Context, epicID uuid.UUID) ([]domain.User, error)
	GetUsersWhoScoredRisk(ctx context.Context, riskID uuid.UUID) ([]domain.User, error)
	GetEpicRoleScoresByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.EpicRoleScore, error)
	CreateRiskScore(ctx context.Context, riskID, userID uuid.UUID, probability, impact int) error
}

// ScoringService defines the scoring business-logic contract.
type ScoringService interface {
	TryCompleteEpicScoring(ctx context.Context, epicID uuid.UUID) error
	TryCompleteRiskScoring(ctx context.Context, riskID uuid.UUID) error
}

// AIClient defines the AI question-answering contract.
type AIClient interface {
	Ask(ctx context.Context, question string) (string, error)
}
