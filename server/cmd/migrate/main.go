package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/deepfurry/tap4furry/server/internal/config"
	"github.com/deepfurry/tap4furry/server/internal/database"
	dbmigrate "github.com/deepfurry/tap4furry/server/internal/database/migrate"
	jobmigrate "github.com/deepfurry/tap4furry/server/internal/jobs/migrate"
	platformruntime "github.com/deepfurry/tap4furry/server/internal/runtime"
)

func main() {
	expectedDB := flag.String("database", "gfp_dev", "expected repository database")
	flag.Parse()
	if err := run(*expectedDB); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(expectedDB string) error {
	if expectedDB != "gfp_dev" && expectedDB != "gfp_ci" {
		return errors.New("migration target must be gfp_dev or disposable gfp_ci")
	}
	c, err := config.Load("migrator")
	if err != nil {
		return err
	}
	if expectedDB == "gfp_dev" && (os.Getenv("CI") != "" || c.Environment != "development") {
		return errors.New("shared development migrations require a local development launch")
	}
	logger := platformruntime.Logger("migrator", c.Environment)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := database.Open(ctx, c.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	capabilities, err := database.Inspect(ctx, pool, "gfp_migrator", expectedDB)
	if err != nil {
		return err
	}
	if capabilities.Vector {
		return errors.New("vector is enabled; stop for infrastructure conflict")
	}
	if err := dbmigrate.Up(ctx, pool, platformruntime.DependencyLogger(logger, "goose")); err != nil {
		return err
	}
	logger.Info("Goose up passed")
	if err := jobmigrate.Up(ctx, pool, c.RiverSchema, platformruntime.DependencyLogger(logger, "river-migrate")); err != nil {
		return err
	}
	logger.Info("official River migrate-up and worker grants passed", "schema", "river")
	return nil
}
