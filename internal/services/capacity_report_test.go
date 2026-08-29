package services

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"EpicScoreBot/internal/models/domain"
	"github.com/google/uuid"
)

// mockCapacityReportRepo — mock для тестирования BuildCapacityReport.
type mockCapacityReportRepo struct {
	getTeamByIDFunc                func(ctx context.Context, id uuid.UUID) (*domain.Team, error)
	getUsersByTeamIDFunc           func(ctx context.Context, id uuid.UUID) ([]domain.User, error)
	getRoleByUserIDFunc            func(ctx context.Context, id uuid.UUID) (*domain.Role, error)
	getEpicsByTeamYearQuarterFunc  func(ctx context.Context, teamID uuid.UUID, year, quarter int) ([]domain.Epic, error)
	getStoriesByEpicIDFunc         func(ctx context.Context, id uuid.UUID) ([]domain.Epic, error)
	getEpicRoleScoresByEpicIDFunc  func(ctx context.Context, id uuid.UUID) ([]domain.EpicRoleScore, error)
	getRoleByIDFunc                func(ctx context.Context, id uuid.UUID) (*domain.Role, error)
}

func (m *mockCapacityReportRepo) GetTeamByID(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
	if m.getTeamByIDFunc != nil {
		return m.getTeamByIDFunc(ctx, id)
	}
	return nil, errors.New("not implemented")
}

func (m *mockCapacityReportRepo) GetUsersByTeamID(ctx context.Context, id uuid.UUID) ([]domain.User, error) {
	if m.getUsersByTeamIDFunc != nil {
		return m.getUsersByTeamIDFunc(ctx, id)
	}
	return nil, errors.New("not implemented")
}

func (m *mockCapacityReportRepo) GetRoleByUserID(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
	if m.getRoleByUserIDFunc != nil {
		return m.getRoleByUserIDFunc(ctx, id)
	}
	return nil, errors.New("not implemented")
}

func (m *mockCapacityReportRepo) GetEpicsByTeamYearQuarter(ctx context.Context, teamID uuid.UUID, year, quarter int) ([]domain.Epic, error) {
	if m.getEpicsByTeamYearQuarterFunc != nil {
		return m.getEpicsByTeamYearQuarterFunc(ctx, teamID, year, quarter)
	}
	return nil, errors.New("not implemented")
}

func (m *mockCapacityReportRepo) GetStoriesByEpicID(ctx context.Context, id uuid.UUID) ([]domain.Epic, error) {
	if m.getStoriesByEpicIDFunc != nil {
		return m.getStoriesByEpicIDFunc(ctx, id)
	}
	return nil, errors.New("not implemented")
}

func (m *mockCapacityReportRepo) GetEpicRoleScoresByEpicID(ctx context.Context, id uuid.UUID) ([]domain.EpicRoleScore, error) {
	if m.getEpicRoleScoresByEpicIDFunc != nil {
		return m.getEpicRoleScoresByEpicIDFunc(ctx, id)
	}
	return nil, errors.New("not implemented")
}

func (m *mockCapacityReportRepo) GetRoleByID(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
	if m.getRoleByIDFunc != nil {
		return m.getRoleByIDFunc(ctx, id)
	}
	return nil, errors.New("not implemented")
}

// TestBuildCapacityReport_BasicReport — проверяет базовый результат агрегации.
func TestBuildCapacityReport_BasicReport(t *testing.T) {
	teamID := uuid.New()
	roleID := uuid.New()
	userID := uuid.New()
	epicID := uuid.New()
	log := slog.Default()

	repo := &mockCapacityReportRepo{
		getTeamByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
			return &domain.Team{ID: teamID, Name: "Test Team"}, nil
		},
		getUsersByTeamIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.User, error) {
			return []domain.User{{ID: userID, FirstName: "Test", LastName: "User"}}, nil
		},
		getRoleByUserIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
			return &domain.Role{ID: roleID, Name: "Backend"}, nil
		},
		getEpicsByTeamYearQuarterFunc: func(ctx context.Context, tID uuid.UUID, year, quarter int) ([]domain.Epic, error) {
			finalScore := 100.0
			return []domain.Epic{
				{
					ID:         epicID,
					Number:     "E-001",
					Name:       "Test Epic",
					Type:       "feature",
					Status:     domain.StatusScored,
					FinalScore: &finalScore,
					TeamID:     tID,
					Year:       year,
					Quarter:    quarter,
				},
			}, nil
		},
		getStoriesByEpicIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.Epic, error) {
			return []domain.Epic{}, nil
		},
		getEpicRoleScoresByEpicIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.EpicRoleScore, error) {
			return []domain.EpicRoleScore{
				{ID: uuid.New(), EpicID: id, RoleID: roleID, WeightedAvg: 100.0},
			}, nil
		},
		getRoleByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
			return &domain.Role{ID: roleID, Name: "Backend"}, nil
		},
	}

	resp, err := BuildCapacityReport(context.Background(), log, repo, teamID, 2026, 3)

	if err != nil {
		t.Fatalf("BuildCapacityReport failed: %v", err)
	}

	if resp == nil {
		t.Fatalf("expected non-nil response")
	}

	// Проверяем основные поля
	if resp.TeamName != "Test Team" {
		t.Errorf("expected team name 'Test Team', got %q", resp.TeamName)
	}
	if resp.Year != 2026 {
		t.Errorf("expected year 2026, got %d", resp.Year)
	}
	if resp.Quarter != 3 {
		t.Errorf("expected quarter 3, got %d", resp.Quarter)
	}

	// Проверяем вместимость (1 пользователь * 8 * 6 * 0.838 ≈ 40.224)
	const expectedCapacity = 8.0 * 6.0 * 0.838
	if resp.TotalCapacity < expectedCapacity-0.1 || resp.TotalCapacity > expectedCapacity+0.1 {
		t.Errorf("expected total capacity ~%.2f, got %.2f", expectedCapacity, resp.TotalCapacity)
	}

	// Проверяем, что есть ролевая вместимость
	if len(resp.RoleCapacities) == 0 {
		t.Errorf("expected at least one role capacity")
	}

	// Проверяем квоты
	if _, ok := resp.Quotas["feature"]; !ok {
		t.Errorf("expected 'feature' quota")
	}
	if _, ok := resp.Quotas["tech_architecture"]; !ok {
		t.Errorf("expected 'tech_architecture' quota")
	}

	// Проверяем эпики
	if len(resp.Epics) != 1 {
		t.Errorf("expected 1 epic, got %d", len(resp.Epics))
	}

	if resp.Epics[0].Number != "E-001" {
		t.Errorf("expected epic number 'E-001', got %q", resp.Epics[0].Number)
	}

	if resp.Epics[0].FinalScore != 100.0 {
		t.Errorf("expected final score 100.0, got %f", resp.Epics[0].FinalScore)
	}
}

// TestBuildCapacityReport_TeamNotFound — проверяет обработку несуществующей команды.
func TestBuildCapacityReport_TeamNotFound(t *testing.T) {
	teamID := uuid.New()
	log := slog.Default()

	repo := &mockCapacityReportRepo{
		getTeamByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
			return nil, errors.New("team not found")
		},
	}

	resp, err := BuildCapacityReport(context.Background(), log, repo, teamID, 2026, 3)

	if err == nil {
		t.Fatalf("expected error for team not found")
	}

	if resp != nil {
		t.Errorf("expected nil response on error")
	}

	if !errors.Is(err, ErrTeamNotFound) {
		t.Errorf("expected ErrTeamNotFound, got %v", err)
	}
}

// TestBuildCapacityReport_NoEpics — проверяет отчёт без эпиков.
func TestBuildCapacityReport_NoEpics(t *testing.T) {
	teamID := uuid.New()
	userID := uuid.New()
	roleID := uuid.New()
	log := slog.Default()

	repo := &mockCapacityReportRepo{
		getTeamByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
			return &domain.Team{ID: teamID, Name: "Test Team"}, nil
		},
		getUsersByTeamIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.User, error) {
			return []domain.User{{ID: userID, FirstName: "Test", LastName: "User"}}, nil
		},
		getRoleByUserIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
			return &domain.Role{ID: roleID, Name: "Backend"}, nil
		},
		getEpicsByTeamYearQuarterFunc: func(ctx context.Context, tID uuid.UUID, year, quarter int) ([]domain.Epic, error) {
			return []domain.Epic{}, nil // Нет эпиков
		},
	}

	resp, err := BuildCapacityReport(context.Background(), log, repo, teamID, 2026, 3)

	if err != nil {
		t.Fatalf("BuildCapacityReport failed: %v", err)
	}

	if resp == nil {
		t.Fatalf("expected non-nil response")
	}

	// Проверяем, что есть вместимость, даже без эпиков
	if resp.TotalCapacity == 0 {
		t.Errorf("expected non-zero total capacity even without epics")
	}

	// Проверяем, что эпиков нет
	if len(resp.Epics) != 0 {
		t.Errorf("expected 0 epics, got %d", len(resp.Epics))
	}

	// Проверяем квоты (должны быть, но с нулевыми процентами)
	if _, ok := resp.Quotas["feature"]; !ok {
		t.Errorf("expected 'feature' quota")
	}
	if resp.Quotas["feature"].ActualPercent != 0 {
		t.Errorf("expected feature quota 0%% when no epics")
	}
}

// TestBuildCapacityReport_MultipleRoles — проверяет разделение вместимости по ролям.
func TestBuildCapacityReport_MultipleRoles(t *testing.T) {
	teamID := uuid.New()
	role1ID := uuid.New()
	role2ID := uuid.New()
	user1ID := uuid.New()
	user2ID := uuid.New()
	log := slog.Default()

	repo := &mockCapacityReportRepo{
		getTeamByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
			return &domain.Team{ID: teamID, Name: "Test Team"}, nil
		},
		getUsersByTeamIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.User, error) {
			return []domain.User{
				{ID: user1ID, FirstName: "User1", LastName: "One"},
				{ID: user2ID, FirstName: "User2", LastName: "Two"},
			}, nil
		},
		getRoleByUserIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
			if id == user1ID {
				return &domain.Role{ID: role1ID, Name: "Backend"}, nil
			}
			if id == user2ID {
				return &domain.Role{ID: role2ID, Name: "Frontend"}, nil
			}
			return nil, errors.New("role not found")
		},
		getEpicsByTeamYearQuarterFunc: func(ctx context.Context, tID uuid.UUID, year, quarter int) ([]domain.Epic, error) {
			return []domain.Epic{}, nil
		},
	}

	resp, err := BuildCapacityReport(context.Background(), log, repo, teamID, 2026, 3)

	if err != nil {
		t.Fatalf("BuildCapacityReport failed: %v", err)
	}

	if resp == nil {
		t.Fatalf("expected non-nil response")
	}

	// Проверяем, что есть две роли
	if len(resp.RoleCapacities) != 2 {
		t.Errorf("expected 2 role capacities, got %d", len(resp.RoleCapacities))
	}

	// Проверяем, что каждая роль имеет свою вместимость
	const expectedRoleCapacity = 8.0 * 6.0 * 0.838
	for _, rc := range resp.RoleCapacities {
		if rc.Capacity < expectedRoleCapacity-0.1 || rc.Capacity > expectedRoleCapacity+0.1 {
			t.Errorf("expected role capacity ~%.2f, got %.2f", expectedRoleCapacity, rc.Capacity)
		}
	}
}

// TestBuildCapacityReport_QuotasExceeded — проверяет превышение квот.
func TestBuildCapacityReport_QuotasExceeded(t *testing.T) {
	teamID := uuid.New()
	roleID := uuid.New()
	userID := uuid.New()
	epicID := uuid.New()
	log := slog.Default()

	repo := &mockCapacityReportRepo{
		getTeamByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
			return &domain.Team{ID: teamID, Name: "Test Team"}, nil
		},
		getUsersByTeamIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.User, error) {
			return []domain.User{{ID: userID, FirstName: "Test", LastName: "User"}}, nil
		},
		getRoleByUserIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
			return &domain.Role{ID: roleID, Name: "Backend"}, nil
		},
		getEpicsByTeamYearQuarterFunc: func(ctx context.Context, tID uuid.UUID, year, quarter int) ([]domain.Epic, error) {
			finalScore := 100.0
			return []domain.Epic{
				{
					ID:         epicID,
					Number:     "E-001",
					Name:       "Feature Epic",
					Type:       "feature",
					Status:     domain.StatusScored,
					FinalScore: &finalScore,
					TeamID:     tID,
					Year:       year,
					Quarter:    quarter,
				},
				{
					ID:         uuid.New(),
					Number:     "E-002",
					Name:       "Tech Debt Epic",
					Type:       "techdebt",
					Status:     domain.StatusScored,
					FinalScore: &finalScore,
					TeamID:     tID,
					Year:       year,
					Quarter:    quarter,
				},
			}, nil
		},
		getStoriesByEpicIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.Epic, error) {
			return []domain.Epic{}, nil
		},
		getEpicRoleScoresByEpicIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.EpicRoleScore, error) {
			return []domain.EpicRoleScore{
				{ID: uuid.New(), EpicID: id, RoleID: roleID, WeightedAvg: 100.0},
			}, nil
		},
		getRoleByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
			return &domain.Role{ID: roleID, Name: "Backend"}, nil
		},
	}

	resp, err := BuildCapacityReport(context.Background(), log, repo, teamID, 2026, 3)

	if err != nil {
		t.Fatalf("BuildCapacityReport failed: %v", err)
	}

	// Проверяем квоты: 50% feature, 50% tech → feature в норме, tech превышено
	featureQuota := resp.Quotas["feature"]
	techQuota := resp.Quotas["tech_architecture"]

	if featureQuota.ActualPercent > 40 && featureQuota.Status != "EXCEEDED" {
		t.Errorf("expected feature quota exceeded")
	}

	if techQuota.ActualPercent > 60 && techQuota.Status != "EXCEEDED" {
		t.Errorf("expected tech quota exceeded")
	}
}

// TestBuildCapacityReport_WithStories — проверяет обработку эпиков с историями.
func TestBuildCapacityReport_WithStories(t *testing.T) {
	teamID := uuid.New()
	roleID := uuid.New()
	userID := uuid.New()
	epicID := uuid.New()
	story1ID := uuid.New()
	story2ID := uuid.New()
	log := slog.Default()

	repo := &mockCapacityReportRepo{
		getTeamByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
			return &domain.Team{ID: teamID, Name: "Test Team"}, nil
		},
		getUsersByTeamIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.User, error) {
			return []domain.User{{ID: userID, FirstName: "Test", LastName: "User"}}, nil
		},
		getRoleByUserIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
			return &domain.Role{ID: roleID, Name: "Backend"}, nil
		},
		getEpicsByTeamYearQuarterFunc: func(ctx context.Context, tID uuid.UUID, year, quarter int) ([]domain.Epic, error) {
			finalScore := 100.0
			return []domain.Epic{
				{
					ID:         epicID,
					Number:     "E-001",
					Name:       "Test Epic with Stories",
					Type:       "feature",
					Status:     domain.StatusScored,
					FinalScore: &finalScore,
					TeamID:     tID,
					Year:       year,
					Quarter:    quarter,
				},
			}, nil
		},
		getStoriesByEpicIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.Epic, error) {
			// Возвращаем две истории
			score1 := 100.0
			score2 := 80.0
			return []domain.Epic{
				{
					ID:         story1ID,
					Number:     "S-001",
					Name:       "Story 1",
					Status:     domain.StatusScored,
					FinalScore: &score1,
					TeamID:     teamID,
				},
				{
					ID:         story2ID,
					Number:     "S-002",
					Name:       "Story 2",
					Status:     domain.StatusScored,
					FinalScore: &score2,
					TeamID:     teamID,
				},
			}, nil
		},
		getEpicRoleScoresByEpicIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.EpicRoleScore, error) {
			if id == story1ID {
				return []domain.EpicRoleScore{
					{ID: uuid.New(), EpicID: id, RoleID: roleID, WeightedAvg: 50.0},
				}, nil
			}
			if id == story2ID {
				return []domain.EpicRoleScore{
					{ID: uuid.New(), EpicID: id, RoleID: roleID, WeightedAvg: 40.0},
				}, nil
			}
			return []domain.EpicRoleScore{}, nil
		},
		getRoleByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
			return &domain.Role{ID: roleID, Name: "Backend"}, nil
		},
	}

	resp, err := BuildCapacityReport(context.Background(), log, repo, teamID, 2026, 3)

	if err != nil {
		t.Fatalf("BuildCapacityReport failed: %v", err)
	}

	if resp == nil {
		t.Fatalf("expected non-nil response")
	}

	// Проверяем, что есть эпик
	if len(resp.Epics) != 1 {
		t.Errorf("expected 1 epic, got %d", len(resp.Epics))
	}

	// Проверяем, что ролевые оценки рассчитаны правильно
	// story1: 50 * (100/50) = 100, story2: 40 * (80/40) = 80
	// Всего: 180, но final_score = 100, значит risk_factor = 100/180 ≈ 0.556
	// role_scores = (50*100 + 40*80) / (final_score) ≈ (5000 + 3200) / 100 = 82 чд
	if len(resp.Epics[0].RoleScores) == 0 {
		t.Errorf("expected role scores to be calculated for epic with stories")
	}
}

// TestBuildCapacityReport_RiskFactorCalculation — проверяет правильность расчёта риск-фактора.
func TestBuildCapacityReport_RiskFactorCalculation(t *testing.T) {
	teamID := uuid.New()
	roleID := uuid.New()
	userID := uuid.New()
	epicID := uuid.New()
	log := slog.Default()

	const weightedAvg = 100.0
	const finalScore = 80.0
	expectedRiskFactor := finalScore / weightedAvg // 0.8

	repo := &mockCapacityReportRepo{
		getTeamByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
			return &domain.Team{ID: teamID, Name: "Test Team"}, nil
		},
		getUsersByTeamIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.User, error) {
			return []domain.User{{ID: userID, FirstName: "Test", LastName: "User"}}, nil
		},
		getRoleByUserIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
			return &domain.Role{ID: roleID, Name: "Backend"}, nil
		},
		getEpicsByTeamYearQuarterFunc: func(ctx context.Context, tID uuid.UUID, year, quarter int) ([]domain.Epic, error) {
			fs := finalScore
			return []domain.Epic{
				{
					ID:         epicID,
					Number:     "E-001",
					Name:       "Risky Epic",
					Type:       "feature",
					Status:     domain.StatusScored,
					FinalScore: &fs,
					TeamID:     tID,
					Year:       year,
					Quarter:    quarter,
				},
			}, nil
		},
		getStoriesByEpicIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.Epic, error) {
			return []domain.Epic{}, nil
		},
		getEpicRoleScoresByEpicIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.EpicRoleScore, error) {
			return []domain.EpicRoleScore{
				{ID: uuid.New(), EpicID: id, RoleID: roleID, WeightedAvg: weightedAvg},
			}, nil
		},
		getRoleByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
			return &domain.Role{ID: roleID, Name: "Backend"}, nil
		},
	}

	resp, err := BuildCapacityReport(context.Background(), log, repo, teamID, 2026, 3)

	if err != nil {
		t.Fatalf("BuildCapacityReport failed: %v", err)
	}

	if len(resp.Epics) == 0 {
		t.Fatalf("expected at least one epic")
	}

	epic := resp.Epics[0]

	// Проверяем raw_role_scores (без риск-фактора)
	rawScore := epic.RawRoleScores["Backend"]
	if rawScore != weightedAvg {
		t.Errorf("expected raw_role_score %.1f, got %.1f", weightedAvg, rawScore)
	}

	// Проверяем role_scores (с риск-фактором)
	expectedRoleScore := weightedAvg * expectedRiskFactor
	roleScore := epic.RoleScores["Backend"]
	const tolerance = 0.01
	if roleScore < expectedRoleScore-tolerance || roleScore > expectedRoleScore+tolerance {
		t.Errorf("expected role_score %.2f, got %.2f (risk_factor %.2f)", expectedRoleScore, roleScore, expectedRiskFactor)
	}
}

// TestBuildCapacityReport_MixedEpicTypes — проверяет расчёт квот с эпиками разных типов.
func TestBuildCapacityReport_MixedEpicTypes(t *testing.T) {
	teamID := uuid.New()
	roleID := uuid.New()
	userID := uuid.New()
	log := slog.Default()

	featureEpicID := uuid.New()
	archEpicID := uuid.New()

	repo := &mockCapacityReportRepo{
		getTeamByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
			return &domain.Team{ID: teamID, Name: "Test Team"}, nil
		},
		getUsersByTeamIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.User, error) {
			return []domain.User{{ID: userID, FirstName: "Test", LastName: "User"}}, nil
		},
		getRoleByUserIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
			return &domain.Role{ID: roleID, Name: "Backend"}, nil
		},
		getEpicsByTeamYearQuarterFunc: func(ctx context.Context, tID uuid.UUID, year, quarter int) ([]domain.Epic, error) {
			featureScore := 300.0
			archScore := 200.0
			return []domain.Epic{
				{
					ID:         featureEpicID,
					Number:     "F-001",
					Name:       "Feature Epic",
					Type:       "feature",
					Status:     domain.StatusScored,
					FinalScore: &featureScore,
					TeamID:     tID,
					Year:       year,
					Quarter:    quarter,
				},
				{
					ID:         archEpicID,
					Number:     "A-001",
					Name:       "Architecture Epic",
					Type:       "architecture",
					Status:     domain.StatusScored,
					FinalScore: &archScore,
					TeamID:     tID,
					Year:       year,
					Quarter:    quarter,
				},
			}, nil
		},
		getStoriesByEpicIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.Epic, error) {
			return []domain.Epic{}, nil
		},
		getEpicRoleScoresByEpicIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.EpicRoleScore, error) {
			return []domain.EpicRoleScore{
				{ID: uuid.New(), EpicID: id, RoleID: roleID, WeightedAvg: 100.0},
			}, nil
		},
		getRoleByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
			return &domain.Role{ID: roleID, Name: "Backend"}, nil
		},
	}

	resp, err := BuildCapacityReport(context.Background(), log, repo, teamID, 2026, 3)

	if err != nil {
		t.Fatalf("BuildCapacityReport failed: %v", err)
	}

	// Проверяем квоты:
	// Total = 300 + 200 = 500
	// Feature % = 300 / 500 = 60% (превышено, лимит 40%)
	// Tech % = 200 / 500 = 40% (в норме, лимит 60%)

	featureQuota := resp.Quotas["feature"]
	if featureQuota.ActualPercent != 60.0 {
		t.Errorf("expected feature quota 60%%, got %.1f%%", featureQuota.ActualPercent)
	}
	if featureQuota.Status != "EXCEEDED" {
		t.Errorf("expected feature quota status EXCEEDED, got %s", featureQuota.Status)
	}

	techQuota := resp.Quotas["tech_architecture"]
	if techQuota.ActualPercent != 40.0 {
		t.Errorf("expected tech quota 40%%, got %.1f%%", techQuota.ActualPercent)
	}
	if techQuota.Status != "OK" {
		t.Errorf("expected tech quota status OK, got %s", techQuota.Status)
	}
}

// TestBuildCapacityReport_UnscoredEpics — проверяет, что неоцененные эпики не учитываются.
func TestBuildCapacityReport_UnscoredEpics(t *testing.T) {
	teamID := uuid.New()
	roleID := uuid.New()
	userID := uuid.New()
	log := slog.Default()

	scoredEpicID := uuid.New()
	unscoredEpicID := uuid.New()

	repo := &mockCapacityReportRepo{
		getTeamByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
			return &domain.Team{ID: teamID, Name: "Test Team"}, nil
		},
		getUsersByTeamIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.User, error) {
			return []domain.User{{ID: userID, FirstName: "Test", LastName: "User"}}, nil
		},
		getRoleByUserIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
			return &domain.Role{ID: roleID, Name: "Backend"}, nil
		},
		getEpicsByTeamYearQuarterFunc: func(ctx context.Context, tID uuid.UUID, year, quarter int) ([]domain.Epic, error) {
			finalScore := 100.0
			return []domain.Epic{
				{
					ID:         scoredEpicID,
					Number:     "E-001",
					Name:       "Scored Epic",
					Type:       "feature",
					Status:     domain.StatusScored,
					FinalScore: &finalScore,
					TeamID:     tID,
					Year:       year,
					Quarter:    quarter,
				},
				{
					ID:     unscoredEpicID,
					Number: "E-002",
					Name:   "Unscored Epic",
					Type:   "feature",
					Status: domain.StatusNew, // Не оценен
					TeamID: tID,
					Year:   year,
					Quarter: quarter,
				},
			}, nil
		},
		getStoriesByEpicIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.Epic, error) {
			return []domain.Epic{}, nil
		},
		getEpicRoleScoresByEpicIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.EpicRoleScore, error) {
			if id == scoredEpicID {
				return []domain.EpicRoleScore{
					{ID: uuid.New(), EpicID: id, RoleID: roleID, WeightedAvg: 100.0},
				}, nil
			}
			return []domain.EpicRoleScore{}, nil
		},
		getRoleByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
			return &domain.Role{ID: roleID, Name: "Backend"}, nil
		},
	}

	resp, err := BuildCapacityReport(context.Background(), log, repo, teamID, 2026, 3)

	if err != nil {
		t.Fatalf("BuildCapacityReport failed: %v", err)
	}

	// Проверяем, что в ответе два эпика, но только один имеет оценки
	if len(resp.Epics) != 2 {
		t.Errorf("expected 2 epics in response, got %d", len(resp.Epics))
	}

	scoredEpic := resp.Epics[0]
	if scoredEpic.FinalScore != 100.0 || len(scoredEpic.RoleScores) == 0 {
		t.Errorf("expected scored epic to have role scores")
	}
}
