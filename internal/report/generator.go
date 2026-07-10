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
	"time"

	"EpicScoreBot/internal/config"

	"github.com/google/uuid"
	"github.com/nativebpm/gotenberg"
)

// epicTemplateData extends EpicReportData with pre-rendered SVG chart strings.
type epicTemplateData struct {
	EpicReportData
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

// templateData is passed to the HTML template.
type templateData struct {
	TeamName           string
	Year               int
	Quarter            int
	TotalCapacity      float64
	TotalPlanned       float64
	TotalFinalPlanned  float64
	TotalDiff          float64
	RoleCapacities     []RoleCapacityReportData
	Quotas             map[string]QuotaReportData
	GeneratedFormatted string
	Epics              []epicTemplateData
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

	// Prepare template data with SVG charts.
	var epics []epicTemplateData
	for _, e := range data.Epics {
		var legend []riskLegendItem
		for i, r := range e.Risks {
			legend = append(legend, riskLegendItem{
				Label:       fmt.Sprintf("Риск %d", i+1),
				Description: r.Description,
			})
		}

		epics = append(epics, epicTemplateData{
			EpicReportData:      e,
			ProbabilityChartSVG: template.HTML(BuildRiskProbabilityDiagram(e.Risks)),
			ImpactChartSVG:      template.HTML(BuildRiskImpactDiagram(e.Risks)),
			CoefficientChartSVG: template.HTML(BuildRiskCoefficientDiagram(e.Risks)),
			RiskLegend:          legend,
		})
	}

	var totalPlanned float64
	for _, rc := range data.RoleCapacities {
		totalPlanned += rc.Planned
	}

	var totalFinalPlanned float64
	for _, e := range epics {
		totalFinalPlanned += e.FinalScore
	}

	td := templateData{
		TeamName:           data.TeamName,
		Year:               data.Year,
		Quarter:            data.Quarter,
		TotalCapacity:      data.TotalCapacity,
		TotalPlanned:       totalPlanned,
		TotalFinalPlanned:  totalFinalPlanned,
		TotalDiff:          data.TotalCapacity - totalPlanned,
		RoleCapacities:     data.RoleCapacities,
		Quotas:             data.Quotas,
		GeneratedFormatted: data.Generated.Format("02.01.2006 15:04"),
		Epics:              epics,
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
