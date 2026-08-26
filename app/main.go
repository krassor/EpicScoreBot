package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"EpicScoreBot/internal/ai"
	"EpicScoreBot/internal/config"
	"EpicScoreBot/internal/gantt"
	"EpicScoreBot/internal/graceful"
	"EpicScoreBot/internal/report"
	"EpicScoreBot/internal/repositories"
	"EpicScoreBot/internal/scoring"
	"EpicScoreBot/internal/services"
	"EpicScoreBot/internal/telegram"
	httpServer "EpicScoreBot/internal/transport/httpServer"
	"EpicScoreBot/internal/transport/httpServer/handlers"
	"EpicScoreBot/internal/transport/httpServer/routers"
	"EpicScoreBot/internal/utils/logger/handlers/slogpretty"
)

const (
	envLocal = "local"
	envDev   = "dev"
	envProd  = "prod"
)

var Version = "0.1"

func main() {
	cfg := config.MustLoad()

	log := setupLogger(cfg.Env)

	log.Info(
		"starting epic score bot",
		slog.String("env", cfg.Env),
		slog.String("version", Version),
	)

	repositoryService := repositories.New(log, cfg)
	scoringService := scoring.New(log, repositoryService)

	// Initialize business services
	userService := services.NewUserService(log, repositoryService)
	teamService := services.NewTeamService(log, repositoryService)
	epicService := services.NewEpicService(log, repositoryService)
	riskService := services.NewRiskService(log, repositoryService)
	roleService := services.NewRoleService(log, repositoryService)
	teamAdminService := services.NewTeamAdminService(log, repositoryService)

	// ai.New may return nil when AI is disabled. We must pass a nil interface
	// (not a typed-nil pointer) so that telegram's epicBot.ai == nil check works.
	var aiClient telegram.AIClient
	if c := ai.New(log, cfg, repositoryService); c != nil {
		aiClient = c
	}

	reportService := report.NewGenerator(log, cfg)

	tgBot := telegram.New(
		log,
		cfg,
		userService,
		teamService,
		epicService,
		riskService,
		roleService,
		teamAdminService,
		scoringService,
		reportService,
		aiClient,
	)
	if tgBot == nil {
		log.Error("failed to initialize telegram bot. the app will continue running without telegram features.")
	}

	// Gantt chart service and HTTP server.
	// teamAdminAuth оборачивает repositoryService, добавляя telegram_id-
	// ориентированные проверки team-admin (см. repositories.TeamAdminAuth) —
	// используется вместо repositoryService везде, где HTTP-слою нужен
	// Repository, т.к. промоутит все его методы и одновременно
	// удовлетворяет middleware.TeamAdminChecker/handlers.TeamAdminScoper.
	teamAdminAuth := repositories.NewTeamAdminAuth(repositoryService)
	ganttService := gantt.New(log, repositoryService)
	// tgBot может быть typed-nil (см. проверку выше), поэтому передаём его в
	// GanttHandler как handlers.TelegramNotifier только если бот успешно
	// инициализирован — иначе рассылка напоминаний из веб-панели будет
	// недоступна (h.notifier == nil), без риска nil-panic в SendDirectMessage.
	var notifier handlers.TelegramNotifier
	if tgBot != nil {
		notifier = tgBot
	}
	ganttHandler := handlers.NewGanttHandler(log, ganttService, teamAdminAuth, scoringService, aiClient, cfg.BotConfig, notifier)
	router := routers.NewRouter(ganttHandler, cfg.BotConfig.TgbotApiToken)
	server := httpServer.NewHttpServer(log, router, cfg)

	operations := map[string]graceful.Operation{
		"Repository service": func(ctx context.Context) error {
			return repositoryService.Shutdown(ctx)
		},
		"HTTP server": func(ctx context.Context) error {
			return server.Shutdown(ctx)
		},
	}
	if tgBot != nil {
		operations["Telegram bot"] = func(ctx context.Context) error {
			return tgBot.Shutdown(ctx)
		}
	}

	maxSecond := 15 * time.Second
	waitShutdown := graceful.GracefulShutdown(
		context.Background(),
		maxSecond,
		operations,
		log,
	)

	if tgBot != nil {
		go tgBot.Start(30)
	}
	go server.Listen()

	<-waitShutdown
}

func setupLogger(env string) *slog.Logger {
	var log *slog.Logger

	switch env {
	case envLocal:
		log = setupPrettySlog(slog.LevelDebug)
	case envDev:
		log = slog.New(
			slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}),
		)
	case envProd:
		log = setupPrettySlog(slog.LevelInfo)
	default:
		log = slog.New(
			slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}),
		)
	}

	return log
}

func setupPrettySlog(level slog.Level) *slog.Logger {
	opts := slogpretty.PrettyHandlerOptions{
		SlogOpts: &slog.HandlerOptions{
			Level: level,
		},
	}
	handler := opts.NewPrettyHandler(os.Stdout)
	return slog.New(handler)
}
