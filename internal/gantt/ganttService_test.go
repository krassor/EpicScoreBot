package gantt

import (
	"EpicScoreBot/internal/models/domain"
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func floatPtr(f float64) *float64 {
	return &f
}

// storyName mirrors the "<number>: <name>" format used to build story/epic
// Gantt task names in GenerateTasksForEpic/recalculateEpicSchedule.
func storyName(e *domain.Epic) string {
	return fmt.Sprintf("%s: %s", e.Number, e.Name)
}

// findTaskByParentAndName locates a task by its parent task ID and name
// (used to find a specific role task after generation/recalculation).
func findTaskByParentAndName(f *fakeRepo, parentID uuid.UUID, name string) *domain.GanttTask {
	for _, t := range f.tasks {
		if t.ParentTaskID != nil && *t.ParentTaskID == parentID && t.Name == name {
			return t
		}
	}
	return nil
}

// findStoryTask locates a story-level (IsParent) task by its epic-scoped
// name (e.g. "E-1-S1: Story 1"), regardless of its current ParentTaskID.
func findStoryTask(f *fakeRepo, name string) *domain.GanttTask {
	for _, t := range f.tasks {
		if t.IsParent && t.Name == name {
			return t
		}
	}
	return nil
}

func TestMoveToWorkDay(t *testing.T) {
	tests := []struct {
		name  string
		input time.Time
		want  time.Time
	}{
		{
			name:  "Monday remains Monday",
			input: time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC), // Monday
			want:  time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC),
		},
		{
			name:  "Friday remains Friday",
			input: time.Date(2026, 7, 10, 15, 30, 0, 0, time.UTC), // Friday
			want:  time.Date(2026, 7, 10, 15, 30, 0, 0, time.UTC),
		},
		{
			name:  "Saturday moves to Monday",
			input: time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC), // Saturday
			want:  time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC), // Monday
		},
		{
			name:  "Sunday moves to Monday",
			input: time.Date(2026, 7, 12, 9, 0, 0, 0, time.UTC), // Sunday
			want:  time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC), // Monday
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := moveToWorkDay(tt.input)
			if !got.Equal(tt.want) {
				t.Errorf("moveToWorkDay(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestAddWorkDays(t *testing.T) {
	tests := []struct {
		name  string
		start time.Time
		days  int
		want  time.Time
	}{
		{
			name:  "Monday add 1 day",
			start: time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
			days:  1,
			want:  time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
		},
		{
			name:  "Monday add 5 days (mon-tue-wed-thu-fri)",
			start: time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
			days:  5,
			want:  time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC),
		},
		{
			name:  "Friday add 2 days (fri-mon)",
			start: time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC),
			days:  2,
			want:  time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
		},
		{
			name:  "Saturday add 2 days (sat becomes mon, + 1 workday -> tue)",
			start: time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC),
			days:  2,
			want:  time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := addWorkDays(tt.start, tt.days)
			if !got.Equal(tt.want) {
				t.Errorf("addWorkDays(%v, %d) = %v, want %v", tt.start, tt.days, got, tt.want)
			}
		})
	}
}

func TestCountWorkDays(t *testing.T) {
	tests := []struct {
		name  string
		start time.Time
		end   time.Time
		want  int
	}{
		{
			name:  "Same day Monday",
			start: time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
			end:   time.Date(2026, 7, 13, 23, 59, 59, 0, time.UTC),
			want:  1,
		},
		{
			name:  "Same day Saturday",
			start: time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC),
			end:   time.Date(2026, 7, 11, 23, 59, 59, 0, time.UTC),
			want:  0,
		},
		{
			name:  "Monday to Friday",
			start: time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC),
			end:   time.Date(2026, 7, 17, 15, 0, 0, 0, time.UTC),
			want:  5,
		},
		{
			name:  "Friday to next Monday",
			start: time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC),
			end:   time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
			want:  2,
		},
		{
			name:  "Weekend only",
			start: time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC),
			end:   time.Date(2026, 7, 12, 23, 0, 0, 0, time.UTC),
			want:  0,
		},
		{
			name:  "Start after end",
			start: time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
			end:   time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC),
			want:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countWorkDays(tt.start, tt.end)
			if got != tt.want {
				t.Errorf("countWorkDays(%v, %v) = %d, want %d", tt.start, tt.end, got, tt.want)
			}
		})
	}
}

// TestRiskCoefficient проверяет граничные значения взвешенного риск-скора,
// от которых напрямую зависит длительность (workDays) ролевых задач в
// конвейерном планировщике.
func TestRiskCoefficient(t *testing.T) {
	tests := []struct {
		name          string
		weightedScore float64
		want          float64
	}{
		{name: "well below lowest threshold", weightedScore: 0, want: 1.03},
		{name: "just below 5 -> lowest tier", weightedScore: 4, want: 1.03},
		{name: "exactly 5 -> crosses into 1.05 tier", weightedScore: 5, want: 1.05},
		{name: "just below 9 -> still 1.05 tier", weightedScore: 8, want: 1.05},
		{name: "exactly 9 -> crosses into 1.10 tier", weightedScore: 9, want: 1.10},
		{name: "just below 13 -> still 1.10 tier", weightedScore: 12, want: 1.10},
		{name: "exactly 13 -> crosses into 1.20 tier", weightedScore: 13, want: 1.20},
		{name: "well above highest threshold", weightedScore: 20, want: 1.20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RiskCoefficient(tt.weightedScore)
			if got != tt.want {
				t.Errorf("RiskCoefficient(%v) = %v, want %v", tt.weightedScore, got, tt.want)
			}
		})
	}
}

// TestGenerateTasksForEpic_Legacy проверяет генерацию плоского Ганта для
// эпика без сторей (обратная совместимость) и итоговую раскладку дат ролей.
func TestGenerateTasksForEpic_Legacy(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	roleID1 := uuid.New() // Аналитик
	roleID2 := uuid.New() // BE разработчик

	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic One", TeamID: teamID})
	f.addRole(&domain.Role{ID: roleID1, Name: "Аналитик"})
	f.addRole(&domain.Role{ID: roleID2, Name: "BE разработчик"})
	f.roleScores[epicID] = []domain.EpicRoleScore{
		{EpicID: epicID, RoleID: roleID1, WeightedAvg: 3.0},
		{EpicID: epicID, RoleID: roleID2, WeightedAvg: 2.0},
	}
	f.risks[epicID] = []domain.Risk{{EpicID: epicID, WeightedScore: floatPtr(8.0)}} // coeff 1.05

	// Wednesday start.
	start := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
	tasks, err := svc.GenerateTasksForEpic(ctx, epicID, start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 1 parent task + 2 child tasks.
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}

	parent := tasks[0]
	if !parent.IsParent {
		t.Fatalf("expected parent task first")
	}

	// Role 1 ("Аналитик"): 3.0 * 1.05 = 3.15 -> ceil -> 4 workDays: Wed Jul8 -> Mon Jul13.
	// Role 2 ("BE разработчик"): 2.0 * 1.05 = 2.10 -> ceil -> 3 workDays: Tue Jul14 -> Thu Jul16.
	expectedParentStart := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
	expectedParentEnd := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	if !parent.StartDate.Equal(expectedParentStart) {
		t.Errorf("parent start %v, want %v", parent.StartDate, expectedParentStart)
	}
	if !parent.EndDate.Equal(expectedParentEnd) {
		t.Errorf("parent end %v, want %v", parent.EndDate, expectedParentEnd)
	}

	analyst := findTaskByParentAndName(f, parent.ID, "Аналитик")
	if analyst == nil {
		t.Fatalf("analyst task not found")
	}
	expectedAnalystEnd := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	if !analyst.EndDate.Equal(expectedAnalystEnd) {
		t.Errorf("Аналитик end %v, want %v", analyst.EndDate, expectedAnalystEnd)
	}

	dev := findTaskByParentAndName(f, parent.ID, "BE разработчик")
	if dev == nil {
		t.Fatalf("dev task not found")
	}
	expectedDevStart := time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)
	expectedDevEnd := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	if !dev.StartDate.Equal(expectedDevStart) {
		t.Errorf("BE разработчик start %v, want %v", dev.StartDate, expectedDevStart)
	}
	if !dev.EndDate.Equal(expectedDevEnd) {
		t.Errorf("BE разработчик end %v, want %v", dev.EndDate, expectedDevEnd)
	}
}

// TestGenerateTasksForEpic_WithStories проверяет 3-уровневую генерацию
// (эпик -> стори -> роли) на эпике с одной сторей.
func TestGenerateTasksForEpic_WithStories(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	storyID := uuid.New()
	roleID1 := uuid.New()
	roleID2 := uuid.New()

	f.addEpic(&domain.Epic{ID: epicID, Number: "E-100", Name: "Parent Epic", TeamID: teamID})
	f.addEpic(&domain.Epic{ID: storyID, Number: "E-100-S1", Name: "Story 1", TeamID: teamID, ParentEpicID: &epicID})
	f.addRole(&domain.Role{ID: roleID1, Name: "Аналитик"})
	f.addRole(&domain.Role{ID: roleID2, Name: "BE разработчик"})
	f.roleScores[storyID] = []domain.EpicRoleScore{
		{EpicID: storyID, RoleID: roleID1, WeightedAvg: 3.0},
		{EpicID: storyID, RoleID: roleID2, WeightedAvg: 2.0},
	}

	start := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
	tasks, err := svc.GenerateTasksForEpic(ctx, epicID, start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tasks) != 4 {
		t.Fatalf("expected 4 tasks, got %d", len(tasks))
	}
	if tasks[0].IsParent != true || tasks[0].ParentTaskID != nil {
		t.Errorf("invalid parent epic task: %+v", tasks[0])
	}
	if tasks[1].IsParent != true || *tasks[1].ParentTaskID != tasks[0].ID {
		t.Errorf("invalid story task: %+v", tasks[1])
	}
	if tasks[2].IsParent != false || *tasks[2].ParentTaskID != tasks[1].ID {
		t.Errorf("invalid child role task 1: %+v", tasks[2])
	}
}

// TestReorderTask проверяет, что переупорядочение роли внутри стори
// (или, для legacy-эпика, внутри самого эпика) пересчитывает расписание.
func TestReorderTask(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	roleID1 := uuid.New()
	roleID2 := uuid.New()

	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic One", TeamID: teamID})
	f.addRole(&domain.Role{ID: roleID1, Name: "Аналитик"})
	f.addRole(&domain.Role{ID: roleID2, Name: "BE разработчик"})
	f.roleScores[epicID] = []domain.EpicRoleScore{
		{EpicID: epicID, RoleID: roleID1, WeightedAvg: 2.0},
		{EpicID: epicID, RoleID: roleID2, WeightedAvg: 2.0},
	}

	start := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC) // Monday
	_, err := svc.GenerateTasksForEpic(ctx, epicID, start)
	if err != nil {
		t.Fatalf("unexpected error generating tasks: %v", err)
	}

	var parentID uuid.UUID
	for _, task := range f.tasks {
		if task.IsParent {
			parentID = task.ID
		}
	}
	analyst := findTaskByParentAndName(f, parentID, "Аналитик")
	dev := findTaskByParentAndName(f, parentID, "BE разработчик")
	if analyst == nil || dev == nil {
		t.Fatalf("expected both role tasks to exist")
	}

	// До реордера: Аналитик идёт первым (sort_order 1), BE dev — вторым (sort_order 2).
	if !analyst.StartDate.Before(dev.StartDate) {
		t.Fatalf("expected Аналитик to start before BE разработчик before reorder")
	}

	// Меняем местами роли: BE dev теперь идёт первым, Аналитик — вторым
	// (как и при реальном drag&drop, фронтенд пересылает новый sort_order
	// для каждого элемента списка).
	if _, err := svc.ReorderTask(ctx, dev.ID, 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	updated, err := svc.ReorderTask(ctx, analyst.ID, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(updated) == 0 {
		t.Fatalf("expected non-empty updated task list")
	}

	devAfter := f.tasks[dev.ID]
	analystAfter := f.tasks[analyst.ID]
	if !devAfter.StartDate.Before(analystAfter.StartDate) {
		t.Errorf("expected BE разработчик to start before Аналитик after reorder, got dev=%v analyst=%v",
			devAfter.StartDate, analystAfter.StartDate)
	}
}

// TestRecalculateTeamSchedule_Pipeline проверяет ключевое свойство
// конвейерного планировщика: роль не простаивает между сторями — как
// только она закончила задачу в стори N, она сразу берёт свою задачу в
// стори N+1, не дожидаясь другой роли (BE-разработчика) той же N-й стори.
func TestRecalculateTeamSchedule_Pipeline(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	story1ID := uuid.New()
	story2ID := uuid.New()
	analystID := uuid.New()
	devID := uuid.New()

	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})
	story1 := f.addEpic(&domain.Epic{ID: story1ID, Number: "E-1-S1", Name: "Story 1", TeamID: teamID, ParentEpicID: &epicID})
	story2 := f.addEpic(&domain.Epic{ID: story2ID, Number: "E-1-S2", Name: "Story 2", TeamID: teamID, ParentEpicID: &epicID})
	f.addRole(&domain.Role{ID: analystID, Name: "Аналитик"})
	f.addRole(&domain.Role{ID: devID, Name: "BE разработчик"})

	f.roleScores[story1ID] = []domain.EpicRoleScore{
		{EpicID: story1ID, RoleID: analystID, WeightedAvg: 2.0}, // 2 workdays
		{EpicID: story1ID, RoleID: devID, WeightedAvg: 3.0},     // 3 workdays
	}
	f.roleScores[story2ID] = []domain.EpicRoleScore{
		{EpicID: story2ID, RoleID: analystID, WeightedAvg: 1.0}, // 1 workday
		{EpicID: story2ID, RoleID: devID, WeightedAvg: 2.0},     // 2 workdays
	}

	start := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC) // Monday
	if _, err := svc.GenerateTasksForEpic(ctx, epicID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	story1Task := findStoryTask(f, storyName(story1))
	story2Task := findStoryTask(f, storyName(story2))
	if story1Task == nil || story2Task == nil {
		t.Fatalf("expected both story tasks to exist")
	}

	analystStory1 := findTaskByParentAndName(f, story1Task.ID, "Аналитик")
	devStory1 := findTaskByParentAndName(f, story1Task.ID, "BE разработчик")
	analystStory2 := findTaskByParentAndName(f, story2Task.ID, "Аналитик")
	devStory2 := findTaskByParentAndName(f, story2Task.ID, "BE разработчик")
	if analystStory1 == nil || devStory1 == nil || analystStory2 == nil || devStory2 == nil {
		t.Fatalf("expected all 4 role tasks to exist")
	}

	// Аналитик story1: Mon Jul13 - Tue Jul14 (2 workdays).
	if !analystStory1.StartDate.Equal(time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("analyst story1 start = %v, want Jul13", analystStory1.StartDate)
	}
	if !analystStory1.EndDate.Equal(time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("analyst story1 end = %v, want Jul14", analystStory1.EndDate)
	}

	// BE dev story1: Wed Jul15 - Fri Jul17 (starts after analyst story1 within the same story).
	if !devStory1.StartDate.Equal(time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("dev story1 start = %v, want Jul15", devStory1.StartDate)
	}
	if !devStory1.EndDate.Equal(time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("dev story1 end = %v, want Jul17", devStory1.EndDate)
	}

	// Ключевая проверка конвейера: Аналитик story2 стартует сразу после
	// своей задачи по story1 (Wed Jul15, на следующий рабочий день после
	// Tue Jul14) — НЕ дожидаясь BE-разработчика story1 (который заканчивает
	// только в Fri Jul17).
	if !analystStory2.StartDate.Equal(time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("analyst story2 start = %v, want Jul15 (immediately after analyst story1, not waiting for dev story1)",
			analystStory2.StartDate)
	}
	if !analystStory2.StartDate.Before(devStory1.EndDate) {
		t.Errorf("analyst story2 (%v) should start before dev story1 finishes (%v) — role must not idle",
			analystStory2.StartDate, devStory1.EndDate)
	}

	// BE dev story2 стартует после освобождения BE-разработчика от story1 (Mon Jul20).
	if !devStory2.StartDate.Equal(time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("dev story2 start = %v, want Jul20", devStory2.StartDate)
	}
}

// TestRecalculateTeamSchedule_FreezesStartedTask проверяет, что задача с
// Progress > 0 (начатая) не двигается планировщиком, даже если её текущие
// даты расходятся с тем, что рассчитал бы планировщик "с нуля".
func TestRecalculateTeamSchedule_FreezesStartedTask(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	roleID := uuid.New()

	epicFloor := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC) // Monday
	f.addRole(&domain.Role{ID: roleID, Name: "Аналитик"})
	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})

	epicParent, err := f.CreateGanttTask(ctx, &domain.GanttTask{
		EpicID: epicID, Name: "E-1: Epic", StartDate: epicFloor, EndDate: epicFloor, IsParent: true,
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Задача уже начата (Progress=40) и её плановые даты (Jul1-Jul10)
	// НЕ совпадают с тем, что рассчитал бы планировщик от epicFloor=Jul13.
	frozenStart := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	frozenEnd := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	frozenTask, err := f.CreateGanttTask(ctx, &domain.GanttTask{
		EpicID: epicID, RoleID: &roleID, Name: "Аналитик",
		StartDate: frozenStart, EndDate: frozenEnd,
		Progress: 40, SortOrder: 1, IsParent: false, ParentTaskID: &epicParent.ID,
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	if _, err := svc.RecalculateTeamSchedule(ctx, teamID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := f.tasks[frozenTask.ID]
	if !got.StartDate.Equal(frozenStart) || !got.EndDate.Equal(frozenEnd) {
		t.Errorf("frozen task dates changed: got [%v, %v], want [%v, %v]",
			got.StartDate, got.EndDate, frozenStart, frozenEnd)
	}
	if got.Progress != 40 {
		t.Errorf("frozen task progress changed: got %v, want 40", got.Progress)
	}

	// Прогресс эпика должен агрегироваться от единственного ребёнка (40%),
	// а его даты — от границ ребёнка (тоже застывших).
	gotParent := f.tasks[epicParent.ID]
	if gotParent.Progress != 40 {
		t.Errorf("epic progress = %v, want 40 (aggregated from the single frozen child)", gotParent.Progress)
	}
	if !gotParent.StartDate.Equal(frozenStart) || !gotParent.EndDate.Equal(frozenEnd) {
		t.Errorf("epic dates = [%v, %v], want [%v, %v] (aggregated from child bounds)",
			gotParent.StartDate, gotParent.EndDate, frozenStart, frozenEnd)
	}
}

// TestReorderStory_MovesSchedule проверяет, что реордер сторей внутри
// эпика меняет итоговую раскладку дат в соответствии с новым порядком.
func TestReorderStory_MovesSchedule(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	story1ID := uuid.New()
	story2ID := uuid.New()
	analystID := uuid.New()

	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})
	story1 := f.addEpic(&domain.Epic{ID: story1ID, Number: "E-1-S1", Name: "Story 1", TeamID: teamID, ParentEpicID: &epicID})
	story2 := f.addEpic(&domain.Epic{ID: story2ID, Number: "E-1-S2", Name: "Story 2", TeamID: teamID, ParentEpicID: &epicID})
	f.addRole(&domain.Role{ID: analystID, Name: "Аналитик"})

	f.roleScores[story1ID] = []domain.EpicRoleScore{{EpicID: story1ID, RoleID: analystID, WeightedAvg: 2.0}}
	f.roleScores[story2ID] = []domain.EpicRoleScore{{EpicID: story2ID, RoleID: analystID, WeightedAvg: 2.0}}

	start := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC) // Monday
	if _, err := svc.GenerateTasksForEpic(ctx, epicID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	story1Task := findStoryTask(f, storyName(story1))
	story2Task := findStoryTask(f, storyName(story2))
	analystStory1 := findTaskByParentAndName(f, story1Task.ID, "Аналитик")
	analystStory2 := findTaskByParentAndName(f, story2Task.ID, "Аналитик")

	// До реордера: story1 идёт первой (Jul13-14), story2 — второй (Jul15-16).
	if !analystStory1.StartDate.Equal(time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("precondition failed: analyst story1 start = %v, want Jul13", analystStory1.StartDate)
	}
	if !analystStory2.StartDate.Equal(time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("precondition failed: analyst story2 start = %v, want Jul15", analystStory2.StartDate)
	}

	// Меняем порядок: story2 теперь первая, story1 — вторая.
	if _, err := svc.ReorderStory(ctx, story2ID, 1); err != nil {
		t.Fatalf("unexpected error reordering story2: %v", err)
	}
	if _, err := svc.ReorderStory(ctx, story1ID, 2); err != nil {
		t.Fatalf("unexpected error reordering story1: %v", err)
	}

	gotAnalystStory1 := f.tasks[analystStory1.ID]
	gotAnalystStory2 := f.tasks[analystStory2.ID]

	if !gotAnalystStory2.StartDate.Equal(time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("after reorder: analyst story2 start = %v, want Jul13 (story2 now first)", gotAnalystStory2.StartDate)
	}
	if !gotAnalystStory1.StartDate.Equal(time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("after reorder: analyst story1 start = %v, want Jul15 (story1 now second)", gotAnalystStory1.StartDate)
	}

	// gantt_tasks.sort_order строк сторей должен быть синхронизирован с
	// новым порядком epics.sort_order (используется деревом рендера).
	gotStory1Task := f.tasks[story1Task.ID]
	gotStory2Task := f.tasks[story2Task.ID]
	if gotStory2Task.SortOrder >= gotStory1Task.SortOrder {
		t.Errorf("expected story2 gantt task sort_order (%d) < story1 (%d) after reorder",
			gotStory2Task.SortOrder, gotStory1Task.SortOrder)
	}
}

// TestReorderEpic_MovesSchedule проверяет, что реордер топ-эпиков в
// очереди команды меняет то, в каком порядке общая роль обслуживает эпики.
func TestReorderEpic_MovesSchedule(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epic1ID := uuid.New()
	epic2ID := uuid.New()
	analystID := uuid.New()

	f.addEpic(&domain.Epic{ID: epic1ID, Number: "E-1", Name: "Epic One", TeamID: teamID})
	f.addEpic(&domain.Epic{ID: epic2ID, Number: "E-2", Name: "Epic Two", TeamID: teamID})
	f.addRole(&domain.Role{ID: analystID, Name: "Аналитик"})
	f.roleScores[epic1ID] = []domain.EpicRoleScore{{EpicID: epic1ID, RoleID: analystID, WeightedAvg: 2.0}}
	f.roleScores[epic2ID] = []domain.EpicRoleScore{{EpicID: epic2ID, RoleID: analystID, WeightedAvg: 2.0}}

	start := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC) // Monday
	if _, err := svc.GenerateTasksForEpic(ctx, epic1ID, start); err != nil {
		t.Fatalf("unexpected error generating epic1: %v", err)
	}
	if _, err := svc.GenerateTasksForEpic(ctx, epic2ID, start); err != nil {
		t.Fatalf("unexpected error generating epic2: %v", err)
	}

	epic1Parent := findStoryTask(f, "E-1: Epic One")
	epic2Parent := findStoryTask(f, "E-2: Epic Two")
	analyst1 := findTaskByParentAndName(f, epic1Parent.ID, "Аналитик")
	analyst2 := findTaskByParentAndName(f, epic2Parent.ID, "Аналитик")

	// До реордера: epic1 идёт первым в очереди команды (Jul13-14),
	// epic2 — вторым (Jul15-16), т.к. делит роль "Аналитик" с epic1.
	if !analyst1.StartDate.Equal(time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("precondition failed: analyst epic1 start = %v, want Jul13", analyst1.StartDate)
	}
	if !analyst2.StartDate.Equal(time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("precondition failed: analyst epic2 start = %v, want Jul15", analyst2.StartDate)
	}

	// Меняем порядок эпиков: epic2 теперь первый в очереди.
	if _, err := svc.ReorderEpic(ctx, epic2ID, 1); err != nil {
		t.Fatalf("unexpected error reordering epic2: %v", err)
	}
	if _, err := svc.ReorderEpic(ctx, epic1ID, 2); err != nil {
		t.Fatalf("unexpected error reordering epic1: %v", err)
	}

	gotAnalyst1 := f.tasks[analyst1.ID]
	gotAnalyst2 := f.tasks[analyst2.ID]
	if !gotAnalyst2.StartDate.Equal(time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("after reorder: analyst epic2 start = %v, want Jul13 (epic2 now first)", gotAnalyst2.StartDate)
	}
	if !gotAnalyst1.StartDate.Equal(time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("after reorder: analyst epic1 start = %v, want Jul15 (epic1 now second)", gotAnalyst1.StartDate)
	}

	// Реордер эпика-стори (ParentEpicID != nil) через ReorderEpic должен отклоняться.
	storyEpicID := uuid.New()
	f.addEpic(&domain.Epic{ID: storyEpicID, Number: "E-1-S1", Name: "Story", TeamID: teamID, ParentEpicID: &epic1ID})
	if _, err := svc.ReorderEpic(ctx, storyEpicID, 1); err == nil {
		t.Errorf("expected error when calling ReorderEpic on a story")
	}
}

// TestRecalculateTeamSchedule_FactShiftsDownstream проверяет, что факт
// завершения задачи (раньше или позже плана) сдвигает последующую задачу
// той же роли в следующей сторе — планировщик считает от факта, а не от плана.
func TestRecalculateTeamSchedule_FactShiftsDownstream(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	story1ID := uuid.New()
	story2ID := uuid.New()
	analystID := uuid.New()

	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})
	story1 := f.addEpic(&domain.Epic{ID: story1ID, Number: "E-1-S1", Name: "Story 1", TeamID: teamID, ParentEpicID: &epicID})
	story2 := f.addEpic(&domain.Epic{ID: story2ID, Number: "E-1-S2", Name: "Story 2", TeamID: teamID, ParentEpicID: &epicID})
	f.addRole(&domain.Role{ID: analystID, Name: "Аналитик"})

	f.roleScores[story1ID] = []domain.EpicRoleScore{{EpicID: story1ID, RoleID: analystID, WeightedAvg: 2.0}} // plan: Jul13-Jul14
	f.roleScores[story2ID] = []domain.EpicRoleScore{{EpicID: story2ID, RoleID: analystID, WeightedAvg: 2.0}} // plan: Jul15-Jul16

	start := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC) // Monday
	if _, err := svc.GenerateTasksForEpic(ctx, epicID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	story1Task := findStoryTask(f, storyName(story1))
	story2Task := findStoryTask(f, storyName(story2))
	analystStory1 := findTaskByParentAndName(f, story1Task.ID, "Аналитик")
	analystStory2 := findTaskByParentAndName(f, story2Task.ID, "Аналитик")

	if !analystStory2.StartDate.Equal(time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("precondition failed: analyst story2 plan start = %v, want Jul15", analystStory2.StartDate)
	}

	// Факт раньше плана: story1 завершена на день раньше (Jul13 вместо Jul14).
	earlyFact := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	f.tasks[analystStory1.ID].Progress = 100
	f.tasks[analystStory1.ID].ActualEndDate = &earlyFact

	if _, err := svc.RecalculateTeamSchedule(ctx, teamID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	gotEarly := f.tasks[analystStory2.ID]
	wantEarly := time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC) // day after the early fact
	if !gotEarly.StartDate.Equal(wantEarly) {
		t.Errorf("after early fact: analyst story2 start = %v, want %v", gotEarly.StartDate, wantEarly)
	}

	// Факт позже плана: story1 на самом деле завершена на 2 дня позже (Jul16).
	lateFact := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	f.tasks[analystStory1.ID].ActualEndDate = &lateFact

	if _, err := svc.RecalculateTeamSchedule(ctx, teamID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	gotLate := f.tasks[analystStory2.ID]
	wantLate := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC) // day after the late fact
	if !gotLate.StartDate.Equal(wantLate) {
		t.Errorf("after late fact: analyst story2 start = %v, want %v", gotLate.StartDate, wantLate)
	}
}

// TestSetTaskProgress_FixesActualsAt100 проверяет, что простановка 100%
// автоматически фиксирует факт (дату и трудоёмкость), а откат ниже 100% —
// сбрасывает его (переоткрытие), и что прогресс родительской задачи
// выставить напрямую нельзя.
func TestSetTaskProgress_FixesActualsAt100(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	roleID := uuid.New()

	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})
	f.addRole(&domain.Role{ID: roleID, Name: "Аналитик"})

	epicParent, err := f.CreateGanttTask(ctx, &domain.GanttTask{
		EpicID: epicID, Name: "E-1: Epic",
		StartDate: time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
		IsParent:  true,
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	roleTask, err := f.CreateGanttTask(ctx, &domain.GanttTask{
		EpicID: epicID, RoleID: &roleID, Name: "Аналитик",
		StartDate: time.Date(2000, 1, 3, 0, 0, 0, 0, time.UTC), // far in the past, deterministic effort > 0
		EndDate:   time.Date(2000, 1, 4, 0, 0, 0, 0, time.UTC),
		SortOrder: 1, IsParent: false, ParentTaskID: &epicParent.ID,
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Нельзя выставить прогресс родительской (is_parent) задаче напрямую.
	if _, err := svc.SetTaskProgress(ctx, epicParent.ID, 50); err == nil {
		t.Errorf("expected error when setting progress on a parent task")
	}

	if _, err := svc.SetTaskProgress(ctx, roleTask.ID, 100); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := f.tasks[roleTask.ID]
	if got.Progress != 100 {
		t.Errorf("progress = %v, want 100", got.Progress)
	}
	if got.ActualEndDate == nil {
		t.Fatalf("expected ActualEndDate to be set at 100%%")
	}
	if got.ActualEffortDays == nil || *got.ActualEffortDays < 1 {
		t.Errorf("expected ActualEffortDays >= 1, got %v", got.ActualEffortDays)
	}

	// Переоткрытие: прогресс падает ниже 100% -> факт должен сброситься.
	if _, err := svc.SetTaskProgress(ctx, roleTask.ID, 60); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got = f.tasks[roleTask.ID]
	if got.Progress != 60 {
		t.Errorf("progress = %v, want 60", got.Progress)
	}
	if got.ActualEndDate != nil || got.ActualEffortDays != nil {
		t.Errorf("expected actuals to be cleared after reopening, got end=%v effort=%v",
			got.ActualEndDate, got.ActualEffortDays)
	}
}
