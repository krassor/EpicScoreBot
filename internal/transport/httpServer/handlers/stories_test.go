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

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type mockStoriesRepo struct {
	Repository
	GetEpicByIDFunc          func(ctx context.Context, id uuid.UUID) (*domain.Epic, error)
	CountStoriesByEpicIDFunc func(ctx context.Context, pid uuid.UUID) (int, error)
	CreateStoryFunc          func(ctx context.Context, parentID uuid.UUID, number, name, description string, tID uuid.UUID, year, quarter int, epicType string, evaluatingRoleIDs []uuid.UUID) (*domain.Epic, error)
	DeleteEpicFunc           func(ctx context.Context, id uuid.UUID) error
	GetStoriesByEpicIDFunc   func(ctx context.Context, epicID uuid.UUID) ([]domain.Epic, error)
}

func (m *mockStoriesRepo) GetEpicByID(ctx context.Context, id uuid.UUID) (*domain.Epic, error) {
	if m.GetEpicByIDFunc != nil {
		return m.GetEpicByIDFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockStoriesRepo) CountStoriesByEpicID(ctx context.Context, pid uuid.UUID) (int, error) {
	if m.CountStoriesByEpicIDFunc != nil {
		return m.CountStoriesByEpicIDFunc(ctx, pid)
	}
	return 0, nil
}

func (m *mockStoriesRepo) CreateStory(ctx context.Context, parentID uuid.UUID, number, name, description string, tID uuid.UUID, year, quarter int, epicType string, evaluatingRoleIDs []uuid.UUID) (*domain.Epic, error) {
	if m.CreateStoryFunc != nil {
		return m.CreateStoryFunc(ctx, parentID, number, name, description, tID, year, quarter, epicType, evaluatingRoleIDs)
	}
	return nil, nil
}

func (m *mockStoriesRepo) DeleteEpic(ctx context.Context, id uuid.UUID) error {
	if m.DeleteEpicFunc != nil {
		return m.DeleteEpicFunc(ctx, id)
	}
	return nil
}

func (m *mockStoriesRepo) GetStoriesByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.Epic, error) {
	if m.GetStoriesByEpicIDFunc != nil {
		return m.GetStoriesByEpicIDFunc(ctx, epicID)
	}
	return nil, nil
}

func TestGanttHandler_CreateStory(t *testing.T) {
	parentID := uuid.New()
	teamID := uuid.New()
	cfg := config.BotConfig{}

	t.Run("успешное создание стори", func(t *testing.T) {
		repo := &mockStoriesRepo{
			GetEpicByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Epic, error) {
				return &domain.Epic{
					ID:           parentID,
					Number:       "E-100",
					Status:       domain.StatusNew,
					TeamID:       teamID,
					Year:         2026,
					Quarter:      3,
					Type:         "feature",
				}, nil
			},
			CountStoriesByEpicIDFunc: func(ctx context.Context, pid uuid.UUID) (int, error) {
				return 2, nil
			},
			CreateStoryFunc: func(ctx context.Context, parentID uuid.UUID, number, name, description string, tID uuid.UUID, year, quarter int, epicType string, evaluatingRoleIDs []uuid.UUID) (*domain.Epic, error) {
				return &domain.Epic{
					ID:           uuid.New(),
					Number:       number,
					Name:         name,
					Description:  description,
					ParentEpicID: &parentID,
				}, nil
			},
		}

		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, &mockScoringService{}, &mockAIClient{}, cfg)

		reqBody, _ := json.Marshal(map[string]string{
			"name":        "Story 3",
			"description": "Desc 3",
		})
		req := httptest.NewRequest("POST", "/api/gantt/epics/"+parentID.String()+"/stories", bytes.NewReader(reqBody))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("epic_id", parentID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		w := httptest.NewRecorder()
		handler.CreateStory(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("expected status 201, got %d, body: %s", w.Code, w.Body.String())
		}

		var resp domain.Epic
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		if resp.Number != "E-100-S3" {
			t.Errorf("expected number E-100-S3, got %s", resp.Number)
		}
	})

	t.Run("ошибка создания — родитель не в NEW", func(t *testing.T) {
		repo := &mockStoriesRepo{
			GetEpicByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Epic, error) {
				return &domain.Epic{
					ID:     parentID,
					Number: "E-100",
					Status: domain.StatusScoring,
				}, nil
			},
		}

		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, &mockScoringService{}, &mockAIClient{}, cfg)

		reqBody, _ := json.Marshal(map[string]string{
			"name": "Story 3",
		})
		req := httptest.NewRequest("POST", "/api/gantt/epics/"+parentID.String()+"/stories", bytes.NewReader(reqBody))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("epic_id", parentID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		w := httptest.NewRecorder()
		handler.CreateStory(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", w.Code)
		}
	})
}

func TestGanttHandler_DeleteStory(t *testing.T) {
	storyID := uuid.New()
	parentID := uuid.New()
	cfg := config.BotConfig{}

	t.Run("успешное удаление", func(t *testing.T) {
		deleteCalled := false
		repo := &mockStoriesRepo{
			GetEpicByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Epic, error) {
				if id == storyID {
					return &domain.Epic{
						ID:           storyID,
						ParentEpicID: &parentID,
					}, nil
				}
				return &domain.Epic{
					ID:     parentID,
					Status: domain.StatusNew,
				}, nil
			},
			DeleteEpicFunc: func(ctx context.Context, id uuid.UUID) error {
				if id == storyID {
					deleteCalled = true
				}
				return nil
			},
		}

		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, &mockScoringService{}, &mockAIClient{}, cfg)

		req := httptest.NewRequest("DELETE", "/api/gantt/stories/"+storyID.String(), nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("story_id", storyID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		w := httptest.NewRecorder()
		handler.DeleteStory(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}
		if !deleteCalled {
			t.Error("expected delete to be called")
		}
	})

	t.Run("ошибка удаления — родитель не в NEW", func(t *testing.T) {
		repo := &mockStoriesRepo{
			GetEpicByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Epic, error) {
				if id == storyID {
					return &domain.Epic{
						ID:           storyID,
						ParentEpicID: &parentID,
					}, nil
				}
				return &domain.Epic{
					ID:     parentID,
					Status: domain.StatusScoring,
				}, nil
			},
			DeleteEpicFunc: func(ctx context.Context, id uuid.UUID) error {
				return errors.New("should not be called")
			},
		}

		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, &mockScoringService{}, &mockAIClient{}, cfg)

		req := httptest.NewRequest("DELETE", "/api/gantt/stories/"+storyID.String(), nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("story_id", storyID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		w := httptest.NewRecorder()
		handler.DeleteStory(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", w.Code)
		}
	})
}
