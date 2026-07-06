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

type teamService struct {
	log  *slog.Logger
	repo Repository
}

func NewTeamService(log *slog.Logger, repo Repository) TeamService {
	return &teamService{
		log:  log.With(slog.String("service", "team")),
		repo: repo,
	}
}

func (s *teamService) CreateTeam(ctx context.Context, name, description string) (*domain.Team, error) {
	op := "teamService.CreateTeam"
	log := s.log.With(slog.String("op", op), slog.String("team_name", name))

	existing, err := s.repo.GetTeamByName(ctx, name)
	if err == nil && existing != nil {
		log.Warn("team already exists")
		return nil, ErrTeamAlreadyExists
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Error("error checking existing team", sl.Err(err))
		return nil, err
	}

	team, err := s.repo.CreateTeam(ctx, name, description)
	if err != nil {
		log.Error("failed to create team", sl.Err(err))
		return nil, err
	}

	log.Info("team created successfully", slog.String("team_id", team.ID.String()))
	return team, nil
}

func (s *teamService) GetTeamByName(ctx context.Context, name string) (*domain.Team, error) {
	return s.repo.GetTeamByName(ctx, name)
}

func (s *teamService) GetTeamByID(ctx context.Context, teamID uuid.UUID) (*domain.Team, error) {
	return s.repo.GetTeamByID(ctx, teamID)
}

func (s *teamService) GetAllTeams(ctx context.Context) ([]domain.Team, error) {
	return s.repo.GetAllTeams(ctx)
}

func (s *teamService) GetTeamsByUserTelegramID(ctx context.Context, telegramID string) ([]domain.Team, error) {
	return s.repo.GetTeamsByUserTelegramID(ctx, telegramID)
}

func (s *teamService) AssignUserTeam(ctx context.Context, userID, teamID uuid.UUID) error {
	op := "teamService.AssignUserTeam"
	log := s.log.With(slog.String("op", op), slog.String("user_id", userID.String()), slog.String("team_id", teamID.String()))

	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		log.Error("failed to get user", sl.Err(err))
		return err
	}

	teams, err := s.repo.GetTeamsByUserTelegramID(ctx, user.TelegramID)
	if err == nil {
		for _, t := range teams {
			if t.ID == teamID {
				log.Warn("user already assigned to team")
				return nil // or we can return a specific error, but let's keep the existing check or return error
			}
		}
	}

	err = s.repo.AssignUserTeam(ctx, userID, teamID)
	if err != nil {
		log.Error("failed to assign user to team", sl.Err(err))
		return err
	}

	log.Info("assigned user to team")
	return nil
}

func (s *teamService) RemoveUserTeam(ctx context.Context, userID, teamID uuid.UUID) error {
	return s.repo.RemoveUserTeam(ctx, userID, teamID)
}
