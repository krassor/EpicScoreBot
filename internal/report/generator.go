package report

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"EpicScoreBot/internal/config"

	"github.com/google/uuid"
	"github.com/nativebpm/gotenberg"
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

// roleCapacityTemplateData расширяет RoleCapacityData округлённым
// «Запланировано» (сумма округлённых вверх ячеек матрицы по этой роли, см.
// RoundCapacityMatrix) — используется строкой «Запланировано» главной
// матрицы, тогда как «Доступно»/«Разница» (Capacity/Diff) считаются по
// точной формуле ёмкости и не округляются (см. design.md change
// simplify-capacity-report, Non-Goals).
type roleCapacityTemplateData struct {
	RoleCapacityData
	RoundedPlanned int
}

// templateData is passed to the HTML template.
type templateData struct {
	TeamName      string
	Year          int
	Quarter       int
	TotalCapacity float64
	// TotalRoundedPlanned — общий итог «Запланировано»: сумма округлённых
	// ячеек по всем ролям (равна сумме RoundedTotal по всем эпикам).
	TotalRoundedPlanned int
	TotalDiff           float64
	RoleCapacities      []roleCapacityTemplateData
	Quotas              map[string]QuotaData
	GeneratedFormatted  string
	Epics               []epicTemplateData
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

// GenerateReport renders the report HTML, converts it to PDF via Gotenberg,
// and returns the absolute path to the generated PDF file.
func (g *Generator) GenerateReport(ctx context.Context, data ReportData) (string, error) {
	op := "report.GenerateReport"
	log := g.log.With(slog.String("op", op))

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

		epics = append(epics, epicTemplateData{
			EpicReportData:      e,
			RoundedRoleScores:   matrix.Cells[i],
			RoundedTotal:        matrix.EpicTotals[i],
			ProbabilityChartSVG: template.HTML(BuildRiskProbabilityDiagram(e.Risks)),
			ImpactChartSVG:      template.HTML(BuildRiskImpactDiagram(e.Risks)),
			CoefficientChartSVG: template.HTML(BuildRiskCoefficientDiagram(e.Risks)),
			RiskLegend:          legend,
		})
	}

	roleCapacitiesTD := make([]roleCapacityTemplateData, len(roleCapacities))
	var totalPlanned float64 // точная (неокруглённая) сумма — только для строки «Разница».
	var totalRoundedPlanned int
	for i, rc := range roleCapacities {
		roleCapacitiesTD[i] = roleCapacityTemplateData{
			RoleCapacityData: rc,
			RoundedPlanned:   matrix.RolePlanned[rc.RoleName],
		}
		totalPlanned += rc.Planned
		totalRoundedPlanned += matrix.RolePlanned[rc.RoleName]
	}

	td := templateData{
		TeamName:            data.TeamName,
		Year:                data.Year,
		Quarter:             data.Quarter,
		TotalCapacity:       data.TotalCapacity,
		TotalRoundedPlanned: totalRoundedPlanned,
		TotalDiff:           data.TotalCapacity - totalPlanned,
		RoleCapacities:      roleCapacitiesTD,
		Quotas:              data.Quotas,
		GeneratedFormatted:  data.Generated.Format("02.01.2006 15:04"),
		Epics:               epics,
	}

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

	// Convert HTML to PDF via Gotenberg.
	httpClient := &http.Client{Timeout: 60 * time.Second}
	gotenbergURL := fmt.Sprintf("http://%s:%d", g.cfg.PdfConfig.PdfHost, g.cfg.PdfConfig.PdfPort)

	client, err := gotenberg.NewClient(httpClient, gotenbergURL)
	if err != nil {
		return "", fmt.Errorf("%s: create gotenberg client: %w", op, err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	requestID := uuid.New()
	filename := fmt.Sprintf("%s.pdf", requestID)

	response, err := client.Chromium().
		ConvertHTML(reqCtx, &htmlBuf).
		PaperSizeA4().
		Margins(1, 1, 1, 1).
		OutputFilename(filename).
		Send()
	if err != nil {
		return "", fmt.Errorf("%s: gotenberg convert: %w", op, err)
	}
	defer response.Body.Close()

	outPath := filepath.Join(g.cfg.PdfConfig.PdfFilePath, filename)
	file, err := os.Create(filepath.Clean(outPath))
	if err != nil {
		return "", fmt.Errorf("%s: create output file: %w", op, err)
	}
	defer file.Close()

	if _, err := file.ReadFrom(response.Body); err != nil {
		return "", fmt.Errorf("%s: write pdf: %w", op, err)
	}

	log.Info("report PDF generated",
		slog.String("path", outPath),
		slog.String("team", data.TeamName),
		slog.Int("epics", len(data.Epics)),
	)

	return outPath, nil
}
