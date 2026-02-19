package graceful

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type Operation func(ctx context.Context) error

// GracefulShutdown waits for termination syscalls and doing clean up operations after received it.
func GracefulShutdown(ctx context.Context, timeout time.Duration, ops map[string]Operation, logger *slog.Logger) <-chan struct{} {
	op := "GracefulShutdown()"
	log := logger.With(
		slog.String("op", op))

	wait := make(chan struct{})
	go func() {
		s := make(chan os.Signal, 1)
		signal.Notify(s, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
		<-s

		log.Info("shutting down")

		ctxTimeout, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		var wg sync.WaitGroup

		for key, op := range ops {
			wg.Add(1)
			innerOp := op
			innerKey := key
			go func() {
				defer wg.Done()

				log.Info("cleaning up: ", slog.String("process", innerKey))
				if err := innerOp(ctxTimeout); err != nil {
					log.Error("error clean up", slog.String("process", innerKey), slog.String("error", err.Error()))
					return
				}

				log.Info("shutdown gracefully", slog.String("process", innerKey))
			}()
		}

		wg.Wait()
		log.Info("graceful shutdown completed")

		close(wait)
	}()

	return wait
}
