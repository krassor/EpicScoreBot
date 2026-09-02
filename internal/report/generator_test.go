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
		TotalCapacity:       1000,
		TotalRoundedPlanned: 42,
		TotalDiff:           958,
		RoleCapacities: []roleCapacityTemplateData{
			{
				RoleCapacityData: RoleCapacityData{RoleName: "Backend", Capacity: 500.5, Planned: 300.2, Diff: 200.3},
				RoundedPlanned:   21,
				RoundedCapacity:  500,
				RoundedDiff:      479,
			},
			{
				RoleCapacityData: RoleCapacityData{RoleName: "Frontend", Capacity: 500.0, Planned: 505.7, Diff: -5.7},
				RoundedPlanned:   21,
				RoundedCapacity:  500,
				RoundedDiff:      479,
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
				RawTotalScore:     14.0,
				RiskLegend: []riskLegendItem{
					{Label: "Риск 1", Description: "Высокая нагрузка"},
				},
			},
			// epic-2 — риск-фактор заметно отличается от 1: сумма сырых
			// оценок по ролям (100.00) намного больше риск-скорректированной
			// FinalScore (60) — проверяем, что карточка показывает оба числа
			// раздельно, а не одно и то же значение трижды (см. change
			// fix-pdf-report).
			{
				EpicReportData: EpicReportData{
					EpicReportItem: EpicReportItem{
						ID:         "epic-2",
						Number:     "EP-2",
						Name:       "Эпик с риском",
						Type:       "feature",
						Status:     "scored",
						FinalScore: 60,
						RoleScores: map[string]float64{
							"Backend":  40,
							"Frontend": 20,
						},
						RawRoleScores: map[string]float64{
							"Backend":  70,
							"Frontend": 30,
						},
					},
				},
				RoundedRoleScores: map[string]int{"Backend": 40, "Frontend": 20},
				RoundedTotal:      60,
				RawTotalScore:     100.0,
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
	// «без рисков» теперь ожидаемая подпись строки суммы сырых оценок по
	// ролям в карточке эпика (см. change fix-pdf-report) — раньше эта
	// проверка ожидала обратное (устаревшая версия правки).
	if !strings.Contains(html, "без рисков") {
		t.Error("шаблон не содержит ожидаемую подпись «без рисков» (сумма сырых оценок)")
	}
	// Низ карточки эпика («Итоговая оценка с учётом рисков: …») не менялся
	// этим change — проверяем реальный текст (множественное число «рисков»,
	// не «риска»).
	if !strings.Contains(html, "с учётом рисков") {
		t.Error("шаблон не содержит ожидаемую подпись «с учётом рисков» (низ карточки эпика)")
	}
	if !strings.Contains(html, "Итого с рисками (чд)") {
		t.Error("шаблон не содержит колонку «Итого с рисками (чд)»")
	}

	// epic-2: сумма сырых оценок по ролям (100.00) должна присутствовать в
	// HTML отдельно от риск-скорректированного итога (60) — это подтверждает,
	// что карточка эпика реально показывает два разных числа, а не одно и то
	// же значение трижды (баг из proposal.md).
	if !strings.Contains(html, "100.00") {
		t.Error("шаблон не содержит сумму сырых оценок по ролям (100.00) для epic-2")
	}
	if !strings.Contains(html, "70.00") || !strings.Contains(html, "30.00") {
		t.Error("шаблон не содержит сырые оценки по ролям (70.00, 30.00) для epic-2 — вместо этого, похоже, используются риск-скорректированные RoleScores")
	}
	// Риск-скорректированные значения по ролям (40, 20) НЕ должны
	// присутствовать в виде "%.2f" в разбивке карточки — это признак того,
	// что таблица всё ещё читает RoleScores, а не RawRoleScores.
	if strings.Contains(html, "40.00") || strings.Contains(html, "20.00") {
		t.Error("шаблон содержит риск-скорректированные оценки по ролям (40.00/20.00) — карточка эпика должна показывать RawRoleScores, а не RoleScores")
	}

	// epic-1: FinalScore=15.5 при округлении "%.0f" (округление до чётного,
	// см. strconv/fmt) даёт "16", а RoundedTotal=17 — числа заведомо
	// расходятся. Это ловит регрессию change fix-pdf-epic-card-total: если
	// шапка/низ карточки эпика снова переключатся на .FinalScore, тест
	// увидит "16" вместо ожидаемого "17" (значение колонки «Итого с
	// рисками (чд)» главной матрицы для того же эпика, см. RoundedTotal:
	// 17 в fixture выше и $epic.RoundedTotal в шаблоне матрицы).
	if !strings.Contains(html, "Итого: 17") {
		t.Error("шапка карточки epic-1 не показывает RoundedTotal (17) — «Итого» должно совпадать с колонкой «Итого с рисками (чд)» главной матрицы")
	}
	if strings.Contains(html, "Итого: 16") {
		t.Error("шапка карточки epic-1 показывает округлённый FinalScore (16) вместо RoundedTotal (17) — регрессия: шаблон снова использует .FinalScore")
	}
	if !strings.Contains(html, "с учётом рисков: <strong>17</strong>") {
		t.Error("низ карточки epic-1 не показывает RoundedTotal (17) в «Итоговая оценка с учётом рисков»")
	}
	if strings.Contains(html, "с учётом рисков: <strong>16</strong>") {
		t.Error("низ карточки epic-1 показывает округлённый FinalScore (16) вместо RoundedTotal (17) — регрессия: шаблон снова использует .FinalScore")
	}
}
