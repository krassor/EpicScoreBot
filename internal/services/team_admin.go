package services

import (
	"context"
	"log/slog"

	"EpicScoreBot/internal/models/domain"

	"github.com/google/uuid"
)

type teamAdminService struct {
	log  *slog.Logger
	repo Repository
}

// NewTeamAdminService создаёт сервис бизнес-логики team-admin (назначение
// снятие и проверки доступа поверх таблицы team_admins).
func NewTeamAdminService(log *slog.Logger, repo Repository) TeamAdminService {
	return &teamAdminService{
		log:  log.With(slog.String("service", "team_admin")),
		repo: repo,
	}
}

func (s *teamAdminService) AssignTeamAdmin(ctx context.Context, userID, teamID, assignedBy uuid.UUID) error {
	op := "teamAdminService.AssignTeamAdmin"
	log := s.log.With(slog.String("op", op),
		slog.String("user_id", userID.String()), slog.String("team_id", teamID.String()))

	if err := s.repo.AssignTeamAdmin(ctx, userID, teamID, assignedBy); err != nil {
		log.Error("failed to assign team admin", slog.String("error", err.Error()))
		return err
	}
	log.Info("team admin assigned")
	return nil
}

func (s *teamAdminService) RemoveTeamAdmin(ctx context.Context, userID, teamID uuid.UUID) error {
	op := "teamAdminService.RemoveTeamAdmin"
	log := s.log.With(slog.String("op", op),
		slog.String("user_id", userID.String()), slog.String("team_id", teamID.String()))

	if err := s.repo.RemoveTeamAdmin(ctx, userID, teamID); err != nil {
		log.Error("failed to remove team admin", slog.String("error", err.Error()))
		return err
	}
	log.Info("team admin removed")
	return nil
}

func (s *teamAdminService) GetTeamAdminsByTeamID(ctx context.Context, teamID uuid.UUID) ([]domain.User, error) {
	return s.repo.GetTeamAdminsByTeamID(ctx, teamID)
}

func (s *teamAdminService) GetTeamIDsByAdminUserID(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	return s.repo.GetTeamIDsByAdminUserID(ctx, userID)
}

func (s *teamAdminService) IsTeamAdmin(ctx context.Context, userID, teamID uuid.UUID) (bool, error) {
	return s.repo.IsTeamAdmin(ctx, userID, teamID)
}

func (s *teamAdminService) IsTeamAdminOfAny(ctx context.Context, userID uuid.UUID) (bool, error) {
	return s.repo.IsTeamAdminOfAny(ctx, userID)
}
