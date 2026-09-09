package report

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"EpicScoreBot/internal/config"
	"EpicScoreBot/internal/utils/logger/sl"

	"github.com/google/uuid"
	"github.com/nativebpm/gotenberg"
)

// ganttPageTemplateFS — шаблон листа диаграммы Ганта, ОТДЕЛЬНЫЙ от
// config/template.html (design.md change add-gantt-page-to-pdf-report,
// Решение 1, tasks.md §3.1). config/template.html — внешний файл,
// монтируемый в контейнер (путь берётся из PdfConfig.HtmlTemplateFilePath/
// Name), и правка такого файла требует согласованной выкатки образа; лист
// диаграммы, наоборот, вкомпилирован в бинарник через go:embed — как и SQL-
// миграции (internal/migrator/migrator.go) — поэтому не участвует в
// монтировании и откат релиза не требует восстанавливать внешний файл
// (design.md Risks/Migration Plan).
//
//go:embed templates/gantt_page.html
var ganttPageTemplateFS embed.FS

// ganttPageTemplateData передаётся templates/gantt_page.html. DiagramSVG —
// уже полностью отрисованный SVG (см. BuildGanttDiagram, gantt_svg.go),
// шаблону остаётся только встроить его в HTML-документ размером листа,
// вычисленным отдельно (см. GanttPageSize, convertGanttPage ниже).
type ganttPageTemplateData struct {
	TeamName   string
	DiagramSVG template.HTML
}

// Тайм-ауты конвейера PDF-отчёта (design.md Решение 1, tasks.md §3.3).
// Раньше был один запрос к Gotenberg с общим тайм-аутом и на HTTP-клиенте, и
// на контексте (60 секунд). Начиная с листа диаграммы Ганта запросов три
// последовательных (основной отчёт → лист диаграммы → объединение
// PDFEngines().Merge()), и общий бюджет на весь GenerateReport вырос
// соответственно — у каждого запроса теперь СВОЙ контекстный тайм-аут, а не
// один на всех.
const (
	// gotenbergHTTPClientTimeout — тайм-аут *http.Client, общего для всех
	// трёх запросов. http.Client.Timeout ограничивает КАЖДЫЙ отдельный
	// вызов независимо (это не суммарный бюджет на три запроса), поэтому
	// значение здесь — не сумма трёх тайм-аутов ниже, а запас НАД самым
	// долгим из них: так решающим для каждого запроса остаётся его
	// собственный context.WithTimeout, а не общий клиентский тайм-аут.
	gotenbergHTTPClientTimeout = 90 * time.Second
	// mainReportRequestTimeout — тайм-аут конвертации основной части отчёта
	// (как и раньше, единственный запрос до этого изменения).
	mainReportRequestTimeout = 60 * time.Second
	// ganttPageRequestTimeout — тайм-аут конвертации листа диаграммы Ганта.
	ganttPageRequestTimeout = 60 * time.Second
	// mergeRequestTimeout — тайм-аут объединения двух уже отрендеренных PDF
	// (PDFEngines().Merge()) — более лёгкая операция, чем рендеринг HTML,
	// поэтому бюджет меньше.
	mergeRequestTimeout = 30 * time.Second
)

// epicTemplateData extends EpicReportData with pre-rendered SVG chart strings
// and поячеечно округлённой вверх матрицей риск-скорректированной
// трудоёмкости этого эпика по ролям (см. RoundCapacityMatrix) — шаблон
// использует только уже округлённые величины в главной матрице.
type epicTemplateData struct {
	EpicReportData
	// RoundedRoleScores[roleName] = ceil(EpicReportData.RoleScores[roleName]).
	RoundedRoleScores map[string]int
	// RoundedTotal — «Итого (чд)» по эпику: сумма округлённых ячеек строки.
	RoundedTotal int
	// RawTotalScore — сумма сырых (без риск-фактора) оценок по ролям
	// (EpicReportItem.RawRoleScores). Используется в карточке эпика для
	// строки «Сумма оценок (без рисков)» — независимо от риск-скорректированной
	// FinalScore, показанной в шапке и внизу той же карточки (см. change
	// fix-pdf-report).
	RawTotalScore float64

	ProbabilityChartSVG template.HTML
	ImpactChartSVG      template.HTML
	CoefficientChartSVG template.HTML
	RiskLegend          []riskLegendItem
}

// riskLegendItem maps "Риск N" label to the risk description.
type riskLegendItem struct {
	Label       string
	Description string
}

// roleCapacityTemplateData расширяет RoleCapacityData округлёнными
// величинами: «Запланировано» (сумма округлённых вверх ячеек матрицы по
// этой роли, см. RoundCapacityMatrix), «Доступно» (округлённая вниз поролево
// ёмкость, см. RoundRoleCapacities) и «Разница» — оба уже целые, поэтому
// результат целый без отдельного правила округления (см. design.md change
// simplify-capacity-report, Decision 3/5).
type roleCapacityTemplateData struct {
	RoleCapacityData
	RoundedPlanned  int
	RoundedCapacity int
	RoundedDiff     int
}

// templateData is passed to the HTML template.
type templateData struct {
	TeamName string
	Year     int
	Quarter  int
	// TotalCapacity — общая доступная трудоёмкость команды: сумма
	// округлённых вниз поролево величин ёмкости (см. RoundRoleCapacities),
	// а не отдельно вычисленная величина от общей численности команды.
	TotalCapacity int
	// TotalRoundedPlanned — общий итог «Запланировано»: сумма округлённых
	// ячеек по всем ролям (равна сумме RoundedTotal по всем эпикам).
	TotalRoundedPlanned int
	// TotalDiff — TotalCapacity − TotalRoundedPlanned, оба уже целые.
	TotalDiff          int
	RoleCapacities     []roleCapacityTemplateData
	Quotas             map[string]QuotaData
	GeneratedFormatted string
	Epics              []epicTemplateData
}

// sortedEpicReportData возвращает копию epics, отсортированную по номеру
// задачи — аналогично sortedEpics (xlsx_generator.go), чтобы порядок строк
// главной матрицы в PDF совпадал с XLSX и веб-таблицей для одного и того же
// периода (см. design.md change simplify-capacity-report, требование
// «Согласованность данных…»).
func sortedEpicReportData(epics []EpicReportData) []EpicReportData {
	sorted := make([]EpicReportData, len(epics))
	copy(sorted, epics)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Number < sorted[j].Number
	})
	return sorted
}

// Generator creates PDF reports via Gotenberg.
type Generator struct {
	log *slog.Logger
	cfg *config.Config
}

// NewGenerator creates a new report generator.
func NewGenerator(logger *slog.Logger, cfg *config.Config) *Generator {
	return &Generator{
		log: logger.With(slog.String("component", "report")),
		cfg: cfg,
	}
}

// buildReportTemplateData преобразует ReportData (общее ядро
// CapacityReportResponse + риски эпиков) в templateData — форму,
// потребляемую config/template.html. Вынесена из GenerateReport в отдельную
// чистую функцию (без побочных эффектов и без зависимости от Gotenberg),
// чтобы числовую согласованность построчного округления PDF-таблицы с XLSX
// (см. RoundCapacityMatrix/RoundRoleCapacities) можно было проверить в
// unit-тестах без живого Gotenberg (см. cross_format_test.go).
func buildReportTemplateData(data ReportData) templateData {
	// Стабильный порядок ролей/эпиков — как в XLSX (sortedRoleCapacities/
	// sortedEpics), чтобы матрица PDF совпадала по составу строк/колонок с
	// XLSX и веб-таблицей для одного и того же периода.
	roleCapacities := sortedRoleCapacities(data.RoleCapacities)
	epicsSorted := sortedEpicReportData(data.Epics)

	roleNames := make([]string, len(roleCapacities))
	for i, rc := range roleCapacities {
		roleNames[i] = rc.RoleName
	}

	items := make([]EpicReportItem, len(epicsSorted))
	for i, e := range epicsSorted {
		items[i] = e.EpicReportItem
	}
	matrix := RoundCapacityMatrix(items, roleNames)

	// Prepare template data with SVG charts.
	var epics []epicTemplateData
	for i, e := range epicsSorted {
		var legend []riskLegendItem
		for idx, r := range e.Risks {
			legend = append(legend, riskLegendItem{
				Label:       fmt.Sprintf("Риск %d", idx+1),
				Description: r.Description,
			})
		}

		// RawTotalScore — сумма сырых (без риск-фактора) оценок по ролям,
		// показываемая в карточке эпика отдельно от риск-скорректированной
		// FinalScore (см. change fix-pdf-report).
		var rawTotal float64
		for _, v := range e.RawRoleScores {
			rawTotal += v
		}

		epics = append(epics, epicTemplateData{
			EpicReportData:      e,
			RoundedRoleScores:   matrix.Cells[i],
			RoundedTotal:        matrix.EpicTotals[i],
			RawTotalScore:       rawTotal,
			ProbabilityChartSVG: template.HTML(BuildRiskProbabilityDiagram(e.Risks)),
			ImpactChartSVG:      template.HTML(BuildRiskImpactDiagram(e.Risks)),
			CoefficientChartSVG: template.HTML(BuildRiskCoefficientDiagram(e.Risks)),
			RiskLegend:          legend,
		})
	}

	// Доступная ёмкость, округлённая вниз поролево — общий итог считается
	// как сумма уже округлённых величин по ролям (см. RoundRoleCapacities,
	// design.md Decision 5), а не отдельно от общей численности команды.
	capacities := RoundRoleCapacities(roleCapacities)

	roleCapacitiesTD := make([]roleCapacityTemplateData, len(roleCapacities))
	var totalRoundedPlanned int
	for i, rc := range roleCapacities {
		roundedCapacity := capacities.RoleCapacity[rc.RoleName]
		roundedPlanned := matrix.RolePlanned[rc.RoleName]
		roleCapacitiesTD[i] = roleCapacityTemplateData{
			RoleCapacityData: rc,
			RoundedPlanned:   roundedPlanned,
			RoundedCapacity:  roundedCapacity,
			RoundedDiff:      roundedCapacity - roundedPlanned,
		}
		totalRoundedPlanned += roundedPlanned
	}

	return templateData{
		TeamName:            data.TeamName,
		Year:                data.Year,
		Quarter:             data.Quarter,
		TotalCapacity:       capacities.Total,
		TotalRoundedPlanned: totalRoundedPlanned,
		TotalDiff:           capacities.Total - totalRoundedPlanned,
		RoleCapacities:      roleCapacitiesTD,
		Quotas:              data.Quotas,
		GeneratedFormatted:  data.Generated.Format("02.01.2006 15:04"),
		Epics:               epics,
	}
}

// buildGanttPageHTML рендерит HTML листа диаграммы Ганта через
// templates/gantt_page.html (см. ganttPageTemplateFS) — отдельно от
// основного шаблона отчёта. Строит саму диаграмму (BuildGanttDiagram,
// gantt_svg.go) из data.GanttTasks; ошибка построителя здесь невозможна —
// он всегда возвращает валидный SVG, включая явное сообщение об отсутствии
// задач для пустого дерева (design.md Решение 5/6, требование «Отсутствие
// задач не ломает отчёт»), — но ошибки самого шаблона (парсинг/выполнение)
// обрабатываются как деградация листа, а не отказ выгрузки, см. вызывающий
// код (GenerateReport, tasks.md §3.4).
func buildGanttPageHTML(data ReportData) (string, error) {
	op := "report.buildGanttPageHTML"

	tmpl, err := template.ParseFS(ganttPageTemplateFS, "templates/gantt_page.html")
	if err != nil {
		return "", fmt.Errorf("%s: parse template: %w", op, err)
	}

	svg := BuildGanttDiagram(data.GanttTasks, data.TeamName, data.Year, data.Quarter)
	td := ganttPageTemplateData{
		TeamName:   data.TeamName,
		DiagramSVG: template.HTML(svg),
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, td); err != nil {
		return "", fmt.Errorf("%s: execute template: %w", op, err)
	}

	return buf.String(), nil
}

// GenerateReport renders the report HTML, converts it to PDF via Gotenberg,
// and returns the absolute path to the generated PDF file.
//
// Отчёт собирается тремя последовательными обращениями к Gotenberg
// (design.md change add-gantt-page-to-pdf-report, Решение 1): основная
// часть — как и раньше, единственная конвертация с PaperSizeA4(); лист
// диаграммы Ганта — отдельная конвертация собственного размера
// (GanttPageSize); затем оба PDF объединяются PDFEngines().Merge() в один
// файл. Ошибка на любом шаге, начиная со второго, деградирует до отчёта БЕЗ
// листа диаграммы, а не отказывает во всей выгрузке (design.md Решение 5,
// tasks.md §3.4) — основная часть отчёта самоценна и до этого изменения
// формировалась вовсе без Ганта.
func (g *Generator) GenerateReport(ctx context.Context, data ReportData) (string, error) {
	op := "report.GenerateReport"
	log := g.log.With(slog.String("op", op), slog.String("team", data.TeamName))

	td := buildReportTemplateData(data)

	templateFullPath := filepath.Join(g.cfg.PdfConfig.HtmlTemplateFilePath, g.cfg.PdfConfig.HtmlTemplateFileName)
	// Parse and render template.
	tmpl, err := template.ParseFiles(templateFullPath)
	if err != nil {
		return "", fmt.Errorf("%s: parse template: %w", op, err)
	}

	var htmlBuf bytes.Buffer
	if err := tmpl.Execute(&htmlBuf, td); err != nil {
		return "", fmt.Errorf("%s: execute template: %w", op, err)
	}

	log.Debug("rendered HTML report", slog.Int("html_size", htmlBuf.Len()))

	gotenbergURL := fmt.Sprintf("http://%s:%d", g.cfg.PdfConfig.PdfHost, g.cfg.PdfConfig.PdfPort)
	httpClient := &http.Client{Timeout: gotenbergHTTPClientTimeout}
	client, err := gotenberg.NewClient(httpClient, gotenbergURL)
	if err != nil {
		return "", fmt.Errorf("%s: create gotenberg client: %w", op, err)
	}

	mainPDF, err := g.convertMainReport(ctx, client, &htmlBuf)
	if err != nil {
		return "", fmt.Errorf("%s: %w", op, err)
	}

	// Лист диаграммы Ганта: сбой на любом из шагов (рендер HTML, конвертация,
	// объединение) не прерывает выгрузку — отчёт отдаётся без него, проблема
	// уходит в журнал (design.md Решение 5, tasks.md §3.4).
	finalPDF := mainPDF
	ganttPageIncluded := false
	ganttPDF, err := g.convertGanttPage(ctx, client, data)
	if err != nil {
		log.Error("failed to build the gantt diagram page, returning the report without it", sl.Err(err))
	} else if merged, err := g.mergeReportPDFs(ctx, client, mainPDF, ganttPDF); err != nil {
		log.Error("failed to merge the gantt diagram page into the report, returning the report without it", sl.Err(err))
	} else {
		finalPDF = merged
		ganttPageIncluded = true
	}

	requestID := uuid.New()
	filename := fmt.Sprintf("%s.pdf", requestID)
	outPath := filepath.Join(g.cfg.PdfConfig.PdfFilePath, filename)

	file, err := os.Create(filepath.Clean(outPath))
	if err != nil {
		return "", fmt.Errorf("%s: create output file: %w", op, err)
	}
	defer file.Close()

	if _, err := file.Write(finalPDF); err != nil {
		return "", fmt.Errorf("%s: write pdf: %w", op, err)
	}

	log.Info("report PDF generated",
		slog.String("path", outPath),
		slog.Int("epics", len(data.Epics)),
		slog.Int("gantt_tasks", len(data.GanttTasks)),
		slog.Bool("gantt_page_included", ganttPageIncluded),
	)

	return outPath, nil
}

// readGotenbergResponse читает тело ответа Gotenberg и превращает
// неуспешный HTTP-статус в ошибку. Клиент gotenberg (nativebpm/gotenberg,
// internal/gotenberg/gotenberg.go: Send()) статус ответа сам НЕ проверяет —
// Send() возвращает err=nil даже для 4xx/5xx, различить успех и отказ можно
// только по response.StatusCode. Без этой проверки сбой конвертации/слияния
// (например, при исчерпании тайм-аута контекста Gotenberg или невалидном
// HTML) читался бы как «пустой, но успешный» PDF, а не как ошибка —
// используется во всех трёх запросах конвейера (convertMainReport,
// convertGanttPage, mergeReportPDFs), чтобы деградация (design.md Решение 5,
// tasks.md §3.4) срабатывала на реальный сбой, а не проглатывала его.
func readGotenbergResponse(op string, response *gotenberg.Response) ([]byte, error) {
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: read response: %w", op, err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("%s: unexpected gotenberg status %d: %s", op, response.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

// convertMainReport конвертирует HTML основной части отчёта в PDF —
// PaperSizeA4(), как и до появления листа диаграммы Ганта (это поведение
// tasks.md §3.2 требует оставить без изменений).
func (g *Generator) convertMainReport(ctx context.Context, client *gotenberg.Client, html *bytes.Buffer) ([]byte, error) {
	op := "report.convertMainReport"

	reqCtx, cancel := context.WithTimeout(ctx, mainReportRequestTimeout)
	defer cancel()

	response, err := client.Chromium().
		ConvertHTML(reqCtx, html).
		PaperSizeA4().
		Margins(1, 1, 1, 1).
		OutputFilename("1_report.pdf").
		Send()
	if err != nil {
		return nil, fmt.Errorf("%s: gotenberg convert: %w", op, err)
	}
	defer response.Body.Close()

	return readGotenbergResponse(op, response)
}

// convertGanttPage строит HTML листа диаграммы Ганта (buildGanttPageHTML) и
// конвертирует его в PDF собственного размера — PaperSize(ширина, высота) из
// GanttPageSize, вычисленного той же формулой, что и высота самого SVG (см.
// gantt_svg.go, design.md Решение 2). Margins(0,0,0,0): отступы уже заложены
// внутри разметки SVG (ganttTopPaddingPx и др.), внешние поля запроса
// конвертации задваивали бы их и сдвигали фактический размер содержимого
// относительно вычисленного размера листа.
func (g *Generator) convertGanttPage(ctx context.Context, client *gotenberg.Client, data ReportData) ([]byte, error) {
	op := "report.convertGanttPage"

	html, err := buildGanttPageHTML(data)
	if err != nil {
		return nil, fmt.Errorf("%s: build html: %w", op, err)
	}

	dims := GanttPageSize(len(data.GanttTasks))

	reqCtx, cancel := context.WithTimeout(ctx, ganttPageRequestTimeout)
	defer cancel()

	response, err := client.Chromium().
		ConvertHTML(reqCtx, strings.NewReader(html)).
		PaperSize(dims.WidthInches, dims.HeightInches).
		Margins(0, 0, 0, 0).
		OutputFilename("2_gantt.pdf").
		Send()
	if err != nil {
		return nil, fmt.Errorf("%s: gotenberg convert: %w", op, err)
	}
	defer response.Body.Close()

	return readGotenbergResponse(op, response)
}

// mergeReportPDFs объединяет основной отчёт и лист диаграммы Ганта в один
// файл через PDFEngines().Merge() (design.md Решение 1). Gotenberg
// объединяет файлы в алфавитном порядке имён из формы запроса — "1_report.pdf"
// и "2_gantt.pdf" гарантируют, что лист диаграммы окажется ПОСЛЕ основного
// отчёта (Requirement «Лист SHALL располагаться после существующих разделов
// отчёта»), не полагаясь на порядок вызовов .File().
func (g *Generator) mergeReportPDFs(ctx context.Context, client *gotenberg.Client, mainPDF, ganttPDF []byte) ([]byte, error) {
	op := "report.mergeReportPDFs"

	reqCtx, cancel := context.WithTimeout(ctx, mergeRequestTimeout)
	defer cancel()

	response, err := client.PDFEngines().
		Merge(reqCtx).
		File("1_report.pdf", bytes.NewReader(mainPDF)).
		File("2_gantt.pdf", bytes.NewReader(ganttPDF)).
		Send()
	if err != nil {
		return nil, fmt.Errorf("%s: gotenberg merge: %w", op, err)
	}
	defer response.Body.Close()

	return readGotenbergResponse(op, response)
}
