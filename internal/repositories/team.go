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
	query := `SELECT id, name, description, created_at, updated_at
		FROM teams WHERE name = $1`
	err := r.DB.QueryRowContext(ctx, query, name).
		Scan(&team.ID, &team.Name, &team.Description,
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
	query := `SELECT id, name, description, created_at, updated_at
		FROM teams WHERE id = $1`
	err := r.DB.QueryRowContext(ctx, query, teamID).
		Scan(&team.ID, &team.Name, &team.Description,
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
	query := `SELECT id, name, description, created_at, updated_at
		FROM teams ORDER BY name`
	rows, err := r.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	for rows.Next() {
		var t domain.Team
		if err := rows.Scan(&t.ID, &t.Name, &t.Description,
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
	query := `SELECT t.id, t.name, t.description, t.created_at, t.updated_at
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
		if err := rows.Scan(&t.ID, &t.Name, &t.Description,
			&t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		teams = append(teams, t)
	}
	return teams, nil
}
