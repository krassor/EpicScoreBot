package report

import (
	"bytes"
	"testing"

	"github.com/xuri/excelize/v2"
)

// TestGenerateCapacityXLSX_CapacitySheetUsesRoundedRiskAdjustedScores
// проверяет, что лист «Вместимость» показывает только округлённую вверх
// риск-скорректированную трудоёмкость (role_scores), одну колонку «Итого с
// рисками (чд)» на эпик (без «С рисками (чд)»), а «Запланировано» по роли —
// сумму уже округлённых ячеек; «Доступно»/«Разница» — округлённую вниз
// поролево ёмкость и её остаток (см. design.md change
// simplify-capacity-report, Decision 2/5).
func TestGenerateCapacityXLSX_CapacitySheetUsesRoundedRiskAdjustedScores(t *testing.T) {
	data := CapacityReportResponse{
		TeamName:      "Команда",
		Year:          2026,
		Quarter:       3,
		TotalCapacity: 1000.7,
		RoleCapacities: []RoleCapacityData{
			{RoleName: "Backend", Capacity: 500.7, Planned: 3, Diff: 497.7},
			{RoleName: "Frontend", Capacity: 500.3, Planned: 3.4, Diff: 496.9},
		},
		Epics: []EpicReportItem{
			{
				ID:            "epic-1",
				Number:        "EP-1",
				Name:          "Первый эпик",
				Type:          "feature",
				Status:        "scored",
				FinalScore:    3.4,
				RoleScores:    map[string]float64{"Backend": 1.1, "Frontend": 2.3},
				RawRoleScores: map[string]float64{"Backend": 1.0, "Frontend": 2.0},
			},
			{
				ID:            "epic-2",
				Number:        "EP-2",
				Name:          "Второй эпик",
				Type:          "techdebt",
				Status:        "scored",
				FinalScore:    1.1,
				RoleScores:    map[string]float64{"Backend": 0.3, "Frontend": 0.0},
				RawRoleScores: map[string]float64{"Backend": 0.3, "Frontend": 0.0},
			},
		},
		Quotas: map[string]QuotaData{
			"feature":           {LimitPercent: 40, ActualPercent: 10, Status: "OK"},
			"tech_architecture": {LimitPercent: 60, ActualPercent: 90, Status: "EXCEEDED"},
		},
	}

	xlsxBytes, err := GenerateCapacityXLSX(data)
	if err != nil {
		t.Fatalf("не удалось сгенерировать xlsx: %v", err)
	}

	f, err := excelize.OpenReader(bytes.NewReader(xlsxBytes))
	if err != nil {
		t.Fatalf("не удалось открыть сгенерированный xlsx: %v", err)
	}
	defer f.Close()

	const sheet = "Вместимость"

	// Строка 6 (после 3 строк шапки + пустой + заголовка) — данные EP-1.
	header, err := f.GetRows(sheet)
	if err != nil {
		t.Fatalf("не удалось прочитать строки листа: %v", err)
	}

	// Заголовок матрицы — 5-я строка (индекс 4): "Задача","Тип","Backend","Frontend","Итого с рисками (чд)".
	headerRow := header[4]
	wantHeader := []string{"Задача", "Тип", "Backend", "Frontend", "Итого с рисками (чд)"}
	if len(headerRow) != len(wantHeader) {
		t.Fatalf("неверное число колонок в заголовке: %v, ожидалось: %v", headerRow, wantHeader)
	}
	for i, w := range wantHeader {
		if headerRow[i] != w {
			t.Errorf("заголовок[%d] = %q, want %q", i, headerRow[i], w)
		}
	}

	// EP-1: Backend ceil(1.1)=2, Frontend ceil(2.3)=3, Итого=5.
	ep1Row := header[5]
	if got, want := ep1Row[2], "2"; got != want {
		t.Errorf("EP-1 Backend = %q, want %q", got, want)
	}
	if got, want := ep1Row[3], "3"; got != want {
		t.Errorf("EP-1 Frontend = %q, want %q", got, want)
	}
	if got, want := ep1Row[4], "5"; got != want {
		t.Errorf("EP-1 Итого = %q, want %q", got, want)
	}

	// EP-2: Backend ceil(0.3)=1, Frontend ceil(0.0)=0, Итого=1.
	ep2Row := header[6]
	if got, want := ep2Row[2], "1"; got != want {
		t.Errorf("EP-2 Backend = %q, want %q", got, want)
	}
	if got, want := ep2Row[4], "1"; got != want {
		t.Errorf("EP-2 Итого = %q, want %q", got, want)
	}

	// Запланировано: Backend = 2+1=3, Frontend = 3+0=3, Итого=6.
	plannedRow := header[7]
	if got, want := plannedRow[2], "3"; got != want {
		t.Errorf("Запланировано Backend = %q, want %q", got, want)
	}
	if got, want := plannedRow[3], "3"; got != want {
		t.Errorf("Запланировано Frontend = %q, want %q", got, want)
	}
	if got, want := plannedRow[4], "6"; got != want {
		t.Errorf("Запланировано Итого = %q, want %q", got, want)
	}

	// Доступно (емкость): Backend floor(500.7)=500, Frontend floor(500.3)=500,
	// общий итог = сумма округлённых по ролям = 1000 (а не отдельный
	// floor(1001.0) от точной суммы точных величин).
	capacityRow := header[8]
	if got, want := capacityRow[2], "500"; got != want {
		t.Errorf("Доступно Backend = %q, want %q", got, want)
	}
	if got, want := capacityRow[3], "500"; got != want {
		t.Errorf("Доступно Frontend = %q, want %q", got, want)
	}
	if got, want := capacityRow[4], "1000"; got != want {
		t.Errorf("Доступно Итого = %q, want %q", got, want)
	}

	// Разница = Доступно(округлённое) − Запланировано(округлённое): Backend
	// 500-3=497, Frontend 500-3=497, Итого 1000-6=994 — оба слагаемых уже
	// целые, отдельного округления не требуется.
	diffRow := header[9]
	if got, want := diffRow[2], "497"; got != want {
		t.Errorf("Разница Backend = %q, want %q", got, want)
	}
	if got, want := diffRow[3], "497"; got != want {
		t.Errorf("Разница Frontend = %q, want %q", got, want)
	}
	if got, want := diffRow[4], "994"; got != want {
		t.Errorf("Разница Итого = %q, want %q", got, want)
	}

	// Шапка листа: "Итоговая вместимость команды, чд:" — тоже округлённая
	// вниз сумма ролевых ёмкостей (та же 1000, что и в итоговой строке
	// «Доступно»), не сырое data.TotalCapacity=1000.7.
	if got, want := header[2][1], "1000"; got != want {
		t.Errorf("Итоговая вместимость команды = %q, want %q", got, want)
	}
}

// TestGenerateCapacityXLSX_TotalCapacityIgnoresUnassignedRoleMembers
// проверяет, что отображаемая в XLSX общая ёмкость команды считается как
// сумма округлённых вниз ролевых ёмкостей (data.RoleCapacities), а не берётся
// из «сырого» data.TotalCapacity (JSON-поле GET /reports/capacity, которое
// раньше считалось от полной численности команды, включая участников без
// назначенной оценивающей роли — см. design.md change
// simplify-capacity-report, Decision 5 и риск «Общая доступная ёмкость
// команды больше не включает участников без назначенной роли»).
//
// services.BuildCapacityReport не включает таких участников ни в одну
// RoleCapacityData (агрегация по roleMembersCount — см.
// internal/services/capacity_report.go), поэтому здесь моделируем это через
// заведомо расходящееся data.TotalCapacity (как если бы JSON учитывал ещё
// одного участника без роли) и проверяем, что рендерер XLSX его игнорирует.
func TestGenerateCapacityXLSX_TotalCapacityIgnoresUnassignedRoleMembers(t *testing.T) {
	data := CapacityReportResponse{
		TeamName: "Команда",
		Year:     2026,
		Quarter:  3,
		// «Сырое» значение искусственно завышено на величину ёмкости одного
		// «безролевого» участника (как раньше считал BuildCapacityReport от
		// len(users)) — рендерер XLSX не должен его использовать.
		TotalCapacity: 1040.2,
		RoleCapacities: []RoleCapacityData{
			{RoleName: "Backend", Capacity: 500.7, Planned: 0, Diff: 500.7},
		},
		Epics:  nil,
		Quotas: map[string]QuotaData{},
	}

	xlsxBytes, err := GenerateCapacityXLSX(data)
	if err != nil {
		t.Fatalf("не удалось сгенерировать xlsx: %v", err)
	}

	f, err := excelize.OpenReader(bytes.NewReader(xlsxBytes))
	if err != nil {
		t.Fatalf("не удалось открыть сгенерированный xlsx: %v", err)
	}
	defer f.Close()

	rows, err := f.GetRows("Вместимость")
	if err != nil {
		t.Fatalf("не удалось прочитать строки листа: %v", err)
	}

	// Шапка: сумма округлённых вниз ролевых ёмкостей = floor(500.7) = 500,
	// а не data.TotalCapacity = 1040.2 и не floor(1040.2) = 1040.
	if got, want := rows[2][1], "500"; got != want {
		t.Errorf("Итоговая вместимость команды = %q, want %q (сумма ролевых ёмкостей, не сырое TotalCapacity)", got, want)
	}
}
