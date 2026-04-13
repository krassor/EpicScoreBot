package handlers

import (
	"EpicScoreBot/internal/models/domain"
	"context"
	"time"

	"github.com/google/uuid"
)

// GanttService defines the business-logic contract used by handlers.
type GanttService interface {
	GenerateTasksForEpic(ctx context.Context, epicID uuid.UUID, startDate time.Time) ([]domain.GanttTask, error)
	UpdateTaskDates(ctx context.Context, taskID uuid.UUID, startDate, endDate time.Time) error
	ReorderTask(ctx context.Context, taskID uuid.UUID, newSortOrder int) ([]domain.GanttTask, error)
}

// Repository defines the data-access contract used by handlers.
type Repository interface {
	// Teams
	GetAllTeams(ctx context.Context) ([]domain.Team, error)
	GetTeamsByUserTelegramID(ctx context.Context, telegramID string) ([]domain.Team, error)

	// Users
	FindUserByTelegramID(ctx context.Context, telegramID string) (*domain.User, error)

	// Epics
	GetEpicsByTeamIDAndStatus(ctx context.Context, teamID uuid.UUID, status domain.Status) ([]domain.Epic, error)

	// Gantt tasks
	GetGanttTasksByTeamID(ctx context.Context, teamID uuid.UUID) ([]domain.GanttTask, error)
	GetGanttTaskByID(ctx context.Context, taskID uuid.UUID) (*domain.GanttTask, error)
	UpdateGanttTaskProgress(ctx context.Context, taskID uuid.UUID, progress float64) error
	DeleteGanttTasksByEpicID(ctx context.Context, epicID uuid.UUID) error
}
