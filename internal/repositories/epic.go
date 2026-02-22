package repositories

import (
	"EpicScoreBot/internal/models/domain"
	"context"
	"fmt"

	"github.com/google/uuid"
)

// CreateEpic inserts a new epic.
func (r *Repository) CreateEpic(ctx context.Context, number, name, description string, teamID uuid.UUID) (*domain.Epic, error) {
	op := "Repository.CreateEpic"
	epic := &domain.Epic{
		ID:          uuid.New(),
		Number:      number,
		Name:        name,
		Description: description,
		TeamID:      teamID,
		Status:      domain.StatusNew,
	}

	query := `INSERT INTO epics (id, number, name, description, team_id, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at, updated_at`
	err := r.DB.QueryRowContext(ctx, query,
		epic.ID, epic.Number, epic.Name, epic.Description,
		epic.TeamID, string(epic.Status)).
		Scan(&epic.CreatedAt, &epic.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return epic, nil
}

// GetEpicByID returns an epic by ID.
func (r *Repository) GetEpicByID(ctx context.Context, epicID uuid.UUID) (*domain.Epic, error) {
	op := "Repository.GetEpicByID"
	var epic domain.Epic
	query := `SELECT id, number, name, description, team_id, status,
		final_score, created_at, updated_at
		FROM epics WHERE id = $1`
	err := r.DB.QueryRowContext(ctx, query, epicID).
		Scan(&epic.ID, &epic.Number, &epic.Name, &epic.Description,
			&epic.TeamID, &epic.Status,
			&epic.FinalScore, &epic.CreatedAt, &epic.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return &epic, nil
}

// GetEpicByNumber returns an epic by its number.
func (r *Repository) GetEpicByNumber(ctx context.Context, number string) (*domain.Epic, error) {
	op := "Repository.GetEpicByNumber"
	var epic domain.Epic
	query := `SELECT id, number, name, description, team_id, status,
		final_score, created_at, updated_at
		FROM epics WHERE number = $1`
	err := r.DB.QueryRowContext(ctx, query, number).
		Scan(&epic.ID, &epic.Number, &epic.Name, &epic.Description,
			&epic.TeamID, &epic.Status,
			&epic.FinalScore, &epic.CreatedAt, &epic.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return &epic, nil
}

// GetEpicsByTeamIDAndStatus returns epics filtered by team and status.
func (r *Repository) GetEpicsByTeamIDAndStatus(ctx context.Context, teamID uuid.UUID, status domain.Status) ([]domain.Epic, error) {
	op := "Repository.GetEpicsByTeamIDAndStatus"
	var epics []domain.Epic
	query := `SELECT id, number, name, description, team_id, status,
		final_score, created_at, updated_at
		FROM epics WHERE team_id = $1 AND status = $2
		ORDER BY number`
	rows, err := r.DB.QueryContext(ctx, query, teamID, string(status))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	for rows.Next() {
		var e domain.Epic
		if err := rows.Scan(&e.ID, &e.Number, &e.Name, &e.Description,
			&e.TeamID, &e.Status,
			&e.FinalScore, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		epics = append(epics, e)
	}
	return epics, nil
}

// UpdateEpicStatus sets the status of an epic.
func (r *Repository) UpdateEpicStatus(ctx context.Context, epicID uuid.UUID, status domain.Status) error {
	op := "Repository.UpdateEpicStatus"
	query := `UPDATE epics SET status = $1, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2`
	_, err := r.DB.ExecContext(ctx, query, string(status), epicID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// SetEpicFinalScore sets the final score and status of an epic.
func (r *Repository) SetEpicFinalScore(ctx context.Context, epicID uuid.UUID, score float64) error {
	op := "Repository.SetEpicFinalScore"
	query := `UPDATE epics SET final_score = $1, status = $2,
		updated_at = CURRENT_TIMESTAMP
		WHERE id = $3`
	_, err := r.DB.ExecContext(ctx, query, score, string(domain.StatusScored), epicID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// GetUnscoredEpicsByUser returns SCORING epics in a team where the user
// still has outstanding work: either the epic effort is not yet scored,
// or one or more of its SCORING risks are not scored by this user.
func (r *Repository) GetUnscoredEpicsByUser(ctx context.Context, userID uuid.UUID, teamID uuid.UUID) ([]domain.Epic, error) {
	op := "Repository.GetUnscoredEpicsByUser"
	query := `SELECT e.id, e.number, e.name, e.description,
		e.team_id, e.status, e.final_score,
		e.created_at, e.updated_at
		FROM epics e
		WHERE e.team_id = $1 AND e.status = $2
		AND (
			-- effort not yet scored by this user
			NOT EXISTS (
				SELECT 1 FROM epic_scores es
				WHERE es.epic_id = e.id AND es.user_id = $3
			)
			OR
			-- at least one SCORING risk not scored by this user
			EXISTS (
				SELECT 1 FROM risks ri
				WHERE ri.epic_id = e.id AND ri.status = $2
				AND NOT EXISTS (
					SELECT 1 FROM risk_scores rs
					WHERE rs.risk_id = ri.id AND rs.user_id = $3
				)
			)
		)
		ORDER BY e.number`
	rows, err := r.DB.QueryContext(ctx, query, teamID, string(domain.StatusScoring), userID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	var epics []domain.Epic
	for rows.Next() {
		var e domain.Epic
		if err := rows.Scan(&e.ID, &e.Number, &e.Name, &e.Description,
			&e.TeamID, &e.Status, &e.FinalScore,
			&e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		epics = append(epics, e)
	}
	return epics, nil
}

// GetAllEpics returns every epic ordered by number.
func (r *Repository) GetAllEpics(ctx context.Context) ([]domain.Epic, error) {
	op := "Repository.GetAllEpics"
	var epics []domain.Epic
	query := `SELECT id, number, name, description, team_id, status,
		final_score, created_at, updated_at
		FROM epics ORDER BY number`
	rows, err := r.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	for rows.Next() {
		var e domain.Epic
		if err := rows.Scan(&e.ID, &e.Number, &e.Name, &e.Description,
			&e.TeamID, &e.Status, &e.FinalScore,
			&e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		epics = append(epics, e)
	}
	return epics, nil
}

// GetEpicsByStatus returns all epics with a given status.
func (r *Repository) GetEpicsByStatus(ctx context.Context, status domain.Status) ([]domain.Epic, error) {
	op := "Repository.GetEpicsByStatus"
	var epics []domain.Epic
	query := `SELECT id, number, name, description, team_id, status,
		final_score, created_at, updated_at
		FROM epics WHERE status = $1 ORDER BY number`
	rows, err := r.DB.QueryContext(ctx, query, string(status))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	for rows.Next() {
		var e domain.Epic
		if err := rows.Scan(&e.ID, &e.Number, &e.Name, &e.Description,
			&e.TeamID, &e.Status, &e.FinalScore,
			&e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		epics = append(epics, e)
	}
	return epics, nil
}

// DeleteEpic permanently removes an epic and all related data (cascade).
func (r *Repository) DeleteEpic(ctx context.Context, epicID uuid.UUID) error {
	op := "Repository.DeleteEpic"
	query := `DELETE FROM epics WHERE id = $1`
	_, err := r.DB.ExecContext(ctx, query, epicID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}
