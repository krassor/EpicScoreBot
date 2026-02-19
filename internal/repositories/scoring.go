package repositories

import (
	"EpicScoreBot/internal/models/domain"
	"context"
	"fmt"

	"github.com/google/uuid"
)

// CreateEpicScore inserts a user's score for an epic.
func (r *Repository) CreateEpicScore(ctx context.Context, epicID, userID, roleID uuid.UUID, score int) error {
	op := "Repository.CreateEpicScore"
	query := `INSERT INTO epic_scores (id, epic_id, user_id, role_id, score)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (epic_id, user_id) DO UPDATE SET score = $5, role_id = $4`
	_, err := r.DB.ExecContext(ctx, query, uuid.New(), epicID, userID, roleID, score)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// GetEpicScoresByEpicID returns all scores for an epic.
func (r *Repository) GetEpicScoresByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.EpicScore, error) {
	op := "Repository.GetEpicScoresByEpicID"
	query := `SELECT id, epic_id, user_id, role_id, score, created_at
		FROM epic_scores WHERE epic_id = $1`
	rows, err := r.DB.QueryContext(ctx, query, epicID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	var scores []domain.EpicScore
	for rows.Next() {
		var s domain.EpicScore
		if err := rows.Scan(&s.ID, &s.EpicID, &s.UserID,
			&s.RoleID, &s.Score, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		scores = append(scores, s)
	}
	return scores, nil
}

// GetEpicScoresByEpicIDAndRoleID returns scores for an epic filtered by role.
func (r *Repository) GetEpicScoresByEpicIDAndRoleID(ctx context.Context, epicID, roleID uuid.UUID) ([]domain.EpicScore, error) {
	op := "Repository.GetEpicScoresByEpicIDAndRoleID"
	query := `SELECT es.id, es.epic_id, es.user_id, es.role_id, es.score, es.created_at
		FROM epic_scores es WHERE es.epic_id = $1 AND es.role_id = $2`
	rows, err := r.DB.QueryContext(ctx, query, epicID, roleID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	var scores []domain.EpicScore
	for rows.Next() {
		var s domain.EpicScore
		if err := rows.Scan(&s.ID, &s.EpicID, &s.UserID,
			&s.RoleID, &s.Score, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		scores = append(scores, s)
	}
	return scores, nil
}

// HasUserScoredEpic checks if a user has already scored an epic.
func (r *Repository) HasUserScoredEpic(ctx context.Context, epicID, userID uuid.UUID) (bool, error) {
	op := "Repository.HasUserScoredEpic"
	var count int
	query := `SELECT COUNT(*) FROM epic_scores
		WHERE epic_id = $1 AND user_id = $2`
	err := r.DB.QueryRowContext(ctx, query, epicID, userID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("%s: %w", op, err)
	}
	return count > 0, nil
}

// CreateRiskScore inserts a user's risk assessment.
func (r *Repository) CreateRiskScore(ctx context.Context, riskID, userID uuid.UUID, probability, impact int) error {
	op := "Repository.CreateRiskScore"
	query := `INSERT INTO risk_scores (id, risk_id, user_id, probability, impact)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (risk_id, user_id) DO UPDATE SET probability = $4, impact = $5`
	_, err := r.DB.ExecContext(ctx, query, uuid.New(), riskID, userID, probability, impact)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// GetRiskScoresByRiskID returns all scores for a risk.
func (r *Repository) GetRiskScoresByRiskID(ctx context.Context, riskID uuid.UUID) ([]domain.RiskScore, error) {
	op := "Repository.GetRiskScoresByRiskID"
	query := `SELECT id, risk_id, user_id, probability, impact, created_at
		FROM risk_scores WHERE risk_id = $1`
	rows, err := r.DB.QueryContext(ctx, query, riskID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	var scores []domain.RiskScore
	for rows.Next() {
		var s domain.RiskScore
		if err := rows.Scan(&s.ID, &s.RiskID, &s.UserID,
			&s.Probability, &s.Impact, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		scores = append(scores, s)
	}
	return scores, nil
}

// HasUserScoredRisk checks if a user has already scored a risk.
func (r *Repository) HasUserScoredRisk(ctx context.Context, riskID, userID uuid.UUID) (bool, error) {
	op := "Repository.HasUserScoredRisk"
	var count int
	query := `SELECT COUNT(*) FROM risk_scores
		WHERE risk_id = $1 AND user_id = $2`
	err := r.DB.QueryRowContext(ctx, query, riskID, userID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("%s: %w", op, err)
	}
	return count > 0, nil
}

// UpsertEpicRoleScore inserts or updates the weighted average for a role.
func (r *Repository) UpsertEpicRoleScore(ctx context.Context, epicID, roleID uuid.UUID, weightedAvg float64) error {
	op := "Repository.UpsertEpicRoleScore"
	query := `INSERT INTO epic_role_scores (id, epic_id, role_id, weighted_avg)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (epic_id, role_id) DO UPDATE SET weighted_avg = $4`
	_, err := r.DB.ExecContext(ctx, query, uuid.New(), epicID, roleID, weightedAvg)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// GetEpicRoleScoresByEpicID returns all role-level weighted averages for an epic.
func (r *Repository) GetEpicRoleScoresByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.EpicRoleScore, error) {
	op := "Repository.GetEpicRoleScoresByEpicID"
	query := `SELECT id, epic_id, role_id, weighted_avg
		FROM epic_role_scores WHERE epic_id = $1`
	rows, err := r.DB.QueryContext(ctx, query, epicID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	var scores []domain.EpicRoleScore
	for rows.Next() {
		var s domain.EpicRoleScore
		if err := rows.Scan(&s.ID, &s.EpicID, &s.RoleID, &s.WeightedAvg); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		scores = append(scores, s)
	}
	return scores, nil
}

// CountTeamMembers returns the number of users in a team.
func (r *Repository) CountTeamMembers(ctx context.Context, teamID uuid.UUID) (int, error) {
	op := "Repository.CountTeamMembers"
	var count int
	query := `SELECT COUNT(*) FROM user_teams WHERE team_id = $1`
	err := r.DB.QueryRowContext(ctx, query, teamID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}
	return count, nil
}

// CountEpicScores returns the number of scores for an epic.
func (r *Repository) CountEpicScores(ctx context.Context, epicID uuid.UUID) (int, error) {
	op := "Repository.CountEpicScores"
	var count int
	query := `SELECT COUNT(*) FROM epic_scores WHERE epic_id = $1`
	err := r.DB.QueryRowContext(ctx, query, epicID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}
	return count, nil
}

// CountRiskScores returns the number of scores for a risk.
func (r *Repository) CountRiskScores(ctx context.Context, riskID uuid.UUID) (int, error) {
	op := "Repository.CountRiskScores"
	var count int
	query := `SELECT COUNT(*) FROM risk_scores WHERE risk_id = $1`
	err := r.DB.QueryRowContext(ctx, query, riskID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}
	return count, nil
}

// GetDistinctRoleIDsForEpicScores returns the distinct role IDs
// that have scores for a given epic.
func (r *Repository) GetDistinctRoleIDsForEpicScores(ctx context.Context, epicID uuid.UUID) ([]uuid.UUID, error) {
	op := "Repository.GetDistinctRoleIDsForEpicScores"
	query := `SELECT DISTINCT role_id FROM epic_scores WHERE epic_id = $1`
	rows, err := r.DB.QueryContext(ctx, query, epicID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	var roleIDs []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		roleIDs = append(roleIDs, id)
	}
	return roleIDs, nil
}
