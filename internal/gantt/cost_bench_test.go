package gantt

import (
	"EpicScoreBot/internal/models/domain"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Замер стоимости повторного прохода (задача 6.8, openspec/changes/
// add-task-start-constraints/design.md, Risks "Повторный проход удорожает
// пересчёт"). Число обращений к БД уже подтверждено не растущим отдельными
// тестами (TestRecalculateTeamSchedule_NoWaitForEdges_ConvergesInOnePassWhenStable
// и TestRecalculateTeamSchedule_WaitForEdges_CostsAtLeastOneExtraPass в
// start_constraint_test.go) — здесь измеряется именно ВРЕМЯ вычисления на
// объёме, сопоставимом с реальным (design.md Решение 6: "на наблюдаемых
// данных — 85" листовых задач на команду).

// realisticTeamFixture — результат buildRealisticTeam: всё, что бенчмарку
// нужно для адресной расстановки ссылок "не ранее задачи" корректными
// парами (storyID, roleID) — теми же, какими адресуется task_assignments
// (design.md Решение 1), а не gantt_tasks.epic_id (тот у любой листовой
// задачи под стори указывает на ВЕРХНИЙ эпик, а не на стори).
type realisticTeamFixture struct {
	teamID   uuid.UUID
	storyIDs []uuid.UUID // по одной стори на эпик, в порядке очереди (sort_order)
	roleIDs  map[string]uuid.UUID
}

// buildRealisticTeam создаёт команду с epicsCount эпиками, у каждого по
// одной стори с 5 ролями (Аналитик, BE, FE, QA, IT-лидер — так же, как
// defaultRoleOrder) и пулом из 3 кандидатов на роль, что даёт
// epicsCount*5 листовых задач и упражняет тот же путь round-robin/пулов,
// что и в бою.
func buildRealisticTeam(f *fakeRepo, epicsCount int) realisticTeamFixture {
	teamID := uuid.New()

	roleNames := []string{"Аналитик", "BE разработчик", "FE разработчик", "Тестировщик", "IT-лидер"}
	roleIDs := make(map[string]uuid.UUID, len(roleNames))
	for _, name := range roleNames {
		roleID := uuid.New()
		roleIDs[name] = roleID
		f.addRole(&domain.Role{ID: roleID, Name: name})
		for u := 0; u < 3; u++ {
			f.addUser(&domain.User{ID: uuid.New(), FirstName: fmt.Sprintf("%s-%d", name, u)},
				[]uuid.UUID{teamID}, []uuid.UUID{roleID})
		}
	}

	fixture := realisticTeamFixture{teamID: teamID, roleIDs: roleIDs}
	for e := 0; e < epicsCount; e++ {
		epicID := uuid.New()
		storyID := uuid.New()
		so := e + 1
		f.addEpic(&domain.Epic{ID: epicID, Number: fmt.Sprintf("E-%d", e+1), Name: fmt.Sprintf("Epic %d", e+1), TeamID: teamID, SortOrder: &so})
		f.addEpic(&domain.Epic{ID: storyID, Number: fmt.Sprintf("E-%d-S1", e+1), Name: "Story 1", TeamID: teamID, ParentEpicID: &epicID})
		fixture.storyIDs = append(fixture.storyIDs, storyID)
		var scores []domain.EpicRoleScore
		for _, roleID := range roleIDs {
			scores = append(scores, domain.EpicRoleScore{EpicID: storyID, RoleID: roleID, WeightedAvg: 3.0})
		}
		f.roleScores[storyID] = scores
	}

	return fixture
}

// generateAllEpics генерирует задачи для всех эпиков команды и пересчитывает
// расписание один раз в конце — тем же приёмом, что и GenerateTasksForQuarter.
func generateAllEpics(ctx context.Context, svc *Service, f *fakeRepo, teamID uuid.UUID, start time.Time) error {
	for _, e := range f.epics {
		if e.TeamID != teamID || e.ParentEpicID != nil {
			continue
		}
		if _, err := svc.generateTaskRowsForEpic(ctx, e.ID, start); err != nil {
			return err
		}
	}
	_, err := svc.RecalculateTeamSchedule(ctx, teamID)
	return err
}

// leafTaskInStory находит листовую (ролевую) задачу заданной роли внутри
// строки-стори storyID — по родительскому gantt_tasks.id стори (которую
// сперва находим по её собственному "<number>: <name>", как это делает
// recalculateEpicSchedule/resolveAssignmentKey), а не по
// gantt_tasks.epic_id (тот у любой листовой задачи указывает на ВЕРХНИЙ
// эпик, а не на стори — design.md Context).
func leafTaskInStory(f *fakeRepo, storyID uuid.UUID, roleID uuid.UUID) *domain.GanttTask {
	story, ok := f.epics[storyID]
	if !ok {
		return nil
	}
	wantParentName := fmt.Sprintf("%s: %s", story.Number, story.Name)
	var parentTaskID uuid.UUID
	found := false
	for _, task := range f.tasks {
		if task.IsParent && task.Name == wantParentName {
			parentTaskID = task.ID
			found = true
			break
		}
	}
	if !found {
		return nil
	}
	for _, task := range f.tasks {
		if !task.IsParent && task.RoleID != nil && *task.RoleID == roleID &&
			task.ParentTaskID != nil && *task.ParentTaskID == parentTaskID {
			return task
		}
	}
	return nil
}

// BenchmarkRecalculateTeamSchedule_NoWaitForEdges измеряет типичный случай
// (подавляющее большинство команд): 85 листовых задач, ни одной ссылки
// "не ранее задачи" — базовая стоимость пересчёта на уже стабильном
// расписании (design.md: "в типичном случае второй проход не находит
// изменений и цикл останавливается").
func BenchmarkRecalculateTeamSchedule_NoWaitForEdges(b *testing.B) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)
	fixture := buildRealisticTeam(f, 17) // 17 epics * 5 roles = 85 leaf tasks
	start := date(2026, 7, 13)
	if err := generateAllEpics(ctx, svc, f, fixture.teamID, start); err != nil {
		b.Fatalf("setup: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.RecalculateTeamSchedule(ctx, fixture.teamID); err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
	}
}

// BenchmarkRecalculateTeamSchedule_WithWaitForEdges измеряет тот же объём
// (85 листовых задач), но с 5 действующими ссылками "не ранее задачи"
// (реалистичное число — не на каждой задаче), указывающими на МЕНЕЕ
// приоритетный эпик (forward-ссылка, требующая повторного прохода —
// design.md Решение 2). Разница со значением
// BenchmarkRecalculateTeamSchedule_NoWaitForEdges — и есть цена фичи на
// уже стабильном расписании.
func BenchmarkRecalculateTeamSchedule_WithWaitForEdges(b *testing.B) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)
	fixture := buildRealisticTeam(f, 17)
	start := date(2026, 7, 13)
	if err := generateAllEpics(ctx, svc, f, fixture.teamID, start); err != nil {
		b.Fatalf("setup: %v", err)
	}

	n := len(fixture.storyIDs)
	beRoleID := fixture.roleIDs["BE разработчик"]
	analystRoleID := fixture.roleIDs["Аналитик"]
	for i := 0; i < 5; i++ {
		beTask := leafTaskInStory(f, fixture.storyIDs[i], beRoleID)
		if beTask == nil {
			b.Fatalf("setup: BE task not found for story %d", i)
		}
		targetStoryID := fixture.storyIDs[(i+n-3)%n] // менее приоритетный (позже в очереди) сосед
		if _, err := svc.SetTaskStartConstraint(ctx, beTask.ID, nil, &targetStoryID, &analystRoleID); err != nil {
			b.Fatalf("setup SetTaskStartConstraint: %v", err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.RecalculateTeamSchedule(ctx, fixture.teamID); err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
	}
}
