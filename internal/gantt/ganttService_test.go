package gantt

import (
	"EpicScoreBot/internal/models/domain"
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
)

type MockRepository struct {
	GetEpicByIDFunc               func(ctx context.Context, epicID uuid.UUID) (*domain.Epic, error)
	GetEpicsByTeamIDAndStatusFunc func(ctx context.Context, teamID uuid.UUID, status domain.Status) ([]domain.Epic, error)
	GetAllRolesFunc               func(ctx context.Context) ([]domain.Role, error)
	GetRoleByIDFunc               func(ctx context.Context, roleID uuid.UUID) (*domain.Role, error)
	GetAllTeamsFunc               func(ctx context.Context) ([]domain.Team, error)
	GetTeamByIDFunc               func(ctx context.Context, teamID uuid.UUID) (*domain.Team, error)
	GetEpicRoleScoresByEpicIDFunc func(ctx context.Context, epicID uuid.UUID) ([]domain.EpicRoleScore, error)
	CreateGanttTaskFunc           func(ctx context.Context, task *domain.GanttTask) (*domain.GanttTask, error)
	GetGanttTasksByTeamIDFunc     func(ctx context.Context, teamID uuid.UUID) ([]domain.GanttTask, error)
	GetGanttTasksByEpicIDFunc     func(ctx context.Context, epicID uuid.UUID) ([]domain.GanttTask, error)
	GetGanttTaskByIDFunc          func(ctx context.Context, taskID uuid.UUID) (*domain.GanttTask, error)
	GetGanttChildTasksFunc        func(ctx context.Context, parentTaskID uuid.UUID) ([]domain.GanttTask, error)
	UpdateGanttTaskDatesFunc      func(ctx context.Context, taskID uuid.UUID, startDate, endDate time.Time) error
	UpdateGanttTaskProgressFunc   func(ctx context.Context, taskID uuid.UUID, progress float64) error
	UpdateGanttTaskSortOrderFunc  func(ctx context.Context, taskID uuid.UUID, sortOrder int) error
	DeleteGanttTasksByEpicIDFunc  func(ctx context.Context, epicID uuid.UUID) error
	HasGanttTasksForEpicFunc      func(ctx context.Context, epicID uuid.UUID) (bool, error)
	GetRisksByEpicIDFunc          func(ctx context.Context, epicID uuid.UUID) ([]domain.Risk, error)
	GetStoriesByEpicIDFunc        func(ctx context.Context, epicID uuid.UUID) ([]domain.Epic, error)
}

func (m *MockRepository) GetEpicByID(ctx context.Context, epicID uuid.UUID) (*domain.Epic, error) {
	if m.GetEpicByIDFunc != nil {
		return m.GetEpicByIDFunc(ctx, epicID)
	}
	return nil, nil
}
func (m *MockRepository) GetEpicsByTeamIDAndStatus(ctx context.Context, teamID uuid.UUID, status domain.Status) ([]domain.Epic, error) {
	if m.GetEpicsByTeamIDAndStatusFunc != nil {
		return m.GetEpicsByTeamIDAndStatusFunc(ctx, teamID, status)
	}
	return nil, nil
}
func (m *MockRepository) GetAllRoles(ctx context.Context) ([]domain.Role, error) {
	if m.GetAllRolesFunc != nil {
		return m.GetAllRolesFunc(ctx)
	}
	return nil, nil
}
func (m *MockRepository) GetRoleByID(ctx context.Context, roleID uuid.UUID) (*domain.Role, error) {
	if m.GetRoleByIDFunc != nil {
		return m.GetRoleByIDFunc(ctx, roleID)
	}
	return nil, nil
}
func (m *MockRepository) GetAllTeams(ctx context.Context) ([]domain.Team, error) {
	if m.GetAllTeamsFunc != nil {
		return m.GetAllTeamsFunc(ctx)
	}
	return nil, nil
}
func (m *MockRepository) GetTeamByID(ctx context.Context, teamID uuid.UUID) (*domain.Team, error) {
	if m.GetTeamByIDFunc != nil {
		return m.GetTeamByIDFunc(ctx, teamID)
	}
	return nil, nil
}
func (m *MockRepository) GetEpicRoleScoresByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.EpicRoleScore, error) {
	if m.GetEpicRoleScoresByEpicIDFunc != nil {
		return m.GetEpicRoleScoresByEpicIDFunc(ctx, epicID)
	}
	return nil, nil
}
func (m *MockRepository) CreateGanttTask(ctx context.Context, task *domain.GanttTask) (*domain.GanttTask, error) {
	if m.CreateGanttTaskFunc != nil {
		return m.CreateGanttTaskFunc(ctx, task)
	}
	return task, nil
}
func (m *MockRepository) GetGanttTasksByTeamID(ctx context.Context, teamID uuid.UUID) ([]domain.GanttTask, error) {
	if m.GetGanttTasksByTeamIDFunc != nil {
		return m.GetGanttTasksByTeamIDFunc(ctx, teamID)
	}
	return nil, nil
}
func (m *MockRepository) GetGanttTasksByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.GanttTask, error) {
	if m.GetGanttTasksByEpicIDFunc != nil {
		return m.GetGanttTasksByEpicIDFunc(ctx, epicID)
	}
	return nil, nil
}
func (m *MockRepository) GetGanttTaskByID(ctx context.Context, taskID uuid.UUID) (*domain.GanttTask, error) {
	if m.GetGanttTaskByIDFunc != nil {
		return m.GetGanttTaskByIDFunc(ctx, taskID)
	}
	return nil, nil
}
func (m *MockRepository) GetGanttChildTasks(ctx context.Context, parentTaskID uuid.UUID) ([]domain.GanttTask, error) {
	if m.GetGanttChildTasksFunc != nil {
		return m.GetGanttChildTasksFunc(ctx, parentTaskID)
	}
	return nil, nil
}
func (m *MockRepository) UpdateGanttTaskDates(ctx context.Context, taskID uuid.UUID, startDate, endDate time.Time) error {
	if m.UpdateGanttTaskDatesFunc != nil {
		return m.UpdateGanttTaskDatesFunc(ctx, taskID, startDate, endDate)
	}
	return nil
}
func (m *MockRepository) UpdateGanttTaskProgress(ctx context.Context, taskID uuid.UUID, progress float64) error {
	if m.UpdateGanttTaskProgressFunc != nil {
		return m.UpdateGanttTaskProgressFunc(ctx, taskID, progress)
	}
	return nil
}
func (m *MockRepository) UpdateGanttTaskSortOrder(ctx context.Context, taskID uuid.UUID, sortOrder int) error {
	if m.UpdateGanttTaskSortOrderFunc != nil {
		return m.UpdateGanttTaskSortOrderFunc(ctx, taskID, sortOrder)
	}
	return nil
}
func (m *MockRepository) DeleteGanttTasksByEpicID(ctx context.Context, epicID uuid.UUID) error {
	if m.DeleteGanttTasksByEpicIDFunc != nil {
		return m.DeleteGanttTasksByEpicIDFunc(ctx, epicID)
	}
	return nil
}
func (m *MockRepository) HasGanttTasksForEpic(ctx context.Context, epicID uuid.UUID) (bool, error) {
	if m.HasGanttTasksForEpicFunc != nil {
		return m.HasGanttTasksForEpicFunc(ctx, epicID)
	}
	return false, nil
}
func (m *MockRepository) GetRisksByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.Risk, error) {
	if m.GetRisksByEpicIDFunc != nil {
		return m.GetRisksByEpicIDFunc(ctx, epicID)
	}
	return nil, nil
}
func (m *MockRepository) GetStoriesByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.Epic, error) {
	if m.GetStoriesByEpicIDFunc != nil {
		return m.GetStoriesByEpicIDFunc(ctx, epicID)
	}
	return nil, nil
}

func TestMoveToWorkDay(t *testing.T) {
	tests := []struct {
		name string
		input time.Time
		want  time.Time
	}{
		{
			name: "Monday remains Monday",
			input: time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC), // Monday
			want:  time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC),
		},
		{
			name: "Friday remains Friday",
			input: time.Date(2026, 7, 10, 15, 30, 0, 0, time.UTC), // Friday
			want:  time.Date(2026, 7, 10, 15, 30, 0, 0, time.UTC),
		},
		{
			name: "Saturday moves to Monday",
			input: time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC), // Saturday
			want:  time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC), // Monday
		},
		{
			name: "Sunday moves to Monday",
			input: time.Date(2026, 7, 12, 9, 0, 0, 0, time.UTC), // Sunday
			want:  time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC), // Monday
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := moveToWorkDay(tt.input)
			if !got.Equal(tt.want) {
				t.Errorf("moveToWorkDay(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestAddWorkDays(t *testing.T) {
	tests := []struct {
		name  string
		start time.Time
		days  int
		want  time.Time
	}{
		{
			name:  "Monday add 1 day",
			start: time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
			days:  1,
			want:  time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
		},
		{
			name:  "Monday add 5 days (mon-tue-wed-thu-fri)",
			start: time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
			days:  5,
			want:  time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC),
		},
		{
			name:  "Friday add 2 days (fri-mon)",
			start: time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC),
			days:  2,
			want:  time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
		},
		{
			name:  "Saturday add 2 days (sat becomes mon, + 1 workday -> tue)",
			start: time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC),
			days:  2,
			want:  time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := addWorkDays(tt.start, tt.days)
			if !got.Equal(tt.want) {
				t.Errorf("addWorkDays(%v, %d) = %v, want %v", tt.start, tt.days, got, tt.want)
			}
		})
	}
}

func TestCountWorkDays(t *testing.T) {
	tests := []struct {
		name  string
		start time.Time
		end   time.Time
		want  int
	}{
		{
			name:  "Same day Monday",
			start: time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
			end:   time.Date(2026, 7, 13, 23, 59, 59, 0, time.UTC),
			want:  1,
		},
		{
			name:  "Same day Saturday",
			start: time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC),
			end:   time.Date(2026, 7, 11, 23, 59, 59, 0, time.UTC),
			want:  0,
		},
		{
			name:  "Monday to Friday",
			start: time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC),
			end:   time.Date(2026, 7, 17, 15, 0, 0, 0, time.UTC),
			want:  5,
		},
		{
			name:  "Friday to next Monday",
			start: time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC),
			end:   time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
			want:  2,
		},
		{
			name:  "Weekend only",
			start: time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC),
			end:   time.Date(2026, 7, 12, 23, 0, 0, 0, time.UTC),
			want:  0,
		},
		{
			name:  "Start after end",
			start: time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
			end:   time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC),
			want:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countWorkDays(tt.start, tt.end)
			if got != tt.want {
				t.Errorf("countWorkDays(%v, %v) = %d, want %d", tt.start, tt.end, got, tt.want)
			}
		})
	}
}

func TestGenerateTasksForEpic(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()

	epicID := uuid.New()
	roleID1 := uuid.New()
	roleID2 := uuid.New()

	mockEpic := &domain.Epic{
		ID:     epicID,
		Number: "E-1",
		Name:   "Epic One",
	}

	mockRoleScores := []domain.EpicRoleScore{
		{
			EpicID:      epicID,
			RoleID:      roleID1,
			WeightedAvg: 3.0,
		},
		{
			EpicID:      epicID,
			RoleID:      roleID2,
			WeightedAvg: 2.0,
		},
	}

	mockRisks := []domain.Risk{
		{
			EpicID:        epicID,
			WeightedScore: floatPtr(8.0), // coeff 1.05
		},
	}

	mockRepo := &MockRepository{
		GetEpicByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Epic, error) {
			return mockEpic, nil
		},
		HasGanttTasksForEpicFunc: func(ctx context.Context, id uuid.UUID) (bool, error) {
			return false, nil
		},
		GetEpicRoleScoresByEpicIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.EpicRoleScore, error) {
			return mockRoleScores, nil
		},
		GetRisksByEpicIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.Risk, error) {
			return mockRisks, nil
		},
		GetRoleByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
			if id == roleID1 {
				return &domain.Role{ID: roleID1, Name: "Аналитик"}, nil
			}
			return &domain.Role{ID: roleID2, Name: "BE разработчик"}, nil
		},
		CreateGanttTaskFunc: func(ctx context.Context, task *domain.GanttTask) (*domain.GanttTask, error) {
			task.ID = uuid.New()
			return task, nil
		},
		UpdateGanttTaskDatesFunc: func(ctx context.Context, id uuid.UUID, start, end time.Time) error {
			return nil
		},
	}

	service := New(logger, mockRepo)

	// Wednesday start
	start := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
	tasks, err := service.GenerateTasksForEpic(ctx, epicID, start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 1 parent task + 2 child tasks
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}

	parent := tasks[0]
	if !parent.IsParent {
		t.Errorf("expected parent task first")
	}

	// Coefficients check:
	// Final coefficient is 1.05.
	// Role 1 ("Аналитик"): 3.0 * 1.05 = 3.15 -> Ceil -> 4 workDays.
	// Role 2 ("BE разработчик"): 2.0 * 1.05 = 2.10 -> Ceil -> 3 workDays.
	// Role 1 starts on Wed Jul 8 (workday).
	// Wed Jul 8 + 4 workdays (wed, thu, fri, mon) -> ends on Mon Jul 13.
	// Next group starts on moveToWorkDay(Mon Jul 13 + 1 day) = Tue Jul 14.
	// Role 2 starts on Tue Jul 14.
	// Tue Jul 14 + 3 workdays (tue, wed, thu) -> ends on Thu Jul 16.
	// Parent task should span Wed Jul 8 to Thu Jul 16.

	expectedParentStart := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
	expectedParentEnd := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)

	if !parent.StartDate.Equal(expectedParentStart) {
		t.Errorf("parent start %v, want %v", parent.StartDate, expectedParentStart)
	}
	if !parent.EndDate.Equal(expectedParentEnd) {
		t.Errorf("parent end %v, want %v", parent.EndDate, expectedParentEnd)
	}

	child1 := tasks[1]
	child2 := tasks[2]

	if child1.Name != "Аналитик" {
		t.Errorf("expected Аналитик, got %s", child1.Name)
	}
	expectedChild1End := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	if !child1.EndDate.Equal(expectedChild1End) {
		t.Errorf("Аналитик end %v, want %v", child1.EndDate, expectedChild1End)
	}

	if child2.Name != "BE разработчик" {
		t.Errorf("expected BE разработчик, got %s", child2.Name)
	}
	expectedChild2Start := time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)
	expectedChild2End := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)

	if !child2.StartDate.Equal(expectedChild2Start) {
		t.Errorf("BE разработчик start %v, want %v", child2.StartDate, expectedChild2Start)
	}
	if !child2.EndDate.Equal(expectedChild2End) {
		t.Errorf("BE разработчик end %v, want %v", child2.EndDate, expectedChild2End)
	}
}

func TestRecalcSiblingDates(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()

	parentID := uuid.New()
	childID1 := uuid.New()
	childID2 := uuid.New()

	parentTask := &domain.GanttTask{
		ID:        parentID,
		StartDate: time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC), // Wed Jul 8
		EndDate:   time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC),
		IsParent:  true,
	}

	children := []domain.GanttTask{
		{
			ID:           childID1,
			ParentTaskID: &parentID,
			Name:         "Аналитик",
			StartDate:    time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC),
			EndDate:      time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC), // 4 workDays (wed, thu, fri, mon)
			SortOrder:    1,
		},
		{
			ID:           childID2,
			ParentTaskID: &parentID,
			Name:         "BE разработчик",
			StartDate:    time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC),
			EndDate:      time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC), // 4 workDays (fri, mon, tue, wed)
			SortOrder:    2,
		},
	}

	mockRepo := &MockRepository{
		GetGanttTaskByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.GanttTask, error) {
			if id == parentID {
				return parentTask, nil
			}
			return nil, errors.New("not found")
		},
		GetGanttChildTasksFunc: func(ctx context.Context, pid uuid.UUID) ([]domain.GanttTask, error) {
			return children, nil
		},
		UpdateGanttTaskDatesFunc: func(ctx context.Context, id uuid.UUID, start, end time.Time) error {
			return nil
		},
	}

	service := New(logger, mockRepo)

	updatedTasks, err := service.recalcSiblingDates(ctx, parentID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 1 parent + 2 children
	if len(updatedTasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(updatedTasks))
	}

	// Sibling dates layout logic check:
	// parentStart = Wed Jul 8
	// child1 starts at Wed Jul 8, workDays = 4 -> ends on Mon Jul 13.
	// Next group starts on moveToWorkDay(Mon Jul 13 + 1 day) = Tue Jul 14.
	// child2 starts at Tue Jul 14, workDays = 4 -> ends on Fri Jul 17 (tue, wed, thu, fri).
	// Parent task should span Wed Jul 8 to Fri Jul 17.

	gotParent := updatedTasks[0]
	if !gotParent.StartDate.Equal(time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("parent start %v, want Wed Jul 8", gotParent.StartDate)
	}
	if !gotParent.EndDate.Equal(time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("parent end %v, want Fri Jul 17", gotParent.EndDate)
	}

	gotChild1 := updatedTasks[1]
	gotChild2 := updatedTasks[2]

	if !gotChild1.StartDate.Equal(time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("child1 start %v, want Wed Jul 8", gotChild1.StartDate)
	}
	if !gotChild1.EndDate.Equal(time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("child1 end %v, want Mon Jul 13", gotChild1.EndDate)
	}

	if !gotChild2.StartDate.Equal(time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("child2 start %v, want Tue Jul 14", gotChild2.StartDate)
	}
	if !gotChild2.EndDate.Equal(time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("child2 end %v, want Fri Jul 17", gotChild2.EndDate)
	}
}

func TestUpdateTaskDates(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()

	parentID := uuid.New()
	taskID := uuid.New()

	task := &domain.GanttTask{
		ID:           taskID,
		ParentTaskID: &parentID,
		StartDate:    time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC), // Fri Jul 10
		EndDate:      time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC),
		IsParent:     false,
	}

	mockRepo := &MockRepository{
		GetGanttTaskByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.GanttTask, error) {
			if id == taskID {
				return task, nil
			}
			return nil, errors.New("not found")
		},
		UpdateGanttTaskDatesFunc: func(ctx context.Context, id uuid.UUID, start, end time.Time) error {
			task.StartDate = start
			task.EndDate = end
			return nil
		},
		GetGanttChildTasksFunc: func(ctx context.Context, pid uuid.UUID) ([]domain.GanttTask, error) {
			return []domain.GanttTask{*task}, nil
		},
	}

	service := New(logger, mockRepo)

	// Update task dates (moving to Saturday -> should move to Monday, with 2 workDays duration -> Monday to Tuesday)
	err := service.UpdateTaskDates(ctx, taskID, 
		time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC), // Sat Jul 11 (becomes Mon Jul 13)
		time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC), // Tue Jul 14 (Mon Jul 13 to Tue Jul 14 is 2 workDays)
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !task.StartDate.Equal(time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("expected adjusted start Mon Jul 13, got %v", task.StartDate)
	}
	if !task.EndDate.Equal(time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("expected adjusted end Tue Jul 14, got %v", task.EndDate)
	}
}

func TestReorderTask(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()

	parentID := uuid.New()
	taskID := uuid.New()

	parentTask := &domain.GanttTask{
		ID:        parentID,
		StartDate: time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC),
		IsParent:  true,
	}

	task := &domain.GanttTask{
		ID:           taskID,
		ParentTaskID: &parentID,
		StartDate:    time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
		EndDate:      time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC),
		IsParent:     false,
		SortOrder:    2,
	}

	mockRepo := &MockRepository{
		GetGanttTaskByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.GanttTask, error) {
			if id == taskID {
				return task, nil
			}
			if id == parentID {
				return parentTask, nil
			}
			return nil, errors.New("not found")
		},
		UpdateGanttTaskSortOrderFunc: func(ctx context.Context, id uuid.UUID, order int) error {
			task.SortOrder = order
			return nil
		},
		GetGanttChildTasksFunc: func(ctx context.Context, pid uuid.UUID) ([]domain.GanttTask, error) {
			return []domain.GanttTask{*task}, nil
		},
		UpdateGanttTaskDatesFunc: func(ctx context.Context, id uuid.UUID, start, end time.Time) error {
			if id == taskID {
				task.StartDate = start
				task.EndDate = end
			}
			if id == parentID {
				parentTask.StartDate = start
				parentTask.EndDate = end
			}
			return nil
		},
	}

	service := New(logger, mockRepo)

	updated, err := service.ReorderTask(ctx, taskID, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(updated) != 2 {
		t.Fatalf("expected 2 updated tasks, got %d", len(updated))
	}
	if task.SortOrder != 1 {
		t.Errorf("expected sort order 1, got %d", task.SortOrder)
	}
}

func floatPtr(f float64) *float64 {
	return &f
}

func TestGenerateTasksForEpic_WithStories(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()

	epicID := uuid.New()
	storyID1 := uuid.New()
	roleID1 := uuid.New()
	roleID2 := uuid.New()

	mockEpic := &domain.Epic{
		ID:     epicID,
		Number: "E-100",
		Name:   "Parent Epic",
	}

	mockStories := []domain.Epic{
		{
			ID:           storyID1,
			Number:       "E-100-S1",
			Name:         "Story 1",
			ParentEpicID: &epicID,
		},
	}

	mockRoleScores := []domain.EpicRoleScore{
		{
			EpicID:      storyID1,
			RoleID:      roleID1,
			WeightedAvg: 3.0,
		},
		{
			EpicID:      storyID1,
			RoleID:      roleID2,
			WeightedAvg: 2.0,
		},
	}

	mockRepo := &MockRepository{
		GetEpicByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Epic, error) {
			if id == epicID {
				return mockEpic, nil
			}
			return &mockStories[0], nil
		},
		HasGanttTasksForEpicFunc: func(ctx context.Context, id uuid.UUID) (bool, error) {
			return false, nil
		},
		GetStoriesByEpicIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.Epic, error) {
			return mockStories, nil
		},
		GetEpicRoleScoresByEpicIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.EpicRoleScore, error) {
			if id == storyID1 {
				return mockRoleScores, nil
			}
			return nil, nil
		},
		GetRisksByEpicIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.Risk, error) {
			return nil, nil
		},
		GetRoleByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
			if id == roleID1 {
				return &domain.Role{ID: roleID1, Name: "Аналитик"}, nil
			}
			return &domain.Role{ID: roleID2, Name: "BE разработчик"}, nil
		},
		CreateGanttTaskFunc: func(ctx context.Context, task *domain.GanttTask) (*domain.GanttTask, error) {
			task.ID = uuid.New()
			return task, nil
		},
		UpdateGanttTaskDatesFunc: func(ctx context.Context, id uuid.UUID, start, end time.Time) error {
			return nil
		},
	}

	service := New(logger, mockRepo)

	start := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
	tasks, err := service.GenerateTasksForEpic(ctx, epicID, start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tasks) != 4 {
		t.Fatalf("expected 4 tasks, got %d", len(tasks))
	}

	if tasks[0].IsParent != true || tasks[0].ParentTaskID != nil {
		t.Errorf("invalid parent epic task: %+v", tasks[0])
	}

	if tasks[1].IsParent != true || *tasks[1].ParentTaskID != tasks[0].ID {
		t.Errorf("invalid story task: %+v", tasks[1])
	}

	if tasks[2].IsParent != false || *tasks[2].ParentTaskID != tasks[1].ID {
		t.Errorf("invalid child role task 1: %+v", tasks[2])
	}
}

