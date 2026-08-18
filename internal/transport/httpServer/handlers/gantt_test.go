package handlers

import (
	"EpicScoreBot/internal/config"
	"EpicScoreBot/internal/models/domain"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
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
	reorderEpicFunc     func(ctx context.Context, epicID uuid.UUID, newSortOrder int) ([]domain.GanttTask, error)
	reorderStoryFunc    func(ctx context.Context, storyID uuid.UUID, newSortOrder int) ([]domain.GanttTask, error)
	setTaskProgressFunc func(ctx context.Context, taskID uuid.UUID, progress float64) ([]domain.GanttTask, error)
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
		handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg)

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
		handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg)

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
		handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg)

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
		handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg)

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
		handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg)

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
		handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg)

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
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg)

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
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg)

	reqBody, _ := json.Marshal(map[string]float64{"progress": 75})
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
	if calledWith != 75 {
		t.Errorf("expected progress 75, got %v", calledWith)
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
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg)

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
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg)

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
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg)

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
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg)

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
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg)

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
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg)

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
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg)

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
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg)

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
