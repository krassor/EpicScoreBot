package services

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"testing"

	"EpicScoreBot/internal/models/domain"

	"github.com/google/uuid"
)

// newDiscardLogger создает логгер, который игнорирует все записи, для использования в тестах.
func newDiscardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestUserService_CreateUser(t *testing.T) {
	ctx := context.Background()
	log := newDiscardLogger()

	t.Run("успешное создание пользователя", func(t *testing.T) {
		repo := &MockRepository{
			FindUserByTelegramIDFunc: func(ctx context.Context, telegramID string) (*domain.User, error) {
				return nil, sql.ErrNoRows
			},
			CreateUserFunc: func(ctx context.Context, firstName, lastName, telegramID string, weight int) (*domain.User, error) {
				return &domain.User{
					ID:         uuid.New(),
					FirstName:  firstName,
					LastName:   lastName,
					TelegramID: telegramID,
					Weight:     weight,
				}, nil
			},
		}

		s := NewUserService(log, repo)
		user, err := s.CreateUser(ctx, "Иван", "Иванов", "12345", 80)
		if err != nil {
			t.Fatalf("ожидалось успешное создание, получена ошибка: %v", err)
		}

		if user == nil {
			t.Fatal("ожидался созданный пользователь, получен nil")
		}
		if user.FirstName != "Иван" || user.LastName != "Иванов" || user.TelegramID != "12345" || user.Weight != 80 {
			t.Errorf("неверные данные созданного пользователя: %+v", user)
		}
	})

	t.Run("пользователь уже существует", func(t *testing.T) {
		repo := &MockRepository{
			FindUserByTelegramIDFunc: func(ctx context.Context, telegramID string) (*domain.User, error) {
				return &domain.User{
					ID:         uuid.New(),
					TelegramID: telegramID,
				}, nil
			},
		}

		s := NewUserService(log, repo)
		user, err := s.CreateUser(ctx, "Иван", "Иванов", "12345", 80)
		if !errors.Is(err, ErrUserAlreadyExists) {
			t.Fatalf("ожидалась ошибка %v, получена: %v", ErrUserAlreadyExists, err)
		}
		if user != nil {
			t.Fatal("ожидался nil-пользователь")
		}
	})

	t.Run("ошибка репозитория при поиске существующего", func(t *testing.T) {
		dbErr := errors.New("database failure")
		repo := &MockRepository{
			FindUserByTelegramIDFunc: func(ctx context.Context, telegramID string) (*domain.User, error) {
				return nil, dbErr
			},
		}

		s := NewUserService(log, repo)
		user, err := s.CreateUser(ctx, "Иван", "Иванов", "12345", 80)
		if !errors.Is(err, dbErr) {
			t.Fatalf("ожидалась ошибка %v, получена: %v", dbErr, err)
		}
		if user != nil {
			t.Fatal("ожидался nil-пользователь")
		}
	})

	t.Run("ошибка репозитория при создании", func(t *testing.T) {
		dbErr := errors.New("database failure on create")
		repo := &MockRepository{
			FindUserByTelegramIDFunc: func(ctx context.Context, telegramID string) (*domain.User, error) {
				return nil, sql.ErrNoRows
			},
			CreateUserFunc: func(ctx context.Context, firstName, lastName, telegramID string, weight int) (*domain.User, error) {
				return nil, dbErr
			},
		}

		s := NewUserService(log, repo)
		user, err := s.CreateUser(ctx, "Иван", "Иванов", "12345", 80)
		if !errors.Is(err, dbErr) {
			t.Fatalf("ожидалась ошибка %v, получена: %v", dbErr, err)
		}
		if user != nil {
			t.Fatal("ожидался nil-пользователь")
		}
	})
}

func TestUserService_OtherMethods(t *testing.T) {
	ctx := context.Background()
	log := newDiscardLogger()
	userID := uuid.New()
	teamID := uuid.New()

	t.Run("FindUserByTelegramID трансляция", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			FindUserByTelegramIDFunc: func(ctx context.Context, telegramID string) (*domain.User, error) {
				called = true
				if telegramID != "tg-123" {
					t.Errorf("неверный telegramID: %s", telegramID)
				}
				return &domain.User{ID: userID, TelegramID: telegramID}, nil
			},
		}
		s := NewUserService(log, repo)
		user, err := s.FindUserByTelegramID(ctx, "tg-123")
		if err != nil || !called || user == nil || user.ID != userID {
			t.Errorf("ошибка трансляции FindUserByTelegramID: err=%v, called=%t, user=%v", err, called, user)
		}
	})

	t.Run("GetUserByID трансляция", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			GetUserByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
				called = true
				if id != userID {
					t.Errorf("неверный id: %v", id)
				}
				return &domain.User{ID: id}, nil
			},
		}
		s := NewUserService(log, repo)
		user, err := s.GetUserByID(ctx, userID)
		if err != nil || !called || user == nil || user.ID != userID {
			t.Errorf("ошибка трансляции GetUserByID: err=%v, called=%t, user=%v", err, called, user)
		}
	})

	t.Run("GetUsersByTeamID трансляция", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			GetUsersByTeamIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.User, error) {
				called = true
				if id != teamID {
					t.Errorf("неверный id: %v", id)
				}
				return []domain.User{{ID: userID}}, nil
			},
		}
		s := NewUserService(log, repo)
		users, err := s.GetUsersByTeamID(ctx, teamID)
		if err != nil || !called || len(users) != 1 || users[0].ID != userID {
			t.Errorf("ошибка трансляции GetUsersByTeamID: err=%v, called=%t, users=%v", err, called, users)
		}
	})

	t.Run("GetAllUsers трансляция", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			GetAllUsersFunc: func(ctx context.Context) ([]domain.User, error) {
				called = true
				return []domain.User{{ID: userID}}, nil
			},
		}
		s := NewUserService(log, repo)
		users, err := s.GetAllUsers(ctx)
		if err != nil || !called || len(users) != 1 || users[0].ID != userID {
			t.Errorf("ошибка трансляции GetAllUsers: err=%v, called=%t, users=%v", err, called, users)
		}
	})

	t.Run("DeleteUser трансляция", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			DeleteUserFunc: func(ctx context.Context, id uuid.UUID) error {
				called = true
				if id != userID {
					t.Errorf("неверный id: %v", id)
				}
				return nil
			},
		}
		s := NewUserService(log, repo)
		err := s.DeleteUser(ctx, userID)
		if err != nil || !called {
			t.Errorf("ошибка трансляции DeleteUser: err=%v, called=%t", err, called)
		}
	})

	t.Run("UpdateUserName трансляция", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			UpdateUserNameFunc: func(ctx context.Context, id uuid.UUID, fn, ln string) error {
				called = true
				if id != userID || fn != "A" || ln != "B" {
					t.Errorf("неверные параметры: id=%v, fn=%s, ln=%s", id, fn, ln)
				}
				return nil
			},
		}
		s := NewUserService(log, repo)
		err := s.UpdateUserName(ctx, userID, "A", "B")
		if err != nil || !called {
			t.Errorf("ошибка трансляции UpdateUserName: err=%v, called=%t", err, called)
		}
	})

	t.Run("UpdateUserWeight трансляция", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			UpdateUserWeightFunc: func(ctx context.Context, id uuid.UUID, weight int) error {
				called = true
				if id != userID || weight != 75 {
					t.Errorf("неверные параметры: id=%v, weight=%d", id, weight)
				}
				return nil
			},
		}
		s := NewUserService(log, repo)
		err := s.UpdateUserWeight(ctx, userID, 75)
		if err != nil || !called {
			t.Errorf("ошибка трансляции UpdateUserWeight: err=%v, called=%t", err, called)
		}
	})

	t.Run("UpdateUserChatID трансляция", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			UpdateUserChatIDFunc: func(ctx context.Context, id uuid.UUID, chatID int64) error {
				called = true
				if id != userID || chatID != 456 {
					t.Errorf("неверные параметры: id=%v, chatID=%d", id, chatID)
				}
				return nil
			},
		}
		s := NewUserService(log, repo)
		err := s.UpdateUserChatID(ctx, userID, 456)
		if err != nil || !called {
			t.Errorf("ошибка трансляции UpdateUserChatID: err=%v, called=%t", err, called)
		}
	})
}
