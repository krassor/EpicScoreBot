package routers

import (
	"net/http"

	"EpicScoreBot/internal/transport/httpServer/handlers"
	myMiddleware "EpicScoreBot/internal/transport/httpServer/middleware"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// Router holds all HTTP handlers and mounts them onto a chi mux.
type Router struct {
	ganttHandler *handlers.GanttHandler
	botToken     string
}

// NewRouter creates a new Router.
func NewRouter(ganttHandler *handlers.GanttHandler, botToken string) *Router {
	return &Router{
		ganttHandler: ganttHandler,
		botToken:     botToken,
	}
}

// Mount registers all routes and middleware on the given chi mux.
func (r *Router) Mount(mux *chi.Mux) {
	mux.Use(cors.AllowAll().Handler)
	mux.Use(myMiddleware.LoggerMiddleware)
	mux.Use(middleware.Heartbeat("/ping"))

	// Serve static files for the Gantt web UI.
	fs := http.FileServer(http.Dir("web/gantt"))
	mux.Get("/gantt", http.RedirectHandler("/gantt/", http.StatusMovedPermanently).ServeHTTP)
	mux.Handle("/gantt/*", http.StripPrefix("/gantt/", fs))

	mux.Route("/api", func(mux chi.Router) {
		mux.Route("/gantt", func(mux chi.Router) {
			// Public: Telegram Login callback.
			mux.Get("/auth", r.ganttHandler.TelegramAuth)

			// Protected: require Telegram auth.
			mux.Group(func(mux chi.Router) {
				mux.Use(myMiddleware.TelegramAuth(r.botToken))

				mux.Get("/teams", r.ganttHandler.GetTeams)
				mux.Get("/epics", r.ganttHandler.GetEpics)
				mux.Get("/tasks", r.ganttHandler.GetTasks)
				mux.Post("/tasks/generate", r.ganttHandler.GenerateTasks)

				mux.Route("/tasks/{id}", func(mux chi.Router) {
					mux.Put("/", r.ganttHandler.UpdateTask)
					mux.Put("/reorder", r.ganttHandler.ReorderTask)
					mux.Delete("/", r.ganttHandler.DeleteTask)
				})
			})
		})
	})
}
