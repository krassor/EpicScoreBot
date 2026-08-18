package gantt

import (
	"EpicScoreBot/internal/models/domain"
	"context"
	"time"

	"github.com/google/uuid"
)

// Repository defines the data-access contract for the Gantt service.
type Repository interface {
	// Epics
	GetEpicByID(ctx context.Context, epicID uuid.UUID) (*domain.Epic, error)
	GetEpicsByTeamIDAndStatus(ctx context.Context, teamID uuid.UUID, status domain.Status) ([]domain.Epic, error)
	// GetTeamEpicsOrdered возвращает топ-эпики команды в порядке очереди
	// конвейерного планировщика (epics.sort_order).
	GetTeamEpicsOrdered(ctx context.Context, teamID uuid.UUID) ([]domain.Epic, error)
	// UpdateEpicSortOrder меняет позицию эпика/стори в очереди планировщика.
	UpdateEpicSortOrder(ctx context.Context, epicID uuid.UUID, sortOrder int) error

	// Roles
	GetAllRoles(ctx context.Context) ([]domain.Role, error)
	GetRoleByID(ctx context.Context, roleID uuid.UUID) (*domain.Role, error)

	// Teams
	GetAllTeams(ctx context.Context) ([]domain.Team, error)
	GetTeamByID(ctx context.Context, teamID uuid.UUID) (*domain.Team, error)

	// Scoring
	GetEpicRoleScoresByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.EpicRoleScore, error)

	// Gantt tasks
	CreateGanttTask(ctx context.Context, task *domain.GanttTask) (*domain.GanttTask, error)
	GetGanttTasksByTeamID(ctx context.Context, teamID uuid.UUID) ([]domain.GanttTask, error)
	GetGanttTasksByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.GanttTask, error)
	GetGanttTaskByID(ctx context.Context, taskID uuid.UUID) (*domain.GanttTask, error)
	GetGanttChildTasks(ctx context.Context, parentTaskID uuid.UUID) ([]domain.GanttTask, error)
	UpdateGanttTaskDates(ctx context.Context, taskID uuid.UUID, startDate, endDate time.Time) error
	UpdateGanttTaskProgress(ctx context.Context, taskID uuid.UUID, progress float64) error
	UpdateGanttTaskSortOrder(ctx context.Context, taskID uuid.UUID, sortOrder int) error
	// UpdateGanttTaskActuals фиксирует факт завершения задачи (дата + трудоёмкость).
	UpdateGanttTaskActuals(ctx context.Context, taskID uuid.UUID, actualEndDate time.Time, effortDays int) error
	// ClearGanttTaskActuals сбрасывает факт завершения задачи (переоткрытие).
	ClearGanttTaskActuals(ctx context.Context, taskID uuid.UUID) error
	DeleteGanttTasksByEpicID(ctx context.Context, epicID uuid.UUID) error
	HasGanttTasksForEpic(ctx context.Context, epicID uuid.UUID) (bool, error)
	GetRisksByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.Risk, error)
	GetStoriesByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.Epic, error)
}
