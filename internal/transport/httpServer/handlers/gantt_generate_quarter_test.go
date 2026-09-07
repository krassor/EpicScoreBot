package handlers

import (
	"EpicScoreBot/internal/config"
	"EpicScoreBot/internal/gantt"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

// newGenerateQuarterHandler создаёт GanttHandler с переданным мок-сервисом
// для тестов GenerateQuarterTasks — общая настройка для всех сценариев ниже.
func newGenerateQuarterHandler(svc *mockGanttSvc) *GanttHandler {
	return NewGanttHandler(slog.Default(), svc, &mockGanttRepo{}, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})
}

// decodeErrorResponse проверяет, что тело ответа соответствует стандартному
// формату ошибок проекта { "error": { "code": "...", "message": "..." } }.
func decodeErrorResponse(t *testing.T, body []byte) (code, message string) {
	t.Helper()
	var resp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("failed to unmarshal error response %q: %v", body, err)
	}
	if resp.Error.Code == "" {
		t.Errorf("expected non-empty error.code in response %q", body)
	}
	if resp.Error.Message == "" {
		t.Errorf("expected non-empty error.message in response %q", body)
	}
	return resp.Error.Code, resp.Error.Message
}

func postGenerateQuarter(handler *GanttHandler, payload map[string]any) *httptest.ResponseRecorder {
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/tasks/generate-quarter", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler.GenerateQuarterTasks(w, req)
	return w
}

// TestGenerateQuarterTasks_Success проверяет успешный запрос массовой
// перегенерации квартала: сервис вызывается с распарсенными параметрами,
// ответ содержит контракт { message, epics_total, epics_regenerated,
// epics_failed, tasks_count }.
func TestGenerateQuarterTasks_Success(t *testing.T) {
	teamID := uuid.New()

	svc := &mockGanttSvc{
		generateTasksForQuarterFunc: func(ctx context.Context, tid uuid.UUID, year, quarter int, startDate time.Time) (gantt.QuarterGenerationResult, error) {
			return gantt.QuarterGenerationResult{
				EpicsTotal:       3,
				EpicsRegenerated: 3,
				EpicsFailed:      0,
				TasksCount:       12,
			}, nil
		},
	}
	handler := newGenerateQuarterHandler(svc)

	w := postGenerateQuarter(handler, map[string]any{
		"team_id":    teamID.String(),
		"year":       2026,
		"quarter":    3,
		"start_date": "2026-07-13",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
	}

	if !svc.generateTasksForQuarterCalled {
		t.Fatal("expected GenerateTasksForQuarter to be called")
	}
	if svc.generateTasksForQuarterArgs.teamID != teamID {
		t.Errorf("expected teamID %s, got %s", teamID, svc.generateTasksForQuarterArgs.teamID)
	}
	if svc.generateTasksForQuarterArgs.year != 2026 {
		t.Errorf("expected year 2026, got %d", svc.generateTasksForQuarterArgs.year)
	}
	if svc.generateTasksForQuarterArgs.quarter != 3 {
		t.Errorf("expected quarter 3, got %d", svc.generateTasksForQuarterArgs.quarter)
	}
	wantStart := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	if !svc.generateTasksForQuarterArgs.startDate.Equal(wantStart) {
		t.Errorf("expected startDate %v, got %v", wantStart, svc.generateTasksForQuarterArgs.startDate)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp["message"] == nil || resp["message"] == "" {
		t.Errorf("expected non-empty message in response, got %v", resp["message"])
	}
	if resp["epics_total"] != float64(3) {
		t.Errorf("expected epics_total 3, got %v", resp["epics_total"])
	}
	if resp["epics_regenerated"] != float64(3) {
		t.Errorf("expected epics_regenerated 3, got %v", resp["epics_regenerated"])
	}
	if resp["epics_failed"] != float64(0) {
		t.Errorf("expected epics_failed 0, got %v", resp["epics_failed"])
	}
	if resp["tasks_count"] != float64(12) {
		t.Errorf("expected tasks_count 12, got %v", resp["tasks_count"])
	}
}

// TestGenerateQuarterTasks_InvalidRequestBody проверяет, что некорректное
// (не-JSON) тело запроса отклоняется с 400 в стандартном формате ошибки.
func TestGenerateQuarterTasks_InvalidRequestBody(t *testing.T) {
	svc := &mockGanttSvc{}
	handler := newGenerateQuarterHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/tasks/generate-quarter", bytes.NewReader([]byte("{not json")))
	w := httptest.NewRecorder()
	handler.GenerateQuarterTasks(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d, body: %s", w.Code, w.Body.String())
	}
	decodeErrorResponse(t, w.Body.Bytes())
	if svc.generateTasksForQuarterCalled {
		t.Error("service must not be called on invalid request body")
	}
}

// TestGenerateQuarterTasks_InvalidTeamID проверяет отклонение запроса с
// невалидным team_id.
func TestGenerateQuarterTasks_InvalidTeamID(t *testing.T) {
	svc := &mockGanttSvc{}
	handler := newGenerateQuarterHandler(svc)

	w := postGenerateQuarter(handler, map[string]any{
		"team_id":    "not-a-uuid",
		"year":       2026,
		"quarter":    3,
		"start_date": "2026-07-13",
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d, body: %s", w.Code, w.Body.String())
	}
	decodeErrorResponse(t, w.Body.Bytes())
	if svc.generateTasksForQuarterCalled {
		t.Error("service must not be called with invalid team_id")
	}
}

// TestGenerateQuarterTasks_InvalidQuarter проверяет отклонение запроса с
// кварталом вне диапазона 1-4.
func TestGenerateQuarterTasks_InvalidQuarter(t *testing.T) {
	cases := []int{0, 5, -1}
	for _, quarter := range cases {
		svc := &mockGanttSvc{}
		handler := newGenerateQuarterHandler(svc)

		w := postGenerateQuarter(handler, map[string]any{
			"team_id":    uuid.New().String(),
			"year":       2026,
			"quarter":    quarter,
			"start_date": "2026-07-13",
		})

		if w.Code != http.StatusBadRequest {
			t.Fatalf("quarter=%d: expected status 400, got %d, body: %s", quarter, w.Code, w.Body.String())
		}
		decodeErrorResponse(t, w.Body.Bytes())
		if svc.generateTasksForQuarterCalled {
			t.Errorf("quarter=%d: service must not be called with invalid quarter", quarter)
		}
	}
}

// TestGenerateQuarterTasks_InvalidYear проверяет отклонение запроса с годом
// вне допустимого диапазона (2000-2100), совпадающего с диапазоном года
// эпика, принятым на фронтенде.
func TestGenerateQuarterTasks_InvalidYear(t *testing.T) {
	cases := []int{1999, 2101, 0}
	for _, year := range cases {
		svc := &mockGanttSvc{}
		handler := newGenerateQuarterHandler(svc)

		w := postGenerateQuarter(handler, map[string]any{
			"team_id":    uuid.New().String(),
			"year":       year,
			"quarter":    3,
			"start_date": "2026-07-13",
		})

		if w.Code != http.StatusBadRequest {
			t.Fatalf("year=%d: expected status 400, got %d, body: %s", year, w.Code, w.Body.String())
		}
		decodeErrorResponse(t, w.Body.Bytes())
		if svc.generateTasksForQuarterCalled {
			t.Errorf("year=%d: service must not be called with invalid year", year)
		}
	}
}

// TestGenerateQuarterTasks_InvalidStartDate проверяет отклонение запроса с
// датой начала не в формате YYYY-MM-DD.
func TestGenerateQuarterTasks_InvalidStartDate(t *testing.T) {
	cases := []string{"13-07-2026", "2026/07/13", "not-a-date", ""}
	for _, startDate := range cases {
		svc := &mockGanttSvc{}
		handler := newGenerateQuarterHandler(svc)

		w := postGenerateQuarter(handler, map[string]any{
			"team_id":    uuid.New().String(),
			"year":       2026,
			"quarter":    3,
			"start_date": startDate,
		})

		if w.Code != http.StatusBadRequest {
			t.Fatalf("start_date=%q: expected status 400, got %d, body: %s", startDate, w.Code, w.Body.String())
		}
		decodeErrorResponse(t, w.Body.Bytes())
		if svc.generateTasksForQuarterCalled {
			t.Errorf("start_date=%q: service must not be called with invalid start_date", startDate)
		}
	}
}

// TestGenerateQuarterTasks_ServiceError проверяет, что ошибка сервиса
// транслируется в 500 в стандартном формате ошибки.
func TestGenerateQuarterTasks_ServiceError(t *testing.T) {
	svc := &mockGanttSvc{
		generateTasksForQuarterFunc: func(ctx context.Context, tid uuid.UUID, year, quarter int, startDate time.Time) (gantt.QuarterGenerationResult, error) {
			return gantt.QuarterGenerationResult{}, errors.New("boom")
		},
	}
	handler := newGenerateQuarterHandler(svc)

	w := postGenerateQuarter(handler, map[string]any{
		"team_id":    uuid.New().String(),
		"year":       2026,
		"quarter":    3,
		"start_date": "2026-07-13",
	})

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d, body: %s", w.Code, w.Body.String())
	}
	decodeErrorResponse(t, w.Body.Bytes())
}
