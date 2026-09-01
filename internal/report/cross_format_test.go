package report

import (
	"bytes"
	"fmt"
	"html/template"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

// crossFormatFixture строит одну общую CapacityReportResponse — используется
// как источник и для XLSX-выгрузки (GenerateCapacityXLSX), и (обёрнутая в
// ReportData, см. epicService.GetReportData) для PDF-HTML-рендера
// (buildReportTemplateData + config/template.html) — чтобы убедиться, что
// оба формата отображают одни и те же числа для одного и того же периода
// (см. openspec/changes/simplify-capacity-report/specs/capacity-report/spec.md,
// требование «Согласованность данных о трудоёмкости между PDF,
// веб-интерфейсом и XLSX»).
//
// Числа подобраны так, чтобы округление было нетривиальным в обе стороны:
//   - RoleScores эпиков — дробные, поячеечное округление вверх (ceil) даёт
//     разные целые для разных эпиков/ролей, ни одна ячейка не равна нулю
//     (иначе шаблон отрисовал бы «-» вместо «0», что не мешает бизнес-логике,
//     но усложнило бы сравнение чисел регулярками);
//   - Capacity ролей — дробная (40.224 × headcount), округление вниз (floor)
//     даёт остаток, не равный «сырому» TotalCapacity;
//   - «сырое» TotalCapacity намеренно расходится с суммой округлённых
//     ролевых ёмкостей — как если бы в команде был участник без назначенной
//     оценивающей роли (см. design.md, Decision 5) — оба рендерера обязаны
//     его игнорировать и в PDF, и в XLSX.
func crossFormatFixture() CapacityReportResponse {
	return CapacityReportResponse{
		TeamName: "Космонавты",
		Year:     2026,
		Quarter:  3,
		// «Сырое» значение искусственно расходится с суммой округлённых
		// ролевых ёмкостей (120 + 80 = 200) — ни PDF, ни XLSX не должны его
		// использовать напрямую (см. RoundRoleCapacities, Decision 5).
		TotalCapacity: 250.999,
		RoleCapacities: []RoleCapacityData{
			{RoleName: "Backend", Capacity: 40.224 * 3, Planned: 11, Diff: 40.224*3 - 11},  // 120.672 -> floor 120
			{RoleName: "Frontend", Capacity: 40.224 * 2, Planned: 11, Diff: 40.224*2 - 11}, // 80.448 -> floor 80
		},
		Epics: []EpicReportItem{
			{
				ID:            "epic-1",
				Number:        "EP-1",
				Name:          "Личный кабинет пользователя",
				Type:          "feature",
				Status:        "scored",
				FinalScore:    9.5,
				RoleScores:    map[string]float64{"Backend": 4.2, "Frontend": 2.05}, // ceil: 5, 3
				RawRoleScores: map[string]float64{"Backend": 3.8, "Frontend": 1.9},
			},
			{
				ID:            "epic-2",
				Number:        "EP-2",
				Name:          "Интеграция с внешним API",
				Type:          "techdebt",
				Status:        "scored",
				FinalScore:    11.2,
				RoleScores:    map[string]float64{"Backend": 1.01, "Frontend": 6.99}, // ceil: 2, 7
				RawRoleScores: map[string]float64{"Backend": 1.0, "Frontend": 6.5},
			},
			{
				ID:            "epic-3",
				Number:        "EP-3",
				Name:          "Миграция схемы БД",
				Type:          "tech_architecture",
				Status:        "scored",
				FinalScore:    6.0,
				RoleScores:    map[string]float64{"Backend": 3.5, "Frontend": 0.1}, // ceil: 4, 1
				RawRoleScores: map[string]float64{"Backend": 3.0, "Frontend": 0.1},
			},
		},
		Quotas: map[string]QuotaData{
			"feature":           {LimitPercent: 40, ActualPercent: 25.5, Status: "OK"},
			"tech_architecture": {LimitPercent: 60, ActualPercent: 74.5, Status: "EXCEEDED"},
		},
	}
}

// findTableRow возвращает подстроку HTML между ближайшим предшествующим
// маркеру <tr и ближайшим следующим за маркером </tr> — используется, чтобы
// вытащить содержимое конкретной строки таблицы отчёта по её текстовой
// подписи (например, «Запланировано (трудоемкость), чд») или по номеру
// эпика (например, «#EP-1»).
func findTableRow(t *testing.T, html, marker string) string {
	t.Helper()

	idx := strings.Index(html, marker)
	if idx == -1 {
		t.Fatalf("маркер %q не найден в отрендеренном HTML отчёта", marker)
	}

	trStart := strings.LastIndex(html[:idx], "<tr")
	if trStart == -1 {
		t.Fatalf("не найден открывающий <tr> перед маркером %q", marker)
	}

	relEnd := strings.Index(html[idx:], "</tr>")
	if relEnd == -1 {
		t.Fatalf("не найден закрывающий </tr> после маркера %q", marker)
	}
	trEnd := idx + relEnd + len("</tr>")

	return html[trStart:trEnd]
}

// tdIntsRe матчит содержимое ячеек <td>, состоящее из одного целого числа
// (с необязательным минусом) — ячейки с вложенной разметкой (например,
// <span> с типом задачи) или текстовыми подписями («Запланировано…») не
// матчатся и корректно пропускаются.
var tdIntsRe = regexp.MustCompile(`<td[^>]*>\s*(-?\d+)\s*</td>`)

// extractRowInts вытаскивает из HTML-строки таблицы все целочисленные
// значения ячеек <td> по порядку появления.
func extractRowInts(row string) []int {
	matches := tdIntsRe.FindAllStringSubmatch(row, -1)
	result := make([]int, 0, len(matches))
	for _, m := range matches {
		n, _ := strconv.Atoi(m[1])
		result = append(result, n)
	}
	return result
}

// assertIntsEqual сравнивает числа, полученные из XLSX-ячеек, и числа,
// найденные в PDF-HTML, для одной именованной строки отчёта (например,
// строки эпика или итоговой строки «Доступно») — при расхождении явно
// указывает, какие именно позиции (роли) разошлись.
func assertIntsEqual(t *testing.T, rowLabel string, roleNames []string, xlsxVals, htmlVals []int) {
	t.Helper()

	if len(xlsxVals) != len(htmlVals) {
		t.Fatalf("%s: разное число числовых ячеек — XLSX=%v (%d), PDF-HTML=%v (%d)",
			rowLabel, xlsxVals, len(xlsxVals), htmlVals, len(htmlVals))
	}

	labels := append(append([]string{}, roleNames...), "Итого")
	for i := range xlsxVals {
		label := "?"
		if i < len(labels) {
			label = labels[i]
		}
		if xlsxVals[i] != htmlVals[i] {
			t.Errorf("%s [%s]: XLSX=%d, PDF-HTML=%d — числа разошлись между форматами",
				rowLabel, label, xlsxVals[i], htmlVals[i])
		}
	}
}

// TestCapacityReport_XLSXAndPDFHTMLMatchForSamePeriod — сквозной тест
// числовой согласованности отчёта о вместимости команды между XLSX-выгрузкой
// (GenerateCapacityXLSX) и HTML, из которого Gotenberg рендерит PDF
// (buildReportTemplateData + реальный config/template.html), для одной и
// той же общей фикстуры CapacityReportResponse (см. crossFormatFixture) —
// закрывает требование «Согласованность данных о трудоёмкости между PDF,
// веб-интерфейсом и XLSX» (openspec/changes/simplify-capacity-report/specs/
// capacity-report/spec.md) в той части, которая не требует живого Gotenberg:
// сам HTML, который Gotenberg превращает в PDF байт-в-байт без дальнейших
// вычислений, здесь строится идентичным продакшену способом (см.
// GenerateReport), различается только источник байтов на выходе (файл на
// диске в проде vs. буфер в тесте).
//
// Проверяются: шапка «Итоговая вместимость команды»/«Общая доступная емкость
// команды» (XLSX и PDF соответственно), строки трёх эпиков матрицы
// «эпик × роль», итоговые строки «Запланировано»/«Доступно»/«Разница» — и
// заголовок колонки «Итого с рисками (чд)» дословно в обоих форматах.
func TestCapacityReport_XLSXAndPDFHTMLMatchForSamePeriod(t *testing.T) {
	fixture := crossFormatFixture()

	// ── XLSX-путь ────────────────────────────────────────────────────────
	xlsxBytes, err := GenerateCapacityXLSX(fixture)
	if err != nil {
		t.Fatalf("не удалось сгенерировать xlsx: %v", err)
	}

	xf, err := excelize.OpenReader(bytes.NewReader(xlsxBytes))
	if err != nil {
		t.Fatalf("не удалось открыть сгенерированный xlsx: %v", err)
	}
	defer xf.Close()

	rows, err := xf.GetRows("Вместимость")
	if err != nil {
		t.Fatalf("не удалось прочитать строки листа «Вместимость»: %v", err)
	}
	// Раскладка листа (см. writeCapacitySheet):
	//   idx0 "Команда:"                 idx1 "Период:"
	//   idx2 "Итоговая вместимость команды, чд:"   <- значение в rows[2][1]
	//   idx3 пусто
	//   idx4 заголовок матрицы: Задача, Тип, <роли...>, "Итого с рисками (чд)"
	//   idx5..idx7 — 3 строки эпиков (EP-1, EP-2, EP-3)
	//   idx8 "Запланировано (трудоемкость), чд"
	//   idx9 "Доступно (емкость), чд"
	//   idx10 "Разница (недо-/перепланирование), чд"
	if len(rows) < 11 {
		t.Fatalf("неожиданно мало строк на листе «Вместимость»: %d, %v", len(rows), rows)
	}

	xlsxTotalCapacityHeader, err := strconv.Atoi(rows[2][1])
	if err != nil {
		t.Fatalf("не удалось разобрать «Итоговая вместимость команды» из xlsx: %v (значение %q)", err, rows[2][1])
	}

	headerRow := rows[4]
	wantHeader := []string{"Задача", "Тип", "Backend", "Frontend", "Итого с рисками (чд)"}
	if len(headerRow) != len(wantHeader) {
		t.Fatalf("неверное число колонок в заголовке xlsx: %v, ожидалось: %v", headerRow, wantHeader)
	}
	for i, w := range wantHeader {
		if headerRow[i] != w {
			t.Errorf("заголовок xlsx[%d] = %q, want %q", i, headerRow[i], w)
		}
	}

	toInts := func(t *testing.T, row []string, from int) []int {
		t.Helper()
		vals := make([]int, 0, len(row)-from)
		for _, s := range row[from:] {
			n, err := strconv.Atoi(s)
			if err != nil {
				t.Fatalf("не удалось разобрать число %q в строке xlsx %v: %v", s, row, err)
			}
			vals = append(vals, n)
		}
		return vals
	}

	xlsxEP1 := toInts(t, rows[5], 2)
	xlsxEP2 := toInts(t, rows[6], 2)
	xlsxEP3 := toInts(t, rows[7], 2)
	xlsxPlanned := toInts(t, rows[8], 2)
	xlsxCapacity := toInts(t, rows[9], 2)
	xlsxDiff := toInts(t, rows[10], 2)

	// ── PDF-HTML путь (без Gotenberg — только рендер html/template) ──────
	reportData := ReportData{
		CapacityReportResponse: fixture,
		Epics: []EpicReportData{
			{EpicReportItem: fixture.Epics[0]},
			{EpicReportItem: fixture.Epics[1]},
			{EpicReportItem: fixture.Epics[2]},
		},
		Generated: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
	}
	td := buildReportTemplateData(reportData)

	tmpl, err := template.ParseFiles(templateHTMLPath(t))
	if err != nil {
		t.Fatalf("не удалось распарсить реальный config/template.html: %v", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, td); err != nil {
		t.Fatalf("не удалось отрендерить config/template.html: %v", err)
	}
	html := buf.String()

	if !strings.Contains(html, "Итого с рисками (чд)") {
		t.Error("PDF-HTML не содержит колонку «Итого с рисками (чд)» — заголовок должен дословно совпадать с XLSX")
	}

	totalCapRe := regexp.MustCompile(`Общая доступная емкость команды:\s*<strong>\s*(-?\d+)\s*чд\s*</strong>`)
	m := totalCapRe.FindStringSubmatch(html)
	if m == nil {
		t.Fatalf("не удалось найти «Общая доступная емкость команды» в PDF-HTML")
	}
	pdfTotalCapacityHeader, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("не удалось разобрать «Общая доступная емкость команды» из PDF-HTML: %v", err)
	}

	pdfEP1 := extractRowInts(findTableRow(t, html, "#EP-1"))
	pdfEP2 := extractRowInts(findTableRow(t, html, "#EP-2"))
	pdfEP3 := extractRowInts(findTableRow(t, html, "#EP-3"))
	pdfPlanned := extractRowInts(findTableRow(t, html, "Запланировано (трудоемкость), чд"))
	pdfCapacity := extractRowInts(findTableRow(t, html, "Доступно (емкость), чд"))
	pdfDiff := extractRowInts(findTableRow(t, html, "Разница (недо-/перепланирование), чд"))

	// ── Сверка чисел между XLSX и PDF-HTML ────────────────────────────────
	roleNames := []string{"Backend", "Frontend"}

	if xlsxTotalCapacityHeader != pdfTotalCapacityHeader {
		t.Errorf("«Итоговая вместимость команды»: XLSX=%d, PDF-HTML=%d — числа разошлись",
			xlsxTotalCapacityHeader, pdfTotalCapacityHeader)
	}
	// Обе шапки обязаны игнорировать «сырое» CapacityReportResponse.TotalCapacity
	// (250.999) — иначе тест ниже не выявил бы регрессию, будь оба числа
	// случайно равны сырому значению.
	if xlsxTotalCapacityHeader == int(fixture.TotalCapacity) || pdfTotalCapacityHeader == int(fixture.TotalCapacity) {
		t.Fatalf("шапка отчёта использует сырое TotalCapacity=%v вместо суммы округлённых ролевых ёмкостей", fixture.TotalCapacity)
	}

	assertIntsEqual(t, "EP-1", roleNames, xlsxEP1, pdfEP1)
	assertIntsEqual(t, "EP-2", roleNames, xlsxEP2, pdfEP2)
	assertIntsEqual(t, "EP-3", roleNames, xlsxEP3, pdfEP3)
	assertIntsEqual(t, "Запланировано", roleNames, xlsxPlanned, pdfPlanned)
	assertIntsEqual(t, "Доступно", roleNames, xlsxCapacity, pdfCapacity)
	assertIntsEqual(t, "Разница", roleNames, xlsxDiff, pdfDiff)

	// Явные ожидаемые значения (не только «xlsx == pdf», но и «оба равны
	// заранее посчитанному вручную результату округления») — страхует от
	// случая, когда оба рендерера синхронно сломаны одинаковым образом.
	want := map[string][]int{
		"EP-1":          {5, 3, 8},
		"EP-2":          {2, 7, 9},
		"EP-3":          {4, 1, 5},
		"Запланировано": {11, 11, 22},
		"Доступно":      {120, 80, 200},
		"Разница":       {109, 69, 178},
	}
	got := map[string][]int{
		"EP-1":          xlsxEP1,
		"EP-2":          xlsxEP2,
		"EP-3":          xlsxEP3,
		"Запланировано": xlsxPlanned,
		"Доступно":      xlsxCapacity,
		"Разница":       xlsxDiff,
	}
	for label, w := range want {
		g := got[label]
		if fmt.Sprint(g) != fmt.Sprint(w) {
			t.Errorf("%s: получено %v, ожидалось (вручную посчитанное) %v", label, g, w)
		}
	}
	if xlsxTotalCapacityHeader != 200 {
		t.Errorf("«Итоговая вместимость команды» = %d, ожидалось 200 (floor(120.672)+floor(80.448))", xlsxTotalCapacityHeader)
	}
}
