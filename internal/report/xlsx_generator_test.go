package report

import (
	"bytes"
	"testing"

	"github.com/xuri/excelize/v2"
)

// TestGenerateCapacityXLSX_CapacitySheetUsesRoundedRiskAdjustedScores
// проверяет, что лист «Вместимость» показывает только округлённую вверх
// риск-скорректированную трудоёмкость (role_scores), одну колонку «Итого
// (чд)» на эпик (без «С рисками (чд)»), а «Запланировано» по роли — сумму
// уже округлённых ячеек (см. design.md change simplify-capacity-report).
func TestGenerateCapacityXLSX_CapacitySheetUsesRoundedRiskAdjustedScores(t *testing.T) {
	data := CapacityReportResponse{
		TeamName:      "Команда",
		Year:          2026,
		Quarter:       3,
		TotalCapacity: 1000,
		RoleCapacities: []RoleCapacityData{
			{RoleName: "Backend", Capacity: 500, Planned: 3, Diff: 497},
			{RoleName: "Frontend", Capacity: 500, Planned: 3.4, Diff: 496.6},
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

	// Заголовок матрицы — 5-я строка (индекс 4): "Задача","Тип","Backend","Frontend","Итого (чд)".
	headerRow := header[4]
	wantHeader := []string{"Задача", "Тип", "Backend", "Frontend", "Итого (чд)"}
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
}
