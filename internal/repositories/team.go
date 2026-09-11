package repositories

import (
	"EpicScoreBot/internal/models/domain"
	"context"
	"fmt"

	"github.com/google/uuid"
)

// CreateTeam inserts a new team.
func (r *Repository) CreateTeam(ctx context.Context, name, description string) (*domain.Team, error) {
	op := "Repository.CreateTeam"
	team := &domain.Team{
		ID:          uuid.New(),
		Name:        name,
		Description: description,
	}

	query := `INSERT INTO teams (id, name, description)
		VALUES ($1, $2, $3)
		RETURNING created_at, updated_at`
	err := r.DB.QueryRowContext(ctx, query,
		team.ID, team.Name, team.Description).
		Scan(&team.CreatedAt, &team.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return team, nil
}

// GetTeamByName returns a team by name.
func (r *Repository) GetTeamByName(ctx context.Context, name string) (*domain.Team, error) {
	op := "Repository.GetTeamByName"
	var team domain.Team
	query := `SELECT id, name, description, backfill_block_past, created_at, updated_at
		FROM teams WHERE name = $1`
	err := r.DB.QueryRowContext(ctx, query, name).
		Scan(&team.ID, &team.Name, &team.Description, &team.BackfillBlockPast,
			&team.CreatedAt, &team.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return &team, nil
}

// GetTeamByID returns a team by ID.
func (r *Repository) GetTeamByID(ctx context.Context, teamID uuid.UUID) (*domain.Team, error) {
	op := "Repository.GetTeamByID"
	var team domain.Team
	query := `SELECT id, name, description, backfill_block_past, created_at, updated_at
		FROM teams WHERE id = $1`
	err := r.DB.QueryRowContext(ctx, query, teamID).
		Scan(&team.ID, &team.Name, &team.Description, &team.BackfillBlockPast,
			&team.CreatedAt, &team.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return &team, nil
}

// GetAllTeams returns all teams.
func (r *Repository) GetAllTeams(ctx context.Context) ([]domain.Team, error) {
	op := "Repository.GetAllTeams"
	var teams []domain.Team
	query := `SELECT id, name, description, backfill_block_past, created_at, updated_at
		FROM teams ORDER BY name`
	rows, err := r.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	for rows.Next() {
		var t domain.Team
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.BackfillBlockPast,
			&t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		teams = append(teams, t)
	}
	return teams, nil
}

// GetTeamsByUserTelegramID returns all teams a user belongs to.
func (r *Repository) GetTeamsByUserTelegramID(ctx context.Context, telegramID string) ([]domain.Team, error) {
	op := "Repository.GetTeamsByUserTelegramID"
	var teams []domain.Team
	query := `SELECT t.id, t.name, t.description, t.backfill_block_past, t.created_at, t.updated_at
		FROM teams t
		INNER JOIN user_teams ut ON t.id = ut.team_id
		INNER JOIN users u ON u.id = ut.user_id
		WHERE u.telegram_id = $1
		ORDER BY t.name`
	rows, err := r.DB.QueryContext(ctx, query, telegramID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	for rows.Next() {
		var t domain.Team
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.BackfillBlockPast,
			&t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		teams = append(teams, t)
	}
	return teams, nil
}

// UpdateTeamBackfillBlockPast задаёт значение настройки команды «не
// занимать промежутки расписания, оставшиеся в прошлом» (design.md
// Решение 3, openspec/changes/backfill-idle-gaps-in-schedule). Переключается
// администратором команды (секция 3 заявки, отдельным HTTP-обработчиком) и
// читается планировщиком в начале каждого RecalculateTeamSchedule.
func (r *Repository) UpdateTeamBackfillBlockPast(ctx context.Context, teamID uuid.UUID, blocked bool) error {
	op := "Repository.UpdateTeamBackfillBlockPast"
	query := `UPDATE teams SET backfill_block_past = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2`
	_, err := r.DB.ExecContext(ctx, query, blocked, teamID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}
