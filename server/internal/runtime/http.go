package runtime

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"time"

	"github.com/deepfurry/tap4furry/server/internal/transport/health"
	"github.com/gofiber/fiber/v3"
)

func HTTP(ctx context.Context, app *fiber.App, address string, checker *health.Checker, logger *slog.Logger) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return errors.New("HTTP listen failed (details withheld)")
	}
	defer listener.Close()
	done := make(chan error, 1)
	go func() { done <- app.Listener(listener, fiber.ListenConfig{DisableStartupMessage: true}) }()
	logger.Info("HTTP started")
	select {
	case err := <-done:
		if err != nil {
			return errors.New("HTTP server stopped unexpectedly (details withheld)")
		}
		return nil
	case <-ctx.Done():
		checker.Stop()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := app.ShutdownWithContext(shutdownCtx); err != nil {
			return errors.New("HTTP shutdown timed out or failed")
		}
		select {
		case <-done:
			logger.Info("HTTP stopped")
			return nil
		case <-shutdownCtx.Done():
			return errors.New("HTTP listener did not stop within deadline")
		}
	}
}
