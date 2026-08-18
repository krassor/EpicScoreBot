package gantt

import (
	"EpicScoreBot/internal/models/domain"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestOrderTasksHierarchically_SingleEpicSingleStoryMultipleRoles
// Базовый случай: один эпик, одна стори с несколькими ролями.
// Порядок: эпик, затем стори, затем роли отсортированные по SortOrder.
func TestOrderTasksHierarchically_SingleEpicSingleStoryMultipleRoles(t *testing.T) {
	epic1ID := uuid.New()
	story1ID := uuid.New()
	role1ID := uuid.New()
	role2ID := uuid.New()
	role3ID := uuid.New()

	tasks := []domain.GanttTask{
		// Эпик (корень)
		{
			ID:           epic1ID,
			Name:         "Epic 1",
			ParentTaskID: nil,
			SortOrder:    0,
			IsParent:     true,
		},
		// Стори (дочерняя эпику)
		{
			ID:           story1ID,
			Name:         "Story 1",
			ParentTaskID: &epic1ID,
			SortOrder:    1,
			IsParent:     true,
		},
		// Роли (дочерние стори, намеренно идут не в порядке sort_order)
		{
			ID:           role3ID,
			Name:         "Role C",
			ParentTaskID: &story1ID,
			SortOrder:    3,
			IsParent:     false,
		},
		{
			ID:           role1ID,
			Name:         "Role A",
			ParentTaskID: &story1ID,
			SortOrder:    1,
			IsParent:     false,
		},
		{
			ID:           role2ID,
			Name:         "Role B",
			ParentTaskID: &story1ID,
			SortOrder:    2,
			IsParent:     false,
		},
	}

	result := orderTasksHierarchically(tasks)

	// Проверяем порядок по индексам
	if len(result) != 5 {
		t.Fatalf("expected 5 tasks, got %d", len(result))
	}

	if result[0].ID != epic1ID {
		t.Errorf("result[0] = %v, want epic1ID", result[0].ID)
	}
	if result[1].ID != story1ID {
		t.Errorf("result[1] = %v, want story1ID", result[1].ID)
	}
	if result[2].ID != role1ID {
		t.Errorf("result[2] = %v, want role1ID (SortOrder=1)", result[2].ID)
	}
	if result[3].ID != role2ID {
		t.Errorf("result[3] = %v, want role2ID (SortOrder=2)", result[3].ID)
	}
	if result[4].ID != role3ID {
		t.Errorf("result[4] = %v, want role3ID (SortOrder=3)", result[4].ID)
	}
}

// TestOrderTasksHierarchically_InterleagedSortOrders
// Проверяет исходный баг: две стори с пересекающимися sort_order ролей.
// После переупорядочения ролей, все роли первой стори должны идти ПОДРЯД
// после неё, затем вторая стори и её роли, без чужих строк между ними.
func TestOrderTasksHierarchically_InterleagedSortOrders(t *testing.T) {
	epic1ID := uuid.New()
	story1ID := uuid.New()
	story2ID := uuid.New()

	// У story1: роли с sort_order 1, 2
	role1_1ID := uuid.New()
	role1_2ID := uuid.New()

	// У story2: роли с sort_order 1, 2 (пересекаются с story1)
	role2_1ID := uuid.New()
	role2_2ID := uuid.New()

	tasks := []domain.GanttTask{
		// Эпик
		{
			ID:           epic1ID,
			Name:         "Epic 1",
			ParentTaskID: nil,
			SortOrder:    0,
			IsParent:     true,
		},
		// Story 1 (sort_order=1 на уровне эпика)
		{
			ID:           story1ID,
			Name:         "Story 1",
			ParentTaskID: &epic1ID,
			SortOrder:    1,
			IsParent:     true,
		},
		// Story 2 (sort_order=2 на уровне эпика)
		{
			ID:           story2ID,
			Name:         "Story 2",
			ParentTaskID: &epic1ID,
			SortOrder:    2,
			IsParent:     true,
		},
		// Роли story1 (с sort_order 1, 2 — на уровне story1)
		{
			ID:           role1_1ID,
			Name:         "Story1 Role 1",
			ParentTaskID: &story1ID,
			SortOrder:    1,
			IsParent:     false,
		},
		{
			ID:           role1_2ID,
			Name:         "Story1 Role 2",
			ParentTaskID: &story1ID,
			SortOrder:    2,
			IsParent:     false,
		},
		// Роли story2 (с тем же sort_order 1, 2 — на уровне story2)
		{
			ID:           role2_1ID,
			Name:         "Story2 Role 1",
			ParentTaskID: &story2ID,
			SortOrder:    1,
			IsParent:     false,
		},
		{
			ID:           role2_2ID,
			Name:         "Story2 Role 2",
			ParentTaskID: &story2ID,
			SortOrder:    2,
			IsParent:     false,
		},
	}

	result := orderTasksHierarchically(tasks)

	if len(result) != 7 {
		t.Fatalf("expected 7 tasks, got %d", len(result))
	}

	// Проверяем, что порядок: Epic, Story1, Story1-Roles, Story2, Story2-Roles
	// (без чужих ролей между story1 и story2)
	epic1Idx := slices.IndexFunc(result, func(t domain.GanttTask) bool { return t.ID == epic1ID })
	story1Idx := slices.IndexFunc(result, func(t domain.GanttTask) bool { return t.ID == story1ID })
	story2Idx := slices.IndexFunc(result, func(t domain.GanttTask) bool { return t.ID == story2ID })
	role1_1Idx := slices.IndexFunc(result, func(t domain.GanttTask) bool { return t.ID == role1_1ID })
	role1_2Idx := slices.IndexFunc(result, func(t domain.GanttTask) bool { return t.ID == role1_2ID })
	role2_1Idx := slices.IndexFunc(result, func(t domain.GanttTask) bool { return t.ID == role2_1ID })
	role2_2Idx := slices.IndexFunc(result, func(t domain.GanttTask) bool { return t.ID == role2_2ID })

	// Проверяем, что индексы возрастают
	if !(epic1Idx < story1Idx) {
		t.Errorf("epic1 should come before story1: %d < %d", epic1Idx, story1Idx)
	}
	if !(story1Idx < role1_1Idx) {
		t.Errorf("story1 should come before its roles: %d < %d", story1Idx, role1_1Idx)
	}
	if !(role1_1Idx < role1_2Idx) {
		t.Errorf("role1_1 should come before role1_2: %d < %d", role1_1Idx, role1_2Idx)
	}
	// Все роли story1 должны идти до story2
	if !(role1_2Idx < story2Idx) {
		t.Errorf("all story1 roles should come before story2: %d < %d", role1_2Idx, story2Idx)
	}
	if !(story2Idx < role2_1Idx) {
		t.Errorf("story2 should come before its roles: %d < %d", story2Idx, role2_1Idx)
	}
	if !(role2_1Idx < role2_2Idx) {
		t.Errorf("role2_1 should come before role2_2: %d < %d", role2_1Idx, role2_2Idx)
	}

	// Проверяем, что между role1_2 и story2 нет ролей story2
	for i := role1_2Idx + 1; i < story2Idx; i++ {
		if result[i].ID == role2_1ID || result[i].ID == role2_2ID {
			t.Errorf("no story2 roles should appear between story1 roles and story2 task")
		}
	}
}

// TestOrderTasksHierarchically_DuplicateSortOrder
// У стори и её собственной роли совпадает sort_order (оба = 1).
// Стори должна быть первой строкой в своём разделе (перед своими ролями).
func TestOrderTasksHierarchically_DuplicateSortOrder(t *testing.T) {
	epic1ID := uuid.New()
	story1ID := uuid.New()
	role1ID := uuid.New()
	role2ID := uuid.New()

	tasks := []domain.GanttTask{
		{
			ID:           epic1ID,
			Name:         "Epic 1",
			ParentTaskID: nil,
			SortOrder:    0,
			IsParent:     true,
		},
		// Стори с sort_order=1
		{
			ID:           story1ID,
			Name:         "Story 1",
			ParentTaskID: &epic1ID,
			SortOrder:    1,
			IsParent:     true,
		},
		// Роль с тем же sort_order=1, но потом роль с sort_order=2
		{
			ID:           role1ID,
			Name:         "Role 1",
			ParentTaskID: &story1ID,
			SortOrder:    1,
			IsParent:     false,
		},
		{
			ID:           role2ID,
			Name:         "Role 2",
			ParentTaskID: &story1ID,
			SortOrder:    2,
			IsParent:     false,
		},
	}

	result := orderTasksHierarchically(tasks)

	if len(result) != 4 {
		t.Fatalf("expected 4 tasks, got %d", len(result))
	}

	story1Idx := slices.IndexFunc(result, func(t domain.GanttTask) bool { return t.ID == story1ID })
	role1Idx := slices.IndexFunc(result, func(t domain.GanttTask) bool { return t.ID == role1ID })

	// Стори должна быть перед своими ролями
	if !(story1Idx < role1Idx) {
		t.Errorf("story1 should come before its own roles: story1 at %d, role1 at %d", story1Idx, role1Idx)
	}
}

// TestOrderTasksHierarchically_NonStandardSortOrder
// Роль имеет нестандартный sort_order (99), что может быть результатом ручного reorder.
// Она должна остаться внутри своей стори, не "убежать" в чужую стори или между стори.
func TestOrderTasksHierarchically_NonStandardSortOrder(t *testing.T) {
	epic1ID := uuid.New()
	story1ID := uuid.New()
	story2ID := uuid.New()
	role1_1ID := uuid.New()
	role1_99ID := uuid.New() // Нестандартный sort_order
	role2_1ID := uuid.New()

	tasks := []domain.GanttTask{
		{
			ID:           epic1ID,
			Name:         "Epic 1",
			ParentTaskID: nil,
			SortOrder:    0,
			IsParent:     true,
		},
		{
			ID:           story1ID,
			Name:         "Story 1",
			ParentTaskID: &epic1ID,
			SortOrder:    1,
			IsParent:     true,
		},
		{
			ID:           story2ID,
			Name:         "Story 2",
			ParentTaskID: &epic1ID,
			SortOrder:    2,
			IsParent:     true,
		},
		// Роли story1
		{
			ID:           role1_1ID,
			Name:         "Story1 Role 1",
			ParentTaskID: &story1ID,
			SortOrder:    1,
			IsParent:     false,
		},
		{
			ID:           role1_99ID,
			Name:         "Story1 Role 99",
			ParentTaskID: &story1ID,
			SortOrder:    99,
			IsParent:     false,
		},
		// Роли story2
		{
			ID:           role2_1ID,
			Name:         "Story2 Role 1",
			ParentTaskID: &story2ID,
			SortOrder:    1,
			IsParent:     false,
		},
	}

	result := orderTasksHierarchically(tasks)

	if len(result) != 6 {
		t.Fatalf("expected 6 tasks, got %d", len(result))
	}

	story1Idx := slices.IndexFunc(result, func(t domain.GanttTask) bool { return t.ID == story1ID })
	story2Idx := slices.IndexFunc(result, func(t domain.GanttTask) bool { return t.ID == story2ID })
	role1_1Idx := slices.IndexFunc(result, func(t domain.GanttTask) bool { return t.ID == role1_1ID })
	role1_99Idx := slices.IndexFunc(result, func(t domain.GanttTask) bool { return t.ID == role1_99ID })
	role2_1Idx := slices.IndexFunc(result, func(t domain.GanttTask) bool { return t.ID == role2_1ID })

	// Все роли story1 (включая role1_99) должны быть между story1 и story2
	if !(story1Idx < role1_1Idx && role1_1Idx < story2Idx) {
		t.Errorf("role1_1 should be between story1 and story2")
	}
	if !(story1Idx < role1_99Idx && role1_99Idx < story2Idx) {
		t.Errorf("role1_99 should be between story1 and story2, not after story2")
	}
	if !(story2Idx < role2_1Idx) {
		t.Errorf("story2 should come before role2_1")
	}

	// role1_99 с sort_order=99 должна быть после role1_1 с sort_order=1 (сортировка по sort_order)
	if !(role1_1Idx < role1_99Idx) {
		t.Errorf("role1_1 (sort_order=1) should come before role1_99 (sort_order=99): %d < %d", role1_1Idx, role1_99Idx)
	}
}

// TestOrderTasksHierarchically_MultipleEpics
// Несколько эпиков с историями и ролями.
// Относительный порядок эпиков в результате должен совпадать с входным порядком
// (roots не пересортировываются).
func TestOrderTasksHierarchically_MultipleEpics(t *testing.T) {
	epic1ID := uuid.New()
	epic2ID := uuid.New()
	story1ID := uuid.New()
	story2ID := uuid.New()
	role1_1ID := uuid.New()
	role1_2ID := uuid.New()
	role2_1ID := uuid.New()

	tasks := []domain.GanttTask{
		// Epic 1
		{
			ID:           epic1ID,
			Name:         "Epic 1",
			ParentTaskID: nil,
			SortOrder:    0,
			IsParent:     true,
		},
		// Story 1 (дочерняя epic1)
		{
			ID:           story1ID,
			Name:         "Story 1",
			ParentTaskID: &epic1ID,
			SortOrder:    1,
			IsParent:     true,
		},
		// Роли story1
		{
			ID:           role1_2ID,
			Name:         "Story1 Role 2",
			ParentTaskID: &story1ID,
			SortOrder:    2,
			IsParent:     false,
		},
		{
			ID:           role1_1ID,
			Name:         "Story1 Role 1",
			ParentTaskID: &story1ID,
			SortOrder:    1,
			IsParent:     false,
		},
		// Epic 2
		{
			ID:           epic2ID,
			Name:         "Epic 2",
			ParentTaskID: nil,
			SortOrder:    0,
			IsParent:     true,
		},
		// Story 2 (дочерняя epic2)
		{
			ID:           story2ID,
			Name:         "Story 2",
			ParentTaskID: &epic2ID,
			SortOrder:    1,
			IsParent:     true,
		},
		// Роли story2
		{
			ID:           role2_1ID,
			Name:         "Story2 Role 1",
			ParentTaskID: &story2ID,
			SortOrder:    1,
			IsParent:     false,
		},
	}

	result := orderTasksHierarchically(tasks)

	if len(result) != 7 {
		t.Fatalf("expected 7 tasks, got %d", len(result))
	}

	epic1Idx := slices.IndexFunc(result, func(t domain.GanttTask) bool { return t.ID == epic1ID })
	epic2Idx := slices.IndexFunc(result, func(t domain.GanttTask) bool { return t.ID == epic2ID })

	// Эпики должны сохранить входной порядок: epic1 перед epic2
	if !(epic1Idx < epic2Idx) {
		t.Errorf("epic1 should come before epic2 (input order): %d < %d", epic1Idx, epic2Idx)
	}

	story1Idx := slices.IndexFunc(result, func(t domain.GanttTask) bool { return t.ID == story1ID })
	story2Idx := slices.IndexFunc(result, func(t domain.GanttTask) bool { return t.ID == story2ID })
	role1_1Idx := slices.IndexFunc(result, func(t domain.GanttTask) bool { return t.ID == role1_1ID })
	role1_2Idx := slices.IndexFunc(result, func(t domain.GanttTask) bool { return t.ID == role1_2ID })
	role2_1Idx := slices.IndexFunc(result, func(t domain.GanttTask) bool { return t.ID == role2_1ID })

	// Роли story1 с sort_order=1 должна быть перед role с sort_order=2
	if !(role1_1Idx < role1_2Idx) {
		t.Errorf("role1_1 (sort_order=1) should come before role1_2 (sort_order=2)")
	}

	// Все роли story1 должны быть между story1 и story2
	if !(story1Idx < role1_1Idx && role1_1Idx < story2Idx) {
		t.Errorf("role1_1 should be between story1 and story2")
	}
	if !(story1Idx < role1_2Idx && role1_2Idx < story2Idx) {
		t.Errorf("role1_2 should be between story1 and story2")
	}

	// Роли story2 должны быть после story2
	if !(story2Idx < role2_1Idx) {
		t.Errorf("story2 should come before role2_1")
	}
}

// TestOrderTasksHierarchically_EmptyInput
// Пустой входной список должен вернуть пустой результат.
func TestOrderTasksHierarchically_EmptyInput(t *testing.T) {
	tasks := []domain.GanttTask{}
	result := orderTasksHierarchically(tasks)

	if len(result) != 0 {
		t.Errorf("expected empty result for empty input, got %d tasks", len(result))
	}
}

// TestOrderTasksHierarchically_OnlyRoots
// Только эпики без историй/ролей должны сохранить входной порядок.
func TestOrderTasksHierarchically_OnlyRoots(t *testing.T) {
	epic1ID := uuid.New()
	epic2ID := uuid.New()
	epic3ID := uuid.New()

	tasks := []domain.GanttTask{
		{
			ID:           epic1ID,
			Name:         "Epic 1",
			ParentTaskID: nil,
			SortOrder:    0,
			IsParent:     true,
		},
		{
			ID:           epic2ID,
			Name:         "Epic 2",
			ParentTaskID: nil,
			SortOrder:    0,
			IsParent:     true,
		},
		{
			ID:           epic3ID,
			Name:         "Epic 3",
			ParentTaskID: nil,
			SortOrder:    0,
			IsParent:     true,
		},
	}

	result := orderTasksHierarchically(tasks)

	if len(result) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(result))
	}

	// Проверяем, что порядок совпадает с входным
	if result[0].ID != epic1ID || result[1].ID != epic2ID || result[2].ID != epic3ID {
		t.Errorf("epic order should match input order")
	}
}

// TestOrderTasksHierarchically_DeeplyNestedHierarchy
// Проверка работы DFS для иерархии глубже, чем 3 уровня (если такая возможна).
// Пример: Epic -> Story -> Role -> SubRole (хотя в текущей модели это маловероятно,
// но функция работает рекурсивно и должна справиться).
func TestOrderTasksHierarchically_DeeplyNestedHierarchy(t *testing.T) {
	epic1ID := uuid.New()
	story1ID := uuid.New()
	role1ID := uuid.New()
	subrole1ID := uuid.New()

	tasks := []domain.GanttTask{
		{
			ID:           epic1ID,
			Name:         "Epic 1",
			ParentTaskID: nil,
			SortOrder:    0,
			IsParent:     true,
		},
		{
			ID:           subrole1ID,
			Name:         "SubRole 1",
			ParentTaskID: &role1ID,
			SortOrder:    1,
			IsParent:     false,
		},
		{
			ID:           story1ID,
			Name:         "Story 1",
			ParentTaskID: &epic1ID,
			SortOrder:    1,
			IsParent:     true,
		},
		{
			ID:           role1ID,
			Name:         "Role 1",
			ParentTaskID: &story1ID,
			SortOrder:    1,
			IsParent:     false,
		},
	}

	result := orderTasksHierarchically(tasks)

	if len(result) != 4 {
		t.Fatalf("expected 4 tasks, got %d", len(result))
	}

	epic1Idx := slices.IndexFunc(result, func(t domain.GanttTask) bool { return t.ID == epic1ID })
	story1Idx := slices.IndexFunc(result, func(t domain.GanttTask) bool { return t.ID == story1ID })
	role1Idx := slices.IndexFunc(result, func(t domain.GanttTask) bool { return t.ID == role1ID })
	subrole1Idx := slices.IndexFunc(result, func(t domain.GanttTask) bool { return t.ID == subrole1ID })

	// DFS order: epic1 -> story1 -> role1 -> subrole1
	if !(epic1Idx < story1Idx && story1Idx < role1Idx && role1Idx < subrole1Idx) {
		t.Errorf("DFS order should be: epic1 < story1 < role1 < subrole1")
	}
}

// TestOrderTasksHierarchically_StableSortByName
// Если sort_order совпадает у нескольких элементов, они должны быть отсортированы
// по имени (и сохранять стабильный порядок).
func TestOrderTasksHierarchically_StableSortByName(t *testing.T) {
	epic1ID := uuid.New()
	story1ID := uuid.New()
	role1ID := uuid.New() // "Aaa"
	role2ID := uuid.New() // "Bbb"
	role3ID := uuid.New() // "Ccc"

	tasks := []domain.GanttTask{
		{
			ID:           epic1ID,
			Name:         "Epic 1",
			ParentTaskID: nil,
			SortOrder:    0,
			IsParent:     true,
		},
		{
			ID:           story1ID,
			Name:         "Story 1",
			ParentTaskID: &epic1ID,
			SortOrder:    1,
			IsParent:     true,
		},
		// Все роли имеют одинаковый sort_order=1, но разные имена.
		// Входной порядок: Ccc, Aaa, Bbb (не в алфавитном порядке).
		{
			ID:           role3ID,
			Name:         "Ccc",
			ParentTaskID: &story1ID,
			SortOrder:    1,
			IsParent:     false,
		},
		{
			ID:           role1ID,
			Name:         "Aaa",
			ParentTaskID: &story1ID,
			SortOrder:    1,
			IsParent:     false,
		},
		{
			ID:           role2ID,
			Name:         "Bbb",
			ParentTaskID: &story1ID,
			SortOrder:    1,
			IsParent:     false,
		},
	}

	result := orderTasksHierarchically(tasks)

	if len(result) != 5 {
		t.Fatalf("expected 5 tasks, got %d", len(result))
	}

	role1Idx := slices.IndexFunc(result, func(t domain.GanttTask) bool { return t.ID == role1ID })
	role2Idx := slices.IndexFunc(result, func(t domain.GanttTask) bool { return t.ID == role2ID })
	role3Idx := slices.IndexFunc(result, func(t domain.GanttTask) bool { return t.ID == role3ID })

	// Так как все имеют sort_order=1, сортировка должна быть по имени: Aaa < Bbb < Ccc
	if !(role1Idx < role2Idx && role2Idx < role3Idx) {
		t.Errorf("roles with same sort_order should be sorted by name: Aaa (%d) < Bbb (%d) < Ccc (%d)",
			role1Idx, role2Idx, role3Idx)
	}
}

// TestOrderTasksHierarchically_PreservesTaskFields
// Проверяем, что функция не теряет и не меняет данные в задачах
// (сортировка не должна нарушить остальные поля).
func TestOrderTasksHierarchically_PreservesTaskFields(t *testing.T) {
	epic1ID := uuid.New()
	story1ID := uuid.New()
	role1ID := uuid.New()

	now := time.Now()

	tasks := []domain.GanttTask{
		{
			ID:           epic1ID,
			Name:         "Epic 1",
			ParentTaskID: nil,
			SortOrder:    0,
			IsParent:     true,
			StartDate:    now,
			EndDate:      now.AddDate(0, 1, 0),
			Progress:     0.5,
		},
		{
			ID:           story1ID,
			Name:         "Story 1",
			ParentTaskID: &epic1ID,
			SortOrder:    1,
			IsParent:     true,
			StartDate:    now.AddDate(0, 0, 1),
			EndDate:      now.AddDate(0, 1, 1),
			Progress:     0.75,
		},
		{
			ID:           role1ID,
			Name:         "Role 1",
			ParentTaskID: &story1ID,
			SortOrder:    1,
			IsParent:     false,
			StartDate:    now.AddDate(0, 0, 2),
			EndDate:      now.AddDate(0, 0, 10),
			Progress:     1.0,
		},
	}

	result := orderTasksHierarchically(tasks)

	// Проверяем, что данные сохранились
	for _, originalTask := range tasks {
		foundIdx := slices.IndexFunc(result, func(t domain.GanttTask) bool {
			return t.ID == originalTask.ID
		})
		if foundIdx == -1 {
			t.Errorf("task %s not found in result", originalTask.Name)
			continue
		}

		resultTask := result[foundIdx]
		if resultTask.Name != originalTask.Name {
			t.Errorf("name changed: %s -> %s", originalTask.Name, resultTask.Name)
		}
		if resultTask.Progress != originalTask.Progress {
			t.Errorf("progress changed: %f -> %f", originalTask.Progress, resultTask.Progress)
		}
		if !resultTask.StartDate.Equal(originalTask.StartDate) {
			t.Errorf("start date changed: %v -> %v", originalTask.StartDate, resultTask.StartDate)
		}
		if !resultTask.EndDate.Equal(originalTask.EndDate) {
			t.Errorf("end date changed: %v -> %v", originalTask.EndDate, resultTask.EndDate)
		}
	}
}
