package migrate

import (
	"context"
	"io/fs"
	"log/slog"

	"github.com/deepfurry/tap4furry/server/db"
	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

// Migrate is only called by the explicit migrator. pgx's database/sql bridge is
// required by Goose's official provider API; runtime queries use pgxpool directly.
func Up(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
	// Goose needs its history table's namespace before it can run migration 1.
	// The prepared migrator cannot CREATE in public, so keep bookkeeping in app.
	if _, err := pool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS app"); err != nil {
		return database.SafeError("Goose bookkeeping namespace", err)
	}
	dbSQL := stdlib.OpenDBFromPool(pool)
	defer dbSQL.Close()
	migrations, err := fs.Sub(db.Migrations, "migrations")
	if err != nil {
		return database.SafeError("load Goose migrations", err)
	}
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return database.SafeError("Goose lock configuration", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, dbSQL, migrations,
		goose.WithTableName("app.goose_db_version"), goose.WithSessionLocker(locker),
		goose.WithDisableGlobalRegistry(true), goose.WithSlog(logger))
	if err != nil {
		return database.SafeError("Goose provider", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return database.SafeError("Goose up", err)
	}
	// Shared default privileges for future app tables must not let runtime roles
	// rewrite migration history. This is an owned-object grant, not a role change.
	if _, err := pool.Exec(ctx, "REVOKE ALL ON app.goose_db_version FROM PUBLIC, gfp_api, gfp_admin, gfp_worker"); err != nil {
		return database.SafeError("Goose history permissions", err)
	}
	return nil
}
