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
		 actual_end_date, actual_effort_days, start_offset_days)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING created_at, updated_at`
	err := r.DB.QueryRowContext(ctx, query,
		task.ID, task.EpicID, task.RoleID, task.Name,
		task.StartDate, task.EndDate, task.Progress,
		task.SortOrder, task.IsParent, task.ParentTaskID,
		task.ActualEndDate, task.ActualEffortDays, task.StartOffsetDays,
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
		gt.actual_end_date, gt.actual_effort_days, gt.start_offset_days,
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
			&t.ActualEndDate, &t.ActualEffortDays, &t.StartOffsetDays,
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
		actual_end_date, actual_effort_days, start_offset_days,
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
			&t.ActualEndDate, &t.ActualEffortDays, &t.StartOffsetDays,
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
		actual_end_date, actual_effort_days, start_offset_days,
		created_at, updated_at
		FROM gantt_tasks WHERE id = $1`
	err := r.DB.QueryRowContext(ctx, query, taskID).Scan(
		&t.ID, &t.EpicID, &t.RoleID, &t.Name,
		&t.StartDate, &t.EndDate, &t.Progress,
		&t.SortOrder, &t.IsParent, &t.ParentTaskID,
		&t.ActualEndDate, &t.ActualEffortDays, &t.StartOffsetDays,
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
		actual_end_date, actual_effort_days, start_offset_days,
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
			&t.ActualEndDate, &t.ActualEffortDays, &t.StartOffsetDays,
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
