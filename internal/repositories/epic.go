package repositories

import (
	"EpicScoreBot/internal/models/domain"
	"context"
	"fmt"

	"github.com/google/uuid"
)

// CreateEpic inserts a new epic.
func (r *Repository) CreateEpic(ctx context.Context, number, name, description string, teamID uuid.UUID, year, quarter int, epicType string, evaluatingRoleIDs []uuid.UUID) (*domain.Epic, error) {
	op := "Repository.CreateEpic"
	epic := &domain.Epic{
		ID:                uuid.New(),
		Number:            number,
		Name:              name,
		Description:       description,
		TeamID:            teamID,
		Status:            domain.StatusNew,
		Year:              year,
		Quarter:           quarter,
		Type:              epicType,
		EvaluatingRoleIDs: evaluatingRoleIDs,
	}

	tx, err := r.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: begin tx: %w", op, err)
	}
	defer tx.Rollback()

	// sort_order назначается подзапросом как следующий в очереди команды
	// (скоуп — team_id, среди топ-эпиков без родителя), без отдельного
	// read-then-write.
	query := `INSERT INTO epics (id, number, name, description, team_id, status, year, quarter, type, sort_order)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9,
			COALESCE((SELECT MAX(sort_order) FROM epics WHERE team_id = $5 AND parent_epic_id IS NULL), 0) + 1)
		RETURNING created_at, updated_at, sort_order`
	err = tx.QueryRowContext(ctx, query,
		epic.ID, epic.Number, epic.Name, epic.Description,
		epic.TeamID, string(epic.Status), epic.Year, epic.Quarter, epic.Type).
		Scan(&epic.CreatedAt, &epic.UpdatedAt, &epic.SortOrder)
	if err != nil {
		return nil, fmt.Errorf("%s: insert epic: %w", op, err)
	}

	for _, roleID := range evaluatingRoleIDs {
		_, err = tx.ExecContext(ctx, `INSERT INTO epic_evaluation_roles (epic_id, role_id) VALUES ($1, $2)`, epic.ID, roleID)
		if err != nil {
			return nil, fmt.Errorf("%s: insert evaluating role: %w", op, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("%s: commit: %w", op, err)
	}

	return epic, nil
}

// CreateStory inserts a new story linked to a parent epic.
func (r *Repository) CreateStory(ctx context.Context, parentEpicID uuid.UUID, number, name, description string, teamID uuid.UUID, year, quarter int, epicType string, evaluatingRoleIDs []uuid.UUID) (*domain.Epic, error) {
	op := "Repository.CreateStory"
	epic := &domain.Epic{
		ID:                uuid.New(),
		Number:            number,
		Name:              name,
		Description:       description,
		TeamID:            teamID,
		Status:            domain.StatusNew,
		Year:              year,
		Quarter:           quarter,
		Type:              epicType,
		EvaluatingRoleIDs: evaluatingRoleIDs,
		ParentEpicID:      &parentEpicID,
	}

	tx, err := r.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: begin tx: %w", op, err)
	}
	defer tx.Rollback()

	// sort_order назначается подзапросом как следующий в очереди сторей
	// родительского эпика (скоуп — parent_epic_id), без отдельного read-then-write.
	query := `INSERT INTO epics (id, number, name, description, team_id, status, year, quarter, type, parent_epic_id, sort_order)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			COALESCE((SELECT MAX(sort_order) FROM epics WHERE parent_epic_id = $10), 0) + 1)
		RETURNING created_at, updated_at, sort_order`
	err = tx.QueryRowContext(ctx, query,
		epic.ID, epic.Number, epic.Name, epic.Description,
		epic.TeamID, string(epic.Status), epic.Year, epic.Quarter, epic.Type, epic.ParentEpicID).
		Scan(&epic.CreatedAt, &epic.UpdatedAt, &epic.SortOrder)
	if err != nil {
		return nil, fmt.Errorf("%s: insert story: %w", op, err)
	}

	for _, roleID := range evaluatingRoleIDs {
		_, err = tx.ExecContext(ctx, `INSERT INTO epic_evaluation_roles (epic_id, role_id) VALUES ($1, $2)`, epic.ID, roleID)
		if err != nil {
			return nil, fmt.Errorf("%s: insert evaluating role: %w", op, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("%s: commit: %w", op, err)
	}

	return epic, nil
}

// GetEpicByID returns an epic by ID.
func (r *Repository) GetEpicByID(ctx context.Context, epicID uuid.UUID) (*domain.Epic, error) {
	op := "Repository.GetEpicByID"
	var epic domain.Epic
	query := `SELECT id, number, name, description, team_id, status,
		final_score, year, quarter, type, parent_epic_id, sort_order, created_at, updated_at
		FROM epics WHERE id = $1`
	err := r.DB.QueryRowContext(ctx, query, epicID).
		Scan(&epic.ID, &epic.Number, &epic.Name, &epic.Description,
			&epic.TeamID, &epic.Status,
			&epic.FinalScore, &epic.Year, &epic.Quarter, &epic.Type, &epic.ParentEpicID, &epic.SortOrder, &epic.CreatedAt, &epic.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	evals, err := r.GetEvaluatingRoleIDs(ctx, epic.ID)
	if err == nil {
		epic.EvaluatingRoleIDs = evals
	}
	return &epic, nil
}

// GetEpicByNumber returns an epic by its number.
func (r *Repository) GetEpicByNumber(ctx context.Context, number string) (*domain.Epic, error) {
	op := "Repository.GetEpicByNumber"
	var epic domain.Epic
	query := `SELECT id, number, name, description, team_id, status,
		final_score, year, quarter, type, parent_epic_id, sort_order, created_at, updated_at
		FROM epics WHERE number = $1`
	err := r.DB.QueryRowContext(ctx, query, number).
		Scan(&epic.ID, &epic.Number, &epic.Name, &epic.Description,
			&epic.TeamID, &epic.Status,
			&epic.FinalScore, &epic.Year, &epic.Quarter, &epic.Type, &epic.ParentEpicID, &epic.SortOrder, &epic.CreatedAt, &epic.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	evals, err := r.GetEvaluatingRoleIDs(ctx, epic.ID)
	if err == nil {
		epic.EvaluatingRoleIDs = evals
	}
	return &epic, nil
}

// GetEpicsByTeamIDAndStatus returns epics filtered by team and status.
func (r *Repository) GetEpicsByTeamIDAndStatus(ctx context.Context, teamID uuid.UUID, status domain.Status) ([]domain.Epic, error) {
	op := "Repository.GetEpicsByTeamIDAndStatus"
	var epics []domain.Epic
	query := `SELECT id, number, name, description, team_id, status,
		final_score, year, quarter, type, parent_epic_id, sort_order, created_at, updated_at
		FROM epics WHERE team_id = $1 AND status = $2 AND parent_epic_id IS NULL
		ORDER BY sort_order NULLS LAST, number`
	rows, err := r.DB.QueryContext(ctx, query, teamID, string(status))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	for rows.Next() {
		var e domain.Epic
		if err := rows.Scan(&e.ID, &e.Number, &e.Name, &e.Description,
			&e.TeamID, &e.Status, &e.FinalScore, &e.Year, &e.Quarter, &e.Type,
			&e.ParentEpicID, &e.SortOrder, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		evals, err := r.GetEvaluatingRoleIDs(ctx, e.ID)
		if err == nil {
			e.EvaluatingRoleIDs = evals
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
		e.team_id, e.status, e.final_score, e.year, e.quarter, e.type, e.parent_epic_id,
		e.sort_order, e.created_at, e.updated_at
		FROM epics e
		WHERE e.team_id = $1 AND e.status = $2 AND e.parent_epic_id IS NOT NULL
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
			&e.TeamID, &e.Status, &e.FinalScore, &e.Year, &e.Quarter, &e.Type,
			&e.ParentEpicID, &e.SortOrder, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		evals, err := r.GetEvaluatingRoleIDs(ctx, e.ID)
		if err == nil {
			e.EvaluatingRoleIDs = evals
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
		final_score, year, quarter, type, parent_epic_id, sort_order, created_at, updated_at
		FROM epics WHERE parent_epic_id IS NULL ORDER BY sort_order NULLS LAST, number`
	rows, err := r.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	for rows.Next() {
		var e domain.Epic
		if err := rows.Scan(&e.ID, &e.Number, &e.Name, &e.Description,
			&e.TeamID, &e.Status, &e.FinalScore, &e.Year, &e.Quarter, &e.Type,
			&e.ParentEpicID, &e.SortOrder, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		evals, err := r.GetEvaluatingRoleIDs(ctx, e.ID)
		if err == nil {
			e.EvaluatingRoleIDs = evals
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
		final_score, year, quarter, type, parent_epic_id, sort_order, created_at, updated_at
		FROM epics WHERE status = $1 AND parent_epic_id IS NULL ORDER BY number`
	rows, err := r.DB.QueryContext(ctx, query, string(status))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	for rows.Next() {
		var e domain.Epic
		if err := rows.Scan(&e.ID, &e.Number, &e.Name, &e.Description,
			&e.TeamID, &e.Status, &e.FinalScore, &e.Year, &e.Quarter, &e.Type,
			&e.ParentEpicID, &e.SortOrder, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		evals, err := r.GetEvaluatingRoleIDs(ctx, e.ID)
		if err == nil {
			e.EvaluatingRoleIDs = evals
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

// StartEpicScoring starts scoring for an epic and its risks.
func (r *Repository) StartEpicScoring(ctx context.Context, epicID uuid.UUID) error {
	op := "Repository.StartEpicScoring"
	tx, err := r.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%s: begin tx: %w", op, err)
	}
	defer tx.Rollback() //nolint:errcheck

	// 1. Update epic status
	queryEpic := `UPDATE epics SET status = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2`
	_, err = tx.ExecContext(ctx, queryEpic, string(domain.StatusScoring), epicID)
	if err != nil {
		return fmt.Errorf("%s: update epic status: %w", op, err)
	}

	// 2. Update risks status
	queryRisks := `UPDATE risks SET status = $1, updated_at = CURRENT_TIMESTAMP WHERE epic_id = $2`
	_, err = tx.ExecContext(ctx, queryRisks, string(domain.StatusScoring), epicID)
	if err != nil {
		return fmt.Errorf("%s: update risks status: %w", op, err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("%s: commit: %w", op, err)
	}
	return nil
}

// GetEvaluatingRoleIDs returns all role IDs configured to evaluate the epic.
func (r *Repository) GetEvaluatingRoleIDs(ctx context.Context, epicID uuid.UUID) ([]uuid.UUID, error) {
	op := "Repository.GetEvaluatingRoleIDs"
	var ids []uuid.UUID
	query := `SELECT role_id FROM epic_evaluation_roles WHERE epic_id = $1`
	err := r.DB.SelectContext(ctx, &ids, query, epicID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return ids, nil
}

// GetEpicsByTeamYearQuarter returns all epics of a team for a specific year and quarter.
func (r *Repository) GetEpicsByTeamYearQuarter(ctx context.Context, teamID uuid.UUID, year, quarter int) ([]domain.Epic, error) {
	op := "Repository.GetEpicsByTeamYearQuarter"
	var epics []domain.Epic
	query := `SELECT id, number, name, description, team_id, status,
		final_score, year, quarter, type, parent_epic_id, sort_order, created_at, updated_at
		FROM epics WHERE team_id = $1 AND year = $2 AND quarter = $3 AND parent_epic_id IS NULL
		ORDER BY number`
	rows, err := r.DB.QueryContext(ctx, query, teamID, year, quarter)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	for rows.Next() {
		var e domain.Epic
		if err := rows.Scan(&e.ID, &e.Number, &e.Name, &e.Description,
			&e.TeamID, &e.Status, &e.FinalScore, &e.Year, &e.Quarter, &e.Type,
			&e.ParentEpicID, &e.SortOrder, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		evals, err := r.GetEvaluatingRoleIDs(ctx, e.ID)
		if err == nil {
			e.EvaluatingRoleIDs = evals
		}
		epics = append(epics, e)
	}
	return epics, nil
}

// GetStoriesByEpicID returns all stories for a parent epic, ordered by their
// place in the pipeline queue (sort_order), falling back to number for
// stories that have not been assigned a sort_order yet.
func (r *Repository) GetStoriesByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.Epic, error) {
	op := "Repository.GetStoriesByEpicID"
	epics := []domain.Epic{}
	query := `SELECT id, number, name, description, team_id, status,
		final_score, year, quarter, type, parent_epic_id, sort_order, created_at, updated_at
		FROM epics WHERE parent_epic_id = $1
		ORDER BY sort_order NULLS LAST, number`
	rows, err := r.DB.QueryContext(ctx, query, epicID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	for rows.Next() {
		var e domain.Epic
		if err := rows.Scan(&e.ID, &e.Number, &e.Name, &e.Description,
			&e.TeamID, &e.Status, &e.FinalScore, &e.Year, &e.Quarter, &e.Type,
			&e.ParentEpicID, &e.SortOrder, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		evals, err := r.GetEvaluatingRoleIDs(ctx, e.ID)
		if err == nil {
			e.EvaluatingRoleIDs = evals
		}
		epics = append(epics, e)
	}
	return epics, nil
}

// GetTeamEpicsOrdered returns top-level epics of a team ordered by their
// place in the pipeline queue (sort_order), used by the Gantt scheduler to
// process epics in the correct order. Falls back to number when sort_order
// is not assigned yet.
func (r *Repository) GetTeamEpicsOrdered(ctx context.Context, teamID uuid.UUID) ([]domain.Epic, error) {
	op := "Repository.GetTeamEpicsOrdered"
	var epics []domain.Epic
	query := `SELECT id, number, name, description, team_id, status,
		final_score, year, quarter, type, parent_epic_id, sort_order, created_at, updated_at
		FROM epics WHERE team_id = $1 AND parent_epic_id IS NULL
		ORDER BY sort_order NULLS LAST, number`
	rows, err := r.DB.QueryContext(ctx, query, teamID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	for rows.Next() {
		var e domain.Epic
		if err := rows.Scan(&e.ID, &e.Number, &e.Name, &e.Description,
			&e.TeamID, &e.Status, &e.FinalScore, &e.Year, &e.Quarter, &e.Type,
			&e.ParentEpicID, &e.SortOrder, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		evals, err := r.GetEvaluatingRoleIDs(ctx, e.ID)
		if err == nil {
			e.EvaluatingRoleIDs = evals
		}
		epics = append(epics, e)
	}
	return epics, nil
}

// UpdateEpicSortOrder sets the pipeline queue position of an epic or a
// story (same column, scope is implicit via parent_epic_id/team_id and
// enforced by the caller).
func (r *Repository) UpdateEpicSortOrder(ctx context.Context, epicID uuid.UUID, sortOrder int) error {
	op := "Repository.UpdateEpicSortOrder"
	query := `UPDATE epics SET sort_order = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2`
	_, err := r.DB.ExecContext(ctx, query, sortOrder, epicID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// CountStoriesByEpicID returns the number of stories for a parent epic.
func (r *Repository) CountStoriesByEpicID(ctx context.Context, epicID uuid.UUID) (int, error) {
	op := "Repository.CountStoriesByEpicID"
	var count int
	query := `SELECT COUNT(*) FROM epics WHERE parent_epic_id = $1`
	err := r.DB.QueryRowContext(ctx, query, epicID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}
	return count, nil
}

// UpdateEpic updates an existing epic and performs cascading updates for child stories.
func (r *Repository) UpdateEpic(ctx context.Context, epic *domain.Epic, newEvaluatingRoles []uuid.UUID, oldNumber string) error {
	op := "Repository.UpdateEpic"
	tx, err := r.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%s: begin tx: %w", op, err)
	}
	defer tx.Rollback()

	// Update main epic row
	query := `UPDATE epics SET 
		number = $1, 
		name = $2, 
		description = $3, 
		team_id = $4, 
		year = $5, 
		quarter = $6, 
		type = $7, 
		updated_at = NOW() 
		WHERE id = $8`
	_, err = tx.ExecContext(ctx, query,
		epic.Number, epic.Name, epic.Description,
		epic.TeamID, epic.Year, epic.Quarter, epic.Type, epic.ID)
	if err != nil {
		return fmt.Errorf("%s: update epic: %w", op, err)
	}

	// Update evaluating roles if provided
	if newEvaluatingRoles != nil {
		_, err = tx.ExecContext(ctx, `DELETE FROM epic_evaluation_roles WHERE epic_id = $1`, epic.ID)
		if err != nil {
			return fmt.Errorf("%s: delete old roles: %w", op, err)
		}
		for _, roleID := range newEvaluatingRoles {
			_, err = tx.ExecContext(ctx, `INSERT INTO epic_evaluation_roles (epic_id, role_id) VALUES ($1, $2)`, epic.ID, roleID)
			if err != nil {
				return fmt.Errorf("%s: insert role: %w", op, err)
			}
		}
	}

	// Cascade update child stories (team_id, year, quarter, type)
	_, err = tx.ExecContext(ctx, `UPDATE epics SET 
		team_id = $1, 
		year = $2, 
		quarter = $3, 
		type = $4, 
		updated_at = NOW() 
		WHERE parent_epic_id = $5`,
		epic.TeamID, epic.Year, epic.Quarter, epic.Type, epic.ID)
	if err != nil {
		return fmt.Errorf("%s: cascade update stories: %w", op, err)
	}

	// Cascade update evaluating roles for child stories if roles changed
	if newEvaluatingRoles != nil {
		storiesQuery := `SELECT id FROM epics WHERE parent_epic_id = $1`
		var storyIDs []uuid.UUID
		err = tx.SelectContext(ctx, &storyIDs, storiesQuery, epic.ID)
		if err == nil {
			for _, storyID := range storyIDs {
				tx.ExecContext(ctx, `DELETE FROM epic_evaluation_roles WHERE epic_id = $1`, storyID)
				for _, roleID := range newEvaluatingRoles {
					tx.ExecContext(ctx, `INSERT INTO epic_evaluation_roles (epic_id, role_id) VALUES ($1, $2)`, storyID, roleID)
				}
			}
		}
	}

	// Cascade rename story numbers if number changed (e.g., "EPIC-42-S1" -> "EPIC-99-S1")
	if oldNumber != "" && oldNumber != epic.Number {
		oldPrefix := oldNumber + "-S"
		newPrefix := epic.Number + "-S"
		_, err = tx.ExecContext(ctx, `UPDATE epics SET 
			number = REPLACE(number, $1, $2), 
			updated_at = NOW() 
			WHERE parent_epic_id = $3 AND number LIKE $4`,
			oldPrefix, newPrefix, epic.ID, oldPrefix+"%")
		if err != nil {
			return fmt.Errorf("%s: cascade update story numbers: %w", op, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("%s: commit: %w", op, err)
	}
	return nil
}

// UpdateStory updates an existing story.
func (r *Repository) UpdateStory(ctx context.Context, story *domain.Epic) error {
	op := "Repository.UpdateStory"
	tx, err := r.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%s: begin tx: %w", op, err)
	}
	defer tx.Rollback()

	query := `UPDATE epics SET 
		number = $1, 
		name = $2, 
		description = $3, 
		parent_epic_id = $4, 
		team_id = $5, 
		year = $6, 
		quarter = $7, 
		type = $8, 
		updated_at = NOW() 
		WHERE id = $9 AND parent_epic_id IS NOT NULL`
	_, err = tx.ExecContext(ctx, query,
		story.Number, story.Name, story.Description,
		story.ParentEpicID, story.TeamID, story.Year, story.Quarter, story.Type, story.ID)
	if err != nil {
		return fmt.Errorf("%s: update story: %w", op, err)
	}

	// Update evaluating roles from parent epic
	if story.EvaluatingRoleIDs != nil {
		_, err = tx.ExecContext(ctx, `DELETE FROM epic_evaluation_roles WHERE epic_id = $1`, story.ID)
		if err == nil {
			for _, roleID := range story.EvaluatingRoleIDs {
				tx.ExecContext(ctx, `INSERT INTO epic_evaluation_roles (epic_id, role_id) VALUES ($1, $2)`, story.ID, roleID)
			}
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("%s: commit: %w", op, err)
	}
	return nil
}

