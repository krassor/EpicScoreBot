package services

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"EpicScoreBot/internal/models/domain"

	"github.com/google/uuid"
)

func floatPtr(f float64) *float64 {
	return &f
}

func TestEpicService_CreateEpic(t *testing.T) {
	ctx := context.Background()
	log := newDiscardLogger()
	teamID := uuid.New()
	epicID := uuid.New()

	t.Run("успешное создание эпика", func(t *testing.T) {
		repo := &MockRepository{
			GetEpicByNumberFunc: func(ctx context.Context, number string) (*domain.Epic, error) {
				return nil, sql.ErrNoRows
			},
			CreateEpicFunc: func(ctx context.Context, number, name, description string, tID uuid.UUID) (*domain.Epic, error) {
				return &domain.Epic{
					ID:          epicID,
					Number:      number,
					Name:        name,
					Description: description,
					TeamID:      tID,
					Status:      domain.StatusNew,
				}, nil
			},
		}

		s := NewEpicService(log, repo)
		epic, err := s.CreateEpic(ctx, "EPIC-101", "Фича 1", "Описание фичи", teamID)
		if err != nil {
			t.Fatalf("ожидалось успешное создание, получена ошибка: %v", err)
		}
		if epic == nil {
			t.Fatal("ожидался созданный эпик, получен nil")
		}
		if epic.Number != "EPIC-101" || epic.Name != "Фича 1" || epic.TeamID != teamID {
			t.Errorf("неверные данные созданного эпика: %+v", epic)
		}
	})

	t.Run("эпик с таким номером уже существует", func(t *testing.T) {
		repo := &MockRepository{
			GetEpicByNumberFunc: func(ctx context.Context, number string) (*domain.Epic, error) {
				return &domain.Epic{
					ID:     epicID,
					Number: number,
				}, nil
			},
		}

		s := NewEpicService(log, repo)
		epic, err := s.CreateEpic(ctx, "EPIC-101", "Фича 1", "Описание фичи", teamID)
		if !errors.Is(err, ErrEpicAlreadyExists) {
			t.Fatalf("ожидалась ошибка %v, получена: %v", ErrEpicAlreadyExists, err)
		}
		if epic != nil {
			t.Fatal("ожидался nil-эпик")
		}
	})

	t.Run("ошибка репозитория при проверке номера", func(t *testing.T) {
		dbErr := errors.New("db error")
		repo := &MockRepository{
			GetEpicByNumberFunc: func(ctx context.Context, number string) (*domain.Epic, error) {
				return nil, dbErr
			},
		}

		s := NewEpicService(log, repo)
		epic, err := s.CreateEpic(ctx, "EPIC-101", "Фича 1", "Описание фичи", teamID)
		if !errors.Is(err, dbErr) {
			t.Fatalf("ожидалась ошибка %v, получена: %v", dbErr, err)
		}
		if epic != nil {
			t.Fatal("ожидался nil-эпик")
		}
	})

	t.Run("ошибка репозитория при создании", func(t *testing.T) {
		dbErr := errors.New("db create error")
		repo := &MockRepository{
			GetEpicByNumberFunc: func(ctx context.Context, number string) (*domain.Epic, error) {
				return nil, sql.ErrNoRows
			},
			CreateEpicFunc: func(ctx context.Context, number, name, description string, tID uuid.UUID) (*domain.Epic, error) {
				return nil, dbErr
			},
		}

		s := NewEpicService(log, repo)
		epic, err := s.CreateEpic(ctx, "EPIC-101", "Фича 1", "Описание фичи", teamID)
		if !errors.Is(err, dbErr) {
			t.Fatalf("ожидалась ошибка %v, получена: %v", dbErr, err)
		}
		if epic != nil {
			t.Fatal("ожидался nil-эпик")
		}
	})
}

func TestEpicService_GetUnscoredEpicsForUserAcrossTeams(t *testing.T) {
	ctx := context.Background()
	log := newDiscardLogger()
	userID := uuid.New()
	teamID1 := uuid.New()
	teamID2 := uuid.New()
	epicID1 := uuid.New()
	epicID2 := uuid.New()

	t.Run("успешный сбор и дедупликация эпиков", func(t *testing.T) {
		repo := &MockRepository{
			GetTeamsByUserTelegramIDFunc: func(ctx context.Context, telegramID string) ([]domain.Team, error) {
				return []domain.Team{
					{ID: teamID1, Name: "Team 1"},
					{ID: teamID2, Name: "Team 2"},
				}, nil
			},
			GetUnscoredEpicsByUserFunc: func(ctx context.Context, uID, tID uuid.UUID) ([]domain.Epic, error) {
				if tID == teamID1 {
					return []domain.Epic{
						{ID: epicID1, Number: "EPIC-1"},
						{ID: epicID2, Number: "EPIC-2"}, // дублируется во второй команде
					}, nil
				}
				if tID == teamID2 {
					return []domain.Epic{
						{ID: epicID2, Number: "EPIC-2"},
						{ID: uuid.New(), Number: "EPIC-3"},
					}, nil
				}
				return nil, nil
			},
		}

		s := NewEpicService(log, repo)
		epics, err := s.GetUnscoredEpicsForUserAcrossTeams(ctx, userID, "tg-123")
		if err != nil {
			t.Fatalf("ожидалось успешное получение, получена ошибка: %v", err)
		}

		// Должно быть ровно 3 уникальных эпика
		if len(epics) != 3 {
			t.Errorf("ожидалось 3 эпика, получено: %d", len(epics))
		}

		seen := make(map[uuid.UUID]bool)
		for _, e := range epics {
			seen[e.ID] = true
		}

		if !seen[epicID1] || !seen[epicID2] {
			t.Errorf("список не содержит ожидаемые epic ID: %+v", epics)
		}
	})

	t.Run("ошибка при получении команд пользователя", func(t *testing.T) {
		dbErr := errors.New("failed to get teams")
		repo := &MockRepository{
			GetTeamsByUserTelegramIDFunc: func(ctx context.Context, telegramID string) ([]domain.Team, error) {
				return nil, dbErr
			},
		}

		s := NewEpicService(log, repo)
		epics, err := s.GetUnscoredEpicsForUserAcrossTeams(ctx, userID, "tg-123")
		if !errors.Is(err, dbErr) {
			t.Fatalf("ожидалась ошибка %v, получена: %v", dbErr, err)
		}
		if epics != nil {
			t.Fatal("ожидался nil слайс эпиков")
		}
	})
}

func TestEpicService_GetReportData(t *testing.T) {
	ctx := context.Background()
	log := newDiscardLogger()
	teamID := uuid.New()
	epicID := uuid.New()
	roleID := uuid.New()
	riskID := uuid.New()
	userID := uuid.New()

	t.Run("успешная генерация отчета", func(t *testing.T) {
		repo := &MockRepository{
			GetTeamByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
				return &domain.Team{ID: teamID, Name: "Супер Команда"}, nil
			},
			GetEpicsByTeamIDAndStatusFunc: func(ctx context.Context, tID uuid.UUID, status domain.Status) ([]domain.Epic, error) {
				if tID != teamID || status != domain.StatusScored {
					t.Errorf("неверные параметры: tID=%v, status=%s", tID, status)
				}
				return []domain.Epic{
					{
						ID:         epicID,
						Number:     "EP-42",
						Name:       "Epic 42",
						TeamID:     teamID,
						Status:     domain.StatusScored,
						FinalScore: floatPtr(15.5),
					},
				}, nil
			},
			GetEpicRoleScoresByEpicIDFunc: func(ctx context.Context, eID uuid.UUID) ([]domain.EpicRoleScore, error) {
				if eID != epicID {
					t.Errorf("неверный epicID: %v", eID)
				}
				return []domain.EpicRoleScore{
					{
						RoleID:      roleID,
						WeightedAvg: 12.5,
					},
				}, nil
			},
			GetRoleByIDFunc: func(ctx context.Context, rID uuid.UUID) (*domain.Role, error) {
				if rID != roleID {
					t.Errorf("неверный roleID: %v", rID)
				}
				return &domain.Role{ID: roleID, Name: "IT-лидер"}, nil
			},
			GetRisksByEpicIDFunc: func(ctx context.Context, eID uuid.UUID) ([]domain.Risk, error) {
				if eID != epicID {
					t.Errorf("неверный epicID: %v", eID)
				}
				return []domain.Risk{
					{
						ID:            riskID,
						Description:   "Высокая нагрузка",
						EpicID:        epicID,
						WeightedScore: floatPtr(6.0), // Rounded 6 -> coeff 1.05
					},
				}, nil
			},
			GetRiskScoresByRiskIDFunc: func(ctx context.Context, rID uuid.UUID) ([]domain.RiskScore, error) {
				if rID != riskID {
					t.Errorf("неверный riskID: %v", rID)
				}
				return []domain.RiskScore{
					{
						RiskID:      riskID,
						UserID:      userID,
						Probability: 2,
						Impact:      3,
					},
				}, nil
			},
		}

		s := NewEpicService(log, repo)
		report, err := s.GetReportData(ctx, teamID)
		if err != nil {
			t.Fatalf("ожидался успешный отчет, получена ошибка: %v", err)
		}

		if report.TeamName != "Супер Команда" {
			t.Errorf("неверное имя команды: %s", report.TeamName)
		}

		if len(report.Epics) != 1 {
			t.Fatalf("ожидался 1 эпик в отчете, получено: %d", len(report.Epics))
		}

		eReport := report.Epics[0]
		if eReport.Number != "EP-42" || eReport.Name != "Epic 42" {
			t.Errorf("неверные данные эпика: %+v", eReport)
		}

		if eReport.FinalScore != 15.5 {
			t.Errorf("ожидался FinalScore = 15.5, получено: %f", eReport.FinalScore)
		}

		if eReport.TotalScore != 12.5 {
			t.Errorf("ожидался TotalScore = 12.5, получено: %f", eReport.TotalScore)
		}

		if len(eReport.RoleScores) != 1 || eReport.RoleScores[0].RoleName != "IT-лидер" || eReport.RoleScores[0].WeightedAvg != 12.5 {
			t.Errorf("неверные оценки ролей: %+v", eReport.RoleScores)
		}

		if len(eReport.Risks) != 1 {
			t.Fatalf("ожидался 1 риск в отчете, получено: %d", len(eReport.Risks))
		}

		rReport := eReport.Risks[0]
		if rReport.Description != "Высокая нагрузка" || rReport.WeightedScore != 6.0 || rReport.Coefficient != 1.05 {
			t.Errorf("неверные данные риска: %+v", rReport)
		}

		if len(rReport.Probabilities) != 1 || rReport.Probabilities[0] != 2 || len(rReport.Impacts) != 1 || rReport.Impacts[0] != 3 {
			t.Errorf("неверные детальные оценки риска: %+v", rReport)
		}
	})

	t.Run("ошибка при получении команды", func(t *testing.T) {
		dbErr := errors.New("team error")
		repo := &MockRepository{
			GetTeamByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
				return nil, dbErr
			},
		}

		s := NewEpicService(log, repo)
		report, err := s.GetReportData(ctx, teamID)
		if err == nil || !errors.Is(err, dbErr) {
			t.Fatalf("ожидалась ошибка %v, получена: %v", dbErr, err)
		}
		if report != nil {
			t.Fatal("ожидался nil отчет")
		}
	})

	t.Run("ошибка при получении эпиков команды", func(t *testing.T) {
		dbErr := errors.New("epics error")
		repo := &MockRepository{
			GetTeamByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
				return &domain.Team{ID: teamID, Name: "Супер Команда"}, nil
			},
			GetEpicsByTeamIDAndStatusFunc: func(ctx context.Context, tID uuid.UUID, status domain.Status) ([]domain.Epic, error) {
				return nil, dbErr
			},
		}

		s := NewEpicService(log, repo)
		report, err := s.GetReportData(ctx, teamID)
		if err == nil || !errors.Is(err, dbErr) {
			t.Fatalf("ожидалась ошибка %v, получена: %v", dbErr, err)
		}
		if report != nil {
			t.Fatal("ожидался nil отчет")
		}
	})

	t.Run("роль не найдена (возвращается строковый ID)", func(t *testing.T) {
		repo := &MockRepository{
			GetTeamByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
				return &domain.Team{ID: teamID, Name: "Супер Команда"}, nil
			},
			GetEpicsByTeamIDAndStatusFunc: func(ctx context.Context, tID uuid.UUID, status domain.Status) ([]domain.Epic, error) {
				return []domain.Epic{{ID: epicID, Number: "EP-42", TeamID: teamID, Status: domain.StatusScored}}, nil
			},
			GetEpicRoleScoresByEpicIDFunc: func(ctx context.Context, eID uuid.UUID) ([]domain.EpicRoleScore, error) {
				return []domain.EpicRoleScore{{RoleID: roleID, WeightedAvg: 10.0}}, nil
			},
			GetRoleByIDFunc: func(ctx context.Context, rID uuid.UUID) (*domain.Role, error) {
				return nil, errors.New("role not found")
			},
			GetRisksByEpicIDFunc: func(ctx context.Context, eID uuid.UUID) ([]domain.Risk, error) {
				return []domain.Risk{}, nil
			},
		}

		s := NewEpicService(log, repo)
		report, err := s.GetReportData(ctx, teamID)
		if err != nil {
			t.Fatalf("ожидался успешный отчет, получена ошибка: %v", err)
		}

		if len(report.Epics[0].RoleScores) != 1 || report.Epics[0].RoleScores[0].RoleName != roleID.String() {
			t.Errorf("ожидалось название роли равным ID строкой, получено: %s", report.Epics[0].RoleScores[0].RoleName)
		}
	})

	t.Run("риск без WeightedScore (коэффициент 1.0)", func(t *testing.T) {
		repo := &MockRepository{
			GetTeamByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
				return &domain.Team{ID: teamID, Name: "Супер Команда"}, nil
			},
			GetEpicsByTeamIDAndStatusFunc: func(ctx context.Context, tID uuid.UUID, status domain.Status) ([]domain.Epic, error) {
				return []domain.Epic{{ID: epicID, Number: "EP-42", TeamID: teamID, Status: domain.StatusScored}}, nil
			},
			GetEpicRoleScoresByEpicIDFunc: func(ctx context.Context, eID uuid.UUID) ([]domain.EpicRoleScore, error) {
				return []domain.EpicRoleScore{}, nil
			},
			GetRisksByEpicIDFunc: func(ctx context.Context, eID uuid.UUID) ([]domain.Risk, error) {
				return []domain.Risk{
					{
						ID:            riskID,
						Description:   "Риск без оценки",
						EpicID:        epicID,
						WeightedScore: nil,
					},
				}, nil
			},
			GetRiskScoresByRiskIDFunc: func(ctx context.Context, rID uuid.UUID) ([]domain.RiskScore, error) {
				return []domain.RiskScore{}, nil
			},
		}

		s := NewEpicService(log, repo)
		report, err := s.GetReportData(ctx, teamID)
		if err != nil {
			t.Fatalf("ожидался отчет, получена ошибка: %v", err)
		}

		if report.Epics[0].Risks[0].Coefficient != 1.0 {
			t.Errorf("ожидался коэффициент 1.0, получено: %f", report.Epics[0].Risks[0].Coefficient)
		}
	})
}

func TestEpicService_OtherMethods(t *testing.T) {
	ctx := context.Background()
	log := newDiscardLogger()
	epicID := uuid.New()
	userID := uuid.New()
	roleID := uuid.New()
	teamID := uuid.New()
	dbErr := errors.New("database error")

	t.Run("GetEpicByID трансляция", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			GetEpicByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Epic, error) {
				called = true
				if id != epicID {
					t.Errorf("неверный id: %v", id)
				}
				return &domain.Epic{ID: id}, nil
			},
		}
		s := NewEpicService(log, repo)
		epic, err := s.GetEpicByID(ctx, epicID)
		if err != nil || !called || epic == nil || epic.ID != epicID {
			t.Errorf("ошибка трансляции GetEpicByID: err=%v, called=%t, epic=%v", err, called, epic)
		}
	})

	t.Run("GetEpicByNumber трансляция", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			GetEpicByNumberFunc: func(ctx context.Context, num string) (*domain.Epic, error) {
				called = true
				if num != "EP-1" {
					t.Errorf("неверный номер: %s", num)
				}
				return &domain.Epic{ID: epicID, Number: num}, nil
			},
		}
		s := NewEpicService(log, repo)
		epic, err := s.GetEpicByNumber(ctx, "EP-1")
		if err != nil || !called || epic == nil || epic.ID != epicID {
			t.Errorf("ошибка трансляции GetEpicByNumber: err=%v, called=%t, epic=%v", err, called, epic)
		}
	})

	t.Run("GetEpicsByStatus трансляция", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			GetEpicsByStatusFunc: func(ctx context.Context, status domain.Status) ([]domain.Epic, error) {
				called = true
				if status != domain.StatusScoring {
					t.Errorf("неверный статус: %s", status)
				}
				return []domain.Epic{{ID: epicID, Status: status}}, nil
			},
		}
		s := NewEpicService(log, repo)
		epics, err := s.GetEpicsByStatus(ctx, domain.StatusScoring)
		if err != nil || !called || len(epics) != 1 || epics[0].ID != epicID {
			t.Errorf("ошибка трансляции GetEpicsByStatus: err=%v, called=%t, epics=%v", err, called, epics)
		}
	})

	t.Run("GetAllEpics трансляция", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			GetAllEpicsFunc: func(ctx context.Context) ([]domain.Epic, error) {
				called = true
				return []domain.Epic{{ID: epicID}}, nil
			},
		}
		s := NewEpicService(log, repo)
		epics, err := s.GetAllEpics(ctx)
		if err != nil || !called || len(epics) != 1 || epics[0].ID != epicID {
			t.Errorf("ошибка трансляции GetAllEpics: err=%v, called=%t, epics=%v", err, called, epics)
		}
	})

	t.Run("GetUnscoredEpicsByUser трансляция", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			GetUnscoredEpicsByUserFunc: func(ctx context.Context, uID, tID uuid.UUID) ([]domain.Epic, error) {
				called = true
				if uID != userID || tID != teamID {
					t.Errorf("неверные параметры: uID=%v, tID=%v", uID, tID)
				}
				return []domain.Epic{{ID: epicID}}, nil
			},
		}
		s := NewEpicService(log, repo)
		epics, err := s.GetUnscoredEpicsByUser(ctx, userID, teamID)
		if err != nil || !called || len(epics) != 1 || epics[0].ID != epicID {
			t.Errorf("ошибка трансляции GetUnscoredEpicsByUser: err=%v, called=%t, epics=%v", err, called, epics)
		}
	})

	t.Run("UpdateEpicStatus трансляция", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			UpdateEpicStatusFunc: func(ctx context.Context, eID uuid.UUID, status domain.Status) error {
				called = true
				if eID != epicID || status != domain.StatusScored {
					t.Errorf("неверные параметры: eID=%v, status=%s", eID, status)
				}
				return nil
			},
		}
		s := NewEpicService(log, repo)
		err := s.UpdateEpicStatus(ctx, epicID, domain.StatusScored)
		if err != nil || !called {
			t.Errorf("ошибка трансляции UpdateEpicStatus: err=%v, called=%t", err, called)
		}
	})

	t.Run("DeleteEpic трансляция", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			DeleteEpicFunc: func(ctx context.Context, eID uuid.UUID) error {
				called = true
				if eID != epicID {
					t.Errorf("неверный id: %v", eID)
				}
				return nil
			},
		}
		s := NewEpicService(log, repo)
		err := s.DeleteEpic(ctx, epicID)
		if err != nil || !called {
			t.Errorf("ошибка трансляции DeleteEpic: err=%v, called=%t", err, called)
		}
	})

	t.Run("GetEpicsByTeamIDAndStatus трансляция", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			GetEpicsByTeamIDAndStatusFunc: func(ctx context.Context, tID uuid.UUID, status domain.Status) ([]domain.Epic, error) {
				called = true
				if tID != teamID || status != domain.StatusNew {
					t.Errorf("неверные параметры: tID=%v, status=%s", tID, status)
				}
				return []domain.Epic{{ID: epicID}}, nil
			},
		}
		s := NewEpicService(log, repo)
		epics, err := s.GetEpicsByTeamIDAndStatus(ctx, teamID, domain.StatusNew)
		if err != nil || !called || len(epics) != 1 || epics[0].ID != epicID {
			t.Errorf("ошибка трансляции GetEpicsByTeamIDAndStatus: err=%v, called=%t, epics=%v", err, called, epics)
		}
	})

	t.Run("CreateEpicScore трансляция", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			CreateEpicScoreFunc: func(ctx context.Context, eID, uID, rID uuid.UUID, val int) error {
				called = true
				if eID != epicID || uID != userID || rID != roleID || val != 8 {
					t.Errorf("неверные параметры: eID=%v, uID=%v, rID=%v, val=%d", eID, uID, rID, val)
				}
				return nil
			},
		}
		s := NewEpicService(log, repo)
		err := s.CreateEpicScore(ctx, epicID, userID, roleID, 8)
		if err != nil || !called {
			t.Errorf("ошибка трансляции CreateEpicScore: err=%v, called=%t", err, called)
		}
	})

	t.Run("HasUserScoredEpic трансляция", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			HasUserScoredEpicFunc: func(ctx context.Context, eID, uID uuid.UUID) (bool, error) {
				called = true
				if eID != epicID || uID != userID {
					t.Errorf("неверные параметры: eID=%v, uID=%v", eID, uID)
				}
				return true, nil
			},
		}
		s := NewEpicService(log, repo)
		hasScored, err := s.HasUserScoredEpic(ctx, epicID, userID)
		if err != nil || !called || !hasScored {
			t.Errorf("ошибка трансляции HasUserScoredEpic: err=%v, called=%t, hasScored=%t", err, called, hasScored)
		}
	})

	t.Run("GetUsersWhoScoredEpic трансляция", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			GetUsersWhoScoredEpicFunc: func(ctx context.Context, eID uuid.UUID) ([]domain.User, error) {
				called = true
				if eID != epicID {
					t.Errorf("неверный epicID: %v", eID)
				}
				return []domain.User{{ID: userID}}, nil
			},
		}
		s := NewEpicService(log, repo)
		users, err := s.GetUsersWhoScoredEpic(ctx, epicID)
		if err != nil || !called || len(users) != 1 || users[0].ID != userID {
			t.Errorf("ошибка трансляции GetUsersWhoScoredEpic: err=%v, called=%t, users=%v", err, called, users)
		}
	})

	t.Run("GetEpicRoleScoresByEpicID трансляция успешный сценарий и ошибка", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			GetEpicRoleScoresByEpicIDFunc: func(ctx context.Context, eID uuid.UUID) ([]domain.EpicRoleScore, error) {
				called = true
				if eID != epicID {
					t.Errorf("неверный epicID: %v", eID)
				}
				return []domain.EpicRoleScore{{EpicID: eID, RoleID: roleID, WeightedAvg: 14.5}}, nil
			},
		}
		s := NewEpicService(log, repo)
		scores, err := s.GetEpicRoleScoresByEpicID(ctx, epicID)
		if err != nil || !called || len(scores) != 1 || scores[0].RoleID != roleID {
			t.Errorf("ошибка трансляции GetEpicRoleScoresByEpicID: err=%v, called=%t, scores=%v", err, called, scores)
		}

		repo.GetEpicRoleScoresByEpicIDFunc = func(ctx context.Context, eID uuid.UUID) ([]domain.EpicRoleScore, error) {
			return nil, dbErr
		}
		_, err = s.GetEpicRoleScoresByEpicID(ctx, epicID)
		if !errors.Is(err, dbErr) {
			t.Errorf("ожидалась ошибка %v, получена: %v", dbErr, err)
		}
	})
}
