package gantt

import (
	"EpicScoreBot/internal/models/domain"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Тесты задачи 1.1 (openspec/changes/fix-team-floor-ratchet/tasks.md) —
// воспроизведение дефекта teamFloor-храповика ДО каких-либо правок
// продуктового кода. См. design.md: teamFloor вычисляется как минимум дат
// старта родительских баров эпиков команды (ganttService.go:575-582), но эти
// даты записывает сам планировщик на предыдущем прогоне — самоссылка,
// которая не даёт расписанию вернуться назад после снятия причины сдвига.
//
// Важно (design.md Решение 3): ловит дефект только пара «сдвинуть — снять
// причину — проверить возврат», а не однократная проверка дат после одного
// пересчёта — та проходит и на текущем (сломанном) коде. Поэтому тест ниже
// специально фиксирует ДВА состояния: baseline (до сдвига) и результат ПОСЛЕ
// того, как причина сдвига снята, — и сравнивает их между собой, а не с
// заранее вычисленной датой.

// TestRecalculateTeamSchedule_SingleEpicTeam_FloorRatchet_ReproducesDefect
// воспроизводит дефект на команде с ЕДИНСТВЕННЫМ эпиком (design.md: "У
// команды с одним эпиком проявляется всегда") — без анкерного эпика, который
// заявка удалит по итогам этой диагностики (design.md Решение 4).
//
// Сценарий: сгенерировать задачи единственного эпика команды, запомнить
// базовые даты, сдвинуть расписание вперёд ручным смещением старта
// (SetTaskStartOffset), убедиться, что сдвиг действительно произошёл, затем
// СНЯТЬ смещение (offsetDays=0) и пересчитать. На корректно работающем
// планировщике даты обязаны вернуться к baseline — на текущем коде они
// остаются на сдвинутой границе, потому что teamFloor уже "запомнил" сдвиг
// через дату родительского бара эпика, а не через что-то, не зависящее от
// результата предыдущего пересчёта.
func TestRecalculateTeamSchedule_SingleEpicTeam_FloorRatchet_ReproducesDefect(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	feID := uuid.New()

	// Единственный эпик команды, без сторей (legacy flat gantt) — одна
	// ролевая задача, чтобы сдвиг её старта сдвигал старт родительского
	// бара эпика напрямую, без маскировки другими эпиками/ролями.
	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})
	f.addRole(&domain.Role{ID: feID, Name: "FE разработчик"})
	f.roleScores[epicID] = []domain.EpicRoleScore{
		{EpicID: epicID, RoleID: feID, WeightedAvg: 2.0},
	}

	start := date(2026, 7, 13) // Monday
	if _, err := svc.GenerateTasksForEpic(ctx, epicID, start); err != nil {
		t.Fatalf("unexpected error (generate): %v", err)
	}

	var epicParentTask *domain.GanttTask
	for _, task := range f.tasks {
		if task.EpicID == epicID && task.IsParent && task.ParentTaskID == nil {
			epicParentTask = task
		}
	}
	if epicParentTask == nil {
		t.Fatalf("expected epic parent task to exist")
	}
	fe := findTaskByParentAndName(f, epicParentTask.ID, "FE разработчик")
	if fe == nil {
		t.Fatalf("expected FE task to exist")
	}

	baselineParentStart := epicParentTask.StartDate
	baselineFEStart := fe.StartDate
	baselineFEEnd := fe.EndDate

	// Сдвигаем расписание вперёд ручным смещением старта — одна из причин
	// сдвига, перечисленных в proposal.md ("снять ручное смещение старта,
	// вернуть эпик выше в очереди, отменить ограничение").
	if _, err := svc.SetTaskStartOffset(ctx, fe.ID, 10); err != nil {
		t.Fatalf("SetTaskStartOffset (set): %v", err)
	}

	shiftedFE := f.tasks[fe.ID]
	if shiftedFE.StartDate.Equal(baselineFEStart) {
		t.Fatalf("precondition failed: start offset должен был сдвинуть FE вперёд")
	}
	var shiftedParent *domain.GanttTask
	for _, task := range f.tasks {
		if task.ID == epicParentTask.ID {
			shiftedParent = task
		}
	}
	if shiftedParent == nil || shiftedParent.StartDate.Equal(baselineParentStart) {
		t.Fatalf("precondition failed: родительский бар эпика должен был сдвинуться вперёд вместе с FE")
	}

	// Снимаем причину сдвига — возвращаем смещение к нулю — и пересчитываем.
	if _, err := svc.SetTaskStartOffset(ctx, fe.ID, 0); err != nil {
		t.Fatalf("SetTaskStartOffset (clear): %v", err)
	}

	feAfter := f.tasks[fe.ID]
	if !feAfter.StartDate.Equal(baselineFEStart) {
		t.Errorf("FloorRatchet: FE start после снятия смещения = %v, хотим %v (возврат к baseline) — "+
			"граница не должна была запомнить сдвинутую дату родительского бара",
			feAfter.StartDate, baselineFEStart)
	}
	if !feAfter.EndDate.Equal(baselineFEEnd) {
		t.Errorf("FloorRatchet: FE end после снятия смещения = %v, хотим %v (возврат к baseline)",
			feAfter.EndDate, baselineFEEnd)
	}

	var parentAfter *domain.GanttTask
	for _, task := range f.tasks {
		if task.ID == epicParentTask.ID {
			parentAfter = task
		}
	}
	if parentAfter == nil || !parentAfter.StartDate.Equal(baselineParentStart) {
		t.Errorf("FloorRatchet: родительский бар эпика после снятия смещения = %v, хотим %v (возврат к baseline)",
			parentAfter.StartDate, baselineParentStart)
	}
}

// TestRecalculateTeamSchedule_LegacyEpicWithoutPlanningStartDate_UsesOldFormula
// проверяет задачу 2.2 (design.md Решение 2): у эпика, чья родительская
// строка создана ДО миграции 014_gantt_planning_start_date (симулируется
// прямым обнулением PlanningStartDate в фейковом репозитории — так же, как
// эта колонка окажется NULL у всех существующих строк сразу после выкатки),
// граница команды продолжает считаться по прежней формуле — из t.StartDate
// этой же строки, — и это поведение детерминировано: не зависит от того,
// какие ещё эпики есть в команде (design.md: "неприемлемо — граница,
// зависящая от того, какие эпики попали в выборку").
func TestRecalculateTeamSchedule_LegacyEpicWithoutPlanningStartDate_UsesOldFormula(t *testing.T) {
	ctx := context.Background()

	// run генерирует "легаси"-эпик (без сохранённой даты, с уже
	// накопленным по старой формуле сдвигом — Jul20 вместо исходного
	// Jul13) и, опционально, второй, полноценный эпик команды, стартующий
	// заметно позже, чтобы он не мог стать новым минимумом. Возвращает
	// итоговый старт листовой задачи легаси-эпика.
	run := func(t *testing.T, withSibling bool) time.Time {
		t.Helper()
		f := newFakeRepo()
		svc := New(newTestLogger(), f)

		teamID := uuid.New()
		legacyEpicID := uuid.New()
		feID := uuid.New()

		f.addEpic(&domain.Epic{ID: legacyEpicID, Number: "E-1", Name: "Legacy", TeamID: teamID})
		f.addRole(&domain.Role{ID: feID, Name: "FE разработчик"})
		f.roleScores[legacyEpicID] = []domain.EpicRoleScore{
			{EpicID: legacyEpicID, RoleID: feID, WeightedAvg: 1.0},
		}

		if _, err := svc.GenerateTasksForEpic(ctx, legacyEpicID, date(2026, 7, 13)); err != nil {
			t.Fatalf("unexpected error (legacy generate): %v", err)
		}

		var legacyParent *domain.GanttTask
		for _, task := range f.tasks {
			if task.EpicID == legacyEpicID && task.IsParent && task.ParentTaskID == nil {
				legacyParent = task
			}
		}
		if legacyParent == nil {
			t.Fatalf("expected legacy parent task to exist")
		}

		// Симулируем строку, существовавшую на момент миграции: сохранённой
		// даты нет (PlanningStartDate == nil), а StartDate уже отражает
		// накопленный по старой формуле сдвиг — ровно то состояние, в
		// котором окажутся реальные эпики в БД сразу после выкатки
		// (design.md Решение 2: бэкафилл намеренно не делается).
		legacyDrifted := date(2026, 7, 20)
		f.tasks[legacyParent.ID].PlanningStartDate = nil
		f.tasks[legacyParent.ID].StartDate = legacyDrifted
		f.tasks[legacyParent.ID].EndDate = legacyDrifted

		if withSibling {
			siblingEpicID := uuid.New()
			f.addEpic(&domain.Epic{ID: siblingEpicID, Number: "E-2", Name: "Sibling", TeamID: teamID})
			f.roleScores[siblingEpicID] = []domain.EpicRoleScore{
				{EpicID: siblingEpicID, RoleID: feID, WeightedAvg: 1.0},
			}
			if _, err := svc.GenerateTasksForEpic(ctx, siblingEpicID, date(2026, 9, 1)); err != nil {
				t.Fatalf("unexpected error (sibling generate): %v", err)
			}
		}

		result, err := svc.RecalculateTeamSchedule(ctx, teamID)
		if err != nil {
			t.Fatalf("RecalculateTeamSchedule: %v", err)
		}

		var legacyFE *domain.GanttTask
		for i := range result {
			if result[i].EpicID == legacyEpicID && !result[i].IsParent {
				legacyFE = &result[i]
			}
		}
		if legacyFE == nil {
			t.Fatalf("expected legacy FE task in result")
		}
		return legacyFE.StartDate
	}

	alone := run(t, false)
	withSibling := run(t, true)

	wantFallback := moveToWorkDay(date(2026, 7, 20))
	if !alone.Equal(wantFallback) {
		t.Errorf("легаси-эпик (один в команде): FE start = %v, хотим %v (fallback на StartDate по старой формуле)",
			alone, wantFallback)
	}
	if !withSibling.Equal(wantFallback) {
		t.Errorf("легаси-эпик (рядом со вторым эпиком): FE start = %v, хотим %v (fallback на StartDate по старой формуле)",
			withSibling, wantFallback)
	}
	if !alone.Equal(withSibling) {
		t.Errorf("вклад легаси-эпика в границу зависит от состава команды: один = %v, со вторым эпиком = %v — должны совпадать (design.md Решение 2)",
			alone, withSibling)
	}
}

// TestRecalculateTeamSchedule_FloorRatchet_ReversibilityByReordering проверяет
// задачу 3.2 в части изменения порядка эпиков (SortOrder).
//
// ВАЖНО про силу этого теста: он проходит и на храповике — проверено прямой
// поломкой формулы границы. Это не недосмотр, а свойство сценария. Граница —
// минимум по родительским барам команды, а эпик с наивысшим приоритетом всегда
// стартует ровно на границе. Перестановка меняет, КАКОЙ эпик стоит первым, но
// кто-то первым остаётся всегда, поэтому минимум сдвинуться вперёд не может:
// изменением порядка храповик не заводится в принципе.
//
// Поэтому тест здесь — сторож от регрессии (перестановка и возврат не должны
// менять даты), а не воспроизведение дефекта. Воспроизводят его два сценария,
// где сдвигается единственный эпик команды и потому двигается сам минимум:
// ReproducesDefect (ручное смещение старта) и ReversibilityByStartConstraint
// (ограничение «Начать не ранее»). Оба падают при сломанной формуле.
func TestRecalculateTeamSchedule_FloorRatchet_ReversibilityByReordering(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epic1ID := uuid.New()
	epic2ID := uuid.New()
	feID := uuid.New()

	so1, so2 := 1, 2
	f.addEpic(&domain.Epic{ID: epic1ID, Number: "E-1", Name: "Epic1", TeamID: teamID, SortOrder: &so1})
	f.addEpic(&domain.Epic{ID: epic2ID, Number: "E-2", Name: "Epic2", TeamID: teamID, SortOrder: &so2})
	f.addRole(&domain.Role{ID: feID, Name: "FE разработчик"})

	f.roleScores[epic1ID] = []domain.EpicRoleScore{{EpicID: epic1ID, RoleID: feID, WeightedAvg: 2.0}}
	f.roleScores[epic2ID] = []domain.EpicRoleScore{{EpicID: epic2ID, RoleID: feID, WeightedAvg: 2.0}}

	start := date(2026, 7, 13)
	if _, err := svc.GenerateTasksForEpic(ctx, epic1ID, start); err != nil {
		t.Fatalf("generate epic1: %v", err)
	}
	if _, err := svc.GenerateTasksForEpic(ctx, epic2ID, start); err != nil {
		t.Fatalf("generate epic2: %v", err)
	}

	result1, err := svc.RecalculateTeamSchedule(ctx, teamID)
	if err != nil {
		t.Fatalf("recalculate (baseline): %v", err)
	}
	var feTaskID uuid.UUID
	for _, task := range result1 {
		if task.Name == "FE разработчик" && !task.IsParent {
			feTaskID = task.ID
			break
		}
	}
	if feTaskID == uuid.Nil {
		t.Fatalf("expected FE task to exist")
	}
	baselineFEStart := f.tasks[feTaskID].StartDate

	// Меняем порядок (SortOrder это int, не *int)
	for _, task := range f.tasks {
		if task.EpicID == epic1ID && task.IsParent && task.ParentTaskID == nil {
			task.SortOrder = 2
			f.tasks[task.ID] = task
		}
		if task.EpicID == epic2ID && task.IsParent && task.ParentTaskID == nil {
			task.SortOrder = 1
			f.tasks[task.ID] = task
		}
	}
	f.epics[epic1ID].SortOrder = &so2
	f.epics[epic2ID].SortOrder = &so1

	_, err = svc.RecalculateTeamSchedule(ctx, teamID)
	if err != nil {
		t.Fatalf("recalculate (reordered): %v", err)
	}

	// Возвращаем исходный порядок
	for _, task := range f.tasks {
		if task.EpicID == epic1ID && task.IsParent && task.ParentTaskID == nil {
			task.SortOrder = 1
			f.tasks[task.ID] = task
		}
		if task.EpicID == epic2ID && task.IsParent && task.ParentTaskID == nil {
			task.SortOrder = 2
			f.tasks[task.ID] = task
		}
	}
	f.epics[epic1ID].SortOrder = &so1
	f.epics[epic2ID].SortOrder = &so2

	_, err = svc.RecalculateTeamSchedule(ctx, teamID)
	if err != nil {
		t.Fatalf("recalculate (restored order): %v", err)
	}

	feTaskAfter := f.tasks[feTaskID]
	if !feTaskAfter.StartDate.Equal(baselineFEStart) {
		t.Errorf("FloorRatchet (reordering): FE start after restoring order = %v, want %v (baseline)",
			feTaskAfter.StartDate, baselineFEStart)
	}
}

// TestRecalculateTeamSchedule_FloorRatchet_ReversibilityByStartConstraint
// проверяет задачу 3.2: обратимость по второй причине сдвига — ограничению
// «Начать не ранее», а не только по ручному смещению старта.
//
// Команда намеренно с ОДНИМ эпиком и ОДНОЙ ролью. Второй эпик якорил бы
// границу (минимум берётся по всем эпикам команды), и тест прошёл бы даже
// на храповике — то есть не проверял бы ничего. Это тот же обход, который
// Решение 4 предписало убрать из тестов add-task-start-constraints.
// Одна роль нужна по той же причине: старт родительского бара равен минимуму
// по детям, поэтому вторая, неограниченная роль удержала бы бар на месте.
func TestRecalculateTeamSchedule_FloorRatchet_ReversibilityByStartConstraint(t *testing.T) {
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
	if _, err := svc.GenerateTasksForEpic(ctx, epicID, start); err != nil {
		t.Fatalf("generate: %v", err)
	}

	var parent *domain.GanttTask
	for _, task := range f.tasks {
		if task.EpicID == epicID && task.IsParent && task.ParentTaskID == nil {
			parent = task
		}
	}
	if parent == nil {
		t.Fatalf("expected epic parent task to exist")
	}
	fe := findTaskByParentAndName(f, parent.ID, "FE разработчик")
	if fe == nil {
		t.Fatalf("expected FE task to exist")
	}

	baselineParentStart := parent.StartDate
	baselineFEStart := fe.StartDate
	baselineFEEnd := fe.EndDate

	// Сдвигаем расписание вперёд ограничением «Начать не ранее».
	notBefore := date(2026, 7, 27)
	if _, err := svc.SetTaskStartConstraint(ctx, fe.ID, &notBefore, nil, nil); err != nil {
		t.Fatalf("set constraint: %v", err)
	}
	if f.tasks[fe.ID].StartDate.Equal(baselineFEStart) {
		t.Fatalf("precondition failed: ограничение должно было сдвинуть FE вперёд")
	}
	if f.tasks[parent.ID].StartDate.Equal(baselineParentStart) {
		t.Fatalf("precondition failed: родительский бар должен был сдвинуться вместе с FE")
	}

	// Снимаем причину сдвига.
	if _, err := svc.SetTaskStartConstraint(ctx, fe.ID, nil, nil, nil); err != nil {
		t.Fatalf("clear constraint: %v", err)
	}

	feAfter := f.tasks[fe.ID]
	if !feAfter.StartDate.Equal(baselineFEStart) {
		t.Errorf("FloorRatchet (constraint): FE start после снятия ограничения = %v, хотим %v (baseline)",
			feAfter.StartDate, baselineFEStart)
	}
	if !feAfter.EndDate.Equal(baselineFEEnd) {
		t.Errorf("FloorRatchet (constraint): FE end после снятия ограничения = %v, хотим %v (baseline)",
			feAfter.EndDate, baselineFEEnd)
	}
	if !f.tasks[parent.ID].StartDate.Equal(baselineParentStart) {
		t.Errorf("FloorRatchet (constraint): родительский бар после снятия ограничения = %v, хотим %v (baseline)",
			f.tasks[parent.ID].StartDate, baselineParentStart)
	}
}

// TestRecalculateTeamSchedule_FloorRatchet_Idempotence проверяет задачу 3.3:
// идемпотентность при многократных пересчётах без изменения данных.
func TestRecalculateTeamSchedule_FloorRatchet_Idempotence(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	feID := uuid.New()

	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})
	f.addRole(&domain.Role{ID: feID, Name: "FE разработчик"})
	f.roleScores[epicID] = []domain.EpicRoleScore{{EpicID: epicID, RoleID: feID, WeightedAvg: 2.0}}

	start := date(2026, 7, 13)
	if _, err := svc.GenerateTasksForEpic(ctx, epicID, start); err != nil {
		t.Fatalf("generate: %v", err)
	}

	result1, err := svc.RecalculateTeamSchedule(ctx, teamID)
	if err != nil {
		t.Fatalf("recalculate (1st): %v", err)
	}

	var baselineStart, baselineEnd time.Time
	for _, task := range result1 {
		if task.Name == "FE разработчик" && !task.IsParent {
			baselineStart = task.StartDate
			baselineEnd = task.EndDate
			break
		}
	}

	// Пересчитываем ещё 5 раз
	for i := 0; i < 5; i++ {
		result, err := svc.RecalculateTeamSchedule(ctx, teamID)
		if err != nil {
			t.Fatalf("recalculate (pass %d): %v", i+2, err)
		}

		for _, task := range result {
			if task.Name == "FE разработчик" && !task.IsParent {
				if !task.StartDate.Equal(baselineStart) {
					t.Errorf("idempotence violation at pass %d: FE start = %v, want %v",
						i+2, task.StartDate, baselineStart)
				}
				if !task.EndDate.Equal(baselineEnd) {
					t.Errorf("idempotence violation at pass %d: FE end = %v, want %v",
						i+2, task.EndDate, baselineEnd)
				}
				break
			}
		}
	}
}
