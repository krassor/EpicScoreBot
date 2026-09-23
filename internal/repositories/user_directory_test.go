package repositories

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// TestFindUserByTelegramID_NormalizedComparison проверяет design.md
// Решение 3 заявки fix-webapp-user-identity: FindUserByTelegramID находит
// пользователя независимо от регистра и ведущего "@" в users.telegram_id —
// справочник заполнялся разными путями (бот срезает "@", CSV-импорт — нет)
// и содержит оба написания; параметр приходит уже нормализованным из
// middleware.UserSession.DirectoryKey().
func TestFindUserByTelegramID_NormalizedComparison(t *testing.T) {
	repo, cleanup := newTestRepository(t)
	defer cleanup()
	ctx := context.Background()

	// Запись занесена с ведущим "@" и в смешанном регистре — как её мог
	// оставить пакетный CSV-импорт (в отличие от бота, срезающего "@").
	user, err := repo.CreateUser(ctx, "Ivan", "Ivanov", "@Ivan_Petrov", 100)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// Ключ приходит уже нормализованным — так его отдаёт DirectoryKey().
	got, err := repo.FindUserByTelegramID(ctx, "ivan_petrov")
	if err != nil {
		t.Fatalf("FindUserByTelegramID(normalized key) failed: %v", err)
	}
	if got.ID != user.ID {
		t.Errorf("expected to find user %s, got %s", user.ID, got.ID)
	}

	// Незарегистрированный ключ по-прежнему не находится — нормализация не
	// подставляет произвольную учётную запись.
	if _, err := repo.FindUserByTelegramID(ctx, "someone_else"); err == nil {
		t.Error("expected an error for an unregistered directory key")
	}
}

// TestGetTeamsByUserTelegramID_NormalizedComparison — то же самое (design.md
// Решение 3), но для GetTeamsByUserTelegramID: команда должна находиться по
// нормализованному ключу независимо от написания telegram_id в справочнике.
func TestGetTeamsByUserTelegramID_NormalizedComparison(t *testing.T) {
	repo, cleanup := newTestRepository(t)
	defer cleanup()
	ctx := context.Background()

	team, err := repo.CreateTeam(ctx, "Team-"+uuid.New().String()[:8], "")
	if err != nil {
		t.Fatalf("failed to create team: %v", err)
	}

	// Запись занесена ботом — без ведущего "@", но в исходном регистре
	// пользователя (в отличие от нормализованного ключа сессии).
	user, err := repo.CreateUser(ctx, "Anna", "Sidorova", "Anna_Sidorova", 100)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	if err := repo.AssignUserTeam(ctx, user.ID, team.ID); err != nil {
		t.Fatalf("failed to assign user to team: %v", err)
	}

	teams, err := repo.GetTeamsByUserTelegramID(ctx, "anna_sidorova")
	if err != nil {
		t.Fatalf("GetTeamsByUserTelegramID(normalized key) failed: %v", err)
	}
	if len(teams) != 1 || teams[0].ID != team.ID {
		t.Errorf("expected to find team %s, got %+v", team.ID, teams)
	}
}
