package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/deepfurry/tap4furry/server/internal/config"
	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/deepfurry/tap4furry/server/internal/jobs"
	"github.com/deepfurry/tap4furry/server/internal/redisstore"
	platformruntime "github.com/deepfurry/tap4furry/server/internal/runtime"
)

func main() {
	service := flag.String("service", "api", "prepared service role")
	expectedDB := flag.String("database", "gfp_dev", "expected repository database")
	flag.Parse()
	if err := run(*service, *expectedDB); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(service, expectedDB string) error {
	if expectedDB != "gfp_dev" && expectedDB != "gfp_ci" {
		return errors.New("smoke target must be gfp_dev or disposable gfp_ci")
	}
	if expectedDB == "gfp_dev" && os.Getenv("CI") != "" {
		return errors.New("CI cannot smoke shared development Infra")
	}
	c, err := config.Load(service)
	if err != nil {
		return err
	}
	logger := platformruntime.Logger("smoke-"+service, c.Environment)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, c.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	capabilities, err := database.Inspect(ctx, pool, "gfp_"+service, expectedDB)
	if err != nil {
		return err
	}
	if !capabilities.AppExists || !capabilities.RiverExists || !capabilities.Trigram || capabilities.Vector {
		return errors.New("database schema/extension acceptance failed")
	}
	if err := database.Ready(ctx, pool); err != nil {
		return err
	}
	logger.Info("pgx identity, schema, extension and sqlc checks passed")
	if service == "migrator" {
		return nil
	}
	redisURL, err := url.Parse(c.RedisURL)
	if err != nil || redisURL.User == nil || redisURL.User.Username() != "gfp_runtime" {
		return errors.New("Redis smoke requires prepared gfp_runtime identity")
	}
	store, err := redisstore.Open(c.RedisURL, c.RedisKeyPrefix)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.Smoke(ctx); err != nil {
		return err
	}
	logger.Info("Redis PING and gfp: SET/GET/DEL passed")
	if service == "worker" {
		worker, err := jobs.New(pool, c.RiverSchema, logger)
		if err != nil {
			return err
		}
		workCtx, cancelWork := context.WithCancel(context.Background())
		defer cancelWork()
		if err := worker.Start(workCtx); err != nil {
			return err
		}
		probeErr := worker.Probe(ctx)
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelShutdown()
		stopErr := worker.Stop(shutdownCtx)
		cancelWork()
		if probeErr != nil {
			return probeErr
		}
		if stopErr != nil {
			return stopErr
		}
		logger.Info("River enqueue, execution, completion, cleanup and shutdown passed", "schema", "river")
	}
	return nil
}
