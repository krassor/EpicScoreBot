package services

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"EpicScoreBot/internal/models/domain"

	"github.com/google/uuid"
)

func TestTeamService_CreateTeam(t *testing.T) {
	ctx := context.Background()
	log := newDiscardLogger()

	t.Run("успешное создание команды", func(t *testing.T) {
		repo := &MockRepository{
			GetTeamByNameFunc: func(ctx context.Context, name string) (*domain.Team, error) {
				return nil, sql.ErrNoRows
			},
			CreateTeamFunc: func(ctx context.Context, name, description string) (*domain.Team, error) {
				return &domain.Team{
					ID:          uuid.New(),
					Name:        name,
					Description: description,
				}, nil
			},
		}

		s := NewTeamService(log, repo)
		team, err := s.CreateTeam(ctx, "Команда А", "Описание команды")
		if err != nil {
			t.Fatalf("ожидалось успешное создание, получена ошибка: %v", err)
		}
		if team == nil {
			t.Fatal("ожидалась созданная команда, получен nil")
		}
		if team.Name != "Команда А" || team.Description != "Описание команды" {
			t.Errorf("неверные данные созданной команды: %+v", team)
		}
	})

	t.Run("команда уже существует", func(t *testing.T) {
		repo := &MockRepository{
			GetTeamByNameFunc: func(ctx context.Context, name string) (*domain.Team, error) {
				return &domain.Team{
					ID:   uuid.New(),
					Name: name,
				}, nil
			},
		}

		s := NewTeamService(log, repo)
		team, err := s.CreateTeam(ctx, "Команда А", "Описание")
		if !errors.Is(err, ErrTeamAlreadyExists) {
			t.Fatalf("ожидалась ошибка %v, получена: %v", ErrTeamAlreadyExists, err)
		}
		if team != nil {
			t.Fatal("ожидалась nil-команда")
		}
	})

	t.Run("ошибка репозитория при поиске существующей", func(t *testing.T) {
		dbErr := errors.New("db error")
		repo := &MockRepository{
			GetTeamByNameFunc: func(ctx context.Context, name string) (*domain.Team, error) {
				return nil, dbErr
			},
		}

		s := NewTeamService(log, repo)
		team, err := s.CreateTeam(ctx, "Команда А", "Описание")
		if !errors.Is(err, dbErr) {
			t.Fatalf("ожидалась ошибка %v, получена: %v", dbErr, err)
		}
		if team != nil {
			t.Fatal("ожидалась nil-команда")
		}
	})

	t.Run("ошибка репозитория при создании", func(t *testing.T) {
		dbErr := errors.New("db create error")
		repo := &MockRepository{
			GetTeamByNameFunc: func(ctx context.Context, name string) (*domain.Team, error) {
				return nil, sql.ErrNoRows
			},
			CreateTeamFunc: func(ctx context.Context, name, description string) (*domain.Team, error) {
				return nil, dbErr
			},
		}

		s := NewTeamService(log, repo)
		team, err := s.CreateTeam(ctx, "Команда А", "Описание")
		if !errors.Is(err, dbErr) {
			t.Fatalf("ожидалась ошибка %v, получена: %v", dbErr, err)
		}
		if team != nil {
			t.Fatal("ожидалась nil-команда")
		}
	})
}

func TestTeamService_AssignUserTeam(t *testing.T) {
	ctx := context.Background()
	log := newDiscardLogger()
	userID := uuid.New()
	teamID := uuid.New()

	t.Run("успешное назначение команды пользователю", func(t *testing.T) {
		repo := &MockRepository{
			GetUserByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
				return &domain.User{ID: userID, TelegramID: "tg-123"}, nil
			},
			GetTeamsByUserTelegramIDFunc: func(ctx context.Context, telegramID string) ([]domain.Team, error) {
				return []domain.Team{}, nil
			},
			AssignUserTeamFunc: func(ctx context.Context, uID, tID uuid.UUID) error {
				if uID != userID || tID != teamID {
					t.Errorf("неверные параметры назначения: uID=%v, tID=%v", uID, tID)
				}
				return nil
			},
		}

		s := NewTeamService(log, repo)
		err := s.AssignUserTeam(ctx, userID, teamID)
		if err != nil {
			t.Fatalf("ожидалось успешное назначение, получена ошибка: %v", err)
		}
	})

	t.Run("пользователь уже состоит в команде", func(t *testing.T) {
		repo := &MockRepository{
			GetUserByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
				return &domain.User{ID: userID, TelegramID: "tg-123"}, nil
			},
			GetTeamsByUserTelegramIDFunc: func(ctx context.Context, telegramID string) ([]domain.Team, error) {
				return []domain.Team{{ID: teamID}}, nil
			},
		}

		s := NewTeamService(log, repo)
		err := s.AssignUserTeam(ctx, userID, teamID)
		if err != nil {
			t.Fatalf("ожидался успешный возврат (nil), получена ошибка: %v", err)
		}
	})

	t.Run("ошибка при получении пользователя", func(t *testing.T) {
		dbErr := errors.New("user not found")
		repo := &MockRepository{
			GetUserByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
				return nil, dbErr
			},
		}

		s := NewTeamService(log, repo)
		err := s.AssignUserTeam(ctx, userID, teamID)
		if !errors.Is(err, dbErr) {
			t.Fatalf("ожидалась ошибка %v, получена: %v", dbErr, err)
		}
	})

	t.Run("ошибка при получении команд пользователя", func(t *testing.T) {
		dbErr := errors.New("teams fetch error")
		repo := &MockRepository{
			GetUserByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
				return &domain.User{ID: userID, TelegramID: "tg-123"}, nil
			},
			GetTeamsByUserTelegramIDFunc: func(ctx context.Context, telegramID string) ([]domain.Team, error) {
				return nil, dbErr
			},
			AssignUserTeamFunc: func(ctx context.Context, uID, tID uuid.UUID) error {
				return nil
			},
		}

		s := NewTeamService(log, repo)
		err := s.AssignUserTeam(ctx, userID, teamID)
		if err != nil {
			t.Fatalf("ожидалось успешное назначение при ошибке получения существующих (т.к. ошибка GetTeamsByUserTelegramID игнорируется в коде), получена ошибка: %v", err)
		}
	})

	t.Run("ошибка репозитория при назначении", func(t *testing.T) {
		dbErr := errors.New("assign error")
		repo := &MockRepository{
			GetUserByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
				return &domain.User{ID: userID, TelegramID: "tg-123"}, nil
			},
			GetTeamsByUserTelegramIDFunc: func(ctx context.Context, telegramID string) ([]domain.Team, error) {
				return []domain.Team{}, nil
			},
			AssignUserTeamFunc: func(ctx context.Context, uID, tID uuid.UUID) error {
				return dbErr
			},
		}

		s := NewTeamService(log, repo)
		err := s.AssignUserTeam(ctx, userID, teamID)
		if !errors.Is(err, dbErr) {
			t.Fatalf("ожидалась ошибка %v, получена: %v", dbErr, err)
		}
	})
}

func TestTeamService_OtherMethods(t *testing.T) {
	ctx := context.Background()
	log := newDiscardLogger()
	userID := uuid.New()
	teamID := uuid.New()

	t.Run("GetTeamByName трансляция", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			GetTeamByNameFunc: func(ctx context.Context, name string) (*domain.Team, error) {
				called = true
				if name != "Team-A" {
					t.Errorf("неверный name: %s", name)
				}
				return &domain.Team{ID: teamID, Name: name}, nil
			},
		}
		s := NewTeamService(log, repo)
		team, err := s.GetTeamByName(ctx, "Team-A")
		if err != nil || !called || team == nil || team.ID != teamID {
			t.Errorf("ошибка трансляции GetTeamByName: err=%v, called=%t, team=%v", err, called, team)
		}
	})

	t.Run("GetTeamByID трансляция", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			GetTeamByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
				called = true
				if id != teamID {
					t.Errorf("неверный id: %v", id)
				}
				return &domain.Team{ID: id}, nil
			},
		}
		s := NewTeamService(log, repo)
		team, err := s.GetTeamByID(ctx, teamID)
		if err != nil || !called || team == nil || team.ID != teamID {
			t.Errorf("ошибка трансляции GetTeamByID: err=%v, called=%t, team=%v", err, called, team)
		}
	})

	t.Run("GetAllTeams трансляция", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			GetAllTeamsFunc: func(ctx context.Context) ([]domain.Team, error) {
				called = true
				return []domain.Team{{ID: teamID}}, nil
			},
		}
		s := NewTeamService(log, repo)
		teams, err := s.GetAllTeams(ctx)
		if err != nil || !called || len(teams) != 1 || teams[0].ID != teamID {
			t.Errorf("ошибка трансляции GetAllTeams: err=%v, called=%t, teams=%v", err, called, teams)
		}
	})

	t.Run("GetTeamsByUserTelegramID трансляция", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			GetTeamsByUserTelegramIDFunc: func(ctx context.Context, telegramID string) ([]domain.Team, error) {
				called = true
				if telegramID != "tg-123" {
					t.Errorf("неверный telegramID: %s", telegramID)
				}
				return []domain.Team{{ID: teamID}}, nil
			},
		}
		s := NewTeamService(log, repo)
		teams, err := s.GetTeamsByUserTelegramID(ctx, "tg-123")
		if err != nil || !called || len(teams) != 1 || teams[0].ID != teamID {
			t.Errorf("ошибка трансляции GetTeamsByUserTelegramID: err=%v, called=%t, teams=%v", err, called, teams)
		}
	})

	t.Run("RemoveUserTeam трансляция", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			RemoveUserTeamFunc: func(ctx context.Context, uID, tID uuid.UUID) error {
				called = true
				if uID != userID || tID != teamID {
					t.Errorf("неверные параметры: uID=%v, tID=%v", uID, tID)
				}
				return nil
			},
		}
		s := NewTeamService(log, repo)
		err := s.RemoveUserTeam(ctx, userID, teamID)
		if err != nil || !called {
			t.Errorf("ошибка трансляции RemoveUserTeam: err=%v, called=%t", err, called)
		}
	})
}
