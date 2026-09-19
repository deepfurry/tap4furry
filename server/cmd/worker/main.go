package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/deepfurry/tap4furry/server/internal/config"
	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/deepfurry/tap4furry/server/internal/jobs"
	"github.com/deepfurry/tap4furry/server/internal/redisstore"
	platformruntime "github.com/deepfurry/tap4furry/server/internal/runtime"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	c, err := config.Load("worker")
	if err != nil {
		return err
	}
	logger := platformruntime.Logger("worker", c.Environment)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	defer platformruntime.ShutdownDeadline(ctx, logger)()
	workCtx, cancelWork := context.WithCancel(context.Background())
	defer cancelWork()
	pool, err := database.Open(workCtx, c.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	store, err := redisstore.Open(c.RedisURL, c.RedisKeyPrefix)
	if err != nil {
		return err
	}
	defer store.Close()
	startupCtx, cancelStartup := context.WithTimeout(workCtx, 10*time.Second)
	err = database.Ready(startupCtx, pool)
	if err == nil && store.Ping(startupCtx) != nil {
		logger.Warn("Redis degraded")
	}
	cancelStartup()
	if err != nil {
		return err
	}
	worker, err := jobs.New(pool, c.RiverSchema, logger)
	if err != nil {
		return err
	}
	if err := worker.Start(workCtx); err != nil {
		return err
	}
	logger.Info("worker started")
	<-ctx.Done()
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := worker.Stop(shutdownCtx); err != nil {
		cancelWork()
		return err
	}
	logger.Info("worker stopped")
	return nil
}
