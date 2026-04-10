package httpServer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"EpicScoreBot/internal/config"
	"EpicScoreBot/internal/transport/httpServer/routers"

	"github.com/go-chi/chi/v5"
)

// HttpServer wraps a net/http server with chi routing.
type HttpServer struct {
	router     *routers.Router
	httpServer *http.Server
	cfg        *config.Config
	log        *slog.Logger
}

// NewHttpServer creates a new HttpServer.
func NewHttpServer(log *slog.Logger, router *routers.Router, cfg *config.Config) *HttpServer {
	return &HttpServer{
		router: router,
		cfg:    cfg,
		log:    log,
	}
}

// Listen starts the HTTP server and blocks until it is closed.
func (h *HttpServer) Listen() {
	op := "httpServer.Listen()"
	h.log.With(slog.String("op", op))

	mux := chi.NewRouter()
	h.router.Mount(mux)

	serverPort := h.cfg.HttpServer.Port
	serverAddress := h.cfg.HttpServer.Address

	h.httpServer = &http.Server{
		Addr:    fmt.Sprintf("%s:%s", serverAddress, serverPort),
		Handler: mux,
	}

	h.log.Info("http server starts on",
		slog.String("address", serverAddress),
		slog.String("port", serverPort))

	err := h.httpServer.ListenAndServe()

	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		h.log.Error("error start httpServer",
			slog.String("error", err.Error()))
	}

	if errors.Is(err, http.ErrServerClosed) {
		h.log.Info("httpServer closed")
	}
}

// Shutdown gracefully stops the HTTP server.
func (h *HttpServer) Shutdown(ctx context.Context) error {
	if h.httpServer == nil {
		return nil
	}
	return h.httpServer.Shutdown(ctx)
}
