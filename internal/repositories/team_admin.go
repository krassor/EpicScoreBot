package repositories

import (
	"EpicScoreBot/internal/models/domain"
	"context"
	"fmt"

	"github.com/google/uuid"
)

// AssignTeamAdmin назначает пользователя администратором команды (team-admin).
// Идемпотентно: повторное назначение той же пары (userID, teamID) не создаёт
// дубликат записи и не считается ошибкой (ON CONFLICT DO NOTHING).
//
// assignedBy — UUID пользователя-superadmin, выполнившего назначение, для
// аудита. Superadmin не обязан быть строкой в таблице users (роль
// config-based), поэтому uuid.Nil трактуется как "неизвестно" и сохраняется
// как NULL (колонка assigned_by допускает NULL, ON DELETE SET NULL).
func (r *Repository) AssignTeamAdmin(ctx context.Context, userID, teamID, assignedBy uuid.UUID) error {
	op := "Repository.AssignTeamAdmin"
	query := `INSERT INTO team_admins (user_id, team_id, assigned_by)
		VALUES ($1, $2, NULLIF($3, '00000000-0000-0000-0000-000000000000'::uuid))
		ON CONFLICT (user_id, team_id) DO NOTHING`
	_, err := r.DB.ExecContext(ctx, query, userID, teamID, assignedBy)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// RemoveTeamAdmin снимает привязку team-admin пользователя к конкретной
// команде, не затрагивая его привязки к другим командам.
func (r *Repository) RemoveTeamAdmin(ctx context.Context, userID, teamID uuid.UUID) error {
	op := "Repository.RemoveTeamAdmin"
	query := `DELETE FROM team_admins WHERE user_id = $1 AND team_id = $2`
	_, err := r.DB.ExecContext(ctx, query, userID, teamID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// GetTeamAdminsByTeamID возвращает всех team-admin указанной команды.
func (r *Repository) GetTeamAdminsByTeamID(ctx context.Context, teamID uuid.UUID) ([]domain.User, error) {
	op := "Repository.GetTeamAdminsByTeamID"
	var users []domain.User
	query := `SELECT u.id, u.first_name, u.last_name, u.telegram_id, u.chat_id,
		u.weight, u.created_at, u.updated_at
		FROM users u
		INNER JOIN team_admins ta ON ta.user_id = u.id
		WHERE ta.team_id = $1
		ORDER BY u.last_name, u.first_name`
	rows, err := r.DB.QueryContext(ctx, query, teamID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	for rows.Next() {
		var u domain.User
		if err := rows.Scan(&u.ID, &u.FirstName, &u.LastName,
			&u.TelegramID, &u.ChatID, &u.Weight,
			&u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		users = append(users, u)
	}
	return users, nil
}

// GetTeamIDsByAdminUserID возвращает ID всех команд, для которых пользователь
// назначен team-admin.
func (r *Repository) GetTeamIDsByAdminUserID(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	op := "Repository.GetTeamIDsByAdminUserID"
	var ids []uuid.UUID
	query := `SELECT team_id FROM team_admins WHERE user_id = $1`
	if err := r.DB.SelectContext(ctx, &ids, query, userID); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return ids, nil
}

// IsTeamAdmin проверяет, является ли пользователь team-admin конкретной
// команды.
func (r *Repository) IsTeamAdmin(ctx context.Context, userID, teamID uuid.UUID) (bool, error) {
	op := "Repository.IsTeamAdmin"
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM team_admins WHERE user_id = $1 AND team_id = $2)`
	if err := r.DB.GetContext(ctx, &exists, query, userID, teamID); err != nil {
		return false, fmt.Errorf("%s: %w", op, err)
	}
	return exists, nil
}

// IsTeamAdminOfAny проверяет, является ли пользователь team-admin хотя бы
// одной команды.
func (r *Repository) IsTeamAdminOfAny(ctx context.Context, userID uuid.UUID) (bool, error) {
	op := "Repository.IsTeamAdminOfAny"
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM team_admins WHERE user_id = $1)`
	if err := r.DB.GetContext(ctx, &exists, query, userID); err != nil {
		return false, fmt.Errorf("%s: %w", op, err)
	}
	return exists, nil
}
