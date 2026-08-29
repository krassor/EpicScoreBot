package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"EpicScoreBot/internal/report"
	"EpicScoreBot/internal/services"
	"github.com/google/uuid"
)

// Типы ниже — алиасы на internal/report (см. report/types.go), где они
// определены как общая модель данных отчёта о вместимости команды, которую
// строит services.BuildCapacityReport и используют одновременно JSON-эндпоинт
// GetCapacityReport и XLSX-ветка ExportTeamReport. Алиасы сохранены, чтобы не
// менять сигнатуры/имена типов, уже используемые в этом пакете и его тестах.
type (
	RoleCapacityData       = report.RoleCapacityData
	QuotaData              = report.QuotaData
	EpicReportItem         = report.EpicReportItem
	CapacityReportResponse = report.CapacityReportResponse
)

// resolveReportPeriod возвращает год/квартал отчёта на основе query-парам
// year/quarter, либо значения по умолчанию (текущий отчётный период), если
// параметры не переданы или некорректны. Используется и GetCapacityReport, и
// ExportTeamReport, чтобы дефолты периода оставались согласованными между
// JSON-эндпоинтом и выгрузкой файлов (см. tasks.md, задача 2.1).
func resolveReportPeriod(yearStr, quarterStr string) (year, quarter int) {
	year = 2026
	if yearStr != "" {
		if y, err := strconv.Atoi(yearStr); err == nil {
			year = y
		}
	}

	quarter = 3
	if quarterStr != "" {
		if q, err := strconv.Atoi(quarterStr); err == nil && q >= 1 && q <= 4 {
			quarter = q
		}
	}

	return year, quarter
}

func (h *GanttHandler) GetCapacityReport(w http.ResponseWriter, r *http.Request) {
	op := "handlers.GetCapacityReport"
	log := h.log.With(slog.String("op", op))

	teamIDStr := r.URL.Query().Get("team_id")
	yearStr := r.URL.Query().Get("year")
	quarterStr := r.URL.Query().Get("quarter")

	if teamIDStr == "" {
		writeError(w, http.StatusBadRequest, "team_id is required")
		return
	}

	teamUUID, err := uuid.Parse(teamIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid team_id")
		return
	}

	year, quarter := resolveReportPeriod(yearStr, quarterStr)

	resp, err := services.BuildCapacityReport(r.Context(), log, h.repo, teamUUID, year, quarter)
	if err != nil {
		if errors.Is(err, services.ErrTeamNotFound) {
			log.Error("failed to get team", slog.String("error", err.Error()))
			writeError(w, http.StatusNotFound, "team not found")
			return
		}
		log.Error("failed to build capacity report", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to build capacity report")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// ExportTeamReport handles GET /api/gantt/reports/export?team_id=&year=&quarter=&format=pdf|xlsx —
// отдаёт отчёт по команде за год/квартал файлом (PDF через существующий
// Gotenberg-генератор, либо XLSX, построенный из того же агрегатора, что и
// GetCapacityReport). См. specs/team-report-export (change add-web-report).
func (h *GanttHandler) ExportTeamReport(w http.ResponseWriter, r *http.Request) {
	op := "handlers.ExportTeamReport"
	log := h.log.With(slog.String("op", op))

	teamIDStr := r.URL.Query().Get("team_id")
	yearStr := r.URL.Query().Get("year")
	quarterStr := r.URL.Query().Get("quarter")
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))

	if teamIDStr == "" {
		writeErrorCode(w, http.StatusBadRequest, "team_id_required", "team_id is required")
		return
	}

	teamUUID, err := uuid.Parse(teamIDStr)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_team_id", "invalid team_id")
		return
	}

	if format != "pdf" && format != "xlsx" {
		writeErrorCode(w, http.StatusBadRequest, "invalid_format", "format must be 'pdf' or 'xlsx'")
		return
	}

	year, quarter := resolveReportPeriod(yearStr, quarterStr)

	team, err := h.repo.GetTeamByID(r.Context(), teamUUID)
	if err != nil {
		log.Error("failed to get team", slog.String("error", err.Error()))
		writeErrorCode(w, http.StatusNotFound, "team_not_found", "team not found")
		return
	}
	if team == nil {
		log.Error("team not found")
		writeErrorCode(w, http.StatusNotFound, "team_not_found", "team not found")
		return
	}

	switch format {
	case "pdf":
		h.exportTeamReportPDF(w, r, log, team.Name, teamUUID, year, quarter)
	case "xlsx":
		h.exportTeamReportXLSX(w, r, log, teamUUID, year, quarter)
	}
}

// exportTeamReportPDF формирует PDF через уже существующие
// epicService.GetReportData/report.Generator.GenerateReport (см.
// internal/telegram/admin_callbacks.go.generateAndSendReportExt — тот же
// источник данных и генератор, без изменения их логики/шаблона) и отдаёт
// результат как файл.
func (h *GanttHandler) exportTeamReportPDF(w http.ResponseWriter, r *http.Request, log *slog.Logger, teamName string, teamID uuid.UUID, year, quarter int) {
	reportData, err := h.reportData.GetReportData(r.Context(), teamID, year, quarter)
	if err != nil {
		log.Error("failed to get report data", slog.String("error", err.Error()))
		writeErrorCode(w, http.StatusInternalServerError, "report_data_failed", "failed to get report data")
		return
	}

	pdfPath, err := h.reportGen.GenerateReport(r.Context(), *reportData)
	if err != nil {
		log.Error("failed to generate pdf report", slog.String("error", err.Error()))
		writeErrorCode(w, http.StatusInternalServerError, "pdf_generation_failed", "failed to generate pdf report")
		return
	}
	// Файл сгенерирован для одноразовой выгрузки по HTTP (в отличие от
	// Telegram-бота, где файл читается один раз в sendDocument) — удаляем
	// после отдачи клиенту, чтобы не копить файлы на диске при повторных
	// скачиваниях из веб-панели.
	defer func() {
		if err := os.Remove(pdfPath); err != nil {
			log.Warn("failed to remove temporary pdf file", slog.String("path", pdfPath), slog.String("error", err.Error()))
		}
	}()

	file, err := os.Open(pdfPath)
	if err != nil {
		log.Error("failed to open generated pdf file", slog.String("error", err.Error()))
		writeErrorCode(w, http.StatusInternalServerError, "pdf_read_failed", "failed to read generated pdf file")
		return
	}
	defer file.Close()

	filename := reportExportFilename(teamName, year, quarter, "pdf")
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", contentDispositionAttachment(filename))
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, file); err != nil {
		log.Error("failed to stream pdf file", slog.String("error", err.Error()))
	}
}

// exportTeamReportXLSX строит данные тем же агрегатором, что и
// GetCapacityReport (services.BuildCapacityReport), и отдаёт их в виде XLSX
// файла (report.GenerateCapacityXLSX).
func (h *GanttHandler) exportTeamReportXLSX(w http.ResponseWriter, r *http.Request, log *slog.Logger, teamID uuid.UUID, year, quarter int) {
	resp, err := services.BuildCapacityReport(r.Context(), log, h.repo, teamID, year, quarter)
	if err != nil {
		if errors.Is(err, services.ErrTeamNotFound) {
			log.Error("failed to get team", slog.String("error", err.Error()))
			writeErrorCode(w, http.StatusNotFound, "team_not_found", "team not found")
			return
		}
		log.Error("failed to build capacity report", slog.String("error", err.Error()))
		writeErrorCode(w, http.StatusInternalServerError, "capacity_report_failed", "failed to build capacity report")
		return
	}

	xlsxBytes, err := report.GenerateCapacityXLSX(*resp)
	if err != nil {
		log.Error("failed to generate xlsx report", slog.String("error", err.Error()))
		writeErrorCode(w, http.StatusInternalServerError, "xlsx_generation_failed", "failed to generate xlsx report")
		return
	}

	filename := reportExportFilename(resp.TeamName, year, quarter, "xlsx")
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", contentDispositionAttachment(filename))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(xlsxBytes); err != nil {
		log.Error("failed to write xlsx response", slog.String("error", err.Error()))
	}
}

// reportExportFilename формирует осмысленное имя файла выгрузки отчёта:
// "Отчет-<команда>-<год>-Q<квартал>.<ext>", с очисткой символов, недопустимых
// в имени файла (используются в Content-Disposition).
func reportExportFilename(teamName string, year, quarter int, ext string) string {
	safeName := strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '_'
		}
		return r
	}, teamName)
	safeName = strings.TrimSpace(safeName)
	if safeName == "" {
		safeName = "team"
	}
	return fmt.Sprintf("Отчет-%s-%d-Q%d.%s", safeName, year, quarter, ext)
}

// contentDispositionAttachment строит значение заголовка Content-Disposition
// с ASCII-фолбэком (для старых клиентов) и RFC 5987/6266 параметром
// filename* с корректной UTF-8-выгрузкой — имена команд в этом проекте
// обычно кириллические (reportExportFilename), поэтому один только
// ASCII-safe filename= привёл бы к нечитаемому имени файла в браузере.
func contentDispositionAttachment(filename string) string {
	asciiFallback := strings.Map(func(r rune) rune {
		if r > 127 {
			return '_'
		}
		return r
	}, filename)
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, asciiFallback, url.PathEscape(filename))
}
