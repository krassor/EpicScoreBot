package repositories

import (
	"EpicScoreBot/internal/models/domain"
	"context"
	"fmt"

	"github.com/google/uuid"
)

// CreateRisk inserts a new risk for an epic.
func (r *Repository) CreateRisk(ctx context.Context, description string, epicID uuid.UUID) (*domain.Risk, error) {
	op := "Repository.CreateRisk"
	risk := &domain.Risk{
		ID:          uuid.New(),
		Description: description,
		EpicID:      epicID,
		Status:      domain.StatusNew,
	}

	query := `INSERT INTO risks (id, description, epic_id, status)
		VALUES ($1, $2, $3, $4)
		RETURNING created_at, updated_at`
	err := r.DB.QueryRowContext(ctx, query,
		risk.ID, risk.Description, risk.EpicID, string(risk.Status)).
		Scan(&risk.CreatedAt, &risk.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return risk, nil
}

// GetRisksByEpicID returns all risks for an epic.
func (r *Repository) GetRisksByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.Risk, error) {
	op := "Repository.GetRisksByEpicID"
	var risks []domain.Risk
	query := `SELECT id, description, epic_id, status, weighted_score,
		created_at, updated_at
		FROM risks WHERE epic_id = $1
		ORDER BY created_at`
	rows, err := r.DB.QueryContext(ctx, query, epicID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	for rows.Next() {
		var risk domain.Risk
		if err := rows.Scan(&risk.ID, &risk.Description, &risk.EpicID,
			&risk.Status, &risk.WeightedScore,
			&risk.CreatedAt, &risk.UpdatedAt); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		risks = append(risks, risk)
	}
	return risks, nil
}

// GetRiskByID returns a risk by ID.
func (r *Repository) GetRiskByID(ctx context.Context, riskID uuid.UUID) (*domain.Risk, error) {
	op := "Repository.GetRiskByID"
	var risk domain.Risk
	query := `SELECT id, description, epic_id, status, weighted_score,
		created_at, updated_at
		FROM risks WHERE id = $1`
	err := r.DB.QueryRowContext(ctx, query, riskID).
		Scan(&risk.ID, &risk.Description, &risk.EpicID,
			&risk.Status, &risk.WeightedScore,
			&risk.CreatedAt, &risk.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return &risk, nil
}

// UpdateRiskStatus sets the status of a risk.
func (r *Repository) UpdateRiskStatus(ctx context.Context, riskID uuid.UUID, status domain.Status) error {
	op := "Repository.UpdateRiskStatus"
	query := `UPDATE risks SET status = $1, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2`
	_, err := r.DB.ExecContext(ctx, query, string(status), riskID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// SetRiskWeightedScore saves the weighted score and sets status to SCORED.
func (r *Repository) SetRiskWeightedScore(ctx context.Context, riskID uuid.UUID, score float64) error {
	op := "Repository.SetRiskWeightedScore"
	query := `UPDATE risks SET weighted_score = $1, status = $2,
		updated_at = CURRENT_TIMESTAMP
		WHERE id = $3`
	_, err := r.DB.ExecContext(ctx, query, score, string(domain.StatusScored), riskID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// GetUnscoredRisksByUser returns SCORING risks for an epic
// that the user has not yet scored.
func (r *Repository) GetUnscoredRisksByUser(ctx context.Context, userID, epicID uuid.UUID) ([]domain.Risk, error) {
	op := "Repository.GetUnscoredRisksByUser"
	query := `SELECT ri.id, ri.description, ri.epic_id, ri.status,
		ri.weighted_score, ri.created_at, ri.updated_at
		FROM risks ri
		WHERE ri.epic_id = $1 AND ri.status = $2
		AND NOT EXISTS (
			SELECT 1 FROM risk_scores rs
			WHERE rs.risk_id = ri.id AND rs.user_id = $3
		)
		ORDER BY ri.created_at`
	rows, err := r.DB.QueryContext(ctx, query, epicID, string(domain.StatusScoring), userID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	var risks []domain.Risk
	for rows.Next() {
		var risk domain.Risk
		if err := rows.Scan(&risk.ID, &risk.Description, &risk.EpicID,
			&risk.Status, &risk.WeightedScore,
			&risk.CreatedAt, &risk.UpdatedAt); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		risks = append(risks, risk)
	}
	return risks, nil
}
