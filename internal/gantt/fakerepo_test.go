package gantt

import (
	"EpicScoreBot/internal/models/domain"
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// fakeRepo — простой стейтфул-репозиторий в памяти, реализующий gantt.Repository.
// Используется вместо function-per-field моков там, где тесты проверяют
// сквозное поведение конвейерного планировщика (RecalculateTeamSchedule),
// которое многократно читает и пишет эпики/задачи в рамках одного вызова —
// закрытие-моки для этого было бы громоздко и хрупко.
type fakeRepo struct {
	epics      map[uuid.UUID]*domain.Epic
	tasks      map[uuid.UUID]*domain.GanttTask
	roles      map[uuid.UUID]*domain.Role
	roleScores map[uuid.UUID][]domain.EpicRoleScore // epicID/storyID -> role scores
	risks      map[uuid.UUID][]domain.Risk          // epicID/storyID -> risks

	// Инструментарий только для тестов блокировки (см. locking_test.go):
	// GetTeamEpicsOrdered — первый repo-вызов внутри RecalculateTeamSchedule,
	// GetGanttTasksByTeamID (через GetTeamTasks) — последний. Используются как
	// граница "открыт/закрыт" для подсчёта, сколько пересчётов одной команды
	// выполняются одновременно (activeRecalcs/maxActiveRecalcs), плюс
	// искусственная задержка (recalcDelay) внутри первого вызова, чтобы
	// гарантированно создать окно конкуренции для второй горутины.
	concurrencyMu     sync.Mutex
	activeRecalcs     int
	maxActiveRecalcs  int
	recalcDelay       time.Duration
	recalcDelayTeamID uuid.UUID // если задан, задержка применяется только к этой команде
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		epics:      make(map[uuid.UUID]*domain.Epic),
		tasks:      make(map[uuid.UUID]*domain.GanttTask),
		roles:      make(map[uuid.UUID]*domain.Role),
		roleScores: make(map[uuid.UUID][]domain.EpicRoleScore),
		risks:      make(map[uuid.UUID][]domain.Risk),
	}
}

func (f *fakeRepo) addEpic(e *domain.Epic) *domain.Epic {
	cp := *e
	f.epics[cp.ID] = &cp
	return &cp
}

func (f *fakeRepo) addRole(r *domain.Role) {
	f.roles[r.ID] = r
}

// compareEpicOrder mirrors "ORDER BY sort_order NULLS LAST, number".
func compareEpicOrder(a, b *domain.Epic) int {
	switch {
	case a.SortOrder == nil && b.SortOrder == nil:
		// fallthrough to number comparison below
	case a.SortOrder == nil:
		return 1
	case b.SortOrder == nil:
		return -1
	case *a.SortOrder != *b.SortOrder:
		return *a.SortOrder - *b.SortOrder
	}
	if a.Number < b.Number {
		return -1
	}
	if a.Number > b.Number {
		return 1
	}
	return 0
}

func (f *fakeRepo) GetEpicByID(ctx context.Context, epicID uuid.UUID) (*domain.Epic, error) {
	e, ok := f.epics[epicID]
	if !ok {
		return nil, errors.New("epic not found")
	}
	cp := *e
	return &cp, nil
}

func (f *fakeRepo) GetEpicsByTeamIDAndStatus(ctx context.Context, teamID uuid.UUID, status domain.Status) ([]domain.Epic, error) {
	var res []domain.Epic
	for _, e := range f.epics {
		if e.TeamID == teamID && e.Status == status && e.ParentEpicID == nil {
			res = append(res, *e)
		}
	}
	sort.Slice(res, func(i, j int) bool { return compareEpicOrder(&res[i], &res[j]) < 0 })
	return res, nil
}

func (f *fakeRepo) GetTeamEpicsOrdered(ctx context.Context, teamID uuid.UUID) ([]domain.Epic, error) {
	// Начало "окна" пересчёта — см. комментарий про concurrencyMu в структуре fakeRepo.
	f.concurrencyMu.Lock()
	f.activeRecalcs++
	if f.activeRecalcs > f.maxActiveRecalcs {
		f.maxActiveRecalcs = f.activeRecalcs
	}
	var delay time.Duration
	if f.recalcDelay > 0 && (f.recalcDelayTeamID == uuid.Nil || f.recalcDelayTeamID == teamID) {
		delay = f.recalcDelay
	}
	f.concurrencyMu.Unlock()

	if delay > 0 {
		time.Sleep(delay)
	}

	var res []*domain.Epic
	for _, e := range f.epics {
		if e.TeamID == teamID && e.ParentEpicID == nil {
			res = append(res, e)
		}
	}
	sort.Slice(res, func(i, j int) bool { return compareEpicOrder(res[i], res[j]) < 0 })
	out := make([]domain.Epic, len(res))
	for i, e := range res {
		out[i] = *e
	}
	return out, nil
}

func (f *fakeRepo) UpdateEpicSortOrder(ctx context.Context, epicID uuid.UUID, sortOrder int) error {
	e, ok := f.epics[epicID]
	if !ok {
		return errors.New("epic not found")
	}
	so := sortOrder
	e.SortOrder = &so
	return nil
}

func (f *fakeRepo) GetAllRoles(ctx context.Context) ([]domain.Role, error) {
	var res []domain.Role
	for _, r := range f.roles {
		res = append(res, *r)
	}
	return res, nil
}

func (f *fakeRepo) GetRoleByID(ctx context.Context, roleID uuid.UUID) (*domain.Role, error) {
	r, ok := f.roles[roleID]
	if !ok {
		return nil, errors.New("role not found")
	}
	cp := *r
	return &cp, nil
}

func (f *fakeRepo) GetAllTeams(ctx context.Context) ([]domain.Team, error) { return nil, nil }

func (f *fakeRepo) GetTeamByID(ctx context.Context, teamID uuid.UUID) (*domain.Team, error) {
	return &domain.Team{ID: teamID}, nil
}

func (f *fakeRepo) GetEpicRoleScoresByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.EpicRoleScore, error) {
	return f.roleScores[epicID], nil
}

func (f *fakeRepo) CreateGanttTask(ctx context.Context, task *domain.GanttTask) (*domain.GanttTask, error) {
	cp := *task
	if cp.ID == uuid.Nil {
		cp.ID = uuid.New()
	}
	now := time.Now()
	cp.CreatedAt = now
	cp.UpdatedAt = now
	f.tasks[cp.ID] = &cp
	res := cp
	return &res, nil
}

func (f *fakeRepo) GetGanttTasksByTeamID(ctx context.Context, teamID uuid.UUID) ([]domain.GanttTask, error) {
	var res []*domain.GanttTask
	for _, t := range f.tasks {
		epic, ok := f.epics[t.EpicID]
		if !ok || epic.TeamID != teamID {
			continue
		}
		res = append(res, t)
	}
	sort.Slice(res, func(i, j int) bool {
		ei, ej := f.epics[res[i].EpicID], f.epics[res[j].EpicID]
		if c := compareEpicOrder(ei, ej); c != 0 {
			return c < 0
		}
		if res[i].SortOrder != res[j].SortOrder {
			return res[i].SortOrder < res[j].SortOrder
		}
		return res[i].Name < res[j].Name
	})
	out := make([]domain.GanttTask, len(res))
	for i, t := range res {
		out[i] = *t
	}

	// Конец "окна" пересчёта — см. комментарий про concurrencyMu в структуре fakeRepo.
	f.concurrencyMu.Lock()
	f.activeRecalcs--
	f.concurrencyMu.Unlock()

	return out, nil
}

func (f *fakeRepo) GetGanttTasksByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.GanttTask, error) {
	var res []*domain.GanttTask
	for _, t := range f.tasks {
		if t.EpicID == epicID {
			res = append(res, t)
		}
	}
	sort.Slice(res, func(i, j int) bool {
		if res[i].SortOrder != res[j].SortOrder {
			return res[i].SortOrder < res[j].SortOrder
		}
		return res[i].Name < res[j].Name
	})
	out := make([]domain.GanttTask, len(res))
	for i, t := range res {
		out[i] = *t
	}
	return out, nil
}

func (f *fakeRepo) GetGanttTaskByID(ctx context.Context, taskID uuid.UUID) (*domain.GanttTask, error) {
	t, ok := f.tasks[taskID]
	if !ok {
		return nil, errors.New("task not found")
	}
	cp := *t
	return &cp, nil
}

func (f *fakeRepo) GetGanttChildTasks(ctx context.Context, parentTaskID uuid.UUID) ([]domain.GanttTask, error) {
	var res []*domain.GanttTask
	for _, t := range f.tasks {
		if t.ParentTaskID != nil && *t.ParentTaskID == parentTaskID {
			res = append(res, t)
		}
	}
	sort.Slice(res, func(i, j int) bool {
		if res[i].SortOrder != res[j].SortOrder {
			return res[i].SortOrder < res[j].SortOrder
		}
		return res[i].Name < res[j].Name
	})
	out := make([]domain.GanttTask, len(res))
	for i, t := range res {
		out[i] = *t
	}
	return out, nil
}

func (f *fakeRepo) UpdateGanttTaskDates(ctx context.Context, taskID uuid.UUID, startDate, endDate time.Time) error {
	t, ok := f.tasks[taskID]
	if !ok {
		return errors.New("task not found")
	}
	t.StartDate = startDate
	t.EndDate = endDate
	return nil
}

func (f *fakeRepo) UpdateGanttTaskProgress(ctx context.Context, taskID uuid.UUID, progress float64) error {
	t, ok := f.tasks[taskID]
	if !ok {
		return errors.New("task not found")
	}
	t.Progress = progress
	return nil
}

func (f *fakeRepo) UpdateGanttTaskSortOrder(ctx context.Context, taskID uuid.UUID, sortOrder int) error {
	t, ok := f.tasks[taskID]
	if !ok {
		return errors.New("task not found")
	}
	t.SortOrder = sortOrder
	return nil
}

func (f *fakeRepo) UpdateGanttTaskStartOffset(ctx context.Context, taskID uuid.UUID, offsetDays int) error {
	t, ok := f.tasks[taskID]
	if !ok {
		return errors.New("task not found")
	}
	t.StartOffsetDays = offsetDays
	return nil
}

func (f *fakeRepo) UpdateGanttTaskActuals(ctx context.Context, taskID uuid.UUID, actualEndDate time.Time, effortDays int) error {
	t, ok := f.tasks[taskID]
	if !ok {
		return errors.New("task not found")
	}
	end := actualEndDate
	days := effortDays
	t.ActualEndDate = &end
	t.ActualEffortDays = &days
	return nil
}

func (f *fakeRepo) ClearGanttTaskActuals(ctx context.Context, taskID uuid.UUID) error {
	t, ok := f.tasks[taskID]
	if !ok {
		return errors.New("task not found")
	}
	t.ActualEndDate = nil
	t.ActualEffortDays = nil
	return nil
}

func (f *fakeRepo) DeleteGanttTasksByEpicID(ctx context.Context, epicID uuid.UUID) error {
	for id, t := range f.tasks {
		if t.EpicID == epicID {
			delete(f.tasks, id)
		}
	}
	return nil
}

func (f *fakeRepo) HasGanttTasksForEpic(ctx context.Context, epicID uuid.UUID) (bool, error) {
	for _, t := range f.tasks {
		if t.EpicID == epicID {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeRepo) GetRisksByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.Risk, error) {
	return f.risks[epicID], nil
}

func (f *fakeRepo) GetStoriesByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.Epic, error) {
	var res []*domain.Epic
	for _, e := range f.epics {
		if e.ParentEpicID != nil && *e.ParentEpicID == epicID {
			res = append(res, e)
		}
	}
	sort.Slice(res, func(i, j int) bool { return compareEpicOrder(res[i], res[j]) < 0 })
	out := make([]domain.Epic, len(res))
	for i, e := range res {
		out[i] = *e
	}
	return out, nil
}
