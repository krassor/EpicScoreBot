package handlers

import (
	"EpicScoreBot/internal/models/domain"
	"context"
	"time"

	"github.com/google/uuid"
)

// GanttService defines the business-logic contract used by handlers.
type GanttService interface {
	GenerateTasksForEpic(ctx context.Context, epicID uuid.UUID, startDate time.Time) ([]domain.GanttTask, error)
	ReorderTask(ctx context.Context, taskID uuid.UUID, newSortOrder int) ([]domain.GanttTask, error)
	// ReorderEpic меняет позицию топ-эпика в очереди конвейерного планировщика команды.
	ReorderEpic(ctx context.Context, epicID uuid.UUID, newSortOrder int) ([]domain.GanttTask, error)
	// ReorderStory меняет позицию стори в очереди сторей родительского эпика.
	ReorderStory(ctx context.Context, storyID uuid.UUID, newSortOrder int) ([]domain.GanttTask, error)
	// SetTaskProgress выставляет прогресс листовой (ролевой) задачи и
	// автоматически фиксирует факт завершения при достижении 100%.
	SetTaskProgress(ctx context.Context, taskID uuid.UUID, progress float64) ([]domain.GanttTask, error)
	// SetTaskStartOffset выставляет смещение (lead/lag, в днях) старта
	// листовой (ролевой) задачи относительно окончания предыдущей ролевой
	// группы внутри той же стори.
	SetTaskStartOffset(ctx context.Context, taskID uuid.UUID, offsetDays int) ([]domain.GanttTask, error)
	GetTeamTasks(ctx context.Context, teamID uuid.UUID) ([]domain.GanttTask, error)
}

// Repository defines the data-access contract used by handlers.
type Repository interface {
	// Teams
	CreateTeam(ctx context.Context, name, description string) (*domain.Team, error)
	GetAllTeams(ctx context.Context) ([]domain.Team, error)
	GetTeamsByUserTelegramID(ctx context.Context, telegramID string) ([]domain.Team, error)
	GetTeamByID(ctx context.Context, teamID uuid.UUID) (*domain.Team, error)
	GetTeamByName(ctx context.Context, name string) (*domain.Team, error)

	CreateUser(ctx context.Context, firstName, lastName string, telegramID string, weight int) (*domain.User, error)
	CreateUserWithRelations(ctx context.Context, user *domain.User, teamUUIDs []uuid.UUID, roleUUIDs []uuid.UUID) error
	UpdateUserWithRelations(ctx context.Context, userID uuid.UUID, firstName, lastName string, weight int, teamUUIDs []uuid.UUID, roleUUIDs []uuid.UUID) error
	GetUserRelations(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, []uuid.UUID, error)
	GetUserTeams(ctx context.Context, userID uuid.UUID) ([]domain.Team, error)
	GetUserRoles(ctx context.Context, userID uuid.UUID) ([]domain.Role, error)
	GetAllUsers(ctx context.Context) ([]domain.User, error)
	BulkCreateUsers(ctx context.Context, users []domain.User, teamID *uuid.UUID, roleID *uuid.UUID) error
	FindUserByTelegramID(ctx context.Context, telegramID string) (*domain.User, error)
	GetUserByID(ctx context.Context, userID uuid.UUID) (*domain.User, error)
	GetUsersByTeamID(ctx context.Context, teamID uuid.UUID) ([]domain.User, error)
	AssignUserTeam(ctx context.Context, userID, teamID uuid.UUID) error
	RemoveUserTeam(ctx context.Context, userID, teamID uuid.UUID) error
	AssignUserRole(ctx context.Context, userID, roleID uuid.UUID) error
	RemoveUserRole(ctx context.Context, userID, roleID uuid.UUID) error
	UpdateUserWeight(ctx context.Context, userID uuid.UUID, weight int) error
	DeleteUser(ctx context.Context, userID uuid.UUID) error

	// Epics
	CreateEpic(ctx context.Context, number, name, description string, teamID uuid.UUID, year, quarter int, epicType string, evaluatingRoleIDs []uuid.UUID) (*domain.Epic, error)
	GetEvaluatingRoleIDs(ctx context.Context, epicID uuid.UUID) ([]uuid.UUID, error)
	GetEpicsByTeamYearQuarter(ctx context.Context, teamID uuid.UUID, year, quarter int) ([]domain.Epic, error)
	GetExpectedScorersCount(ctx context.Context, epicID uuid.UUID, teamID uuid.UUID) (int, error)
	GetSubmittedEpicScorersCount(ctx context.Context, epicID uuid.UUID, teamID uuid.UUID) (int, error)
	GetSubmittedRiskScorersCount(ctx context.Context, riskID uuid.UUID, epicID uuid.UUID, teamID uuid.UUID) (int, error)
	GetEpicByID(ctx context.Context, epicID uuid.UUID) (*domain.Epic, error)
	GetEpicByNumber(ctx context.Context, number string) (*domain.Epic, error)
	GetEpicsByTeamIDAndStatus(ctx context.Context, teamID uuid.UUID, status domain.Status) ([]domain.Epic, error)
	UpdateEpicStatus(ctx context.Context, epicID uuid.UUID, status domain.Status) error
	StartEpicScoring(ctx context.Context, epicID uuid.UUID) error
	DeleteEpic(ctx context.Context, epicID uuid.UUID) error
	GetAllEpics(ctx context.Context) ([]domain.Epic, error)
	GetStoriesByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.Epic, error)
	CountStoriesByEpicID(ctx context.Context, epicID uuid.UUID) (int, error)
	UpdateEpic(ctx context.Context, epic *domain.Epic, newEvaluatingRoles []uuid.UUID, oldNumber string) error
	UpdateStory(ctx context.Context, story *domain.Epic) error
	CreateStory(ctx context.Context, parentEpicID uuid.UUID, number, name, description string, teamID uuid.UUID, year, quarter int, epicType string, evaluatingRoleIDs []uuid.UUID) (*domain.Epic, error)

	// Risks
	CreateRisk(ctx context.Context, description string, epicID uuid.UUID) (*domain.Risk, error)
	GetRiskByID(ctx context.Context, riskID uuid.UUID) (*domain.Risk, error)
	GetRisksByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.Risk, error)
	UpdateRisk(ctx context.Context, riskID uuid.UUID, description string) error
	DeleteRisk(ctx context.Context, riskID uuid.UUID) error

	// Roles
	GetAllRoles(ctx context.Context) ([]domain.Role, error)
	GetRoleByID(ctx context.Context, roleID uuid.UUID) (*domain.Role, error)
	GetRoleByName(ctx context.Context, name string) (*domain.Role, error)
	GetRoleByUserID(ctx context.Context, userID uuid.UUID) (*domain.Role, error)

	// Scoring
	CreateEpicScore(ctx context.Context, epicID, userID, roleID uuid.UUID, score int) error
	CreateRiskScore(ctx context.Context, riskID, userID uuid.UUID, probability, impact int) error
	GetEpicScoresByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.EpicScore, error)
	GetEpicScoresByUserID(ctx context.Context, userID uuid.UUID) ([]domain.EpicScore, error)
	GetEpicRoleScoresByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.EpicRoleScore, error)
	GetRiskScoresByRiskID(ctx context.Context, riskID uuid.UUID) ([]domain.RiskScore, error)
	GetRiskScoresByUserID(ctx context.Context, userID uuid.UUID) ([]domain.RiskScore, error)
	GetUsersWhoScoredEpic(ctx context.Context, epicID uuid.UUID) ([]domain.User, error)
	GetUsersWhoScoredRisk(ctx context.Context, riskID uuid.UUID) ([]domain.User, error)
	CountTeamMembers(ctx context.Context, teamID uuid.UUID) (int, error)
	CountEpicScores(ctx context.Context, epicID uuid.UUID) (int, error)
	CountRiskScores(ctx context.Context, riskID uuid.UUID) (int, error)

	// Gantt tasks
	GetGanttTasksByTeamID(ctx context.Context, teamID uuid.UUID) ([]domain.GanttTask, error)
	GetGanttTaskByID(ctx context.Context, taskID uuid.UUID) (*domain.GanttTask, error)
	UpdateGanttTaskProgress(ctx context.Context, taskID uuid.UUID, progress float64) error
	DeleteGanttTasksByEpicID(ctx context.Context, epicID uuid.UUID) error
}

// ScoringService defines the contract for epic and risk scoring completion checks.
type ScoringService interface {
	TryCompleteEpicScoring(ctx context.Context, epicID uuid.UUID) error
	TryCompleteRiskScoring(ctx context.Context, riskID uuid.UUID) error
	// SetManualFinalScore позволяет вручную переопределить итоговую оценку
	// (final_score) уже оцененного эпика/стори (статус SCORED), с каскадным
	// пересчётом родительского эпика при необходимости.
	SetManualFinalScore(ctx context.Context, epicID uuid.UUID, finalScore float64) (*domain.Epic, error)
}

// AIClient defines the contract for interacting with the AI assistant.
type AIClient interface {
	Ask(ctx context.Context, question string) (string, error)
}
