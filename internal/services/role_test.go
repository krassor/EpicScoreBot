package services

import (
	"context"
	"errors"
	"testing"

	"EpicScoreBot/internal/models/domain"

	"github.com/google/uuid"
)

func TestRoleService(t *testing.T) {
	ctx := context.Background()
	log := newDiscardLogger()
	userID := uuid.New()
	roleID := uuid.New()
	dbErr := errors.New("database error")

	t.Run("GetAllRoles успешный сценарий и ошибка", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			GetAllRolesFunc: func(ctx context.Context) ([]domain.Role, error) {
				called = true
				return []domain.Role{{ID: roleID, Name: "IT-leader"}}, nil
			},
		}

		s := NewRoleService(log, repo)
		roles, err := s.GetAllRoles(ctx)
		if err != nil || !called || len(roles) != 1 || roles[0].ID != roleID {
			t.Errorf("ошибка трансляции GetAllRoles: err=%v, called=%t, roles=%v", err, called, roles)
		}

		// Тест ошибки
		repo.GetAllRolesFunc = func(ctx context.Context) ([]domain.Role, error) {
			return nil, dbErr
		}
		_, err = s.GetAllRoles(ctx)
		if !errors.Is(err, dbErr) {
			t.Errorf("ожидалась ошибка %v, получена: %v", dbErr, err)
		}
	})

	t.Run("GetRoleByID успешный сценарий и ошибка", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			GetRoleByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
				called = true
				if id != roleID {
					t.Errorf("неверный id: %v", id)
				}
				return &domain.Role{ID: id, Name: "analyst"}, nil
			},
		}

		s := NewRoleService(log, repo)
		role, err := s.GetRoleByID(ctx, roleID)
		if err != nil || !called || role == nil || role.ID != roleID {
			t.Errorf("ошибка трансляции GetRoleByID: err=%v, called=%t, role=%v", err, called, role)
		}

		// Тест ошибки
		repo.GetRoleByIDFunc = func(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
			return nil, dbErr
		}
		_, err = s.GetRoleByID(ctx, roleID)
		if !errors.Is(err, dbErr) {
			t.Errorf("ожидалась ошибка %v, получена: %v", dbErr, err)
		}
	})

	t.Run("GetRoleByUserID успешный сценарий и ошибка", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			GetRoleByUserIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
				called = true
				if id != userID {
					t.Errorf("неверный id: %v", id)
				}
				return &domain.Role{ID: roleID, Name: "developer"}, nil
			},
		}

		s := NewRoleService(log, repo)
		role, err := s.GetRoleByUserID(ctx, userID)
		if err != nil || !called || role == nil || role.ID != roleID {
			t.Errorf("ошибка трансляции GetRoleByUserID: err=%v, called=%t, role=%v", err, called, role)
		}

		// Тест ошибки
		repo.GetRoleByUserIDFunc = func(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
			return nil, dbErr
		}
		_, err = s.GetRoleByUserID(ctx, userID)
		if !errors.Is(err, dbErr) {
			t.Errorf("ожидалась ошибка %v, получена: %v", dbErr, err)
		}
	})

	t.Run("AssignUserRole успешный сценарий и ошибка", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			AssignUserRoleFunc: func(ctx context.Context, uID, rID uuid.UUID) error {
				called = true
				if uID != userID || rID != roleID {
					t.Errorf("неверные параметры: uID=%v, rID=%v", uID, rID)
				}
				return nil
			},
		}

		s := NewRoleService(log, repo)
		err := s.AssignUserRole(ctx, userID, roleID)
		if err != nil || !called {
			t.Errorf("ошибка трансляции AssignUserRole: err=%v, called=%t", err, called)
		}

		// Тест ошибки
		repo.AssignUserRoleFunc = func(ctx context.Context, uID, rID uuid.UUID) error {
			return dbErr
		}
		err = s.AssignUserRole(ctx, userID, roleID)
		if !errors.Is(err, dbErr) {
			t.Errorf("ожидалась ошибка %v, получена: %v", dbErr, err)
		}
	})

	t.Run("RemoveUserRole успешный сценарий и ошибка", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			RemoveUserRoleFunc: func(ctx context.Context, uID, rID uuid.UUID) error {
				called = true
				if uID != userID || rID != roleID {
					t.Errorf("неверные параметры: uID=%v, rID=%v", uID, rID)
				}
				return nil
			},
		}

		s := NewRoleService(log, repo)
		err := s.RemoveUserRole(ctx, userID, roleID)
		if err != nil || !called {
			t.Errorf("ошибка трансляции RemoveUserRole: err=%v, called=%t", err, called)
		}

		// Тест ошибки
		repo.RemoveUserRoleFunc = func(ctx context.Context, uID, rID uuid.UUID) error {
			return dbErr
		}
		err = s.RemoveUserRole(ctx, userID, roleID)
		if !errors.Is(err, dbErr) {
			t.Errorf("ожидалась ошибка %v, получена: %v", dbErr, err)
		}
	})
}
