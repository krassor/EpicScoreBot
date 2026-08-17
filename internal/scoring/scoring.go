package scoring

import (
	"EpicScoreBot/internal/models/domain"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"

	"github.com/google/uuid"
)

// ErrScoringNotComplete возвращается, когда запрошена операция,
// требующая завершённого скоринга эпика/стори (статус SCORED),
// но фактический статус эпика этому не соответствует.
var ErrScoringNotComplete = errors.New("epic scoring is not completed yet")

// Service provides scoring business logic.
type Service struct {
	repo Repository
	log  *slog.Logger
}

// New creates a new scoring service.
func New(logger *slog.Logger, repo Repository) *Service {
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

// TODO: rewrite this func. In scoring.go finalCoeff must be saved in db
// RiskCoefficient maps a weighted risk score to a multiplier coefficient.
func RiskCoefficient(weightedScore float64) float64 {
	rounded := math.Round(weightedScore)
	switch {
	case rounded >= 13:
		return 1.20
	case rounded >= 9:
		return 1.10
	case rounded >= 5:
		return 1.05
	default:
		return 1.03
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

	teamMembers, err := s.repo.GetExpectedScorersCount(ctx, epic.ID, epic.TeamID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	riskScoreCount, err := s.repo.GetSubmittedRiskScorersCount(ctx, riskID, epic.ID, epic.TeamID)
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

	// Примечание: ранее здесь был ранний выход при epic.Status == domain.StatusScored.
	// Он убран намеренно: метод должен корректно пересчитывать final_score и при
	// повторном вызове (например, после того как админ переотправил оценку участника
	// или риска post-factum через AdminSubmitEpicScore/AdminSubmitRiskScore).
	// Метод идемпотентен: UpsertEpicRoleScore и SetEpicFinalScore — это upsert/UPDATE,
	// повторный вызов безопасен.

	teamMembers, err := s.repo.GetExpectedScorersCount(ctx, epicID, epic.TeamID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	epicScoreCount, err := s.repo.GetSubmittedEpicScorersCount(ctx, epicID, epic.TeamID)
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

	s.log.Info("epic/story scoring completed",
		slog.String("epicID", epicID.String()),
		slog.Float64("baseScore", epicBaseScore),
		slog.Float64("finalScore", finalScore))

	// If this is a story, check if all sibling stories are scored and aggregate them to the parent epic
	if epic.ParentEpicID != nil {
		if err := s.recalcParentEpic(ctx, epic, finalScore); err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
	}

	return nil
}

// recalcParentEpic пересчитывает агрегированную оценку родительского эпика
// на основе дочерних сторей. story — только что пересчитанная стори,
// finalScore — её новое итоговое значение (может ещё не быть сохранено в story.FinalScore).
// Родитель обновляется, только если у всех сиблинг-сторей статус SCORED.
func (s *Service) recalcParentEpic(ctx context.Context, story *domain.Epic, finalScore float64) error {
	op := "scoring.recalcParentEpic"

	if story.ParentEpicID == nil {
		return nil
	}
	parentID := *story.ParentEpicID

	stories, err := s.repo.GetStoriesByEpicID(ctx, parentID)
	if err != nil {
		return fmt.Errorf("%s: get stories for parent: %w", op, err)
	}

	allCompleted := true
	var parentFinalScore float64
	roleTotals := make(map[uuid.UUID]float64)

	for _, sibling := range stories {
		actualStory := sibling
		if sibling.ID == story.ID {
			actualStory = *story
			actualStory.Status = domain.StatusScored
			actualStory.FinalScore = &finalScore
		} else {
			st, err := s.repo.GetEpicByID(ctx, sibling.ID)
			if err != nil {
				return fmt.Errorf("%s: get actual story status: %w", op, err)
			}
			actualStory = *st
		}

		if actualStory.Status != domain.StatusScored || actualStory.FinalScore == nil {
			allCompleted = false
			break
		}
		parentFinalScore += *actualStory.FinalScore

		// Aggregate role scores for this story
		storyRoleScores, err := s.repo.GetEpicRoleScoresByEpicID(ctx, actualStory.ID)
		if err == nil {
			for _, rs := range storyRoleScores {
				roleTotals[rs.RoleID] += rs.WeightedAvg
			}
		}
	}

	if !allCompleted {
		return nil
	}

	// Save aggregated role scores for the parent epic (needed for reports)
	for roleID, totalAvg := range roleTotals {
		if err := s.repo.UpsertEpicRoleScore(ctx, parentID, roleID, totalAvg); err != nil {
			return fmt.Errorf("%s: upsert parent role score: %w", op, err)
		}
	}

	if err := s.repo.SetEpicFinalScore(ctx, parentID, parentFinalScore); err != nil {
		return fmt.Errorf("%s: set parent final score: %w", op, err)
	}

	s.log.Info("parent epic scoring completed",
		slog.String("parentID", parentID.String()),
		slog.Float64("finalScore", parentFinalScore))

	return nil
}

// SetManualFinalScore позволяет администратору вручную переопределить итоговую оценку
// (final_score) уже полностью оцененного эпика/стори (статус SCORED). Используется,
// когда нужно скорректировать результат после завершения скоринга без повторного
// прохождения всего цикла оценки участниками. Если у эпика есть родитель (это стори),
// каскадно пересчитывает и агрегированную оценку родительского эпика.
func (s *Service) SetManualFinalScore(ctx context.Context, epicID uuid.UUID, finalScore float64) (*domain.Epic, error) {
	op := "scoring.SetManualFinalScore"

	epic, err := s.repo.GetEpicByID(ctx, epicID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	if epic == nil {
		return nil, fmt.Errorf("%s: epic not found", op)
	}

	if epic.Status != domain.StatusScored {
		return nil, ErrScoringNotComplete
	}

	if err := s.repo.SetEpicFinalScore(ctx, epicID, finalScore); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	if epic.ParentEpicID != nil {
		if err := s.recalcParentEpic(ctx, epic, finalScore); err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
	}

	updatedEpic, err := s.repo.GetEpicByID(ctx, epicID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	s.log.Info("manual final score override applied",
		slog.String("epicID", epicID.String()),
		slog.Float64("finalScore", finalScore))

	return updatedEpic, nil
}
