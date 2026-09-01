package report

import (
	"bytes"
	"html/template"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// templateHTMLPath возвращает путь до config/template.html относительно
// корня репозитория — используется, чтобы протестировать реальный шаблон
// (используемый Gotenberg в проде), а не его копию.
func templateHTMLPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("не удалось определить путь текущего файла")
	}
	// internal/report/generator_test.go -> <repo root>/config/template.html
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	return filepath.Join(repoRoot, "config", "template.html")
}

// TestTemplateHTML_RendersWithoutError проверяет, что реальный
// config/template.html успешно парсится и рендерится с данными в форме,
// которую строит Generator.GenerateReport (templateData/epicTemplateData) —
// это защищает от рассинхронизации полей шаблона и Go-структур при
// рефакторинге (см. design.md change simplify-capacity-report).
func TestTemplateHTML_RendersWithoutError(t *testing.T) {
	tmpl, err := template.ParseFiles(templateHTMLPath(t))
	if err != nil {
		t.Fatalf("не удалось распарсить шаблон: %v", err)
	}

	td := templateData{
		TeamName:            "Тестовая команда",
		Year:                2026,
		Quarter:             3,
		TotalCapacity:       1000.5,
		TotalRoundedPlanned: 42,
		TotalDiff:           -5.5,
		RoleCapacities: []roleCapacityTemplateData{
			{
				RoleCapacityData: RoleCapacityData{RoleName: "Backend", Capacity: 500.5, Planned: 300.2, Diff: 200.3},
				RoundedPlanned:   21,
			},
			{
				RoleCapacityData: RoleCapacityData{RoleName: "Frontend", Capacity: 500.0, Planned: 505.7, Diff: -5.7},
				RoundedPlanned:   21,
			},
		},
		Quotas: map[string]QuotaData{
			"feature":           {LimitPercent: 40, ActualPercent: 35.5, Status: "OK"},
			"tech_architecture": {LimitPercent: 60, ActualPercent: 65.2, Status: "EXCEEDED"},
		},
		GeneratedFormatted: "01.09.2026 12:00",
		Epics: []epicTemplateData{
			{
				EpicReportData: EpicReportData{
					EpicReportItem: EpicReportItem{
						ID:         "epic-1",
						Number:     "EP-1",
						Name:       "Тестовый эпик",
						Type:       "feature",
						Status:     "scored",
						FinalScore: 15.5,
						RoleScores: map[string]float64{
							"Backend":  10.1,
							"Frontend": 5.4,
						},
						RawRoleScores: map[string]float64{
							"Backend":  9.0,
							"Frontend": 5.0,
						},
					},
					Risks: []RiskReportData{
						{
							Description:   "Высокая нагрузка",
							Probabilities: []int{2, 3},
							Impacts:       []int{3, 4},
							WeightedScore: 6.0,
							Coefficient:   1.05,
						},
					},
				},
				RoundedRoleScores: map[string]int{"Backend": 11, "Frontend": 6},
				RoundedTotal:      17,
				RiskLegend: []riskLegendItem{
					{Label: "Риск 1", Description: "Высокая нагрузка"},
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, td); err != nil {
		t.Fatalf("не удалось отрендерить шаблон: %v", err)
	}

	html := buf.String()

	if strings.Contains(html, "С рисками (чд)") {
		t.Error("шаблон всё ещё содержит устаревшую колонку «С рисками (чд)»")
	}
	if strings.Contains(html, "без рисков") {
		t.Error("шаблон всё ещё содержит некорректную подпись «без рисков»")
	}
	if !strings.Contains(html, "с учётом риска") {
		t.Error("шаблон не содержит ожидаемую подпись «с учётом риска»")
	}
	if !strings.Contains(html, "Итого (чд)") {
		t.Error("шаблон не содержит колонку «Итого (чд)»")
	}
}
