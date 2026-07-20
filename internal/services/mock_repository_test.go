package services

import (
	"context"

	"EpicScoreBot/internal/models/domain"

	"github.com/google/uuid"
)

// MockRepository — реализация интерфейса Repository для тестирования.
// Каждый метод перенаправляет вызов соответствующему полю-функции,
// что позволяет настраивать поведение репозитория в отдельных тестах.
type MockRepository struct {
	// Users
	CreateUserFunc             func(ctx context.Context, firstName, lastName, telegramID string, weight int) (*domain.User, error)
	FindUserByTelegramIDFunc   func(ctx context.Context, telegramID string) (*domain.User, error)
	GetUserByIDFunc           func(ctx context.Context, userID uuid.UUID) (*domain.User, error)
	GetUsersByTeamIDFunc       func(ctx context.Context, teamID uuid.UUID) ([]domain.User, error)
	GetAllUsersFunc            func(ctx context.Context) ([]domain.User, error)
	DeleteUserFunc             func(ctx context.Context, userID uuid.UUID) error
	UpdateUserNameFunc         func(ctx context.Context, userID uuid.UUID, firstName, lastName string) error
	UpdateUserWeightFunc       func(ctx context.Context, userID uuid.UUID, weight int) error
	UpdateUserChatIDFunc       func(ctx context.Context, userID uuid.UUID, chatID int64) error
	CreateUserWithRelationsFunc func(ctx context.Context, user *domain.User, teamUUIDs []uuid.UUID, roleUUIDs []uuid.UUID) error
	UpdateUserWithRelationsFunc func(ctx context.Context, userID uuid.UUID, firstName, lastName string, weight int, teamUUIDs []uuid.UUID, roleUUIDs []uuid.UUID) error
	GetUserRelationsFunc        func(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, []uuid.UUID, error)
	GetUserTeamsFunc            func(ctx context.Context, userID uuid.UUID) ([]domain.Team, error)
	GetUserRolesFunc            func(ctx context.Context, userID uuid.UUID) ([]domain.Role, error)

	// Roles
	GetAllRolesFunc            func(ctx context.Context) ([]domain.Role, error)
	GetRoleByIDFunc            func(ctx context.Context, roleID uuid.UUID) (*domain.Role, error)
	GetRoleByUserIDFunc        func(ctx context.Context, userID uuid.UUID) (*domain.Role, error)
	AssignUserRoleFunc         func(ctx context.Context, userID, roleID uuid.UUID) error
	RemoveUserRoleFunc         func(ctx context.Context, userID, roleID uuid.UUID) error

	// Teams
	CreateTeamFunc             func(ctx context.Context, name, description string) (*domain.Team, error)
	GetTeamByNameFunc          func(ctx context.Context, name string) (*domain.Team, error)
	GetTeamByIDFunc            func(ctx context.Context, teamID uuid.UUID) (*domain.Team, error)
	GetAllTeamsFunc            func(ctx context.Context) ([]domain.Team, error)
	GetTeamsByUserTelegramIDFunc func(ctx context.Context, telegramID string) ([]domain.Team, error)
	AssignUserTeamFunc         func(ctx context.Context, userID, teamID uuid.UUID) error
	RemoveUserTeamFunc         func(ctx context.Context, userID, teamID uuid.UUID) error

	// Epics
	CreateEpicFunc             func(ctx context.Context, number, name, description string, teamID uuid.UUID, year, quarter int, epicType string, evaluatingRoleIDs []uuid.UUID) (*domain.Epic, error)
	CreateStoryFunc            func(ctx context.Context, parentEpicID uuid.UUID, number, name, description string, teamID uuid.UUID, year, quarter int, epicType string, evaluatingRoleIDs []uuid.UUID) (*domain.Epic, error)
	GetEpicsByTeamYearQuarterFunc func(ctx context.Context, teamID uuid.UUID, year, quarter int) ([]domain.Epic, error)
	GetEpicByIDFunc            func(ctx context.Context, epicID uuid.UUID) (*domain.Epic, error)
	GetEpicByNumberFunc        func(ctx context.Context, number string) (*domain.Epic, error)
	GetEpicsByStatusFunc       func(ctx context.Context, status domain.Status) ([]domain.Epic, error)
	GetAllEpicsFunc            func(ctx context.Context) ([]domain.Epic, error)
	GetUnscoredEpicsByUserFunc func(ctx context.Context, userID, teamID uuid.UUID) ([]domain.Epic, error)
	UpdateEpicStatusFunc       func(ctx context.Context, epicID uuid.UUID, status domain.Status) error
	DeleteEpicFunc             func(ctx context.Context, epicID uuid.UUID) error
	StartEpicScoringFunc       func(ctx context.Context, epicID uuid.UUID) error
	GetEpicsByTeamIDAndStatusFunc func(ctx context.Context, teamID uuid.UUID, status domain.Status) ([]domain.Epic, error)
	GetStoriesByEpicIDFunc        func(ctx context.Context, epicID uuid.UUID) ([]domain.Epic, error)
	CountStoriesByEpicIDFunc      func(ctx context.Context, epicID uuid.UUID) (int, error)

	// Risks
	CreateRiskFunc             func(ctx context.Context, description string, epicID uuid.UUID) (*domain.Risk, error)
	GetRisksByEpicIDFunc       func(ctx context.Context, epicID uuid.UUID) ([]domain.Risk, error)
	GetRiskByIDFunc            func(ctx context.Context, riskID uuid.UUID) (*domain.Risk, error)
	GetUnscoredRisksByUserFunc func(ctx context.Context, userID, epicID uuid.UUID) ([]domain.Risk, error)
	UpdateRiskStatusFunc       func(ctx context.Context, riskID uuid.UUID, status domain.Status) error
	DeleteRiskFunc             func(ctx context.Context, riskID uuid.UUID) error

	// Scoring data
	CreateEpicScoreFunc        func(ctx context.Context, epicID, userID, roleID uuid.UUID, score int) error
	HasUserScoredEpicFunc      func(ctx context.Context, epicID, userID uuid.UUID) (bool, error)
	GetUsersWhoScoredEpicFunc  func(ctx context.Context, epicID uuid.UUID) ([]domain.User, error)
	GetUsersWhoScoredRiskFunc  func(ctx context.Context, riskID uuid.UUID) ([]domain.User, error)
	GetEpicRoleScoresByEpicIDFunc func(ctx context.Context, epicID uuid.UUID) ([]domain.EpicRoleScore, error)
	CreateRiskScoreFunc        func(ctx context.Context, riskID, userID uuid.UUID, probability, impact int) error
	GetRiskScoresByRiskIDFunc  func(ctx context.Context, riskID uuid.UUID) ([]domain.RiskScore, error)
}

// Ensure MockRepository implements Repository interface
var _ Repository = (*MockRepository)(nil)

func (m *MockRepository) CreateUser(ctx context.Context, firstName, lastName, telegramID string, weight int) (*domain.User, error) {
	if m.CreateUserFunc != nil {
		return m.CreateUserFunc(ctx, firstName, lastName, telegramID, weight)
	}
	return nil, nil
}

func (m *MockRepository) FindUserByTelegramID(ctx context.Context, telegramID string) (*domain.User, error) {
	if m.FindUserByTelegramIDFunc != nil {
		return m.FindUserByTelegramIDFunc(ctx, telegramID)
	}
	return nil, nil
}

func (m *MockRepository) GetUserByID(ctx context.Context, userID uuid.UUID) (*domain.User, error) {
	if m.GetUserByIDFunc != nil {
		return m.GetUserByIDFunc(ctx, userID)
	}
	return nil, nil
}

func (m *MockRepository) GetUsersByTeamID(ctx context.Context, teamID uuid.UUID) ([]domain.User, error) {
	if m.GetUsersByTeamIDFunc != nil {
		return m.GetUsersByTeamIDFunc(ctx, teamID)
	}
	return nil, nil
}

func (m *MockRepository) GetAllUsers(ctx context.Context) ([]domain.User, error) {
	if m.GetAllUsersFunc != nil {
		return m.GetAllUsersFunc(ctx)
	}
	return nil, nil
}

func (m *MockRepository) DeleteUser(ctx context.Context, userID uuid.UUID) error {
	if m.DeleteUserFunc != nil {
		return m.DeleteUserFunc(ctx, userID)
	}
	return nil
}

func (m *MockRepository) UpdateUserName(ctx context.Context, userID uuid.UUID, firstName, lastName string) error {
	if m.UpdateUserNameFunc != nil {
		return m.UpdateUserNameFunc(ctx, userID, firstName, lastName)
	}
	return nil
}

func (m *MockRepository) UpdateUserWeight(ctx context.Context, userID uuid.UUID, weight int) error {
	if m.UpdateUserWeightFunc != nil {
		return m.UpdateUserWeightFunc(ctx, userID, weight)
	}
	return nil
}

func (m *MockRepository) UpdateUserChatID(ctx context.Context, userID uuid.UUID, chatID int64) error {
	if m.UpdateUserChatIDFunc != nil {
		return m.UpdateUserChatIDFunc(ctx, userID, chatID)
	}
	return nil
}

func (m *MockRepository) CreateUserWithRelations(ctx context.Context, user *domain.User, teamUUIDs []uuid.UUID, roleUUIDs []uuid.UUID) error {
	if m.CreateUserWithRelationsFunc != nil {
		return m.CreateUserWithRelationsFunc(ctx, user, teamUUIDs, roleUUIDs)
	}
	return nil
}

func (m *MockRepository) UpdateUserWithRelations(ctx context.Context, userID uuid.UUID, firstName, lastName string, weight int, teamUUIDs []uuid.UUID, roleUUIDs []uuid.UUID) error {
	if m.UpdateUserWithRelationsFunc != nil {
		return m.UpdateUserWithRelationsFunc(ctx, userID, firstName, lastName, weight, teamUUIDs, roleUUIDs)
	}
	return nil
}

func (m *MockRepository) GetUserRelations(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, []uuid.UUID, error) {
	if m.GetUserRelationsFunc != nil {
		return m.GetUserRelationsFunc(ctx, userID)
	}
	return nil, nil, nil
}

func (m *MockRepository) GetUserTeams(ctx context.Context, userID uuid.UUID) ([]domain.Team, error) {
	if m.GetUserTeamsFunc != nil {
		return m.GetUserTeamsFunc(ctx, userID)
	}
	return nil, nil
}

func (m *MockRepository) GetUserRoles(ctx context.Context, userID uuid.UUID) ([]domain.Role, error) {
	if m.GetUserRolesFunc != nil {
		return m.GetUserRolesFunc(ctx, userID)
	}
	return nil, nil
}

func (m *MockRepository) GetAllRoles(ctx context.Context) ([]domain.Role, error) {
	if m.GetAllRolesFunc != nil {
		return m.GetAllRolesFunc(ctx)
	}
	return nil, nil
}

func (m *MockRepository) GetRoleByID(ctx context.Context, roleID uuid.UUID) (*domain.Role, error) {
	if m.GetRoleByIDFunc != nil {
		return m.GetRoleByIDFunc(ctx, roleID)
	}
	return nil, nil
}

func (m *MockRepository) GetRoleByUserID(ctx context.Context, userID uuid.UUID) (*domain.Role, error) {
	if m.GetRoleByUserIDFunc != nil {
		return m.GetRoleByUserIDFunc(ctx, userID)
	}
	return nil, nil
}

func (m *MockRepository) AssignUserRole(ctx context.Context, userID, roleID uuid.UUID) error {
	if m.AssignUserRoleFunc != nil {
		return m.AssignUserRoleFunc(ctx, userID, roleID)
	}
	return nil
}

func (m *MockRepository) RemoveUserRole(ctx context.Context, userID, roleID uuid.UUID) error {
	if m.RemoveUserRoleFunc != nil {
		return m.RemoveUserRoleFunc(ctx, userID, roleID)
	}
	return nil
}

func (m *MockRepository) CreateTeam(ctx context.Context, name, description string) (*domain.Team, error) {
	if m.CreateTeamFunc != nil {
		return m.CreateTeamFunc(ctx, name, description)
	}
	return nil, nil
}

func (m *MockRepository) GetTeamByName(ctx context.Context, name string) (*domain.Team, error) {
	if m.GetTeamByNameFunc != nil {
		return m.GetTeamByNameFunc(ctx, name)
	}
	return nil, nil
}

func (m *MockRepository) GetTeamByID(ctx context.Context, teamID uuid.UUID) (*domain.Team, error) {
	if m.GetTeamByIDFunc != nil {
		return m.GetTeamByIDFunc(ctx, teamID)
	}
	return nil, nil
}

func (m *MockRepository) GetAllTeams(ctx context.Context) ([]domain.Team, error) {
	if m.GetAllTeamsFunc != nil {
		return m.GetAllTeamsFunc(ctx)
	}
	return nil, nil
}

func (m *MockRepository) GetTeamsByUserTelegramID(ctx context.Context, telegramID string) ([]domain.Team, error) {
	if m.GetTeamsByUserTelegramIDFunc != nil {
		return m.GetTeamsByUserTelegramIDFunc(ctx, telegramID)
	}
	return nil, nil
}

func (m *MockRepository) AssignUserTeam(ctx context.Context, userID, teamID uuid.UUID) error {
	if m.AssignUserTeamFunc != nil {
		return m.AssignUserTeamFunc(ctx, userID, teamID)
	}
	return nil
}

func (m *MockRepository) RemoveUserTeam(ctx context.Context, userID, teamID uuid.UUID) error {
	if m.RemoveUserTeamFunc != nil {
		return m.RemoveUserTeamFunc(ctx, userID, teamID)
	}
	return nil
}

func (m *MockRepository) CreateEpic(ctx context.Context, number, name, description string, teamID uuid.UUID, year, quarter int, epicType string, evaluatingRoleIDs []uuid.UUID) (*domain.Epic, error) {
	if m.CreateEpicFunc != nil {
		return m.CreateEpicFunc(ctx, number, name, description, teamID, year, quarter, epicType, evaluatingRoleIDs)
	}
	return nil, nil
}

func (m *MockRepository) CreateStory(ctx context.Context, parentEpicID uuid.UUID, number, name, description string, teamID uuid.UUID, year, quarter int, epicType string, evaluatingRoleIDs []uuid.UUID) (*domain.Epic, error) {
	if m.CreateStoryFunc != nil {
		return m.CreateStoryFunc(ctx, parentEpicID, number, name, description, teamID, year, quarter, epicType, evaluatingRoleIDs)
	}
	return nil, nil
}

func (m *MockRepository) GetEpicByID(ctx context.Context, epicID uuid.UUID) (*domain.Epic, error) {
	if m.GetEpicByIDFunc != nil {
		return m.GetEpicByIDFunc(ctx, epicID)
	}
	return nil, nil
}

func (m *MockRepository) GetEpicByNumber(ctx context.Context, number string) (*domain.Epic, error) {
	if m.GetEpicByNumberFunc != nil {
		return m.GetEpicByNumberFunc(ctx, number)
	}
	return nil, nil
}

func (m *MockRepository) GetEpicsByStatus(ctx context.Context, status domain.Status) ([]domain.Epic, error) {
	if m.GetEpicsByStatusFunc != nil {
		return m.GetEpicsByStatusFunc(ctx, status)
	}
	return nil, nil
}

func (m *MockRepository) GetAllEpics(ctx context.Context) ([]domain.Epic, error) {
	if m.GetAllEpicsFunc != nil {
		return m.GetAllEpicsFunc(ctx)
	}
	return nil, nil
}

func (m *MockRepository) GetUnscoredEpicsByUser(ctx context.Context, userID, teamID uuid.UUID) ([]domain.Epic, error) {
	if m.GetUnscoredEpicsByUserFunc != nil {
		return m.GetUnscoredEpicsByUserFunc(ctx, userID, teamID)
	}
	return nil, nil
}

func (m *MockRepository) UpdateEpicStatus(ctx context.Context, epicID uuid.UUID, status domain.Status) error {
	if m.UpdateEpicStatusFunc != nil {
		return m.UpdateEpicStatusFunc(ctx, epicID, status)
	}
	return nil
}

func (m *MockRepository) DeleteEpic(ctx context.Context, epicID uuid.UUID) error {
	if m.DeleteEpicFunc != nil {
		return m.DeleteEpicFunc(ctx, epicID)
	}
	return nil
}

func (m *MockRepository) StartEpicScoring(ctx context.Context, epicID uuid.UUID) error {
	if m.StartEpicScoringFunc != nil {
		return m.StartEpicScoringFunc(ctx, epicID)
	}
	return nil
}

func (m *MockRepository) GetEpicsByTeamIDAndStatus(ctx context.Context, teamID uuid.UUID, status domain.Status) ([]domain.Epic, error) {
	if m.GetEpicsByTeamIDAndStatusFunc != nil {
		return m.GetEpicsByTeamIDAndStatusFunc(ctx, teamID, status)
	}
	return nil, nil
}

func (m *MockRepository) CreateRisk(ctx context.Context, description string, epicID uuid.UUID) (*domain.Risk, error) {
	if m.CreateRiskFunc != nil {
		return m.CreateRiskFunc(ctx, description, epicID)
	}
	return nil, nil
}

func (m *MockRepository) GetRisksByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.Risk, error) {
	if m.GetRisksByEpicIDFunc != nil {
		return m.GetRisksByEpicIDFunc(ctx, epicID)
	}
	return nil, nil
}

func (m *MockRepository) GetRiskByID(ctx context.Context, riskID uuid.UUID) (*domain.Risk, error) {
	if m.GetRiskByIDFunc != nil {
		return m.GetRiskByIDFunc(ctx, riskID)
	}
	return nil, nil
}

func (m *MockRepository) GetUnscoredRisksByUser(ctx context.Context, userID, epicID uuid.UUID) ([]domain.Risk, error) {
	if m.GetUnscoredRisksByUserFunc != nil {
		return m.GetUnscoredRisksByUserFunc(ctx, userID, epicID)
	}
	return nil, nil
}

func (m *MockRepository) UpdateRiskStatus(ctx context.Context, riskID uuid.UUID, status domain.Status) error {
	if m.UpdateRiskStatusFunc != nil {
		return m.UpdateRiskStatusFunc(ctx, riskID, status)
	}
	return nil
}

func (m *MockRepository) DeleteRisk(ctx context.Context, riskID uuid.UUID) error {
	if m.DeleteRiskFunc != nil {
		return m.DeleteRiskFunc(ctx, riskID)
	}
	return nil
}

func (m *MockRepository) CreateEpicScore(ctx context.Context, epicID, userID, roleID uuid.UUID, score int) error {
	if m.CreateEpicScoreFunc != nil {
		return m.CreateEpicScoreFunc(ctx, epicID, userID, roleID, score)
	}
	return nil
}

func (m *MockRepository) HasUserScoredEpic(ctx context.Context, epicID, userID uuid.UUID) (bool, error) {
	if m.HasUserScoredEpicFunc != nil {
		return m.HasUserScoredEpicFunc(ctx, epicID, userID)
	}
	return false, nil
}

func (m *MockRepository) GetUsersWhoScoredEpic(ctx context.Context, epicID uuid.UUID) ([]domain.User, error) {
	if m.GetUsersWhoScoredEpicFunc != nil {
		return m.GetUsersWhoScoredEpicFunc(ctx, epicID)
	}
	return nil, nil
}

func (m *MockRepository) GetUsersWhoScoredRisk(ctx context.Context, riskID uuid.UUID) ([]domain.User, error) {
	if m.GetUsersWhoScoredRiskFunc != nil {
		return m.GetUsersWhoScoredRiskFunc(ctx, riskID)
	}
	return nil, nil
}

func (m *MockRepository) GetEpicRoleScoresByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.EpicRoleScore, error) {
	if m.GetEpicRoleScoresByEpicIDFunc != nil {
		return m.GetEpicRoleScoresByEpicIDFunc(ctx, epicID)
	}
	return nil, nil
}

func (m *MockRepository) CreateRiskScore(ctx context.Context, riskID, userID uuid.UUID, probability, impact int) error {
	if m.CreateRiskScoreFunc != nil {
		return m.CreateRiskScoreFunc(ctx, riskID, userID, probability, impact)
	}
	return nil
}

func (m *MockRepository) GetRiskScoresByRiskID(ctx context.Context, riskID uuid.UUID) ([]domain.RiskScore, error) {
	if m.GetRiskScoresByRiskIDFunc != nil {
		return m.GetRiskScoresByRiskIDFunc(ctx, riskID)
	}
	return nil, nil
}

func (m *MockRepository) GetEvaluatingRoleIDs(ctx context.Context, epicID uuid.UUID) ([]uuid.UUID, error) {
	return nil, nil
}

func (m *MockRepository) GetEpicsByTeamYearQuarter(ctx context.Context, teamID uuid.UUID, year, quarter int) ([]domain.Epic, error) {
	if m.GetEpicsByTeamYearQuarterFunc != nil {
		return m.GetEpicsByTeamYearQuarterFunc(ctx, teamID, year, quarter)
	}
	return nil, nil
}

func (m *MockRepository) GetExpectedScorersCount(ctx context.Context, epicID uuid.UUID, teamID uuid.UUID) (int, error) {
	return 0, nil
}

func (m *MockRepository) GetSubmittedEpicScorersCount(ctx context.Context, epicID uuid.UUID, teamID uuid.UUID) (int, error) {
	return 0, nil
}

func (m *MockRepository) GetSubmittedRiskScorersCount(ctx context.Context, riskID uuid.UUID, epicID uuid.UUID, teamID uuid.UUID) (int, error) {
	return 0, nil
}

func (m *MockRepository) GetStoriesByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.Epic, error) {
	if m.GetStoriesByEpicIDFunc != nil {
		return m.GetStoriesByEpicIDFunc(ctx, epicID)
	}
	return nil, nil
}

func (m *MockRepository) CountStoriesByEpicID(ctx context.Context, epicID uuid.UUID) (int, error) {
	if m.CountStoriesByEpicIDFunc != nil {
		return m.CountStoriesByEpicIDFunc(ctx, epicID)
	}
	return 0, nil
}

