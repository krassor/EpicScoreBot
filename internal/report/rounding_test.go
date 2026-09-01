package report

import "testing"

func TestRoundCapacityCell(t *testing.T) {
	cases := []struct {
		name  string
		score float64
		want  int
	}{
		{"ровно ноль", 0.0, 0},
		{"целое значение не меняется", 5.0, 5},
		{"дробная часть .1 округляется вверх", 3.1, 4},
		{"дробная часть .9 округляется вверх", 3.9, 4},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RoundCapacityCell(tc.score); got != tc.want {
				t.Errorf("RoundCapacityCell(%v) = %d, want %d", tc.score, got, tc.want)
			}
		})
	}
}

func TestRoundCapacityMatrix(t *testing.T) {
	epics := []EpicReportItem{
		{Number: "EP-1", RoleScores: map[string]float64{"Backend": 1.1, "Frontend": 2.9}},
		{Number: "EP-2", RoleScores: map[string]float64{"Backend": 0.3, "Frontend": 0.0}},
	}
	roleNames := []string{"Backend", "Frontend"}

	m := RoundCapacityMatrix(epics, roleNames)

	t.Run("поячеечное округление вверх", func(t *testing.T) {
		cases := map[string]struct {
			epicIdx  int
			roleName string
			want     int
		}{
			"EP-1 Backend (1.1 -> 2)":  {0, "Backend", 2},
			"EP-1 Frontend (2.9 -> 3)": {0, "Frontend", 3},
			"EP-2 Backend (0.3 -> 1)":  {1, "Backend", 1},
			"EP-2 Frontend (0.0 -> 0)": {1, "Frontend", 0},
		}
		for name, tc := range cases {
			t.Run(name, func(t *testing.T) {
				if got := m.Cells[tc.epicIdx][tc.roleName]; got != tc.want {
					t.Errorf("cell = %d, want %d", got, tc.want)
				}
			})
		}
	})

	t.Run("Итого по эпику — сумма округлённых ячеек строки", func(t *testing.T) {
		if m.EpicTotals[0] != 5 { // 2 + 3
			t.Errorf("EP-1 total = %d, want 5", m.EpicTotals[0])
		}
		if m.EpicTotals[1] != 1 { // 1 + 0
			t.Errorf("EP-2 total = %d, want 1", m.EpicTotals[1])
		}
	})

	t.Run("Запланировано по роли — сумма округлённых ячеек колонки, не ceil точной суммы", func(t *testing.T) {
		// Backend: точная сумма 1.1+0.3=1.4 -> ceil(1.4)=2, но ожидаем
		// сумму уже округлённых ячеек: ceil(1.1)+ceil(0.3) = 2+1 = 3.
		if got, want := m.RolePlanned["Backend"], 3; got != want {
			t.Errorf("RolePlanned[Backend] = %d, want %d", got, want)
		}
		if got, want := m.RolePlanned["Frontend"], 3; got != want { // 3 + 0
			t.Errorf("RolePlanned[Frontend] = %d, want %d", got, want)
		}
	})

	t.Run("сумма Итого по эпикам равна сумме Запланировано по ролям", func(t *testing.T) {
		var totalByEpics int
		for _, v := range m.EpicTotals {
			totalByEpics += v
		}
		var totalByRoles int
		for _, v := range m.RolePlanned {
			totalByRoles += v
		}
		if totalByEpics != totalByRoles {
			t.Errorf("sum(EpicTotals)=%d != sum(RolePlanned)=%d", totalByEpics, totalByRoles)
		}
	})
}

func TestRoundCapacityMatrix_EmptyInput(t *testing.T) {
	m := RoundCapacityMatrix(nil, []string{"Backend"})
	if len(m.Cells) != 0 || len(m.EpicTotals) != 0 {
		t.Errorf("ожидалась пустая матрица для пустого списка эпиков, получено: %+v", m)
	}
	if m.RolePlanned["Backend"] != 0 {
		t.Errorf("RolePlanned[Backend] = %d, want 0", m.RolePlanned["Backend"])
	}
}

// TestRoundCapacityMatrix_ThreeSmallEpics проверяет явный случай: три эпика
// по 0.3 → Запланировано=3 (сумма округлённых 1+1+1), а не ceil(0.9)=1
// (отдельно округлённая точная сумма). Это гарантирует, что итоговые
// значения равны сумме отображаемых ячеек, как требует design.md.
func TestRoundCapacityMatrix_ThreeSmallEpics(t *testing.T) {
	epics := []EpicReportItem{
		{Number: "EP-1", RoleScores: map[string]float64{"Backend": 0.3}},
		{Number: "EP-2", RoleScores: map[string]float64{"Backend": 0.3}},
		{Number: "EP-3", RoleScores: map[string]float64{"Backend": 0.3}},
	}
	roleNames := []string{"Backend"}

	m := RoundCapacityMatrix(epics, roleNames)

	// Каждая ячейка: ceil(0.3) = 1
	if got, want := m.Cells[0]["Backend"], 1; got != want {
		t.Errorf("EP-1 Backend: got %d, want %d", got, want)
	}
	if got, want := m.Cells[1]["Backend"], 1; got != want {
		t.Errorf("EP-2 Backend: got %d, want %d", got, want)
	}
	if got, want := m.Cells[2]["Backend"], 1; got != want {
		t.Errorf("EP-3 Backend: got %d, want %d", got, want)
	}

	// Итого по эпикам: каждый = 1
	if got, want := m.EpicTotals[0], 1; got != want {
		t.Errorf("EP-1 total: got %d, want %d", got, want)
	}
	if got, want := m.EpicTotals[1], 1; got != want {
		t.Errorf("EP-2 total: got %d, want %d", got, want)
	}
	if got, want := m.EpicTotals[2], 1; got != want {
		t.Errorf("EP-3 total: got %d, want %d", got, want)
	}

	// Запланировано Backend: сумма округлённых ячеек = 1+1+1 = 3,
	// а НЕ ceil(0.3+0.3+0.3) = ceil(0.9) = 1
	if got, want := m.RolePlanned["Backend"], 3; got != want {
		t.Errorf("RolePlanned[Backend]: got %d, want %d (sum of rounded cells), not ceil(0.9)=%d", got, want, 1)
	}

	// Общий итог: сумма EpicTotals = сумма RolePlanned
	var totalByEpics int
	for _, v := range m.EpicTotals {
		totalByEpics += v
	}
	var totalByRoles int
	for _, v := range m.RolePlanned {
		totalByRoles += v
	}
	if totalByEpics != totalByRoles {
		t.Errorf("sum(EpicTotals)=%d != sum(RolePlanned)=%d", totalByEpics, totalByRoles)
	}
}

// TestRoundCapacityMatrix_BoundaryZeroFraction проверяет граничные случаи
// с нулевой дробной частью (например, ровно 3.0) и очень малыми дробными
// частями (например, 3.001), чтобы убедиться, что округление вверх
// работает правильно во всех случаях.
func TestRoundCapacityMatrix_BoundaryZeroFraction(t *testing.T) {
	epics := []EpicReportItem{
		{Number: "EP-1", RoleScores: map[string]float64{"Backend": 3.0, "Frontend": 3.001}},
		{Number: "EP-2", RoleScores: map[string]float64{"Backend": 0.001, "Frontend": 0.5}},
	}
	roleNames := []string{"Backend", "Frontend"}

	m := RoundCapacityMatrix(epics, roleNames)

	// EP-1: Backend 3.0 → 3, Frontend 3.001 → 4
	if got, want := m.Cells[0]["Backend"], 3; got != want {
		t.Errorf("EP-1 Backend (3.0): got %d, want %d", got, want)
	}
	if got, want := m.Cells[0]["Frontend"], 4; got != want {
		t.Errorf("EP-1 Frontend (3.001): got %d, want %d", got, want)
	}

	// EP-2: Backend 0.001 → 1, Frontend 0.5 → 1
	if got, want := m.Cells[1]["Backend"], 1; got != want {
		t.Errorf("EP-2 Backend (0.001): got %d, want %d", got, want)
	}
	if got, want := m.Cells[1]["Frontend"], 1; got != want {
		t.Errorf("EP-2 Frontend (0.5): got %d, want %d", got, want)
	}

	// Проверяем суммы
	if got, want := m.RolePlanned["Backend"], 4; got != want { // 3 + 1
		t.Errorf("RolePlanned[Backend]: got %d, want %d", got, want)
	}
	if got, want := m.RolePlanned["Frontend"], 5; got != want { // 4 + 1
		t.Errorf("RolePlanned[Frontend]: got %d, want %d", got, want)
	}
}

func TestRoundCapacityFloor(t *testing.T) {
	cases := []struct {
		name     string
		capacity float64
		want     int
	}{
		{"ровно ноль", 0.0, 0},
		{"целое значение не меняется", 5.0, 5},
		{"дробная часть .1 округляется вниз", 3.1, 3},
		{"дробная часть .9 округляется вниз", 3.9, 3},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RoundCapacityFloor(tc.capacity); got != tc.want {
				t.Errorf("RoundCapacityFloor(%v) = %d, want %d", tc.capacity, got, tc.want)
			}
		})
	}
}

func TestRoundRoleCapacities(t *testing.T) {
	// headcount(R) × 8 × 6 × 0.838 обычно даёт дробное число — берём
	// заведомо дробные значения для проверки поролевого floor.
	roleCapacities := []RoleCapacityData{
		{RoleName: "Backend", Capacity: 40.224},  // 3 участника
		{RoleName: "Frontend", Capacity: 13.408}, // 1 участник
		{RoleName: "QA", Capacity: 0.0},          // нет участников с этой ролью
	}

	m := RoundRoleCapacities(roleCapacities)

	t.Run("поролевое округление вниз", func(t *testing.T) {
		cases := map[string]struct {
			roleName string
			want     int
		}{
			"Backend (40.224 -> 40)":  {"Backend", 40},
			"Frontend (13.408 -> 13)": {"Frontend", 13},
			"QA (0.0 -> 0)":           {"QA", 0},
		}
		for name, tc := range cases {
			t.Run(name, func(t *testing.T) {
				if got := m.RoleCapacity[tc.roleName]; got != tc.want {
					t.Errorf("RoleCapacity[%s] = %d, want %d", tc.roleName, got, tc.want)
				}
			})
		}
	})

	t.Run("общий итог — сумма округлённых величин по ролям", func(t *testing.T) {
		// 40 + 13 + 0 = 53, а НЕ floor(40.224+13.408+0.0) = floor(53.632) = 53
		// (в данном примере совпадает случайно — проверяем явно ниже отдельным
		// кейсом с расхождением).
		if got, want := m.Total, 53; got != want {
			t.Errorf("Total = %d, want %d", got, want)
		}
	})
}

// TestRoundRoleCapacities_SumDiffersFromFloorOfExactSum проверяет явный
// случай расхождения между «суммой округлённых по ролям» (требуемое
// поведение) и «floor от точной суммы» (отклонённая альтернатива) — чтобы
// зафиксировать именно то правило агрегации, которое требует design.md
// (Decision 5): Total = Σ floor(Capacity[R]), а не floor(Σ Capacity[R]).
func TestRoundRoleCapacities_SumDiffersFromFloorOfExactSum(t *testing.T) {
	roleCapacities := []RoleCapacityData{
		{RoleName: "Backend", Capacity: 1.9},
		{RoleName: "Frontend", Capacity: 1.9},
		{RoleName: "QA", Capacity: 1.9},
	}

	m := RoundRoleCapacities(roleCapacities)

	// Σ floor(1.9) = 1+1+1 = 3, тогда как floor(1.9*3) = floor(5.7) = 5.
	if got, want := m.Total, 3; got != want {
		t.Errorf("Total = %d, want %d (sum of per-role floor, not floor of exact sum)", got, want)
	}
}

func TestRoundRoleCapacities_EmptyInput(t *testing.T) {
	m := RoundRoleCapacities(nil)
	if len(m.RoleCapacity) != 0 {
		t.Errorf("ожидалась пустая карта для пустого списка ролей, получено: %+v", m.RoleCapacity)
	}
	if m.Total != 0 {
		t.Errorf("Total = %d, want 0", m.Total)
	}
}
