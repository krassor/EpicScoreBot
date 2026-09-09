package repositories

import (
	"EpicScoreBot/internal/models/domain"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestTaskAssignments_UpsertResetReadBatch проверяет CRUD-контракт
// пользовательского ввода task_assignments (задача 1.3): upsert закрепления
// исполнителя и смещения старта, сброс закрепления (user_id -> NULL) и
// пачечное чтение всех записей команды одним запросом.
func TestTaskAssignments_UpsertResetReadBatch(t *testing.T) {
	repo, cleanup := newTestRepository(t)
	defer cleanup()
	ctx := context.Background()

	team, err := repo.CreateTeam(ctx, "Team-"+uuid.New().String()[:8], "")
	if err != nil {
		t.Fatalf("failed to create team: %v", err)
	}

	analyst, err := repo.GetRoleByName(ctx, "Аналитик")
	if err != nil {
		t.Fatalf("failed to get role: %v", err)
	}

	epic, err := repo.CreateEpic(ctx, "E-1", "Epic One", "", team.ID, 2026, 3, "feature", nil)
	if err != nil {
		t.Fatalf("failed to create epic: %v", err)
	}

	user, err := repo.CreateUser(ctx, "First", "Last", "tg_"+uuid.New().String(), 100)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// До первого upsert записей нет.
	assignments, err := repo.GetTaskAssignmentsByTeamID(ctx, team.ID)
	if err != nil {
		t.Fatalf("GetTaskAssignmentsByTeamID: %v", err)
	}
	if len(assignments) != 0 {
		t.Fatalf("expected no assignments before any upsert, got %d", len(assignments))
	}

	// Upsert смещения старта создаёт строку с user_id = NULL (автоматически).
	if err := repo.UpsertTaskAssignmentStartOffset(ctx, epic.ID, analyst.ID, 3); err != nil {
		t.Fatalf("UpsertTaskAssignmentStartOffset (create): %v", err)
	}
	assignments, err = repo.GetTaskAssignmentsByTeamID(ctx, team.ID)
	if err != nil {
		t.Fatalf("GetTaskAssignmentsByTeamID: %v", err)
	}
	if len(assignments) != 1 {
		t.Fatalf("expected 1 assignment after offset upsert, got %d", len(assignments))
	}
	if assignments[0].UserID != nil {
		t.Errorf("expected UserID nil after offset-only upsert, got %v", assignments[0].UserID)
	}
	if assignments[0].StartOffsetDays != 3 {
		t.Errorf("StartOffsetDays = %d, want 3", assignments[0].StartOffsetDays)
	}

	// Upsert закрепления исполнителя на ту же пару (epic_id, role_id) не
	// должен затереть уже выставленное смещение старта (design.md Решение 4:
	// закрепление и смещение — независимые пользовательские действия над
	// одной строкой).
	if err := repo.UpsertTaskAssignmentUser(ctx, epic.ID, analyst.ID, &user.ID); err != nil {
		t.Fatalf("UpsertTaskAssignmentUser (pin): %v", err)
	}
	assignments, err = repo.GetTaskAssignmentsByTeamID(ctx, team.ID)
	if err != nil {
		t.Fatalf("GetTaskAssignmentsByTeamID: %v", err)
	}
	if len(assignments) != 1 {
		t.Fatalf("expected still 1 assignment row (same key), got %d", len(assignments))
	}
	got := assignments[0]
	if got.UserID == nil || *got.UserID != user.ID {
		t.Errorf("UserID = %v, want %v", got.UserID, user.ID)
	}
	if got.StartOffsetDays != 3 {
		t.Errorf("StartOffsetDays after pin = %d, want 3 (preserved)", got.StartOffsetDays)
	}

	// Изменение смещения старта не должно затереть закрепление.
	if err := repo.UpsertTaskAssignmentStartOffset(ctx, epic.ID, analyst.ID, -2); err != nil {
		t.Fatalf("UpsertTaskAssignmentStartOffset (update): %v", err)
	}
	assignments, err = repo.GetTaskAssignmentsByTeamID(ctx, team.ID)
	if err != nil {
		t.Fatalf("GetTaskAssignmentsByTeamID: %v", err)
	}
	got = assignments[0]
	if got.UserID == nil || *got.UserID != user.ID {
		t.Errorf("UserID after offset update = %v, want %v (preserved)", got.UserID, user.ID)
	}
	if got.StartOffsetDays != -2 {
		t.Errorf("StartOffsetDays = %d, want -2", got.StartOffsetDays)
	}

	// Сброс закрепления (возврат к автоматическому распределению): user_id -> NULL,
	// строка не удаляется, смещение сохраняется.
	if err := repo.UpsertTaskAssignmentUser(ctx, epic.ID, analyst.ID, nil); err != nil {
		t.Fatalf("UpsertTaskAssignmentUser (unpin): %v", err)
	}
	assignments, err = repo.GetTaskAssignmentsByTeamID(ctx, team.ID)
	if err != nil {
		t.Fatalf("GetTaskAssignmentsByTeamID: %v", err)
	}
	if len(assignments) != 1 {
		t.Fatalf("expected row to survive unpin, got %d rows", len(assignments))
	}
	got = assignments[0]
	if got.UserID != nil {
		t.Errorf("UserID after unpin = %v, want nil", got.UserID)
	}
	if got.StartOffsetDays != -2 {
		t.Errorf("StartOffsetDays after unpin = %d, want -2 (preserved)", got.StartOffsetDays)
	}
}

// TestGetTaskAssignmentsByTeamID_ScopedToTeam проверяет, что пачечное
// чтение закреплений одной команды не подтягивает записи другой (design.md
// Решение 8: загрузка считается в границах команды).
func TestGetTaskAssignmentsByTeamID_ScopedToTeam(t *testing.T) {
	repo, cleanup := newTestRepository(t)
	defer cleanup()
	ctx := context.Background()

	teamA, err := repo.CreateTeam(ctx, "Team-A-"+uuid.New().String()[:8], "")
	if err != nil {
		t.Fatalf("failed to create teamA: %v", err)
	}
	teamB, err := repo.CreateTeam(ctx, "Team-B-"+uuid.New().String()[:8], "")
	if err != nil {
		t.Fatalf("failed to create teamB: %v", err)
	}

	role, err := repo.GetRoleByName(ctx, "BE разработчик")
	if err != nil {
		t.Fatalf("failed to get role: %v", err)
	}

	epicA, err := repo.CreateEpic(ctx, "EA-1", "Epic A", "", teamA.ID, 2026, 3, "feature", nil)
	if err != nil {
		t.Fatalf("failed to create epicA: %v", err)
	}
	epicB, err := repo.CreateEpic(ctx, "EB-1", "Epic B", "", teamB.ID, 2026, 3, "feature", nil)
	if err != nil {
		t.Fatalf("failed to create epicB: %v", err)
	}

	if err := repo.UpsertTaskAssignmentStartOffset(ctx, epicA.ID, role.ID, 1); err != nil {
		t.Fatalf("upsert teamA assignment: %v", err)
	}
	if err := repo.UpsertTaskAssignmentStartOffset(ctx, epicB.ID, role.ID, 2); err != nil {
		t.Fatalf("upsert teamB assignment: %v", err)
	}

	assignmentsA, err := repo.GetTaskAssignmentsByTeamID(ctx, teamA.ID)
	if err != nil {
		t.Fatalf("GetTaskAssignmentsByTeamID teamA: %v", err)
	}
	if len(assignmentsA) != 1 || assignmentsA[0].EpicID != epicA.ID {
		t.Fatalf("expected teamA to see only its own assignment, got %+v", assignmentsA)
	}

	assignmentsB, err := repo.GetTaskAssignmentsByTeamID(ctx, teamB.ID)
	if err != nil {
		t.Fatalf("GetTaskAssignmentsByTeamID teamB: %v", err)
	}
	if len(assignmentsB) != 1 || assignmentsB[0].EpicID != epicB.ID {
		t.Fatalf("expected teamB to see only its own assignment, got %+v", assignmentsB)
	}
}

// TestUpdateGanttTaskAssignee_RoundTrip проверяет запись/чтение
// gantt_tasks.assignee_id (производного состояния диаграммы, в отличие от
// task_assignments — пользовательского ввода).
func TestUpdateGanttTaskAssignee_RoundTrip(t *testing.T) {
	repo, cleanup := newTestRepository(t)
	defer cleanup()
	ctx := context.Background()

	team, err := repo.CreateTeam(ctx, "Team-"+uuid.New().String()[:8], "")
	if err != nil {
		t.Fatalf("failed to create team: %v", err)
	}
	epic, err := repo.CreateEpic(ctx, "E-1", "Epic One", "", team.ID, 2026, 3, "feature", nil)
	if err != nil {
		t.Fatalf("failed to create epic: %v", err)
	}
	role, err := repo.GetRoleByName(ctx, "Аналитик")
	if err != nil {
		t.Fatalf("failed to get role: %v", err)
	}
	user, err := repo.CreateUser(ctx, "First", "Last", "tg_"+uuid.New().String(), 100)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	now := time.Now()
	task, err := repo.CreateGanttTask(ctx, &domain.GanttTask{
		EpicID:    epic.ID,
		RoleID:    &role.ID,
		Name:      role.Name,
		StartDate: now,
		EndDate:   now,
		IsParent:  false,
	})
	if err != nil {
		t.Fatalf("CreateGanttTask: %v", err)
	}
	if task.AssigneeID != nil {
		t.Fatalf("expected AssigneeID nil right after creation, got %v", task.AssigneeID)
	}

	if err := repo.UpdateGanttTaskAssignee(ctx, task.ID, &user.ID); err != nil {
		t.Fatalf("UpdateGanttTaskAssignee (set): %v", err)
	}
	got, err := repo.GetGanttTaskByID(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetGanttTaskByID: %v", err)
	}
	if got.AssigneeID == nil || *got.AssigneeID != user.ID {
		t.Errorf("AssigneeID = %v, want %v", got.AssigneeID, user.ID)
	}

	if err := repo.UpdateGanttTaskAssignee(ctx, task.ID, nil); err != nil {
		t.Fatalf("UpdateGanttTaskAssignee (clear): %v", err)
	}
	got, err = repo.GetGanttTaskByID(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetGanttTaskByID: %v", err)
	}
	if got.AssigneeID != nil {
		t.Errorf("AssigneeID after clear = %v, want nil", got.AssigneeID)
	}
}
