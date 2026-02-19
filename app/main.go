package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"EpicScoreBot/internal/config"
	"EpicScoreBot/internal/graceful"
	"EpicScoreBot/internal/repositories"
	"EpicScoreBot/internal/scoring"
	"EpicScoreBot/internal/telegram"
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
	tgBot := telegram.New(log, cfg, repositoryService, scoringService)

	maxSecond := 15 * time.Second
	waitShutdown := graceful.GracefulShutdown(
		context.Background(),
		maxSecond,
		map[string]graceful.Operation{
			"Repository service": func(ctx context.Context) error {
				return repositoryService.Shutdown(ctx)
			},
			"Telegram bot": func(ctx context.Context) error {
				return tgBot.Shutdown(ctx)
			},
		},
		log,
	)

	go tgBot.Start(30)

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
