package repositories

import (
	"EpicScoreBot/internal/models/domain"
	"context"
	"fmt"

	"github.com/google/uuid"
)

// GetAllRoles returns all roles.
func (r *Repository) GetAllRoles(ctx context.Context) ([]domain.Role, error) {
	op := "Repository.GetAllRoles"
	var roles []domain.Role
	query := `SELECT id, name, description FROM roles ORDER BY name`
	rows, err := r.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	for rows.Next() {
		var role domain.Role
		if err := rows.Scan(&role.ID, &role.Name, &role.Description); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		roles = append(roles, role)
	}
	return roles, nil
}

// GetRoleByID returns a role by ID.
func (r *Repository) GetRoleByID(ctx context.Context, roleID uuid.UUID) (*domain.Role, error) {
	op := "Repository.GetRoleByID"
	var role domain.Role
	query := `SELECT id, name, description FROM roles WHERE id = $1`
	err := r.DB.QueryRowContext(ctx, query, roleID).
		Scan(&role.ID, &role.Name, &role.Description)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return &role, nil
}

// GetRoleByName returns a role by name.
func (r *Repository) GetRoleByName(ctx context.Context, name string) (*domain.Role, error) {
	op := "Repository.GetRoleByName"
	var role domain.Role
	query := `SELECT id, name, description FROM roles WHERE name = $1`
	err := r.DB.QueryRowContext(ctx, query, name).
		Scan(&role.ID, &role.Name, &role.Description)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return &role, nil
}

// GetRoleByUserID returns the role assigned to a user.
// A user can only have one role at a time.
func (r *Repository) GetRoleByUserID(ctx context.Context, userID uuid.UUID) (*domain.Role, error) {
	op := "Repository.GetRoleByUserID"
	var role domain.Role
	query := `SELECT r.id, r.name, r.description
		FROM roles r
		INNER JOIN user_roles ur ON r.id = ur.role_id
		WHERE ur.user_id = $1
		LIMIT 1`
	err := r.DB.QueryRowContext(ctx, query, userID).
		Scan(&role.ID, &role.Name, &role.Description)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return &role, nil
}
