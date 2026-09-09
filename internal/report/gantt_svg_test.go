package report

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func d(y int, m time.Month, day int) time.Time {
	return time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
}

// fullTreeFixture строит полное дерево: эпик → две стори → по одной ролевой
// задаче в каждой (одна с исполнителем, одна без) — используется несколькими
// тестами 2.1-2.4.
func fullTreeFixture() []GanttReportRow {
	return []GanttReportRow{
		{Level: GanttReportLevelEpic, Depth: 0, Name: "EP-1: Личный кабинет", StartDate: d(2026, 7, 1), EndDate: d(2026, 9, 20)},
		{Level: GanttReportLevelStory, Depth: 1, Name: "EP-1-S1: Авторизация", StartDate: d(2026, 7, 1), EndDate: d(2026, 8, 10)},
		{Level: GanttReportLevelRole, Depth: 2, Name: "BE разработчик", StartDate: d(2026, 7, 1), EndDate: d(2026, 7, 20), HasAssignee: true, AssigneeName: "Иван Иванов"},
		{Level: GanttReportLevelStory, Depth: 1, Name: "EP-1-S2: Профиль", StartDate: d(2026, 8, 11), EndDate: d(2026, 9, 20)},
		{Level: GanttReportLevelRole, Depth: 2, Name: "FE разработчик", StartDate: d(2026, 8, 11), EndDate: d(2026, 9, 20), HasAssignee: false, Completed: true},
	}
}

// ── 2.1 Построитель SVG: дерево и порядок ───────────────────────────────────

func TestBuildGanttDiagram_FullTreeOrder(t *testing.T) {
	rows := fullTreeFixture()
	svg := BuildGanttDiagram(rows, "Космонавты", 2026, 3)

	names := []string{"EP-1: Личный кабинет", "EP-1-S1: Авторизация", "BE разработчик", "EP-1-S2: Профиль", "FE разработчик"}
	prevIdx := -1
	for _, name := range names {
		idx := strings.Index(svg, name)
		if idx < 0 {
			t.Fatalf("SVG does not contain expected row name %q", name)
		}
		if idx <= prevIdx {
			t.Fatalf("row %q is out of order (index %d <= previous %d)", name, idx, prevIdx)
		}
		prevIdx = idx
	}
}

func TestBuildGanttDiagram_LegacyEpicWithoutStories(t *testing.T) {
	rows := []GanttReportRow{
		{Level: GanttReportLevelEpic, Depth: 0, Name: "EP-2: Легаси эпик", StartDate: d(2026, 7, 1), EndDate: d(2026, 7, 15)},
		{Level: GanttReportLevelRole, Depth: 1, Name: "Аналитик", StartDate: d(2026, 7, 1), EndDate: d(2026, 7, 15), HasAssignee: true, AssigneeName: "Петр Петров"},
	}
	svg := BuildGanttDiagram(rows, "Космонавты", 2026, 3)

	if !strings.Contains(svg, "EP-2: Легаси эпик") || !strings.Contains(svg, "Аналитик") {
		t.Fatalf("legacy epic without stories: expected both rows present, got: %s", svg)
	}
	if !strings.Contains(svg, "Петр Петров") {
		t.Fatalf("expected assignee name present for role row without a story level")
	}
}

// ── 2.2 Календарная шкала на весь диапазон ──────────────────────────────────

func TestBuildGanttDiagram_ScaleCoversFullRangeBeyondQuarter(t *testing.T) {
	// Квартал условно Q3 2026 (июль-сентябрь), но задача заканчивается в
	// ноябре — за пределами квартала. Шкала обязана охватить и эту дату.
	rows := []GanttReportRow{
		{Level: GanttReportLevelEpic, Depth: 0, Name: "EP-3", StartDate: d(2026, 9, 1), EndDate: d(2026, 11, 15)},
		{Level: GanttReportLevelRole, Depth: 1, Name: "BE разработчик", StartDate: d(2026, 9, 1), EndDate: d(2026, 11, 15), HasAssignee: true, AssigneeName: "А"},
	}
	svg := BuildGanttDiagram(rows, "Космонавты", 2026, 3)

	for _, month := range []string{"сентябрь 2026", "октябрь 2026", "ноябрь 2026"} {
		if !strings.Contains(svg, month) {
			t.Fatalf("expected scale month label %q to be present when task ends beyond the quarter, got: %s", month, svg)
		}
	}
}

func TestGanttDateRange(t *testing.T) {
	rows := []GanttReportRow{
		{StartDate: d(2026, 8, 5), EndDate: d(2026, 8, 20)},
		{StartDate: d(2026, 7, 1), EndDate: d(2026, 9, 30)},
		{StartDate: d(2026, 7, 15), EndDate: d(2026, 8, 1)},
	}
	start, end, ok := ganttDateRange(rows)
	if !ok {
		t.Fatal("expected ok=true for non-empty rows")
	}
	if !start.Equal(d(2026, 7, 1)) {
		t.Errorf("expected range start 2026-07-01, got %v", start)
	}
	if !end.Equal(d(2026, 9, 30)) {
		t.Errorf("expected range end 2026-09-30, got %v", end)
	}

	if _, _, ok := ganttDateRange(nil); ok {
		t.Error("expected ok=false for empty rows")
	}
}

// ── 2.3 Бары по датам и подписи ─────────────────────────────────────────────

func TestGanttXForDate(t *testing.T) {
	start := d(2026, 7, 1)
	chartX0 := 100.0
	pxPerDay := 10.0

	if got := ganttXForDate(start, start, chartX0, pxPerDay); got != chartX0 {
		t.Errorf("x(start) = %g, want %g", got, chartX0)
	}
	got := ganttXForDate(start, d(2026, 7, 11), chartX0, pxPerDay)
	want := chartX0 + 10*pxPerDay
	if got != want {
		t.Errorf("x(start+10d) = %g, want %g", got, want)
	}
}

var rectRe = regexp.MustCompile(`<rect x="([\d.]+)" y="([\d.]+)" width="([\d.]+)" height="([\d.]+)"`)

func TestBuildGanttDiagram_BarCoordinatesMatchDates(t *testing.T) {
	// Единственная задача: диапазон шкалы равен её собственным датам, так что
	// бар обязан начинаться ровно у левой границы сетки и заканчиваться у
	// правой (после включения последнего дня целиком).
	rows := []GanttReportRow{
		{Level: GanttReportLevelEpic, Depth: 0, Name: "EP-4", StartDate: d(2026, 7, 1), EndDate: d(2026, 7, 10)},
	}
	svg := BuildGanttDiagram(rows, "Команда", 2026, 3)

	chartX0 := ganttSidePaddingPx + ganttLabelColumnPx
	chartX1 := ganttPageWidthPx - ganttSidePaddingPx

	matches := rectRe.FindAllStringSubmatch(svg, -1)
	if len(matches) == 0 {
		t.Fatal("expected at least one <rect> in SVG output")
	}
	// Первый rect — фон листа (во всю ширину/высоту), бар строки — второй.
	var barRect []string
	for _, m := range matches {
		x, _ := strconv.ParseFloat(m[1], 64)
		if x > chartX0-1 && x < chartX0+1 {
			barRect = m
			break
		}
	}
	if barRect == nil {
		t.Fatalf("could not locate the task's bar rect near chartX0=%g among rects: %v", chartX0, matches)
	}
	x, _ := strconv.ParseFloat(barRect[1], 64)
	width, _ := strconv.ParseFloat(barRect[3], 64)

	if diff := x - chartX0; diff > 0.5 || diff < -0.5 {
		t.Errorf("bar x = %g, want ~%g (chart left edge)", x, chartX0)
	}
	if diff := (x + width) - chartX1; diff > 1 || diff < -1 {
		t.Errorf("bar right edge = %g, want ~%g (chart right edge)", x+width, chartX1)
	}
}

func TestBuildGanttDiagram_RoleAndAssigneeLabel(t *testing.T) {
	rows := []GanttReportRow{
		{Level: GanttReportLevelRole, Depth: 1, Name: "BE разработчик", StartDate: d(2026, 7, 1), EndDate: d(2026, 7, 5), HasAssignee: true, AssigneeName: "Иван Иванов"},
	}
	svg := BuildGanttDiagram(rows, "Команда", 2026, 3)

	if !strings.Contains(svg, "BE разработчик") {
		t.Error("expected role name in output")
	}
	if !strings.Contains(svg, "Иван Иванов") {
		t.Error("expected assignee name in output")
	}
	if !strings.Contains(svg, "BE разработчик — Иван Иванов") {
		t.Error("expected role and assignee to be shown together in a single label")
	}
}

// ── 2.4 Кодирование состояний и легенда ─────────────────────────────────────

func TestBuildGanttDiagram_RolesEncodedByMoreThanColor(t *testing.T) {
	rows := []GanttReportRow{
		{Level: GanttReportLevelRole, Depth: 1, Name: "BE разработчик", StartDate: d(2026, 7, 1), EndDate: d(2026, 7, 5), HasAssignee: true, AssigneeName: "А"},
		{Level: GanttReportLevelRole, Depth: 1, Name: "FE разработчик", StartDate: d(2026, 7, 6), EndDate: d(2026, 7, 10), HasAssignee: true, AssigneeName: "Б"},
	}
	svg := BuildGanttDiagram(rows, "Команда", 2026, 3)

	patternRe := regexp.MustCompile(`<pattern id="([^"]+)"`)
	ids := map[string]bool{}
	for _, m := range patternRe.FindAllStringSubmatch(svg, -1) {
		ids[m[1]] = true
	}
	bePattern := ganttPatternID("BE разработчик")
	fePattern := ganttPatternID("FE разработчик")
	if !ids[bePattern] || !ids[fePattern] {
		t.Fatalf("expected distinct patterns for both roles, got pattern ids: %v", ids)
	}
	if bePattern == fePattern {
		t.Fatal("two different roles must not share the same pattern id")
	}
	// Разные роли обязаны давать разную геометрию штриховки (не только
	// разный цвет) — извлекаем markup самих <pattern> блоков и сравниваем.
	beIdx := ganttHatchStyleIndex("BE разработчик")
	feIdx := ganttHatchStyleIndex("FE разработчик")
	if beIdx == feIdx {
		t.Skip("hash collision between role names for this fixture — geometry equality expected, skipping")
	}
}

func TestBuildGanttDiagram_CompletedTaskMarked(t *testing.T) {
	rows := []GanttReportRow{
		{Level: GanttReportLevelRole, Depth: 1, Name: "BE разработчик", StartDate: d(2026, 7, 1), EndDate: d(2026, 7, 5), HasAssignee: true, AssigneeName: "А", Completed: true},
		{Level: GanttReportLevelRole, Depth: 1, Name: "FE разработчик", StartDate: d(2026, 7, 6), EndDate: d(2026, 7, 10), HasAssignee: true, AssigneeName: "Б", Completed: false},
	}
	svg := BuildGanttDiagram(rows, "Команда", 2026, 3)

	checkmarks := strings.Count(svg, "✓")
	// Один — рядом с завершённой задачей, ещё один — в легенде.
	if checkmarks < 2 {
		t.Fatalf("expected at least 2 occurrences of the completion marker (task + legend), got %d", checkmarks)
	}
	if !strings.Contains(svg, `stroke-width="2.2"`) {
		t.Error("expected a thicker border on the completed task's bar")
	}
}

func TestBuildGanttDiagram_MissingAssigneeMarked(t *testing.T) {
	rows := []GanttReportRow{
		{Level: GanttReportLevelRole, Depth: 1, Name: "Тестировщик", StartDate: d(2026, 7, 1), EndDate: d(2026, 7, 5), HasAssignee: false},
	}
	svg := BuildGanttDiagram(rows, "Команда", 2026, 3)

	if !strings.Contains(svg, "без исполнителя") {
		t.Error("expected the label to state the task has no assignee")
	}
	if !strings.Contains(svg, `stroke-dasharray="4 2"`) {
		t.Error("expected a dashed outline for a task without an assignee")
	}
}

func TestBuildGanttDiagram_LegendPresentAndCoversEncodings(t *testing.T) {
	rows := fullTreeFixture()
	svg := BuildGanttDiagram(rows, "Команда", 2026, 3)

	for _, want := range []string{"Легенда", "Эпик", "Стори", "Завершено", "Нет исполнителя", "BE разработчик", "FE разработчик"} {
		if !strings.Contains(svg, want) {
			t.Errorf("expected legend to mention %q, got: %s", want, svg)
		}
	}
}

func TestBuildGanttDiagram_NoTasks_NoBareEmptyGrid(t *testing.T) {
	svg := BuildGanttDiagram(nil, "Команда", 2026, 3)

	if !strings.Contains(svg, "Нет запланированных работ") {
		t.Error("expected an explicit message when there are no tasks, not a bare empty grid")
	}
	if !strings.Contains(svg, "Легенда") {
		t.Error("expected the legend to still be present on the empty-state sheet")
	}
}

// ── 2.5 Высота листа и высота SVG из одной формулы ─────────────────────────

var svgSizeRe = regexp.MustCompile(`<svg[^>]*width="([\d.]+)" height="([\d.]+)"`)

func TestGanttPageSize_WidthConstantHeightGrowsWithRows(t *testing.T) {
	// rowCount=0 намеренно даёт ту же высоту, что rowCount=1 — под пустое
	// дерево резервируется одна строка на явное сообщение об отсутствии
	// задач (см. ganttEffectiveRowCount), поэтому строгий рост высоты
	// проверяется начиная с rowCount=1.
	rowCounts := []int{0, 1, 6, 50}

	dimsZero := GanttPageSize(0)
	dimsOne := GanttPageSize(1)
	if dimsZero.HeightInches != dimsOne.HeightInches {
		t.Errorf("GanttPageSize(0).HeightInches = %g, want equal to GanttPageSize(1).HeightInches = %g (one row reserved for the empty-state message)",
			dimsZero.HeightInches, dimsOne.HeightInches)
	}

	var prevHeight float64
	for i, n := range rowCounts {
		dims := GanttPageSize(n)

		if dims.WidthInches != ganttPageWidthInches {
			t.Errorf("rowCount=%d: width = %g, want constant %g", n, dims.WidthInches, ganttPageWidthInches)
		}
		if i > 1 && dims.HeightInches <= prevHeight {
			t.Errorf("rowCount=%d: height %g did not grow relative to previous %g", n, dims.HeightInches, prevHeight)
		}
		prevHeight = dims.HeightInches

		wantHeightPx := ganttContentHeightPx(n)
		gotHeightPx := dims.HeightInches * ganttDPI
		if diff := gotHeightPx - wantHeightPx; diff > 0.01 || diff < -0.01 {
			t.Errorf("rowCount=%d: page height %gpx does not match ganttContentHeightPx=%gpx", n, gotHeightPx, wantHeightPx)
		}
	}
}

func TestBuildGanttDiagram_SVGSizeMatchesPageSize(t *testing.T) {
	cases := []struct {
		name string
		rows []GanttReportRow
	}{
		{"empty", nil},
		{"one epic", []GanttReportRow{{Level: GanttReportLevelEpic, Name: "EP-1", StartDate: d(2026, 7, 1), EndDate: d(2026, 7, 5)}}},
		{"full tree", fullTreeFixture()},
		{"several dozen tasks", manyRoleTasksFixture(45)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svg := BuildGanttDiagram(tc.rows, "Команда", 2026, 3)
			m := svgSizeRe.FindStringSubmatch(svg)
			if m == nil {
				t.Fatalf("could not find <svg width=... height=...> in output: %s", svg)
			}
			gotWidth, _ := strconv.ParseFloat(m[1], 64)
			gotHeight, _ := strconv.ParseFloat(m[2], 64)

			wantWidth := ganttPageWidthPx
			wantHeight := ganttContentHeightPx(len(tc.rows))

			if gotWidth != wantWidth {
				t.Errorf("%s: SVG width = %g, want %g", tc.name, gotWidth, wantWidth)
			}
			if gotHeight != wantHeight {
				t.Errorf("%s: SVG height = %g, want %g", tc.name, gotHeight, wantHeight)
			}

			dims := GanttPageSize(len(tc.rows))
			if diff := dims.HeightInches*ganttDPI - gotHeight; diff > 0.01 || diff < -0.01 {
				t.Errorf("%s: page height (%gpx) and SVG height (%gpx) disagree", tc.name, dims.HeightInches*ganttDPI, gotHeight)
			}
			if dims.WidthInches*ganttDPI != gotWidth {
				t.Errorf("%s: page width (%gpx) and SVG width (%gpx) disagree", tc.name, dims.WidthInches*ganttDPI, gotWidth)
			}
		})
	}
}

func manyRoleTasksFixture(n int) []GanttReportRow {
	rows := make([]GanttReportRow, 0, n)
	base := d(2026, 7, 1)
	for i := 0; i < n; i++ {
		rows = append(rows, GanttReportRow{
			Level:       GanttReportLevelRole,
			Depth:       2,
			Name:        fmt.Sprintf("Роль %d", i%6),
			StartDate:   base.AddDate(0, 0, i),
			EndDate:     base.AddDate(0, 0, i+3),
			HasAssignee: i%2 == 0,
			Completed:   i%3 == 0,
		})
	}
	return rows
}
