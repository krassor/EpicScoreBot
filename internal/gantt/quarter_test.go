package gantt

import (
	"EpicScoreBot/internal/models/domain"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// addScoredEpicWithRole — вспомогательная функция теста: создаёт заскоренный
// эпик за указанный год/квартал с одной ролевой оценкой (без сторей,
// legacy-раскладка), чтобы GenerateTasksForQuarter могла его перегенерировать.
func addScoredEpicWithRole(f *fakeRepo, teamID, roleID uuid.UUID, number string, year, quarter, sortOrder int) *domain.Epic {
	epicID := uuid.New()
	so := sortOrder
	epic := f.addEpic(&domain.Epic{
		ID:        epicID,
		Number:    number,
		Name:      "Epic " + number,
		TeamID:    teamID,
		Year:      year,
		Quarter:   quarter,
		Status:    domain.StatusScored,
		SortOrder: &so,
	})
	f.roleScores[epicID] = []domain.EpicRoleScore{
		{EpicID: epicID, RoleID: roleID, WeightedAvg: 2.0},
	}
	return epic
}

// TestGenerateTasksForQuarter_RecalculatesOnce проверяет, что при
// перегенерации нескольких эпиков квартала пересчёт расписания команды
// (RecalculateTeamSchedule) выполняется ровно один раз, а не по разу на
// каждый эпик.
func TestGenerateTasksForQuarter_RecalculatesOnce(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	roleID := uuid.New()
	f.addRole(&domain.Role{ID: roleID, Name: "Аналитик"})

	addScoredEpicWithRole(f, teamID, roleID, "E-1", 2026, 3, 1)
	addScoredEpicWithRole(f, teamID, roleID, "E-2", 2026, 3, 2)
	addScoredEpicWithRole(f, teamID, roleID, "E-3", 2026, 3, 3)

	start := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC) // Monday

	result, err := svc.GenerateTasksForQuarter(ctx, teamID, 2026, 3, start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.EpicsTotal != 3 {
		t.Errorf("EpicsTotal = %d, want 3", result.EpicsTotal)
	}
	if result.EpicsRegenerated != 3 {
		t.Errorf("EpicsRegenerated = %d, want 3", result.EpicsRegenerated)
	}
	if result.EpicsFailed != 0 {
		t.Errorf("EpicsFailed = %d, want 0", result.EpicsFailed)
	}
	if result.TasksCount != len(f.tasks) {
		t.Errorf("TasksCount = %d, want %d (all tasks in fake repo)", result.TasksCount, len(f.tasks))
	}

	// 1 родительская + 1 ролевая задача на эпик, 3 эпика.
	if len(f.tasks) != 6 {
		t.Fatalf("expected 6 tasks total, got %d", len(f.tasks))
	}

	if f.getGanttTasksByTeamIDCalls != 1 {
		t.Errorf("RecalculateTeamSchedule calls (via GetGanttTasksByTeamID) = %d, want exactly 1", f.getGanttTasksByTeamIDCalls)
	}
}

// TestGenerateTasksForQuarter_SkipsUnscoredAndOtherPeriods проверяет, что
// эпики в статусах NEW/SCORING, а также эпики других год/квартал не
// затрагиваются перегенерацией — их задачи не удаляются и не пересоздаются.
func TestGenerateTasksForQuarter_SkipsUnscoredAndOtherPeriods(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	roleID := uuid.New()
	f.addRole(&domain.Role{ID: roleID, Name: "Аналитик"})

	// Целевой квартал: один заскоренный эпик — должен быть перегенерирован.
	target := addScoredEpicWithRole(f, teamID, roleID, "E-1", 2026, 3, 1)

	// Тот же период, но не заскорен — не должен попасть в операцию.
	scoringEpicID := uuid.New()
	so2 := 2
	f.addEpic(&domain.Epic{
		ID: scoringEpicID, Number: "E-2", Name: "Epic E-2", TeamID: teamID,
		Year: 2026, Quarter: 3, Status: domain.StatusScoring, SortOrder: &so2,
	})
	f.roleScores[scoringEpicID] = []domain.EpicRoleScore{
		{EpicID: scoringEpicID, RoleID: roleID, WeightedAvg: 2.0},
	}
	// У этого эпика уже есть сгенерированные задачи (как будто до перехода
	// обратно в SCORING) — они не должны быть тронуты.
	start := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	if _, err := svc.GenerateTasksForEpic(ctx, scoringEpicID, start); err != nil {
		t.Fatalf("setup: unexpected error generating scoring epic tasks: %v", err)
	}
	var scoringTaskIDsBefore []uuid.UUID
	for id, task := range f.tasks {
		if task.EpicID == scoringEpicID {
			scoringTaskIDsBefore = append(scoringTaskIDsBefore, id)
		}
	}
	if len(scoringTaskIDsBefore) == 0 {
		t.Fatalf("setup: expected scoring epic to have generated tasks")
	}

	// Заскоренный эпик другого квартала — тоже не должен быть тронут.
	otherQuarterEpic := addScoredEpicWithRole(f, teamID, roleID, "E-3", 2026, 4, 3)
	if _, err := svc.GenerateTasksForEpic(ctx, otherQuarterEpic.ID, start); err != nil {
		t.Fatalf("setup: unexpected error generating other-quarter epic tasks: %v", err)
	}
	var otherQuarterTaskIDsBefore []uuid.UUID
	for id, task := range f.tasks {
		if task.EpicID == otherQuarterEpic.ID {
			otherQuarterTaskIDsBefore = append(otherQuarterTaskIDsBefore, id)
		}
	}

	f.getGanttTasksByTeamIDCalls = 0 // сбрасываем счётчик после setup-генераций выше

	result, err := svc.GenerateTasksForQuarter(ctx, teamID, 2026, 3, start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.EpicsTotal != 1 || result.EpicsRegenerated != 1 {
		t.Errorf("expected exactly 1 selected/regenerated epic, got total=%d regenerated=%d",
			result.EpicsTotal, result.EpicsRegenerated)
	}

	// Целевой эпик перегенерирован — задачи существуют.
	hasTasks, err := f.HasGanttTasksForEpic(ctx, target.ID)
	if err != nil || !hasTasks {
		t.Errorf("expected target epic to have regenerated tasks, hasTasks=%v err=%v", hasTasks, err)
	}

	// Задачи SCORING-эпика не изменились (те же ID).
	var scoringTaskIDsAfter []uuid.UUID
	for id, task := range f.tasks {
		if task.EpicID == scoringEpicID {
			scoringTaskIDsAfter = append(scoringTaskIDsAfter, id)
		}
	}
	if len(scoringTaskIDsAfter) != len(scoringTaskIDsBefore) {
		t.Errorf("scoring epic task count changed: before=%d after=%d",
			len(scoringTaskIDsBefore), len(scoringTaskIDsAfter))
	}

	// Задачи эпика другого квартала не удалены (те же ID тоже сохраняются,
	// хотя их даты могли измениться в рамках общего пересчёта расписания).
	var otherQuarterTaskIDsAfter []uuid.UUID
	for id, task := range f.tasks {
		if task.EpicID == otherQuarterEpic.ID {
			otherQuarterTaskIDsAfter = append(otherQuarterTaskIDsAfter, id)
		}
	}
	if len(otherQuarterTaskIDsAfter) != len(otherQuarterTaskIDsBefore) {
		t.Errorf("other-quarter epic task count changed: before=%d after=%d",
			len(otherQuarterTaskIDsBefore), len(otherQuarterTaskIDsAfter))
	}
}

// TestGenerateTasksForQuarter_PartialFailure проверяет, что ошибка генерации
// одного из отобранных эпиков не прерывает обработку остальных: они
// перегенерируются успешно, а счётчик неуспешных равен количеству упавших.
func TestGenerateTasksForQuarter_PartialFailure(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	roleID := uuid.New()
	f.addRole(&domain.Role{ID: roleID, Name: "Аналитик"})

	ok1 := addScoredEpicWithRole(f, teamID, roleID, "E-1", 2026, 3, 1)
	ok2 := addScoredEpicWithRole(f, teamID, roleID, "E-2", 2026, 3, 2)

	// Заскоренный, но без ролевых оценок — legacy-эпик, для которого
	// buildRoleTasks вернёт пустой список, что приведёт к ошибке генерации.
	brokenID := uuid.New()
	so3 := 3
	f.addEpic(&domain.Epic{
		ID: brokenID, Number: "E-3", Name: "Epic E-3", TeamID: teamID,
		Year: 2026, Quarter: 3, Status: domain.StatusScored, SortOrder: &so3,
	})

	start := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)

	result, err := svc.GenerateTasksForQuarter(ctx, teamID, 2026, 3, start)
	if err != nil {
		t.Fatalf("unexpected top-level error: %v", err)
	}

	if result.EpicsTotal != 3 {
		t.Errorf("EpicsTotal = %d, want 3", result.EpicsTotal)
	}
	if result.EpicsRegenerated != 2 {
		t.Errorf("EpicsRegenerated = %d, want 2", result.EpicsRegenerated)
	}
	if result.EpicsFailed != 1 {
		t.Errorf("EpicsFailed = %d, want 1", result.EpicsFailed)
	}

	for _, e := range []*domain.Epic{ok1, ok2} {
		hasTasks, err := f.HasGanttTasksForEpic(ctx, e.ID)
		if err != nil || !hasTasks {
			t.Errorf("expected epic %s to have regenerated tasks, hasTasks=%v err=%v", e.Number, hasTasks, err)
		}
	}

	// generateTaskRowsForEpic создаёт родительскую задачу эпика до того, как
	// узнаёт об отсутствии ролевых оценок (та же особенность существующего
	// GenerateTasksForEpic, не вводится этим изменением) — поэтому для
	// упавшего эпика допустима родительская задача-заглушка, но не должно
	// быть ни одной дочерней (ролевой) задачи.
	for _, task := range f.tasks {
		if task.EpicID == brokenID && !task.IsParent {
			t.Errorf("broken epic should not have any child role tasks, found: %+v", task)
		}
	}

	// Пересчёт всё равно должен был произойти ровно один раз, т.к.
	// хотя бы один эпик обработан успешно.
	if f.getGanttTasksByTeamIDCalls != 1 {
		t.Errorf("RecalculateTeamSchedule calls = %d, want exactly 1", f.getGanttTasksByTeamIDCalls)
	}
}

// TestGenerateTasksForQuarter_NoEligibleEpics проверяет, что при отсутствии
// подходящих (заскоренных) эпиков квартала задачи не изменяются и пересчёт
// расписания не запускается.
func TestGenerateTasksForQuarter_NoEligibleEpics(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	roleID := uuid.New()
	f.addRole(&domain.Role{ID: roleID, Name: "Аналитик"})

	// Эпик есть, но не в целевом квартале.
	addScoredEpicWithRole(f, teamID, roleID, "E-1", 2026, 4, 1)

	start := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)

	result, err := svc.GenerateTasksForQuarter(ctx, teamID, 2026, 3, start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.EpicsTotal != 0 || result.EpicsRegenerated != 0 || result.EpicsFailed != 0 || result.TasksCount != 0 {
		t.Errorf("expected all-zero result, got %+v", result)
	}

	if len(f.tasks) != 0 {
		t.Errorf("expected no tasks to be created, got %d", len(f.tasks))
	}

	if f.getGanttTasksByTeamIDCalls != 0 {
		t.Errorf("RecalculateTeamSchedule must not be called when there are no eligible epics, calls=%d",
			f.getGanttTasksByTeamIDCalls)
	}
}

// TestGenerateTasksForQuarter_PreservesEpicOrder проверяет, что эпики
// сохраняют свой порядок в очереди команды (sort_order) после перегенерации
// задач квартала. Это критично для предсказуемости диаграммы Ганта.
func TestGenerateTasksForQuarter_PreservesEpicOrder(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	roleID := uuid.New()
	f.addRole(&domain.Role{ID: roleID, Name: "Аналитик"})

	// Создаём три эпика с разными sort_order-ами.
	epic1 := addScoredEpicWithRole(f, teamID, roleID, "E-1", 2026, 3, 10)
	epic2 := addScoredEpicWithRole(f, teamID, roleID, "E-2", 2026, 3, 20)
	epic3 := addScoredEpicWithRole(f, teamID, roleID, "E-3", 2026, 3, 30)

	// GetTeamEpicsOrdered должна вернуть их в порядке sort_order,
	// а GenerateTasksForQuarter должна сохранить этот порядок при обработке.
	epicsBeforeReorder := f.epics[epic1.ID].SortOrder
	if epicsBeforeReorder == nil || *epicsBeforeReorder != 10 {
		t.Fatalf("setup: epic1 sort_order expected 10, got %v", epicsBeforeReorder)
	}

	start := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)

	result, err := svc.GenerateTasksForQuarter(ctx, teamID, 2026, 3, start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.EpicsTotal != 3 || result.EpicsRegenerated != 3 {
		t.Errorf("expected 3 epics, got total=%d regenerated=%d",
			result.EpicsTotal, result.EpicsRegenerated)
	}

	// Проверяем, что sort_order-ы остались неизменными.
	if f.epics[epic1.ID].SortOrder == nil || *f.epics[epic1.ID].SortOrder != 10 {
		t.Errorf("epic1 sort_order changed: expected 10, got %v",
			f.epics[epic1.ID].SortOrder)
	}
	if f.epics[epic2.ID].SortOrder == nil || *f.epics[epic2.ID].SortOrder != 20 {
		t.Errorf("epic2 sort_order changed: expected 20, got %v",
			f.epics[epic2.ID].SortOrder)
	}
	if f.epics[epic3.ID].SortOrder == nil || *f.epics[epic3.ID].SortOrder != 30 {
		t.Errorf("epic3 sort_order changed: expected 30, got %v",
			f.epics[epic3.ID].SortOrder)
	}
}

// TestGenerateTasksForQuarter_AllEpicsFail проверяет, что если все отобранные
// эпики упадут при генерации, пересчёт расписания не запускается (т.к.
// EpicsRegenerated == 0). Счётчик EpicsTotal и EpicsFailed корректны.
func TestGenerateTasksForQuarter_AllEpicsFail(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()

	// Создаём два заскоренных эпика БЕЗ ролевых оценок (legacy, будут ошибки).
	so1 := 1
	f.addEpic(&domain.Epic{
		ID:        uuid.New(),
		Number:    "E-1",
		Name:      "Epic E-1",
		TeamID:    teamID,
		Year:      2026,
		Quarter:   3,
		Status:    domain.StatusScored,
		SortOrder: &so1,
	})

	so2 := 2
	f.addEpic(&domain.Epic{
		ID:        uuid.New(),
		Number:    "E-2",
		Name:      "Epic E-2",
		TeamID:    teamID,
		Year:      2026,
		Quarter:   3,
		Status:    domain.StatusScored,
		SortOrder: &so2,
	})

	start := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)

	result, err := svc.GenerateTasksForQuarter(ctx, teamID, 2026, 3, start)
	if err != nil {
		t.Fatalf("unexpected top-level error: %v", err)
	}

	if result.EpicsTotal != 2 {
		t.Errorf("EpicsTotal = %d, want 2", result.EpicsTotal)
	}
	if result.EpicsRegenerated != 0 {
		t.Errorf("EpicsRegenerated = %d, want 0", result.EpicsRegenerated)
	}
	if result.EpicsFailed != 2 {
		t.Errorf("EpicsFailed = %d, want 2", result.EpicsFailed)
	}
	if result.TasksCount != 0 {
		t.Errorf("TasksCount = %d, want 0", result.TasksCount)
	}

	// RecalculateTeamSchedule НЕ должен быть вызван, т.к. ни один эпик
	// не был успешно обработан.
	if f.getGanttTasksByTeamIDCalls != 0 {
		t.Errorf("RecalculateTeamSchedule must not be called when all epics fail, calls=%d",
			f.getGanttTasksByTeamIDCalls)
	}
}
