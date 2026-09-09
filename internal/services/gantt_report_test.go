package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"EpicScoreBot/internal/models/domain"
	"EpicScoreBot/internal/report"

	"github.com/google/uuid"
)

func mustParseDate(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("invalid test date %q: %v", s, err)
	}
	return tm
}

// fakeGanttTaskSource — тестовая реализация GanttTaskSource (см.
// gantt.Service.GetTeamTasks в проде) для unit-тестов
// epicService.GetReportData (tasks.md §1.2/§2.6).
type fakeGanttTaskSource struct {
	tasks []domain.GanttTask
	err   error
}

func (f *fakeGanttTaskSource) GetTeamTasks(ctx context.Context, teamID uuid.UUID) ([]domain.GanttTask, error) {
	return f.tasks, f.err
}

// minimalCapacityRepo строит MockRepository, достаточный для
// BuildCapacityReport без единого эпика — команда без эпиков в периоде
// (используется тестами, которые проверяют только заполнение
// ReportData.GanttTasks, а не остальную часть отчёта — она уже покрыта
// TestEpicService_GetReportData). periodEpics — эпики верхнего уровня,
// которые вернёт GetEpicsByTeamYearQuarterFunc (§2.6: используется и ядром
// агрегатора, и фильтром задач Ганта по отчётному периоду).
func minimalCapacityRepo(teamID uuid.UUID, periodEpics ...domain.Epic) *MockRepository {
	return &MockRepository{
		GetTeamByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
			return &domain.Team{ID: teamID, Name: "Команда"}, nil
		},
		GetEpicsByTeamYearQuarterFunc: func(ctx context.Context, tID uuid.UUID, year, quarter int) ([]domain.Epic, error) {
			return periodEpics, nil
		},
	}
}

func TestEpicService_GetReportData_NoGanttTaskSource(t *testing.T) {
	ctx := context.Background()
	log := newDiscardLogger()
	teamID := uuid.New()

	// WithGanttTaskSource ни разу не вызывался — GetReportData обязана
	// отработать без ошибок, просто не заполнив GanttTasks.
	s := NewEpicService(log, minimalCapacityRepo(teamID))

	data, err := s.GetReportData(ctx, teamID, 2026, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data.GanttTasks) != 0 {
		t.Errorf("expected empty GanttTasks without a configured source, got %d rows", len(data.GanttTasks))
	}
}

func TestEpicService_GetReportData_GanttTaskSourceError_DoesNotFailReport(t *testing.T) {
	ctx := context.Background()
	log := newDiscardLogger()
	teamID := uuid.New()

	src := &fakeGanttTaskSource{err: errors.New("db unavailable")}
	s := NewEpicService(log, minimalCapacityRepo(teamID)).WithGanttTaskSource(src)

	data, err := s.GetReportData(ctx, teamID, 2026, 3)
	if err != nil {
		t.Fatalf("expected GetReportData to succeed despite gantt task source error, got: %v", err)
	}
	if data == nil {
		t.Fatal("expected non-nil report data")
	}
	if len(data.GanttTasks) != 0 {
		t.Errorf("expected empty GanttTasks on source error, got %d rows", len(data.GanttTasks))
	}
	// Остальной отчёт по-прежнему должен формироваться штатно.
	if data.TeamName != "Команда" {
		t.Errorf("expected the rest of the report to be built normally, got TeamName=%q", data.TeamName)
	}
}

func TestEpicService_GetReportData_GanttTasksEmpty(t *testing.T) {
	ctx := context.Background()
	log := newDiscardLogger()
	teamID := uuid.New()

	src := &fakeGanttTaskSource{tasks: nil, err: nil}
	s := NewEpicService(log, minimalCapacityRepo(teamID)).WithGanttTaskSource(src)

	data, err := s.GetReportData(ctx, teamID, 2026, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data.GanttTasks) != 0 {
		t.Errorf("expected empty GanttTasks for a team with no generated tasks, got %d rows", len(data.GanttTasks))
	}
}

// fullTreeGanttTasks строит реалистичный плоский срез domain.GanttTask для
// одного эпика периода: EpicID у ВСЕХ строк (родительской, стори, ролевой)
// равен переданному domain-эпику epicDomainID — так же, как в проде (см.
// generateTaskRowsForEpic/EpicID в ganttService.go) — а собственный ID
// каждой строки gantt_tasks(id) — отдельный, случайный.
func fullTreeGanttTasks(epicDomainID uuid.UUID, userID, roleID uuid.UUID, start, end time.Time) []domain.GanttTask {
	epicRow := uuid.New()
	storyRow := uuid.New()
	roleRow := uuid.New()
	return []domain.GanttTask{
		{ID: epicRow, EpicID: epicDomainID, IsParent: true, ParentTaskID: nil, Name: "EP-1: Эпик", StartDate: start, EndDate: end},
		{ID: storyRow, EpicID: epicDomainID, IsParent: true, ParentTaskID: &epicRow, Name: "EP-1-S1: Стори", StartDate: start, EndDate: end},
		{ID: roleRow, EpicID: epicDomainID, IsParent: false, ParentTaskID: &storyRow, RoleID: &roleID, Name: "BE разработчик",
			StartDate: start, EndDate: end, AssigneeID: &userID, Progress: 100},
	}
}

func TestEpicService_GetReportData_GanttTasksPopulated(t *testing.T) {
	ctx := context.Background()
	log := newDiscardLogger()
	teamID := uuid.New()
	userID := uuid.New()
	roleID := uuid.New()
	epicDomainID := uuid.New()

	start := mustParseDate(t, "2026-07-01")
	end := mustParseDate(t, "2026-07-10")

	tasks := fullTreeGanttTasks(epicDomainID, userID, roleID, start, end)

	repo := minimalCapacityRepo(teamID, domain.Epic{ID: epicDomainID, Number: "EP-1", Name: "Эпик", TeamID: teamID, Year: 2026, Quarter: 3})
	repo.GetUsersByTeamIDFunc = func(ctx context.Context, tID uuid.UUID) ([]domain.User, error) {
		return []domain.User{{ID: userID, FirstName: "Иван", LastName: "Иванов"}}, nil
	}

	src := &fakeGanttTaskSource{tasks: tasks}
	s := NewEpicService(log, repo).WithGanttTaskSource(src)

	data, err := s.GetReportData(ctx, teamID, 2026, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data.GanttTasks) != 3 {
		t.Fatalf("expected 3 gantt report rows, got %d: %+v", len(data.GanttTasks), data.GanttTasks)
	}

	epicR, storyR, roleR := data.GanttTasks[0], data.GanttTasks[1], data.GanttTasks[2]

	if epicR.Level != report.GanttReportLevelEpic || epicR.Depth != 0 {
		t.Errorf("epic row: got level=%v depth=%d, want epic/0", epicR.Level, epicR.Depth)
	}
	if storyR.Level != report.GanttReportLevelStory || storyR.Depth != 1 {
		t.Errorf("story row: got level=%v depth=%d, want story/1", storyR.Level, storyR.Depth)
	}
	if roleR.Level != report.GanttReportLevelRole || roleR.Depth != 2 {
		t.Errorf("role row: got level=%v depth=%d, want role/2", roleR.Level, roleR.Depth)
	}
	if roleR.Name != "BE разработчик" {
		t.Errorf("role row: got name=%q, want %q", roleR.Name, "BE разработчик")
	}
	if !roleR.HasAssignee || roleR.AssigneeName != "Иван Иванов" {
		t.Errorf("role row: got HasAssignee=%v AssigneeName=%q, want true/\"Иван Иванов\"", roleR.HasAssignee, roleR.AssigneeName)
	}
	if !roleR.Completed {
		t.Error("role row: expected Completed=true for Progress=100")
	}
	if epicR.Completed {
		t.Error("epic row: expected Completed=false (Progress=0, no ActualEndDate)")
	}
}

func TestEpicService_GetReportData_GanttTasksLegacyEpicWithoutStory(t *testing.T) {
	ctx := context.Background()
	log := newDiscardLogger()
	teamID := uuid.New()
	roleID := uuid.New()
	epicDomainID := uuid.New()

	epicRow := uuid.New()
	roleRow := uuid.New()

	start := mustParseDate(t, "2026-07-01")
	end := mustParseDate(t, "2026-07-05")

	tasks := []domain.GanttTask{
		{ID: epicRow, EpicID: epicDomainID, IsParent: true, ParentTaskID: nil, Name: "EP-2: Легаси эпик", StartDate: start, EndDate: end},
		{ID: roleRow, EpicID: epicDomainID, IsParent: false, ParentTaskID: &epicRow, RoleID: &roleID, Name: "Аналитик", StartDate: start, EndDate: end},
	}

	repo := minimalCapacityRepo(teamID, domain.Epic{ID: epicDomainID, Number: "EP-2", Name: "Легаси эпик", TeamID: teamID, Year: 2026, Quarter: 3})
	src := &fakeGanttTaskSource{tasks: tasks}
	s := NewEpicService(log, repo).WithGanttTaskSource(src)

	data, err := s.GetReportData(ctx, teamID, 2026, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data.GanttTasks) != 2 {
		t.Fatalf("expected 2 gantt report rows, got %d", len(data.GanttTasks))
	}
	roleR := data.GanttTasks[1]
	if roleR.Level != report.GanttReportLevelRole || roleR.Depth != 1 {
		t.Errorf("role row directly under a legacy epic: got level=%v depth=%d, want role/1", roleR.Level, roleR.Depth)
	}
	if roleR.HasAssignee {
		t.Error("expected HasAssignee=false: task has no AssigneeID")
	}
}

// ── 2.6 Фильтрация задач диаграммы по отчётному периоду ─────────────────────

func TestEpicService_GetReportData_GanttTasksFilteredByReportPeriod(t *testing.T) {
	ctx := context.Background()
	log := newDiscardLogger()
	teamID := uuid.New()
	roleID := uuid.New()

	// Эпик отчётного периода (Q3 2026) — задача заканчивается уже В
	// СЛЕДУЮЩЕМ квартале (Q4), но принадлежит эпику Q3: не должна
	// обрезаться по границе квартала (Requirement «Календарная шкала
	// покрывает весь диапазон работ» — не путать с фильтрацией по эпикам).
	q3EpicID := uuid.New()
	q3Start := mustParseDate(t, "2026-07-01")
	q3End := mustParseDate(t, "2026-10-15") // за пределами Q3, внутри Q4

	// Эпик другого периода (Q4 2026) — целиком отсекается фильтром §2.6.
	q4EpicID := uuid.New()
	q4Start := mustParseDate(t, "2026-10-01")
	q4End := mustParseDate(t, "2026-10-20")

	tasks := []domain.GanttTask{
		{ID: uuid.New(), EpicID: q3EpicID, IsParent: true, ParentTaskID: nil, Name: "EP-Q3: Эпик отчётного квартала", StartDate: q3Start, EndDate: q3End},
		{ID: uuid.New(), EpicID: q3EpicID, IsParent: false, RoleID: &roleID, Name: "BE разработчик", StartDate: q3Start, EndDate: q3End},
		{ID: uuid.New(), EpicID: q4EpicID, IsParent: true, ParentTaskID: nil, Name: "EP-Q4: Эпик другого квартала", StartDate: q4Start, EndDate: q4End},
		{ID: uuid.New(), EpicID: q4EpicID, IsParent: false, RoleID: &roleID, Name: "FE разработчик", StartDate: q4Start, EndDate: q4End},
	}

	repo := minimalCapacityRepo(teamID, domain.Epic{ID: q3EpicID, Number: "EP-Q3", Name: "Эпик отчётного квартала", TeamID: teamID, Year: 2026, Quarter: 3})
	src := &fakeGanttTaskSource{tasks: tasks}
	s := NewEpicService(log, repo).WithGanttTaskSource(src)

	data, err := s.GetReportData(ctx, teamID, 2026, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(data.GanttTasks) != 2 {
		t.Fatalf("expected only the Q3 epic's 2 rows to remain, got %d: %+v", len(data.GanttTasks), data.GanttTasks)
	}
	for _, row := range data.GanttTasks {
		if row.Name == "EP-Q4: Эпик другого квартала" || row.Name == "FE разработчик" {
			t.Errorf("row %q from a different quarter's epic must not be present in the report period diagram", row.Name)
		}
	}

	// Задача отчётного эпика, заканчивающаяся за границей квартала, не
	// обрезана — сохраняет своё исходное EndDate целиком.
	epicRow := data.GanttTasks[0]
	if epicRow.Name != "EP-Q3: Эпик отчётного квартала" {
		t.Fatalf("expected the first row to be the Q3 epic, got %q", epicRow.Name)
	}
	if !epicRow.EndDate.Equal(q3End) {
		t.Errorf("expected the in-period epic's task to keep its full end date %v, got %v (must not be clipped to the quarter boundary)", q3End, epicRow.EndDate)
	}
}
