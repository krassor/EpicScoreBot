package report

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"EpicScoreBot/internal/config"
)

// discardLogger — тихий *slog.Logger для тестов конвейера GenerateReport
// (tasks.md §3.2-§3.4): сами тесты проверяют поведение (успех/деградацию),
// а не содержимое журнала.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeGotenbergServer — минимальная имитация трёх маршрутов Gotenberg,
// которые использует GenerateReport (design.md Решение 1):
//   - POST /forms/chromium/convert/html — вызывается ДВАЖДЫ последовательно
//     (основной отчёт, затем лист диаграммы); различаются по порядковому
//     номеру вызова, а не по URL (это тот же маршрут).
//   - POST /forms/pdfengines/merge — объединение; в этой имитации не
//     реализует настоящее слияние PDF, а конкатенирует байты полученных
//     файлов В ТОМ ЖЕ ПОРЯДКЕ, в котором они присутствуют в multipart-форме —
//     этого достаточно, чтобы тест проверил и содержимое, и порядок
//     (Requirement «Лист SHALL располагаться после существующих разделов
//     отчёта»).
type fakeGotenbergServer struct {
	t *testing.T

	convertCalls atomic.Int32
	mainPDF      []byte
	ganttPDF     []byte

	// failOnConvertCall — если >0, N-й вызов /forms/chromium/convert/html
	// (1 = основной отчёт, 2 = лист диаграммы) отвечает 500.
	failOnConvertCall int32
	failMerge         bool

	// lastGanttPaperWidth/Height — форм-параметры paperWidth/paperHeight
	// второго вызова convert/html (лист диаграммы), захватываются для
	// проверки размера листа (tasks.md §3.2).
	lastGanttPaperWidth  string
	lastGanttPaperHeight string
}

func newFakeGotenbergServer(t *testing.T) *fakeGotenbergServer {
	return &fakeGotenbergServer{
		t:        t,
		mainPDF:  []byte("%PDF-MAIN-REPORT-BYTES"),
		ganttPDF: []byte("%PDF-GANTT-PAGE-BYTES"),
	}
}

func (f *fakeGotenbergServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/forms/chromium/convert/html", func(w http.ResponseWriter, r *http.Request) {
		call := f.convertCalls.Add(1)

		if err := r.ParseMultipartForm(10 << 20); err != nil {
			f.t.Fatalf("fake gotenberg: parse multipart form: %v", err)
		}

		if call == f.failOnConvertCall {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/pdf")
		switch call {
		case 1: // основной отчёт
			_, _ = w.Write(f.mainPDF)
		case 2: // лист диаграммы Ганта
			f.lastGanttPaperWidth = r.FormValue("paperWidth")
			f.lastGanttPaperHeight = r.FormValue("paperHeight")
			_, _ = w.Write(f.ganttPDF)
		default:
			f.t.Fatalf("fake gotenberg: unexpected extra call to convert/html: %d", call)
		}
	})
	mux.HandleFunc("/forms/pdfengines/merge", func(w http.ResponseWriter, r *http.Request) {
		if f.failMerge {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			f.t.Fatalf("fake gotenberg: parse multipart form: %v", err)
		}
		files := r.MultipartForm.File["files"]
		w.Header().Set("Content-Type", "application/pdf")
		for _, fh := range files {
			file, err := fh.Open()
			if err != nil {
				f.t.Fatalf("fake gotenberg: open uploaded file: %v", err)
			}
			_, _ = io.Copy(w, file)
			_ = file.Close()
		}
	})
	return mux
}

// testConfig строит *config.Config, указывающий на fake-сервер вместо
// реального Gotenberg и на реальный config/template.html (основная часть
// отчёта рендерится настоящим шаблоном — тест проверяет конвейер, а не его
// содержимое).
func testConfig(t *testing.T, serverURL string) *config.Config {
	t.Helper()
	u, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("invalid fake server URL %q: %v", serverURL, err)
	}
	host, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatalf("invalid fake server host:port %q: %v", u.Host, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("invalid fake server port %q: %v", portStr, err)
	}

	templatePath := templateHTMLPath(t)
	dir := templatePath[:len(templatePath)-len("template.html")]

	return &config.Config{
		PdfConfig: config.PdfConfig{
			PdfHost:              host,
			PdfPort:              port,
			HtmlTemplateFilePath: dir,
			HtmlTemplateFileName: "template.html",
			PdfFilePath:          t.TempDir(),
		},
	}
}

func minimalPipelineReportData(ganttTasks []GanttReportRow) ReportData {
	return ReportData{
		CapacityReportResponse: CapacityReportResponse{
			TeamName: "Пилоты",
			Year:     2026,
			Quarter:  3,
		},
		GanttTasks: ganttTasks,
	}
}

// ── 3.2 Вторая конвертация + объединение ─────────────────────────────────

func TestGenerateReport_MergesGanttPageAfterMainReport(t *testing.T) {
	fake := newFakeGotenbergServer(t)
	server := httptest.NewServer(fake.handler())
	defer server.Close()

	cfg := testConfig(t, server.URL)
	gen := NewGenerator(discardLogger(), cfg)

	data := minimalPipelineReportData([]GanttReportRow{
		{Level: GanttReportLevelEpic, Name: "EP-1", StartDate: d(2026, 7, 1), EndDate: d(2026, 7, 5)},
	})

	outPath, err := gen.GenerateReport(t.Context(), data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read generated file: %v", err)
	}

	if !bytes.HasPrefix(got, fake.mainPDF) {
		t.Errorf("expected the merged file to start with the main report bytes")
	}
	if !bytes.HasSuffix(got, fake.ganttPDF) {
		t.Errorf("expected the merged file to end with the gantt page bytes (after the main report — Requirement «Лист SHALL располагаться после существующих разделов»)")
	}

	wantDims := GanttPageSize(len(data.GanttTasks))
	if fake.lastGanttPaperWidth != fmt.Sprintf("%g", wantDims.WidthInches) {
		t.Errorf("gantt page paperWidth = %q, want %g", fake.lastGanttPaperWidth, wantDims.WidthInches)
	}
	if fake.lastGanttPaperHeight != fmt.Sprintf("%g", wantDims.HeightInches) {
		t.Errorf("gantt page paperHeight = %q, want %g", fake.lastGanttPaperHeight, wantDims.HeightInches)
	}

	if fake.convertCalls.Load() != 2 {
		t.Errorf("expected exactly 2 calls to chromium convert (main + gantt page), got %d", fake.convertCalls.Load())
	}
}

// ── 3.4 Деградация: отчёт без листа диаграммы, а не отказ выгрузки ───────

func TestGenerateReport_NoGanttTasks_StillSucceedsWithExplicitMessage(t *testing.T) {
	fake := newFakeGotenbergServer(t)
	server := httptest.NewServer(fake.handler())
	defer server.Close()

	cfg := testConfig(t, server.URL)
	gen := NewGenerator(discardLogger(), cfg)

	// Команда без задач Ганта — GanttTasks пуст (design.md Решение 5,
	// Requirement «Отсутствие задач не ломает отчёт»). Лист диаграммы всё
	// равно строится (BuildGanttDiagram сам выводит явное сообщение вместо
	// голой сетки — см. gantt_svg.go), поэтому конвейер идёт по тому же
	// счастливому пути, что и с задачами.
	data := minimalPipelineReportData(nil)

	outPath, err := gen.GenerateReport(t.Context(), data)
	if err != nil {
		t.Fatalf("expected successful generation for a team with no gantt tasks, got: %v", err)
	}
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read generated file: %v", err)
	}
	if !bytes.HasPrefix(got, fake.mainPDF) || !bytes.HasSuffix(got, fake.ganttPDF) {
		t.Errorf("expected the report to still include the gantt page (with its own empty-state message)")
	}
}

func TestGenerateReport_GanttPageConvertFails_DegradesToReportWithoutIt(t *testing.T) {
	fake := newFakeGotenbergServer(t)
	fake.failOnConvertCall = 2 // второй вызов convert/html — лист диаграммы
	server := httptest.NewServer(fake.handler())
	defer server.Close()

	cfg := testConfig(t, server.URL)
	gen := NewGenerator(discardLogger(), cfg)

	data := minimalPipelineReportData([]GanttReportRow{
		{Level: GanttReportLevelEpic, Name: "EP-1", StartDate: d(2026, 7, 1), EndDate: d(2026, 7, 5)},
	})

	outPath, err := gen.GenerateReport(t.Context(), data)
	if err != nil {
		t.Fatalf("expected the export to succeed despite the gantt page conversion failing, got: %v", err)
	}
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read generated file: %v", err)
	}
	if !bytes.Equal(got, fake.mainPDF) {
		t.Errorf("expected the report to fall back to exactly the main report bytes, got %d bytes (want %d)", len(got), len(fake.mainPDF))
	}
}

func TestGenerateReport_MergeFails_DegradesToReportWithoutGanttPage(t *testing.T) {
	fake := newFakeGotenbergServer(t)
	fake.failMerge = true
	server := httptest.NewServer(fake.handler())
	defer server.Close()

	cfg := testConfig(t, server.URL)
	gen := NewGenerator(discardLogger(), cfg)

	data := minimalPipelineReportData([]GanttReportRow{
		{Level: GanttReportLevelEpic, Name: "EP-1", StartDate: d(2026, 7, 1), EndDate: d(2026, 7, 5)},
	})

	outPath, err := gen.GenerateReport(t.Context(), data)
	if err != nil {
		t.Fatalf("expected the export to succeed despite the merge step failing, got: %v", err)
	}
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read generated file: %v", err)
	}
	if !bytes.Equal(got, fake.mainPDF) {
		t.Errorf("expected the report to fall back to exactly the main report bytes when merge fails, got %d bytes (want %d)", len(got), len(fake.mainPDF))
	}
}

func TestGenerateReport_MainReportConvertFails_WholeExportFails(t *testing.T) {
	fake := newFakeGotenbergServer(t)
	fake.failOnConvertCall = 1 // основной отчёт — единственная часть, чей сбой обязан отказывать выгрузке целиком
	server := httptest.NewServer(fake.handler())
	defer server.Close()

	cfg := testConfig(t, server.URL)
	gen := NewGenerator(discardLogger(), cfg)

	data := minimalPipelineReportData(nil)

	if _, err := gen.GenerateReport(t.Context(), data); err == nil {
		t.Fatal("expected an error when the main report conversion itself fails")
	}
}

// ── 3.3 Тайм-ауты: собственный контекст на каждый из трёх запросов ────────

// TestGenerateReport_CanceledContext_FailsFastNotHangs проверяет, что
// GenerateReport реально прокидывает переданный ctx в запрос к Gotenberg
// (а не игнорирует его в пользу только внутренних тайм-аутов) — уже
// отменённый контекст обязан провалить конвертацию быстро и с понятной
// ошибкой, а не зависнуть. Сами тайм-ауты (§3.3) — десятки секунд, ждать их
// исчерпания в юнит-тесте непрактично; это минимальный автоматизируемый
// прокси-тест механизма «внятная ошибка, а не зависание» (ручная проверка
// полных тайм-аутов — задача §5, вне периметра автотестов).
func TestGenerateReport_CanceledContext_FailsFastNotHangs(t *testing.T) {
	fake := newFakeGotenbergServer(t)
	server := httptest.NewServer(fake.handler())
	defer server.Close()

	cfg := testConfig(t, server.URL)
	gen := NewGenerator(discardLogger(), cfg)

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // контекст уже отменён к моменту вызова

	data := minimalPipelineReportData(nil)

	done := make(chan struct{})
	var err error
	go func() {
		_, err = gen.GenerateReport(ctx, data)
		close(done)
	}()

	select {
	case <-done:
		if err == nil {
			t.Fatal("expected an error for an already-canceled context, not a silent success")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("GenerateReport hung instead of failing fast on an already-canceled context")
	}
}
