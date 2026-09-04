package gantt

import (
	"EpicScoreBot/internal/models/domain"
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

// GenerateTasksForEpic creates Gantt task rows for a scored epic: a parent
// task (the epic itself), a wrapper task per story (or, for legacy epics
// without stories, role tasks directly under the epic), and a role task per
// scored role. It does NOT lay out dates itself — that's the job of the
// global pipeline scheduler (RecalculateTeamSchedule), which this method
// invokes once all rows exist. startDate only seeds the epic's initial
// "floor" (its parent task's StartDate), used by the scheduler as the
// earliest possible start for this epic's own tasks.
func (s *Service) GenerateTasksForEpic(
	ctx context.Context,
	epicID uuid.UUID,
	startDate time.Time,
) ([]domain.GanttTask, error) {
	op := "gantt.GenerateTasksForEpic"

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

	result, err := s.RecalculateTeamSchedule(ctx, epic.TeamID)
	if err != nil {
		return nil, fmt.Errorf("%s: recalc schedule: %w", op, err)
	}

	s.log.Info("generated gantt tasks",
		slog.String("epicID", epicID.String()),
		slog.Int("taskCount", len(result)))

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

	// roleFreeAt tracks, per role, the effective completion time of that
	// role's latest task processed so far. Recomputed from scratch on every
	// call so there's no state drift between calls.
	roleFreeAt := make(map[uuid.UUID]time.Time)

	for _, ewt := range inScope {
		if err := s.recalculateEpicSchedule(ctx, ewt.epic, ewt.tasks, teamFloor, roleFreeAt); err != nil {
			return nil, fmt.Errorf("%s: epic %s: %w", op, ewt.epic.ID, err)
		}
	}

	return s.GetTeamTasks(ctx, teamID)
}

// recalculateEpicSchedule recalculates dates and aggregated progress for a
// single epic's tasks (its stories/legacy role tasks), advancing roleFreeAt
// as it goes. teamFloor is the team-wide lower bound (see RecalculateTeamSchedule).
func (s *Service) recalculateEpicSchedule(
	ctx context.Context,
	epic domain.Epic,
	epicTasks []domain.GanttTask,
	teamFloor time.Time,
	roleFreeAt map[uuid.UUID]time.Time,
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
			units = append(units, unit{task: st, roles: roleTasksByParent[st.ID]})
		}
	} else {
		units = append(units, unit{task: epicParent, roles: roleTasksByParent[epicParent.ID]})
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

				var effectiveEnd time.Time
				frozen := task.Progress > 0 || task.ActualEndDate != nil
				if frozen {
					effectiveEnd = task.EndDate
					if task.ActualEndDate != nil {
						effectiveEnd = *task.ActualEndDate
					}
				} else {
					// roleFreeAt stores the last busy day of the role's
					// previous task (its "effective end"), not the next
					// available day — advance it by one work day here so
					// the new task starts strictly after the previous one
					// ends, matching groupPrevEnd's own "+1 work day" semantics.
					roleNextAvailable := roleFreeAt[roleID]
					if !roleNextAvailable.IsZero() {
						roleNextAvailable = moveToWorkDay(roleNextAvailable.AddDate(0, 0, 1))
					}
					// StartOffsetDays — lead/lag на FS-зависимости между ролевыми
					// группами внутри стори: сдвигает groupPrevEnd на целое число
					// календарных дней (отрицательное значение — начать раньше,
					// положительное — намеренная задержка). roleNextAvailable и
					// epicFloor остаются жёсткими нижними границами через maxTime —
					// офсет не может нарушить непрерывность самой роли в конвейере.
					target := groupPrevEnd.AddDate(0, 0, task.StartOffsetDays)
					newStart := moveToWorkDay(maxTime(target, roleNextAvailable, epicFloor))
					newEnd := addWorkDays(newStart, workDays)
					if !newStart.Equal(task.StartDate) || !newEnd.Equal(task.EndDate) {
						if err := s.repo.UpdateGanttTaskDates(ctx, task.ID, newStart, newEnd); err != nil {
							return fmt.Errorf("%s: update role task: %w", op, err)
						}
					}
					task.StartDate = newStart
					task.EndDate = newEnd
					effectiveEnd = newEnd
				}

				if cur, ok := roleFreeAt[roleID]; !ok || effectiveEnd.After(cur) {
					roleFreeAt[roleID] = effectiveEnd
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
// own to offset. Recalculates the whole team's pipeline schedule afterwards.
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

	if err := s.repo.UpdateGanttTaskStartOffset(ctx, taskID, offsetDays); err != nil {
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
