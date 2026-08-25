package services

import (
	"context"
	"errors"
	"testing"

	"EpicScoreBot/internal/models/domain"

	"github.com/google/uuid"
)

func TestTeamAdminService_AssignRemove(t *testing.T) {
	ctx := context.Background()
	log := newDiscardLogger()

	userID := uuid.New()
	teamID := uuid.New()
	assignedBy := uuid.New()

	t.Run("успешное назначение", func(t *testing.T) {
		var gotUserID, gotTeamID, gotAssignedBy uuid.UUID
		repo := &MockRepository{
			AssignTeamAdminFunc: func(ctx context.Context, uID, tID, aBy uuid.UUID) error {
				gotUserID, gotTeamID, gotAssignedBy = uID, tID, aBy
				return nil
			},
		}
		s := NewTeamAdminService(log, repo)
		if err := s.AssignTeamAdmin(ctx, userID, teamID, assignedBy); err != nil {
			t.Fatalf("ожидалось успешное назначение, получена ошибка: %v", err)
		}
		if gotUserID != userID || gotTeamID != teamID || gotAssignedBy != assignedBy {
			t.Errorf("репозиторий вызван с неверными параметрами: %v %v %v", gotUserID, gotTeamID, gotAssignedBy)
		}
	})

	t.Run("ошибка репозитория при назначении пробрасывается", func(t *testing.T) {
		wantErr := errors.New("db error")
		repo := &MockRepository{
			AssignTeamAdminFunc: func(ctx context.Context, uID, tID, aBy uuid.UUID) error {
				return wantErr
			},
		}
		s := NewTeamAdminService(log, repo)
		if err := s.AssignTeamAdmin(ctx, userID, teamID, assignedBy); !errors.Is(err, wantErr) {
			t.Errorf("ожидалась ошибка %v, получена %v", wantErr, err)
		}
	})

	t.Run("успешное снятие", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			RemoveTeamAdminFunc: func(ctx context.Context, uID, tID uuid.UUID) error {
				called = true
				return nil
			},
		}
		s := NewTeamAdminService(log, repo)
		if err := s.RemoveTeamAdmin(ctx, userID, teamID); err != nil {
			t.Fatalf("ожидалось успешное снятие, получена ошибка: %v", err)
		}
		if !called {
			t.Error("ожидался вызов RemoveTeamAdmin репозитория")
		}
	})
}

func TestTeamAdminService_Queries(t *testing.T) {
	ctx := context.Background()
	log := newDiscardLogger()

	userID := uuid.New()
	teamID := uuid.New()

	t.Run("GetTeamAdminsByTeamID делегирует репозиторию", func(t *testing.T) {
		want := []domain.User{{ID: uuid.New(), FirstName: "Иван"}}
		repo := &MockRepository{
			GetTeamAdminsByTeamIDFunc: func(ctx context.Context, tID uuid.UUID) ([]domain.User, error) {
				if tID != teamID {
					t.Errorf("ожидался team_id %v, получен %v", teamID, tID)
				}
				return want, nil
			},
		}
		s := NewTeamAdminService(log, repo)
		got, err := s.GetTeamAdminsByTeamID(ctx, teamID)
		if err != nil {
			t.Fatalf("неожиданная ошибка: %v", err)
		}
		if len(got) != 1 || got[0].ID != want[0].ID {
			t.Errorf("неверный результат: %+v", got)
		}
	})

	t.Run("GetTeamIDsByAdminUserID делегирует репозиторию", func(t *testing.T) {
		want := []uuid.UUID{teamID}
		repo := &MockRepository{
			GetTeamIDsByAdminUserIDFunc: func(ctx context.Context, uID uuid.UUID) ([]uuid.UUID, error) {
				if uID != userID {
					t.Errorf("ожидался user_id %v, получен %v", userID, uID)
				}
				return want, nil
			},
		}
		s := NewTeamAdminService(log, repo)
		got, err := s.GetTeamIDsByAdminUserID(ctx, userID)
		if err != nil {
			t.Fatalf("неожиданная ошибка: %v", err)
		}
		if len(got) != 1 || got[0] != teamID {
			t.Errorf("неверный результат: %+v", got)
		}
	})

	t.Run("IsTeamAdmin делегирует репозиторию", func(t *testing.T) {
		repo := &MockRepository{
			IsTeamAdminFunc: func(ctx context.Context, uID, tID uuid.UUID) (bool, error) {
				return uID == userID && tID == teamID, nil
			},
		}
		s := NewTeamAdminService(log, repo)
		ok, err := s.IsTeamAdmin(ctx, userID, teamID)
		if err != nil {
			t.Fatalf("неожиданная ошибка: %v", err)
		}
		if !ok {
			t.Error("ожидался true")
		}
	})

	t.Run("IsTeamAdminOfAny делегирует репозиторию", func(t *testing.T) {
		repo := &MockRepository{
			IsTeamAdminOfAnyFunc: func(ctx context.Context, uID uuid.UUID) (bool, error) {
				return uID == userID, nil
			},
		}
		s := NewTeamAdminService(log, repo)
		ok, err := s.IsTeamAdminOfAny(ctx, userID)
		if err != nil {
			t.Fatalf("неожиданная ошибка: %v", err)
		}
		if !ok {
			t.Error("ожидался true")
		}
	})
}
