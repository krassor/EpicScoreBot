package gantt

import (
	"EpicScoreBot/internal/models/domain"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
)

// assignmentUserID reads the pinned user (nil = automatic) stored in
// task_assignments for a (story-or-epic, role) pair.
func assignmentUserID(f *fakeRepo, storyOrEpicID, roleID uuid.UUID) *uuid.UUID {
	return f.assignments[assignmentKey{epicID: storyOrEpicID, roleID: roleID}].UserID
}

// TestRecalculateTeamSchedule_ParallelAssigneesAcrossStories проверяет
// ключевое следствие Решения 1/2 design.md: единица занятости — исполнитель,
// а не роль. Две стори с задачей одной роли и два свободных кандидата дают
// параллельно выполняющиеся (пересекающиеся по датам) задачи у РАЗНЫХ
// людей — суммарный срок закрытия обеих стори (5 workdays) заметно меньше,
// чем при последовательном выполнении одним человеком (10 workdays).
func TestRecalculateTeamSchedule_ParallelAssigneesAcrossStories(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	story1ID := uuid.New()
	story2ID := uuid.New()
	devID := uuid.New()

	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})
	f.addEpic(&domain.Epic{ID: story1ID, Number: "E-1-S1", Name: "Story 1", TeamID: teamID, ParentEpicID: &epicID})
	f.addEpic(&domain.Epic{ID: story2ID, Number: "E-1-S2", Name: "Story 2", TeamID: teamID, ParentEpicID: &epicID})
	f.addRole(&domain.Role{ID: devID, Name: "BE разработчик"})

	f.roleScores[story1ID] = []domain.EpicRoleScore{{EpicID: story1ID, RoleID: devID, WeightedAvg: 5.0}} // 5 workdays
	f.roleScores[story2ID] = []domain.EpicRoleScore{{EpicID: story2ID, RoleID: devID, WeightedAvg: 5.0}} // 5 workdays

	dev1 := f.addUser(&domain.User{ID: uuid.New(), FirstName: "Dev", LastName: "One"}, []uuid.UUID{teamID}, []uuid.UUID{devID})
	dev2 := f.addUser(&domain.User{ID: uuid.New(), FirstName: "Dev", LastName: "Two"}, []uuid.UUID{teamID}, []uuid.UUID{devID})

	start := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC) // Monday
	if _, err := svc.GenerateTasksForEpic(ctx, epicID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	story1Task := findStoryTask(f, "E-1-S1: Story 1")
	story2Task := findStoryTask(f, "E-1-S2: Story 2")
	devStory1 := findTaskByParentAndName(f, story1Task.ID, "BE разработчик")
	devStory2 := findTaskByParentAndName(f, story2Task.ID, "BE разработчик")
	if devStory1 == nil || devStory2 == nil {
		t.Fatalf("expected both dev tasks to exist")
	}

	// Оба свободны в начале — обе задачи стартуют в один день (Jul13),
	// т.е. пересекаются по датам целиком (5 рабочих дней, Jul13-Jul17).
	wantStart := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)
	if !devStory1.StartDate.Equal(wantStart) || !devStory1.EndDate.Equal(wantEnd) {
		t.Errorf("dev story1 = [%v, %v], want [%v, %v]", devStory1.StartDate, devStory1.EndDate, wantStart, wantEnd)
	}
	if !devStory2.StartDate.Equal(wantStart) || !devStory2.EndDate.Equal(wantEnd) {
		t.Errorf("dev story2 = [%v, %v], want [%v, %v] (parallel with story1, not sequential)",
			devStory2.StartDate, devStory2.EndDate, wantStart, wantEnd)
	}

	// Разные исполнители — иначе одна и та же задача роли не могла бы
	// физически выполняться в одно и то же время дважды.
	if devStory1.AssigneeID == nil || devStory2.AssigneeID == nil {
		t.Fatalf("expected both tasks to have an assignee, got story1=%v story2=%v",
			devStory1.AssigneeID, devStory2.AssigneeID)
	}
	if *devStory1.AssigneeID == *devStory2.AssigneeID {
		t.Errorf("expected different assignees for parallel tasks, both got %v", *devStory1.AssigneeID)
	}
	assignees := map[uuid.UUID]bool{*devStory1.AssigneeID: true, *devStory2.AssigneeID: true}
	if !assignees[dev1.ID] || !assignees[dev2.ID] {
		t.Errorf("expected assignees to be exactly {dev1, dev2}, got %v", assignees)
	}
}

// TestRecalculateTeamSchedule_SingleCandidateStaysSequential — контрольный
// случай к предыдущему тесту: с ОДНИМ кандидатом на роль те же две стори
// не могут выполняться параллельно — вторая ждёт освобождения того же
// человека (design.md, сценарий "Исполнителей меньше, чем задач").
func TestRecalculateTeamSchedule_SingleCandidateStaysSequential(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	story1ID := uuid.New()
	story2ID := uuid.New()
	devID := uuid.New()

	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})
	f.addEpic(&domain.Epic{ID: story1ID, Number: "E-1-S1", Name: "Story 1", TeamID: teamID, ParentEpicID: &epicID})
	f.addEpic(&domain.Epic{ID: story2ID, Number: "E-1-S2", Name: "Story 2", TeamID: teamID, ParentEpicID: &epicID})
	f.addRole(&domain.Role{ID: devID, Name: "BE разработчик"})

	f.roleScores[story1ID] = []domain.EpicRoleScore{{EpicID: story1ID, RoleID: devID, WeightedAvg: 5.0}}
	f.roleScores[story2ID] = []domain.EpicRoleScore{{EpicID: story2ID, RoleID: devID, WeightedAvg: 5.0}}

	solo := f.addUser(&domain.User{ID: uuid.New(), FirstName: "Solo", LastName: "Dev"}, []uuid.UUID{teamID}, []uuid.UUID{devID})

	start := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC) // Monday
	if _, err := svc.GenerateTasksForEpic(ctx, epicID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	story1Task := findStoryTask(f, "E-1-S1: Story 1")
	story2Task := findStoryTask(f, "E-1-S2: Story 2")
	devStory1 := findTaskByParentAndName(f, story1Task.ID, "BE разработчик")
	devStory2 := findTaskByParentAndName(f, story2Task.ID, "BE разработчик")

	if devStory1.AssigneeID == nil || *devStory1.AssigneeID != solo.ID {
		t.Fatalf("expected story1 assigned to the only candidate, got %v", devStory1.AssigneeID)
	}
	if devStory2.AssigneeID == nil || *devStory2.AssigneeID != solo.ID {
		t.Fatalf("expected story2 assigned to the only candidate, got %v", devStory2.AssigneeID)
	}
	// Не пересекаются: story2 начинается только на следующий рабочий день
	// после конца story1 (Jul17 пятница -> Jul20 понедельник).
	wantStory2Start := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	if !devStory2.StartDate.Equal(wantStory2Start) {
		t.Errorf("dev story2 start = %v, want %v (sequential, one candidate)", devStory2.StartDate, wantStory2Start)
	}
	if !devStory1.EndDate.Before(devStory2.StartDate) {
		t.Errorf("expected story1 (%v) to fully finish before story2 starts (%v)",
			devStory1.EndDate, devStory2.StartDate)
	}
}

// TestRecalculateTeamSchedule_EmptyPoolSequentialByRole проверяет, что роль
// без кандидатов в команде по-прежнему планируется как единый ресурс
// (design.md "Роль без кандидатов планируется как единый ресурс") —
// задачи остаются без исполнителя, но не накладываются друг на друга.
func TestRecalculateTeamSchedule_EmptyPoolSequentialByRole(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	story1ID := uuid.New()
	story2ID := uuid.New()
	mobileID := uuid.New()

	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})
	f.addEpic(&domain.Epic{ID: story1ID, Number: "E-1-S1", Name: "Story 1", TeamID: teamID, ParentEpicID: &epicID})
	f.addEpic(&domain.Epic{ID: story2ID, Number: "E-1-S2", Name: "Story 2", TeamID: teamID, ParentEpicID: &epicID})
	f.addRole(&domain.Role{ID: mobileID, Name: "Mobile разработчик"})
	// Нет ни одного пользователя с ролью Mobile разработчик в команде.

	f.roleScores[story1ID] = []domain.EpicRoleScore{{EpicID: story1ID, RoleID: mobileID, WeightedAvg: 2.0}}
	f.roleScores[story2ID] = []domain.EpicRoleScore{{EpicID: story2ID, RoleID: mobileID, WeightedAvg: 2.0}}

	start := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC) // Monday
	if _, err := svc.GenerateTasksForEpic(ctx, epicID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	story1Task := findStoryTask(f, "E-1-S1: Story 1")
	story2Task := findStoryTask(f, "E-1-S2: Story 2")
	mobile1 := findTaskByParentAndName(f, story1Task.ID, "Mobile разработчик")
	mobile2 := findTaskByParentAndName(f, story2Task.ID, "Mobile разработчик")
	if mobile1 == nil || mobile2 == nil {
		t.Fatalf("expected both role tasks to exist even without candidates")
	}

	if mobile1.AssigneeID != nil || mobile2.AssigneeID != nil {
		t.Errorf("expected no assignee for a role with an empty pool, got story1=%v story2=%v",
			mobile1.AssigneeID, mobile2.AssigneeID)
	}
	// Единый ресурс роли: story2 не может начаться раньше следующего
	// рабочего дня после конца story1.
	if !mobile1.EndDate.Before(mobile2.StartDate) {
		t.Errorf("expected role tasks to stay sequential when pool is empty: story1 end %v, story2 start %v",
			mobile1.EndDate, mobile2.StartDate)
	}
}

// TestRecalculateTeamSchedule_FrozenTaskKeepsAssigneeAcrossRosterChange
// проверяет design.md Решение 6: замороженная (начатая) задача сохраняет
// исполнителя, назначенного ранее, даже если состав команды меняется
// (в команду добавляется новый кандидат той же роли) — переназначения не
// происходит.
func TestRecalculateTeamSchedule_FrozenTaskKeepsAssigneeAcrossRosterChange(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	roleID := uuid.New()

	f.addRole(&domain.Role{ID: roleID, Name: "Аналитик"})
	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})

	userA := f.addUser(&domain.User{ID: uuid.New(), FirstName: "A"}, []uuid.UUID{teamID}, []uuid.UUID{roleID})

	epicParent, err := f.CreateGanttTask(ctx, &domain.GanttTask{
		EpicID: epicID, Name: "E-1: Epic",
		StartDate: time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
		IsParent:  true,
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	frozenStart := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	frozenEnd := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	frozenTask, err := f.CreateGanttTask(ctx, &domain.GanttTask{
		EpicID: epicID, RoleID: &roleID, Name: "Аналитик",
		StartDate: frozenStart, EndDate: frozenEnd, Progress: 40,
		SortOrder: 1, IsParent: false, ParentTaskID: &epicParent.ID,
		AssigneeID: &userA.ID,
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	if _, err := svc.RecalculateTeamSchedule(ctx, teamID); err != nil {
		t.Fatalf("unexpected error (before roster change): %v", err)
	}
	got := f.tasks[frozenTask.ID]
	if got.AssigneeID == nil || *got.AssigneeID != userA.ID {
		t.Fatalf("frozen task assignee before roster change = %v, want %v", got.AssigneeID, userA.ID)
	}

	// Состав команды меняется: добавляется второй кандидат той же роли.
	f.addUser(&domain.User{ID: uuid.New(), FirstName: "B"}, []uuid.UUID{teamID}, []uuid.UUID{roleID})

	if _, err := svc.RecalculateTeamSchedule(ctx, teamID); err != nil {
		t.Fatalf("unexpected error (after roster change): %v", err)
	}
	got = f.tasks[frozenTask.ID]
	if got.AssigneeID == nil || *got.AssigneeID != userA.ID {
		t.Errorf("frozen task assignee after roster change = %v, want unchanged %v", got.AssigneeID, userA.ID)
	}
	if !got.StartDate.Equal(frozenStart) || !got.EndDate.Equal(frozenEnd) || got.Progress != 40 {
		t.Errorf("frozen task dates/progress changed: got [%v, %v, %v]", got.StartDate, got.EndDate, got.Progress)
	}
}

// TestRecalculateTeamSchedule_FrozenTaskBlocksSameAssigneeNextTask
// проверяет вторую часть design.md Решение 6: занятость, создаваемая
// замороженной задачей, учитывается в календаре ИМЕННО её исполнителя —
// следующая автоматически назначенная задача той же роли тому же
// (единственному) кандидату не может начаться раньше следующего рабочего
// дня после эффективного окончания замороженной задачи.
func TestRecalculateTeamSchedule_FrozenTaskBlocksSameAssigneeNextTask(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	story1ID := uuid.New()
	story2ID := uuid.New()
	roleID := uuid.New()

	f.addRole(&domain.Role{ID: roleID, Name: "Аналитик"})
	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})
	f.addEpic(&domain.Epic{ID: story1ID, Number: "E-1-S1", Name: "Story 1", TeamID: teamID, ParentEpicID: &epicID})
	story2 := f.addEpic(&domain.Epic{ID: story2ID, Number: "E-1-S2", Name: "Story 2", TeamID: teamID, ParentEpicID: &epicID})

	// Единственный кандидат роли в команде.
	solo := f.addUser(&domain.User{ID: uuid.New(), FirstName: "Solo"}, []uuid.UUID{teamID}, []uuid.UUID{roleID})

	epicParent, err := f.CreateGanttTask(ctx, &domain.GanttTask{
		EpicID: epicID, Name: "E-1: Epic",
		StartDate: time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
		IsParent:  true,
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	story1Task, err := f.CreateGanttTask(ctx, &domain.GanttTask{
		EpicID: epicID, Name: "E-1-S1: Story 1",
		StartDate: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC),
		IsParent:  true, ParentTaskID: &epicParent.ID, SortOrder: 1,
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	story2Task, err := f.CreateGanttTask(ctx, &domain.GanttTask{
		EpicID: epicID, Name: "E-1-S2: Story 2",
		StartDate: time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
		IsParent:  true, ParentTaskID: &epicParent.ID, SortOrder: 2,
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Замороженная задача story1, уже назначена solo, заканчивается Jul10 (пятница).
	frozenStart := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	frozenEnd := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	if _, err := f.CreateGanttTask(ctx, &domain.GanttTask{
		EpicID: epicID, RoleID: &roleID, Name: "Аналитик",
		StartDate: frozenStart, EndDate: frozenEnd, Progress: 100,
		ActualEndDate: &frozenEnd,
		SortOrder:     1, IsParent: false, ParentTaskID: &story1Task.ID,
		AssigneeID: &solo.ID,
	}); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Незамороженная задача story2 той же роли — единственный кандидат тот же.
	story2RoleTask, err := f.CreateGanttTask(ctx, &domain.GanttTask{
		EpicID: epicID, RoleID: &roleID, Name: "Аналитик",
		StartDate: time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC),
		SortOrder: 1, IsParent: false, ParentTaskID: &story2Task.ID,
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	_ = story2

	if _, err := svc.RecalculateTeamSchedule(ctx, teamID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := f.tasks[story2RoleTask.ID]
	if got.AssigneeID == nil || *got.AssigneeID != solo.ID {
		t.Fatalf("expected story2 task assigned to the only candidate (%v), got %v", solo.ID, got.AssigneeID)
	}
	// Jul10 (пятница) + 1 рабочий день -> Jul13 (понедельник). epicFloor тоже
	// Jul13, так что оба ограничения совпадают, но принципиально важно, что
	// расчёт идёт от занятости исполнителя, а не «с нуля».
	wantStart := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	if !got.StartDate.Equal(wantStart) {
		t.Errorf("story2 task start = %v, want %v", got.StartDate, wantStart)
	}
}

// TestRecalculateTeamSchedule_DeterministicAcrossRepeatedCalls закрывает
// design.md Risks "Недетерминированный порядок итерации map в Go даёт
// «мигающих» исполнителей": несколько последовательных пересчётов без
// изменения входных данных (состав команды, оценки, закрепления) должны
// дать ТО ЖЕ САМОЕ распределение исполнителей каждый раз — round-robin
// тай-брейк и сортировка пула по uuid обязаны это гарантировать.
func TestRecalculateTeamSchedule_DeterministicAcrossRepeatedCalls(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	devID := uuid.New()
	f.addRole(&domain.Role{ID: devID, Name: "BE разработчик"})
	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})

	var storyIDs []uuid.UUID
	for i := 1; i <= 4; i++ {
		storyID := uuid.New()
		number := fmt.Sprintf("E-1-S%d", i)
		f.addEpic(&domain.Epic{ID: storyID, Number: number, Name: fmt.Sprintf("Story %d", i), TeamID: teamID, ParentEpicID: &epicID})
		f.roleScores[storyID] = []domain.EpicRoleScore{{EpicID: storyID, RoleID: devID, WeightedAvg: 2.0}}
		storyIDs = append(storyIDs, storyID)
	}

	for i := 0; i < 3; i++ {
		f.addUser(&domain.User{ID: uuid.New(), FirstName: fmt.Sprintf("Dev%d", i)}, []uuid.UUID{teamID}, []uuid.UUID{devID})
	}

	start := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC) // Monday
	if _, err := svc.GenerateTasksForEpic(ctx, epicID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	snapshot := func() map[uuid.UUID]uuid.UUID {
		result := make(map[uuid.UUID]uuid.UUID)
		for id, task := range f.tasks {
			if !task.IsParent && task.AssigneeID != nil {
				result[id] = *task.AssigneeID
			}
		}
		return result
	}

	first := snapshot()
	if len(first) != len(storyIDs) {
		t.Fatalf("expected %d assigned leaf tasks, got %d", len(storyIDs), len(first))
	}

	for attempt := 0; attempt < 5; attempt++ {
		if _, err := svc.RecalculateTeamSchedule(ctx, teamID); err != nil {
			t.Fatalf("attempt %d: unexpected error: %v", attempt, err)
		}
		got := snapshot()
		if len(got) != len(first) {
			t.Fatalf("attempt %d: assignee count changed: got %d, want %d", attempt, len(got), len(first))
		}
		for taskID, assignee := range first {
			if got[taskID] != assignee {
				t.Errorf("attempt %d: task %s assignee changed: got %v, want %v (must stay stable across recalcs)",
					attempt, taskID, got[taskID], assignee)
			}
		}
	}
}

// TestSetTaskAssignee_PriorityOverAutomaticChoice проверяет design.md
// Решение 3: закрепление меняет ИСПОЛНИТЕЛЯ по сравнению с тем, кого выбрал
// бы автомат, и это отражается после пересчёта.
func TestSetTaskAssignee_PriorityOverAutomaticChoice(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	roleID := uuid.New()

	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic One", TeamID: teamID})
	f.addRole(&domain.Role{ID: roleID, Name: "Аналитик"})
	f.roleScores[epicID] = []domain.EpicRoleScore{{EpicID: epicID, RoleID: roleID, WeightedAvg: 2.0}}

	userA := f.addUser(&domain.User{ID: uuid.New(), FirstName: "A"}, []uuid.UUID{teamID}, []uuid.UUID{roleID})
	userB := f.addUser(&domain.User{ID: uuid.New(), FirstName: "B"}, []uuid.UUID{teamID}, []uuid.UUID{roleID})

	start := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	if _, err := svc.GenerateTasksForEpic(ctx, epicID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parentID uuid.UUID
	for _, task := range f.tasks {
		if task.IsParent {
			parentID = task.ID
		}
	}
	task := findTaskByParentAndName(f, parentID, "Аналитик")
	if task == nil {
		t.Fatalf("expected role task to exist")
	}

	auto := *f.tasks[task.ID].AssigneeID
	var other *domain.User
	if auto == userA.ID {
		other = userB
	} else {
		other = userA
	}

	if _, err := svc.SetTaskAssignee(ctx, task.ID, &other.ID); err != nil {
		t.Fatalf("SetTaskAssignee: %v", err)
	}

	got := f.tasks[task.ID]
	if got.AssigneeID == nil || *got.AssigneeID != other.ID {
		t.Errorf("assignee after pin = %v, want %v (pin overrides automatic choice)", got.AssigneeID, other.ID)
	}
	if uid := assignmentUserID(f, epicID, roleID); uid == nil || *uid != other.ID {
		t.Errorf("task_assignments.user_id = %v, want %v", uid, other.ID)
	}
}

// TestSetTaskAssignee_InvalidPinFallsBackAndRecovers проверяет design.md
// Решение 7: закрепление на человека, лишившегося роли, не удаляется, но
// перестаёт действовать (задача уходит в автомат); когда роль возвращается,
// закрепление снова вступает в силу.
func TestSetTaskAssignee_InvalidPinFallsBackAndRecovers(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	roleID := uuid.New()

	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic One", TeamID: teamID})
	f.addRole(&domain.Role{ID: roleID, Name: "Аналитик"})
	f.roleScores[epicID] = []domain.EpicRoleScore{{EpicID: epicID, RoleID: roleID, WeightedAvg: 2.0}}

	pinned := f.addUser(&domain.User{ID: uuid.New(), FirstName: "Pinned"}, []uuid.UUID{teamID}, []uuid.UUID{roleID})
	fallback := f.addUser(&domain.User{ID: uuid.New(), FirstName: "Fallback"}, []uuid.UUID{teamID}, []uuid.UUID{roleID})

	start := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	if _, err := svc.GenerateTasksForEpic(ctx, epicID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parentID uuid.UUID
	for _, task := range f.tasks {
		if task.IsParent {
			parentID = task.ID
		}
	}
	task := findTaskByParentAndName(f, parentID, "Аналитик")

	if _, err := svc.SetTaskAssignee(ctx, task.ID, &pinned.ID); err != nil {
		t.Fatalf("SetTaskAssignee (pin): %v", err)
	}
	if got := f.tasks[task.ID]; got.AssigneeID == nil || *got.AssigneeID != pinned.ID {
		t.Fatalf("expected task pinned to %v, got %v", pinned.ID, got.AssigneeID)
	}
	_ = fallback

	// Закреплённый теряет роль (например, администратор снял её).
	delete(f.userRoles[pinned.ID], roleID)

	if _, err := svc.RecalculateTeamSchedule(ctx, teamID); err != nil {
		t.Fatalf("unexpected error after role removal: %v", err)
	}
	got := f.tasks[task.ID]
	if got.AssigneeID == nil {
		t.Fatalf("expected automatic fallback assignee, got nil")
	}
	if *got.AssigneeID == pinned.ID {
		t.Errorf("expected task NOT to stay with a candidate who lost the role, got %v", *got.AssigneeID)
	}
	if *got.AssigneeID != fallback.ID {
		t.Errorf("expected fallback assignee %v (only remaining candidate), got %v", fallback.ID, *got.AssigneeID)
	}
	// Закрепление не удалено — строка сохраняется с прежним user_id.
	if uid := assignmentUserID(f, epicID, roleID); uid == nil || *uid != pinned.ID {
		t.Errorf("task_assignments.user_id should be preserved as %v (inactive, not deleted), got %v", pinned.ID, uid)
	}

	// Роль возвращается — закрепление снова вступает в силу.
	f.userRoles[pinned.ID][roleID] = true

	if _, err := svc.RecalculateTeamSchedule(ctx, teamID); err != nil {
		t.Fatalf("unexpected error after role restored: %v", err)
	}
	got = f.tasks[task.ID]
	if got.AssigneeID == nil || *got.AssigneeID != pinned.ID {
		t.Errorf("expected pin to take effect again once role is restored, got %v, want %v",
			got.AssigneeID, pinned.ID)
	}
}

// TestGenerateTasksForQuarter_PreservesAssignmentsAcrossRegeneration
// проверяет design.md Решение 4: закрепление исполнителя и ручное смещение
// старта переживают перегенерацию задач квартала, которая удаляет и
// пересоздаёт все строки gantt_tasks.
func TestGenerateTasksForQuarter_PreservesAssignmentsAcrossRegeneration(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	roleID := uuid.New()
	f.addRole(&domain.Role{ID: roleID, Name: "Аналитик"})

	epic := addScoredEpicWithRole(f, teamID, roleID, "E-1", 2026, 3, 1)
	pinned := f.addUser(&domain.User{ID: uuid.New(), FirstName: "Pinned"}, []uuid.UUID{teamID}, []uuid.UUID{roleID})

	start := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC) // Monday
	if _, err := svc.GenerateTasksForQuarter(ctx, teamID, 2026, 3, start); err != nil {
		t.Fatalf("initial generation: unexpected error: %v", err)
	}

	var oldTaskID uuid.UUID
	for id, task := range f.tasks {
		if !task.IsParent && task.EpicID == epic.ID {
			oldTaskID = id
		}
	}
	if oldTaskID == uuid.Nil {
		t.Fatalf("setup: expected a role task to exist")
	}

	if _, err := svc.SetTaskAssignee(ctx, oldTaskID, &pinned.ID); err != nil {
		t.Fatalf("SetTaskAssignee: %v", err)
	}
	if _, err := svc.SetTaskStartOffset(ctx, oldTaskID, 4); err != nil {
		t.Fatalf("SetTaskStartOffset: %v", err)
	}

	// Массовая перегенерация квартала: DeleteGanttTasksByEpicID удаляет
	// старые строки, включая oldTaskID.
	if _, err := svc.GenerateTasksForQuarter(ctx, teamID, 2026, 3, start); err != nil {
		t.Fatalf("regeneration: unexpected error: %v", err)
	}
	if _, stillExists := f.tasks[oldTaskID]; stillExists {
		t.Fatalf("setup invariant broken: expected old task row to be deleted by regeneration")
	}

	var newTaskID uuid.UUID
	for id, task := range f.tasks {
		if !task.IsParent && task.EpicID == epic.ID {
			newTaskID = id
		}
	}
	if newTaskID == uuid.Nil {
		t.Fatalf("expected a regenerated role task to exist")
	}

	newTask := f.tasks[newTaskID]
	if newTask.AssigneeID == nil || *newTask.AssigneeID != pinned.ID {
		t.Errorf("regenerated task assignee = %v, want %v (pin should survive regeneration)",
			newTask.AssigneeID, pinned.ID)
	}
	if offset := assignmentStartOffset(f, epic.ID, roleID); offset != 4 {
		t.Errorf("task_assignments start offset after regeneration = %d, want 4 (survives regeneration)", offset)
	}
}

// TestGetTeamMembers_ReturnsAllRolesPerMember проверяет design.md Решение 9:
// участник с несколькими ролями возвращается со всеми ними (в отличие от
// GetRoleByUserID, дающего одну роль на пользователя).
func TestGetTeamMembers_ReturnsAllRolesPerMember(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	otherTeamID := uuid.New()
	analystID := uuid.New()
	devID := uuid.New()
	f.addRole(&domain.Role{ID: analystID, Name: "Аналитик"})
	f.addRole(&domain.Role{ID: devID, Name: "BE разработчик"})

	multiRole := f.addUser(&domain.User{ID: uuid.New(), FirstName: "Multi", LastName: "Role"},
		[]uuid.UUID{teamID}, []uuid.UUID{analystID, devID})
	singleRole := f.addUser(&domain.User{ID: uuid.New(), FirstName: "Single", LastName: "Role"},
		[]uuid.UUID{teamID}, []uuid.UUID{devID})
	// Участник другой команды не должен попасть в выборку.
	f.addUser(&domain.User{ID: uuid.New(), FirstName: "Other", LastName: "Team"},
		[]uuid.UUID{otherTeamID}, []uuid.UUID{devID})

	members, err := svc.GetTeamMembers(ctx, teamID)
	if err != nil {
		t.Fatalf("GetTeamMembers: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d: %+v", len(members), members)
	}

	byID := make(map[uuid.UUID]domain.TeamMember, len(members))
	for _, m := range members {
		byID[m.ID] = m
	}

	multi, ok := byID[multiRole.ID]
	if !ok {
		t.Fatalf("expected multi-role member in result")
	}
	if len(multi.RoleIDs) != 2 {
		t.Errorf("expected multi-role member to have 2 roles, got %v", multi.RoleIDs)
	}
	roleSet := map[uuid.UUID]bool{}
	for _, r := range multi.RoleIDs {
		roleSet[r] = true
	}
	if !roleSet[analystID] || !roleSet[devID] {
		t.Errorf("expected both roles present, got %v", multi.RoleIDs)
	}

	single, ok := byID[singleRole.ID]
	if !ok {
		t.Fatalf("expected single-role member in result")
	}
	if len(single.RoleIDs) != 1 || single.RoleIDs[0] != devID {
		t.Errorf("expected single-role member to have exactly [devID], got %v", single.RoleIDs)
	}
}

// TestGetTeamTasksWithAssignments_ResolvesByLeafTaskID проверяет единый
// путь чтения, спроектированный для HTTP-слоя (backend §3.1/§3.5): и
// закрепление, и смещение старта резолвятся по правильному ключу
// «стори (или эпик без сторей) + роль» и возвращаются с ключом по ID
// самой листовой (ролевой) строки Ганта, а не «размазаны» по паре
// параллельных вызовов. Родительские (стори/эпик) строки в карте
// отсутствуют — у них нет ни роли, ни исполнителя.
func TestGetTeamTasksWithAssignments_ResolvesByLeafTaskID(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()

	// Эпик со стори.
	epicID := uuid.New()
	storyID := uuid.New()
	devID := uuid.New()
	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic One", TeamID: teamID})
	f.addEpic(&domain.Epic{ID: storyID, Number: "E-1-S1", Name: "Story 1", TeamID: teamID, ParentEpicID: &epicID})
	f.addRole(&domain.Role{ID: devID, Name: "BE разработчик"})
	f.roleScores[storyID] = []domain.EpicRoleScore{{EpicID: storyID, RoleID: devID, WeightedAvg: 2.0}}
	pinned := f.addUser(&domain.User{ID: uuid.New(), FirstName: "Pinned"}, []uuid.UUID{teamID}, []uuid.UUID{devID})

	// Legacy-эпик без сторей той же команды.
	legacyEpicID := uuid.New()
	analystID := uuid.New()
	f.addEpic(&domain.Epic{ID: legacyEpicID, Number: "E-2", Name: "Epic Two", TeamID: teamID})
	f.addRole(&domain.Role{ID: analystID, Name: "Аналитик"})
	f.roleScores[legacyEpicID] = []domain.EpicRoleScore{{EpicID: legacyEpicID, RoleID: analystID, WeightedAvg: 2.0}}

	start := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	if _, err := svc.GenerateTasksForEpic(ctx, epicID, start); err != nil {
		t.Fatalf("unexpected error (story epic): %v", err)
	}
	if _, err := svc.GenerateTasksForEpic(ctx, legacyEpicID, start); err != nil {
		t.Fatalf("unexpected error (legacy epic): %v", err)
	}

	storyTask := findStoryTask(f, "E-1-S1: Story 1")
	devTask := findTaskByParentAndName(f, storyTask.ID, "BE разработчик")
	if devTask == nil {
		t.Fatalf("expected story dev task to exist")
	}
	if _, err := svc.SetTaskAssignee(ctx, devTask.ID, &pinned.ID); err != nil {
		t.Fatalf("SetTaskAssignee: %v", err)
	}

	var legacyParentID uuid.UUID
	for _, task := range f.tasks {
		if task.IsParent && task.EpicID == legacyEpicID {
			legacyParentID = task.ID
		}
	}
	analystTask := findTaskByParentAndName(f, legacyParentID, "Аналитик")
	if analystTask == nil {
		t.Fatalf("expected legacy analyst task to exist")
	}
	if _, err := svc.SetTaskStartOffset(ctx, analystTask.ID, 5); err != nil {
		t.Fatalf("SetTaskStartOffset: %v", err)
	}

	tasks, assignments, err := svc.GetTeamTasksWithAssignments(ctx, teamID)
	if err != nil {
		t.Fatalf("GetTeamTasksWithAssignments: %v", err)
	}
	if len(tasks) == 0 {
		t.Fatalf("expected non-empty task list")
	}

	devAssignment, ok := assignments[devTask.ID]
	if !ok {
		t.Fatalf("expected an assignment entry for the pinned story task")
	}
	if devAssignment.UserID == nil || *devAssignment.UserID != pinned.ID {
		t.Errorf("story task assignment.UserID = %v, want %v", devAssignment.UserID, pinned.ID)
	}

	analystAssignment, ok := assignments[analystTask.ID]
	if !ok {
		t.Fatalf("expected an assignment entry for the offset legacy task")
	}
	if analystAssignment.UserID != nil {
		t.Errorf("legacy task assignment.UserID = %v, want nil (only offset was set)", analystAssignment.UserID)
	}
	if analystAssignment.StartOffsetDays != 5 {
		t.Errorf("legacy task assignment.StartOffsetDays = %d, want 5", analystAssignment.StartOffsetDays)
	}

	// Родительские строки (эпики/стори) не должны попасть в карту вовсе.
	for _, task := range tasks {
		if task.IsParent {
			if _, ok := assignments[task.ID]; ok {
				t.Errorf("parent task %s unexpectedly present in assignments map", task.ID)
			}
		}
	}
}

// TestRecalculateTeamSchedule_RoundRobinWhenEqualDates явно проверяет
// design.md Решение 2 и spec.md сценарий "Равные по дате старта кандидаты
// получают задачи по кругу": когда несколько кандидатов одной роли свободны
// и несколько задач могут начаться в один день, задачи распределяются
// между кандидатами по кругу, а не достаются одному.
func TestRecalculateTeamSchedule_RoundRobinWhenEqualDates(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	devID := uuid.New()
	f.addRole(&domain.Role{ID: devID, Name: "BE разработчик"})
	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})

	// 3 стори, каждая с задачей одной роли.
	story1ID := uuid.New()
	story2ID := uuid.New()
	story3ID := uuid.New()
	f.addEpic(&domain.Epic{ID: story1ID, Number: "E-1-S1", Name: "Story 1", TeamID: teamID, ParentEpicID: &epicID})
	f.addEpic(&domain.Epic{ID: story2ID, Number: "E-1-S2", Name: "Story 2", TeamID: teamID, ParentEpicID: &epicID})
	f.addEpic(&domain.Epic{ID: story3ID, Number: "E-1-S3", Name: "Story 3", TeamID: teamID, ParentEpicID: &epicID})
	f.roleScores[story1ID] = []domain.EpicRoleScore{{EpicID: story1ID, RoleID: devID, WeightedAvg: 2.0}}
	f.roleScores[story2ID] = []domain.EpicRoleScore{{EpicID: story2ID, RoleID: devID, WeightedAvg: 2.0}}
	f.roleScores[story3ID] = []domain.EpicRoleScore{{EpicID: story3ID, RoleID: devID, WeightedAvg: 2.0}}

	// 3 кандидата, все изначально свободны.
	dev0 := f.addUser(&domain.User{ID: uuid.New(), FirstName: "Dev0"}, []uuid.UUID{teamID}, []uuid.UUID{devID})
	dev1 := f.addUser(&domain.User{ID: uuid.New(), FirstName: "Dev1"}, []uuid.UUID{teamID}, []uuid.UUID{devID})
	dev2 := f.addUser(&domain.User{ID: uuid.New(), FirstName: "Dev2"}, []uuid.UUID{teamID}, []uuid.UUID{devID})

	start := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC) // Monday
	if _, err := svc.GenerateTasksForEpic(ctx, epicID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Каждая задача должна быть назначена следующему кандидату по кругу.
	story1Task := findStoryTask(f, "E-1-S1: Story 1")
	story2Task := findStoryTask(f, "E-1-S2: Story 2")
	story3Task := findStoryTask(f, "E-1-S3: Story 3")

	task1 := findTaskByParentAndName(f, story1Task.ID, "BE разработчик")
	task2 := findTaskByParentAndName(f, story2Task.ID, "BE разработчик")
	task3 := findTaskByParentAndName(f, story3Task.ID, "BE разработчик")

	if task1 == nil || task2 == nil || task3 == nil {
		t.Fatalf("expected all three tasks to exist")
	}

	assignees := []uuid.UUID{
		*task1.AssigneeID, *task2.AssigneeID, *task3.AssigneeID,
	}
	candidates := []uuid.UUID{dev0.ID, dev1.ID, dev2.ID}

	// Проверяем, что все три кандидата назначены (по кругу), а не все одному.
	candidateSet := make(map[uuid.UUID]bool)
	for _, a := range assignees {
		candidateSet[a] = true
	}
	if len(candidateSet) != 3 {
		t.Errorf("expected 3 different assignees (round-robin), got %d unique: %v",
			len(candidateSet), candidateSet)
	}

	// Проверяем, что это наши кандидаты.
	for _, a := range assignees {
		found := false
		for _, c := range candidates {
			if a == c {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("assignee %v is not one of our candidates", a)
		}
	}

	// Все три задачи начинаются в один день (одинаковая дата старта).
	if !task1.StartDate.Equal(task2.StartDate) || !task2.StartDate.Equal(task3.StartDate) {
		t.Errorf("expected all three tasks to have the same start date (round-robin precondition), got %v, %v, %v",
			task1.StartDate, task2.StartDate, task3.StartDate)
	}
}
