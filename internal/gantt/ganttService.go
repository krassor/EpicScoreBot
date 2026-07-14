package gantt

import (
	"EpicScoreBot/internal/models/domain"
	"context"
	"fmt"
	"log/slog"
	"math"
	"slices"
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

// GenerateTasksForEpic creates Gantt tasks for a scored epic.
// It generates a parent task (the epic itself) and child tasks
// for each role that has scores, laid out sequentially by default
// sort order. 1 SP = 1 day.
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

	roleScores, err := s.repo.GetEpicRoleScoresByEpicID(ctx, epicID)
	if err != nil {
		return nil, fmt.Errorf("%s: get role scores: %w", op, err)
	}
	if len(roleScores) == 0 {
		return nil, fmt.Errorf("%s: no role scores for epic %s", op, epicID)
	}

	risks, err := s.repo.GetRisksByEpicID(ctx, epicID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	finalCoeff := 1.0
	for _, risk := range risks {
		if risk.WeightedScore != nil {
			coeff := RiskCoefficient(*risk.WeightedScore)
			finalCoeff *= coeff
		}
	}

	// Build role info for each score.
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

	// Sort by default order.
	slices.SortFunc(roleTasks, func(a, b roleTask) int {
		if a.sortOrder != b.sortOrder {
			return a.sortOrder - b.sortOrder
		}
		return 0
	})

	adjustedStartDate := moveToWorkDay(startDate)
	// Create parent task (epic).
	parentTask := &domain.GanttTask{
		EpicID:    epicID,
		Name:      fmt.Sprintf("%s: %s", epic.Number, epic.Name),
		StartDate: adjustedStartDate,
		EndDate:   adjustedStartDate, // will be recalculated
		SortOrder: 0,
		IsParent:  true,
	}
	parentTask, err = s.repo.CreateGanttTask(ctx, parentTask)
	if err != nil {
		return nil, fmt.Errorf("%s: create parent: %w", op, err)
	}

	// Lay out child tasks sequentially by sort_order groups.
	var result []domain.GanttTask
	result = append(result, *parentTask)

	cursor := adjustedStartDate
	parentEndDate := adjustedStartDate
	groups := groupBySortOrder(roleTasks)
	for _, group := range groups {
		var groupEnd time.Time
		for _, rt := range group {
			childStart := cursor
			childEnd := addWorkDays(cursor, rt.workDays)
			roleID := rt.roleID
			child := &domain.GanttTask{
				EpicID:       epicID,
				RoleID:       &roleID,
				Name:         rt.roleName,
				StartDate:    childStart,
				EndDate:      childEnd,
				SortOrder:    rt.sortOrder,
				IsParent:     false,
				ParentTaskID: &parentTask.ID,
			}
			child, err = s.repo.CreateGanttTask(ctx, child)
			if err != nil {
				return nil, fmt.Errorf(
					"%s: create child %s: %w", op, rt.roleName, err,
				)
			}
			result = append(result, *child)
			if groupEnd.IsZero() || childEnd.After(groupEnd) {
				groupEnd = childEnd
			}
		}
		if parentEndDate.Before(groupEnd) {
			parentEndDate = groupEnd
		}
		cursor = moveToWorkDay(groupEnd.AddDate(0, 0, 1))
	}

	// Update parent task dates.
	if err := s.repo.UpdateGanttTaskDates(
		ctx, parentTask.ID, adjustedStartDate, parentEndDate,
	); err != nil {
		return nil, fmt.Errorf(
			"%s: update parent dates: %w", op, err,
		)
	}
	result[0].EndDate = parentEndDate

	s.log.Info("generated gantt tasks",
		slog.String("epicID", epicID.String()),
		slog.Int("taskCount", len(result)))

	return result, nil
}

// groupBySortOrder groups role tasks by their sort order,
// preserving the order of groups.
func groupBySortOrder(items []roleTask) [][]roleTask {
	if len(items) == 0 {
		return nil
	}
	var groups [][]roleTask
	var current []roleTask
	currentOrder := items[0].sortOrder
	for _, item := range items {
		if item.sortOrder != currentOrder {
			groups = append(groups, current)
			current = nil
			currentOrder = item.sortOrder
		}
		current = append(current, item)
	}
	if len(current) > 0 {
		groups = append(groups, current)
	}
	return groups
}

// UpdateTaskDates updates a task's dates and recalculates the parent.
func (s *Service) UpdateTaskDates(
	ctx context.Context,
	taskID uuid.UUID,
	startDate, endDate time.Time,
) error {
	op := "gantt.UpdateTaskDates"

	workDays := max(1, countWorkDays(startDate, endDate))
	adjustedStart := moveToWorkDay(startDate)
	adjustedEnd := addWorkDays(adjustedStart, workDays)

	if err := s.repo.UpdateGanttTaskDates(
		ctx, taskID, adjustedStart, adjustedEnd,
	); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	task, err := s.repo.GetGanttTaskByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("%s: get task: %w", op, err)
	}

	// If this is a child task, recalculate parent bounds.
	if task.ParentTaskID != nil {
		if err := s.recalcParentDates(ctx, *task.ParentTaskID); err != nil {
			return fmt.Errorf("%s: recalc parent: %w", op, err)
		}
	}

	return nil
}

// ReorderTask changes a task's sort_order and recalculates sibling dates.
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

	if err := s.repo.UpdateGanttTaskSortOrder(
		ctx, taskID, newSortOrder,
	); err != nil {
		return nil, fmt.Errorf("%s: update sort: %w", op, err)
	}

	// Recalculate all sibling dates based on new order.
	if task.ParentTaskID == nil {
		return nil, fmt.Errorf(
			"%s: child task has no parent", op,
		)
	}

	return s.recalcSiblingDates(ctx, *task.ParentTaskID)
}

// recalcSiblingDates recalculates dates for all children of a parent
// based on their sort_order groups, then updates the parent.
func (s *Service) recalcSiblingDates(
	ctx context.Context,
	parentID uuid.UUID,
) ([]domain.GanttTask, error) {
	op := "gantt.recalcSiblingDates"

	parent, err := s.repo.GetGanttTaskByID(ctx, parentID)
	if err != nil {
		return nil, fmt.Errorf("%s: get parent: %w", op, err)
	}

	children, err := s.repo.GetGanttChildTasks(ctx, parentID)
	if err != nil {
		return nil, fmt.Errorf("%s: get children: %w", op, err)
	}
	if len(children) == 0 {
		return []domain.GanttTask{*parent}, nil
	}

	// Group children by sort_order.
	type group struct {
		order    int
		children []domain.GanttTask
	}
	var groups []group
	var current *group
	for _, c := range children {
		if current == nil || current.order != c.SortOrder {
			if current != nil {
				groups = append(groups, *current)
			}
			current = &group{order: c.SortOrder}
		}
		current.children = append(current.children, c)
	}
	if current != nil {
		groups = append(groups, *current)
	}

	// Lay out sequentially.
	parentStartDate := moveToWorkDay(parent.StartDate)
	cursor := parentStartDate
	parentEndDate := parentStartDate
	for _, g := range groups {
		var groupEnd time.Time
		for i := range g.children {
			workDays := max(1, countWorkDays(g.children[i].StartDate, g.children[i].EndDate))
			newStart := cursor
			newEnd := addWorkDays(newStart, workDays)
			g.children[i].StartDate = newStart
			g.children[i].EndDate = newEnd
			if err := s.repo.UpdateGanttTaskDates(
				ctx, g.children[i].ID, newStart, newEnd,
			); err != nil {
				return nil, fmt.Errorf(
					"%s: update child: %w", op, err,
				)
			}
			if groupEnd.IsZero() || newEnd.After(groupEnd) {
				groupEnd = newEnd
			}
		}
		if parentEndDate.Before(groupEnd) {
			parentEndDate = groupEnd
		}
		cursor = moveToWorkDay(groupEnd.AddDate(0, 0, 1))
	}

	// Update parent.
	if err := s.repo.UpdateGanttTaskDates(
		ctx, parentID, parentStartDate, parentEndDate,
	); err != nil {
		return nil, fmt.Errorf(
			"%s: update parent: %w", op, err,
		)
	}
	parent.StartDate = parentStartDate
	parent.EndDate = parentEndDate

	// Return updated task list.
	var result []domain.GanttTask
	result = append(result, *parent)
	for _, g := range groups {
		result = append(result, g.children...)
	}
	return result, nil
}

// recalcParentDates recalculates a parent task's start/end
// from its children's bounds.
func (s *Service) recalcParentDates(
	ctx context.Context,
	parentID uuid.UUID,
) error {
	op := "gantt.recalcParentDates"

	children, err := s.repo.GetGanttChildTasks(ctx, parentID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if len(children) == 0 {
		return nil
	}

	minStart := children[0].StartDate
	maxEnd := children[0].EndDate
	for _, c := range children[1:] {
		if c.StartDate.Before(minStart) {
			minStart = c.StartDate
		}
		if c.EndDate.After(maxEnd) {
			maxEnd = c.EndDate
		}
	}

	return s.repo.UpdateGanttTaskDates(
		ctx, parentID, minStart, maxEnd,
	)
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
