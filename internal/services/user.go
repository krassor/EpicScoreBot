package services

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"

	"EpicScoreBot/internal/models/domain"
	"EpicScoreBot/internal/utils/logger/sl"

	"github.com/google/uuid"
)

type userService struct {
	log  *slog.Logger
	repo Repository
}

func NewUserService(log *slog.Logger, repo Repository) UserService {
	return &userService{
		log:  log.With(slog.String("service", "user")),
		repo: repo,
	}
}

func (s *userService) CreateUser(ctx context.Context, firstName, lastName, telegramID string, weight int) (*domain.User, error) {
	op := "userService.CreateUser"
	log := s.log.With(slog.String("op", op), slog.String("telegram_id", telegramID))

	existing, err := s.repo.FindUserByTelegramID(ctx, telegramID)
	if err == nil && existing != nil {
		log.Warn("user already exists")
		return nil, ErrUserAlreadyExists
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Error("error checking existing user", sl.Err(err))
		return nil, err
	}

	user, err := s.repo.CreateUser(ctx, firstName, lastName, telegramID, weight)
	if err != nil {
		log.Error("failed to create user", sl.Err(err))
		return nil, err
	}

	log.Info("user created successfully", slog.String("user_id", user.ID.String()))
	return user, nil
}

func (s *userService) FindUserByTelegramID(ctx context.Context, telegramID string) (*domain.User, error) {
	return s.repo.FindUserByTelegramID(ctx, telegramID)
}

func (s *userService) GetUserByID(ctx context.Context, userID uuid.UUID) (*domain.User, error) {
	return s.repo.GetUserByID(ctx, userID)
}

func (s *userService) GetUsersByTeamID(ctx context.Context, teamID uuid.UUID) ([]domain.User, error) {
	return s.repo.GetUsersByTeamID(ctx, teamID)
}

func (s *userService) GetAllUsers(ctx context.Context) ([]domain.User, error) {
	return s.repo.GetAllUsers(ctx)
}

func (s *userService) DeleteUser(ctx context.Context, userID uuid.UUID) error {
	return s.repo.DeleteUser(ctx, userID)
}

func (s *userService) UpdateUserName(ctx context.Context, userID uuid.UUID, firstName, lastName string) error {
	return s.repo.UpdateUserName(ctx, userID, firstName, lastName)
}

func (s *userService) UpdateUserWeight(ctx context.Context, userID uuid.UUID, weight int) error {
	return s.repo.UpdateUserWeight(ctx, userID, weight)
}

func (s *userService) UpdateUserChatID(ctx context.Context, userID uuid.UUID, chatID int64) error {
	return s.repo.UpdateUserChatID(ctx, userID, chatID)
}
