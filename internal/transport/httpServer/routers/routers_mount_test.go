package routers

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"EpicScoreBot/internal/config"
	"EpicScoreBot/internal/transport/httpServer/handlers"

	"github.com/go-chi/chi/v5"
)

// TestMount_NoPanic проверяет, что регистрация всех маршрутов (в т.ч. нового
// DELETE /epics/{epic_id} рядом с уже существующим PUT /epics/{epic_id})
// не приводит к панике chi на конфликте имени параметра пути. Сами хендлеры
// не вызываются — тест проверяет только построение дерева маршрутов.
func TestMount_NoPanic(t *testing.T) {
	h := handlers.NewGanttHandler(slog.Default(), nil, nil, nil, nil, config.BotConfig{}, nil)
	router := NewRouter(h, "test-token")

	mux := chi.NewMux()

	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("Mount() запаниковал: %v", rec)
		}
	}()
	router.Mount(mux)
}

// TestMount_TaskAssigneeAndTeamMembersAreProtected проверяет (backend §3.4,
// openspec/changes/add-gantt-task-assignees, и backend §3.3,
// openspec/changes/backfill-idle-gaps-in-schedule), что маршруты
// PUT /api/gantt/tasks/{id}/assignee, GET /api/gantt/teams/{id}/members и
// GET|PUT /api/gantt/teams/{id}/schedule-settings смонтированы и закрыты
// аутентификацией: запрос без cookie сессии TelegramAuth возвращает 401, а
// не 404 (маршрут не существует) — ни middleware.TelegramAuth, ни (для PUT)
// следующий за ним middleware.RoleAuth не обращаются к handler/service/repo
// зависимостям хендлера при отсутствии валидной сессии, поэтому
// nil-зависимости из TestMount_NoPanic безопасны и здесь.
func TestMount_TaskAssigneeAndTeamMembersAreProtected(t *testing.T) {
	h := handlers.NewGanttHandler(slog.Default(), nil, nil, nil, nil, config.BotConfig{}, nil)
	router := NewRouter(h, "test-token")

	mux := chi.NewMux()
	router.Mount(mux)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	cases := []struct {
		name   string
		method string
		path   string
	}{
		{"PUT /tasks/{id}/assignee without session", http.MethodPut, "/api/gantt/tasks/00000000-0000-0000-0000-000000000000/assignee"},
		{"GET /teams/{id}/members without session", http.MethodGet, "/api/gantt/teams/00000000-0000-0000-0000-000000000000/members"},
		{"GET /teams/{id}/schedule-settings without session", http.MethodGet, "/api/gantt/teams/00000000-0000-0000-0000-000000000000/schedule-settings"},
		{"PUT /teams/{id}/schedule-settings without session", http.MethodPut, "/api/gantt/teams/00000000-0000-0000-0000-000000000000/schedule-settings"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, srv.URL+tc.path, nil)
			if err != nil {
				t.Fatalf("failed to build request: %v", err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("expected 401 (route mounted and gated by auth), got %d", resp.StatusCode)
			}
		})
	}
}
