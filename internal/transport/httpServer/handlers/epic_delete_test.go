package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"EpicScoreBot/internal/config"
	"EpicScoreBot/internal/models/domain"
	"EpicScoreBot/internal/transport/httpServer/middleware"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"log/slog"
)

// TestGanttHandler_DeleteEpic проверяет поведение хендлера DeleteEpic
// (openspec/changes/add-epic-delete-button, п. 1.3): успешное удаление,
// невалидный epic_id и ошибку репозитория. mockStoriesRepo уже реализует
// DeleteEpic (см. stories_test.go) — переиспользуем его здесь.
func TestGanttHandler_DeleteEpic(t *testing.T) {
	cfg := config.BotConfig{}

	t.Run("успешное удаление", func(t *testing.T) {
		epicID := uuid.New()
		deletedID := uuid.Nil
		repo := &mockStoriesRepo{
			DeleteEpicFunc: func(ctx context.Context, id uuid.UUID) error {
				deletedID = id
				return nil
			},
		}
		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})

		req := httptest.NewRequest("DELETE", "/api/gantt/epics/"+epicID.String(), nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("epic_id", epicID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		w := httptest.NewRecorder()
		handler.DeleteEpic(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
		}
		if deletedID != epicID {
			t.Errorf("expected DeleteEpic called with %s, got %s", epicID, deletedID)
		}

		var resp map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		if resp["status"] != "ok" {
			t.Errorf("expected status ok, got %+v", resp)
		}
	})

	t.Run("невалидный epic_id", func(t *testing.T) {
		repo := &mockStoriesRepo{
			DeleteEpicFunc: func(ctx context.Context, id uuid.UUID) error {
				t.Fatal("DeleteEpic не должен вызываться при невалидном epic_id")
				return nil
			},
		}
		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})

		req := httptest.NewRequest("DELETE", "/api/gantt/epics/not-a-uuid", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("epic_id", "not-a-uuid")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		w := httptest.NewRecorder()
		handler.DeleteEpic(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("ошибка репозитория", func(t *testing.T) {
		epicID := uuid.New()
		repo := &mockStoriesRepo{
			DeleteEpicFunc: func(ctx context.Context, id uuid.UUID) error {
				return errors.New("db connection lost")
			},
		}
		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})

		req := httptest.NewRequest("DELETE", "/api/gantt/epics/"+epicID.String(), nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("epic_id", epicID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		w := httptest.NewRecorder()
		handler.DeleteEpic(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("expected status 500, got %d, body: %s", w.Code, w.Body.String())
		}

		var resp map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		if resp["error"] == "" {
			t.Errorf("expected non-empty error message, got %+v", resp)
		}
	})
}

// TestDeleteEpicRoute_RequireSuperAdmin проверяет уровень доступа маршрута
// DELETE /api/gantt/epics/{epic_id} (openspec/changes/add-epic-delete-button,
// п. 1.4): доступ разрешён только роли superadmin, для admin/leader/member —
// 403 в стандартном для RoleAuth формате {"error":"forbidden"}. По аналогии с
// TestTeamAdminRoutes_RequireSuperAdmin (team_admin_test.go) реальный
// middleware.RoleAuth оборачивает хендлер напрямую — это эквивалент проверки
// маршрута из superadmin-группы routers.go.
func TestDeleteEpicRoute_RequireSuperAdmin(t *testing.T) {
	cfg := config.BotConfig{SuperAdmins: []string{"root"}}
	epicID := uuid.New()

	doRequest := func(t *testing.T, mw http.Handler, telegramID, username string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest("DELETE", "/api/gantt/epics/"+epicID.String(), nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("epic_id", epicID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserSessionKey,
			&middleware.UserSession{TelegramID: telegramID, Username: username}))
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)
		return rr
	}

	t.Run("superadmin получает 200", func(t *testing.T) {
		// mockRepository не реализует DeleteEpic — используем
		// mockStoriesRepo (см. stories_test.go), чтобы дойти до самого
		// хендлера; UserFinder/TeamAdminChecker для RoleAuth у него тоже
		// есть через встроенный Repository (для superadmin не вызываются).
		storiesRepo := &mockStoriesRepo{}
		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, storiesRepo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})
		mw := middleware.RoleAuth(storiesRepo, storiesRepo, cfg, "superadmin")(http.HandlerFunc(handler.DeleteEpic))

		rr := doRequest(t, mw, "1", "root")
		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 for superadmin, got %d, body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("admin (team-admin команды) получает 403", func(t *testing.T) {
		repo := &mockRepository{teamAdminOfAny: true}
		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})
		mw := middleware.RoleAuth(repo, repo, cfg, "superadmin")(http.HandlerFunc(handler.DeleteEpic))

		rr := doRequest(t, mw, "2", "team_admin_user")
		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403 for admin, got %d, body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("leader (обычный участник команды с доменной ролью IT-лидер) получает 403", func(t *testing.T) {
		// RoleAuth не различает доменные роли (IT-лидер, аналитик и т.д.) —
		// с точки зрения middleware это обычный member, не team-admin и не
		// superadmin, поэтому доступ также запрещён.
		repo := &mockRepository{
			users: map[string]*domain.User{
				"leader_user": {ID: uuid.New(), TelegramID: "leader_user"},
			},
		}
		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})
		mw := middleware.RoleAuth(repo, repo, cfg, "superadmin")(http.HandlerFunc(handler.DeleteEpic))

		rr := doRequest(t, mw, "leader_user", "leader_user")
		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403 for leader, got %d, body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("member получает 403", func(t *testing.T) {
		repo := &mockRepository{
			users: map[string]*domain.User{
				"member_user": {ID: uuid.New(), TelegramID: "member_user"},
			},
		}
		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})
		mw := middleware.RoleAuth(repo, repo, cfg, "superadmin")(http.HandlerFunc(handler.DeleteEpic))

		rr := doRequest(t, mw, "member_user", "member_user")
		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403 for member, got %d, body: %s", rr.Code, rr.Body.String())
		}
	})
}
