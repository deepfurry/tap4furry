// ci-setup provisions disposable roles/database only on a fresh loopback CI server.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/redis/go-redis/v9"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	if os.Getenv("GFP_DISPOSABLE_INFRA") != "1" || os.Getenv("CI") != "true" {
		return errors.New("disposable CI guard is required")
	}
	raw := os.Getenv("CI_POSTGRES_URL")
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() != "127.0.0.1" || u.Path != "/postgres" {
		return errors.New("CI setup requires loopback disposable postgres database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, raw)
	if err != nil {
		return err
	}
	defer pool.Close()
	// Fixed disposable password is intentionally public; never used outside CI.
	for _, statement := range []string{
		"CREATE ROLE gfp_migrator LOGIN PASSWORD 'gfp_ci_only' NOSUPERUSER NOCREATEDB NOCREATEROLE",
		"CREATE ROLE gfp_api LOGIN PASSWORD 'gfp_ci_only' NOSUPERUSER NOCREATEDB NOCREATEROLE",
		"CREATE ROLE gfp_admin LOGIN PASSWORD 'gfp_ci_only' NOSUPERUSER NOCREATEDB NOCREATEROLE",
		"CREATE ROLE gfp_worker LOGIN PASSWORD 'gfp_ci_only' NOSUPERUSER NOCREATEDB NOCREATEROLE",
		"CREATE ROLE gfp_readonly LOGIN PASSWORD 'gfp_ci_only' NOSUPERUSER NOCREATEDB NOCREATEROLE",
		"CREATE DATABASE gfp_ci OWNER gfp_migrator",
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			return database.SafeError("fresh disposable CI provisioning", err)
		}
	}
	u.Path = "/gfp_ci"
	target, err := database.Open(ctx, u.String())
	if err != nil {
		return err
	}
	defer target.Close()
	// CREATE in public permits the trusted pg_trgm extension in a fresh database.
	if _, err := target.Exec(ctx, "GRANT CREATE ON SCHEMA public TO gfp_migrator"); err != nil {
		return database.SafeError("disposable extension capability", err)
	}
	// This admin connection exists only in the guarded disposable fixture.
	cache := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379", DisableIdentity: true})
	defer cache.Close()
	if err := cache.Do(ctx, "ACL", "SETUSER", "gfp_runtime", "reset", "on", ">gfp_ci_only", "~gfp:*", "+ping", "+get", "+getdel", "+set", "+del", "+hello", "+eval", "+incr", "+expire", "+ttl").Err(); err != nil {
		return errors.New("disposable Redis ACL fixture failed (details withheld)")
	}
	fmt.Println("Fresh disposable CI roles and database prepared")
	return nil
}
