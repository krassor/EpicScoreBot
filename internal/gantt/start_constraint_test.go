package gantt

import (
	"EpicScoreBot/internal/models/domain"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Тесты ограничения "Начать не ранее" (openspec/changes/
// add-task-start-constraints, задачи 2.1—2.7). Обе новые нижние границы
// (дата, ссылка на другую задачу) добавляются к уже существующим правилам
// (предусловия стори, смещение старта, граница команды) — тесты проверяют
// каждую по отдельности, вместе и во взаимодействии с уже существующей
// логикой (заполнение простоев, заморозка, повторный проход).

// TestRecalculateTeamSchedule_StartConstraint_DateLaterThanComputed_PushesStart
// проверяет задачу 2.1, Scenario "Дата позже, чем получилось бы без неё":
// указанная дата, наступающая позже расчётного старта, отодвигает задачу.
func TestRecalculateTeamSchedule_StartConstraint_DateLaterThanComputed_PushesStart(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	feID := uuid.New()

	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})
	f.addRole(&domain.Role{ID: feID, Name: "FE разработчик"})
	f.roleScores[epicID] = []domain.EpicRoleScore{
		{EpicID: epicID, RoleID: feID, WeightedAvg: 2.0},
	}

	start := date(2026, 7, 13) // Monday
	tasks, err := svc.GenerateTasksForEpic(ctx, epicID, start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parent := tasks[0]
	fe := findTaskByParentAndName(f, parent.ID, "FE разработчик")
	if fe == nil {
		t.Fatalf("expected FE task to exist")
	}
	if !fe.StartDate.Equal(date(2026, 7, 13)) {
		t.Fatalf("precondition failed: FE start = %v, want Jul13", fe.StartDate)
	}

	notBefore := date(2026, 7, 20) // Monday, later than the computed start
	if _, err := svc.SetTaskStartConstraint(ctx, fe.ID, &notBefore, nil, nil); err != nil {
		t.Fatalf("SetTaskStartConstraint: %v", err)
	}

	feGot := f.tasks[fe.ID]
	if !feGot.StartDate.Equal(notBefore) {
		t.Errorf("FE start = %v, want %v (pushed to the not-before date)", feGot.StartDate, notBefore)
	}
	wantEnd := date(2026, 7, 21)
	if !feGot.EndDate.Equal(wantEnd) {
		t.Errorf("FE end = %v, want %v (full duration kept, not shortened)", feGot.EndDate, wantEnd)
	}
}

// TestRecalculateTeamSchedule_StartConstraint_DateEarlierThanComputed_DoesNotPullBack
// проверяет задачу 2.1, Scenario "Дата раньше расчётного старта": дата
// ограничивает СНИЗУ и не притягивает задачу к себе, если она и без
// ограничения стартовала бы позже.
func TestRecalculateTeamSchedule_StartConstraint_DateEarlierThanComputed_DoesNotPullBack(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	storyID := uuid.New()
	analystID := uuid.New()
	feID := uuid.New()

	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})
	story := f.addEpic(&domain.Epic{ID: storyID, Number: "E-1-S1", Name: "Story 1", TeamID: teamID, ParentEpicID: &epicID})
	f.addRole(&domain.Role{ID: analystID, Name: "Аналитик"})
	f.addRole(&domain.Role{ID: feID, Name: "FE разработчик"})
	f.roleScores[storyID] = []domain.EpicRoleScore{
		{EpicID: storyID, RoleID: analystID, WeightedAvg: 5.0},
		{EpicID: storyID, RoleID: feID, WeightedAvg: 1.0},
	}

	start := date(2026, 7, 13) // Monday
	if _, err := svc.GenerateTasksForEpic(ctx, epicID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	storyTask := findStoryTask(f, storyName(story))
	fe := findTaskByParentAndName(f, storyTask.ID, "FE разработчик")
	if fe == nil {
		t.Fatalf("expected FE task to exist")
	}
	baselineStart := fe.StartDate // after the analyst's 5-day group

	// Дата раньше квартала — заведомо раньше расчётного старта FE.
	notBefore := date(2026, 7, 6)
	if _, err := svc.SetTaskStartConstraint(ctx, fe.ID, &notBefore, nil, nil); err != nil {
		t.Fatalf("SetTaskStartConstraint: %v", err)
	}

	feGot := f.tasks[fe.ID]
	if !feGot.StartDate.Equal(baselineStart) {
		t.Errorf("FE start = %v, want %v (unchanged — date must not pull the task back)", feGot.StartDate, baselineStart)
	}
}

// TestSetTaskStartConstraint_ClearingDateReturnsToComputedStart проверяет
// задачу 2.1, Scenario "Снятие ограничения": очистка поля возвращает задачу
// к расчётному старту.
func TestSetTaskStartConstraint_ClearingDateReturnsToComputedStart(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	anchorEpicID := uuid.New()
	epicID := uuid.New()
	analystID := uuid.New()
	feID := uuid.New()

	// anchorEpicID — вспомогательный эпик, единственная роль в котором
	// ничем не ограничена и остаётся на Jul13 весь тест: он анкерует
	// teamFloor команды на Jul13 независимо от того, куда временно уедет
	// epicID из-за проверяемого ограничения. Без анкера teamFloor команды
	// с ЕДИНСТВЕННЫМ эпиком считается по его же собственной (уже
	// сдвинутой ограничением) дате родителя — самоссылающееся поведение,
	// существовавшее до этой заявки (design.md, "teamFloor... самокоррек-
	// тируется" относится к переупорядочиванию эпиков, а не к отмене
	// пользовательского ввода на эпике-одиночке) и не входящее в объём
	// этой заявки.
	f.addEpic(&domain.Epic{ID: anchorEpicID, Number: "E-0", Name: "Anchor", TeamID: teamID})
	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})
	f.addRole(&domain.Role{ID: analystID, Name: "Аналитик"})
	f.addRole(&domain.Role{ID: feID, Name: "FE разработчик"})
	f.roleScores[anchorEpicID] = []domain.EpicRoleScore{{EpicID: anchorEpicID, RoleID: analystID, WeightedAvg: 1.0}}
	f.roleScores[epicID] = []domain.EpicRoleScore{{EpicID: epicID, RoleID: feID, WeightedAvg: 2.0}}

	start := date(2026, 7, 13)
	if _, err := svc.GenerateTasksForEpic(ctx, anchorEpicID, start); err != nil {
		t.Fatalf("unexpected error (anchor): %v", err)
	}
	if _, err := svc.GenerateTasksForEpic(ctx, epicID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var epicParentTask, fe *domain.GanttTask
	for _, task := range f.tasks {
		if task.EpicID == epicID && task.IsParent && task.ParentTaskID == nil {
			epicParentTask = task
		}
	}
	if epicParentTask == nil {
		t.Fatalf("expected epic parent task to exist")
	}
	fe = findTaskByParentAndName(f, epicParentTask.ID, "FE разработчик")
	if fe == nil {
		t.Fatalf("expected FE task to exist")
	}
	baselineStart := fe.StartDate

	notBefore := date(2026, 7, 27)
	if _, err := svc.SetTaskStartConstraint(ctx, fe.ID, &notBefore, nil, nil); err != nil {
		t.Fatalf("SetTaskStartConstraint (set): %v", err)
	}
	if f.tasks[fe.ID].StartDate.Equal(baselineStart) {
		t.Fatalf("precondition failed: date should have moved the start")
	}

	if _, err := svc.SetTaskStartConstraint(ctx, fe.ID, nil, nil, nil); err != nil {
		t.Fatalf("SetTaskStartConstraint (clear): %v", err)
	}
	feGot := f.tasks[fe.ID]
	if !feGot.StartDate.Equal(baselineStart) {
		t.Errorf("FE start after clearing = %v, want %v (back to computed start)", feGot.StartDate, baselineStart)
	}
}

// TestRecalculateTeamSchedule_StartConstraint_WaitForTask_BackwardReference
// проверяет задачу 2.1/спеку "Задача не начинается раньше завершения
// выбранной задачи": ссылка на задачу другого (более приоритетного, то есть
// уже обработанного) эпика той же команды. Разрешается за один проход, но
// упражняет тот же путь кода, что и forward-ссылка ниже.
func TestRecalculateTeamSchedule_StartConstraint_WaitForTask_BackwardReference(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicAID := uuid.New() // sort_order 1 — обрабатывается первым
	epicBID := uuid.New() // sort_order 2 — обрабатывается вторым, ждёт задачу A
	beID := uuid.New()
	feID := uuid.New()

	soA, soB := 1, 2
	f.addEpic(&domain.Epic{ID: epicAID, Number: "E-1", Name: "Epic A", TeamID: teamID, SortOrder: &soA})
	f.addEpic(&domain.Epic{ID: epicBID, Number: "E-2", Name: "Epic B", TeamID: teamID, SortOrder: &soB})
	f.addRole(&domain.Role{ID: beID, Name: "BE разработчик"})
	f.addRole(&domain.Role{ID: feID, Name: "FE разработчик"})
	f.roleScores[epicAID] = []domain.EpicRoleScore{{EpicID: epicAID, RoleID: beID, WeightedAvg: 5.0}}
	f.roleScores[epicBID] = []domain.EpicRoleScore{{EpicID: epicBID, RoleID: feID, WeightedAvg: 1.0}}

	start := date(2026, 7, 13) // Monday
	if _, err := svc.GenerateTasksForEpic(ctx, epicAID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := svc.GenerateTasksForEpic(ctx, epicBID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	be := findTaskByParentAndName(f, epicAID, "BE разработчик")
	if be == nil {
		// Legacy-эпик без сторей: ролевая задача — прямой child родителя эпика.
		parentA := f.tasks[epicAID]
		_ = parentA
	}
	var beTask, feTask *domain.GanttTask
	for _, task := range f.tasks {
		if task.Name == "BE разработчик" {
			beTask = task
		}
		if task.Name == "FE разработчик" {
			feTask = task
		}
	}
	if beTask == nil || feTask == nil {
		t.Fatalf("expected both BE and FE tasks to exist")
	}
	if !beTask.EndDate.Equal(date(2026, 7, 17)) {
		t.Fatalf("precondition failed: BE end = %v, want Jul17", beTask.EndDate)
	}

	// FE (эпик B) ждёт BE (эпик A, более приоритетный, уже обработан).
	if _, err := svc.SetTaskStartConstraint(ctx, feTask.ID, nil, &epicAID, &beID); err != nil {
		t.Fatalf("SetTaskStartConstraint: %v", err)
	}

	feGot := f.tasks[feTask.ID]
	want := date(2026, 7, 20) // nextAvailable(Fri Jul17) -> Mon Jul20
	if !feGot.StartDate.Equal(want) {
		t.Errorf("FE start = %v, want %v (after BE finishes)", feGot.StartDate, want)
	}
}

// TestRecalculateTeamSchedule_StartConstraint_WaitForTask_ForwardReferenceAcrossEpics
// проверяет задачу 2.2 — ключевой сценарий заявки: ссылка "не ранее задачи
// X" указывает на задачу МЕНЕЕ приоритетного эпика, обрабатываемого ПОЗЖЕ
// текущей в очереди планировщика. Один прямой проход такую ссылку не
// разрешил бы (design.md Решение 2) — проверяется, что повторный проход
// всё равно даёт корректный старт.
func TestRecalculateTeamSchedule_StartConstraint_WaitForTask_ForwardReferenceAcrossEpics(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicAID := uuid.New() // sort_order 1 — обрабатывается ПЕРВЫМ, но ждёт B
	epicBID := uuid.New() // sort_order 2 — обрабатывается ВТОРЫМ (менее приоритетный)
	beID := uuid.New()
	feID := uuid.New()

	soA, soB := 1, 2
	f.addEpic(&domain.Epic{ID: epicAID, Number: "E-1", Name: "Epic A", TeamID: teamID, SortOrder: &soA})
	f.addEpic(&domain.Epic{ID: epicBID, Number: "E-2", Name: "Epic B", TeamID: teamID, SortOrder: &soB})
	f.addRole(&domain.Role{ID: beID, Name: "BE разработчик"})
	f.addRole(&domain.Role{ID: feID, Name: "FE разработчик"})
	f.roleScores[epicAID] = []domain.EpicRoleScore{{EpicID: epicAID, RoleID: beID, WeightedAvg: 1.0}}
	f.roleScores[epicBID] = []domain.EpicRoleScore{{EpicID: epicBID, RoleID: feID, WeightedAvg: 5.0}}

	start := date(2026, 7, 13) // Monday
	if _, err := svc.GenerateTasksForEpic(ctx, epicAID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := svc.GenerateTasksForEpic(ctx, epicBID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var beTask, feTask *domain.GanttTask
	for _, task := range f.tasks {
		if task.Name == "BE разработчик" {
			beTask = task
		}
		if task.Name == "FE разработчик" {
			feTask = task
		}
	}
	if beTask == nil || feTask == nil {
		t.Fatalf("expected both BE and FE tasks to exist")
	}
	if !beTask.StartDate.Equal(date(2026, 7, 13)) || !feTask.EndDate.Equal(date(2026, 7, 17)) {
		t.Fatalf("precondition failed: BE start=%v FE end=%v", beTask.StartDate, feTask.EndDate)
	}

	// A (обрабатывается первым, sort_order=1) ждёт B (обрабатывается вторым,
	// менее приоритетный, sort_order=2) — forward-ссылка.
	if _, err := svc.SetTaskStartConstraint(ctx, beTask.ID, nil, &epicBID, &feID); err != nil {
		t.Fatalf("SetTaskStartConstraint: %v", err)
	}

	beGot := f.tasks[beTask.ID]
	want := date(2026, 7, 20) // nextAvailable(Fri Jul17) -> Mon Jul20
	if !beGot.StartDate.Equal(want) {
		t.Errorf("BE (higher-priority epic) start = %v, want %v (waits for the lower-priority epic's task)",
			beGot.StartDate, want)
	}
}

// TestRecalculateTeamSchedule_StartConstraint_WaitForTask_EndsEarly_DoesNotMoveStart
// проверяет спеку "Выбранная задача завершается рано": если выбранная
// задача завершается раньше расчётного старта текущей, старт не меняется.
func TestRecalculateTeamSchedule_StartConstraint_WaitForTask_EndsEarly_DoesNotMoveStart(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicAID := uuid.New()
	epicBID := uuid.New()
	analystID := uuid.New()
	beID := uuid.New()
	feID := uuid.New()

	soA, soB := 1, 2
	f.addEpic(&domain.Epic{ID: epicAID, Number: "E-1", Name: "Epic A", TeamID: teamID, SortOrder: &soA})
	f.addEpic(&domain.Epic{ID: epicBID, Number: "E-2", Name: "Epic B", TeamID: teamID, SortOrder: &soB})
	f.addRole(&domain.Role{ID: analystID, Name: "Аналитик"})
	f.addRole(&domain.Role{ID: beID, Name: "BE разработчик"})
	f.addRole(&domain.Role{ID: feID, Name: "FE разработчик"})
	// A: длинный аналитик + короткий BE (расчётный старт BE поздний).
	f.roleScores[epicAID] = []domain.EpicRoleScore{
		{EpicID: epicAID, RoleID: analystID, WeightedAvg: 5.0},
		{EpicID: epicAID, RoleID: beID, WeightedAvg: 1.0},
	}
	// B: очень короткий FE (заканчивается рано).
	f.roleScores[epicBID] = []domain.EpicRoleScore{{EpicID: epicBID, RoleID: feID, WeightedAvg: 1.0}}

	start := date(2026, 7, 13)
	if _, err := svc.GenerateTasksForEpic(ctx, epicAID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := svc.GenerateTasksForEpic(ctx, epicBID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var beTask *domain.GanttTask
	for _, task := range f.tasks {
		if task.Name == "BE разработчик" {
			beTask = task
		}
	}
	baselineStart := beTask.StartDate

	if _, err := svc.SetTaskStartConstraint(ctx, beTask.ID, nil, &epicBID, &feID); err != nil {
		t.Fatalf("SetTaskStartConstraint: %v", err)
	}

	beGot := f.tasks[beTask.ID]
	if !beGot.StartDate.Equal(baselineStart) {
		t.Errorf("BE start = %v, want %v (unchanged — waited-for task ends earlier than the computed start)",
			beGot.StartDate, baselineStart)
	}
}

// TestRecalculateTeamSchedule_StartConstraint_BothBounds_LatestWins проверяет
// задачу 2.1, Requirement "Оба ограничения действуют одновременно": когда
// заданы и дата, и задача, действует позднейшее из двух.
func TestRecalculateTeamSchedule_StartConstraint_BothBounds_LatestWins(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicAID := uuid.New()
	epicBID := uuid.New()
	beID := uuid.New()
	feID := uuid.New()

	soA, soB := 1, 2
	f.addEpic(&domain.Epic{ID: epicAID, Number: "E-1", Name: "Epic A", TeamID: teamID, SortOrder: &soA})
	f.addEpic(&domain.Epic{ID: epicBID, Number: "E-2", Name: "Epic B", TeamID: teamID, SortOrder: &soB})
	f.addRole(&domain.Role{ID: beID, Name: "BE разработчик"})
	f.addRole(&domain.Role{ID: feID, Name: "FE разработчик"})
	f.roleScores[epicAID] = []domain.EpicRoleScore{{EpicID: epicAID, RoleID: beID, WeightedAvg: 1.0}}
	f.roleScores[epicBID] = []domain.EpicRoleScore{{EpicID: epicBID, RoleID: feID, WeightedAvg: 1.0}}

	start := date(2026, 7, 13)
	if _, err := svc.GenerateTasksForEpic(ctx, epicAID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := svc.GenerateTasksForEpic(ctx, epicBID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var beTask *domain.GanttTask
	for _, task := range f.tasks {
		if task.Name == "BE разработчик" {
			beTask = task
		}
	}
	// FE (эпик B, "не ранее задачи") заканчивается Jul13 -> nextAvailable = Jul14.
	waitForStart := date(2026, 7, 14)

	// Case 1: дата (Aug 3) позже, чем окончание выбранной задачи (Jul14) — побеждает дата.
	notBeforeLater := date(2026, 8, 3)
	if _, err := svc.SetTaskStartConstraint(ctx, beTask.ID, &notBeforeLater, &epicBID, &feID); err != nil {
		t.Fatalf("SetTaskStartConstraint: %v", err)
	}
	beGot := f.tasks[beTask.ID]
	if !beGot.StartDate.Equal(notBeforeLater) {
		t.Errorf("case date>task: BE start = %v, want %v (later of the two — the date)", beGot.StartDate, notBeforeLater)
	}

	// Case 2: дата (Jul06) раньше, чем окончание выбранной задачи — побеждает задача.
	notBeforeEarlier := date(2026, 7, 6)
	if _, err := svc.SetTaskStartConstraint(ctx, beTask.ID, &notBeforeEarlier, &epicBID, &feID); err != nil {
		t.Fatalf("SetTaskStartConstraint: %v", err)
	}
	beGot = f.tasks[beTask.ID]
	if !beGot.StartDate.Equal(waitForStart) {
		t.Errorf("case date<task: BE start = %v, want %v (later of the two — the waited-for task's end)", beGot.StartDate, waitForStart)
	}
}

// TestRecalculateTeamSchedule_StartConstraint_StoryPredecessorStillWins
// проверяет задачу 2.1, Scenario "Предусловия стори сильнее": указанная
// дата раньше, чем завершается предыдущая ролевая группа той же стори — она
// не притягивает задачу, предусловие стори остаётся сильнее.
func TestRecalculateTeamSchedule_StartConstraint_StoryPredecessorStillWins(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	storyID := uuid.New()
	analystID := uuid.New()
	feID := uuid.New()

	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})
	story := f.addEpic(&domain.Epic{ID: storyID, Number: "E-1-S1", Name: "Story 1", TeamID: teamID, ParentEpicID: &epicID})
	f.addRole(&domain.Role{ID: analystID, Name: "Аналитик"})
	f.addRole(&domain.Role{ID: feID, Name: "FE разработчик"})
	f.roleScores[storyID] = []domain.EpicRoleScore{
		{EpicID: storyID, RoleID: analystID, WeightedAvg: 5.0},
		{EpicID: storyID, RoleID: feID, WeightedAvg: 1.0},
	}

	start := date(2026, 7, 13)
	if _, err := svc.GenerateTasksForEpic(ctx, epicID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	storyTask := findStoryTask(f, storyName(story))
	analystTask := findTaskByParentAndName(f, storyTask.ID, "Аналитик")
	feTask := findTaskByParentAndName(f, storyTask.ID, "FE разработчик")

	// Дата раньше окончания аналитика (предусловие стори).
	notBefore := date(2026, 7, 13)
	if _, err := svc.SetTaskStartConstraint(ctx, feTask.ID, &notBefore, nil, nil); err != nil {
		t.Fatalf("SetTaskStartConstraint: %v", err)
	}

	feGot := f.tasks[feTask.ID]
	if feGot.StartDate.Before(analystTask.EndDate) {
		t.Errorf("FE start = %v must not be before analyst end %v — story predecessor constraint must win",
			feGot.StartDate, analystTask.EndDate)
	}
	if feGot.StartDate.Equal(notBefore) {
		t.Errorf("FE start must not equal the not-before date (%v) — it's earlier than the story predecessor allows", notBefore)
	}
}

// TestRecalculateTeamSchedule_StartConstraint_OffsetStillApplies проверяет
// задачу 2.1, Scenario "Смещение старта продолжает действовать": при
// одновременно заданных ручном смещении и ограничении "Начать не ранее"
// задача начинается не раньше позднейшего из двух.
func TestRecalculateTeamSchedule_StartConstraint_OffsetStillApplies(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	anchorEpicID := uuid.New()
	epicID := uuid.New()
	analystID := uuid.New()
	feID := uuid.New()

	// anchorEpicID анкерует teamFloor команды на Jul13 весь тест — иначе,
	// в команде с ОДНИМ эпиком, teamFloor считается по его же собственной
	// (уже сдвинутой предыдущим вызовом) дате родителя, и каждый следующий
	// SetTaskStartOffset/SetTaskStartConstraint компаундил бы смещение на
	// уже сдвинутый floor. Тот же пре-экзистинг эффект и то же обоснование,
	// что и в TestSetTaskStartConstraint_ClearingDateReturnsToComputedStart —
	// вне объёма этой заявки.
	f.addEpic(&domain.Epic{ID: anchorEpicID, Number: "E-0", Name: "Anchor", TeamID: teamID})
	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})
	f.addRole(&domain.Role{ID: analystID, Name: "Аналитик"})
	f.addRole(&domain.Role{ID: feID, Name: "FE разработчик"})
	f.roleScores[anchorEpicID] = []domain.EpicRoleScore{{EpicID: anchorEpicID, RoleID: analystID, WeightedAvg: 1.0}}
	f.roleScores[epicID] = []domain.EpicRoleScore{{EpicID: epicID, RoleID: feID, WeightedAvg: 1.0}}

	start := date(2026, 7, 13) // Monday
	if _, err := svc.GenerateTasksForEpic(ctx, anchorEpicID, start); err != nil {
		t.Fatalf("unexpected error (anchor): %v", err)
	}
	if _, err := svc.GenerateTasksForEpic(ctx, epicID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var epicParentTask, fe *domain.GanttTask
	for _, task := range f.tasks {
		if task.EpicID == epicID && task.IsParent && task.ParentTaskID == nil {
			epicParentTask = task
		}
	}
	if epicParentTask == nil {
		t.Fatalf("expected epic parent task to exist")
	}
	fe = findTaskByParentAndName(f, epicParentTask.ID, "FE разработчик")
	if fe == nil {
		t.Fatalf("expected FE task to exist")
	}

	// Смещение "+10 дней" сдвинуло бы старт на Jul23 (10 календарных дней
	// вперёд, затем на рабочий день); дата "не ранее" — Jul16, раньше
	// смещённой цели — должно победить смещение (оно позднее).
	if _, err := svc.SetTaskStartOffset(ctx, fe.ID, 10); err != nil {
		t.Fatalf("SetTaskStartOffset: %v", err)
	}
	offsetOnlyStart := f.tasks[fe.ID].StartDate

	notBefore := date(2026, 7, 16)
	if _, err := svc.SetTaskStartConstraint(ctx, fe.ID, &notBefore, nil, nil); err != nil {
		t.Fatalf("SetTaskStartConstraint: %v", err)
	}
	feGot := f.tasks[fe.ID]
	if !feGot.StartDate.Equal(offsetOnlyStart) {
		t.Errorf("FE start = %v, want %v (offset is later than the not-before date and must still win)",
			feGot.StartDate, offsetOnlyStart)
	}

	// Теперь дата позже смещения — дата должна победить.
	notBeforeLater := offsetOnlyStart.AddDate(0, 0, 14)
	if _, err := svc.SetTaskStartConstraint(ctx, fe.ID, &notBeforeLater, nil, nil); err != nil {
		t.Fatalf("SetTaskStartConstraint (later date): %v", err)
	}
	feGot = f.tasks[fe.ID]
	if !feGot.StartDate.Equal(notBeforeLater) {
		t.Errorf("FE start = %v, want %v (not-before date is later than the offset and must win)",
			feGot.StartDate, notBeforeLater)
	}
}

// TestRecalculateTeamSchedule_StartConstraint_BackfillDoesNotUseGapBeforeConstraint
// проверяет задачу 2.3 (дельта к gantt-schedule-backfill): промежуток,
// начинающийся раньше, чем допускает ограничение "Начать не ранее", не
// занимается — задача уходит в первый подходящий промежуток ПОСЛЕ него.
func TestRecalculateTeamSchedule_StartConstraint_BackfillDoesNotUseGapBeforeConstraint(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	story1ID := uuid.New()
	story2ID := uuid.New()
	feID := uuid.New()
	feUser := uuid.New()

	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})
	story1 := f.addEpic(&domain.Epic{ID: story1ID, Number: "E-1-S1", Name: "Story 1", TeamID: teamID, ParentEpicID: &epicID})
	story2 := f.addEpic(&domain.Epic{ID: story2ID, Number: "E-1-S2", Name: "Story 2", TeamID: teamID, ParentEpicID: &epicID})
	f.addRole(&domain.Role{ID: feID, Name: "FE разработчик"})
	f.addUser(&domain.User{ID: feUser, FirstName: "F"}, []uuid.UUID{teamID}, []uuid.UUID{feID})

	f.roleScores[story1ID] = []domain.EpicRoleScore{{EpicID: story1ID, RoleID: feID, WeightedAvg: 1.0}}
	f.roleScores[story2ID] = []domain.EpicRoleScore{{EpicID: story2ID, RoleID: feID, WeightedAvg: 1.0}}

	start := date(2026, 7, 13) // Monday
	if _, err := svc.GenerateTasksForEpic(ctx, epicID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	story1Task := findStoryTask(f, storyName(story1))
	story2Task := findStoryTask(f, storyName(story2))
	feStory1 := findTaskByParentAndName(f, story1Task.ID, "FE разработчик")
	feStory2 := findTaskByParentAndName(f, story2Task.ID, "FE разработчик")
	if feStory1 == nil || feStory2 == nil {
		t.Fatalf("expected both FE tasks to exist")
	}
	if !feStory1.StartDate.Equal(date(2026, 7, 13)) || !feStory2.StartDate.Equal(date(2026, 7, 14)) {
		t.Fatalf("precondition failed: story1=%v story2=%v", feStory1.StartDate, feStory2.StartDate)
	}

	// Сдвигаем story1 далеко вперёд (освобождает простой Jul13-Jul16 перед
	// ней) и задаём story2 (тому же исполнителю) ограничение "не ранее
	// Jul15" — простой доступен (Jul13-Jul16), но НАЧИНАЕТСЯ он раньше
	// разрешённой даты, и задача должна встать не раньше Jul15, а не занять
	// простой целиком с Jul13.
	if _, err := svc.SetTaskStartOffset(ctx, feStory1.ID, 10); err != nil {
		t.Fatalf("SetTaskStartOffset: %v", err)
	}
	notBefore := date(2026, 7, 15)
	if _, err := svc.SetTaskStartConstraint(ctx, feStory2.ID, &notBefore, nil, nil); err != nil {
		t.Fatalf("SetTaskStartConstraint: %v", err)
	}

	feStory2Got := f.tasks[feStory2.ID]
	if feStory2Got.StartDate.Before(notBefore) {
		t.Errorf("FE story2 start = %v must not be before the not-before date %v", feStory2Got.StartDate, notBefore)
	}
	if !feStory2Got.StartDate.Equal(notBefore) {
		t.Errorf("FE story2 start = %v, want %v (first work day allowed by the constraint)", feStory2Got.StartDate, notBefore)
	}
}

// TestRecalculateTeamSchedule_StartConstraint_BackfillSkipsGapEntirelyTooEarlyForConstraint
// проверяет задачу 6.3 более строго, чем BackfillDoesNotUseGapBeforeConstraint
// выше: календарь исполнителя содержит ДВА промежутка — ближний, который
// без ограничения был бы выбран first-fit (задача туда физически
// помещается по длительности), и дальний, после второй брони. Ограничение
// "не ранее" делает старт внутри ближнего промежутка настолько поздним,
// что задача 2 рабочих дня туда уже не помещается целиком (упёрлась бы во
// вторую бронь) — она обязана уйти в дальний промежуток (открытый хвост
// после второй брони), а не остаться в ближнем только потому, что тот
// "в принципе подходил" без учёта ограничения. Так проверяется, что
// earliest действительно доходит до КАЖДОЙ итерации поиска внутри
// findEarliestSlot (design.md, задача 2.3), а не только до первого вызова.
func TestRecalculateTeamSchedule_StartConstraint_BackfillSkipsGapEntirelyTooEarlyForConstraint(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	story1ID := uuid.New()
	story2ID := uuid.New()
	story3ID := uuid.New()
	feID := uuid.New()
	feUser := uuid.New()

	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})
	story1 := f.addEpic(&domain.Epic{ID: story1ID, Number: "E-1-S1", Name: "Story 1", TeamID: teamID, ParentEpicID: &epicID})
	story2 := f.addEpic(&domain.Epic{ID: story2ID, Number: "E-1-S2", Name: "Story 2", TeamID: teamID, ParentEpicID: &epicID})
	story3 := f.addEpic(&domain.Epic{ID: story3ID, Number: "E-1-S3", Name: "Story 3", TeamID: teamID, ParentEpicID: &epicID})
	f.addRole(&domain.Role{ID: feID, Name: "FE разработчик"})
	f.addUser(&domain.User{ID: feUser, FirstName: "F"}, []uuid.UUID{teamID}, []uuid.UUID{feID})

	f.roleScores[story1ID] = []domain.EpicRoleScore{{EpicID: story1ID, RoleID: feID, WeightedAvg: 1.0}}
	f.roleScores[story2ID] = []domain.EpicRoleScore{{EpicID: story2ID, RoleID: feID, WeightedAvg: 1.0}}
	f.roleScores[story3ID] = []domain.EpicRoleScore{{EpicID: story3ID, RoleID: feID, WeightedAvg: 2.0}}

	start := date(2026, 7, 13) // Monday
	if _, err := svc.GenerateTasksForEpic(ctx, epicID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	story1Task := findStoryTask(f, storyName(story1))
	story2Task := findStoryTask(f, storyName(story2))
	story3Task := findStoryTask(f, storyName(story3))
	feStory1 := findTaskByParentAndName(f, story1Task.ID, "FE разработчик")
	feStory2 := findTaskByParentAndName(f, story2Task.ID, "FE разработчик")
	feStory3 := findTaskByParentAndName(f, story3Task.ID, "FE разработчик")
	if feStory1 == nil || feStory2 == nil || feStory3 == nil {
		t.Fatalf("expected all three FE tasks to exist")
	}

	// Замораживаем story1 на Jul13 (Mon) и story2 на Jul20 (Mon) — календарь
	// feUser получает промежуток Jul14-Jul17 (Tue-Fri, 4 рабочих дня) между
	// ними и открытый хвост после Jul20.
	feStory1.Progress = 100
	feStory1.StartDate = date(2026, 7, 13)
	feStory1.EndDate = date(2026, 7, 13)
	feStory1.ActualEndDate = &feStory1.EndDate
	f.tasks[feStory1.ID] = feStory1

	feStory2.Progress = 100
	feStory2.StartDate = date(2026, 7, 20)
	feStory2.EndDate = date(2026, 7, 20)
	feStory2.ActualEndDate = &feStory2.EndDate
	f.tasks[feStory2.ID] = feStory2

	if _, err := svc.RecalculateTeamSchedule(ctx, teamID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Контроль: БЕЗ ограничения story3 (2 рабочих дня) заняла бы ближний
	// промежуток целиком (Jul14-Jul15) — first-fit.
	feStory3Got := f.tasks[feStory3.ID]
	if !feStory3Got.StartDate.Equal(date(2026, 7, 14)) {
		t.Fatalf("precondition failed: without a constraint FE story3 should backfill the near gap at Jul14, got %v",
			feStory3Got.StartDate)
	}

	// Ограничение "не ранее Jul17" делает старт внутри ближнего промежутка
	// настолько поздним, что 2 рабочих дня (Jul17-Jul20) уже упираются в
	// бронь story2 (Jul20) — промежуток для story3 больше не подходит
	// целиком, несмотря на то что "по длительности" он бы вместил задачу,
	// если бы не ограничение.
	notBefore := date(2026, 7, 17)
	if _, err := svc.SetTaskStartConstraint(ctx, feStory3.ID, &notBefore, nil, nil); err != nil {
		t.Fatalf("SetTaskStartConstraint: %v", err)
	}

	feStory3Got = f.tasks[feStory3.ID]
	if feStory3Got.StartDate.Before(notBefore) {
		t.Errorf("FE story3 start = %v must not be before the not-before date %v", feStory3Got.StartDate, notBefore)
	}
	// Обязан уйти в открытый хвост после story2 (Jul20), а не втиснуться в
	// уже негодный ближний промежуток и тем более не столкнуться со story2.
	wantStart := date(2026, 7, 21) // nextAvailable(Jul20) -> Jul21 (Tue)
	if !feStory3Got.StartDate.Equal(wantStart) {
		t.Errorf("FE story3 start = %v, want %v (near gap no longer fits once constrained — must move to the tail after story2)",
			feStory3Got.StartDate, wantStart)
	}
	if feStory3Got.StartDate.Before(feStory2.EndDate) || feStory3Got.StartDate.Equal(feStory2.EndDate) {
		t.Errorf("FE story3 start = %v must be strictly after story2's booking (%v) — no overlap", feStory3Got.StartDate, feStory2.EndDate)
	}
}

// TestRecalculateTeamSchedule_StartConstraint_InvalidReference_IgnoredThenRecovers
// проверяет задачу 2.4 / спеку "Исчезнувшая задача не ломает генерацию":
// снятие оценки по роли делает ссылку недействующей (пересчёт не падает,
// ограничение по задаче не применяется), а ограничение по дате продолжает
// действовать; возврат оценки возвращает ссылку в силу.
func TestRecalculateTeamSchedule_StartConstraint_InvalidReference_IgnoredThenRecovers(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicAID := uuid.New()
	epicBID := uuid.New()
	beID := uuid.New()
	feID := uuid.New()
	qaID := uuid.New()

	soA, soB := 1, 2
	f.addEpic(&domain.Epic{ID: epicAID, Number: "E-1", Name: "Epic A", TeamID: teamID, SortOrder: &soA})
	f.addEpic(&domain.Epic{ID: epicBID, Number: "E-2", Name: "Epic B", TeamID: teamID, SortOrder: &soB})
	f.addRole(&domain.Role{ID: beID, Name: "BE разработчик"})
	f.addRole(&domain.Role{ID: feID, Name: "FE разработчик"})
	f.addRole(&domain.Role{ID: qaID, Name: "Тестировщик"})
	f.roleScores[epicAID] = []domain.EpicRoleScore{{EpicID: epicAID, RoleID: beID, WeightedAvg: 1.0}}
	// epicB получает ВТОРУЮ роль (QA), которая остаётся оценённой весь
	// тест — снятие оценки конкретно у FE ниже не должно оставлять эпик B
	// вовсе без ролей (generateTaskRowsForEpic отказывается генерировать
	// пустой эпик), это моделирует спеку буквально: "у стори... снимается
	// оценка по ЭТОЙ роли" (FE), а не удаление эпика целиком.
	f.roleScores[epicBID] = []domain.EpicRoleScore{
		{EpicID: epicBID, RoleID: feID, WeightedAvg: 3.0},
		{EpicID: epicBID, RoleID: qaID, WeightedAvg: 1.0},
	}

	start := date(2026, 7, 13)
	if _, err := svc.GenerateTasksForEpic(ctx, epicAID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := svc.GenerateTasksForEpic(ctx, epicBID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var beTask *domain.GanttTask
	for _, task := range f.tasks {
		if task.Name == "BE разработчик" {
			beTask = task
		}
	}
	baselineStart := beTask.StartDate

	notBefore := date(2026, 7, 15)
	if _, err := svc.SetTaskStartConstraint(ctx, beTask.ID, &notBefore, &epicBID, &feID); err != nil {
		t.Fatalf("SetTaskStartConstraint: %v", err)
	}
	// FE (эпик B, WeightedAvg=3.0 -> 3 рабочих дня): Jul13-Jul15,
	// nextAvailable(Jul15) -> Jul16 (Thu, уже рабочий день). notBefore
	// (Jul15) раньше этого — побеждает ссылка на задачу.
	if f.tasks[beTask.ID].StartDate.Before(date(2026, 7, 16)) {
		t.Fatalf("precondition failed: BE should be waiting for FE (epic B), got %v", f.tasks[beTask.ID].StartDate)
	}

	// Роль FE перестаёт оцениваться у эпика B (QA остаётся) — задача, на
	// которую указывает ссылка, исчезает (её строка Ганта удаляется при
	// перегенерации ниже), но сам эпик по-прежнему генерируется.
	f.roleScores[epicBID] = []domain.EpicRoleScore{{EpicID: epicBID, RoleID: qaID, WeightedAvg: 1.0}}
	if _, err := svc.GenerateTasksForEpic(ctx, epicBID, start); err != nil {
		t.Fatalf("regenerate epic B (unscored role): %v", err)
	}

	beGot := f.tasks[beTask.ID]
	// Ограничение по задаче больше не действует (цель исчезла), но
	// ограничение по дате продолжает работать (design.md Решение 4).
	if beGot.StartDate.Before(notBefore) {
		t.Errorf("BE start = %v must still respect the not-before date %v", beGot.StartDate, notBefore)
	}
	if !beGot.StartDate.Equal(maxTime(baselineStart, notBefore)) {
		t.Errorf("BE start = %v, want %v (date-only lower bound, task reference no longer resolvable)",
			beGot.StartDate, maxTime(baselineStart, notBefore))
	}

	// Оценка по роли возвращается — ссылка снова разрешается и снова действует.
	f.roleScores[epicBID] = []domain.EpicRoleScore{
		{EpicID: epicBID, RoleID: feID, WeightedAvg: 3.0},
		{EpicID: epicBID, RoleID: qaID, WeightedAvg: 1.0},
	}
	if _, err := svc.GenerateTasksForEpic(ctx, epicBID, start); err != nil {
		t.Fatalf("regenerate epic B (role scored again): %v", err)
	}
	beGot = f.tasks[beTask.ID]
	if beGot.StartDate.Before(date(2026, 7, 16)) {
		t.Errorf("BE start = %v — the task reference must be effective again once the role is scored again", beGot.StartDate)
	}
}

// TestSetTaskStartConstraint_SurvivesQuarterRegenerationAndRescoring
// проверяет задачу 6.5: ограничение и ссылка на задачу переживают массовую
// перегенерацию квартала (GenerateTasksForQuarter удаляет и пересоздаёт
// ВСЕ строки gantt_tasks обоих эпиков — задача 1.1/design.md Решение 1,
// тот же дефект, что уже случался со смещением старта до
// add-gantt-task-assignees) и последующую переоценку роли: строки задач
// получают НОВЫЕ ID при каждой регенерации, но ссылка (ключ "стори + роль")
// продолжает резолвиться в ту же логическую задачу и учитывать её
// АКТУАЛЬНЫЕ (пересчитанные после переоценки) даты, а не устаревшие.
func TestSetTaskStartConstraint_SurvivesQuarterRegenerationAndRescoring(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	beID := uuid.New()
	feID := uuid.New()
	f.addRole(&domain.Role{ID: beID, Name: "BE разработчик"})
	f.addRole(&domain.Role{ID: feID, Name: "FE разработчик"})

	epicA := addScoredEpicWithRole(f, teamID, beID, "E-1", 2026, 3, 1)
	epicB := addScoredEpicWithRole(f, teamID, feID, "E-2", 2026, 3, 2)
	// addScoredEpicWithRole статически прописывает WeightedAvg=2.0 под
	// ролью, переданной в аргументе — переопределяем роль эпика B на feID
	// с известной длительностью 1 рабочий день для точного контроля дат.
	f.roleScores[epicB.ID] = []domain.EpicRoleScore{{EpicID: epicB.ID, RoleID: feID, WeightedAvg: 1.0}}

	start := date(2026, 7, 13) // Monday
	if _, err := svc.GenerateTasksForQuarter(ctx, teamID, 2026, 3, start); err != nil {
		t.Fatalf("unexpected error (initial generation): %v", err)
	}

	findByName := func(name string) *domain.GanttTask {
		for _, task := range f.tasks {
			if task.Name == name {
				return task
			}
		}
		return nil
	}
	beTask := findByName("BE разработчик")
	feTask := findByName("FE разработчик")
	if beTask == nil || feTask == nil {
		t.Fatalf("expected both role tasks to exist after initial generation")
	}
	beTaskIDGen1 := beTask.ID

	notBefore := date(2026, 7, 15)
	if _, err := svc.SetTaskStartConstraint(ctx, beTask.ID, &notBefore, &epicB.ID, &feID); err != nil {
		t.Fatalf("SetTaskStartConstraint: %v", err)
	}
	// FE (1 рабочий день, Jul13) заканчивается Jul13 -> nextAvailable=Jul14;
	// notBefore=Jul15 позже — побеждает дата.
	if !f.tasks[beTaskIDGen1].StartDate.Equal(notBefore) {
		t.Fatalf("precondition failed: BE start = %v, want %v", f.tasks[beTaskIDGen1].StartDate, notBefore)
	}

	// Массовая перегенерация квартала: ВСЕ строки gantt_tasks обоих эпиков
	// удаляются и создаются заново — id листовых задач меняются.
	if _, err := svc.GenerateTasksForQuarter(ctx, teamID, 2026, 3, start); err != nil {
		t.Fatalf("unexpected error (quarter regeneration): %v", err)
	}
	beTask = findByName("BE разработчик")
	feTask = findByName("FE разработчик")
	if beTask == nil || feTask == nil {
		t.Fatalf("expected both role tasks to exist after quarter regeneration")
	}
	if beTask.ID == beTaskIDGen1 {
		t.Fatalf("precondition failed: expected a NEW gantt_tasks row id after regeneration (rows are deleted and recreated), got the same id")
	}
	// Ограничение должно было пережить удаление старой строки и
	// применяться к НОВОЙ — сама ссылка хранится в task_assignments по
	// паре "эпик (без сторей, как эпик) + роль", а не по id строки.
	if !beTask.StartDate.Equal(notBefore) {
		t.Errorf("after quarter regeneration: BE start = %v, want %v (constraint must survive regeneration)",
			beTask.StartDate, notBefore)
	}
	assignment, ok := f.assignments[assignmentKey{epicID: epicA.ID, roleID: beID}]
	if !ok {
		t.Fatalf("expected the task_assignments row to survive regeneration")
	}
	if assignment.WaitForStoryID == nil || *assignment.WaitForStoryID != epicB.ID ||
		assignment.WaitForRoleID == nil || *assignment.WaitForRoleID != feID {
		t.Errorf("wait-for reference changed across regeneration: got story=%v role=%v, want story=%v role=%v",
			assignment.WaitForStoryID, assignment.WaitForRoleID, epicB.ID, feID)
	}
	if assignment.NotBeforeDate == nil || !assignment.NotBeforeDate.Equal(notBefore) {
		t.Errorf("NotBeforeDate changed across regeneration: got %v, want %v", assignment.NotBeforeDate, notBefore)
	}
	beTaskIDGen2 := beTask.ID

	// "Переоценка стори": WeightedAvg роли FE эпика B меняется (задача
	// становится длиннее), и эпик B перегенерируется — цель ссылки должна
	// снова смениться на НОВУЮ строку и её НОВЫЕ (более поздние) даты
	// должны реально повлиять на BE, а не оставить его на устаревшей дате.
	f.roleScores[epicB.ID] = []domain.EpicRoleScore{{EpicID: epicB.ID, RoleID: feID, WeightedAvg: 5.0}}
	if _, err := svc.GenerateTasksForEpic(ctx, epicB.ID, start); err != nil {
		t.Fatalf("unexpected error (rescoring regeneration): %v", err)
	}
	beTask = findByName("BE разработчик")
	feTask = findByName("FE разработчик")
	if beTask == nil || feTask == nil {
		t.Fatalf("expected both role tasks to exist after rescoring regeneration")
	}
	// GenerateTasksForEpic(epicB) регенерирует только строки эпика B —
	// строка BE (эпик A) не пересоздаётся, её id не меняется; меняется
	// только то, что реально важно для этого сценария: цель ссылки (FE)
	// пересчитана заново со своей новой (более долгой) датой окончания.
	if beTask.ID != beTaskIDGen2 {
		t.Errorf("BE row id changed even though only epic B was regenerated — unexpected")
	}
	// FE теперь 5 рабочих дней (Jul13-Jul17) -> nextAvailable=Jul20,
	// позже notBefore (Jul15) — теперь побеждает ссылка на задачу, а не
	// дата, и её конец взят из АКТУАЛЬНОГО (пересчитанного) FE, не из
	// значения до переоценки.
	wantStart := date(2026, 7, 20)
	if !beTask.StartDate.Equal(wantStart) {
		t.Errorf("after rescoring: BE start = %v, want %v (must follow FE's NEW, longer duration — the reference re-resolves, not stuck on stale dates)",
			beTask.StartDate, wantStart)
	}
}

// TestRecalculateTeamSchedule_StartConstraint_CycleInDataDoesNotHang проверяет
// задачу 2.5: цикл, попавший в данные в обход проверки при сохранении
// (design.md Решение 3), не подвешивает расчёт — RecalculateTeamSchedule
// завершается за конечное время. Цикл собирается напрямую через
// fakeRepo.assignments, в обход SetTaskStartConstraint (которая цикл бы
// отклонила).
func TestRecalculateTeamSchedule_StartConstraint_CycleInDataDoesNotHang(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicAID := uuid.New()
	epicBID := uuid.New()
	beID := uuid.New()
	feID := uuid.New()

	soA, soB := 1, 2
	f.addEpic(&domain.Epic{ID: epicAID, Number: "E-1", Name: "Epic A", TeamID: teamID, SortOrder: &soA})
	f.addEpic(&domain.Epic{ID: epicBID, Number: "E-2", Name: "Epic B", TeamID: teamID, SortOrder: &soB})
	f.addRole(&domain.Role{ID: beID, Name: "BE разработчик"})
	f.addRole(&domain.Role{ID: feID, Name: "FE разработчик"})
	f.roleScores[epicAID] = []domain.EpicRoleScore{{EpicID: epicAID, RoleID: beID, WeightedAvg: 1.0}}
	f.roleScores[epicBID] = []domain.EpicRoleScore{{EpicID: epicBID, RoleID: feID, WeightedAvg: 1.0}}

	start := date(2026, 7, 13)
	if _, err := svc.GenerateTasksForEpic(ctx, epicAID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := svc.GenerateTasksForEpic(ctx, epicBID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Цикл напрямую в данных: A ждёт B, и B ждёт A — SetTaskStartConstraint
	// такое бы отклонила, здесь пишем в обход неё.
	f.assignments[assignmentKey{epicID: epicAID, roleID: beID}] = domain.TaskAssignment{
		EpicID: epicAID, RoleID: beID,
		WaitForStoryID: &epicBID, WaitForRoleID: &feID,
	}
	f.assignments[assignmentKey{epicID: epicBID, roleID: feID}] = domain.TaskAssignment{
		EpicID: epicBID, RoleID: feID,
		WaitForStoryID: &epicAID, WaitForRoleID: &beID,
	}

	done := make(chan error, 1)
	go func() {
		_, err := svc.RecalculateTeamSchedule(ctx, teamID)
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RecalculateTeamSchedule hung on a cyclic wait-for graph — expected it to terminate (design.md Решение 3)")
	}
}

// TestRecalculateTeamSchedule_StartConstraint_FrozenTaskIgnoresConstraint
// проверяет задачу 2.6: замороженная задача (Progress > 0) не пересчитывает
// даты — ограничение "Начать не ранее" к ней задним числом не применяется.
func TestRecalculateTeamSchedule_StartConstraint_FrozenTaskIgnoresConstraint(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	feID := uuid.New()
	feUser := uuid.New()

	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})
	f.addRole(&domain.Role{ID: feID, Name: "FE разработчик"})
	f.addUser(&domain.User{ID: feUser, FirstName: "F"}, []uuid.UUID{teamID}, []uuid.UUID{feID})
	f.roleScores[epicID] = []domain.EpicRoleScore{{EpicID: epicID, RoleID: feID, WeightedAvg: 3.0}}

	start := date(2026, 7, 13)
	tasks, err := svc.GenerateTasksForEpic(ctx, epicID, start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fe := findTaskByParentAndName(f, tasks[0].ID, "FE разработчик")

	// Ограничение задаём ДО заморозки — чтобы убедиться, что оно не
	// применяется задним числом уже к замороженной задаче.
	notBefore := date(2026, 8, 10)
	if _, err := svc.SetTaskStartConstraint(ctx, fe.ID, &notBefore, nil, nil); err != nil {
		t.Fatalf("SetTaskStartConstraint: %v", err)
	}
	// Заморозка (Progress > 0) как факт с более ранними, чем ограничение,
	// плановыми датами — как это сделал бы SetTaskProgress до 100%.
	feStored := f.tasks[fe.ID]
	feStored.Progress = 50
	feStored.StartDate = date(2026, 7, 13)
	feStored.EndDate = date(2026, 7, 15)
	f.tasks[fe.ID] = feStored

	if _, err := svc.RecalculateTeamSchedule(ctx, teamID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	feGot := f.tasks[fe.ID]
	if !feGot.StartDate.Equal(date(2026, 7, 13)) || !feGot.EndDate.Equal(date(2026, 7, 15)) {
		t.Errorf("frozen task must keep its own dates regardless of the constraint: got [%v,%v]",
			feGot.StartDate, feGot.EndDate)
	}
}

// TestRecalculateTeamSchedule_StartConstraint_DeterministicAcrossRepeatedCalls
// проверяет задачу 6.7: повторные пересчёты одного и того же, не
// меняющегося входа с несколькими ссылками "не ранее задачи" (в т.ч.
// вперёд по очереди, назад по очереди и цепочкой) дают ИДЕНТИЧНЫЕ даты и
// назначения раз за разом — ни resolvedEnd/assignments (карты, используемые
// повторным проходом, design.md Решение 2), ни пулы исполнителей не должны
// вносить зависимость от порядка обхода map. Отдельно прогоняется
// `go test -count=20 ./internal/gantt/`, чтобы поймать зависимость от
// случайного порядка обхода map между ЗАПУСКАМИ процесса (не только между
// вызовами внутри одного).
func TestRecalculateTeamSchedule_StartConstraint_DeterministicAcrossRepeatedCalls(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	beID, feID, qaID := uuid.New(), uuid.New(), uuid.New()
	f.addRole(&domain.Role{ID: beID, Name: "BE разработчик"})
	f.addRole(&domain.Role{ID: feID, Name: "FE разработчик"})
	f.addRole(&domain.Role{ID: qaID, Name: "Тестировщик"})

	// Пулы с несколькими кандидатами — исторический источник
	// недетерминизма (round-robin тай-брейк, design.md Решение 2 в
	// add-gantt-task-assignees), проверяется совместно с новыми картами.
	for i := 0; i < 3; i++ {
		f.addUser(&domain.User{ID: uuid.New(), FirstName: fmt.Sprintf("BE%d", i)}, []uuid.UUID{teamID}, []uuid.UUID{beID})
	}
	for i := 0; i < 2; i++ {
		f.addUser(&domain.User{ID: uuid.New(), FirstName: fmt.Sprintf("FE%d", i)}, []uuid.UUID{teamID}, []uuid.UUID{feID})
	}

	epic1ID, epic2ID, epic3ID, epic4ID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	so1, so2, so3, so4 := 1, 2, 3, 4
	f.addEpic(&domain.Epic{ID: epic1ID, Number: "E-1", Name: "Epic 1", TeamID: teamID, SortOrder: &so1})
	f.addEpic(&domain.Epic{ID: epic2ID, Number: "E-2", Name: "Epic 2", TeamID: teamID, SortOrder: &so2})
	f.addEpic(&domain.Epic{ID: epic3ID, Number: "E-3", Name: "Epic 3", TeamID: teamID, SortOrder: &so3})
	f.addEpic(&domain.Epic{ID: epic4ID, Number: "E-4", Name: "Epic 4", TeamID: teamID, SortOrder: &so4})
	f.roleScores[epic1ID] = []domain.EpicRoleScore{{EpicID: epic1ID, RoleID: beID, WeightedAvg: 2.0}}
	f.roleScores[epic2ID] = []domain.EpicRoleScore{{EpicID: epic2ID, RoleID: feID, WeightedAvg: 3.0}}
	f.roleScores[epic3ID] = []domain.EpicRoleScore{{EpicID: epic3ID, RoleID: feID, WeightedAvg: 4.0}}
	f.roleScores[epic4ID] = []domain.EpicRoleScore{{EpicID: epic4ID, RoleID: qaID, WeightedAvg: 1.0}}

	start := date(2026, 7, 13)
	for _, epicID := range []uuid.UUID{epic1ID, epic2ID, epic3ID, epic4ID} {
		if _, err := svc.GenerateTasksForEpic(ctx, epicID, start); err != nil {
			t.Fatalf("unexpected error generating epic %s: %v", epicID, err)
		}
	}

	findTaskByEpicAndName := func(epicID uuid.UUID, name string) *domain.GanttTask {
		for _, task := range f.tasks {
			if task.EpicID == epicID && task.Name == name {
				return task
			}
		}
		return nil
	}
	be1 := findTaskByEpicAndName(epic1ID, "BE разработчик")
	fe2 := findTaskByEpicAndName(epic2ID, "FE разработчик")
	fe3 := findTaskByEpicAndName(epic3ID, "FE разработчик")
	qa4 := findTaskByEpicAndName(epic4ID, "Тестировщик")
	if be1 == nil || fe2 == nil || fe3 == nil || qa4 == nil {
		t.Fatalf("expected all four leaf tasks to exist")
	}

	// Цепочка вперёд/назад по очереди: E1(BE) ждёт E3(FE) — ВПЕРЁД
	// (E3 обрабатывается позже E1, требует повторного прохода);
	// E2(FE) ждёт E1(BE) — НАЗАД; E4(QA) ждёт E2(FE) — тоже НАЗАД, но
	// целиком зависит от цепочки выше.
	if _, err := svc.SetTaskStartConstraint(ctx, be1.ID, nil, &epic3ID, &feID); err != nil {
		t.Fatalf("SetTaskStartConstraint (E1 waits E3): %v", err)
	}
	if _, err := svc.SetTaskStartConstraint(ctx, fe2.ID, nil, &epic1ID, &beID); err != nil {
		t.Fatalf("SetTaskStartConstraint (E2 waits E1): %v", err)
	}
	if _, err := svc.SetTaskStartConstraint(ctx, qa4.ID, nil, &epic2ID, &feID); err != nil {
		t.Fatalf("SetTaskStartConstraint (E4 waits E2): %v", err)
	}

	type taskState struct {
		start, end time.Time
		assignee   uuid.UUID
	}
	snapshot := func() map[uuid.UUID]taskState {
		result := make(map[uuid.UUID]taskState)
		for id, task := range f.tasks {
			if task.IsParent {
				continue
			}
			st := taskState{start: task.StartDate, end: task.EndDate}
			if task.AssigneeID != nil {
				st.assignee = *task.AssigneeID
			}
			result[id] = st
		}
		return result
	}

	first := snapshot()
	if len(first) != 4 {
		t.Fatalf("expected 4 leaf tasks in snapshot, got %d", len(first))
	}

	for attempt := 0; attempt < 10; attempt++ {
		if _, err := svc.RecalculateTeamSchedule(ctx, teamID); err != nil {
			t.Fatalf("attempt %d: unexpected error: %v", attempt, err)
		}
		got := snapshot()
		if len(got) != len(first) {
			t.Fatalf("attempt %d: leaf task count changed: got %d, want %d", attempt, len(got), len(first))
		}
		for taskID, want := range first {
			gotState, ok := got[taskID]
			if !ok {
				t.Fatalf("attempt %d: task %s missing from snapshot", attempt, taskID)
			}
			if !gotState.start.Equal(want.start) || !gotState.end.Equal(want.end) {
				t.Errorf("attempt %d: task %s dates changed: got [%v,%v], want [%v,%v]",
					attempt, taskID, gotState.start, gotState.end, want.start, want.end)
			}
			if gotState.assignee != want.assignee {
				t.Errorf("attempt %d: task %s assignee changed: got %v, want %v",
					attempt, taskID, gotState.assignee, want.assignee)
			}
		}
	}
}

// TestSetTaskStartConstraint_RejectsOnParentTask проверяет, что ограничение
// нельзя задать на родительской (стори/эпик) задаче.
func TestSetTaskStartConstraint_RejectsOnParentTask(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	feID := uuid.New()
	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})
	f.addRole(&domain.Role{ID: feID, Name: "FE разработчик"})
	f.roleScores[epicID] = []domain.EpicRoleScore{{EpicID: epicID, RoleID: feID, WeightedAvg: 1.0}}

	tasks, err := svc.GenerateTasksForEpic(ctx, epicID, date(2026, 7, 13))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parent := tasks[0]

	notBefore := date(2026, 7, 20)
	_, err = svc.SetTaskStartConstraint(ctx, parent.ID, &notBefore, nil, nil)
	if !errors.Is(err, ErrStartConstraintOnParent) {
		t.Errorf("err = %v, want ErrStartConstraintOnParent", err)
	}
}

// TestSetTaskStartConstraint_RejectsIncompleteRef проверяет, что задать
// только один из компонентов ссылки (стори без роли или наоборот) нельзя.
func TestSetTaskStartConstraint_RejectsIncompleteRef(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	feID := uuid.New()
	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})
	f.addRole(&domain.Role{ID: feID, Name: "FE разработчик"})
	f.roleScores[epicID] = []domain.EpicRoleScore{{EpicID: epicID, RoleID: feID, WeightedAvg: 1.0}}

	tasks, err := svc.GenerateTasksForEpic(ctx, epicID, date(2026, 7, 13))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fe := findTaskByParentAndName(f, tasks[0].ID, "FE разработчик")

	someStoryID := uuid.New()
	_, err = svc.SetTaskStartConstraint(ctx, fe.ID, nil, &someStoryID, nil)
	if !errors.Is(err, ErrStartConstraintIncompleteRef) {
		t.Errorf("story-only: err = %v, want ErrStartConstraintIncompleteRef", err)
	}

	someRoleID := uuid.New()
	_, err = svc.SetTaskStartConstraint(ctx, fe.ID, nil, nil, &someRoleID)
	if !errors.Is(err, ErrStartConstraintIncompleteRef) {
		t.Errorf("role-only: err = %v, want ErrStartConstraintIncompleteRef", err)
	}
}

// TestSetTaskStartConstraint_RejectsCrossTeamReference проверяет, что
// нельзя выбрать задачу другой команды.
func TestSetTaskStartConstraint_RejectsCrossTeamReference(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamA := uuid.New()
	teamB := uuid.New()
	epicAID := uuid.New()
	epicBID := uuid.New()
	feID := uuid.New()

	f.addEpic(&domain.Epic{ID: epicAID, Number: "E-1", Name: "Epic A", TeamID: teamA})
	f.addEpic(&domain.Epic{ID: epicBID, Number: "E-2", Name: "Epic B", TeamID: teamB})
	f.addRole(&domain.Role{ID: feID, Name: "FE разработчик"})
	f.roleScores[epicAID] = []domain.EpicRoleScore{{EpicID: epicAID, RoleID: feID, WeightedAvg: 1.0}}
	f.roleScores[epicBID] = []domain.EpicRoleScore{{EpicID: epicBID, RoleID: feID, WeightedAvg: 1.0}}

	tasksA, err := svc.GenerateTasksForEpic(ctx, epicAID, date(2026, 7, 13))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := svc.GenerateTasksForEpic(ctx, epicBID, date(2026, 7, 13)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	feA := findTaskByParentAndName(f, tasksA[0].ID, "FE разработчик")

	_, err = svc.SetTaskStartConstraint(ctx, feA.ID, nil, &epicBID, &feID)
	if !errors.Is(err, ErrStartConstraintCrossTeam) {
		t.Errorf("err = %v, want ErrStartConstraintCrossTeam", err)
	}
}

// TestSetTaskStartConstraint_RejectsSelfReference проверяет задачу 2.5,
// Scenario "Задача не может ожидать саму себя".
func TestSetTaskStartConstraint_RejectsSelfReference(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	feID := uuid.New()
	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})
	f.addRole(&domain.Role{ID: feID, Name: "FE разработчик"})
	f.roleScores[epicID] = []domain.EpicRoleScore{{EpicID: epicID, RoleID: feID, WeightedAvg: 1.0}}

	tasks, err := svc.GenerateTasksForEpic(ctx, epicID, date(2026, 7, 13))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fe := findTaskByParentAndName(f, tasks[0].ID, "FE разработчик")

	// Legacy-эпик без сторей: assignmentKey.epicID == epicID.
	_, err = svc.SetTaskStartConstraint(ctx, fe.ID, nil, &epicID, &feID)
	if !errors.Is(err, ErrStartConstraintCycle) {
		t.Errorf("err = %v, want ErrStartConstraintCycle", err)
	}

	// Ограничение не должно было сохраниться.
	assignments, _ := f.GetTaskAssignmentsByTeamID(ctx, teamID)
	for _, a := range assignments {
		if a.WaitForStoryID != nil {
			t.Errorf("self-reference must not have been persisted, got %+v", a)
		}
	}
}

// TestSetTaskStartConstraint_RejectsDirectCycle проверяет задачу 2.5,
// Scenario "Прямой цикл": A уже ждёт B, попытка задать B ждать A отклоняется.
func TestSetTaskStartConstraint_RejectsDirectCycle(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicAID := uuid.New()
	epicBID := uuid.New()
	beID := uuid.New()
	feID := uuid.New()

	f.addEpic(&domain.Epic{ID: epicAID, Number: "E-1", Name: "Epic A", TeamID: teamID})
	f.addEpic(&domain.Epic{ID: epicBID, Number: "E-2", Name: "Epic B", TeamID: teamID})
	f.addRole(&domain.Role{ID: beID, Name: "BE разработчик"})
	f.addRole(&domain.Role{ID: feID, Name: "FE разработчик"})
	f.roleScores[epicAID] = []domain.EpicRoleScore{{EpicID: epicAID, RoleID: beID, WeightedAvg: 1.0}}
	f.roleScores[epicBID] = []domain.EpicRoleScore{{EpicID: epicBID, RoleID: feID, WeightedAvg: 1.0}}

	if _, err := svc.GenerateTasksForEpic(ctx, epicAID, date(2026, 7, 13)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := svc.GenerateTasksForEpic(ctx, epicBID, date(2026, 7, 13)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Ищем по имени роли напрямую в f.tasks, а не по индексу возвращённого
	// GenerateTasksForEpic среза: он отдаёт ВСЕ задачи команды, а не только
	// только что сгенерированного эпика, и их порядок зависит от
	// sort_order — индексация по позиции здесь ненадёжна.
	var beA, feB *domain.GanttTask
	for _, task := range f.tasks {
		if task.Name == "BE разработчик" {
			beA = task
		}
		if task.Name == "FE разработчик" {
			feB = task
		}
	}
	if beA == nil || feB == nil {
		t.Fatalf("expected both BE and FE tasks to exist")
	}

	// A (BE) ждёт B (FE).
	if _, err := svc.SetTaskStartConstraint(ctx, beA.ID, nil, &epicBID, &feID); err != nil {
		t.Fatalf("SetTaskStartConstraint (A waits B): %v", err)
	}

	// Попытка B ждать A должна быть отклонена.
	_, err := svc.SetTaskStartConstraint(ctx, feB.ID, nil, &epicAID, &beID)
	if !errors.Is(err, ErrStartConstraintCycle) {
		t.Errorf("err = %v, want ErrStartConstraintCycle", err)
	}

	// У B по-прежнему нет ссылки (отклонённое значение не сохранилось).
	assignments, _ := f.GetTaskAssignmentsByTeamID(ctx, teamID)
	for _, a := range assignments {
		if a.EpicID == epicBID && a.WaitForStoryID != nil {
			t.Errorf("rejected reference must not have been persisted for B, got %+v", a)
		}
	}
}

// TestSetTaskStartConstraint_RejectsChainCycle проверяет задачу 2.5,
// Scenario "Цикл через цепочку": A ждёт B, B ждёт C, попытка C ждать A
// отклоняется.
func TestSetTaskStartConstraint_RejectsChainCycle(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicAID, epicBID, epicCID := uuid.New(), uuid.New(), uuid.New()
	roleA, roleB, roleC := uuid.New(), uuid.New(), uuid.New()

	f.addEpic(&domain.Epic{ID: epicAID, Number: "E-1", Name: "Epic A", TeamID: teamID})
	f.addEpic(&domain.Epic{ID: epicBID, Number: "E-2", Name: "Epic B", TeamID: teamID})
	f.addEpic(&domain.Epic{ID: epicCID, Number: "E-3", Name: "Epic C", TeamID: teamID})
	f.addRole(&domain.Role{ID: roleA, Name: "BE разработчик"})
	f.addRole(&domain.Role{ID: roleB, Name: "FE разработчик"})
	f.addRole(&domain.Role{ID: roleC, Name: "Тестировщик"})
	f.roleScores[epicAID] = []domain.EpicRoleScore{{EpicID: epicAID, RoleID: roleA, WeightedAvg: 1.0}}
	f.roleScores[epicBID] = []domain.EpicRoleScore{{EpicID: epicBID, RoleID: roleB, WeightedAvg: 1.0}}
	f.roleScores[epicCID] = []domain.EpicRoleScore{{EpicID: epicCID, RoleID: roleC, WeightedAvg: 1.0}}

	if _, err := svc.GenerateTasksForEpic(ctx, epicAID, date(2026, 7, 13)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := svc.GenerateTasksForEpic(ctx, epicBID, date(2026, 7, 13)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := svc.GenerateTasksForEpic(ctx, epicCID, date(2026, 7, 13)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var taskA, taskB, taskC *domain.GanttTask
	for _, task := range f.tasks {
		switch task.Name {
		case "BE разработчик":
			taskA = task
		case "FE разработчик":
			taskB = task
		case "Тестировщик":
			taskC = task
		}
	}
	if taskA == nil || taskB == nil || taskC == nil {
		t.Fatalf("expected all three role tasks to exist")
	}

	if _, err := svc.SetTaskStartConstraint(ctx, taskA.ID, nil, &epicBID, &roleB); err != nil {
		t.Fatalf("SetTaskStartConstraint (A waits B): %v", err)
	}
	if _, err := svc.SetTaskStartConstraint(ctx, taskB.ID, nil, &epicCID, &roleC); err != nil {
		t.Fatalf("SetTaskStartConstraint (B waits C): %v", err)
	}

	_, err := svc.SetTaskStartConstraint(ctx, taskC.ID, nil, &epicAID, &roleA)
	if !errors.Is(err, ErrStartConstraintCycle) {
		t.Errorf("err = %v, want ErrStartConstraintCycle", err)
	}
}

// TestGetTeamTaskOptions_ListsLeafTasksSortedByStoryThenRole проверяет
// задачу 2.7 ("получение состава задач команды для выбора"): выдаются все
// текущие листовые задачи команды, сгруппированные парой "стори + роль",
// отсортированные для стабильного порядка.
func TestGetTeamTaskOptions_ListsLeafTasksSortedByStoryThenRole(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	storyID := uuid.New()
	analystID := uuid.New()
	feID := uuid.New()

	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})
	story := f.addEpic(&domain.Epic{ID: storyID, Number: "E-1-S1", Name: "Story 1", TeamID: teamID, ParentEpicID: &epicID})
	f.addRole(&domain.Role{ID: analystID, Name: "Аналитик"})
	f.addRole(&domain.Role{ID: feID, Name: "FE разработчик"})
	f.roleScores[storyID] = []domain.EpicRoleScore{
		{EpicID: storyID, RoleID: analystID, WeightedAvg: 1.0},
		{EpicID: storyID, RoleID: feID, WeightedAvg: 1.0},
	}

	if _, err := svc.GenerateTasksForEpic(ctx, epicID, date(2026, 7, 13)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	options, err := svc.GetTeamTaskOptions(ctx, teamID)
	if err != nil {
		t.Fatalf("GetTeamTaskOptions: %v", err)
	}
	if len(options) != 2 {
		t.Fatalf("expected 2 options (one per role task), got %d: %+v", len(options), options)
	}
	for _, o := range options {
		if o.StoryID != storyID {
			t.Errorf("StoryID = %v, want %v", o.StoryID, storyID)
		}
		if o.StoryName != storyName(story) {
			t.Errorf("StoryName = %q, want %q", o.StoryName, storyName(story))
		}
	}
	// Роли внутри стори идут в порядке диаграммы (defaultRoleOrder:
	// Аналитик=1, раньше FE разработчик=2), а не по алфавиту — задача 3.5.
	if options[0].RoleName != "Аналитик" || options[1].RoleName != "FE разработчик" {
		t.Errorf("role order = [%s, %s], want [Аналитик, FE разработчик] (diagram/sort_order, not alphabetical)",
			options[0].RoleName, options[1].RoleName)
	}
}

// TestGetTeamTaskOptions_OrderMatchesDiagramNotAlphabet проверяет задачу
// 3.5 (UX-находка, ganttService.go:1503-1508 до исправления): выдача
// GetTeamTaskOptions идёт в порядке диаграммы — очередь эпиков, затем стори
// внутри эпика, затем роли внутри стори, — а не по алфавиту. Номера
// (Number, они же префикс StoryName — "<number>: <name>") НАРОЧНО не
// совпадают с sort_order на уровне эпика и стори, а имена ролей подобраны
// так, чтобы байтовое и ролевое (defaultRoleOrder) упорядочение были
// противоположны — на каждом из трёх уровней алфавитный порядок обязан
// ПРОТИВОРЕЧИТЬ диаграммному, иначе тест не был бы решающим: он обязан
// падать, если где-то в цепочке (группировка по эпику, стори, роли) снова
// просочится сортировка по имени/номеру вместо sort_order.
func TestGetTeamTaskOptions_OrderMatchesDiagramNotAlphabet(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	analystID := uuid.New()
	itLeadID := uuid.New()
	f.addRole(&domain.Role{ID: analystID, Name: "Аналитик"}) // defaultRoleOrder 1
	f.addRole(&domain.Role{ID: itLeadID, Name: "IT-лидер"})  // defaultRoleOrder 4
	// Байтовое сравнение кириллицы/латиницы даёт "IT-лидер" < "Аналитик" —
	// алфавитная сортировка расставила бы роли в ОБРАТНОМ порядке
	// относительно defaultRoleOrder.

	// epicFirst стоит В ОЧЕРЕДИ ПЕРВЫМ (sort_order=1), но его Number ("E-9")
	// алфавитно ПОЗЖЕ Number эпика epicSecond ("E-1", sort_order=2) —
	// алфавитный порядок строк "<number>: <name>" здесь прямо
	// противоположен порядку очереди.
	epicFirstID := uuid.New()
	epicSecondID := uuid.New()
	soFirst, soSecond := 1, 2
	f.addEpic(&domain.Epic{ID: epicFirstID, Number: "E-9", Name: "Later Alphabetically", TeamID: teamID, SortOrder: &soFirst})
	f.addEpic(&domain.Epic{ID: epicSecondID, Number: "E-1", Name: "Earlier Alphabetically", TeamID: teamID, SortOrder: &soSecond})

	// Внутри epicFirst стори storyFirst стоит ПЕРВОЙ (sort_order=1), но
	// её Number ("E-9-S9") алфавитно ПОЗЖЕ Number стори storySecond
	// ("E-9-S1", sort_order=2) — та же намеренная инверсия на уровне стори.
	storyFirstID := uuid.New()
	storySecondID := uuid.New()
	storySoFirst, storySoSecond := 1, 2
	f.addEpic(&domain.Epic{ID: storyFirstID, Number: "E-9-S9", Name: "Story queued first", TeamID: teamID, ParentEpicID: &epicFirstID, SortOrder: &storySoFirst})
	f.addEpic(&domain.Epic{ID: storySecondID, Number: "E-9-S1", Name: "Story queued second", TeamID: teamID, ParentEpicID: &epicFirstID, SortOrder: &storySoSecond})

	f.roleScores[storyFirstID] = []domain.EpicRoleScore{
		{EpicID: storyFirstID, RoleID: analystID, WeightedAvg: 1.0},
		{EpicID: storyFirstID, RoleID: itLeadID, WeightedAvg: 1.0},
	}
	f.roleScores[storySecondID] = []domain.EpicRoleScore{
		{EpicID: storySecondID, RoleID: analystID, WeightedAvg: 1.0},
	}
	f.roleScores[epicSecondID] = []domain.EpicRoleScore{
		{EpicID: epicSecondID, RoleID: analystID, WeightedAvg: 1.0},
	}

	start := date(2026, 7, 13)
	if _, err := svc.GenerateTasksForEpic(ctx, epicFirstID, start); err != nil {
		t.Fatalf("unexpected error (epicFirst): %v", err)
	}
	if _, err := svc.GenerateTasksForEpic(ctx, epicSecondID, start); err != nil {
		t.Fatalf("unexpected error (epicSecond): %v", err)
	}

	options, err := svc.GetTeamTaskOptions(ctx, teamID)
	if err != nil {
		t.Fatalf("GetTeamTaskOptions: %v", err)
	}
	if len(options) != 4 {
		t.Fatalf("expected 4 options, got %d: %+v", len(options), options)
	}

	// Ожидаемый порядок диаграммы: storyFirst (Аналитик, IT-лидер),
	// storySecond (Аналитик), затем epicSecond без сторей (Аналитик) —
	// ровно порядок очереди (sort_order), а не Number/имени.
	wantStoryIDs := []uuid.UUID{storyFirstID, storyFirstID, storySecondID, epicSecondID}
	wantRoleNames := []string{"Аналитик", "IT-лидер", "Аналитик", "Аналитик"}
	for i, o := range options {
		if o.StoryID != wantStoryIDs[i] {
			t.Errorf("options[%d].StoryID = %v, want %v (%q) — order must follow the pipeline queue (sort_order), not the alphabet",
				i, o.StoryID, wantStoryIDs[i], o.StoryName)
		}
		if o.RoleName != wantRoleNames[i] {
			t.Errorf("options[%d].RoleName = %q, want %q", i, o.RoleName, wantRoleNames[i])
		}
	}
}

// TestGetTeamTaskOptionsFor_MarksSelfAndCycleUnavailable проверяет задачу
// 2.7/3.3 на уровне сервиса (не через HTTP-мок): GetTeamTaskOptionsFor
// возвращает полный список задач команды, помечая недоступными саму
// редактируемую задачу и всё, что замкнуло бы цикл ожидания через уже
// существующие ссылки — но НЕ исключая их из выдачи (design.md Решение 6).
func TestGetTeamTaskOptionsFor_MarksSelfAndCycleUnavailable(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicAID, epicBID, epicCID := uuid.New(), uuid.New(), uuid.New()
	roleA, roleB, roleC := uuid.New(), uuid.New(), uuid.New()
	f.addEpic(&domain.Epic{ID: epicAID, Number: "E-1", Name: "Epic A", TeamID: teamID})
	f.addEpic(&domain.Epic{ID: epicBID, Number: "E-2", Name: "Epic B", TeamID: teamID})
	f.addEpic(&domain.Epic{ID: epicCID, Number: "E-3", Name: "Epic C", TeamID: teamID})
	f.addRole(&domain.Role{ID: roleA, Name: "BE разработчик"})
	f.addRole(&domain.Role{ID: roleB, Name: "FE разработчик"})
	f.addRole(&domain.Role{ID: roleC, Name: "Тестировщик"})
	f.roleScores[epicAID] = []domain.EpicRoleScore{{EpicID: epicAID, RoleID: roleA, WeightedAvg: 1.0}}
	f.roleScores[epicBID] = []domain.EpicRoleScore{{EpicID: epicBID, RoleID: roleB, WeightedAvg: 1.0}}
	f.roleScores[epicCID] = []domain.EpicRoleScore{{EpicID: epicCID, RoleID: roleC, WeightedAvg: 1.0}}

	start := date(2026, 7, 13)
	if _, err := svc.GenerateTasksForEpic(ctx, epicAID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := svc.GenerateTasksForEpic(ctx, epicBID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := svc.GenerateTasksForEpic(ctx, epicCID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var taskA, taskB, taskC *domain.GanttTask
	for _, task := range f.tasks {
		switch task.Name {
		case "BE разработчик":
			taskA = task
		case "FE разработчик":
			taskB = task
		case "Тестировщик":
			taskC = task
		}
	}
	if taskA == nil || taskB == nil || taskC == nil {
		t.Fatalf("expected all three role tasks to exist")
	}

	// A уже ждёт B — при выборе цели для A: B (замкнёт цикл через прямую
	// ссылку "назад") и сама A (самоссылка) должны быть недоступны, C —
	// доступна.
	if _, err := svc.SetTaskStartConstraint(ctx, taskA.ID, nil, &epicBID, &roleB); err != nil {
		t.Fatalf("SetTaskStartConstraint (A waits B): %v", err)
	}

	options, err := svc.GetTeamTaskOptionsFor(ctx, taskB.ID)
	if err != nil {
		t.Fatalf("GetTeamTaskOptionsFor: %v", err)
	}
	if len(options) != 3 {
		t.Fatalf("expected all 3 tasks present (unavailable ones NOT excluded), got %d: %+v", len(options), options)
	}

	byRole := make(map[uuid.UUID]TeamTaskOption, len(options))
	for _, o := range options {
		byRole[o.RoleID] = o
	}

	selfOpt := byRole[roleB]
	if !selfOpt.Unavailable || selfOpt.UnavailableReason == "" {
		t.Errorf("B (self) must be marked unavailable with a reason, got %+v", selfOpt)
	}
	cycleOpt := byRole[roleA]
	if !cycleOpt.Unavailable || cycleOpt.UnavailableReason == "" {
		t.Errorf("A must be marked unavailable (already waits for B — selecting it for B would close a cycle), got %+v", cycleOpt)
	}
	freeOpt := byRole[roleC]
	if freeOpt.Unavailable {
		t.Errorf("C must remain available (unrelated to the A/B edge), got %+v", freeOpt)
	}
}

// TestGetTeamTaskOptionsFor_RejectsParentTask проверяет отказ
// ErrStartConstraintOnParent при запросе для родительской (стори/эпик)
// задачи.
func TestGetTeamTaskOptionsFor_RejectsParentTask(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	feID := uuid.New()
	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})
	f.addRole(&domain.Role{ID: feID, Name: "FE разработчик"})
	f.roleScores[epicID] = []domain.EpicRoleScore{{EpicID: epicID, RoleID: feID, WeightedAvg: 1.0}}

	tasks, err := svc.GenerateTasksForEpic(ctx, epicID, date(2026, 7, 13))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parent := tasks[0]

	_, err = svc.GetTeamTaskOptionsFor(ctx, parent.ID)
	if !errors.Is(err, ErrStartConstraintOnParent) {
		t.Errorf("err = %v, want ErrStartConstraintOnParent", err)
	}
}

// TestWouldCreateCycle_DirectAndChainAndSelf — прямые юнит-тесты чистой
// функции wouldCreateCycle (задача 2.5), без прохода через сервис.
func TestWouldCreateCycle_DirectAndChainAndSelf(t *testing.T) {
	a := assignmentKey{epicID: uuid.New(), roleID: uuid.New()}
	b := assignmentKey{epicID: uuid.New(), roleID: uuid.New()}
	c := assignmentKey{epicID: uuid.New(), roleID: uuid.New()}
	d := assignmentKey{epicID: uuid.New(), roleID: uuid.New()}

	mkAssignment := func(from, to assignmentKey) domain.TaskAssignment {
		story, role := to.epicID, to.roleID
		return domain.TaskAssignment{EpicID: from.epicID, RoleID: from.roleID, WaitForStoryID: &story, WaitForRoleID: &role}
	}

	t.Run("no edges at all", func(t *testing.T) {
		if wouldCreateCycle(map[assignmentKey]domain.TaskAssignment{}, a, b) {
			t.Errorf("expected no cycle with an empty graph")
		}
	})

	t.Run("self reference", func(t *testing.T) {
		if !wouldCreateCycle(map[assignmentKey]domain.TaskAssignment{}, a, a) {
			t.Errorf("expected a self-reference to always be a cycle")
		}
	})

	t.Run("direct cycle: a waits b, b tries to wait a", func(t *testing.T) {
		// Существующее ребро: a ждёт b (assignments[a] указывает на b).
		// Предложение "b ждёт a" в терминах SetTaskStartConstraint —
		// ownKey=b, waitForKey=a, и вызов идёт как
		// wouldCreateCycle(assignments, start=waitForKey, target=ownKey).
		assignments := map[assignmentKey]domain.TaskAssignment{a: mkAssignment(a, b)}
		if !wouldCreateCycle(assignments, a, b) {
			t.Errorf("expected wouldCreateCycle(start=a, target=b) to detect the direct cycle")
		}
	})

	t.Run("chain cycle: a waits b, b waits c, c tries to wait a", func(t *testing.T) {
		assignments := map[assignmentKey]domain.TaskAssignment{
			a: mkAssignment(a, b),
			b: mkAssignment(b, c),
		}
		if !wouldCreateCycle(assignments, a, c) {
			t.Errorf("expected wouldCreateCycle(start=a, target=c) to detect the chain cycle")
		}
	})

	t.Run("unrelated chain is not a cycle", func(t *testing.T) {
		assignments := map[assignmentKey]domain.TaskAssignment{
			a: mkAssignment(a, b),
		}
		if wouldCreateCycle(assignments, c, d) {
			t.Errorf("expected no cycle between two unrelated nodes")
		}
	})

	t.Run("does not hang on a pre-existing cycle unrelated to the query", func(t *testing.T) {
		// c <-> d уже образуют цикл сами по себе (данные испорчены в обход
		// проверки) — запрос про a/b не должен зависнуть, гуляя по нему.
		assignments := map[assignmentKey]domain.TaskAssignment{
			c: mkAssignment(c, d),
			d: mkAssignment(d, c),
		}
		if wouldCreateCycle(assignments, c, a) {
			t.Errorf("expected no cycle: c's chain never reaches a")
		}
	})
}

// TestCountWaitForEdges_CountsOnlyCompleteReferences проверяет, что
// countWaitForEdges считает только строки с ОБОИМИ полями ссылки заданными.
func TestCountWaitForEdges_CountsOnlyCompleteReferences(t *testing.T) {
	complete := assignmentKey{epicID: uuid.New(), roleID: uuid.New()}
	incomplete := assignmentKey{epicID: uuid.New(), roleID: uuid.New()}
	none := assignmentKey{epicID: uuid.New(), roleID: uuid.New()}
	storyID, roleID := uuid.New(), uuid.New()

	assignments := map[assignmentKey]domain.TaskAssignment{
		complete:   {EpicID: complete.epicID, RoleID: complete.roleID, WaitForStoryID: &storyID, WaitForRoleID: &roleID},
		incomplete: {EpicID: incomplete.epicID, RoleID: incomplete.roleID, WaitForStoryID: &storyID},
		none:       {EpicID: none.epicID, RoleID: none.roleID, StartOffsetDays: 5},
	}

	if got := countWaitForEdges(assignments); got != 1 {
		t.Errorf("countWaitForEdges = %d, want 1", got)
	}
}

// TestRecalculateTeamSchedule_NoWaitForEdges_ConvergesInOnePassWhenStable
// измеряет задачу 2.2 ("замер стоимости повторного прохода"): без единой
// ссылки "не ранее задачи" повторный пересчёт уже стабильного расписания
// останавливается сразу после первого прохода — той же ценой, что и до
// добавления фичи (никаких дополнительных обращений к
// GetGanttTasksByEpicID).
func TestRecalculateTeamSchedule_NoWaitForEdges_ConvergesInOnePassWhenStable(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	feID := uuid.New()
	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})
	f.addRole(&domain.Role{ID: feID, Name: "FE разработчик"})
	f.roleScores[epicID] = []domain.EpicRoleScore{{EpicID: epicID, RoleID: feID, WeightedAvg: 1.0}}

	if _, err := svc.GenerateTasksForEpic(ctx, epicID, date(2026, 7, 13)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Расписание уже стабильно — повторный вызов не должен ничего менять
	// и не должен перечитывать задачи эпика между проходами (только 1 раз,
	// как часть обычного сбора inScope).
	f.getGanttTasksByEpicIDCalls = 0
	if _, err := svc.RecalculateTeamSchedule(ctx, teamID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.getGanttTasksByEpicIDCalls != 1 {
		t.Errorf("GetGanttTasksByEpicID calls = %d, want exactly 1 (no extra passes without wait-for edges)",
			f.getGanttTasksByEpicIDCalls)
	}
}

// TestRecalculateTeamSchedule_WaitForEdges_CostsAtLeastOneExtraPass измеряет
// стоимость повторного прохода, когда ссылка есть: минимум 2 полных прохода
// (2 обращения к GetGanttTasksByEpicID для единственного в области видимости
// эпика — по одному на инициализацию inScope и ровно одно обновление между
// проходом 0 и проверочным проходом 1).
func TestRecalculateTeamSchedule_WaitForEdges_CostsAtLeastOneExtraPass(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicAID := uuid.New()
	epicBID := uuid.New()
	beID := uuid.New()
	feID := uuid.New()

	soA, soB := 1, 2
	f.addEpic(&domain.Epic{ID: epicAID, Number: "E-1", Name: "Epic A", TeamID: teamID, SortOrder: &soA})
	f.addEpic(&domain.Epic{ID: epicBID, Number: "E-2", Name: "Epic B", TeamID: teamID, SortOrder: &soB})
	f.addRole(&domain.Role{ID: beID, Name: "BE разработчик"})
	f.addRole(&domain.Role{ID: feID, Name: "FE разработчик"})
	f.roleScores[epicAID] = []domain.EpicRoleScore{{EpicID: epicAID, RoleID: beID, WeightedAvg: 1.0}}
	f.roleScores[epicBID] = []domain.EpicRoleScore{{EpicID: epicBID, RoleID: feID, WeightedAvg: 1.0}}

	if _, err := svc.GenerateTasksForEpic(ctx, epicAID, date(2026, 7, 13)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := svc.GenerateTasksForEpic(ctx, epicBID, date(2026, 7, 13)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var beTask *domain.GanttTask
	for _, task := range f.tasks {
		if task.Name == "BE разработчик" {
			beTask = task
		}
	}

	if _, err := svc.SetTaskStartConstraint(ctx, beTask.ID, nil, &epicBID, &feID); err != nil {
		t.Fatalf("SetTaskStartConstraint: %v", err)
	}

	// Расписание с constraint уже стабильно (SetTaskStartConstraint выше
	// само пересчитало его). Повторный вызов на УЖЕ стабильном состоянии
	// с присутствующей ссылкой всё равно обязан пройти минимум 2 полных
	// прохода (design.md Решение 2: без топологической сортировки
	// надёжность важнее экономии на уже разрешённых ссылках) — 2 эпика в
	// области видимости, 1 обновление между проходом 0 и проверочным
	// проходом 1 -> 2*1=2 дополнительных вызова GetGanttTasksByEpicID.
	f.getGanttTasksByEpicIDCalls = 0
	if _, err := svc.RecalculateTeamSchedule(ctx, teamID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.getGanttTasksByEpicIDCalls != 2 {
		t.Errorf("GetGanttTasksByEpicID calls = %d, want 2 (2 epics x 1 refresh before the mandatory verification pass)",
			f.getGanttTasksByEpicIDCalls)
	}
}
