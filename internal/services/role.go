package services

import (
	"context"
	"log/slog"

	"EpicScoreBot/internal/models/domain"

	"github.com/google/uuid"
)

type roleService struct {
	log  *slog.Logger
	repo Repository
}

func NewRoleService(log *slog.Logger, repo Repository) RoleService {
	return &roleService{
		log:  log.With(slog.String("service", "role")),
		repo: repo,
	}
}

func (s *roleService) GetAllRoles(ctx context.Context) ([]domain.Role, error) {
	return s.repo.GetAllRoles(ctx)
}

func (s *roleService) GetRoleByID(ctx context.Context, roleID uuid.UUID) (*domain.Role, error) {
	return s.repo.GetRoleByID(ctx, roleID)
}

func (s *roleService) GetRoleByUserID(ctx context.Context, userID uuid.UUID) (*domain.Role, error) {
	return s.repo.GetRoleByUserID(ctx, userID)
}

func (s *roleService) AssignUserRole(ctx context.Context, userID, roleID uuid.UUID) error {
	return s.repo.AssignUserRole(ctx, userID, roleID)
}

func (s *roleService) RemoveUserRole(ctx context.Context, userID, roleID uuid.UUID) error {
	return s.repo.RemoveUserRole(ctx, userID, roleID)
}
