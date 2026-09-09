package gantt

import (
	"EpicScoreBot/internal/models/domain"
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
)

// defaultRoleOrder maps role names to their default sort order.
// Roles with the same sort order run in parallel.
var defaultRoleOrder = map[string]int{
	"Аналитик":           1,
	"BE разработчик":     2,
	"FE разработчик":     2,
	"Mobile разработчик": 2,
	"Тестировщик":        3,
	"IT-лидер":           4,
}

// roleTask holds intermediate data during task generation.
type roleTask struct {
	roleID    uuid.UUID
	roleName  string
	workDays  int
	sortOrder int
}

// assignmentKey identifies a task_assignments row (domain.TaskAssignment):
// a story — or a legacy epic without stories — plus a role. Mirrors the
// table's primary key (epic_id, role_id), where epic_id is the STORY's ID,
// not gantt_tasks.epic_id (always the top epic for every leaf task
// regardless of story) — see design.md Решение 4.
type assignmentKey struct {
	epicID uuid.UUID
	roleID uuid.UUID
}

// scheduleState holds the resource calendars and manual-assignment overrides
// accumulated while walking a team's whole pipeline in
// RecalculateTeamSchedule. Loaded/reset once per call — see design.md
// Решение 2 (determinism: pools sorted by uuid, rrCursor recreated fresh)
// and Решение 8 (calendars scoped to one team, never shared across calls).
type scheduleState struct {
	// pools — кандидаты на исполнителя каждой роли, отсортированные по
	// uuid (design.md Решение 2: детерминированный порядок для
	// round-robin, не зависящий от порядка выдачи БД).
	pools map[uuid.UUID][]domain.User
	// assignments — пользовательский ввод команды (закрепления
	// исполнителя, смещения старта), прочитанный пачкой одним запросом.
	assignments map[assignmentKey]domain.TaskAssignment
	// assigneeFreeAt — эффективное окончание последней обработанной задачи
	// конкретного исполнителя (design.md Решение 1 — единица занятости).
	assigneeFreeAt map[uuid.UUID]time.Time
	// rrCursor — курсор кругового (round-robin) тай-брейка при выборе
	// исполнителя, per role.
	rrCursor map[uuid.UUID]int
	// roleFreeAt — то же самое, что и раньше (до этого изменения),
	// но используется ТОЛЬКО для ролей с пустым пулом кандидатов в
	// команде — тогда роль по-прежнему планируется как единый
	// последовательный ресурс (design.md Решение 1, "Роль без кандидатов
	// планируется как единый ресурс").
	roleFreeAt map[uuid.UUID]time.Time
}

// Service provides Gantt chart business logic.
type Service struct {
	repo Repository
	log  *slog.Logger

	// scheduleMu хранит per-team мьютексы (teamID -> *sync.Mutex), которыми
	// сериализуются конкурентные вызовы RecalculateTeamSchedule для одной и
	// той же команды — без общей транзакции/блокировки на уровне БД
	// промежуточные чтения/записи (roleFreeAt, teamFloor) двух почти
	// одновременных пересчётов (например, drag-reorder на графике и
	// сохранение через модалку «Порядок…») могли бы переплестись и оставить
	// несогласованные даты. См. openspec/changes/add-schedule-recalc-locking/
	// design.md (Decision 1-2) — in-process блокировка достаточна, т.к.
	// бэкенд деплоится одним контейнером, без реплик.
	scheduleMu sync.Map
}

// teamScheduleLock returns the mutex guarding RecalculateTeamSchedule for a
// given team, lazily creating it on first use. Never removed — the number of
// teams is small enough that this is not a practical memory concern.
func (s *Service) teamScheduleLock(teamID uuid.UUID) *sync.Mutex {
	v, _ := s.scheduleMu.LoadOrStore(teamID, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// TODO: rewrite this func. In scoring.go finalCoeff must be saved in db
// RiskCoefficient maps a weighted risk score to a multiplier coefficient.
func RiskCoefficient(weightedScore float64) float64 {
	rounded := math.Round(weightedScore)
	switch {
	case rounded >= 13:
		return 1.20
	case rounded >= 9:
		return 1.10
	case rounded >= 5:
		return 1.05
	default:
		return 1.03
	}
}

// New creates a new Gantt service.
func New(logger *slog.Logger, repo Repository) *Service {
	return &Service{
		repo: repo,
		log:  logger.With(slog.String("component", "gantt")),
	}
}

// buildRoleTasks calculates role work-day durations (1 SP = 1 day, adjusted
// by the story/epic's risk coefficients) for an epic or a story, sorted by
// defaultRoleOrder. Returns (nil, nil) when there are no role scores yet
// (e.g. a story that hasn't been scored).
func (s *Service) buildRoleTasks(ctx context.Context, storyOrEpicID uuid.UUID) ([]roleTask, error) {
	op := "gantt.buildRoleTasks"

	roleScores, err := s.repo.GetEpicRoleScoresByEpicID(ctx, storyOrEpicID)
	if err != nil {
		return nil, fmt.Errorf("%s: get role scores: %w", op, err)
	}
	if len(roleScores) == 0 {
		return nil, nil
	}

	risks, err := s.repo.GetRisksByEpicID(ctx, storyOrEpicID)
	if err != nil {
		return nil, fmt.Errorf("%s: get risks: %w", op, err)
	}

	finalCoeff := 1.0
	for _, risk := range risks {
		if risk.WeightedScore != nil {
			finalCoeff *= RiskCoefficient(*risk.WeightedScore)
		}
	}

	var roleTasks []roleTask
	for _, rs := range roleScores {
		role, err := s.repo.GetRoleByID(ctx, rs.RoleID)
		if err != nil {
			return nil, fmt.Errorf("%s: get role: %w", op, err)
		}
		workDays := max(1, int(math.Ceil(rs.WeightedAvg*finalCoeff)))
		order, ok := defaultRoleOrder[role.Name]
		if !ok {
			order = 99
		}
		roleTasks = append(roleTasks, roleTask{
			roleID:    rs.RoleID,
			roleName:  role.Name,
			workDays:  workDays,
			sortOrder: order,
		})
	}

	slices.SortFunc(roleTasks, func(a, b roleTask) int {
		if a.sortOrder != b.sortOrder {
			return a.sortOrder - b.sortOrder
		}
		return 0
	})

	return roleTasks, nil
}

// generateTaskRowsForEpic creates Gantt task rows for a scored epic: a parent
// task (the epic itself), a wrapper task per story (or, for legacy epics
// without stories, role tasks directly under the epic), and a role task per
// scored role. It does NOT lay out dates itself — that's the job of the
// global pipeline scheduler (RecalculateTeamSchedule) — and, unlike the
// public GenerateTasksForEpic, it does NOT invoke it either: callers that
// need to (re)generate rows for several epics in one operation (see
// GenerateTasksForQuarter) call RecalculateTeamSchedule themselves, once,
// after all rows across all epics have been created. startDate only seeds
// the epic's initial "floor" (its parent task's StartDate), used by the
// scheduler as the earliest possible start for this epic's own tasks.
// Returns the epic (needed by callers for TeamID/logging).
func (s *Service) generateTaskRowsForEpic(
	ctx context.Context,
	epicID uuid.UUID,
	startDate time.Time,
) (*domain.Epic, error) {
	op := "gantt.generateTaskRowsForEpic"

	epic, err := s.repo.GetEpicByID(ctx, epicID)
	if err != nil {
		return nil, fmt.Errorf("%s: get epic: %w", op, err)
	}

	// Check if tasks already exist.
	exists, err := s.repo.HasGanttTasksForEpic(ctx, epicID)
	if err != nil {
		return nil, fmt.Errorf("%s: check existing: %w", op, err)
	}
	if exists {
		if err := s.repo.DeleteGanttTasksByEpicID(ctx, epicID); err != nil {
			return nil, fmt.Errorf("%s: delete existing: %w", op, err)
		}
	}

	// Get stories of this epic
	stories, err := s.repo.GetStoriesByEpicID(ctx, epicID)
	if err != nil {
		return nil, fmt.Errorf("%s: get stories: %w", op, err)
	}

	adjustedStartDate := moveToWorkDay(startDate)

	// Create parent task (epic). Dates are placeholders, recalculated below.
	parentTask := &domain.GanttTask{
		EpicID:    epicID,
		Name:      fmt.Sprintf("%s: %s", epic.Number, epic.Name),
		StartDate: adjustedStartDate,
		EndDate:   adjustedStartDate,
		SortOrder: 0,
		IsParent:  true,
	}
	parentTask, err = s.repo.CreateGanttTask(ctx, parentTask)
	if err != nil {
		return nil, fmt.Errorf("%s: create parent: %w", op, err)
	}

	createChild := func(parentID uuid.UUID, rt roleTask) error {
		roleID := rt.roleID
		child := &domain.GanttTask{
			EpicID:       epicID,
			RoleID:       &roleID,
			Name:         rt.roleName,
			StartDate:    adjustedStartDate,
			EndDate:      addWorkDays(adjustedStartDate, rt.workDays),
			SortOrder:    rt.sortOrder,
			IsParent:     false,
			ParentTaskID: &parentID,
		}
		if _, err := s.repo.CreateGanttTask(ctx, child); err != nil {
			return fmt.Errorf("%s: create child %s: %w", op, rt.roleName, err)
		}
		return nil
	}

	if len(stories) > 0 {
		for storyIdx, story := range stories {
			storyTask := &domain.GanttTask{
				EpicID:       epicID,
				Name:         fmt.Sprintf("%s: %s", story.Number, story.Name),
				StartDate:    adjustedStartDate,
				EndDate:      adjustedStartDate,
				SortOrder:    storyIdx + 1,
				IsParent:     true,
				ParentTaskID: &parentTask.ID,
			}
			storyTask, err = s.repo.CreateGanttTask(ctx, storyTask)
			if err != nil {
				return nil, fmt.Errorf("%s: create story task %s: %w", op, story.Number, err)
			}

			roleTasks, err := s.buildRoleTasks(ctx, story.ID)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", op, err)
			}
			for _, rt := range roleTasks {
				if err := createChild(storyTask.ID, rt); err != nil {
					return nil, err
				}
			}
		}
	} else {
		// Legacy: flat Gantt for epics without stories (compatibility support).
		roleTasks, err := s.buildRoleTasks(ctx, epicID)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
		if len(roleTasks) == 0 {
			return nil, fmt.Errorf("%s: no role scores for epic %s", op, epicID)
		}
		for _, rt := range roleTasks {
			if err := createChild(parentTask.ID, rt); err != nil {
				return nil, err
			}
		}
	}

	// Assign this epic's place in the team-wide pipeline queue if it
	// doesn't have one yet (e.g. legacy epics created before this column
	// existed, or any other edge case where the insert-time subquery in
	// CreateEpic/CreateStory didn't run).
	if epic.SortOrder == nil {
		if err := s.assignNextEpicSortOrder(ctx, epic); err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
	}

	return epic, nil
}

// GenerateTasksForEpic creates Gantt task rows for a scored epic (see
// generateTaskRowsForEpic) and then rebuilds the team-wide pipeline schedule.
// startDate only seeds the epic's initial "floor" — the scheduler is what
// actually lays out dates.
func (s *Service) GenerateTasksForEpic(
	ctx context.Context,
	epicID uuid.UUID,
	startDate time.Time,
) ([]domain.GanttTask, error) {
	op := "gantt.GenerateTasksForEpic"

	epic, err := s.generateTaskRowsForEpic(ctx, epicID, startDate)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	result, err := s.RecalculateTeamSchedule(ctx, epic.TeamID)
	if err != nil {
		return nil, fmt.Errorf("%s: recalc schedule: %w", op, err)
	}

	s.log.Info("generated gantt tasks",
		slog.String("epicID", epicID.String()),
		slog.Int("taskCount", len(result)))

	return result, nil
}

// QuarterGenerationResult holds counters describing the outcome of a bulk
// GenerateTasksForQuarter call.
type QuarterGenerationResult struct {
	// EpicsTotal — количество отобранных заскоренных эпиков квартала.
	EpicsTotal int
	// EpicsRegenerated — количество эпиков, для которых задачи были
	// успешно пересозданы.
	EpicsRegenerated int
	// EpicsFailed — количество отобранных эпиков, генерация задач для
	// которых завершилась ошибкой (например, отсутствуют оценки ролей).
	EpicsFailed int
	// TasksCount — итоговое число задач Ганта команды после пересчёта
	// расписания (0, если пересчёт не выполнялся).
	TasksCount int
}

// GenerateTasksForQuarter (пере)генерирует задачи Ганта для всех топ-эпиков
// команды за указанные год и квартал, находящихся в статусе SCORED, и
// пересчитывает расписание команды ровно один раз в конце — в отличие от
// вызова GenerateTasksForEpic по каждому эпику отдельно, что дало бы N
// пересчётов подряд (design.md Decision 2, change add-gantt-quarter-regenerate).
//
// Эпики отбираются из GetTeamEpicsOrdered с фильтрацией в памяти, что
// сохраняет порядок очереди планировщика (sort_order) без добавления нового
// метода в Repository. Эпики других периодов или в статусах NEW/SCORING не
// затрагиваются. Ошибка генерации отдельного эпика логируется и увеличивает
// EpicsFailed, не прерывая обработку остальных эпиков квартала (частичный
// успех, design.md Decision 3).
func (s *Service) GenerateTasksForQuarter(
	ctx context.Context,
	teamID uuid.UUID,
	year, quarter int,
	startDate time.Time,
) (QuarterGenerationResult, error) {
	op := "gantt.GenerateTasksForQuarter"

	epics, err := s.repo.GetTeamEpicsOrdered(ctx, teamID)
	if err != nil {
		return QuarterGenerationResult{}, fmt.Errorf("%s: get team epics: %w", op, err)
	}

	var selected []domain.Epic
	for _, e := range epics {
		if e.Year == year && e.Quarter == quarter && e.Status == domain.StatusScored {
			selected = append(selected, e)
		}
	}

	result := QuarterGenerationResult{EpicsTotal: len(selected)}
	if len(selected) == 0 {
		return result, nil
	}

	for _, e := range selected {
		if _, err := s.generateTaskRowsForEpic(ctx, e.ID, startDate); err != nil {
			s.log.Error("failed to generate gantt tasks for epic in quarter regeneration",
				slog.String("epicID", e.ID.String()),
				slog.Int("year", year),
				slog.Int("quarter", quarter),
				slog.Any("error", err))
			result.EpicsFailed++
			continue
		}
		result.EpicsRegenerated++
	}

	if result.EpicsRegenerated == 0 {
		return result, nil
	}

	tasks, err := s.RecalculateTeamSchedule(ctx, teamID)
	if err != nil {
		return result, fmt.Errorf("%s: recalc schedule: %w", op, err)
	}
	result.TasksCount = len(tasks)

	s.log.Info("regenerated gantt tasks for quarter",
		slog.String("teamID", teamID.String()),
		slog.Int("year", year),
		slog.Int("quarter", quarter),
		slog.Int("epicsTotal", result.EpicsTotal),
		slog.Int("epicsRegenerated", result.EpicsRegenerated),
		slog.Int("epicsFailed", result.EpicsFailed),
		slog.Int("tasksCount", result.TasksCount))

	return result, nil
}

// assignNextEpicSortOrder assigns epic the next free position in its
// team's top-level pipeline queue.
func (s *Service) assignNextEpicSortOrder(ctx context.Context, epic *domain.Epic) error {
	op := "gantt.assignNextEpicSortOrder"

	epics, err := s.repo.GetTeamEpicsOrdered(ctx, epic.TeamID)
	if err != nil {
		return fmt.Errorf("%s: get team epics: %w", op, err)
	}
	next := 1
	for _, e := range epics {
		if e.SortOrder != nil && *e.SortOrder >= next {
			next = *e.SortOrder + 1
		}
	}
	if err := s.repo.UpdateEpicSortOrder(ctx, epic.ID, next); err != nil {
		return fmt.Errorf("%s: update sort order: %w", op, err)
	}
	return nil
}

// RecalculateTeamSchedule rebuilds the pipeline schedule for the whole team:
// all epics (ordered by epics.sort_order) -> their stories (ordered by
// epics.sort_order, a "story" being either a real story row or, for legacy
// epics without stories, the epic itself) -> role tasks within a story
// (grouped by defaultRoleOrder, unchanged from before). Unlike the old
// per-epic wave layout, a role does not wait for its siblings from other
// roles to finish the previous story before starting the next one: as soon
// as a role finishes its task in story N, it can start story N+1's task for
// that same role, as long as story N+1's earlier-order roles (e.g. the
// analyst) have already finished for that particular story.
//
// Tasks that are already in progress (Progress > 0) or fully completed
// (ActualEndDate set) are frozen — their StartDate/EndDate are left
// untouched — but their effective completion (ActualEndDate if set,
// otherwise EndDate) is still used as the earliest possible start for that
// role's next task in the pipeline, so a fact that differs from the plan
// reshuffles everything downstream.
//
// Since add-gantt-task-assignees, the unit of occupancy for a role with at
// least one candidate in the team is the individual assignee, not the role
// itself: two tasks of the same role assigned to different people may
// overlap in time (they're scheduled in parallel), while two tasks assigned
// to the same person never do. A role with no candidates in the team keeps
// the old single-queue behaviour (see scheduleState.roleFreeAt and
// design.md Решение 1). Automatic assignee selection is greedy by actual
// resulting start date with a deterministic round-robin tie-break (Решение
// 2); a manual pin (task_assignments) takes priority over the automatic
// choice but not over the date (Решение 3).
func (s *Service) RecalculateTeamSchedule(ctx context.Context, teamID uuid.UUID) ([]domain.GanttTask, error) {
	op := "gantt.RecalculateTeamSchedule"

	// Сериализация: блокировка берётся вокруг ВСЕЙ функции (все её чтения и
	// записи), а не отдельных операций внутри — так конкурентный вызов для
	// той же команды либо целиком не начался, либо целиком завершился, и
	// никогда не видит "перемешанное" промежуточное состояние. См. design.md
	// Decision 2.
	lock := s.teamScheduleLock(teamID)
	if !lock.TryLock() {
		s.log.Info("ожидание пересчёта расписания команды",
			slog.String("teamID", teamID.String()))
		lock.Lock()
	}
	defer lock.Unlock()

	s.log.Debug("старт пересчёта расписания команды",
		slog.String("teamID", teamID.String()))
	defer s.log.Debug("завершён пересчёт расписания команды",
		slog.String("teamID", teamID.String()))

	epics, err := s.repo.GetTeamEpicsOrdered(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("%s: get team epics: %w", op, err)
	}

	// First pass: collect the Gantt rows of every in-scope epic (those that
	// already have a generated chart) and derive a single team-wide floor —
	// the earliest currently recorded epic parent StartDate. A *per-epic*
	// floor read from that same (mutable) field would drift upward every
	// time an epic happens to be scheduled later due to its position in the
	// queue, and that drift would then persist even after the epic is
	// reordered back to the front — defeating ReorderEpic/ReorderStory.
	// A single team floor, recomputed fresh at the start of every call from
	// whichever epic currently holds the earliest date, self-corrects instead.
	type epicWithTasks struct {
		epic  domain.Epic
		tasks []domain.GanttTask
	}
	var inScope []epicWithTasks
	var teamFloor time.Time
	for _, epic := range epics {
		hasTasks, err := s.repo.HasGanttTasksForEpic(ctx, epic.ID)
		if err != nil {
			return nil, fmt.Errorf("%s: check tasks for epic %s: %w", op, epic.ID, err)
		}
		if !hasTasks {
			continue
		}
		epicTasks, err := s.repo.GetGanttTasksByEpicID(ctx, epic.ID)
		if err != nil {
			return nil, fmt.Errorf("%s: get epic tasks: %w", op, err)
		}
		inScope = append(inScope, epicWithTasks{epic: epic, tasks: epicTasks})

		for _, t := range epicTasks {
			if t.IsParent && t.ParentTaskID == nil {
				floor := moveToWorkDay(t.StartDate)
				if teamFloor.IsZero() || floor.Before(teamFloor) {
					teamFloor = floor
				}
			}
		}
	}

	// Загрузка пулов исполнителей по роли и пользовательского ввода
	// (закреплений/смещений) — пачкой на команду, вне цикла по задачам
	// (design.md Risks: "Рост числа запросов к БД в пересчёте"). Роли
	// собираются только те, что реально встречаются среди листовых задач
	// эпиков команды — роль на команду опрашивается один раз за вызов.
	roleIDs := make(map[uuid.UUID]bool)
	for _, ewt := range inScope {
		for _, t := range ewt.tasks {
			if !t.IsParent && t.RoleID != nil {
				roleIDs[*t.RoleID] = true
			}
		}
	}

	pools := make(map[uuid.UUID][]domain.User, len(roleIDs))
	for roleID := range roleIDs {
		candidates, err := s.repo.GetUsersByTeamIDAndRoleID(ctx, teamID, roleID)
		if err != nil {
			return nil, fmt.Errorf("%s: get pool for role %s: %w", op, roleID, err)
		}
		// Сортировка по uuid — единственный порядок, не зависящий от
		// порядка выдачи БД (ORDER BY last_name, first_name у
		// GetUsersByTeamIDAndRoleID) и не рандомизирующийся между
		// вызовами, поэтому round-robin тай-брейк детерминирован
		// (design.md Решение 2).
		slices.SortFunc(candidates, func(a, b domain.User) int {
			return bytes.Compare(a.ID[:], b.ID[:])
		})
		pools[roleID] = candidates
	}

	assignmentRows, err := s.repo.GetTaskAssignmentsByTeamID(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("%s: get task assignments: %w", op, err)
	}
	assignments := make(map[assignmentKey]domain.TaskAssignment, len(assignmentRows))
	for _, a := range assignmentRows {
		assignments[assignmentKey{epicID: a.EpicID, roleID: a.RoleID}] = a
	}

	// state пересоздаётся с нуля на каждый вызов (в т.ч. rrCursor и
	// roleFreeAt) — без этого повторный пересчёт без изменения входных
	// данных мог бы дать другое распределение исполнителей (design.md
	// Risks: "мигающие" исполнители).
	state := &scheduleState{
		pools:          pools,
		assignments:    assignments,
		assigneeFreeAt: make(map[uuid.UUID]time.Time),
		rrCursor:       make(map[uuid.UUID]int),
		roleFreeAt:     make(map[uuid.UUID]time.Time),
	}

	for _, ewt := range inScope {
		if err := s.recalculateEpicSchedule(ctx, ewt.epic, ewt.tasks, teamFloor, state); err != nil {
			return nil, fmt.Errorf("%s: epic %s: %w", op, ewt.epic.ID, err)
		}
	}

	return s.GetTeamTasks(ctx, teamID)
}

// recalculateEpicSchedule recalculates dates, assignees and aggregated
// progress for a single epic's tasks (its stories/legacy role tasks),
// advancing state's calendars as it goes. teamFloor is the team-wide lower
// bound (see RecalculateTeamSchedule).
func (s *Service) recalculateEpicSchedule(
	ctx context.Context,
	epic domain.Epic,
	epicTasks []domain.GanttTask,
	teamFloor time.Time,
	state *scheduleState,
) error {
	op := "gantt.recalculateEpicSchedule"

	var epicParent *domain.GanttTask
	storyTasksByName := make(map[string]*domain.GanttTask)
	roleTasksByParent := make(map[uuid.UUID][]domain.GanttTask)

	for i := range epicTasks {
		t := epicTasks[i]
		switch {
		case t.IsParent && t.ParentTaskID == nil:
			cp := t
			epicParent = &cp
		case t.IsParent && t.ParentTaskID != nil:
			cp := t
			storyTasksByName[t.Name] = &cp
		case !t.IsParent && t.ParentTaskID != nil:
			roleTasksByParent[*t.ParentTaskID] = append(roleTasksByParent[*t.ParentTaskID], t)
		}
	}
	if epicParent == nil {
		return nil
	}

	// epicFloor is the team-wide lower bound (see RecalculateTeamSchedule):
	// no task of any epic is scheduled earlier than this, regardless of
	// queue position, so reordering epics/stories can always move a unit's
	// tasks earlier, not just later.
	epicFloor := teamFloor

	stories, err := s.repo.GetStoriesByEpicID(ctx, epic.ID)
	if err != nil {
		return fmt.Errorf("%s: get stories: %w", op, err)
	}

	type unit struct {
		task  *domain.GanttTask
		roles []domain.GanttTask
		// storyOrEpicID — ключ для task_assignments (assignmentKey.epicID):
		// ID стори, либо ID самого эпика для legacy-эпиков без сторей.
		// НЕ равен gantt_tasks.epic_id для стори (тот всегда указывает на
		// верхний эпик) — см. design.md Решение 4.
		storyOrEpicID uuid.UUID
	}
	var units []unit
	if len(stories) > 0 {
		for _, story := range stories {
			name := fmt.Sprintf("%s: %s", story.Number, story.Name)
			st, ok := storyTasksByName[name]
			if !ok {
				// Story exists but its Gantt row hasn't been generated
				// (shouldn't normally happen once HasGanttTasksForEpic is
				// true, but be defensive rather than panic).
				continue
			}
			units = append(units, unit{task: st, roles: roleTasksByParent[st.ID], storyOrEpicID: story.ID})
		}
	} else {
		units = append(units, unit{task: epicParent, roles: roleTasksByParent[epicParent.ID], storyOrEpicID: epic.ID})
	}

	var epicStart, epicEnd time.Time
	epicHasBounds := false
	var epicProgressSum, epicWeightSum float64

	for _, u := range units {
		if len(u.roles) == 0 {
			continue
		}

		groups := groupGanttTasksBySortOrder(u.roles)
		groupPrevEnd := epicFloor

		var storyStart, storyEnd time.Time
		storyHasBounds := false
		var storyProgressSum, storyWeightSum float64

		for _, group := range groups {
			var groupEnd time.Time
			for _, task := range group {
				if task.RoleID == nil {
					continue
				}
				roleID := *task.RoleID
				workDays := max(1, countWorkDays(task.StartDate, task.EndDate))

				// StartOffsetDays теперь приходит из task_assignments, а не
				// из строки Ганта (design.md Решение 5) — см. комментарий у
				// domain.GanttTask.StartOffsetDays.
				assignment, hasAssignment := state.assignments[assignmentKey{epicID: u.storyOrEpicID, roleID: roleID}]
				offsetDays := 0
				if hasAssignment {
					offsetDays = assignment.StartOffsetDays
				}
				// target — lead/lag на FS-зависимости между ролевыми группами
				// внутри стори: сдвигает groupPrevEnd на целое число
				// календарных дней (отрицательное значение — начать раньше,
				// положительное — намеренная задержка). Календарь
				// исполнителя/роли и epicFloor остаются жёсткими нижними
				// границами через maxTime — офсет не может нарушить
				// непрерывность самой роли/исполнителя в конвейере.
				target := groupPrevEnd.AddDate(0, 0, offsetDays)

				var effectiveEnd time.Time
				frozen := task.Progress > 0 || task.ActualEndDate != nil
				if frozen {
					// Замороженная задача сохраняет исполнителя, назначенного
					// ранее — читаем его как факт, не пересчитываем
					// (design.md Решение 6). Её занятость двигает календарь
					// именно этого исполнителя; если исполнитель неизвестен
					// (например, строка создана до введения assignee_id),
					// используем прежний ролевой календарь, чтобы не
					// потерять непрерывность роли для остальных её задач.
					effectiveEnd = task.EndDate
					if task.ActualEndDate != nil {
						effectiveEnd = *task.ActualEndDate
					}
					if task.AssigneeID != nil {
						advanceFreeAt(state.assigneeFreeAt, *task.AssigneeID, effectiveEnd)
					} else {
						advanceFreeAt(state.roleFreeAt, roleID, effectiveEnd)
					}
				} else {
					pool := state.pools[roleID]

					var chosen *uuid.UUID
					var newStart time.Time
					switch {
					case len(pool) == 0:
						// Роль без кандидатов в команде планируется как
						// единый последовательный ресурс — прежнее
						// поведение (design.md Решение 1).
						newStart = moveToWorkDay(maxTime(target, nextAvailable(state.roleFreeAt[roleID]), epicFloor))
					case hasAssignment && assignment.UserID != nil && poolContains(pool, *assignment.UserID):
						// Действующее закрепление — приоритет над автоматом
						// на выбор человека, но не на дату (design.md
						// Решение 3): задача ждёт освобождения закреплённого
						// человека, даже если свободный кандидат дал бы
						// более ранний старт.
						pinned := *assignment.UserID
						newStart = moveToWorkDay(maxTime(target, nextAvailable(state.assigneeFreeAt[pinned]), epicFloor))
						chosen = &pinned
					default:
						// Нет действующего закрепления (не закреплено вовсе,
						// либо закреплённый человек выбыл из пула роли —
						// design.md Решение 7) — автоматический выбор:
						// argmin по фактической дате старта, round-robin как
						// тай-брейк (design.md Решение 2).
						candidate, start, nextCursor := pickAssignee(pool, state.rrCursor[roleID], func(userID uuid.UUID) time.Time {
							return moveToWorkDay(maxTime(target, nextAvailable(state.assigneeFreeAt[userID]), epicFloor))
						})
						state.rrCursor[roleID] = nextCursor
						chosen = &candidate
						newStart = start
					}

					newEnd := addWorkDays(newStart, workDays)
					if !newStart.Equal(task.StartDate) || !newEnd.Equal(task.EndDate) {
						if err := s.repo.UpdateGanttTaskDates(ctx, task.ID, newStart, newEnd); err != nil {
							return fmt.Errorf("%s: update role task: %w", op, err)
						}
					}
					task.StartDate = newStart
					task.EndDate = newEnd
					effectiveEnd = newEnd

					if !assigneeIDEqual(task.AssigneeID, chosen) {
						if err := s.repo.UpdateGanttTaskAssignee(ctx, task.ID, chosen); err != nil {
							return fmt.Errorf("%s: update assignee: %w", op, err)
						}
						task.AssigneeID = chosen
					}

					if chosen != nil {
						advanceFreeAt(state.assigneeFreeAt, *chosen, effectiveEnd)
					} else {
						advanceFreeAt(state.roleFreeAt, roleID, effectiveEnd)
					}
				}

				if groupEnd.IsZero() || effectiveEnd.After(groupEnd) {
					groupEnd = effectiveEnd
				}

				if !storyHasBounds || task.StartDate.Before(storyStart) {
					storyStart = task.StartDate
				}
				if !storyHasBounds || task.EndDate.After(storyEnd) {
					storyEnd = task.EndDate
				}
				storyHasBounds = true

				weight := float64(workDays)
				storyProgressSum += task.Progress * weight
				storyWeightSum += weight
			}
			groupPrevEnd = moveToWorkDay(groupEnd.AddDate(0, 0, 1))
		}

		if !storyHasBounds {
			continue
		}

		storyProgress := 0.0
		if storyWeightSum > 0 {
			storyProgress = storyProgressSum / storyWeightSum
		}

		// For legacy epics without stories, u.task IS the epic parent task —
		// its dates/progress are set once below, no separate story row exists.
		if u.task.ID != epicParent.ID {
			if !u.task.StartDate.Equal(storyStart) || !u.task.EndDate.Equal(storyEnd) {
				if err := s.repo.UpdateGanttTaskDates(ctx, u.task.ID, storyStart, storyEnd); err != nil {
					return fmt.Errorf("%s: update story task dates: %w", op, err)
				}
			}
			if err := s.repo.UpdateGanttTaskProgress(ctx, u.task.ID, storyProgress); err != nil {
				return fmt.Errorf("%s: update story task progress: %w", op, err)
			}
		}

		if !epicHasBounds || storyStart.Before(epicStart) {
			epicStart = storyStart
		}
		if !epicHasBounds || storyEnd.After(epicEnd) {
			epicEnd = storyEnd
		}
		epicHasBounds = true

		epicProgressSum += storyProgress * storyWeightSum
		epicWeightSum += storyWeightSum
	}

	if !epicHasBounds {
		return nil
	}

	epicProgress := 0.0
	if epicWeightSum > 0 {
		epicProgress = epicProgressSum / epicWeightSum
	}
	if !epicParent.StartDate.Equal(epicStart) || !epicParent.EndDate.Equal(epicEnd) {
		if err := s.repo.UpdateGanttTaskDates(ctx, epicParent.ID, epicStart, epicEnd); err != nil {
			return fmt.Errorf("%s: update epic dates: %w", op, err)
		}
	}
	if err := s.repo.UpdateGanttTaskProgress(ctx, epicParent.ID, epicProgress); err != nil {
		return fmt.Errorf("%s: update epic progress: %w", op, err)
	}

	return nil
}

// SetTaskProgress sets the progress of a leaf (role) task and, when it
// reaches 100%, automatically fixes the completion fact (actual end date +
// actual effort in working days between the task's current planned start
// and now). Dropping progress back below 100% on a previously completed
// task clears that fact (reopening it). Progress cannot be set directly on
// a parent (story/epic) task — it's always aggregated from its children.
// Recalculates the whole team's pipeline schedule afterwards, since a fact
// that differs from the plan can reshuffle downstream tasks.
func (s *Service) SetTaskProgress(
	ctx context.Context,
	taskID uuid.UUID,
	progress float64,
) ([]domain.GanttTask, error) {
	op := "gantt.SetTaskProgress"

	task, err := s.repo.GetGanttTaskByID(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("%s: get task: %w", op, err)
	}
	if task.IsParent {
		return nil, fmt.Errorf(
			"%s: progress of a story/epic is aggregated automatically and cannot be set directly", op,
		)
	}

	if err := s.repo.UpdateGanttTaskProgress(ctx, taskID, progress); err != nil {
		return nil, fmt.Errorf("%s: update progress: %w", op, err)
	}

	// Контракт progress между фронтендом и бэкендом — дробь 0.0-1.0 (фронтенд
	// шлёт progress/100, читает progress*100 для Frappe Gantt), поэтому порог
	// фиксации/снятия факта сравнивается с 1, а не со 100.
	switch {
	case progress >= 1 && task.ActualEndDate == nil:
		actualEnd := toMidnight(time.Now())
		effort := max(1, countWorkDays(task.StartDate, actualEnd))
		if err := s.repo.UpdateGanttTaskActuals(ctx, taskID, actualEnd, effort); err != nil {
			return nil, fmt.Errorf("%s: update actuals: %w", op, err)
		}
	case progress < 1 && task.ActualEndDate != nil:
		if err := s.repo.ClearGanttTaskActuals(ctx, taskID); err != nil {
			return nil, fmt.Errorf("%s: clear actuals: %w", op, err)
		}
	}

	epic, err := s.repo.GetEpicByID(ctx, task.EpicID)
	if err != nil {
		return nil, fmt.Errorf("%s: get epic: %w", op, err)
	}

	result, err := s.RecalculateTeamSchedule(ctx, epic.TeamID)
	if err != nil {
		return nil, fmt.Errorf("%s: recalc schedule: %w", op, err)
	}
	return result, nil
}

// SetTaskStartOffset sets the start offset (lead/lag, in days) of a leaf
// (role) Gantt task — see recalculateEpicSchedule for how it's applied.
// Cannot be set on a parent (story/epic) task, which has no role of its
// own to offset. Stored in task_assignments (design.md Решение 4/5), so it
// survives task regeneration, unlike gantt_tasks.start_offset_days before
// it. Recalculates the whole team's pipeline schedule afterwards.
func (s *Service) SetTaskStartOffset(
	ctx context.Context,
	taskID uuid.UUID,
	offsetDays int,
) ([]domain.GanttTask, error) {
	op := "gantt.SetTaskStartOffset"

	task, err := s.repo.GetGanttTaskByID(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("%s: get task: %w", op, err)
	}
	if task.IsParent {
		return nil, fmt.Errorf(
			"%s: start offset can only be set on a leaf (role) task, not a story/epic", op,
		)
	}

	assignmentEpicID, err := s.resolveAssignmentKey(ctx, task)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	if err := s.repo.UpsertTaskAssignmentStartOffset(ctx, assignmentEpicID, *task.RoleID, offsetDays); err != nil {
		return nil, fmt.Errorf("%s: update start offset: %w", op, err)
	}

	epic, err := s.repo.GetEpicByID(ctx, task.EpicID)
	if err != nil {
		return nil, fmt.Errorf("%s: get epic: %w", op, err)
	}

	result, err := s.RecalculateTeamSchedule(ctx, epic.TeamID)
	if err != nil {
		return nil, fmt.Errorf("%s: recalc schedule: %w", op, err)
	}
	return result, nil
}

// SetTaskAssignee pins (userID != nil) or unpins (userID == nil, i.e.
// "Automatically") the executor of a leaf (role) Gantt task. The pin is
// stored in task_assignments (design.md Решение 4) — the actual
// gantt_tasks.assignee_id is a derived result that recalculateEpicSchedule
// fills in on the recalculation this call triggers below (Решение 3: the
// pin has priority over the automatic choice of who, not over when — if
// the pinned person is busy, the task waits for them). Whether userID is a
// current candidate of the task's role is the caller's (HTTP handler's)
// responsibility to validate before calling this — see design.md Решение 7
// for what happens here if it isn't (or stops being one later): the
// assignment row is kept regardless, and the scheduler simply falls back to
// automatic distribution until the person is a candidate again.
func (s *Service) SetTaskAssignee(
	ctx context.Context,
	taskID uuid.UUID,
	userID *uuid.UUID,
) ([]domain.GanttTask, error) {
	op := "gantt.SetTaskAssignee"

	task, err := s.repo.GetGanttTaskByID(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("%s: get task: %w", op, err)
	}
	if task.IsParent {
		return nil, fmt.Errorf(
			"%s: assignee can only be set on a leaf (role) task, not a story/epic", op,
		)
	}

	assignmentEpicID, err := s.resolveAssignmentKey(ctx, task)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	if err := s.repo.UpsertTaskAssignmentUser(ctx, assignmentEpicID, *task.RoleID, userID); err != nil {
		return nil, fmt.Errorf("%s: update assignment: %w", op, err)
	}

	epic, err := s.repo.GetEpicByID(ctx, task.EpicID)
	if err != nil {
		return nil, fmt.Errorf("%s: get epic: %w", op, err)
	}

	result, err := s.RecalculateTeamSchedule(ctx, epic.TeamID)
	if err != nil {
		return nil, fmt.Errorf("%s: recalc schedule: %w", op, err)
	}
	return result, nil
}

// resolveAssignmentKey returns the task_assignments.epic_id under which a
// leaf (role) task's manual input (pin/offset) is stored — the ID of the
// STORY that owns the task, or the top epic itself for legacy epics
// without stories. This is deliberately NOT task.EpicID: that field is
// always the top epic's ID for every leaf task of an epic regardless of
// which story it belongs to (see design.md Context), so it can't
// distinguish stories sharing the same parent epic.
//
// Resolved by walking up to the task's immediate parent (a story-level or
// the epic-level Gantt row) and, if it's a story, matching its name against
// real domain.Epic stories the same way recalculateEpicSchedule and
// migration 011 do — via "<number>: <name>". If no story matches (e.g. it
// was renamed after generation — see design.md Решение 5 "Риск сравнения
// имён"), falls back to the top epic ID, mirroring the migration's
// COALESCE, so the input still lands *somewhere* stable rather than being
// rejected outright.
func (s *Service) resolveAssignmentKey(ctx context.Context, task *domain.GanttTask) (uuid.UUID, error) {
	op := "gantt.resolveAssignmentKey"

	if task.RoleID == nil {
		return uuid.Nil, fmt.Errorf("%s: task %s has no role", op, task.ID)
	}
	if task.ParentTaskID == nil {
		return uuid.Nil, fmt.Errorf("%s: task %s has no parent", op, task.ID)
	}

	parent, err := s.repo.GetGanttTaskByID(ctx, *task.ParentTaskID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("%s: get parent task: %w", op, err)
	}
	if parent.ParentTaskID == nil {
		// The parent IS the epic's own root task -> legacy epic without stories.
		return task.EpicID, nil
	}

	stories, err := s.repo.GetStoriesByEpicID(ctx, task.EpicID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("%s: get stories: %w", op, err)
	}
	for _, story := range stories {
		if fmt.Sprintf("%s: %s", story.Number, story.Name) == parent.Name {
			return story.ID, nil
		}
	}

	return task.EpicID, nil
}

// GetTeamMembers returns a team's roster with the full set of roles per
// member — unlike GetRoleByUserID (a single role per user), this correctly
// reflects user_roles as an M:N relation, so a member with two roles is
// returned with both. Used as the source of assignee candidates for manual
// pinning (design.md Решение 9).
func (s *Service) GetTeamMembers(ctx context.Context, teamID uuid.UUID) ([]domain.TeamMember, error) {
	op := "gantt.GetTeamMembers"

	users, err := s.repo.GetUsersByTeamID(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("%s: get team users: %w", op, err)
	}

	members := make([]domain.TeamMember, 0, len(users))
	for _, u := range users {
		roles, err := s.repo.GetUserRoles(ctx, u.ID)
		if err != nil {
			return nil, fmt.Errorf("%s: get roles for user %s: %w", op, u.ID, err)
		}
		roleIDs := make([]uuid.UUID, len(roles))
		for i, r := range roles {
			roleIDs[i] = r.ID
		}
		members = append(members, domain.TeamMember{
			ID:        u.ID,
			FirstName: u.FirstName,
			LastName:  u.LastName,
			RoleIDs:   roleIDs,
		})
	}
	return members, nil
}

// GetTeamTasks returns Gantt tasks for a team ordered hierarchically:
// epic -> its stories (by sort_order) -> role tasks of each story (by sort_order).
// Repository.GetGanttTasksByTeamID sorts tasks in a flat space
// (ORDER BY e.sort_order, e.number, sort_order, name) without grouping by
// parent_task_id, so rows of different levels/stories end up interleaved.
// Rebuild the correct order explicitly here instead of touching the SQL/stored values.
func (s *Service) GetTeamTasks(ctx context.Context, teamID uuid.UUID) ([]domain.GanttTask, error) {
	op := "gantt.GetTeamTasks"

	tasks, err := s.repo.GetGanttTasksByTeamID(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return orderTasksHierarchically(tasks), nil
}

// GetTeamTasksWithAssignments returns a team's Gantt tasks (see
// GetTeamTasks) together with, for every leaf (role) task that has one, its
// resolved task_assignments row (manual assignee pin and/or start offset) —
// keyed by gantt_tasks.id for direct lookup by the caller.
//
// This is the single shared read path for both HTTP needs that require
// task_assignments data: assignee_id/assignee_name/assignee_is_manual and
// the "pin no longer effective" flag (handlers.ganttTaskResp, backend §3.1),
// and the actually-applied start_offset_days (backend §3.5) — deliberately
// one call instead of two independent lookups scattered across the handler.
//
// The story/epic key each task_assignments row is keyed on (see
// domain.TaskAssignment) is resolved once per top-level epic present in
// tasks, not once per leaf task, to avoid N+1 queries on an endpoint that's
// polled on every progress change (see design.md Risks, "Рост числа
// запросов к БД").
func (s *Service) GetTeamTasksWithAssignments(
	ctx context.Context,
	teamID uuid.UUID,
) ([]domain.GanttTask, map[uuid.UUID]domain.TaskAssignment, error) {
	op := "gantt.GetTeamTasksWithAssignments"

	tasks, err := s.GetTeamTasks(ctx, teamID)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", op, err)
	}

	assignmentRows, err := s.repo.GetTaskAssignmentsByTeamID(ctx, teamID)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: get task assignments: %w", op, err)
	}
	byKey := make(map[assignmentKey]domain.TaskAssignment, len(assignmentRows))
	for _, a := range assignmentRows {
		byKey[assignmentKey{epicID: a.EpicID, roleID: a.RoleID}] = a
	}

	// tasksByEpic группирует все строки (родительские и листовые) по
	// верхнему эпику: gantt_tasks.epic_id одинаков у всех задач одного
	// эпика, включая листовые задачи любой его стори (см. design.md Context).
	tasksByEpic := make(map[uuid.UUID][]domain.GanttTask)
	for _, t := range tasks {
		tasksByEpic[t.EpicID] = append(tasksByEpic[t.EpicID], t)
	}

	result := make(map[uuid.UUID]domain.TaskAssignment)
	for epicID, epicTasks := range tasksByEpic {
		storyOrEpicIDByParent, err := s.storyOrEpicIDByParentTaskID(ctx, epicID, epicTasks)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", op, err)
		}
		for _, t := range epicTasks {
			if t.IsParent || t.RoleID == nil || t.ParentTaskID == nil {
				continue
			}
			storyOrEpicID, ok := storyOrEpicIDByParent[*t.ParentTaskID]
			if !ok {
				continue
			}
			if a, ok := byKey[assignmentKey{epicID: storyOrEpicID, roleID: *t.RoleID}]; ok {
				result[t.ID] = a
			}
		}
	}

	return tasks, result, nil
}

// storyOrEpicIDByParentTaskID resolves, for every parent-level Gantt row of
// a single top-level epic (its own root row, plus each of its story rows),
// the task_assignments.epic_id key that row's leaf tasks are stored under
// (see assignmentKey/domain.TaskAssignment) — the ID of the domain Epic
// that row represents. For the epic's own root row that's simply epicID
// (also what legacy epics without stories use for all their leaf tasks,
// which are parented directly to the root row); for a story row it's
// resolved by matching its name against real domain.Epic stories, the same
// "<number>: <name>" comparison recalculateEpicSchedule and migration 011
// use, falling back to epicID if no story matches (e.g. renamed since
// generation — design.md Решение 5).
func (s *Service) storyOrEpicIDByParentTaskID(
	ctx context.Context,
	epicID uuid.UUID,
	epicTasks []domain.GanttTask,
) (map[uuid.UUID]uuid.UUID, error) {
	op := "gantt.storyOrEpicIDByParentTaskID"

	stories, err := s.repo.GetStoriesByEpicID(ctx, epicID)
	if err != nil {
		return nil, fmt.Errorf("%s: get stories: %w", op, err)
	}
	storyIDByName := make(map[string]uuid.UUID, len(stories))
	for _, story := range stories {
		storyIDByName[fmt.Sprintf("%s: %s", story.Number, story.Name)] = story.ID
	}

	result := make(map[uuid.UUID]uuid.UUID)
	for _, t := range epicTasks {
		if !t.IsParent {
			continue
		}
		if t.ParentTaskID == nil {
			result[t.ID] = epicID
			continue
		}
		if storyID, ok := storyIDByName[t.Name]; ok {
			result[t.ID] = storyID
		} else {
			result[t.ID] = epicID
		}
	}
	return result, nil
}

// orderTasksHierarchically restores correct parent-child row order.
// Roots (tasks without a parent, i.e. epics) keep their incoming relative
// order — the SQL query already returns them ordered by epics.sort_order.
// Each parent's direct children are grouped together and sorted stably
// by (SortOrder, Name), then the tree is walked depth-first so that every
// story's role rows immediately follow that story, regardless of depth.
func orderTasksHierarchically(tasks []domain.GanttTask) []domain.GanttTask {
	children := make(map[uuid.UUID][]domain.GanttTask)
	var roots []domain.GanttTask
	for _, t := range tasks {
		if t.ParentTaskID == nil {
			roots = append(roots, t)
			continue
		}
		children[*t.ParentTaskID] = append(children[*t.ParentTaskID], t)
	}

	// sort_order сравним только между прямыми siblings (общий ParentTaskID) —
	// между уровнями иерархии значения sort_order не связаны.
	for parentID, group := range children {
		slices.SortStableFunc(group, func(a, b domain.GanttTask) int {
			if a.SortOrder != b.SortOrder {
				return a.SortOrder - b.SortOrder
			}
			if a.Name < b.Name {
				return -1
			}
			if a.Name > b.Name {
				return 1
			}
			return 0
		})
		children[parentID] = group
	}

	result := make([]domain.GanttTask, 0, len(tasks))
	for _, root := range roots {
		result = appendSubtree(result, root, children)
	}
	return result
}

// appendSubtree appends a task and its descendants (DFS) to result,
// working for hierarchies of arbitrary depth.
func appendSubtree(
	result []domain.GanttTask,
	task domain.GanttTask,
	children map[uuid.UUID][]domain.GanttTask,
) []domain.GanttTask {
	result = append(result, task)
	for _, child := range children[task.ID] {
		result = appendSubtree(result, child, children)
	}
	return result
}

// groupGanttTasksBySortOrder groups already-persisted role tasks by their
// sort_order (roles meant to run in parallel share the same value),
// preserving the relative order of groups and of tasks within a group.
func groupGanttTasksBySortOrder(tasks []domain.GanttTask) [][]domain.GanttTask {
	if len(tasks) == 0 {
		return nil
	}
	sorted := make([]domain.GanttTask, len(tasks))
	copy(sorted, tasks)
	slices.SortStableFunc(sorted, func(a, b domain.GanttTask) int {
		return a.SortOrder - b.SortOrder
	})

	var groups [][]domain.GanttTask
	var current []domain.GanttTask
	currentOrder := sorted[0].SortOrder
	for _, t := range sorted {
		if t.SortOrder != currentOrder {
			groups = append(groups, current)
			current = nil
			currentOrder = t.SortOrder
		}
		current = append(current, t)
	}
	if len(current) > 0 {
		groups = append(groups, current)
	}
	return groups
}

// ReorderTask changes a role task's sort_order within its story (or,
// for legacy epics, within the epic) and recalculates the whole team's
// pipeline schedule.
func (s *Service) ReorderTask(
	ctx context.Context,
	taskID uuid.UUID,
	newSortOrder int,
) ([]domain.GanttTask, error) {
	op := "gantt.ReorderTask"

	task, err := s.repo.GetGanttTaskByID(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("%s: get task: %w", op, err)
	}
	if task.IsParent {
		return nil, fmt.Errorf(
			"%s: cannot reorder parent task", op,
		)
	}
	if task.ParentTaskID == nil {
		return nil, fmt.Errorf(
			"%s: child task has no parent", op,
		)
	}

	if err := s.repo.UpdateGanttTaskSortOrder(
		ctx, taskID, newSortOrder,
	); err != nil {
		return nil, fmt.Errorf("%s: update sort: %w", op, err)
	}

	epic, err := s.repo.GetEpicByID(ctx, task.EpicID)
	if err != nil {
		return nil, fmt.Errorf("%s: get epic: %w", op, err)
	}

	return s.RecalculateTeamSchedule(ctx, epic.TeamID)
}

// ReorderEpic changes a top-level epic's place in its team's pipeline queue
// and recalculates the whole team's schedule.
func (s *Service) ReorderEpic(
	ctx context.Context,
	epicID uuid.UUID,
	newSortOrder int,
) ([]domain.GanttTask, error) {
	op := "gantt.ReorderEpic"

	epic, err := s.repo.GetEpicByID(ctx, epicID)
	if err != nil {
		return nil, fmt.Errorf("%s: get epic: %w", op, err)
	}
	if epic.ParentEpicID != nil {
		return nil, fmt.Errorf("%s: use ReorderStory to reorder a story", op)
	}

	if err := s.repo.UpdateEpicSortOrder(ctx, epicID, newSortOrder); err != nil {
		return nil, fmt.Errorf("%s: update sort: %w", op, err)
	}

	return s.RecalculateTeamSchedule(ctx, epic.TeamID)
}

// ReorderStory changes a story's place in its parent epic's pipeline queue.
// If the story already has a generated Gantt row, the sort_order of the
// corresponding story-level gantt_tasks rows among its siblings is
// re-synced to match the new epics.sort_order order, so the rendered tree
// (orderTasksHierarchically, which sorts by gantt_tasks.sort_order) stays
// consistent with the pipeline order. Recalculates the whole team's
// schedule afterwards.
func (s *Service) ReorderStory(
	ctx context.Context,
	storyID uuid.UUID,
	newSortOrder int,
) ([]domain.GanttTask, error) {
	op := "gantt.ReorderStory"

	story, err := s.repo.GetEpicByID(ctx, storyID)
	if err != nil {
		return nil, fmt.Errorf("%s: get story: %w", op, err)
	}
	if story.ParentEpicID == nil {
		return nil, fmt.Errorf("%s: use ReorderEpic to reorder a top-level epic", op)
	}
	parentEpicID := *story.ParentEpicID

	if err := s.repo.UpdateEpicSortOrder(ctx, storyID, newSortOrder); err != nil {
		return nil, fmt.Errorf("%s: update sort: %w", op, err)
	}

	if err := s.syncStoryGanttSortOrder(ctx, parentEpicID); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	epic, err := s.repo.GetEpicByID(ctx, parentEpicID)
	if err != nil {
		return nil, fmt.Errorf("%s: get parent epic: %w", op, err)
	}

	return s.RecalculateTeamSchedule(ctx, epic.TeamID)
}

// syncStoryGanttSortOrder re-numbers the sort_order of story-level
// gantt_tasks rows under parentEpicID to match the current epics.sort_order
// order of its stories. No-op if the parent epic has no generated tasks yet.
func (s *Service) syncStoryGanttSortOrder(ctx context.Context, parentEpicID uuid.UUID) error {
	op := "gantt.syncStoryGanttSortOrder"

	hasTasks, err := s.repo.HasGanttTasksForEpic(ctx, parentEpicID)
	if err != nil {
		return fmt.Errorf("%s: check tasks: %w", op, err)
	}
	if !hasTasks {
		return nil
	}

	siblings, err := s.repo.GetStoriesByEpicID(ctx, parentEpicID)
	if err != nil {
		return fmt.Errorf("%s: get sibling stories: %w", op, err)
	}

	epicTasks, err := s.repo.GetGanttTasksByEpicID(ctx, parentEpicID)
	if err != nil {
		return fmt.Errorf("%s: get epic tasks: %w", op, err)
	}

	taskIDByStoryName := make(map[string]uuid.UUID)
	for _, t := range epicTasks {
		if t.IsParent && t.ParentTaskID != nil {
			taskIDByStoryName[t.Name] = t.ID
		}
	}

	for i, sibling := range siblings {
		name := fmt.Sprintf("%s: %s", sibling.Number, sibling.Name)
		taskID, ok := taskIDByStoryName[name]
		if !ok {
			continue
		}
		if err := s.repo.UpdateGanttTaskSortOrder(ctx, taskID, i+1); err != nil {
			return fmt.Errorf("%s: sync story task sort order: %w", op, err)
		}
	}
	return nil
}

// toMidnight returns a time with the same date as t but set to midnight (00:00:00).
func toMidnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// moveToWorkDay moves the date to Monday if it falls on Saturday or Sunday.
func moveToWorkDay(t time.Time) time.Time {
	switch t.Weekday() {
	case time.Saturday:
		return t.AddDate(0, 0, 2)
	case time.Sunday:
		return t.AddDate(0, 0, 1)
	default:
		return t
	}
}

// addWorkDays adds days - 1 working days to start.
func addWorkDays(start time.Time, days int) time.Time {
	curr := moveToWorkDay(start)
	if days <= 1 {
		return curr
	}
	remaining := days - 1
	for remaining > 0 {
		curr = curr.AddDate(0, 0, 1)
		if curr.Weekday() != time.Saturday && curr.Weekday() != time.Sunday {
			remaining--
		}
	}
	return curr
}

// countWorkDays counts the number of working days between start and end inclusive.
func countWorkDays(start, end time.Time) int {
	s := toMidnight(start)
	e := toMidnight(end)
	if s.After(e) {
		return 0
	}
	count := 0
	for !s.After(e) {
		if s.Weekday() != time.Saturday && s.Weekday() != time.Sunday {
			count++
		}
		s = s.AddDate(0, 0, 1)
	}
	return count
}

// maxTime returns the latest of the given times (zero-value times.Time{}
// compare as the earliest possible date, so callers don't need to special-
// case "not set yet" values such as roleFreeAt's zero-value default).
func maxTime(times ...time.Time) time.Time {
	m := times[0]
	for _, t := range times[1:] {
		if t.After(m) {
			m = t
		}
	}
	return m
}

// nextAvailable converts a calendar's raw "busy until" timestamp (the
// effective end of the last task assigned to a person/role) into the
// earliest work day a new task for that same person/role could start —
// advanced by one work day, matching groupPrevEnd's own "+1 work day"
// semantics. A zero time.Time (nothing assigned yet) is returned unchanged:
// maxTime treats zero as always earliest, so it never constrains selection.
func nextAvailable(freeAt time.Time) time.Time {
	if freeAt.IsZero() {
		return freeAt
	}
	return moveToWorkDay(freeAt.AddDate(0, 0, 1))
}

// advanceFreeAt records effectiveEnd as the calendar's new "busy until" mark
// for key, but only if it's later than what's already recorded — a
// calendar can only move forward, never backward, within one
// RecalculateTeamSchedule call.
func advanceFreeAt(freeAt map[uuid.UUID]time.Time, key uuid.UUID, effectiveEnd time.Time) {
	if cur, ok := freeAt[key]; !ok || effectiveEnd.After(cur) {
		freeAt[key] = effectiveEnd
	}
}

// poolContains reports whether userID is among the role's current
// candidates — used to tell an effective manual pin (design.md Решение 3)
// from one that no longer applies because the pinned person left the team
// or lost the role (Решение 7).
func poolContains(pool []domain.User, userID uuid.UUID) bool {
	for _, u := range pool {
		if u.ID == userID {
			return true
		}
	}
	return false
}

// assigneeIDEqual compares two possibly-nil assignee IDs for equality —
// used to decide whether GanttTask.AssigneeID actually needs a DB write
// this pass (both parent-less, i.e. no assignee vs no assignee, count as
// equal too).
func assigneeIDEqual(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// pickAssignee selects, from pool (must be non-empty, sorted by uuid — see
// design.md Решение 2), the candidate with the earliest start(u) as
// computed by start. Ties are broken by round-robin: the scan begins at
// cursor and wraps around, so the first candidate reached that matches the
// minimum wins, and nextCursor points right after it — spreading equally
// good candidates across calls instead of always picking the same one.
func pickAssignee(
	pool []domain.User,
	cursor int,
	start func(userID uuid.UUID) time.Time,
) (chosen uuid.UUID, chosenStart time.Time, nextCursor int) {
	bestIdx := -1
	for i := range pool {
		idx := (cursor + i) % len(pool)
		s := start(pool[idx].ID)
		if bestIdx == -1 || s.Before(chosenStart) {
			bestIdx = idx
			chosenStart = s
		}
	}
	chosen = pool[bestIdx].ID
	nextCursor = (bestIdx + 1) % len(pool)
	return chosen, chosenStart, nextCursor
}
