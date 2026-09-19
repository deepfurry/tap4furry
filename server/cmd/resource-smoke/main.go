package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/deepfurry/tap4furry/server/internal/database/resourcecheck"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	expected := flag.String("database", "gfp_dev", "expected resource smoke database")
	flag.Parse()
	if err := run(*expected); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(expected string) (result error) {
	if expected != "gfp_dev" && expected != "gfp_ci" {
		return errors.New("resource smoke target must be dev or disposable CI")
	}
	if expected == "gfp_dev" && os.Getenv("CI") != "" {
		return errors.New("shared resource smoke is forbidden in CI")
	}
	if expected == "gfp_ci" && (os.Getenv("CI") != "true" || os.Getenv("GFP_DISPOSABLE_INFRA") != "1") {
		return errors.New("resource smoke requires disposable guards")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	pools := map[string]*pgxpool.Pool{}
	for _, role := range []string{"api", "admin", "worker", "migrator", "readonly"} {
		raw := os.Getenv("RESOURCE_SMOKE_" + strings.ToUpper(role) + "_DATABASE_URL")
		if expected == "gfp_ci" {
			u, err := url.Parse(raw)
			if err != nil || u.Hostname() != "127.0.0.1" || u.Port() != "5432" || u.Path != "/gfp_ci" {
				return errors.New("CI resource smoke requires fixed loopback target")
			}
		}
		pool, err := database.Open(ctx, raw)
		if err != nil {
			return err
		}
		defer pool.Close()
		if _, err = database.Inspect(ctx, pool, "gfp_"+role, expected); err != nil {
			return err
		}
		pools[role] = pool
	}
	fixture := resourcecheck.NewFixture()
	defer func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		if err := fixture.Cleanup(cleanupCtx, pools["migrator"]); err != nil {
			result = errors.Join(result, err)
		} else {
			fmt.Println("Resource fixture cleanup PASS")
		}
	}()
	if err := fixture.Create(ctx, pools["admin"]); err != nil {
		return err
	}
	if err := fixture.ExerciseMutation(ctx, pools["admin"]); err != nil {
		return err
	}
	fmt.Println("Resource fixture, localizations, relation and endpoint CAS PASS")
	for _, role := range []string{"api", "admin", "worker", "readonly"} {
		if err := resourcecheck.VerifyRole(ctx, pools[role], role, fixture); err != nil {
			return err
		}
		fmt.Println("gfp_" + role + " Resource Core privilege checks PASS")
	}
	return nil
}
