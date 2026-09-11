package gantt

import (
	"EpicScoreBot/internal/models/domain"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Юнит-тесты findEarliestSlot/reserveInterval (задача 2.2 заявки
// backfill-idle-gaps-in-schedule): поиск самого раннего подходящего
// промежутка календаря — first-fit, без дробления задачи и без изменения
// уже занятых интервалов.

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestFindEarliestSlot_EmptyCalendarDegeneratesToEarliest(t *testing.T) {
	// Без броней задача встаёт прямо на earliest (moveToWorkDay) — вырождение
	// в поведение до появления заполнения простоев (design.md Решение 2).
	earliest := date(2026, 7, 13) // Monday
	got := findEarliestSlot(nil, 3, earliest)
	want := date(2026, 7, 13)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFindEarliestSlot_FitsWholeInGapBeforeBusyInterval(t *testing.T) {
	// Простой Jul13-Jul14 перед бронью Jul15-Jul17: задача на 2 рабочих дня
	// помещается в него целиком.
	ivs := []interval{{start: date(2026, 7, 15), end: date(2026, 7, 17)}}
	got := findEarliestSlot(ivs, 2, date(2026, 7, 13))
	want := date(2026, 7, 13)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v (task should occupy the gap before the busy interval)", got, want)
	}
}

func TestFindEarliestSlot_DoesNotFitFallsBackAfterLastBooking_NoSplitting(t *testing.T) {
	// Тот же простой Jul13-Jul14 (2 рабочих дня), но задача требует 3 —
	// не помещается целиком, поэтому НЕ дробится и не встаёт в промежуток:
	// размещается после последней брони, теми же 3 рабочими днями подряд.
	ivs := []interval{{start: date(2026, 7, 15), end: date(2026, 7, 17)}}
	got := findEarliestSlot(ivs, 3, date(2026, 7, 13))
	want := date(2026, 7, 20) // nextAvailable(Fri Jul17) -> Mon Jul20
	if !got.Equal(want) {
		t.Errorf("got %v, want %v (must fall back to after last booking, not split)", got, want)
	}
	end := addWorkDays(got, 3)
	wantEnd := date(2026, 7, 22)
	if !end.Equal(wantEnd) {
		t.Errorf("end date got %v, want %v — task must keep its full duration, not be shortened", end, wantEnd)
	}
}

func TestFindEarliestSlot_SkipsGapsTooEarlyForEarliestBound(t *testing.T) {
	// earliest сам по себе лежит внутри/после первой брони — простой перед
	// ней уже недоступен, задача уходит в промежуток МЕЖДУ бронями.
	ivs := []interval{
		{start: date(2026, 7, 13), end: date(2026, 7, 13)}, // Mon busy
		{start: date(2026, 7, 17), end: date(2026, 7, 17)}, // Fri busy
	}
	got := findEarliestSlot(ivs, 2, date(2026, 7, 13))
	want := date(2026, 7, 14) // Tue-Wed, fits before Fri
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFindEarliestSlot_MultipleFittingGaps_PicksEarliest(t *testing.T) {
	// Несколько подходящих промежутков — first-fit (design.md Решение 2):
	// выбирается самый ранний, а не более тесный.
	ivs := []interval{
		{start: date(2026, 7, 16), end: date(2026, 7, 16)}, // Thu busy
		{start: date(2026, 7, 23), end: date(2026, 7, 23)}, // next Thu busy
	}
	got := findEarliestSlot(ivs, 1, date(2026, 7, 13))
	want := date(2026, 7, 13) // Mon, earliest fitting gap (before the first booking)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v (earliest fitting gap, not a later one)", got, want)
	}
}

func TestReserveInterval_KeepsCalendarSortedByStart(t *testing.T) {
	calendar := make(map[uuid.UUID][]interval)
	key := uuid.New()

	// Резервируем в НЕ хронологическом порядке — как это происходит при
	// заполнении простоев (менее приоритетная задача может занять более
	// ранний промежуток уже после того, как более приоритетная зарезерви-
	// ровала более поздний).
	reserveInterval(calendar, key, date(2026, 7, 20), date(2026, 7, 21))
	reserveInterval(calendar, key, date(2026, 7, 13), date(2026, 7, 13))
	reserveInterval(calendar, key, date(2026, 7, 16), date(2026, 7, 16))

	ivs := calendar[key]
	if len(ivs) != 3 {
		t.Fatalf("expected 3 intervals, got %d", len(ivs))
	}
	for i := 1; i < len(ivs); i++ {
		if !ivs[i].start.After(ivs[i-1].start) {
			t.Errorf("calendar not sorted by start: ivs[%d].start=%v, ivs[%d].start=%v",
				i-1, ivs[i-1].start, i, ivs[i].start)
		}
	}
	if !ivs[0].start.Equal(date(2026, 7, 13)) {
		t.Errorf("first interval start = %v, want Jul13", ivs[0].start)
	}
}

// --- Интеграционные тесты через RecalculateTeamSchedule ---

// TestRecalculateTeamSchedule_BackfillsIdleGapBeforeHigherPriorityStory —
// ключевой сценарий заявки (proposal.md "Why", design.md сценарий "Короткая
// задача помещается в простой перед более приоритетной"): у стори более
// высокого приоритета длинный анализ, из-за которого FE-разработчик
// простаивает; у стори более низкого приоритета анализ и FE-задача короткие
// и целиком помещаются в этот простой. Задача 2.2/2.3.
func TestRecalculateTeamSchedule_BackfillsIdleGapBeforeHigherPriorityStory(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	story1ID := uuid.New()
	story2ID := uuid.New()
	analystID := uuid.New()
	feID := uuid.New()

	analystA := uuid.New()
	analystB := uuid.New()
	feUser := uuid.New()

	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})
	story1 := f.addEpic(&domain.Epic{ID: story1ID, Number: "E-1-S1", Name: "Story 1", TeamID: teamID, ParentEpicID: &epicID})
	story2 := f.addEpic(&domain.Epic{ID: story2ID, Number: "E-1-S2", Name: "Story 2", TeamID: teamID, ParentEpicID: &epicID})
	f.addRole(&domain.Role{ID: analystID, Name: "Аналитик"})
	f.addRole(&domain.Role{ID: feID, Name: "FE разработчик"})

	// Два аналитика — позволяет истории 2 не ждать аналитика истории 1
	// (design.md "add-gantt-task-assignees": единица занятости — исполнитель).
	f.addUser(&domain.User{ID: analystA, FirstName: "A"}, []uuid.UUID{teamID}, []uuid.UUID{analystID})
	f.addUser(&domain.User{ID: analystB, FirstName: "B"}, []uuid.UUID{teamID}, []uuid.UUID{analystID})
	// Один FE-разработчик — тот самый простаивающий исполнитель из сценария.
	f.addUser(&domain.User{ID: feUser, FirstName: "F"}, []uuid.UUID{teamID}, []uuid.UUID{feID})

	f.roleScores[story1ID] = []domain.EpicRoleScore{
		{EpicID: story1ID, RoleID: analystID, WeightedAvg: 8.0}, // long analysis
		{EpicID: story1ID, RoleID: feID, WeightedAvg: 3.0},
	}
	f.roleScores[story2ID] = []domain.EpicRoleScore{
		{EpicID: story2ID, RoleID: analystID, WeightedAvg: 1.0}, // short analysis
		{EpicID: story2ID, RoleID: feID, WeightedAvg: 2.0},      // ready early, fits the gap
	}

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

	// FE story2 назначена позже (story1 обрабатывается первой в конвейере),
	// но её короткая задача целиком помещается в простой FE-разработчика
	// перед долгим FE story1 и занимает его.
	if !feStory2.StartDate.Before(feStory1.StartDate) {
		t.Errorf("FE story2 (%v) should start before FE story1 (%v) — it fits the idle gap",
			feStory2.StartDate, feStory1.StartDate)
	}

	// Оба назначены одному и тому же (единственному) FE-исполнителю — и не
	// пересекаются по датам (design.md "не двигает уже размещённые задачи").
	if feStory1.AssigneeID == nil || feStory2.AssigneeID == nil || *feStory1.AssigneeID != feUser || *feStory2.AssigneeID != feUser {
		t.Fatalf("expected both FE tasks assigned to the single FE candidate")
	}
	if !feStory2.EndDate.Before(feStory1.StartDate) {
		t.Errorf("FE story2 (ends %v) must fully finish before FE story1 starts (%v) — no overlap",
			feStory2.EndDate, feStory1.StartDate)
	}

	// FE story1 (более приоритетная задача) не сдвинулась заполнением
	// простоя: стартует сразу после аналитика story1 (Mon Jul13 + 8 рабочих
	// дней -> Wed Jul22, +1 рабочий день -> четверг Jul23).
	wantFeStory1Start := date(2026, 7, 23)
	if !feStory1.StartDate.Equal(wantFeStory1Start) {
		t.Errorf("FE story1 start = %v, want %v (unaffected by backfill)", feStory1.StartDate, wantFeStory1Start)
	}
}

// TestRecalculateTeamSchedule_BackfillRespectsPastBlockSetting проверяет
// задачу 2.4 в изолированном виде: нижняя граница поиска промежутка
// (epicFloor) равна teamFloor, когда настройка выключена (совпадает с
// прежним поведением, квартал вполне может начинаться в прошлом — design.md
// Risks), и max(teamFloor, today), когда включена. Взаимодействие настройки
// с реальным простоем (свободный промежуток, начинающийся раньше текущей
// даты) проверяется в паре с замороженной задачей — см.
// TestRecalculateTeamSchedule_FrozenTaskReservesActualIntervalOpeningGapBefore,
// где момент "закрытия" такого простоя виден напрямую (design.md Решение 4).
func TestRecalculateTeamSchedule_BackfillRespectsPastBlockSetting(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	feID := uuid.New()

	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})
	f.addRole(&domain.Role{ID: feID, Name: "FE разработчик"})
	f.roleScores[epicID] = []domain.EpicRoleScore{
		{EpicID: epicID, RoleID: feID, WeightedAvg: 1.0},
	}

	// Квартал начался достаточно давно в прошлом.
	pastStart := moveToWorkDay(toMidnight(time.Now()).AddDate(0, 0, -60))
	tasks, err := svc.GenerateTasksForEpic(ctx, epicID, pastStart)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parent := tasks[0]

	// Legacy-эпик без сторей: ролевая задача — прямой child родителя эпика.
	fe := findTaskByParentAndName(f, parent.ID, "FE разработчик")
	if fe == nil {
		t.Fatalf("expected FE task to exist")
	}

	// Настройка выключена по умолчанию — нижняя граница совпадает с
	// teamFloor (прошлым стартом квартала), задача не двигается вперёд.
	if !fe.StartDate.Equal(pastStart) {
		t.Errorf("setting disabled: FE start = %v, want %v (floor = teamFloor, unchanged)", fe.StartDate, pastStart)
	}

	// Включаем настройку и пересчитываем — нижняя граница поднимается до
	// текущей даты.
	f.setTeamBackfillBlockPast(teamID, true)
	if _, err := svc.RecalculateTeamSchedule(ctx, teamID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fe = f.tasks[fe.ID]
	wantEnabled := moveToWorkDay(toMidnight(time.Now()))
	if !fe.StartDate.Equal(wantEnabled) {
		t.Errorf("setting enabled: FE start = %v, want %v (floor = max(teamFloor, today))", fe.StartDate, wantEnabled)
	}
	// Примечание: teamFloor сам вычисляется заново на каждый вызов из
	// текущей даты родительского бара эпика (design.md, комментарий у
	// RecalculateTeamSchedule про self-correcting floor) — после того как
	// эта дата подтянулась к "сегодня", повторное выключение настройки не
	// обязано откатить её обратно к pastStart: это свойство самого
	// teamFloor, не специфичное для заявки backfill-idle-gaps-in-schedule.
}

// TestRecalculateTeamSchedule_FrozenTaskReservesActualIntervalOpeningGapBefore
// проверяет задачу 2.5: замороженная задача (Progress > 0) резервирует свой
// фактический интервал [StartDate, EndDate], а не просто двигает границу
// "занят до" вперёд — это открывает простой ПЕРЕД ней, доступный при
// выключенной настройке и недоступный при включённой (design.md Решение 4).
func TestRecalculateTeamSchedule_FrozenTaskReservesActualIntervalOpeningGapBefore(t *testing.T) {
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

	f.roleScores[story1ID] = []domain.EpicRoleScore{
		{EpicID: story1ID, RoleID: feID, WeightedAvg: 1.0},
	}
	f.roleScores[story2ID] = []domain.EpicRoleScore{
		{EpicID: story2ID, RoleID: feID, WeightedAvg: 1.0},
	}

	start := date(2026, 7, 13) // Monday
	if _, err := svc.GenerateTasksForEpic(ctx, epicID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	story1Task := findStoryTask(f, storyName(story1))
	story2Task := findStoryTask(f, storyName(story2))
	feStory1 := findTaskByParentAndName(f, story1Task.ID, "FE разработчик")
	feStory2 := findTaskByParentAndName(f, story2Task.ID, "FE разработчик")

	// Замораживаем FE story1 фактом с ПОЗДНЕЙ фактической датой начала
	// (частый в реальности случай — задачу взяли в работу не в плановый
	// срок), сохраняя её плановые даты как есть — только Progress > 0
	// делает её замороженной; сдвигаем StartDate вручную, как это сделал бы
	// планировщик при более сложном сценарии, чтобы получить видимый
	// простой перед ней.
	feStory1.Progress = 50
	feStory1.StartDate = date(2026, 7, 20)
	feStory1.EndDate = date(2026, 7, 20)
	f.tasks[feStory1.ID] = feStory1

	if _, err := svc.RecalculateTeamSchedule(ctx, teamID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	feStory1Got := f.tasks[feStory1.ID]
	if !feStory1Got.StartDate.Equal(date(2026, 7, 20)) || !feStory1Got.EndDate.Equal(date(2026, 7, 20)) {
		t.Fatalf("frozen task must not be rescheduled: got [%v,%v]", feStory1Got.StartDate, feStory1Got.EndDate)
	}

	feStory2Got := f.tasks[feStory2.ID]
	// Простой Jul13-Jul17 (перед замороженной FE story1, Jul20) доступен —
	// story2 занимает его самое раннее место (Mon Jul13), а не после
	// story1 (design.md Решение 4).
	if !feStory2Got.StartDate.Equal(date(2026, 7, 13)) {
		t.Errorf("FE story2 start = %v, want Jul13 (gap before the frozen task is usable)", feStory2Got.StartDate)
	}
	if !feStory2Got.EndDate.Before(feStory1Got.StartDate) {
		t.Errorf("FE story2 (ends %v) must not overlap the frozen FE story1 (starts %v)",
			feStory2Got.EndDate, feStory1Got.StartDate)
	}

	// Взаимодействие с задачей 2.4: даты этого сценария (Jul 2026) заведомо
	// в прошлом относительно текущей даты выполнения теста — включаем
	// настройку «не занимать промежутки в прошлом» и убеждаемся, что тот же
	// простой перед замороженной задачей больше не используется (design.md
	// Решение 4, "Следствие, о котором нужно знать").
	if !date(2026, 7, 13).Before(toMidnight(time.Now())) {
		t.Fatalf("test setup invalid: fixture dates must be in the past relative to time.Now() for this assertion")
	}
	f.setTeamBackfillBlockPast(teamID, true)
	if _, err := svc.RecalculateTeamSchedule(ctx, teamID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	feStory1Got = f.tasks[feStory1.ID]
	if !feStory1Got.StartDate.Equal(date(2026, 7, 20)) || !feStory1Got.EndDate.Equal(date(2026, 7, 20)) {
		t.Errorf("frozen task must stay put regardless of the setting: got [%v,%v]",
			feStory1Got.StartDate, feStory1Got.EndDate)
	}
	feStory2Got = f.tasks[feStory2.ID]
	wantEnabled := moveToWorkDay(toMidnight(time.Now()))
	if !feStory2Got.StartDate.Equal(wantEnabled) {
		t.Errorf("setting enabled: FE story2 start = %v, want %v (past gap before the frozen task is now off-limits)",
			feStory2Got.StartDate, wantEnabled)
	}
}

// TestRecalculateTeamSchedule_RoleWithoutPoolBackfillsGapAndDoesNotOverlap
// проверяет задачу 2.6: заполнение простоев распространяется на календарь
// роли без кандидатов в команде (roleFreeAt) — тем же first-fit поиском,
// что и для исполнителя, без пересечений задач роли между собой.
func TestRecalculateTeamSchedule_RoleWithoutPoolBackfillsGapAndDoesNotOverlap(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	story1ID := uuid.New()
	story2ID := uuid.New()
	story3ID := uuid.New()
	itID := uuid.New()

	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})
	story1 := f.addEpic(&domain.Epic{ID: story1ID, Number: "E-1-S1", Name: "Story 1", TeamID: teamID, ParentEpicID: &epicID})
	story2 := f.addEpic(&domain.Epic{ID: story2ID, Number: "E-1-S2", Name: "Story 2", TeamID: teamID, ParentEpicID: &epicID})
	story3 := f.addEpic(&domain.Epic{ID: story3ID, Number: "E-1-S3", Name: "Story 3", TeamID: teamID, ParentEpicID: &epicID})
	// IT-лидер — роль без кандидатов в команде (пул пуст): planируется как
	// единый последовательный ресурс (roleFreeAt), design.md Решение 1/5.
	f.addRole(&domain.Role{ID: itID, Name: "IT-лидер"})

	f.roleScores[story1ID] = []domain.EpicRoleScore{{EpicID: story1ID, RoleID: itID, WeightedAvg: 1.0}}
	f.roleScores[story2ID] = []domain.EpicRoleScore{{EpicID: story2ID, RoleID: itID, WeightedAvg: 1.0}}
	f.roleScores[story3ID] = []domain.EpicRoleScore{{EpicID: story3ID, RoleID: itID, WeightedAvg: 1.0}}

	start := date(2026, 7, 13) // Monday
	if _, err := svc.GenerateTasksForEpic(ctx, epicID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	story1Task := findStoryTask(f, storyName(story1))
	story2Task := findStoryTask(f, storyName(story2))
	story3Task := findStoryTask(f, storyName(story3))
	it1 := findTaskByParentAndName(f, story1Task.ID, "IT-лидер")
	it2 := findTaskByParentAndName(f, story2Task.ID, "IT-лидер")
	it3 := findTaskByParentAndName(f, story3Task.ID, "IT-лидер")
	if it1 == nil || it2 == nil || it3 == nil {
		t.Fatalf("expected all 3 IT-лидер tasks to exist")
	}

	// Единый ресурс без backfill-простоев (все таски по 1 дню, сразу друг за
	// другом, обхода без ручных смещений): Jul13, Jul14, Jul15.
	if !it1.StartDate.Equal(date(2026, 7, 13)) {
		t.Errorf("it1 start = %v, want Jul13", it1.StartDate)
	}
	if !it2.StartDate.Equal(date(2026, 7, 14)) {
		t.Errorf("it2 start = %v, want Jul14", it2.StartDate)
	}
	if !it3.StartDate.Equal(date(2026, 7, 15)) {
		t.Errorf("it3 start = %v, want Jul15", it3.StartDate)
	}

	// Освобождаем простой перед it3, сдвинув её ручным смещением старта
	// далеко назад — она должна занять один из простоёв в очереди роли
	// (после it1, перед it2), а не встать после it2.
	if _, err := svc.SetTaskStartOffset(ctx, it3.ID, -10); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	it1Got := f.tasks[it1.ID]
	it2Got := f.tasks[it2.ID]
	it3Got := f.tasks[it3.ID]

	// it1 не сдвинулась (более приоритетная, размещена раньше).
	if !it1Got.StartDate.Equal(date(2026, 7, 13)) || !it1Got.EndDate.Equal(date(2026, 7, 13)) {
		t.Errorf("it1 must stay at Jul13, got [%v,%v]", it1Got.StartDate, it1Got.EndDate)
	}

	// Непересечение: ни одна пара задач роли не пересекается по датам.
	all := []*domain.GanttTask{it1Got, it2Got, it3Got}
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			a, b := all[i], all[j]
			overlap := !a.EndDate.Before(b.StartDate) && !b.EndDate.Before(a.StartDate)
			if overlap {
				t.Errorf("tasks %s [%v,%v] and %s [%v,%v] overlap",
					a.ID, a.StartDate, a.EndDate, b.ID, b.StartDate, b.EndDate)
			}
		}
	}
}

// TestRecalculateTeamSchedule_PastBlockAppliesBeyondGaps проверяет сценарий
// спеки «Запрет действует и вне промежутков» (specs/gantt-schedule-backfill,
// design.md Решение 3): граница `floor` действует на ВСЁ размещение
// (`earliest = max(target, floor)`), а не только на поиск промежутка. Когда
// запрет включён, последняя занятая дата исполнителя лежит в прошлом и для
// очередной задачи подходящего простоя нет вовсе (frozen-задача занимает
// календарь ровно от нижней границы очереди, не оставляя простоя перед
// собой), задача всё равно размещается не раньше текущей даты — а не сразу
// после последней брони, как было бы при более узкой границе, действующей
// только на поиск промежутка.
func TestRecalculateTeamSchedule_PastBlockAppliesBeyondGaps(t *testing.T) {
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

	f.roleScores[story1ID] = []domain.EpicRoleScore{
		{EpicID: story1ID, RoleID: feID, WeightedAvg: 1.0},
	}
	f.roleScores[story2ID] = []domain.EpicRoleScore{
		{EpicID: story2ID, RoleID: feID, WeightedAvg: 1.0},
	}

	start := date(2026, 7, 13) // Monday, в прошлом относительно текущей даты выполнения теста
	if _, err := svc.GenerateTasksForEpic(ctx, epicID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !start.Before(toMidnight(time.Now())) {
		t.Fatalf("test setup invalid: fixture dates must be in the past relative to time.Now()")
	}

	story1Task := findStoryTask(f, storyName(story1))
	story2Task := findStoryTask(f, storyName(story2))
	feStory1 := findTaskByParentAndName(f, story1Task.ID, "FE разработчик")
	feStory2 := findTaskByParentAndName(f, story2Task.ID, "FE разработчик")
	if feStory1 == nil || feStory2 == nil {
		t.Fatalf("expected both FE tasks to exist")
	}

	// Замораживаем FE story1 так, чтобы она заняла календарь ровно от
	// нижней границы очереди (Jul13) без единого дня простоя перед собой —
	// единственный кандидат на размещение story2 в обоих режимах
	// настройки это открытый хвост ПОСЛЕ story1, не промежуток.
	feStory1.Progress = 50
	feStory1.StartDate = date(2026, 7, 13)
	feStory1.EndDate = date(2026, 7, 17)
	f.tasks[feStory1.ID] = feStory1

	if _, err := svc.RecalculateTeamSchedule(ctx, teamID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Настройка выключена (по умолчанию) — story2 встаёт сразу после
	// последней брони (следующий рабочий день после Fri Jul17 — Mon Jul20),
	// в прошлом, как и раньше.
	feStory2Got := f.tasks[feStory2.ID]
	wantDisabled := date(2026, 7, 20)
	if !feStory2Got.StartDate.Equal(wantDisabled) {
		t.Fatalf("setting disabled: FE story2 start = %v, want %v (right after the last booking)",
			feStory2Got.StartDate, wantDisabled)
	}
	if !wantDisabled.Before(toMidnight(time.Now())) {
		t.Fatalf("test setup invalid: expected the day right after story1 to still be in the past, got %v", wantDisabled)
	}

	// Включаем запрет и пересчитываем — подходящего промежутка по-прежнему
	// нет (frozen story1 занимает календарь от Jul13 без простоя перед
	// собой), но задача всё равно размещается не раньше текущей даты, а не
	// сразу после последней брони (Jul20).
	f.setTeamBackfillBlockPast(teamID, true)
	if _, err := svc.RecalculateTeamSchedule(ctx, teamID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	feStory1Got := f.tasks[feStory1.ID]
	if !feStory1Got.StartDate.Equal(date(2026, 7, 13)) || !feStory1Got.EndDate.Equal(date(2026, 7, 17)) {
		t.Errorf("frozen task must stay put regardless of the setting: got [%v,%v]",
			feStory1Got.StartDate, feStory1Got.EndDate)
	}
	feStory2Got = f.tasks[feStory2.ID]
	wantEnabled := moveToWorkDay(toMidnight(time.Now()))
	if !feStory2Got.StartDate.Equal(wantEnabled) {
		t.Errorf("setting enabled, no fitting gap: FE story2 start = %v, want %v (clamped to today, not to the day after the last booking)",
			feStory2Got.StartDate, wantEnabled)
	}
	if feStory2Got.StartDate.Equal(wantDisabled) {
		t.Errorf("FE story2 start must not equal the day right after the last booking (%v) once the setting is enabled",
			wantDisabled)
	}
}

// TestSetTeamBackfillBlockPast_UpdatesSettingAndRecalculates проверяет
// сервисный метод, вызываемый обработчиком PUT-эндпоинта настройки
// (backend §3.2): значение сохраняется через репозиторий и сразу же
// применяется к расписанию — команда, у которой квартал начался в прошлом,
// после включения запрета получает пересчитанные даты не раньше текущей
// даты (design.md Решение 3, Scenario "Администратор переключает
// настройку").
func TestSetTeamBackfillBlockPast_UpdatesSettingAndRecalculates(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	feID := uuid.New()

	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})
	f.addRole(&domain.Role{ID: feID, Name: "FE разработчик"})
	f.roleScores[epicID] = []domain.EpicRoleScore{
		{EpicID: epicID, RoleID: feID, WeightedAvg: 1.0},
	}

	pastStart := moveToWorkDay(toMidnight(time.Now()).AddDate(0, 0, -60))
	tasks, err := svc.GenerateTasksForEpic(ctx, epicID, pastStart)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parent := tasks[0]
	fe := findTaskByParentAndName(f, parent.ID, "FE разработчик")
	if fe == nil {
		t.Fatalf("expected FE task to exist")
	}
	if !fe.StartDate.Equal(pastStart) {
		t.Fatalf("precondition failed: FE start = %v, want %v", fe.StartDate, pastStart)
	}

	got, err := svc.SetTeamBackfillBlockPast(ctx, teamID, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) == 0 {
		t.Fatalf("expected recalculated tasks to be returned")
	}

	team, err := f.GetTeamByID(ctx, teamID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !team.BackfillBlockPast {
		t.Fatalf("expected the setting to be persisted as enabled")
	}

	feGot := f.tasks[fe.ID]
	wantEnabled := moveToWorkDay(toMidnight(time.Now()))
	if !feGot.StartDate.Equal(wantEnabled) {
		t.Errorf("FE start after enabling = %v, want %v (recalculated with the new floor)", feGot.StartDate, wantEnabled)
	}

	// Выключаем обратно тем же методом.
	if _, err := svc.SetTeamBackfillBlockPast(ctx, teamID, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	team, err = f.GetTeamByID(ctx, teamID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if team.BackfillBlockPast {
		t.Errorf("expected the setting to be persisted as disabled")
	}
}


// TestRecalculateTeamSchedule_MultipleAssigneesNoOverlapWithBackfill проверяет
// задачу 6.3 (QA): непересечение по каждому исполнителю на наборе, где часть
// задач заняла промежутки. Параллельные бары одной роли теперь штатны
// (add-gantt-task-assignees), но пересечение задач одного человека на диаграмме
// легко не заметить — требуется явная проверка попарного непересечения.
func TestRecalculateTeamSchedule_MultipleAssigneesNoOverlapWithBackfill(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	story1ID := uuid.New()
	story2ID := uuid.New()
	story3ID := uuid.New()
	feID := uuid.New()

	feUser1 := uuid.New()
	feUser2 := uuid.New()

	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})
	f.addEpic(&domain.Epic{ID: story1ID, Number: "E-1-S1", Name: "Story 1", TeamID: teamID, ParentEpicID: &epicID})
	f.addEpic(&domain.Epic{ID: story2ID, Number: "E-1-S2", Name: "Story 2", TeamID: teamID, ParentEpicID: &epicID})
	f.addEpic(&domain.Epic{ID: story3ID, Number: "E-1-S3", Name: "Story 3", TeamID: teamID, ParentEpicID: &epicID})
	f.addRole(&domain.Role{ID: feID, Name: "FE разработчик"})

	// Два исполнителя одной роли — позволяет параллельное заполнение простоев
	f.addUser(&domain.User{ID: feUser1, FirstName: "F1"}, []uuid.UUID{teamID}, []uuid.UUID{feID})
	f.addUser(&domain.User{ID: feUser2, FirstName: "F2"}, []uuid.UUID{teamID}, []uuid.UUID{feID})

	f.roleScores[story1ID] = []domain.EpicRoleScore{
		{EpicID: story1ID, RoleID: feID, WeightedAvg: 5.0}, // long
	}
	f.roleScores[story2ID] = []domain.EpicRoleScore{
		{EpicID: story2ID, RoleID: feID, WeightedAvg: 2.0}, // fits gap
	}
	f.roleScores[story3ID] = []domain.EpicRoleScore{
		{EpicID: story3ID, RoleID: feID, WeightedAvg: 2.0},
	}

	start := date(2026, 7, 13) // Monday
	if _, err := svc.GenerateTasksForEpic(ctx, epicID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Собираем все FE-задачи и группируем по исполнителям
	tasksByAssignee := make(map[uuid.UUID][]*domain.GanttTask)
	for _, task := range f.tasks {
		if task.Name == "FE разработчик" && task.AssigneeID != nil {
			tasksByAssignee[*task.AssigneeID] = append(tasksByAssignee[*task.AssigneeID], task)
		}
	}

	// Явная проверка попарного непересечения для каждого исполнителя:
	// это критично, потому что параллельные бары одной роли теперь нормальны,
	// и визуально пересечение не всегда заметно.
	for assigneeID, tasks := range tasksByAssignee {
		for i := 0; i < len(tasks); i++ {
			for j := i + 1; j < len(tasks); j++ {
				a, b := tasks[i], tasks[j]
				// Задачи перекрываются, если одна не закончилась до начала другой
				// И вторая не закончилась до начала первой
				aBeforeB := a.EndDate.Before(b.StartDate) || a.EndDate.Equal(b.StartDate)
				bBeforeA := b.EndDate.Before(a.StartDate) || b.EndDate.Equal(a.StartDate)
				noOverlap := aBeforeB || bBeforeA
				if !noOverlap {
					t.Errorf("assignee %s has overlapping FE tasks: %s [%v,%v] and %s [%v,%v]",
						assigneeID.String()[:8], a.ID.String()[:8], a.StartDate, a.EndDate,
						b.ID.String()[:8], b.StartDate, b.EndDate)
				}
			}
		}
	}
}

// TestRecalculateTeamSchedule_StoryPredecessorConstraintsDuringBackfill проверяет
// задачу 6.4 (QA): соблюдение предусловий (зависимостей между ролевыми группами
// в стори) при заполнении простоев. Промежуток раньше окончания предыдущей
// ролевой группы стори не занимается следующей ролью.
func TestRecalculateTeamSchedule_StoryPredecessorConstraintsDuringBackfill(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	storyID := uuid.New()
	analystID := uuid.New()
	feID := uuid.New()

	analystUser := uuid.New()
	feUser := uuid.New()

	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})
	f.addEpic(&domain.Epic{ID: storyID, Number: "E-1-S1", Name: "Story 1", TeamID: teamID, ParentEpicID: &epicID})
	f.addRole(&domain.Role{ID: analystID, Name: "Аналитик"})
	f.addRole(&domain.Role{ID: feID, Name: "FE разработчик"})

	f.addUser(&domain.User{ID: analystUser, FirstName: "A"}, []uuid.UUID{teamID}, []uuid.UUID{analystID})
	f.addUser(&domain.User{ID: feUser, FirstName: "F"}, []uuid.UUID{teamID}, []uuid.UUID{feID})

	f.roleScores[storyID] = []domain.EpicRoleScore{
		{EpicID: storyID, RoleID: analystID, WeightedAvg: 3.0},
		{EpicID: storyID, RoleID: feID, WeightedAvg: 2.0},
	}

	start := date(2026, 7, 13) // Monday
	if _, err := svc.GenerateTasksForEpic(ctx, epicID, start); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	storyTask := findStoryTask(f, storyName(&domain.Epic{Number: "E-1-S1", Name: "Story 1"}))
	if storyTask == nil {
		t.Fatalf("expected story task to exist")
	}
	analystTask := findTaskByParentAndName(f, storyTask.ID, "Аналитик")
	feTask := findTaskByParentAndName(f, storyTask.ID, "FE разработчик")
	if analystTask == nil || feTask == nil {
		t.Fatalf("expected both analyst and FE tasks to exist")
	}

	// Ключевая проверка: FE (поздняя ролевая группа) не может начать раньше,
	// чем окончилась задача аналитика (ранняя ролевая группа) в той же стори.
	// Это сохраняется при заполнении простоев.
	if feTask.StartDate.Before(analystTask.EndDate) {
		t.Errorf("FE task (start %v) must not start before analyst task ends (end %v) — predecessor constraint violated",
			feTask.StartDate, analystTask.EndDate)
	}
}
