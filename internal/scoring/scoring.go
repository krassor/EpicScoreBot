package scoring

import (
	"EpicScoreBot/internal/models/domain"
	"EpicScoreBot/internal/repositories"
	"context"
	"fmt"
	"log/slog"
	"math"

	"github.com/google/uuid"
)

// Service provides scoring business logic.
type Service struct {
	repo *repositories.Repository
	log  *slog.Logger
}

// New creates a new scoring service.
func New(logger *slog.Logger, repo *repositories.Repository) *Service {
	return &Service{
		repo: repo,
		log:  logger.With(slog.String("component", "scoring")),
	}
}

// CalculateEpicRoleAvg computes the weighted average score
// for a specific role on an epic.
// Formula: Σ(score_i × weight_i) / Σ(weight_i)
func (s *Service) CalculateEpicRoleAvg(ctx context.Context, epicID, roleID uuid.UUID) (float64, error) {
	op := "scoring.CalculateEpicRoleAvg"

	scores, err := s.repo.GetEpicScoresByEpicIDAndRoleID(ctx, epicID, roleID)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	if len(scores) == 0 {
		return 0, nil
	}

	var weightedSum float64
	var totalWeight float64

	for _, sc := range scores {
		user, err := s.repo.GetUserByID(ctx, sc.UserID)
		if err != nil {
			return 0, fmt.Errorf("%s: get user: %w", op, err)
		}
		w := float64(user.Weight)
		weightedSum += float64(sc.Score) * w
		totalWeight += w
	}

	if totalWeight == 0 {
		return 0, nil
	}

	return weightedSum / totalWeight, nil
}

// RiskCoefficient maps a weighted risk score to a multiplier coefficient.
func RiskCoefficient(weightedScore float64) float64 {
	rounded := math.Round(weightedScore)
	switch {
	case rounded >= 13:
		return 1.30
	case rounded >= 9:
		return 1.20
	case rounded >= 5:
		return 1.10
	default:
		return 1.05
	}
}

// CalculateRiskWeightedScore computes the weighted average risk score.
// Each user's risk score = probability × impact.
// weighted_avg = Σ(score_i × weight_i) / Σ(weight_i)
func (s *Service) CalculateRiskWeightedScore(ctx context.Context, riskID uuid.UUID) (float64, error) {
	op := "scoring.CalculateRiskWeightedScore"

	riskScores, err := s.repo.GetRiskScoresByRiskID(ctx, riskID)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	if len(riskScores) == 0 {
		return 0, nil
	}

	var weightedSum float64
	var totalWeight float64

	for _, rs := range riskScores {
		user, err := s.repo.GetUserByID(ctx, rs.UserID)
		if err != nil {
			return 0, fmt.Errorf("%s: get user: %w", op, err)
		}
		userScore := float64(rs.Probability * rs.Impact)
		w := float64(user.Weight)
		weightedSum += userScore * w
		totalWeight += w
	}

	if totalWeight == 0 {
		return 0, nil
	}

	return weightedSum / totalWeight, nil
}

// TryCompleteRiskScoring checks if all team members have scored a risk.
// If so, calculates the weighted score and saves it.
func (s *Service) TryCompleteRiskScoring(ctx context.Context, riskID uuid.UUID) error {
	op := "scoring.TryCompleteRiskScoring"
	log := slog.With(
		slog.String("op", op),
	)

	risk, err := s.repo.GetRiskByID(ctx, riskID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	epic, err := s.repo.GetEpicByID(ctx, risk.EpicID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	teamMembers, err := s.repo.CountTeamMembers(ctx, epic.TeamID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	riskScoreCount, err := s.repo.CountRiskScores(ctx, riskID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if riskScoreCount < teamMembers {
		log.Debug("risk scoring not complete yet",
			slog.String("riskID", riskID.String()),
			slog.Int("scored", riskScoreCount),
			slog.Int("total", teamMembers))
		return nil
	}

	weightedScore, err := s.CalculateRiskWeightedScore(ctx, riskID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if err := s.repo.SetRiskWeightedScore(ctx, riskID, weightedScore); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	log.Info("risk scoring completed",
		slog.String("riskID", riskID.String()),
		slog.Float64("weightedScore", weightedScore),
		slog.Float64("coefficient", RiskCoefficient(weightedScore)))

	// Try to complete the epic scoring too
	return s.TryCompleteEpicScoring(ctx, risk.EpicID)
}

// TryCompleteEpicScoring checks if all team members have scored an epic
// and all its risks are scored. If so, calculates the final score.
func (s *Service) TryCompleteEpicScoring(ctx context.Context, epicID uuid.UUID) error {
	op := "scoring.TryCompleteEpicScoring"
	log := slog.With(
		slog.String("op", op),
	)

	epic, err := s.repo.GetEpicByID(ctx, epicID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if epic.Status == domain.StatusScored {
		return nil
	}

	teamMembers, err := s.repo.CountTeamMembers(ctx, epic.TeamID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	epicScoreCount, err := s.repo.CountEpicScores(ctx, epicID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if epicScoreCount < teamMembers {
		log.Debug("epic scoring not complete yet",
			slog.String("epicID", epicID.String()),
			slog.Int("scored", epicScoreCount),
			slog.Int("total", teamMembers))
		return nil
	}

	// Calculate weighted averages per role
	roleIDs, err := s.repo.GetDistinctRoleIDsForEpicScores(ctx, epicID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	var epicBaseScore float64
	for _, roleID := range roleIDs {
		avg, err := s.CalculateEpicRoleAvg(ctx, epicID, roleID)
		if err != nil {
			return fmt.Errorf("%s: role avg: %w", op, err)
		}

		if err := s.repo.UpsertEpicRoleScore(ctx, epicID, roleID, avg); err != nil {
			return fmt.Errorf("%s: upsert role score: %w", op, err)
		}

		epicBaseScore += avg
	}

	// Check if all risks are scored
	risks, err := s.repo.GetRisksByEpicID(ctx, epicID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	for _, risk := range risks {
		if risk.Status != domain.StatusScored {
			log.Debug("waiting for risk scoring",
				slog.String("epicID", epicID.String()),
				slog.String("riskID", risk.ID.String()))
			return nil
		}
	}

	// Apply risk coefficients
	finalScore := epicBaseScore
	for _, risk := range risks {
		if risk.WeightedScore != nil {
			coeff := RiskCoefficient(*risk.WeightedScore)
			finalScore *= coeff
		}
	}

	// Round to integer
	finalScore = math.Round(finalScore)

	if err := s.repo.SetEpicFinalScore(ctx, epicID, finalScore); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	s.log.Info("epic scoring completed",
		slog.String("epicID", epicID.String()),
		slog.Float64("baseScore", epicBaseScore),
		slog.Float64("finalScore", finalScore))

	return nil
}
