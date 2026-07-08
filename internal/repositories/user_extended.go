package repositories

import (
	"EpicScoreBot/internal/models/domain"
	"context"
	"fmt"

	"github.com/google/uuid"
)

// CreateUserWithRelations создает пользователя и привязывает его к командам и ролям в рамках одной транзакции.
func (r *Repository) CreateUserWithRelations(
	ctx context.Context,
	user *domain.User,
	teamUUIDs []uuid.UUID,
	roleUUIDs []uuid.UUID,
) error {
	op := "Repository.CreateUserWithRelations"
	tx, err := r.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%s: begin tx: %w", op, err)
	}
	defer tx.Rollback() //nolint:errcheck

	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}

	queryUser := `INSERT INTO users (id, first_name, last_name, telegram_id, chat_id, weight)
		VALUES ($1, $2, $3, $4, 0, $5)
		RETURNING created_at, updated_at`
	err = tx.QueryRowContext(ctx, queryUser,
		user.ID, user.FirstName, user.LastName, user.TelegramID, user.Weight).
		Scan(&user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return fmt.Errorf("%s: insert user: %w", op, err)
	}

	for _, teamID := range teamUUIDs {
		queryTeam := `INSERT INTO user_teams (user_id, team_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`
		_, err = tx.ExecContext(ctx, queryTeam, user.ID, teamID)
		if err != nil {
			return fmt.Errorf("%s: assign team %s: %w", op, teamID, err)
		}
	}

	for _, roleID := range roleUUIDs {
		queryRole := `INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`
		_, err = tx.ExecContext(ctx, queryRole, user.ID, roleID)
		if err != nil {
			return fmt.Errorf("%s: assign role %s: %w", op, roleID, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("%s: commit: %w", op, err)
	}
	return nil
}

// UpdateUserWithRelations обновляет профиль пользователя и синхронизирует его связи с командами и ролями в одной транзакции.
func (r *Repository) UpdateUserWithRelations(
	ctx context.Context,
	userID uuid.UUID,
	firstName string,
	lastName string,
	weight int,
	teamUUIDs []uuid.UUID,
	roleUUIDs []uuid.UUID,
) error {
	op := "Repository.UpdateUserWithRelations"
	tx, err := r.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%s: begin tx: %w", op, err)
	}
	defer tx.Rollback() //nolint:errcheck

	queryUser := `UPDATE users SET first_name = $2, last_name = $3, weight = $4, updated_at = NOW() WHERE id = $1`
	_, err = tx.ExecContext(ctx, queryUser, userID, firstName, lastName, weight)
	if err != nil {
		return fmt.Errorf("%s: update user: %w", op, err)
	}

	// Очищаем старые связи пользователя с командами
	queryDelTeams := `DELETE FROM user_teams WHERE user_id = $1`
	_, err = tx.ExecContext(ctx, queryDelTeams, userID)
	if err != nil {
		return fmt.Errorf("%s: delete user teams: %w", op, err)
	}

	// Очищаем старые связи пользователя с ролями
	queryDelRoles := `DELETE FROM user_roles WHERE user_id = $1`
	_, err = tx.ExecContext(ctx, queryDelRoles, userID)
	if err != nil {
		return fmt.Errorf("%s: delete user roles: %w", op, err)
	}

	// Записываем новые связи
	for _, teamID := range teamUUIDs {
		queryTeam := `INSERT INTO user_teams (user_id, team_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`
		_, err = tx.ExecContext(ctx, queryTeam, userID, teamID)
		if err != nil {
			return fmt.Errorf("%s: assign team %s: %w", op, teamID, err)
		}
	}

	for _, roleID := range roleUUIDs {
		queryRole := `INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`
		_, err = tx.ExecContext(ctx, queryRole, userID, roleID)
		if err != nil {
			return fmt.Errorf("%s: assign role %s: %w", op, roleID, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("%s: commit: %w", op, err)
	}
	return nil
}

// GetUserRelations возвращает списки UUID команд и ролей для пользователя.
func (r *Repository) GetUserRelations(
	ctx context.Context,
	userID uuid.UUID,
) ([]uuid.UUID, []uuid.UUID, error) {
	op := "Repository.GetUserRelations"

	var teamIDs []uuid.UUID
	queryTeams := `SELECT team_id FROM user_teams WHERE user_id = $1`
	rowsTeams, err := r.DB.QueryContext(ctx, queryTeams, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: get team IDs: %w", op, err)
	}
	defer rowsTeams.Close()
	for rowsTeams.Next() {
		var tid uuid.UUID
		if err := rowsTeams.Scan(&tid); err != nil {
			return nil, nil, fmt.Errorf("%s: scan team ID: %w", op, err)
		}
		teamIDs = append(teamIDs, tid)
	}
	if err = rowsTeams.Err(); err != nil {
		return nil, nil, fmt.Errorf("%s: rowsTeams error: %w", op, err)
	}

	var roleIDs []uuid.UUID
	queryRoles := `SELECT role_id FROM user_roles WHERE user_id = $1`
	rowsRoles, err := r.DB.QueryContext(ctx, queryRoles, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: get role IDs: %w", op, err)
	}
	defer rowsRoles.Close()
	for rowsRoles.Next() {
		var rid uuid.UUID
		if err := rowsRoles.Scan(&rid); err != nil {
			return nil, nil, fmt.Errorf("%s: scan role ID: %w", op, err)
		}
		roleIDs = append(roleIDs, rid)
	}
	if err = rowsRoles.Err(); err != nil {
		return nil, nil, fmt.Errorf("%s: rowsRoles error: %w", op, err)
	}

	return teamIDs, roleIDs, nil
}

// GetUserTeams возвращает все команды, к которым принадлежит пользователь.
func (r *Repository) GetUserTeams(ctx context.Context, userID uuid.UUID) ([]domain.Team, error) {
	op := "Repository.GetUserTeams"
	var teams []domain.Team
	query := `SELECT t.id, t.name, t.description FROM teams t
		INNER JOIN user_teams ut ON t.id = ut.team_id
		WHERE ut.user_id = $1 ORDER BY t.name`
	err := r.DB.SelectContext(ctx, &teams, query, userID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return teams, nil
}

// GetUserRoles возвращает все роли, назначенные пользователю.
func (r *Repository) GetUserRoles(ctx context.Context, userID uuid.UUID) ([]domain.Role, error) {
	op := "Repository.GetUserRoles"
	var roles []domain.Role
	query := `SELECT r.id, r.name, r.description FROM roles r
		INNER JOIN user_roles ur ON r.id = ur.role_id
		WHERE ur.user_id = $1 ORDER BY r.name`
	err := r.DB.SelectContext(ctx, &roles, query, userID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return roles, nil
}
