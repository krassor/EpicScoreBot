package report

import (
	"fmt"
	"hash/fnv"
	"html"
	"sort"
	"strings"
	"time"
)

// Лист диаграммы Ганта в PDF-отчёте — openspec/changes/add-gantt-page-to-pdf-report.
// Построитель тем же приёмом, что диаграммы рисков (см. BuildRiskProbabilityDiagram
// и соседей выше в svg.go): собирает строку SVG, которая кладётся в шаблон
// как template.HTML на стороне вызывающего кода (см. generator.go).

// ── Геометрия листа (design.md Решение 2) ──────────────────────────────────
//
// Высота листа и высота SVG диаграммы ОБЯЗАНЫ выводиться из одного числа
// строк по одной формуле в одном месте — см. ganttContentHeightPx, которую
// использует и GanttPageSize (размер страницы для Gotenberg PaperSize в
// секции 3), и BuildGanttDiagram (атрибуты width/height и viewBox самого
// SVG). Расхождение этих двух мест дало бы обрезанную снизу диаграмму или
// пустое поле (design.md Risks).
const (
	// ganttDPI — условное разрешение перевода CSS-пикселей в дюймы. Chromium
	// внутри Gotenberg рендерит HTML/SVG из расчёта 96 CSS-пикселей на дюйм
	// — то же соотношение, что и у готовых PaperSizeA4()/PaperSizeLetter()
	// клиента gotenberg (см. design.md Context/Решение 1), поэтому размер
	// страницы (в дюймах) и размер SVG (в пикселях) остаются согласованы.
	ganttDPI = 96.0

	// ganttPageWidthMM — ширина листа A3 в альбомной ориентации (ISO 216).
	// Единственная величина, задающая ширину листа диаграммы — она
	// постоянна и не зависит от числа строк или длительности работ
	// (Requirement «Размер листа подстраивается под число строк»).
	ganttPageWidthMM = 420.0
	// ganttPageWidthInches — та же ширина в дюймах (единицы
	// gotenberg.Chromium.PaperSize, секция 3).
	ganttPageWidthInches = ganttPageWidthMM / 25.4
	// ganttPageWidthPx — та же ширина в CSS-пикселях SVG, выведена из
	// ganttPageWidthInches по ganttDPI, чтобы избежать двух независимых
	// констант ширины, которые могли бы разойтись.
	ganttPageWidthPx = ganttPageWidthInches * ganttDPI

	// ganttRowHeightPx — фиксированная читаемая высота одной строки дерева
	// (эпик/стори/ролевая задача). Лист растёт по числу строк; уменьшение
	// этой величины ради того, чтобы уместиться в фиксированную высоту,
	// запрещено требованием (tasks.md §2.5).
	ganttRowHeightPx = 28.0
	// ganttTitleHeightPx — блок заголовка листа: команда, период, подпись
	// диаграммы (Requirement «Лист SHALL быть подписан...»).
	ganttTitleHeightPx = 40.0
	// ganttHeaderHeightPx — высота шапки с календарной шкалой (полоса
	// месяцев + полоса недель, см. buildGanttScale).
	ganttHeaderHeightPx = 56.0
	// ganttLegendHeightPx — блок легенды кодирования состояний (design.md
	// Решение 6), фиксированной высоты независимо от числа строк.
	ganttLegendHeightPx = 96.0
	// ganttTopPaddingPx/ganttBottomPaddingPx — внешние отступы листа сверху
	// и снизу.
	ganttTopPaddingPx    = 16.0
	ganttBottomPaddingPx = 16.0
	// ganttSidePaddingPx — внешний отступ листа слева/справа.
	ganttSidePaddingPx = 16.0

	// ganttLabelColumnPx — ширина левой колонки подписей строк (имя
	// эпика/стори/роли и исполнителя), расположенной левее календарной
	// сетки с барами.
	ganttLabelColumnPx = 380.0
	// ganttIndentPx — отступ на один уровень вложенности дерева
	// (GanttReportRow.Depth).
	ganttIndentPx = 18.0

	// ganttFixedChromePx — часть высоты листа, не зависящая от числа строк:
	// внешние отступы, заголовок, шапка со шкалой и легенда. Ровно то, что
	// design.md Решение 2 называет «отступы + шапка со шкалой» (легенда и
	// заголовок листа — тоже фиксированные блоки, не растущие вместе со
	// строками, поэтому входят в ту же неизменную часть формулы).
	ganttFixedChromePx = ganttTopPaddingPx + ganttTitleHeightPx + ganttHeaderHeightPx + ganttLegendHeightPx + ganttBottomPaddingPx
)

// ganttEffectiveRowCount — число строк, участвующее в формуле высоты листа
// (GanttPageSize) и высоты SVG (BuildGanttDiagram) — design.md Решение 2:
// обе величины ОБЯЗАНЫ выводиться из одного числа по одной формуле в одном
// месте (эта функция). Если задач нет, резервируется одна строка под явное
// сообщение об их отсутствии — пустая размеченная сетка без объяснения не
// выводится (Requirement «Отсутствие задач не ломает отчёт»), поэтому
// эффективное число строк не бывает меньше 1.
func ganttEffectiveRowCount(rowCount int) int {
	if rowCount < 1 {
		return 1
	}
	return rowCount
}

// ganttContentHeightPx — высота содержимого листа в CSS-пикселях для
// rowCount строк дерева. Единственное место, где считается эта величина —
// используется и GanttPageSize, и BuildGanttDiagram (design.md Решение 2).
func ganttContentHeightPx(rowCount int) float64 {
	return ganttFixedChromePx + float64(ganttEffectiveRowCount(rowCount))*ganttRowHeightPx
}

// GanttPageDimensions — размер листа с диаграммой Ганта: ширина листа A3 в
// альбомной ориентации (постоянна) и высота, растущая под число строк
// дерева (design.md Решение 2). Единицы измерения — дюймы, как у
// gotenberg.Chromium.PaperSize (используется секцией 3, generator.go).
type GanttPageDimensions struct {
	WidthInches  float64
	HeightInches float64
}

// GanttPageSize вычисляет размер листа диаграммы Ганта под число строк
// дерева rowCount (обычно len(ReportData.GanttTasks) — tasks.md §2.5).
// Высота SVG самой диаграммы (см. BuildGanttDiagram) вычисляется из того же
// rowCount функцией ganttContentHeightPx — единственным местом, где эта
// величина считается, поэтому высота листа и высота SVG не расходятся.
func GanttPageSize(rowCount int) GanttPageDimensions {
	return GanttPageDimensions{
		WidthInches:  ganttPageWidthInches,
		HeightInches: ganttContentHeightPx(rowCount) / ganttDPI,
	}
}

// ── Кодирование состояний (design.md Решение 6) ─────────────────────────────

// ganttRoleFillColor — фоновые (не единственные, см. ganttHatchPatternSVG)
// цвета заливки бара ролевой задачи — тот же приём наглядности, что и
// palette (см. svg.go), не единственный признак роли на бумаге: см.
// ganttHatchStyleIndex/ganttHatchPatternSVG ниже.
var ganttRoleFillColors = palette

// ganttHatchStyleCount — число различных геометрических штриховок,
// применяемых для кодирования роли ролевой задачи. Различение состояний не
// опирается только на цвет (design.md Решение 6): даже при чёрно-белой
// печати роли остаются различимы по штриховке.
const ganttHatchStyleCount = 6

// ganttHatchStyleIndex детерминированно сопоставляет название роли номеру
// штриховки/цвета — один и тот же ключ всегда даёт один и тот же стиль в
// пределах одного вызова BuildGanttDiagram и между разными вызовами.
func ganttHatchStyleIndex(roleName string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(roleName))
	return int(h.Sum32() % ganttHatchStyleCount)
}

// ganttHatchPatternSVG строит markup <pattern> с уникальным id, различимый
// геометрически (не только по цвету) — угол/тип штриховки зависит от
// styleIndex (0..ganttHatchStyleCount-1, см. ganttHatchStyleIndex).
func ganttHatchPatternSVG(id, color string, styleIndex int) string {
	const size = 10
	const stroke = 1.6
	switch styleIndex % ganttHatchStyleCount {
	case 0: // диагональная штриховка 45°
		return fmt.Sprintf(`<pattern id="%s" width="%d" height="%d" patternUnits="userSpaceOnUse" patternTransform="rotate(45)"><rect width="%d" height="%d" fill="%s" fill-opacity="0.35"/><line x1="0" y1="0" x2="0" y2="%d" stroke="#222" stroke-width="%g"/></pattern>`,
			id, size, size, size, size, color, size, stroke)
	case 1: // диагональная штриховка 135°
		return fmt.Sprintf(`<pattern id="%s" width="%d" height="%d" patternUnits="userSpaceOnUse" patternTransform="rotate(135)"><rect width="%d" height="%d" fill="%s" fill-opacity="0.35"/><line x1="0" y1="0" x2="0" y2="%d" stroke="#222" stroke-width="%g"/></pattern>`,
			id, size, size, size, size, color, size, stroke)
	case 2: // перекрёстная штриховка
		return fmt.Sprintf(`<pattern id="%s" width="%d" height="%d" patternUnits="userSpaceOnUse"><rect width="%d" height="%d" fill="%s" fill-opacity="0.35"/><line x1="0" y1="0" x2="%d" y2="%d" stroke="#222" stroke-width="%g"/><line x1="%d" y1="0" x2="0" y2="%d" stroke="#222" stroke-width="%g"/></pattern>`,
			id, size, size, size, size, color, size, size, stroke, size, size, stroke)
	case 3: // горизонтальная штриховка
		return fmt.Sprintf(`<pattern id="%s" width="%d" height="%d" patternUnits="userSpaceOnUse"><rect width="%d" height="%d" fill="%s" fill-opacity="0.35"/><line x1="0" y1="0" x2="%d" y2="0" stroke="#222" stroke-width="%g"/></pattern>`,
			id, size, size, size, size, color, size, stroke)
	case 4: // вертикальная штриховка
		return fmt.Sprintf(`<pattern id="%s" width="%d" height="%d" patternUnits="userSpaceOnUse"><rect width="%d" height="%d" fill="%s" fill-opacity="0.35"/><line x1="0" y1="0" x2="0" y2="%d" stroke="#222" stroke-width="%g"/></pattern>`,
			id, size, size, size, size, color, size, stroke)
	default: // точки
		return fmt.Sprintf(`<pattern id="%s" width="%d" height="%d" patternUnits="userSpaceOnUse"><rect width="%d" height="%d" fill="%s" fill-opacity="0.35"/><circle cx="%d" cy="%d" r="%g" fill="#222"/></pattern>`,
			id, size, size, size, size, color, size/2, size/2, stroke)
	}
}

// ganttPatternID возвращает id SVG-паттерна штриховки для названия роли —
// стабильный идентификатор, безопасный для использования в атрибуте id/url().
func ganttPatternID(roleName string) string {
	sum := fnv.New32a()
	_, _ = sum.Write([]byte(roleName))
	return fmt.Sprintf("gantt-role-%x", sum.Sum32())
}

// ── Календарная шкала (Requirement «Календарная шкала покрывает весь
// диапазон работ») ──────────────────────────────────────────────────────────

// ganttDateRange возвращает минимальную дату начала и максимальную дату
// окончания среди строк дерева — диапазон, который SHALL целиком покрывать
// календарная шкала, включая даты за пределами квартала отчёта.
func ganttDateRange(rows []GanttReportRow) (start, end time.Time, ok bool) {
	for i, r := range rows {
		if i == 0 || r.StartDate.Before(start) {
			start = r.StartDate
		}
		if i == 0 || r.EndDate.After(end) {
			end = r.EndDate
		}
	}
	return start, end, len(rows) > 0
}

// ganttXForDate переводит дату d в X-координату внутри области календарной
// сетки: chartX0 (левая граница) плюс число дней от start, умноженное на
// плотность пикселей на день (pxPerDay, см. BuildGanttDiagram). Вынесена в
// отдельную функцию, чтобы координаты бара можно было проверить в тестах
// той же формулой, что использует сам рендер (tasks.md §2.3).
func ganttXForDate(start, d time.Time, chartX0, pxPerDay float64) float64 {
	days := d.Sub(start).Hours() / 24
	return chartX0 + days*pxPerDay
}

// monthNamesRu — русские названия месяцев в родительном/именительном падеже
// для подписи шкалы.
var monthNamesRu = [...]string{
	"январь", "февраль", "март", "апрель", "май", "июнь",
	"июль", "август", "сентябрь", "октябрь", "ноябрь", "декабрь",
}

// BuildGanttDiagram строит SVG-диаграмму Ганта для листа PDF-отчёта:
// дерево строк (эпик → стори → ролевая задача, порядок как на вкладке
// «Гант» — см. GanttReportRow), календарная шкала на весь диапазон дат
// работ, бары по плановым датам, подписи с ролью/исполнителем, кодирование
// состояний, не опирающееся только на цвет, и легенда (design.md Решения
// 3/6). Возвращает строку SVG, встраиваемую в шаблон как template.HTML —
// тем же приёмом, что и диаграммы рисков (см. BuildRiskProbabilityDiagram).
//
// Высота SVG вычисляется из числа строк той же формулой, что и высота листа
// (см. GanttPageSize/ganttContentHeightPx) — они не могут разойтись, т.к.
// обе выведены из одной функции.
func BuildGanttDiagram(rows []GanttReportRow, teamName string, year, quarter int) string {
	width := ganttPageWidthPx
	height := ganttContentHeightPx(len(rows))

	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" width="%g" height="%g" viewBox="0 0 %g %g" font-family="Inter, sans-serif">`,
		width, height, width, height)

	// Общий фон листа.
	fmt.Fprintf(&sb, `<rect x="0" y="0" width="%g" height="%g" fill="#ffffff"/>`, width, height)

	// Заголовок листа — команда и период (Requirement «Лист SHALL быть
	// подписан так, чтобы из самого отчёта было понятно, к какой команде и
	// какому периоду относится диаграмма»).
	titleY := ganttTopPaddingPx + ganttTitleHeightPx*0.65
	fmt.Fprintf(&sb, `<text x="%g" y="%g" font-size="18" font-weight="bold" fill="#222">%s</text>`,
		ganttSidePaddingPx, titleY, html.EscapeString(fmt.Sprintf("Диаграмма Ганта — %s, %d квартал %d года", teamName, quarter, year)))

	chartX0 := ganttSidePaddingPx + ganttLabelColumnPx
	chartX1 := width - ganttSidePaddingPx
	headerY0 := ganttTopPaddingPx + ganttTitleHeightPx
	rowsY0 := headerY0 + ganttHeaderHeightPx
	rowsHeight := float64(ganttEffectiveRowCount(len(rows))) * ganttRowHeightPx
	legendY0 := rowsY0 + rowsHeight

	start, end, hasRows := ganttDateRange(rows)
	if !hasRows {
		// Нет ни одной задачи — явное сообщение вместо пустой размеченной
		// сетки (Requirement «Отсутствие задач не ломает отчёт»).
		fmt.Fprintf(&sb, `<text x="%g" y="%g" font-size="14" fill="#666">Нет запланированных работ за этот период.</text>`,
			chartX0, rowsY0+ganttRowHeightPx*0.6)
		sb.WriteString(ganttLegendSVG(nil, ganttSidePaddingPx, legendY0, width-2*ganttSidePaddingPx))
		sb.WriteString(`</svg>`)
		return sb.String()
	}

	// xForDate переводит дату в X-координату внутри области сетки
	// [chartX0, chartX1]. Диапазон включает и конечный день endDate целиком
	// (Requirement «Работы, выходящие за границы квартала… показаны
	// целиком»), поэтому знаменатель — число дней от start до end+1.
	totalDays := int(end.Sub(start).Hours()/24) + 1
	if totalDays < 1 {
		totalDays = 1
	}
	pxPerDay := (chartX1 - chartX0) / float64(totalDays)
	xForDate := func(d time.Time) float64 {
		return ganttXForDate(start, d, chartX0, pxPerDay)
	}

	// Календарная шкала: полоса месяцев + полоса недель с подписями дат —
	// по любой из них можно определить период бара без обращения к другим
	// источникам (Requirement «Календарная шкала покрывает весь диапазон
	// работ»).
	sb.WriteString(buildGanttScale(start, end, chartX0, chartX1, headerY0, rowsY0+rowsHeight, xForDate))

	// Паттерны штриховки — один <defs> на диаграмму, по одному на каждую
	// встретившуюся роль (design.md Решение 6).
	sb.WriteString(`<defs>`)
	roleNames := make(map[string]struct{})
	for _, r := range rows {
		if r.Level == GanttReportLevelRole {
			roleNames[r.Name] = struct{}{}
		}
	}
	sortedRoles := make([]string, 0, len(roleNames))
	for name := range roleNames {
		sortedRoles = append(sortedRoles, name)
	}
	sort.Strings(sortedRoles)
	for _, name := range sortedRoles {
		idx := ganttHatchStyleIndex(name)
		color := ganttRoleFillColors[idx%len(ganttRoleFillColors)]
		sb.WriteString(ganttHatchPatternSVG(ganttPatternID(name), color, idx))
	}
	sb.WriteString(`</defs>`)

	// Строки дерева: подписи слева, бары в области сетки.
	for i, row := range rows {
		rowY := rowsY0 + float64(i)*ganttRowHeightPx
		sb.WriteString(ganttRowSVG(row, rowY, chartX0, xForDate))
	}

	// Легенда — покрывает все применённые обозначения (design.md Решение 6).
	sb.WriteString(ganttLegendSVG(sortedRoles, ganttSidePaddingPx, legendY0, width-2*ganttSidePaddingPx))

	sb.WriteString(`</svg>`)
	return sb.String()
}

// buildGanttScale рисует полосу месяцев и полосу недель календарной шкалы,
// а также вертикальные направляющие на всю высоту строк дерева — по ним
// определяется период выполнения любого бара (Requirement «Период бара
// читается по шкале»).
func buildGanttScale(start, end time.Time, x0, x1, headerY0, gridBottomY float64, xForDate func(time.Time) float64) string {
	var sb strings.Builder

	weekY := headerY0 + ganttHeaderHeightPx*0.5
	monthY := headerY0 + ganttHeaderHeightPx*0.18

	fmt.Fprintf(&sb, `<rect x="%g" y="%g" width="%g" height="%g" fill="#f4f4f4" stroke="#ccc" stroke-width="1"/>`,
		x0, headerY0, x1-x0, ganttHeaderHeightPx)

	// Полоса месяцев: подпись на начало каждого месяца, попавшего в
	// диапазон (включая частично покрытые месяцы на границах квартала).
	monthCursor := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, start.Location())
	for !monthCursor.After(end) {
		xm := xForDate(monthCursor)
		if xm < x0 {
			xm = x0
		}
		fmt.Fprintf(&sb, `<line x1="%g" y1="%g" x2="%g" y2="%g" stroke="#999" stroke-width="1"/>`,
			xm, headerY0, xm, gridBottomY)
		label := fmt.Sprintf("%s %d", monthNamesRu[monthCursor.Month()-1], monthCursor.Year())
		fmt.Fprintf(&sb, `<text x="%g" y="%g" font-size="11" font-weight="bold" fill="#333">%s</text>`,
			xm+3, monthY+6, html.EscapeString(label))
		monthCursor = monthCursor.AddDate(0, 1, 0)
	}

	// Полоса недель: тонкая направляющая и дата (dd.mm) на каждый
	// понедельник в диапазоне (плюс граница диапазона), чтобы период любого
	// бара читался по шкале без обращения к другим источникам.
	weekCursor := start
	for int(weekCursor.Weekday()) != 1 { // 1 = понедельник (time.Monday)
		weekCursor = weekCursor.AddDate(0, 0, -1)
	}
	for !weekCursor.After(end) {
		if !weekCursor.Before(start) {
			xw := xForDate(weekCursor)
			fmt.Fprintf(&sb, `<line x1="%g" y1="%g" x2="%g" y2="%g" stroke="#ddd" stroke-width="1"/>`,
				xw, headerY0+ganttHeaderHeightPx*0.5, xw, gridBottomY)
			fmt.Fprintf(&sb, `<text x="%g" y="%g" font-size="9" fill="#555">%s</text>`,
				xw+2, weekY+16, weekCursor.Format("02.01"))
		}
		weekCursor = weekCursor.AddDate(0, 0, 7)
	}

	// Граница шкалы и области строк.
	fmt.Fprintf(&sb, `<line x1="%g" y1="%g" x2="%g" y2="%g" stroke="#999" stroke-width="1"/>`,
		x0, gridBottomY, x1, gridBottomY)

	return sb.String()
}

// ganttRowSVG рисует одну строку дерева: подпись слева (с отступом по
// глубине вложенности и, для ролевой задачи, ролью/исполнителем) и бар в
// области календарной сетки, закодированный по design.md Решение 6:
//   - роль (только у ролевых задач) — штриховка + цвет заливки;
//   - завершённость — маркер «✓» у правого края бара, для любой строки;
//   - отсутствие исполнителя (только у ролевых задач) — пунктирная обводка
//     бара вместо сплошной.
func ganttRowSVG(row GanttReportRow, rowY, labelX float64, xForDate func(time.Time) float64) string {
	var sb strings.Builder

	const vPad = 4.0
	barY := rowY + vPad
	barH := ganttRowHeightPx - 2*vPad

	x1 := xForDate(row.StartDate)
	x2 := xForDate(row.EndDate.AddDate(0, 0, 1))
	if x2-x1 < 2 {
		x2 = x1 + 2 // минимальная видимая ширина для однодневных задач
	}

	label := strings.Repeat("›", row.Depth)
	if row.Depth > 0 {
		label += " "
	}
	label += row.Name
	switch row.Level {
	case GanttReportLevelRole:
		if row.HasAssignee {
			label += " — " + row.AssigneeName
		} else {
			label += " — без исполнителя"
		}
	}
	labelIndent := labelX + float64(row.Depth)*ganttIndentPx

	fontWeight := "normal"
	fill := "#333"
	if row.Level != GanttReportLevelRole {
		fontWeight = "bold"
	}
	fmt.Fprintf(&sb, `<text x="%g" y="%g" font-size="11" font-weight="%s" fill="%s">%s</text>`,
		labelIndent, rowY+ganttRowHeightPx*0.65, fontWeight, fill, html.EscapeString(label))

	var fillAttr string
	switch row.Level {
	case GanttReportLevelRole:
		fillAttr = fmt.Sprintf("url(#%s)", ganttPatternID(row.Name))
	case GanttReportLevelStory:
		fillAttr = "#dbe4f0"
	default: // GanttReportLevelEpic
		fillAttr = "#c9d6e8"
	}

	strokeDash := ""
	if row.Level == GanttReportLevelRole && !row.HasAssignee {
		strokeDash = ` stroke-dasharray="4 2"`
	}
	strokeWidth := 1.0
	if row.Completed {
		strokeWidth = 2.2
	}

	fmt.Fprintf(&sb, `<rect x="%g" y="%g" width="%g" height="%g" fill="%s" stroke="#222" stroke-width="%g"%s/>`,
		x1, barY, x2-x1, barH, fillAttr, strokeWidth, strokeDash)

	if row.Completed {
		fmt.Fprintf(&sb, `<text x="%g" y="%g" font-size="12" font-weight="bold" fill="#0a6b2d">✓</text>`,
			x2+3, barY+barH*0.85)
	}

	return sb.String()
}

// ganttLegendSVG рисует легенду, покрывающую применённые обозначения
// (design.md Решение 6, Requirement «Легенда объясняет обозначения»):
// строки дерева по уровню (эпик/стори), по одной записи на каждую роль,
// встретившуюся на диаграмме, и отдельные маркеры завершённости/отсутствия
// исполнителя.
func ganttLegendSVG(roleNames []string, x, y, width float64) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, `<rect x="%g" y="%g" width="%g" height="%g" fill="#fafafa" stroke="#ddd" stroke-width="1"/>`,
		x, y, width, ganttLegendHeightPx)
	fmt.Fprintf(&sb, `<text x="%g" y="%g" font-size="12" font-weight="bold" fill="#222">Легенда</text>`,
		x+8, y+18)

	const swatchW, swatchH = 18.0, 12.0
	const colGap = 190.0
	const rowGap = 20.0
	col := 0
	row := 0
	maxCols := 3
	if width > 0 {
		if c := int(width / colGap); c > 0 {
			maxCols = c
		}
	}

	place := func() (cx, cy float64) {
		cx = x + 8 + float64(col)*colGap
		cy = y + 32 + float64(row)*rowGap
		col++
		if col >= maxCols {
			col = 0
			row++
		}
		return
	}

	// Уровни дерева (эпик/стори) — сплошная заливка без штриховки, как и на
	// самих строках диаграммы (см. ganttRowSVG).
	drawLevel := func(fill, labelText string) {
		cx, cy := place()
		fmt.Fprintf(&sb, `<rect x="%g" y="%g" width="%g" height="%g" fill="%s" stroke="#222" stroke-width="1"/>`,
			cx, cy, swatchW, swatchH, fill)
		fmt.Fprintf(&sb, `<text x="%g" y="%g" font-size="10" fill="#333">%s</text>`,
			cx+swatchW+6, cy+swatchH-2, html.EscapeString(labelText))
	}
	drawLevel("#c9d6e8", "Эпик")
	drawLevel("#dbe4f0", "Стори")

	for _, name := range roleNames {
		idx := ganttHatchStyleIndex(name)
		color := ganttRoleFillColors[idx%len(ganttRoleFillColors)]
		cx, cy := place()
		patternID := ganttPatternID(name) + "-legend"
		sb.WriteString(ganttHatchPatternSVG(patternID, color, idx))
		fmt.Fprintf(&sb, `<rect x="%g" y="%g" width="%g" height="%g" fill="url(#%s)" stroke="#222" stroke-width="1"/>`,
			cx, cy, swatchW, swatchH, patternID)
		fmt.Fprintf(&sb, `<text x="%g" y="%g" font-size="10" fill="#333">%s</text>`,
			cx+swatchW+6, cy+swatchH-2, html.EscapeString(name))
	}

	// Завершённость.
	{
		cx, cy := place()
		fmt.Fprintf(&sb, `<rect x="%g" y="%g" width="%g" height="%g" fill="#eee" stroke="#222" stroke-width="2.2"/>`,
			cx, cy, swatchW, swatchH)
		fmt.Fprintf(&sb, `<text x="%g" y="%g" font-size="11" font-weight="bold" fill="#0a6b2d">✓</text>`,
			cx+swatchW+2, cy+swatchH-2)
		fmt.Fprintf(&sb, `<text x="%g" y="%g" font-size="10" fill="#333">%s</text>`,
			cx+swatchW+16, cy+swatchH-2, "Завершено")
	}
	// Отсутствие исполнителя.
	{
		cx, cy := place()
		fmt.Fprintf(&sb, `<rect x="%g" y="%g" width="%g" height="%g" fill="#eee" stroke="#222" stroke-width="1" stroke-dasharray="4 2"/>`,
			cx, cy, swatchW, swatchH)
		fmt.Fprintf(&sb, `<text x="%g" y="%g" font-size="10" fill="#333">%s</text>`,
			cx+swatchW+6, cy+swatchH-2, "Нет исполнителя (пунктирная обводка)")
	}

	return sb.String()
}
