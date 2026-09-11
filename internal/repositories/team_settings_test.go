package repositories

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// TestTeamBackfillBlockPast_DefaultAndReadWrite проверяет задачу 1.2 заявки
// backfill-idle-gaps-in-schedule: настройка команды «не занимать промежутки
// расписания в прошлом» по умолчанию выключена (design.md Решение 3,
// Migration Plan) и корректно читается/пишется через репозиторий.
func TestTeamBackfillBlockPast_DefaultAndReadWrite(t *testing.T) {
	repo, cleanup := newTestRepository(t)
	defer cleanup()
	ctx := context.Background()

	team, err := repo.CreateTeam(ctx, "Team-"+uuid.New().String()[:8], "")
	if err != nil {
		t.Fatalf("failed to create team: %v", err)
	}
	if team.BackfillBlockPast {
		t.Fatalf("expected BackfillBlockPast to default to false on CreateTeam, got true")
	}

	got, err := repo.GetTeamByID(ctx, team.ID)
	if err != nil {
		t.Fatalf("failed to get team: %v", err)
	}
	if got.BackfillBlockPast {
		t.Fatalf("expected BackfillBlockPast to default to false, got true")
	}

	if err := repo.UpdateTeamBackfillBlockPast(ctx, team.ID, true); err != nil {
		t.Fatalf("failed to enable setting: %v", err)
	}

	got, err = repo.GetTeamByID(ctx, team.ID)
	if err != nil {
		t.Fatalf("failed to get team: %v", err)
	}
	if !got.BackfillBlockPast {
		t.Fatalf("expected BackfillBlockPast to be true after update, got false")
	}

	// GetTeamByName и GetAllTeams должны видеть то же значение — оно читается
	// тем же столбцом, что и GetTeamByID.
	byName, err := repo.GetTeamByName(ctx, got.Name)
	if err != nil {
		t.Fatalf("failed to get team by name: %v", err)
	}
	if !byName.BackfillBlockPast {
		t.Fatalf("expected BackfillBlockPast to be true via GetTeamByName, got false")
	}

	if err := repo.UpdateTeamBackfillBlockPast(ctx, team.ID, false); err != nil {
		t.Fatalf("failed to disable setting: %v", err)
	}
	got, err = repo.GetTeamByID(ctx, team.ID)
	if err != nil {
		t.Fatalf("failed to get team: %v", err)
	}
	if got.BackfillBlockPast {
		t.Fatalf("expected BackfillBlockPast to be false after disabling, got true")
	}
}
