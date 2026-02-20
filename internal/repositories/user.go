package repositories

import (
	"EpicScoreBot/internal/models/domain"
	"context"
	"fmt"

	"github.com/google/uuid"
)

// CreateUser inserts a new user.
func (r *Repository) CreateUser(ctx context.Context, firstName, lastName string, telegramID string, weight int) (*domain.User, error) {
	op := "Repository.CreateUser"
	user := &domain.User{
		ID:         uuid.New(),
		FirstName:  firstName,
		LastName:   lastName,
		TelegramID: telegramID,
		Weight:     weight,
	}

	query := `INSERT INTO users (id, first_name, last_name, telegram_id, weight)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at, updated_at`
	err := r.DB.QueryRowContext(ctx, query,
		user.ID, user.FirstName, user.LastName, user.TelegramID, user.Weight).
		Scan(&user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return user, nil
}

// FindUserByTelegramID returns a user by Telegram ID.
func (r *Repository) FindUserByTelegramID(ctx context.Context, telegramID string) (*domain.User, error) {
	op := "Repository.FindUserByTelegramID"
	var user domain.User
	query := `SELECT id, first_name, last_name, telegram_id, weight,
		created_at, updated_at
		FROM users WHERE telegram_id = $1`
	err := r.DB.QueryRowContext(ctx, query, telegramID).
		Scan(&user.ID, &user.FirstName, &user.LastName,
			&user.TelegramID, &user.Weight,
			&user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return &user, nil
}

// GetUsersByTeamID returns all users in a team.
func (r *Repository) GetUsersByTeamID(ctx context.Context, teamID uuid.UUID) ([]domain.User, error) {
	op := "Repository.GetUsersByTeamID"
	var users []domain.User
	query := `SELECT u.id, u.first_name, u.last_name, u.telegram_id,
		u.weight, u.created_at, u.updated_at
		FROM users u
		INNER JOIN user_teams ut ON u.id = ut.user_id
		WHERE ut.team_id = $1
		ORDER BY u.last_name, u.first_name`
	rows, err := r.DB.QueryContext(ctx, query, teamID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	for rows.Next() {
		var u domain.User
		if err := rows.Scan(&u.ID, &u.FirstName, &u.LastName,
			&u.TelegramID, &u.Weight,
			&u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		users = append(users, u)
	}
	return users, nil
}

// GetUsersByTeamIDAndRoleID returns users in a team with a specific role.
func (r *Repository) GetUsersByTeamIDAndRoleID(ctx context.Context, teamID, roleID uuid.UUID) ([]domain.User, error) {
	op := "Repository.GetUsersByTeamIDAndRoleID"
	var users []domain.User
	query := `SELECT u.id, u.first_name, u.last_name, u.telegram_id,
		u.weight, u.created_at, u.updated_at
		FROM users u
		INNER JOIN user_teams ut ON u.id = ut.user_id
		INNER JOIN user_roles ur ON u.id = ur.user_id
		WHERE ut.team_id = $1 AND ur.role_id = $2
		ORDER BY u.last_name, u.first_name`
	rows, err := r.DB.QueryContext(ctx, query, teamID, roleID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	for rows.Next() {
		var u domain.User
		if err := rows.Scan(&u.ID, &u.FirstName, &u.LastName,
			&u.TelegramID, &u.Weight,
			&u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		users = append(users, u)
	}
	return users, nil
}

// AssignUserRole assigns a role to a user. Ignores conflicts.
func (r *Repository) AssignUserRole(ctx context.Context, userID, roleID uuid.UUID) error {
	op := "Repository.AssignUserRole"
	query := `INSERT INTO user_roles (user_id, role_id)
		VALUES ($1, $2) ON CONFLICT DO NOTHING`
	_, err := r.DB.ExecContext(ctx, query, userID, roleID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// AssignUserTeam assigns a user to a team. Ignores conflicts.
func (r *Repository) AssignUserTeam(ctx context.Context, userID, teamID uuid.UUID) error {
	op := "Repository.AssignUserTeam"
	query := `INSERT INTO user_teams (user_id, team_id)
		VALUES ($1, $2) ON CONFLICT DO NOTHING`
	_, err := r.DB.ExecContext(ctx, query, userID, teamID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// GetUserByID returns a user by ID.
func (r *Repository) GetUserByID(ctx context.Context, userID uuid.UUID) (*domain.User, error) {
	op := "Repository.GetUserByID"
	var user domain.User
	query := `SELECT id, first_name, last_name, telegram_id, weight,
		created_at, updated_at
		FROM users WHERE id = $1`
	err := r.DB.QueryRowContext(ctx, query, userID).
		Scan(&user.ID, &user.FirstName, &user.LastName,
			&user.TelegramID, &user.Weight,
			&user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return &user, nil
}
