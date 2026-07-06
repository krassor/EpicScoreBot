package services

import (
	"context"
	"log/slog"

	"EpicScoreBot/internal/models/domain"

	"github.com/google/uuid"
)

type riskService struct {
	log  *slog.Logger
	repo Repository
}

func NewRiskService(log *slog.Logger, repo Repository) RiskService {
	return &riskService{
		log:  log.With(slog.String("service", "risk")),
		repo: repo,
	}
}

func (s *riskService) CreateRisk(ctx context.Context, description string, epicID uuid.UUID) (*domain.Risk, error) {
	return s.repo.CreateRisk(ctx, description, epicID)
}

func (s *riskService) GetRisksByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.Risk, error) {
	return s.repo.GetRisksByEpicID(ctx, epicID)
}

func (s *riskService) GetRiskByID(ctx context.Context, riskID uuid.UUID) (*domain.Risk, error) {
	return s.repo.GetRiskByID(ctx, riskID)
}

func (s *riskService) GetUnscoredRisksByUser(ctx context.Context, userID, epicID uuid.UUID) ([]domain.Risk, error) {
	return s.repo.GetUnscoredRisksByUser(ctx, userID, epicID)
}

func (s *riskService) UpdateRiskStatus(ctx context.Context, riskID uuid.UUID, status domain.Status) error {
	return s.repo.UpdateRiskStatus(ctx, riskID, status)
}

func (s *riskService) DeleteRisk(ctx context.Context, riskID uuid.UUID) error {
	return s.repo.DeleteRisk(ctx, riskID)
}

func (s *riskService) CreateRiskScore(ctx context.Context, riskID, userID uuid.UUID, probability, impact int) error {
	return s.repo.CreateRiskScore(ctx, riskID, userID, probability, impact)
}

func (s *riskService) GetRiskScoresByRiskID(ctx context.Context, riskID uuid.UUID) ([]domain.RiskScore, error) {
	return s.repo.GetRiskScoresByRiskID(ctx, riskID)
}

func (s *riskService) GetUsersWhoScoredRisk(ctx context.Context, riskID uuid.UUID) ([]domain.User, error) {
	return s.repo.GetUsersWhoScoredRisk(ctx, riskID)
}
