package services

import (
	"context"
	"errors"

	"EpicScoreBot/internal/models/domain"
	"EpicScoreBot/internal/report"

	"github.com/google/uuid"
)

var (
	ErrUserAlreadyExists = errors.New("user already exists")
	ErrTeamAlreadyExists = errors.New("team already exists")
	ErrEpicAlreadyExists = errors.New("epic already exists")
	// ErrTeamNotFound возвращается BuildCapacityReport/GetCapacityReport,
	// когда команда с указанным team_id не найдена в хранилище.
	ErrTeamNotFound = errors.New("team not found")
)

// Repository defines the data-access contract.
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
	UpdateUserChatID(ctx context.Context, userID uuid.UUID, chatID int64) error
	CreateUserWithRelations(ctx context.Context, user *domain.User, teamUUIDs []uuid.UUID, roleUUIDs []uuid.UUID) error
	UpdateUserWithRelations(ctx context.Context, userID uuid.UUID, firstName, lastName string, weight int, teamUUIDs []uuid.UUID, roleUUIDs []uuid.UUID) error
	GetUserRelations(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, []uuid.UUID, error)
	GetUserTeams(ctx context.Context, userID uuid.UUID) ([]domain.Team, error)
	GetUserRoles(ctx context.Context, userID uuid.UUID) ([]domain.Role, error)

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

	// Team-admins (team-scoped роль admin, назначается только superadmin)
	AssignTeamAdmin(ctx context.Context, userID, teamID, assignedBy uuid.UUID) error
	RemoveTeamAdmin(ctx context.Context, userID, teamID uuid.UUID) error
	GetTeamAdminsByTeamID(ctx context.Context, teamID uuid.UUID) ([]domain.User, error)
	GetTeamIDsByAdminUserID(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
	IsTeamAdmin(ctx context.Context, userID, teamID uuid.UUID) (bool, error)
	IsTeamAdminOfAny(ctx context.Context, userID uuid.UUID) (bool, error)

	// Epics
	CreateEpic(ctx context.Context, number, name, description string, teamID uuid.UUID, year, quarter int, epicType string, evaluatingRoleIDs []uuid.UUID) (*domain.Epic, error)
	CreateStory(ctx context.Context, parentEpicID uuid.UUID, number, name, description string, teamID uuid.UUID, year, quarter int, epicType string, evaluatingRoleIDs []uuid.UUID) (*domain.Epic, error)
	GetEvaluatingRoleIDs(ctx context.Context, epicID uuid.UUID) ([]uuid.UUID, error)
	GetEpicsByTeamYearQuarter(ctx context.Context, teamID uuid.UUID, year, quarter int) ([]domain.Epic, error)
	GetExpectedScorersCount(ctx context.Context, epicID uuid.UUID, teamID uuid.UUID) (int, error)
	GetSubmittedEpicScorersCount(ctx context.Context, epicID uuid.UUID, teamID uuid.UUID) (int, error)
	GetSubmittedRiskScorersCount(ctx context.Context, riskID uuid.UUID, epicID uuid.UUID, teamID uuid.UUID) (int, error)
	GetEpicByID(ctx context.Context, epicID uuid.UUID) (*domain.Epic, error)
	GetEpicByNumber(ctx context.Context, number string) (*domain.Epic, error)
	GetEpicsByStatus(ctx context.Context, status domain.Status) ([]domain.Epic, error)
	GetAllEpics(ctx context.Context) ([]domain.Epic, error)
	GetUnscoredEpicsByUser(ctx context.Context, userID, teamID uuid.UUID) ([]domain.Epic, error)
	UpdateEpicStatus(ctx context.Context, epicID uuid.UUID, status domain.Status) error
	DeleteEpic(ctx context.Context, epicID uuid.UUID) error
	StartEpicScoring(ctx context.Context, epicID uuid.UUID) error
	GetEpicsByTeamIDAndStatus(ctx context.Context, teamID uuid.UUID, status domain.Status) ([]domain.Epic, error)
	GetStoriesByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.Epic, error)
	CountStoriesByEpicID(ctx context.Context, epicID uuid.UUID) (int, error)
	UpdateEpic(ctx context.Context, epic *domain.Epic, newEvaluatingRoles []uuid.UUID, oldNumber string) error
	UpdateStory(ctx context.Context, story *domain.Epic) error
	// GetTeamEpicsOrdered и UpdateEpicSortOrder используются конвейерным
	// планировщиком диаграммы Ганта (internal/gantt) для управления очередью
	// эпиков/сторей; объявлены здесь для единообразия с остальными методами Epic.
	GetTeamEpicsOrdered(ctx context.Context, teamID uuid.UUID) ([]domain.Epic, error)
	UpdateEpicSortOrder(ctx context.Context, epicID uuid.UUID, sortOrder int) error

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
	GetRiskScoresByRiskID(ctx context.Context, riskID uuid.UUID) ([]domain.RiskScore, error)
}

// UserService defines the business logic for users.
type UserService interface {
	CreateUser(ctx context.Context, firstName, lastName, telegramID string, weight int) (*domain.User, error)
	FindUserByTelegramID(ctx context.Context, telegramID string) (*domain.User, error)
	GetUserByID(ctx context.Context, userID uuid.UUID) (*domain.User, error)
	GetUsersByTeamID(ctx context.Context, teamID uuid.UUID) ([]domain.User, error)
	GetAllUsers(ctx context.Context) ([]domain.User, error)
	DeleteUser(ctx context.Context, userID uuid.UUID) error
	UpdateUserName(ctx context.Context, userID uuid.UUID, firstName, lastName string) error
	UpdateUserWeight(ctx context.Context, userID uuid.UUID, weight int) error
	UpdateUserChatID(ctx context.Context, userID uuid.UUID, chatID int64) error
}

// TeamService defines the business logic for teams.
type TeamService interface {
	CreateTeam(ctx context.Context, name, description string) (*domain.Team, error)
	GetTeamByName(ctx context.Context, name string) (*domain.Team, error)
	GetTeamByID(ctx context.Context, teamID uuid.UUID) (*domain.Team, error)
	GetAllTeams(ctx context.Context) ([]domain.Team, error)
	GetTeamsByUserTelegramID(ctx context.Context, telegramID string) ([]domain.Team, error)
	AssignUserTeam(ctx context.Context, userID, teamID uuid.UUID) error
	RemoveUserTeam(ctx context.Context, userID, teamID uuid.UUID) error
}

// EpicService defines the business logic for epics.
type EpicService interface {
	CreateEpic(ctx context.Context, number, name, description string, teamID uuid.UUID, year, quarter int, epicType string, evaluatingRoleIDs []uuid.UUID) (*domain.Epic, error)
	GetEpicByID(ctx context.Context, epicID uuid.UUID) (*domain.Epic, error)
	GetEpicByNumber(ctx context.Context, number string) (*domain.Epic, error)
	GetEpicsByStatus(ctx context.Context, status domain.Status) ([]domain.Epic, error)
	GetAllEpics(ctx context.Context) ([]domain.Epic, error)
	GetUnscoredEpicsByUser(ctx context.Context, userID, teamID uuid.UUID) ([]domain.Epic, error)
	GetUnscoredEpicsForUserAcrossTeams(ctx context.Context, userID uuid.UUID, telegramID string) ([]domain.Epic, error)
	UpdateEpicStatus(ctx context.Context, epicID uuid.UUID, status domain.Status) error
	UpdateEpic(ctx context.Context, id uuid.UUID, req domain.UpdateEpicReq) (*domain.Epic, error)
	UpdateStory(ctx context.Context, id uuid.UUID, req domain.UpdateStoryReq) (*domain.Epic, error)
	DeleteEpic(ctx context.Context, epicID uuid.UUID) error
	GetEpicsByTeamIDAndStatus(ctx context.Context, teamID uuid.UUID, status domain.Status) ([]domain.Epic, error)
	CreateEpicScore(ctx context.Context, epicID, userID, roleID uuid.UUID, score int) error
	HasUserScoredEpic(ctx context.Context, epicID, userID uuid.UUID) (bool, error)
	GetUsersWhoScoredEpic(ctx context.Context, epicID uuid.UUID) ([]domain.User, error)
	GetEpicRoleScoresByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.EpicRoleScore, error)
	GetReportData(ctx context.Context, teamID uuid.UUID, year, quarter int) (*report.ReportData, error)
	// GetCapacityReport агрегирует вместимость/квоты/эпики команды за
	// год/квартал — тот же формат данных, что и GET
	// /api/gantt/reports/capacity (см. BuildCapacityReport).
	GetCapacityReport(ctx context.Context, teamID uuid.UUID, year, quarter int) (*report.CapacityReportResponse, error)
	GetEvaluatingRoleIDs(ctx context.Context, epicID uuid.UUID) ([]uuid.UUID, error)
	GetEpicsByTeamYearQuarter(ctx context.Context, teamID uuid.UUID, year, quarter int) ([]domain.Epic, error)
	CreateStory(ctx context.Context, epicID uuid.UUID, name, description string) (*domain.Epic, error)
	GetStoriesByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.Epic, error)
	DeleteStory(ctx context.Context, storyID uuid.UUID) error
	StartEpicScoring(ctx context.Context, epicID uuid.UUID) error
}

// RiskService defines the business logic for risks.
type RiskService interface {
	CreateRisk(ctx context.Context, description string, epicID uuid.UUID) (*domain.Risk, error)
	GetRisksByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.Risk, error)
	GetRiskByID(ctx context.Context, riskID uuid.UUID) (*domain.Risk, error)
	GetUnscoredRisksByUser(ctx context.Context, userID, epicID uuid.UUID) ([]domain.Risk, error)
	UpdateRiskStatus(ctx context.Context, riskID uuid.UUID, status domain.Status) error
	DeleteRisk(ctx context.Context, riskID uuid.UUID) error
	CreateRiskScore(ctx context.Context, riskID, userID uuid.UUID, probability, impact int) error
	GetRiskScoresByRiskID(ctx context.Context, riskID uuid.UUID) ([]domain.RiskScore, error)
	GetUsersWhoScoredRisk(ctx context.Context, riskID uuid.UUID) ([]domain.User, error)
}

// TeamAdminService defines the business logic for the team-scoped admin role
// (team_admins): назначение/снятие и проверки доступа. Используется
// Telegram-ботом, который резолвит telegram-username в userID через
// UserService.FindUserByTelegramID перед обращением к этим методам. HTTP-слой
// использует telegram_id-ориентированный адаптер (см.
// repositories.TeamAdminAuth) напрямую поверх Repository, минуя этот сервис.
type TeamAdminService interface {
	AssignTeamAdmin(ctx context.Context, userID, teamID, assignedBy uuid.UUID) error
	RemoveTeamAdmin(ctx context.Context, userID, teamID uuid.UUID) error
	GetTeamAdminsByTeamID(ctx context.Context, teamID uuid.UUID) ([]domain.User, error)
	GetTeamIDsByAdminUserID(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
	IsTeamAdmin(ctx context.Context, userID, teamID uuid.UUID) (bool, error)
	IsTeamAdminOfAny(ctx context.Context, userID uuid.UUID) (bool, error)
}

// RoleService defines the business logic for roles.
type RoleService interface {
	GetAllRoles(ctx context.Context) ([]domain.Role, error)
	GetRoleByID(ctx context.Context, roleID uuid.UUID) (*domain.Role, error)
	GetRoleByUserID(ctx context.Context, userID uuid.UUID) (*domain.Role, error)
	AssignUserRole(ctx context.Context, userID, roleID uuid.UUID) error
	RemoveUserRole(ctx context.Context, userID, roleID uuid.UUID) error
}
