package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/deepfurry/tap4furry/server/internal/database/publicreadcheck"
	"github.com/deepfurry/tap4furry/server/internal/transport/public"
	"github.com/gofiber/fiber/v3"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() (result error) {
	if os.Getenv("CI") != "" {
		return errors.New("shared public read smoke is forbidden in CI")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	api, err := database.Open(ctx, os.Getenv("PUBLIC_READ_API_DATABASE_URL"))
	if err != nil {
		return err
	}
	defer api.Close()
	owner, err := database.Open(ctx, os.Getenv("PUBLIC_READ_MIGRATOR_DATABASE_URL"))
	if err != nil {
		return err
	}
	defer owner.Close()
	if _, err = database.Inspect(ctx, api, "gfp_api", "gfp_dev"); err != nil {
		return err
	}
	if _, err = database.Inspect(ctx, owner, "gfp_migrator", "gfp_dev"); err != nil {
		return err
	}
	var version int
	if err = owner.QueryRow(ctx, "SELECT version_id FROM app.goose_db_version ORDER BY id DESC LIMIT 1").Scan(&version); err != nil || version != 7 {
		return errors.New("public read smoke requires current Goose version 7")
	}
	f := publicreadcheck.NewFixture()
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		if err := f.Cleanup(cleanup, owner); err != nil {
			result = errors.Join(result, err)
		} else {
			fmt.Println("Public read fixture cleanup PASS")
		}
	}()
	if err = f.Create(ctx, owner); err != nil {
		return err
	}
	app := fiber.New()
	public.Register(app, nil, nil, nil, public.Options{ResourcePool: api})
	if err = f.Verify(app); err != nil {
		return err
	}
	fmt.Println("Four anonymous Public endpoints, visibility, locale, Source/Relation privacy, pagination and cache PASS")
	fmt.Println("Serialized Public DTO privacy and current Goose version 7 PASS")
	return nil
}
