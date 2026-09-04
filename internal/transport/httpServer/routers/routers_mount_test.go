package routers

import (
	"log/slog"
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
