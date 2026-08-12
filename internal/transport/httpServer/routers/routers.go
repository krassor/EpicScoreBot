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
	mux.Handle("/gantt/*", http.StripPrefix("/gantt/", fs))

	mux.Route("/api", func(mux chi.Router) {
		mux.Route("/gantt", func(mux chi.Router) {
			// Public: Telegram Login callback.
			mux.Get("/auth", r.ganttHandler.TelegramAuth)
			mux.Post("/auth/webapp", r.ganttHandler.TelegramWebAppAuth)

			// Protected: require Telegram auth.
			mux.Group(func(mux chi.Router) {
				mux.Use(myMiddleware.TelegramAuth(r.botToken))

				mux.Get("/profile", r.ganttHandler.GetProfile)
				mux.Get("/roles", r.ganttHandler.GetRoles)
				mux.Get("/reports/capacity", r.ganttHandler.GetCapacityReport)

				// Admin endpoints
				mux.Post("/teams", r.ganttHandler.CreateTeam)
				mux.Post("/epics", r.ganttHandler.AddEpic)
				mux.Post("/risks", r.ganttHandler.AddRisk)
				mux.Post("/users/bulk", r.ganttHandler.BulkCreateUsers)

				// User management (Admin only)
				mux.Group(func(mux chi.Router) {
					mux.Use(myMiddleware.RoleAuth(r.ganttHandler.Repo(), r.ganttHandler.Config(), "admin"))
					mux.Get("/admin/users", r.ganttHandler.GetUsersList)
					mux.Get("/admin/users/{id}", r.ganttHandler.GetUserDetails)
					mux.Post("/admin/users", r.ganttHandler.CreateSingleUser)
					mux.Put("/admin/users/{id}", r.ganttHandler.UpdateUser)

					// Admin override scores
					mux.Post("/admin/scores/epic", r.ganttHandler.AdminSubmitEpicScore)
					mux.Post("/admin/scores/risk", r.ganttHandler.AdminSubmitRiskScore)

					// Admin editing epics/stories
					mux.Put("/epics/{epic_id}", r.ganttHandler.UpdateEpic)
					mux.Put("/stories/{story_id}", r.ganttHandler.UpdateStory)
				})


				// Scoring endpoints
				mux.Post("/epics/start", r.ganttHandler.StartEpicScoring)
				mux.Post("/scores/epic", r.ganttHandler.SubmitEpicScore)
				mux.Post("/scores/risk", r.ganttHandler.SubmitRiskScore)
				mux.Get("/scores/my", r.ganttHandler.GetMyScores)
				
				mux.Get("/epics/{epic_id}/scores", r.ganttHandler.GetEpicScores)
				mux.Get("/epics/{epic_id}/role-scores", r.ganttHandler.GetEpicRoleScores)
				mux.Get("/epics/{epic_id}/risks", r.ganttHandler.GetEpicRisks)
				mux.Get("/epics/{epic_id}/stories", r.ganttHandler.GetStories)
				mux.Post("/epics/{epic_id}/stories", r.ganttHandler.CreateStory)
				mux.Delete("/stories/{story_id}", r.ganttHandler.DeleteStory)

				// AI chat endpoint
				mux.Post("/ask-ai", r.ganttHandler.AskAI)

				// Existing Gantt routes
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
