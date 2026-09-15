package repositories

import (
	"EpicScoreBot/internal/models/domain"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CreateGanttTask inserts a new Gantt task.
func (r *Repository) CreateGanttTask(ctx context.Context, task *domain.GanttTask) (*domain.GanttTask, error) {
	op := "Repository.CreateGanttTask"
	task.ID = uuid.New()

	query := `INSERT INTO gantt_tasks
		(id, epic_id, role_id, name, start_date, end_date,
		 progress, sort_order, is_parent, parent_task_id,
		 actual_end_date, actual_effort_days, start_offset_days, assignee_id,
		 planning_start_date)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING created_at, updated_at`
	err := r.DB.QueryRowContext(ctx, query,
		task.ID, task.EpicID, task.RoleID, task.Name,
		task.StartDate, task.EndDate, task.Progress,
		task.SortOrder, task.IsParent, task.ParentTaskID,
		task.ActualEndDate, task.ActualEffortDays, task.StartOffsetDays, task.AssigneeID,
		task.PlanningStartDate,
	).Scan(&task.CreatedAt, &task.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return task, nil
}

// GetGanttTasksByTeamID returns all Gantt tasks for epics belonging to a team.
func (r *Repository) GetGanttTasksByTeamID(ctx context.Context, teamID uuid.UUID) ([]domain.GanttTask, error) {
	op := "Repository.GetGanttTasksByTeamID"
	query := `SELECT gt.id, gt.epic_id, gt.role_id, gt.name,
		gt.start_date, gt.end_date, gt.progress,
		gt.sort_order, gt.is_parent, gt.parent_task_id,
		gt.actual_end_date, gt.actual_effort_days, gt.start_offset_days, gt.assignee_id,
		gt.planning_start_date,
		gt.created_at, gt.updated_at
		FROM gantt_tasks gt
		INNER JOIN epics e ON e.id = gt.epic_id
		WHERE e.team_id = $1
		ORDER BY e.sort_order NULLS LAST, e.number, gt.sort_order, gt.name`
	rows, err := r.DB.QueryContext(ctx, query, teamID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	var tasks []domain.GanttTask
	for rows.Next() {
		var t domain.GanttTask
		if err := rows.Scan(
			&t.ID, &t.EpicID, &t.RoleID, &t.Name,
			&t.StartDate, &t.EndDate, &t.Progress,
			&t.SortOrder, &t.IsParent, &t.ParentTaskID,
			&t.ActualEndDate, &t.ActualEffortDays, &t.StartOffsetDays, &t.AssigneeID,
			&t.PlanningStartDate,
			&t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// GetGanttTasksByEpicID returns all Gantt tasks for a specific epic.
func (r *Repository) GetGanttTasksByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.GanttTask, error) {
	op := "Repository.GetGanttTasksByEpicID"
	query := `SELECT id, epic_id, role_id, name,
		start_date, end_date, progress,
		sort_order, is_parent, parent_task_id,
		actual_end_date, actual_effort_days, start_offset_days, assignee_id,
		planning_start_date,
		created_at, updated_at
		FROM gantt_tasks WHERE epic_id = $1
		ORDER BY sort_order, name`
	rows, err := r.DB.QueryContext(ctx, query, epicID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	var tasks []domain.GanttTask
	for rows.Next() {
		var t domain.GanttTask
		if err := rows.Scan(
			&t.ID, &t.EpicID, &t.RoleID, &t.Name,
			&t.StartDate, &t.EndDate, &t.Progress,
			&t.SortOrder, &t.IsParent, &t.ParentTaskID,
			&t.ActualEndDate, &t.ActualEffortDays, &t.StartOffsetDays, &t.AssigneeID,
			&t.PlanningStartDate,
			&t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// GetGanttTaskByID returns a single Gantt task by its ID.
func (r *Repository) GetGanttTaskByID(ctx context.Context, taskID uuid.UUID) (*domain.GanttTask, error) {
	op := "Repository.GetGanttTaskByID"
	var t domain.GanttTask
	query := `SELECT id, epic_id, role_id, name,
		start_date, end_date, progress,
		sort_order, is_parent, parent_task_id,
		actual_end_date, actual_effort_days, start_offset_days, assignee_id,
		planning_start_date,
		created_at, updated_at
		FROM gantt_tasks WHERE id = $1`
	err := r.DB.QueryRowContext(ctx, query, taskID).Scan(
		&t.ID, &t.EpicID, &t.RoleID, &t.Name,
		&t.StartDate, &t.EndDate, &t.Progress,
		&t.SortOrder, &t.IsParent, &t.ParentTaskID,
		&t.ActualEndDate, &t.ActualEffortDays, &t.StartOffsetDays, &t.AssigneeID,
		&t.PlanningStartDate,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return &t, nil
}

// UpdateGanttTaskDates updates the start and end dates of a Gantt task.
func (r *Repository) UpdateGanttTaskDates(ctx context.Context, taskID uuid.UUID, startDate, endDate time.Time) error {
	op := "Repository.UpdateGanttTaskDates"
	query := `UPDATE gantt_tasks
		SET start_date = $1, end_date = $2,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $3`
	_, err := r.DB.ExecContext(ctx, query, startDate, endDate, taskID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// UpdateGanttTaskProgress updates the progress percentage of a Gantt task.
func (r *Repository) UpdateGanttTaskProgress(ctx context.Context, taskID uuid.UUID, progress float64) error {
	op := "Repository.UpdateGanttTaskProgress"
	query := `UPDATE gantt_tasks
		SET progress = $1, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2`
	_, err := r.DB.ExecContext(ctx, query, progress, taskID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// UpdateGanttTaskSortOrder updates the sort order of a Gantt task.
func (r *Repository) UpdateGanttTaskSortOrder(ctx context.Context, taskID uuid.UUID, sortOrder int) error {
	op := "Repository.UpdateGanttTaskSortOrder"
	query := `UPDATE gantt_tasks
		SET sort_order = $1, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2`
	_, err := r.DB.ExecContext(ctx, query, sortOrder, taskID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// UpdateGanttTaskStartOffset updates the start offset (lead/lag, in days)
// of a leaf (role) Gantt task.
func (r *Repository) UpdateGanttTaskStartOffset(ctx context.Context, taskID uuid.UUID, offsetDays int) error {
	op := "Repository.UpdateGanttTaskStartOffset"
	query := `UPDATE gantt_tasks
		SET start_offset_days = $1, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2`
	_, err := r.DB.ExecContext(ctx, query, offsetDays, taskID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// DeleteGanttTasksByEpicID removes all Gantt tasks for a given epic.
func (r *Repository) DeleteGanttTasksByEpicID(ctx context.Context, epicID uuid.UUID) error {
	op := "Repository.DeleteGanttTasksByEpicID"
	query := `DELETE FROM gantt_tasks WHERE epic_id = $1`
	_, err := r.DB.ExecContext(ctx, query, epicID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// GetGanttChildTasks returns child tasks for a parent task, ordered by sort_order.
func (r *Repository) GetGanttChildTasks(ctx context.Context, parentTaskID uuid.UUID) ([]domain.GanttTask, error) {
	op := "Repository.GetGanttChildTasks"
	query := `SELECT id, epic_id, role_id, name,
		start_date, end_date, progress,
		sort_order, is_parent, parent_task_id,
		actual_end_date, actual_effort_days, start_offset_days, assignee_id,
		planning_start_date,
		created_at, updated_at
		FROM gantt_tasks WHERE parent_task_id = $1
		ORDER BY sort_order, name`
	rows, err := r.DB.QueryContext(ctx, query, parentTaskID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	var tasks []domain.GanttTask
	for rows.Next() {
		var t domain.GanttTask
		if err := rows.Scan(
			&t.ID, &t.EpicID, &t.RoleID, &t.Name,
			&t.StartDate, &t.EndDate, &t.Progress,
			&t.SortOrder, &t.IsParent, &t.ParentTaskID,
			&t.ActualEndDate, &t.ActualEffortDays, &t.StartOffsetDays, &t.AssigneeID,
			&t.PlanningStartDate,
			&t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// UpdateGanttTaskActuals fixes the actual completion fact for a task: the
// date it reached 100% progress and the actual effort in working days
// between its (planned) start date and that date.
func (r *Repository) UpdateGanttTaskActuals(ctx context.Context, taskID uuid.UUID, actualEndDate time.Time, effortDays int) error {
	op := "Repository.UpdateGanttTaskActuals"
	query := `UPDATE gantt_tasks
		SET actual_end_date = $1, actual_effort_days = $2,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $3`
	_, err := r.DB.ExecContext(ctx, query, actualEndDate, effortDays, taskID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// ClearGanttTaskActuals clears the actual completion fact of a task
// (used when a task is reopened, i.e. its progress drops below 100%).
func (r *Repository) ClearGanttTaskActuals(ctx context.Context, taskID uuid.UUID) error {
	op := "Repository.ClearGanttTaskActuals"
	query := `UPDATE gantt_tasks
		SET actual_end_date = NULL, actual_effort_days = NULL,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1`
	_, err := r.DB.ExecContext(ctx, query, taskID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// HasGanttTasksForEpic checks if Gantt tasks already exist for an epic.
func (r *Repository) HasGanttTasksForEpic(ctx context.Context, epicID uuid.UUID) (bool, error) {
	op := "Repository.HasGanttTasksForEpic"
	var count int
	query := `SELECT COUNT(*) FROM gantt_tasks WHERE epic_id = $1`
	err := r.DB.QueryRowContext(ctx, query, epicID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("%s: %w", op, err)
	}
	return count > 0, nil
}

// UpdateGanttTaskAssignee sets the assignee (executor) of a Gantt task —
// the outcome of the scheduler's automatic distribution or of an applied
// manual pin (task_assignments), never a direct user action. nil clears it
// (e.g. the role's pool is empty, or the pinned candidate no longer belongs
// to it — see design.md Решение 1/7).
func (r *Repository) UpdateGanttTaskAssignee(ctx context.Context, taskID uuid.UUID, assigneeID *uuid.UUID) error {
	op := "Repository.UpdateGanttTaskAssignee"
	query := `UPDATE gantt_tasks
		SET assignee_id = $1, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2`
	_, err := r.DB.ExecContext(ctx, query, assigneeID, taskID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// GetTaskAssignmentsByTeamID returns every task_assignments row belonging to
// a team's epics/stories in one query (read pачкой на команду — см.
// design.md Risks/Trade-offs, "Рост числа запросов к БД в пересчёте"). Both
// stories and legacy epics without stories live in the epics table and
// carry team_id, so a single join on task_assignments.epic_id covers both
// keys used by the scheduler.
func (r *Repository) GetTaskAssignmentsByTeamID(ctx context.Context, teamID uuid.UUID) ([]domain.TaskAssignment, error) {
	op := "Repository.GetTaskAssignmentsByTeamID"
	query := `SELECT ta.epic_id, ta.role_id, ta.user_id, ta.start_offset_days,
		ta.not_before_date, ta.wait_for_story_id, ta.wait_for_role_id
		FROM task_assignments ta
		INNER JOIN epics e ON e.id = ta.epic_id
		WHERE e.team_id = $1`
	rows, err := r.DB.QueryContext(ctx, query, teamID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	var assignments []domain.TaskAssignment
	for rows.Next() {
		var a domain.TaskAssignment
		if err := rows.Scan(
			&a.EpicID, &a.RoleID, &a.UserID, &a.StartOffsetDays,
			&a.NotBeforeDate, &a.WaitForStoryID, &a.WaitForRoleID,
		); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		assignments = append(assignments, a)
	}
	return assignments, nil
}

// UpsertTaskAssignmentUser pins (userID != nil) or unpins (userID == nil) an
// executor for a story/epic+role pair, creating the task_assignments row on
// first use (start_offset_days defaults to 0) and preserving an existing
// start_offset_days on conflict — pinning an assignee and shifting a start
// are independent user actions on the same row (design.md Решение 4).
func (r *Repository) UpsertTaskAssignmentUser(ctx context.Context, epicID, roleID uuid.UUID, userID *uuid.UUID) error {
	op := "Repository.UpsertTaskAssignmentUser"
	query := `INSERT INTO task_assignments (epic_id, role_id, user_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (epic_id, role_id) DO UPDATE SET user_id = EXCLUDED.user_id`
	_, err := r.DB.ExecContext(ctx, query, epicID, roleID, userID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// UpsertTaskAssignmentStartOffset sets the manual start offset (lead/lag, in
// days) for a story/epic+role pair, creating the task_assignments row on
// first use (user_id defaults to NULL, i.e. automatic) and preserving an
// existing user_id on conflict — see UpsertTaskAssignmentUser.
func (r *Repository) UpsertTaskAssignmentStartOffset(ctx context.Context, epicID, roleID uuid.UUID, offsetDays int) error {
	op := "Repository.UpsertTaskAssignmentStartOffset"
	query := `INSERT INTO task_assignments (epic_id, role_id, start_offset_days)
		VALUES ($1, $2, $3)
		ON CONFLICT (epic_id, role_id) DO UPDATE SET start_offset_days = EXCLUDED.start_offset_days`
	_, err := r.DB.ExecContext(ctx, query, epicID, roleID, offsetDays)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// UpsertTaskAssignmentStartConstraint sets (or clears, via nil) the "Начать
// не ранее" lower bound for a story/epic+role pair: a calendar date and/or a
// reference to another role task of the same team, identified by the same
// (story-or-epic, role) pair the row itself is keyed on — see
// domain.TaskAssignment.NotBeforeDate/WaitForStoryID/WaitForRoleID and
// openspec/changes/add-task-start-constraints/design.md, Решения 1 и 5.
// Creates the task_assignments row on first use (user_id/start_offset_days
// default to their zero values) and preserves an existing
// user_id/start_offset_days on conflict — independent of
// UpsertTaskAssignmentUser/UpsertTaskAssignmentStartOffset, the same way
// those two are independent of each other. waitForStoryID and waitForRoleID
// are written together (both nil clears the reference) — the caller
// (gantt.Service.SetTaskStartConstraint) is responsible for ensuring they're
// either both nil or both set.
func (r *Repository) UpsertTaskAssignmentStartConstraint(
	ctx context.Context,
	epicID, roleID uuid.UUID,
	notBeforeDate *time.Time,
	waitForStoryID, waitForRoleID *uuid.UUID,
) error {
	op := "Repository.UpsertTaskAssignmentStartConstraint"
	query := `INSERT INTO task_assignments (epic_id, role_id, not_before_date, wait_for_story_id, wait_for_role_id)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (epic_id, role_id) DO UPDATE SET
			not_before_date = EXCLUDED.not_before_date,
			wait_for_story_id = EXCLUDED.wait_for_story_id,
			wait_for_role_id = EXCLUDED.wait_for_role_id`
	_, err := r.DB.ExecContext(ctx, query, epicID, roleID, notBeforeDate, waitForStoryID, waitForRoleID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}
