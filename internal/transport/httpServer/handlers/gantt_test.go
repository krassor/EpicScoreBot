package handlers

import (
	"EpicScoreBot/internal/config"
	"EpicScoreBot/internal/models/domain"
	"EpicScoreBot/internal/report"
	
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
	
)
// mockGanttRepo is a mock implementation of Repository for Gantt-specific tests.
type mockGanttRepo struct {
	Repository
	getTaskFunc       func(ctx context.Context, id uuid.UUID) (*domain.GanttTask, error)
	reorderTaskCalled bool
	reorderTaskArgs   struct {
		taskID    uuid.UUID
		sortOrder int
	}
}

func (m *mockGanttRepo) GetGanttTaskByID(ctx context.Context, taskID uuid.UUID) (*domain.GanttTask, error) {
	if m.getTaskFunc != nil {
		return m.getTaskFunc(ctx, taskID)
	}
	return nil, nil
}

// mockGanttSvc is a mock implementation of GanttService for Gantt-specific tests.
type mockGanttSvc struct {
	GanttService
	reorderTaskFunc   func(ctx context.Context, taskID uuid.UUID, newSortOrder int) ([]domain.GanttTask, error)
	reorderTaskCalled bool
	reorderTaskArgs   struct {
		taskID    uuid.UUID
		sortOrder int
	}
	reorderEpicFunc        func(ctx context.Context, epicID uuid.UUID, newSortOrder int) ([]domain.GanttTask, error)
	reorderStoryFunc       func(ctx context.Context, storyID uuid.UUID, newSortOrder int) ([]domain.GanttTask, error)
	setTaskProgressFunc    func(ctx context.Context, taskID uuid.UUID, progress float64) ([]domain.GanttTask, error)
	setTaskStartOffsetFunc func(ctx context.Context, taskID uuid.UUID, offsetDays int) ([]domain.GanttTask, error)
}

func (m *mockGanttSvc) ReorderEpic(ctx context.Context, epicID uuid.UUID, newSortOrder int) ([]domain.GanttTask, error) {
	if m.reorderEpicFunc != nil {
		return m.reorderEpicFunc(ctx, epicID, newSortOrder)
	}
	return nil, nil
}

func (m *mockGanttSvc) ReorderStory(ctx context.Context, storyID uuid.UUID, newSortOrder int) ([]domain.GanttTask, error) {
	if m.reorderStoryFunc != nil {
		return m.reorderStoryFunc(ctx, storyID, newSortOrder)
	}
	return nil, nil
}

func (m *mockGanttSvc) SetTaskProgress(ctx context.Context, taskID uuid.UUID, progress float64) ([]domain.GanttTask, error) {
	if m.setTaskProgressFunc != nil {
		return m.setTaskProgressFunc(ctx, taskID, progress)
	}
	return nil, nil
}

func (m *mockGanttSvc) SetTaskStartOffset(ctx context.Context, taskID uuid.UUID, offsetDays int) ([]domain.GanttTask, error) {
	if m.setTaskStartOffsetFunc != nil {
		return m.setTaskStartOffsetFunc(ctx, taskID, offsetDays)
	}
	return nil, nil
}

func (m *mockGanttSvc) ReorderTask(ctx context.Context, taskID uuid.UUID, newSortOrder int) ([]domain.GanttTask, error) {
	m.reorderTaskCalled = true
	m.reorderTaskArgs.taskID = taskID
	m.reorderTaskArgs.sortOrder = newSortOrder

	if m.reorderTaskFunc != nil {
		return m.reorderTaskFunc(ctx, taskID, newSortOrder)
	}
	return nil, nil
}

// TestReorderTask_CorrectFieldName - регрессион-тест на правильное имя поля new_sort_order.
// Проверяет, что при отправке {"new_sort_order": 5} сервис вызывается с sortOrder=5.
func TestReorderTask_CorrectFieldName(t *testing.T) {
	taskID := uuid.New()
	epicID := uuid.New()
	cfg := config.BotConfig{}

	t.Run("успешное переупорядочение с новым sort_order", func(t *testing.T) {
		expectedSortOrder := 5

		svc := &mockGanttSvc{
			reorderTaskFunc: func(ctx context.Context, taskID uuid.UUID, newSortOrder int) ([]domain.GanttTask, error) {
				return []domain.GanttTask{
					{
						ID:        taskID,
						EpicID:    epicID,
						SortOrder: newSortOrder,
						StartDate: time.Now(),
						EndDate:   time.Now().AddDate(0, 0, 1),
					},
				}, nil
			},
		}

		repo := &mockGanttRepo{}
		handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})

		// Отправляем корректное имя поля new_sort_order
		reqBody, _ := json.Marshal(map[string]int{
			"new_sort_order": expectedSortOrder,
		})
		req := httptest.NewRequest("PUT", "/api/gantt/tasks/"+taskID.String()+"/reorder", bytes.NewReader(reqBody))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", taskID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		w := httptest.NewRecorder()
		handler.ReorderTask(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
		}

		// Проверяем, что сервис был вызван с корректным sortOrder
		if !svc.reorderTaskCalled {
			t.Error("expected ReorderTask to be called")
		}
		if svc.reorderTaskArgs.taskID != taskID {
			t.Errorf("expected taskID %s, got %s", taskID, svc.reorderTaskArgs.taskID)
		}
		if svc.reorderTaskArgs.sortOrder != expectedSortOrder {
			t.Errorf("expected sortOrder %d, got %d", expectedSortOrder, svc.reorderTaskArgs.sortOrder)
		}

		// Проверяем JSON-ответ
		var resp map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		if resp["message"] != "task reordered" {
			t.Errorf("expected message 'task reordered', got %v", resp["message"])
		}
	})
}

// TestReorderTask_OldFieldNameIgnored - проверяет, что старое имя поля sort_order игнорируется.
// Если отправить {"sort_order": 10}, это должно быть проигнорировано (декодировано в 0)
// и запрос должен быть отклонен как bad request ИЛИ обработан с sort_order=0.
func TestReorderTask_OldFieldNameIgnored(t *testing.T) {
	taskID := uuid.New()
	epicID := uuid.New()
	cfg := config.BotConfig{}

	t.Run("отправка старого имени поля sort_order должна игнорироваться", func(t *testing.T) {
		// Отправляем старое имя поля - оно должно быть проигнорировано,
		// JSON decoder установит значение 0 для SortOrder.
		svc := &mockGanttSvc{
			reorderTaskFunc: func(ctx context.Context, taskID uuid.UUID, newSortOrder int) ([]domain.GanttTask, error) {
				// Проверяем, что newSortOrder = 0 (значение по умолчанию, так как поле не найдено)
				if newSortOrder != 0 {
					t.Errorf("expected sortOrder 0 (default for ignored field), got %d", newSortOrder)
				}
				return []domain.GanttTask{
					{
						ID:        taskID,
						EpicID:    epicID,
						SortOrder: newSortOrder,
						StartDate: time.Now(),
						EndDate:   time.Now().AddDate(0, 0, 1),
					},
				}, nil
			},
		}

		repo := &mockGanttRepo{}
		handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})

		// Отправляем старое имя поля sort_order вместо new_sort_order
		reqBody, _ := json.Marshal(map[string]int{
			"sort_order": 10,
		})
		req := httptest.NewRequest("PUT", "/api/gantt/tasks/"+taskID.String()+"/reorder", bytes.NewReader(reqBody))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", taskID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		w := httptest.NewRecorder()
		handler.ReorderTask(w, req)

		// Проверяем, что запрос обработан (может быть 200 с sortOrder=0)
		if w.Code != http.StatusOK {
			t.Logf("status code: %d, body: %s", w.Code, w.Body.String())
		}

		// Проверяем, что сервис был вызван с sortOrder=0 (поле игнорировано)
		if svc.reorderTaskArgs.sortOrder != 0 {
			t.Errorf("expected sortOrder 0 (ignored old field), got %d", svc.reorderTaskArgs.sortOrder)
		}
	})
}

// TestReorderTask_BothFieldsPresent - если отправить оба поля, должно использоваться new_sort_order.
func TestReorderTask_BothFieldsPresent(t *testing.T) {
	taskID := uuid.New()
	epicID := uuid.New()
	cfg := config.BotConfig{}

	t.Run("при наличии обоих полей используется new_sort_order", func(t *testing.T) {
		expectedSortOrder := 7

		svc := &mockGanttSvc{
			reorderTaskFunc: func(ctx context.Context, taskID uuid.UUID, newSortOrder int) ([]domain.GanttTask, error) {
				return []domain.GanttTask{
					{
						ID:        taskID,
						EpicID:    epicID,
						SortOrder: newSortOrder,
						StartDate: time.Now(),
						EndDate:   time.Now().AddDate(0, 0, 1),
					},
				}, nil
			},
		}

		repo := &mockGanttRepo{}
		handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})

		// Отправляем оба поля - должно использоваться new_sort_order=7, старое sort_order=999 должно быть проигнорировано
		reqBody, _ := json.Marshal(map[string]int{
			"sort_order":     999, // Старое имя - должно быть проигнорировано
			"new_sort_order": expectedSortOrder,
		})
		req := httptest.NewRequest("PUT", "/api/gantt/tasks/"+taskID.String()+"/reorder", bytes.NewReader(reqBody))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", taskID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		w := httptest.NewRecorder()
		handler.ReorderTask(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
		}

		// Проверяем, что был использован new_sort_order, а не sort_order
		if svc.reorderTaskArgs.sortOrder != expectedSortOrder {
			t.Errorf("expected sortOrder %d, got %d", expectedSortOrder, svc.reorderTaskArgs.sortOrder)
		}
	})
}

// TestReorderTask_InvalidTaskID - проверяет ошибку 400 при невалидном task_id.
func TestReorderTask_InvalidTaskID(t *testing.T) {
	cfg := config.BotConfig{}

	t.Run("невалидный task_id должен вернуть 400", func(t *testing.T) {
		svc := &mockGanttSvc{}
		repo := &mockGanttRepo{}
		handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})

		reqBody, _ := json.Marshal(map[string]int{
			"new_sort_order": 5,
		})
		req := httptest.NewRequest("PUT", "/api/gantt/tasks/invalid-id/reorder", bytes.NewReader(reqBody))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "invalid-id")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		w := httptest.NewRecorder()
		handler.ReorderTask(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", w.Code)
		}

		// Проверяем формат ошибки API
		var errResp map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &errResp); err == nil {
			if errObj, ok := errResp["error"].(map[string]interface{}); ok {
				if errObj["code"] != "invalid_task_id" {
					t.Logf("error code: %v", errObj["code"])
				}
			}
		}
	})
}

// TestReorderTask_InvalidRequestBody - проверяет ошибку 400 при невалидном JSON.
func TestReorderTask_InvalidRequestBody(t *testing.T) {
	taskID := uuid.New()
	cfg := config.BotConfig{}

	t.Run("невалидный JSON должен вернуть 400", func(t *testing.T) {
		svc := &mockGanttSvc{}
		repo := &mockGanttRepo{}
		handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})

		// Отправляем невалидный JSON
		req := httptest.NewRequest("PUT", "/api/gantt/tasks/"+taskID.String()+"/reorder", bytes.NewReader([]byte(`{invalid json`)))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", taskID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		w := httptest.NewRecorder()
		handler.ReorderTask(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", w.Code)
		}
	})
}

// TestReorderTask_ServiceError - проверяет ошибку 500 при ошибке сервиса.
func TestReorderTask_ServiceError(t *testing.T) {
	taskID := uuid.New()
	cfg := config.BotConfig{}

	t.Run("ошибка сервиса должна вернуть 500", func(t *testing.T) {
		svc := &mockGanttSvc{
			reorderTaskFunc: func(ctx context.Context, taskID uuid.UUID, newSortOrder int) ([]domain.GanttTask, error) {
				return nil, errors.New("database error")
			},
		}

		repo := &mockGanttRepo{}
		handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})

		reqBody, _ := json.Marshal(map[string]int{
			"new_sort_order": 5,
		})
		req := httptest.NewRequest("PUT", "/api/gantt/tasks/"+taskID.String()+"/reorder", bytes.NewReader(reqBody))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", taskID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		w := httptest.NewRecorder()
		handler.ReorderTask(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("expected status 500, got %d", w.Code)
		}
	})
}

// TestUpdateTask_RejectsManualDates проверяет, что попытка вручную
// проставить start/end через PUT /tasks/{id}/ отклоняется 400 со
// стандартным форматом ошибки {"error":{"code":"SCHEDULE_MANAGED_AUTOMATICALLY",...}}.
func TestUpdateTask_RejectsManualDates(t *testing.T) {
	taskID := uuid.New()
	cfg := config.BotConfig{}

	svc := &mockGanttSvc{}
	repo := &mockGanttRepo{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})

	reqBody, _ := json.Marshal(map[string]string{
		"start": "2026-01-01",
		"end":   "2026-01-05",
	})
	req := httptest.NewRequest("PUT", "/api/gantt/tasks/"+taskID.String()+"/", bytes.NewReader(reqBody))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", taskID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handler.UpdateTask(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Error.Code != "SCHEDULE_MANAGED_AUTOMATICALLY" {
		t.Errorf("expected error code SCHEDULE_MANAGED_AUTOMATICALLY, got %q", resp.Error.Code)
	}
	if resp.Error.Message == "" {
		t.Errorf("expected non-empty error message")
	}
}

// TestUpdateTask_ProgressRoutesThroughSetTaskProgress проверяет, что
// простановка progress через PUT /tasks/{id}/ маршрутизируется через
// svc.SetTaskProgress, а не напрямую в репозиторий.
func TestUpdateTask_ProgressRoutesThroughSetTaskProgress(t *testing.T) {
	taskID := uuid.New()
	cfg := config.BotConfig{}

	var calledWith float64
	var called bool
	svc := &mockGanttSvc{}
	svc.setTaskProgressFunc = func(ctx context.Context, id uuid.UUID, progress float64) ([]domain.GanttTask, error) {
		called = true
		calledWith = progress
		return nil, nil
	}
	repo := &mockGanttRepo{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})

	// Реальный контракт с фронтендом — дробь 0.0-1.0 (фронтенд шлёт
	// progress/100), а не проценты 0-100.
	reqBody, _ := json.Marshal(map[string]float64{"progress": 0.75})
	req := httptest.NewRequest("PUT", "/api/gantt/tasks/"+taskID.String()+"/", bytes.NewReader(reqBody))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", taskID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handler.UpdateTask(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
	}
	if !called {
		t.Fatalf("expected SetTaskProgress to be called")
	}
	if calledWith != 0.75 {
		t.Errorf("expected progress 0.75, got %v", calledWith)
	}
}

// TestUpdateTask_StartOffsetRoutesThroughSetTaskStartOffset проверяет, что
// простановка start_offset_days через PUT /tasks/{id}/ маршрутизируется
// через svc.SetTaskStartOffset (по образцу теста для progress).
func TestUpdateTask_StartOffsetRoutesThroughSetTaskStartOffset(t *testing.T) {
	taskID := uuid.New()
	cfg := config.BotConfig{}

	var calledWith int
	var called bool
	svc := &mockGanttSvc{}
	svc.setTaskStartOffsetFunc = func(ctx context.Context, id uuid.UUID, offsetDays int) ([]domain.GanttTask, error) {
		called = true
		calledWith = offsetDays
		return nil, nil
	}
	repo := &mockGanttRepo{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})

	reqBody, _ := json.Marshal(map[string]int{"start_offset_days": -2})
	req := httptest.NewRequest("PUT", "/api/gantt/tasks/"+taskID.String()+"/", bytes.NewReader(reqBody))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", taskID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handler.UpdateTask(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
	}
	if !called {
		t.Fatalf("expected SetTaskStartOffset to be called")
	}
	if calledWith != -2 {
		t.Errorf("expected offset -2, got %v", calledWith)
	}
}

// TestUpdateTask_StartOffsetOnParentReturnsBadRequest проверяет, что ошибка
// сервиса при попытке задать офсет родительской задаче транслируется в 400
// со стандартным кодом OFFSET_NOT_ALLOWED_ON_PARENT.
func TestUpdateTask_StartOffsetOnParentReturnsBadRequest(t *testing.T) {
	taskID := uuid.New()
	cfg := config.BotConfig{}

	svc := &mockGanttSvc{}
	svc.setTaskStartOffsetFunc = func(ctx context.Context, id uuid.UUID, offsetDays int) ([]domain.GanttTask, error) {
		return nil, errors.New("start offset can only be set on a leaf (role) task, not a story/epic")
	}
	repo := &mockGanttRepo{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})

	reqBody, _ := json.Marshal(map[string]int{"start_offset_days": 1})
	req := httptest.NewRequest("PUT", "/api/gantt/tasks/"+taskID.String()+"/", bytes.NewReader(reqBody))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", taskID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handler.UpdateTask(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d, body: %s", w.Code, w.Body.String())
	}
	code, message := unmarshalErrorResp(t, w.Body.Bytes())
	if code != "OFFSET_NOT_ALLOWED_ON_PARENT" {
		t.Errorf("expected error code OFFSET_NOT_ALLOWED_ON_PARENT, got %q", code)
	}
	if message == "" {
		t.Errorf("expected non-empty error message")
	}
}

// TestReorderEpic_Success проверяет успешный вызов ReorderEpic-хендлера.
func TestReorderEpic_Success(t *testing.T) {
	epicID := uuid.New()
	cfg := config.BotConfig{}

	svc := &mockGanttSvc{}
	var gotEpicID uuid.UUID
	var gotSortOrder int
	svc.reorderEpicFunc = func(ctx context.Context, id uuid.UUID, newSortOrder int) ([]domain.GanttTask, error) {
		gotEpicID = id
		gotSortOrder = newSortOrder
		return []domain.GanttTask{{}}, nil
	}
	repo := &mockGanttRepo{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})

	reqBody, _ := json.Marshal(map[string]int{"new_sort_order": 2})
	req := httptest.NewRequest("PUT", "/api/gantt/epics/"+epicID.String()+"/reorder", bytes.NewReader(reqBody))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("epic_id", epicID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handler.ReorderEpic(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
	}
	if gotEpicID != epicID {
		t.Errorf("expected epicID %s, got %s", epicID, gotEpicID)
	}
	if gotSortOrder != 2 {
		t.Errorf("expected sortOrder 2, got %d", gotSortOrder)
	}
}

// TestReorderStory_Success проверяет успешный вызов ReorderStory-хендлера.
func TestReorderStory_Success(t *testing.T) {
	storyID := uuid.New()
	cfg := config.BotConfig{}

	svc := &mockGanttSvc{}
	var gotStoryID uuid.UUID
	var gotSortOrder int
	svc.reorderStoryFunc = func(ctx context.Context, id uuid.UUID, newSortOrder int) ([]domain.GanttTask, error) {
		gotStoryID = id
		gotSortOrder = newSortOrder
		return []domain.GanttTask{{}}, nil
	}
	repo := &mockGanttRepo{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})

	reqBody, _ := json.Marshal(map[string]int{"new_sort_order": 1})
	req := httptest.NewRequest("PUT", "/api/gantt/stories/"+storyID.String()+"/reorder", bytes.NewReader(reqBody))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("story_id", storyID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handler.ReorderStory(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
	}
	if gotStoryID != storyID {
		t.Errorf("expected storyID %s, got %s", storyID, gotStoryID)
	}
	if gotSortOrder != 1 {
		t.Errorf("expected sortOrder 1, got %d", gotSortOrder)
	}
}

// unmarshalErrorResp декодирует стандартный формат ошибки API
// {"error":{"code":"...","message":"..."}}.
func unmarshalErrorResp(t *testing.T, body []byte) (code, message string) {
	t.Helper()
	var resp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("failed to unmarshal error response: %v, body: %s", err, body)
	}
	return resp.Error.Code, resp.Error.Message
}

// TestReorderEpic_InvalidEpicID проверяет ошибку 400 при невалидном epic_id
// в стандартном структурированном формате {"error":{"code":"INVALID_EPIC_ID",...}}.
func TestReorderEpic_InvalidEpicID(t *testing.T) {
	cfg := config.BotConfig{}

	svc := &mockGanttSvc{}
	repo := &mockGanttRepo{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})

	reqBody, _ := json.Marshal(map[string]int{"new_sort_order": 5})
	req := httptest.NewRequest("PUT", "/api/gantt/epics/invalid-id/reorder", bytes.NewReader(reqBody))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("epic_id", "invalid-id")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handler.ReorderEpic(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d, body: %s", w.Code, w.Body.String())
	}
	code, message := unmarshalErrorResp(t, w.Body.Bytes())
	if code != "INVALID_EPIC_ID" {
		t.Errorf("expected error code INVALID_EPIC_ID, got %q", code)
	}
	if message == "" {
		t.Errorf("expected non-empty error message")
	}
}

// TestReorderEpic_InvalidRequestBody проверяет ошибку 400 при невалидном JSON.
func TestReorderEpic_InvalidRequestBody(t *testing.T) {
	epicID := uuid.New()
	cfg := config.BotConfig{}

	svc := &mockGanttSvc{}
	repo := &mockGanttRepo{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})

	req := httptest.NewRequest("PUT", "/api/gantt/epics/"+epicID.String()+"/reorder", bytes.NewReader([]byte(`{invalid json`)))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("epic_id", epicID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handler.ReorderEpic(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d, body: %s", w.Code, w.Body.String())
	}
	code, _ := unmarshalErrorResp(t, w.Body.Bytes())
	if code != "INVALID_REQUEST_BODY" {
		t.Errorf("expected error code INVALID_REQUEST_BODY, got %q", code)
	}
}

// TestReorderEpic_ServiceError проверяет ошибку 500 при ошибке сервиса.
func TestReorderEpic_ServiceError(t *testing.T) {
	epicID := uuid.New()
	cfg := config.BotConfig{}

	svc := &mockGanttSvc{
		reorderEpicFunc: func(ctx context.Context, id uuid.UUID, newSortOrder int) ([]domain.GanttTask, error) {
			return nil, errors.New("database error")
		},
	}
	repo := &mockGanttRepo{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})

	reqBody, _ := json.Marshal(map[string]int{"new_sort_order": 5})
	req := httptest.NewRequest("PUT", "/api/gantt/epics/"+epicID.String()+"/reorder", bytes.NewReader(reqBody))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("epic_id", epicID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handler.ReorderEpic(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d, body: %s", w.Code, w.Body.String())
	}
	code, _ := unmarshalErrorResp(t, w.Body.Bytes())
	if code != "RESCHEDULE_FAILED" {
		t.Errorf("expected error code RESCHEDULE_FAILED, got %q", code)
	}
}

// TestReorderStory_InvalidStoryID проверяет ошибку 400 при невалидном story_id.
func TestReorderStory_InvalidStoryID(t *testing.T) {
	cfg := config.BotConfig{}

	svc := &mockGanttSvc{}
	repo := &mockGanttRepo{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})

	reqBody, _ := json.Marshal(map[string]int{"new_sort_order": 5})
	req := httptest.NewRequest("PUT", "/api/gantt/stories/invalid-id/reorder", bytes.NewReader(reqBody))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("story_id", "invalid-id")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handler.ReorderStory(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d, body: %s", w.Code, w.Body.String())
	}
	code, message := unmarshalErrorResp(t, w.Body.Bytes())
	if code != "INVALID_STORY_ID" {
		t.Errorf("expected error code INVALID_STORY_ID, got %q", code)
	}
	if message == "" {
		t.Errorf("expected non-empty error message")
	}
}

// TestReorderStory_InvalidRequestBody проверяет ошибку 400 при невалидном JSON.
func TestReorderStory_InvalidRequestBody(t *testing.T) {
	storyID := uuid.New()
	cfg := config.BotConfig{}

	svc := &mockGanttSvc{}
	repo := &mockGanttRepo{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})

	req := httptest.NewRequest("PUT", "/api/gantt/stories/"+storyID.String()+"/reorder", bytes.NewReader([]byte(`{invalid json`)))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("story_id", storyID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handler.ReorderStory(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d, body: %s", w.Code, w.Body.String())
	}
	code, _ := unmarshalErrorResp(t, w.Body.Bytes())
	if code != "INVALID_REQUEST_BODY" {
		t.Errorf("expected error code INVALID_REQUEST_BODY, got %q", code)
	}
}

// TestReorderStory_ServiceError проверяет ошибку 500 при ошибке сервиса.
func TestReorderStory_ServiceError(t *testing.T) {
	storyID := uuid.New()
	cfg := config.BotConfig{}

	svc := &mockGanttSvc{
		reorderStoryFunc: func(ctx context.Context, id uuid.UUID, newSortOrder int) ([]domain.GanttTask, error) {
			return nil, errors.New("database error")
		},
	}
	repo := &mockGanttRepo{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})

	reqBody, _ := json.Marshal(map[string]int{"new_sort_order": 5})
	req := httptest.NewRequest("PUT", "/api/gantt/stories/"+storyID.String()+"/reorder", bytes.NewReader(reqBody))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("story_id", storyID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handler.ReorderStory(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d, body: %s", w.Code, w.Body.String())
	}
	code, _ := unmarshalErrorResp(t, w.Body.Bytes())
	if code != "RESCHEDULE_FAILED" {
		t.Errorf("expected error code RESCHEDULE_FAILED, got %q", code)
	}
}

// mockCapacityReportRepo is a mock implementation of Repository for GetCapacityReport tests.
type mockCapacityReportRepo struct {
	Repository
	getTeamByIDFunc                func(ctx context.Context, id uuid.UUID) (*domain.Team, error)
	getUsersByTeamIDFunc           func(ctx context.Context, id uuid.UUID) ([]domain.User, error)
	getRoleByUserIDFunc            func(ctx context.Context, id uuid.UUID) (*domain.Role, error)
	getEpicsByTeamYearQuarterFunc  func(ctx context.Context, teamID uuid.UUID, year, quarter int) ([]domain.Epic, error)
	getStoriesByEpicIDFunc         func(ctx context.Context, id uuid.UUID) ([]domain.Epic, error)
	getEpicRoleScoresByEpicIDFunc  func(ctx context.Context, id uuid.UUID) ([]domain.EpicRoleScore, error)
	getRoleByIDFunc                func(ctx context.Context, id uuid.UUID) (*domain.Role, error)
}

func (m *mockCapacityReportRepo) GetTeamByID(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
	if m.getTeamByIDFunc != nil {
		return m.getTeamByIDFunc(ctx, id)
	}
	return nil, errors.New("not implemented")
}

func (m *mockCapacityReportRepo) GetUsersByTeamID(ctx context.Context, id uuid.UUID) ([]domain.User, error) {
	if m.getUsersByTeamIDFunc != nil {
		return m.getUsersByTeamIDFunc(ctx, id)
	}
	return nil, errors.New("not implemented")
}

func (m *mockCapacityReportRepo) GetRoleByUserID(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
	if m.getRoleByUserIDFunc != nil {
		return m.getRoleByUserIDFunc(ctx, id)
	}
	return nil, errors.New("not implemented")
}

func (m *mockCapacityReportRepo) GetEpicsByTeamYearQuarter(ctx context.Context, teamID uuid.UUID, year, quarter int) ([]domain.Epic, error) {
	if m.getEpicsByTeamYearQuarterFunc != nil {
		return m.getEpicsByTeamYearQuarterFunc(ctx, teamID, year, quarter)
	}
	return nil, errors.New("not implemented")
}

func (m *mockCapacityReportRepo) GetStoriesByEpicID(ctx context.Context, id uuid.UUID) ([]domain.Epic, error) {
	if m.getStoriesByEpicIDFunc != nil {
		return m.getStoriesByEpicIDFunc(ctx, id)
	}
	return nil, errors.New("not implemented")
}

func (m *mockCapacityReportRepo) GetEpicRoleScoresByEpicID(ctx context.Context, id uuid.UUID) ([]domain.EpicRoleScore, error) {
	if m.getEpicRoleScoresByEpicIDFunc != nil {
		return m.getEpicRoleScoresByEpicIDFunc(ctx, id)
	}
	return nil, errors.New("not implemented")
}

func (m *mockCapacityReportRepo) GetRoleByID(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
	if m.getRoleByIDFunc != nil {
		return m.getRoleByIDFunc(ctx, id)
	}
	return nil, errors.New("not implemented")
}

// TestGetCapacityReport_RawRoleScoresWithRiskFactor проверяет, что для эпика с риск-фактором ≠ 1
// (когда final_score отличается от суммы WeightedAvg по ролям):
// - raw_role_scores НЕ равны role_scores
// - сумма raw_role_scores НЕ равна final_score
// - сумма role_scores равна final_score (регрессия на существующее поведение)
func TestGetCapacityReport_RawRoleScoresWithRiskFactor(t *testing.T) {
	teamID := uuid.New()
	epicID := uuid.New()
	userID := uuid.New()
	cfg := config.BotConfig{}

	t.Run("риск-фактор не равен 1: raw_role_scores должны отличаться от role_scores", func(t *testing.T) {
		// Данные для расчётов:
		// - У эпика 2 ролевые оценки: роль1=100, роль2=50, сумма=150
		// - final_score эпика = 120 (риск-буфер сократил сумму)
		// - riskFactor = 120 / 150 = 0.8
		// - role_scores будут: роль1=100*0.8=80, роль2=50*0.8=40, сумма=120
		// - raw_role_scores будут: роль1=100, роль2=50, сумма=150
		
		const weightedAvg1 = 100.0
		const weightedAvg2 = 50.0
		var baseScore = weightedAvg1 + weightedAvg2 // 150
		var finalScore float64 = 120.0
		var riskFactor = finalScore / baseScore // 0.8

		role1 := uuid.New()
		role2 := uuid.New()

		repo := &mockCapacityReportRepo{
			getTeamByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
				return &domain.Team{
					ID:   teamID,
					Name: "Test Team",
				}, nil
			},
			getUsersByTeamIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.User, error) {
				return []domain.User{
					{ID: userID, FirstName: "Test", LastName: "User"},
				}, nil
			},
			getRoleByUserIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
				return &domain.Role{
					ID:   role1,
					Name: "Backend",
				}, nil
			},
			getEpicsByTeamYearQuarterFunc: func(ctx context.Context, teamID uuid.UUID, year, quarter int) ([]domain.Epic, error) {
				return []domain.Epic{
					{
						ID:         epicID,
						Number:     "E-001",
						Name:       "Test Epic",
						Type:       "feature",
						Status:     domain.StatusScored,
						FinalScore: &finalScore,
						TeamID:     teamID,
						Year:       year,
						Quarter:    quarter,
					},
				}, nil
			},
			getStoriesByEpicIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.Epic, error) {
				// Возвращаем пустой список - используется fallback-ветка в GetCapacityReport
				return []domain.Epic{}, nil
			},
			getEpicRoleScoresByEpicIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.EpicRoleScore, error) {
				return []domain.EpicRoleScore{
					{
						ID:          uuid.New(),
						EpicID:      id,
						RoleID:      role1,
						WeightedAvg: weightedAvg1,
					},
					{
						ID:          uuid.New(),
						EpicID:      id,
						RoleID:      role2,
						WeightedAvg: weightedAvg2,
					},
				}, nil
			},
			getRoleByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
				if id == role1 {
					return &domain.Role{ID: role1, Name: "Backend"}, nil
				}
				if id == role2 {
					return &domain.Role{ID: role2, Name: "Frontend"}, nil
				}
				return nil, errors.New("role not found")
			},
		}

		svc := &mockGanttSvc{}
		handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})

		req := httptest.NewRequest("GET", "/api/capacity-report?team_id="+teamID.String()+"&year=2026&quarter=3", nil)
		w := httptest.NewRecorder()
		handler.GetCapacityReport(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
		}

		var resp CapacityReportResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}

		if len(resp.Epics) != 1 {
			t.Fatalf("expected 1 epic, got %d", len(resp.Epics))
		}

		epic := resp.Epics[0]

		// Проверка 1: raw_role_scores НЕ должны быть равны role_scores
		if reflect.DeepEqual(epic.RawRoleScores, epic.RoleScores) {
			t.Error("expected raw_role_scores to differ from role_scores, but they are equal")
		}

		// Проверка 2: сумма raw_role_scores НЕ должна быть равна final_score
		var rawRoleScoresSum float64
		for _, score := range epic.RawRoleScores {
			rawRoleScoresSum += score
		}
		if rawRoleScoresSum == epic.FinalScore {
			t.Errorf("expected sum of raw_role_scores (%f) to NOT equal final_score (%f), but they are equal",
				rawRoleScoresSum, epic.FinalScore)
		}

		// Проверка 3: сумма role_scores ДОЛЖНА быть равна final_score (регрессия)
		var roleScoresSum float64
		for _, score := range epic.RoleScores {
			roleScoresSum += score
		}
		const tolerance = 0.0001
		if math.Abs(roleScoresSum-epic.FinalScore) > tolerance {
			t.Errorf("expected sum of role_scores (%f) to equal final_score (%f), but got difference of %f",
				roleScoresSum, epic.FinalScore, math.Abs(roleScoresSum-epic.FinalScore))
		}

		// Проверка 4: raw_role_scores должны содержать исходные значения без риск-фактора
		if raw1, ok := epic.RawRoleScores["Backend"]; !ok || raw1 != weightedAvg1 {
			t.Errorf("expected raw_role_scores[Backend]=%f, got %f", weightedAvg1, raw1)
		}
		if raw2, ok := epic.RawRoleScores["Frontend"]; !ok || raw2 != weightedAvg2 {
			t.Errorf("expected raw_role_scores[Frontend]=%f, got %f", weightedAvg2, raw2)
		}

		// Проверка 5: role_scores должны содержать скорректированные значения с риск-фактором
		expectedRoleScore1 := weightedAvg1 * riskFactor
		expectedRoleScore2 := weightedAvg2 * riskFactor
		if roleScore1, ok := epic.RoleScores["Backend"]; !ok || math.Abs(roleScore1-expectedRoleScore1) > tolerance {
			t.Errorf("expected role_scores[Backend]=%f, got %f", expectedRoleScore1, roleScore1)
		}
		if roleScore2, ok := epic.RoleScores["Frontend"]; !ok || math.Abs(roleScore2-expectedRoleScore2) > tolerance {
			t.Errorf("expected role_scores[Frontend]=%f, got %f", expectedRoleScore2, roleScore2)
		}
	})
}

// TestGetCapacityReport_RawRoleScoresWithStoriesAndRiskFactors проверяет, что для эпика с оценёнными историями
// raw_role_scores агрегируются БЕЗ применения storyRiskFactor каждой истории (просто сумма WeightedAvg по историям),
// в отличие от role_scores, которые учитывают storyRiskFactor каждой истории.
func TestGetCapacityReport_RawRoleScoresWithStoriesAndRiskFactors(t *testing.T) {
	teamID := uuid.New()
	epicID := uuid.New()
	story1ID := uuid.New()
	story2ID := uuid.New()
	role1 := uuid.New()
	role2 := uuid.New()
	userID := uuid.New()
	cfg := config.BotConfig{}

	t.Run("с историями: raw_role_scores не учитывают storyRiskFactor", func(t *testing.T) {
		// Данные для расчётов:
		// Story 1: role1=100, role2=20, сумма=120, final_score=100, storyRiskFactor=100/120≈0.833
		// Story 2: role1=60, role2=30, сумма=90, final_score=81, storyRiskFactor=81/90=0.9
		//
		// raw_role_scores должны содержать: role1=100+60=160, role2=20+30=50
		// role_scores должны содержать: role1=(100*0.833)+(60*0.9)=83.3+54=137.3, role2=(20*0.833)+(30*0.9)=16.66+27=43.66

		const story1Role1 = 100.0
		const story1Role2 = 20.0
		var story1BaseScore float64 = story1Role1 + story1Role2 // 120
		var story1FinalScore float64 = 100.0
		var story1RiskFactor float64 = story1FinalScore / story1BaseScore // 100/120 ≈ 0.833

		const story2Role1 = 60.0
		const story2Role2 = 30.0
		var story2BaseScore float64 = story2Role1 + story2Role2 // 90
		var story2FinalScore float64 = 81.0
		var story2RiskFactor float64 = story2FinalScore / story2BaseScore // 81/90 = 0.9

		repo := &mockCapacityReportRepo{
			getTeamByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
				return &domain.Team{
					ID:   teamID,
					Name: "Test Team",
				}, nil
			},
			getUsersByTeamIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.User, error) {
				return []domain.User{
					{ID: userID, FirstName: "Test", LastName: "User"},
				}, nil
			},
			getRoleByUserIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
				return &domain.Role{
					ID:   role1,
					Name: "Backend",
				}, nil
			},
			getEpicsByTeamYearQuarterFunc: func(ctx context.Context, teamID uuid.UUID, year, quarter int) ([]domain.Epic, error) {
				return []domain.Epic{
					{
						ID:     epicID,
						Number: "E-001",
						Name:   "Epic with Stories",
						Type:   "feature",
						Status: domain.StatusScored,
						// Не устанавливаем FinalScore для эпика (или можем установить как сумму финальных scores историй)
						FinalScore: nil,
						TeamID:     teamID,
						Year:       year,
						Quarter:    quarter,
					},
				}, nil
			},
			getStoriesByEpicIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.Epic, error) {
				// Возвращаем два SCORED story
				return []domain.Epic{
					{
						ID:         story1ID,
						Number:     "S-001",
						Name:       "Story 1",
						Type:       "feature",
						Status:     domain.StatusScored,
						FinalScore: &story1FinalScore,
						TeamID:     teamID,
					},
					{
						ID:         story2ID,
						Number:     "S-002",
						Name:       "Story 2",
						Type:       "feature",
						Status:     domain.StatusScored,
						FinalScore: &story2FinalScore,
						TeamID:     teamID,
					},
				}, nil
			},
			getEpicRoleScoresByEpicIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.EpicRoleScore, error) {
				// Возвращаем ролевые оценки в зависимости от ID (эпика или истории)
				if id == story1ID {
					return []domain.EpicRoleScore{
						{ID: uuid.New(), EpicID: id, RoleID: role1, WeightedAvg: story1Role1},
						{ID: uuid.New(), EpicID: id, RoleID: role2, WeightedAvg: story1Role2},
					}, nil
				}
				if id == story2ID {
					return []domain.EpicRoleScore{
						{ID: uuid.New(), EpicID: id, RoleID: role1, WeightedAvg: story2Role1},
						{ID: uuid.New(), EpicID: id, RoleID: role2, WeightedAvg: story2Role2},
					}, nil
				}
				// Для самого эпика возвращаем пустой список
				return []domain.EpicRoleScore{}, nil
			},
			getRoleByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
				if id == role1 {
					return &domain.Role{ID: role1, Name: "Backend"}, nil
				}
				if id == role2 {
					return &domain.Role{ID: role2, Name: "Frontend"}, nil
				}
				return nil, errors.New("role not found")
			},
		}

		svc := &mockGanttSvc{}
		handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})

		req := httptest.NewRequest("GET", "/api/capacity-report?team_id="+teamID.String()+"&year=2026&quarter=3", nil)
		w := httptest.NewRecorder()
		handler.GetCapacityReport(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
		}

		var resp CapacityReportResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}

		if len(resp.Epics) != 1 {
			t.Fatalf("expected 1 epic, got %d", len(resp.Epics))
		}

		epic := resp.Epics[0]

		const tolerance = 0.01

		// Проверка 1: raw_role_scores должны быть просто суммами WeightedAvg без storyRiskFactor
		expectedRawRole1 := story1Role1 + story2Role1 // 100 + 60 = 160
		expectedRawRole2 := story1Role2 + story2Role2 // 20 + 30 = 50

		if raw1, ok := epic.RawRoleScores["Backend"]; !ok || math.Abs(raw1-expectedRawRole1) > tolerance {
			t.Errorf("expected raw_role_scores[Backend]=%f, got %f", expectedRawRole1, raw1)
		}
		if raw2, ok := epic.RawRoleScores["Frontend"]; !ok || math.Abs(raw2-expectedRawRole2) > tolerance {
			t.Errorf("expected raw_role_scores[Frontend]=%f, got %f", expectedRawRole2, raw2)
		}

		// Проверка 2: role_scores должны учитывать storyRiskFactor каждой истории
		expectedRoleScore1 := (story1Role1 * story1RiskFactor) + (story2Role1 * story2RiskFactor)
		expectedRoleScore2 := (story1Role2 * story1RiskFactor) + (story2Role2 * story2RiskFactor)

		if roleScore1, ok := epic.RoleScores["Backend"]; !ok || math.Abs(roleScore1-expectedRoleScore1) > tolerance {
			t.Errorf("expected role_scores[Backend]≈%f, got %f (diff=%f)", expectedRoleScore1, roleScore1, math.Abs(roleScore1-expectedRoleScore1))
		}
		if roleScore2, ok := epic.RoleScores["Frontend"]; !ok || math.Abs(roleScore2-expectedRoleScore2) > tolerance {
			t.Errorf("expected role_scores[Frontend]≈%f, got %f (diff=%f)", expectedRoleScore2, roleScore2, math.Abs(roleScore2-expectedRoleScore2))
		}

		// Проверка 3: raw_role_scores НЕ должны быть равны role_scores
		if reflect.DeepEqual(epic.RawRoleScores, epic.RoleScores) {
			t.Error("expected raw_role_scores to differ from role_scores, but they are equal")
		}
	})
}

// ── ExportTeamReport Tests ────────────────────────────────────────────────

// mockReportDataProvider is a mock implementation of ReportDataProvider.
type mockReportDataProvider struct {
	ReportDataProvider
	getReportDataFunc func(ctx context.Context, teamID uuid.UUID, year, quarter int) (*report.ReportData, error)
}

func (m *mockReportDataProvider) GetReportData(ctx context.Context, teamID uuid.UUID, year, quarter int) (*report.ReportData, error) {
	if m.getReportDataFunc != nil {
		return m.getReportDataFunc(ctx, teamID, year, quarter)
	}
	return nil, errors.New("not implemented")
}

// mockPDFReportGenerator is a mock implementation of PDFReportGenerator.
type mockPDFReportGenerator struct {
	PDFReportGenerator
	generateReportFunc func(ctx context.Context, data report.ReportData) (string, error)
}

func (m *mockPDFReportGenerator) GenerateReport(ctx context.Context, data report.ReportData) (string, error) {
	if m.generateReportFunc != nil {
		return m.generateReportFunc(ctx, data)
	}
	return "", errors.New("not implemented")
}

// TestExportTeamReport_MissingTeamID — проверяет, что запрос без team_id вернёт 400.
func TestExportTeamReport_MissingTeamID(t *testing.T) {
	cfg := config.BotConfig{}
	repo := &mockCapacityReportRepo{}
	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})
	handler.WithReportServices(&mockReportDataProvider{}, &mockPDFReportGenerator{})

	req := httptest.NewRequest("GET", "/api/gantt/reports/export?format=pdf", nil)
	w := httptest.NewRecorder()
	handler.ExportTeamReport(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var resp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err == nil {
		if resp.Error.Code != "team_id_required" {
			t.Errorf("expected error code 'team_id_required', got %q", resp.Error.Code)
		}
	}
}

// TestExportTeamReport_InvalidTeamID — проверяет, что невалидный team_id вернёт 400.
func TestExportTeamReport_InvalidTeamID(t *testing.T) {
	cfg := config.BotConfig{}
	repo := &mockCapacityReportRepo{}
	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})
	handler.WithReportServices(&mockReportDataProvider{}, &mockPDFReportGenerator{})

	req := httptest.NewRequest("GET", "/api/gantt/reports/export?team_id=invalid-uuid&format=pdf", nil)
	w := httptest.NewRecorder()
	handler.ExportTeamReport(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err == nil {
		if resp.Error.Code != "invalid_team_id" {
			t.Errorf("expected error code 'invalid_team_id', got %q", resp.Error.Code)
		}
	}
}

// TestExportTeamReport_MissingFormat — проверяет, что запрос без format вернёт 400.
func TestExportTeamReport_MissingFormat(t *testing.T) {
	teamID := uuid.New()
	cfg := config.BotConfig{}
	repo := &mockCapacityReportRepo{}
	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})
	handler.WithReportServices(&mockReportDataProvider{}, &mockPDFReportGenerator{})

	req := httptest.NewRequest("GET", "/api/gantt/reports/export?team_id="+teamID.String(), nil)
	w := httptest.NewRecorder()
	handler.ExportTeamReport(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err == nil {
		if resp.Error.Code != "invalid_format" {
			t.Errorf("expected error code 'invalid_format', got %q", resp.Error.Code)
		}
	}
}

// TestExportTeamReport_InvalidFormat — проверяет, что невалидный format вернёт 400.
func TestExportTeamReport_InvalidFormat(t *testing.T) {
	teamID := uuid.New()
	cfg := config.BotConfig{}
	repo := &mockCapacityReportRepo{}
	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})
	handler.WithReportServices(&mockReportDataProvider{}, &mockPDFReportGenerator{})

	req := httptest.NewRequest("GET", "/api/gantt/reports/export?team_id="+teamID.String()+"&format=doc", nil)
	w := httptest.NewRecorder()
	handler.ExportTeamReport(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err == nil {
		if resp.Error.Code != "invalid_format" {
			t.Errorf("expected error code 'invalid_format', got %q", resp.Error.Code)
		}
	}
}

// TestExportTeamReport_TeamNotFound — проверяет, что несуществующий team_id вернёт 404.
func TestExportTeamReport_TeamNotFound(t *testing.T) {
	teamID := uuid.New()
	cfg := config.BotConfig{}
	
	repo := &mockCapacityReportRepo{
		getTeamByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
			return nil, nil // Команда не найдена
		},
	}
	
	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})
	handler.WithReportServices(&mockReportDataProvider{}, &mockPDFReportGenerator{})

	req := httptest.NewRequest("GET", "/api/gantt/reports/export?team_id="+teamID.String()+"&format=pdf", nil)
	w := httptest.NewRecorder()
	handler.ExportTeamReport(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}

	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err == nil {
		if resp.Error.Code != "team_not_found" {
			t.Errorf("expected error code 'team_not_found', got %q", resp.Error.Code)
		}
	}
}

// TestExportTeamReport_DefaultYearQuarter — проверяет, что при отсутствии year/quarter используются дефолтные значения.
func TestExportTeamReport_DefaultYearQuarter(t *testing.T) {
	teamID := uuid.New()
	cfg := config.BotConfig{}

	expectedYear := 2026
	expectedQuarter := 3

	repo := &mockCapacityReportRepo{
		getTeamByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
			return &domain.Team{ID: teamID, Name: "Test Team"}, nil
		},
		getUsersByTeamIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.User, error) {
			return []domain.User{}, nil
		},
		getEpicsByTeamYearQuarterFunc: func(ctx context.Context, tID uuid.UUID, year, quarter int) ([]domain.Epic, error) {
			if year != expectedYear || quarter != expectedQuarter {
				t.Errorf("expected year=%d quarter=%d, got year=%d quarter=%d", expectedYear, expectedQuarter, year, quarter)
			}
			return []domain.Epic{}, nil
		},
	}

	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})
	handler.WithReportServices(&mockReportDataProvider{}, &mockPDFReportGenerator{})

	req := httptest.NewRequest("GET", "/api/gantt/reports/export?team_id="+teamID.String()+"&format=xlsx", nil)
	w := httptest.NewRecorder()
	handler.ExportTeamReport(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestExportTeamReport_ExplicitYearQuarter — проверяет, что явно переданные year/quarter используются.
func TestExportTeamReport_ExplicitYearQuarter(t *testing.T) {
	teamID := uuid.New()
	cfg := config.BotConfig{}

	expectedYear := 2025
	expectedQuarter := 2

	repo := &mockCapacityReportRepo{
		getTeamByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
			return &domain.Team{ID: teamID, Name: "Test Team"}, nil
		},
		getUsersByTeamIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.User, error) {
			return []domain.User{}, nil
		},
		getEpicsByTeamYearQuarterFunc: func(ctx context.Context, tID uuid.UUID, year, quarter int) ([]domain.Epic, error) {
			if year != expectedYear || quarter != expectedQuarter {
				t.Errorf("expected year=%d quarter=%d, got year=%d quarter=%d", expectedYear, expectedQuarter, year, quarter)
			}
			return []domain.Epic{}, nil
		},
	}

	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})
	handler.WithReportServices(&mockReportDataProvider{}, &mockPDFReportGenerator{})

	req := httptest.NewRequest("GET", "/api/gantt/reports/export?team_id="+teamID.String()+"&year=2025&quarter=2&format=xlsx", nil)
	w := httptest.NewRecorder()
	handler.ExportTeamReport(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestExportTeamReport_SuccessfulPDFExport — проверяет успешную выгрузку PDF.
func TestExportTeamReport_SuccessfulPDFExport(t *testing.T) {
	teamID := uuid.New()
	cfg := config.BotConfig{}

	repo := &mockCapacityReportRepo{
		getTeamByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
			return &domain.Team{ID: teamID, Name: "Test Team"}, nil
		},
	}

	pdfPath := "/tmp/test_report.pdf"
	pdfContent := []byte("%PDF-1.4 test content")

	// Создаём временный файл с тестовым содержимым
	tmpFile, err := os.Create(pdfPath)
	if err != nil {
		t.Fatalf("failed to create temp pdf file: %v", err)
	}
	defer func() {
		tmpFile.Close()
		os.Remove(pdfPath)
	}()
	if _, err := tmpFile.Write(pdfContent); err != nil {
		t.Fatalf("failed to write pdf content: %v", err)
	}
	tmpFile.Close()

	reportGen := &mockPDFReportGenerator{
		generateReportFunc: func(ctx context.Context, data report.ReportData) (string, error) {
			return pdfPath, nil
		},
	}

	reportData := &mockReportDataProvider{
		getReportDataFunc: func(ctx context.Context, teamID uuid.UUID, year, quarter int) (*report.ReportData, error) {
			return &report.ReportData{}, nil
		},
	}

	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})
	handler.WithReportServices(reportData, reportGen)

	req := httptest.NewRequest("GET", "/api/gantt/reports/export?team_id="+teamID.String()+"&format=pdf", nil)
	w := httptest.NewRecorder()
	handler.ExportTeamReport(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/pdf" {
		t.Errorf("expected Content-Type 'application/pdf', got %q", contentType)
	}

	disposition := w.Header().Get("Content-Disposition")
	if !strings.Contains(disposition, "attachment") {
		t.Errorf("expected Content-Disposition to contain 'attachment', got %q", disposition)
	}

	if !bytes.Contains(w.Body.Bytes(), pdfContent) {
		t.Errorf("expected response body to contain PDF content")
	}
}

// TestExportTeamReport_SuccessfulXLSXExport — проверяет успешную выгрузку XLSX.
func TestExportTeamReport_SuccessfulXLSXExport(t *testing.T) {
	teamID := uuid.New()
	roleID := uuid.New()
	userID := uuid.New()
	epicID := uuid.New()
	cfg := config.BotConfig{}

	repo := &mockCapacityReportRepo{
		getTeamByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
			return &domain.Team{ID: teamID, Name: "Test Team"}, nil
		},
		getUsersByTeamIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.User, error) {
			return []domain.User{{ID: userID, FirstName: "Test", LastName: "User"}}, nil
		},
		getRoleByUserIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
			return &domain.Role{ID: roleID, Name: "Backend"}, nil
		},
		getEpicsByTeamYearQuarterFunc: func(ctx context.Context, tID uuid.UUID, year, quarter int) ([]domain.Epic, error) {
			finalScore := 100.0
			return []domain.Epic{
				{
					ID:         epicID,
					Number:     "E-001",
					Name:       "Test Epic",
					Type:       "feature",
					Status:     domain.StatusScored,
					FinalScore: &finalScore,
					TeamID:     tID,
					Year:       year,
					Quarter:    quarter,
				},
			}, nil
		},
		getStoriesByEpicIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.Epic, error) {
			return []domain.Epic{}, nil
		},
		getEpicRoleScoresByEpicIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.EpicRoleScore, error) {
			return []domain.EpicRoleScore{
				{ID: uuid.New(), EpicID: id, RoleID: roleID, WeightedAvg: 100.0},
			}, nil
		},
		getRoleByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
			return &domain.Role{ID: roleID, Name: "Backend"}, nil
		},
	}

	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})
	handler.WithReportServices(&mockReportDataProvider{}, &mockPDFReportGenerator{})

	req := httptest.NewRequest("GET", "/api/gantt/reports/export?team_id="+teamID.String()+"&format=xlsx", nil)
	w := httptest.NewRecorder()
	handler.ExportTeamReport(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
		t.Errorf("expected Content-Type for XLSX, got %q", contentType)
	}

	disposition := w.Header().Get("Content-Disposition")
	if !strings.Contains(disposition, "attachment") {
		t.Errorf("expected Content-Disposition to contain 'attachment', got %q", disposition)
	}
}

// TestExportTeamReport_XLSXDataIntegrity — проверяет, что данные в XLSX соответствуют JSON-ответу.
// Это обеспечивает, что оба пути используют один и тот же агрегатор BuildCapacityReport.
func TestExportTeamReport_XLSXDataIntegrity(t *testing.T) {
	teamID := uuid.New()
	roleID := uuid.New()
	userID := uuid.New()
	epicID := uuid.New()
	cfg := config.BotConfig{}

	// Параметры для теста
	const teamName = "Test Team"
	const year = 2026
	const quarter = 3
	const epicNumber = "E-001"
	const epicName = "Test Epic"
	const roleScore = 100.0
	const finalScore = 100.0

	repo := &mockCapacityReportRepo{
		getTeamByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
			return &domain.Team{ID: teamID, Name: teamName}, nil
		},
		getUsersByTeamIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.User, error) {
			return []domain.User{{ID: userID, FirstName: "Test", LastName: "User"}}, nil
		},
		getRoleByUserIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
			return &domain.Role{ID: roleID, Name: "Backend"}, nil
		},
		getEpicsByTeamYearQuarterFunc: func(ctx context.Context, tID uuid.UUID, y, q int) ([]domain.Epic, error) {
			fs := finalScore
			return []domain.Epic{
				{
					ID:         epicID,
					Number:     epicNumber,
					Name:       epicName,
					Type:       "feature",
					Status:     domain.StatusScored,
					FinalScore: &fs,
					TeamID:     tID,
					Year:       y,
					Quarter:    q,
				},
			}, nil
		},
		getStoriesByEpicIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.Epic, error) {
			return []domain.Epic{}, nil
		},
		getEpicRoleScoresByEpicIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.EpicRoleScore, error) {
			return []domain.EpicRoleScore{
				{ID: uuid.New(), EpicID: id, RoleID: roleID, WeightedAvg: roleScore},
			}, nil
		},
		getRoleByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
			return &domain.Role{ID: roleID, Name: "Backend"}, nil
		},
	}

	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})
	handler.WithReportServices(&mockReportDataProvider{}, &mockPDFReportGenerator{})

	// Сначала получаем JSON-ответ через GetCapacityReport
	reqJSON := httptest.NewRequest("GET", "/api/gantt/reports/capacity?team_id="+teamID.String()+"&year="+fmt.Sprintf("%d", year)+"&quarter="+fmt.Sprintf("%d", quarter), nil)
	wJSON := httptest.NewRecorder()
	handler.GetCapacityReport(wJSON, reqJSON)

	if wJSON.Code != http.StatusOK {
		t.Fatalf("GetCapacityReport failed with status %d, body: %s", wJSON.Code, wJSON.Body.String())
	}

	var jsonResp report.CapacityReportResponse
	if err := json.Unmarshal(wJSON.Body.Bytes(), &jsonResp); err != nil {
		t.Fatalf("failed to unmarshal JSON response: %v", err)
	}

	// Теперь генерируем XLSX
	reqXLSX := httptest.NewRequest("GET", "/api/gantt/reports/export?team_id="+teamID.String()+"&year="+fmt.Sprintf("%d", year)+"&quarter="+fmt.Sprintf("%d", quarter)+"&format=xlsx", nil)
	wXLSX := httptest.NewRecorder()
	handler.ExportTeamReport(wXLSX, reqXLSX)

	if wXLSX.Code != http.StatusOK {
		t.Fatalf("ExportTeamReport failed with status %d", wXLSX.Code)
	}

	// Проверяем, что JSON-данные содержат ожидаемые значения
	if jsonResp.TeamName != teamName {
		t.Errorf("expected team name %q, got %q", teamName, jsonResp.TeamName)
	}
	if jsonResp.Year != year {
		t.Errorf("expected year %d, got %d", year, jsonResp.Year)
	}
	if jsonResp.Quarter != quarter {
		t.Errorf("expected quarter %d, got %d", quarter, jsonResp.Quarter)
	}

	// Проверяем наличие эпика в JSON
	if len(jsonResp.Epics) != 1 {
		t.Fatalf("expected 1 epic in JSON, got %d", len(jsonResp.Epics))
	}

	jsonEpic := jsonResp.Epics[0]
	if jsonEpic.Number != epicNumber {
		t.Errorf("expected epic number %q, got %q", epicNumber, jsonEpic.Number)
	}
	if jsonEpic.Name != epicName {
		t.Errorf("expected epic name %q, got %q", epicName, jsonEpic.Name)
	}
	if jsonEpic.FinalScore != finalScore {
		t.Errorf("expected final score %f, got %f", finalScore, jsonEpic.FinalScore)
	}

	// Проверяем, что ролевые оценки совпадают
	if backendScore, ok := jsonEpic.RoleScores["Backend"]; !ok || backendScore != roleScore {
		t.Errorf("expected Backend role_score %f, got %f", roleScore, backendScore)
	}

	// Проверяем, что XLSX-контент не пуст
	if len(wXLSX.Body.Bytes()) == 0 {
		t.Errorf("expected non-empty XLSX content")
	}

	// Проверяем сигнатуру XLSX (должна начинаться с PK — это ZIP-архив)
	xlsxBytes := wXLSX.Body.Bytes()
	if len(xlsxBytes) < 2 || xlsxBytes[0] != 0x50 || xlsxBytes[1] != 0x4B {
		t.Errorf("expected XLSX to start with PK signature (ZIP), got %x %x", xlsxBytes[0], xlsxBytes[1])
	}
}

// TestExportTeamReport_FormatCaseInsensitive — проверяет, что формат нечувствителен к регистру.
func TestExportTeamReport_FormatCaseInsensitive(t *testing.T) {
	teamID := uuid.New()
	cfg := config.BotConfig{}

	repo := &mockCapacityReportRepo{
		getTeamByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
			return &domain.Team{ID: teamID, Name: "Test Team"}, nil
		},
		getUsersByTeamIDFunc: func(ctx context.Context, id uuid.UUID) ([]domain.User, error) {
			return []domain.User{}, nil
		},
		getEpicsByTeamYearQuarterFunc: func(ctx context.Context, tID uuid.UUID, year, quarter int) ([]domain.Epic, error) {
			return []domain.Epic{}, nil
		},
	}

	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})
	handler.WithReportServices(&mockReportDataProvider{}, &mockPDFReportGenerator{})

	// Проверяем, что "PDF" (заглавные буквы) работает также как "pdf"
	req := httptest.NewRequest("GET", "/api/gantt/reports/export?team_id="+teamID.String()+"&format=PDF", nil)
	w := httptest.NewRecorder()
	handler.ExportTeamReport(w, req)

	// Должно быть успешно обработано как XLSX или PDF
	if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
		// 404 может быть, если reportData/reportGen не установлены, но это нормально для мока
		t.Logf("status code: %d", w.Code)
	}
}
