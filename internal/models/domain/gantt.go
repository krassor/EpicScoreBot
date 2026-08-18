package domain

import (
	"time"

	"github.com/google/uuid"
)

// GanttTask represents a task on the Gantt chart.
// Parent tasks correspond to epics; child tasks correspond to roles.
type GanttTask struct {
	ID           uuid.UUID
	EpicID       uuid.UUID
	RoleID       *uuid.UUID // nil for parent (epic) tasks
	Name         string
	StartDate    time.Time
	EndDate      time.Time
	Progress     float64
	SortOrder    int
	IsParent     bool
	ParentTaskID *uuid.UUID // nil for parent tasks
	// ActualEndDate — фактическая дата завершения задачи (проставляется
	// автоматически при простановке 100% прогресса), nil пока не завершена.
	ActualEndDate *time.Time
	// ActualEffortDays — фактическая трудоёмкость в рабочих днях между
	// плановым стартом задачи и ActualEndDate, вычисляется автоматически.
	ActualEffortDays *int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
