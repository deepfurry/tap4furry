package migrate

import (
	"io"
	"io/fs"
	"log/slog"
	"os"
	"testing"

	"github.com/deepfurry/tap4furry/server/db"
	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

// There is deliberately no production/shared-development down function or flag.
func TestIntegrationResourceMigrationRoundTrip(t *testing.T) {
	if os.Getenv("GFP_RESOURCE_INTEGRATION") != "1" {
		t.Skip("explicit disposable resource integration not enabled")
	}
	if os.Getenv("CI") != "true" || os.Getenv("GFP_DISPOSABLE_INFRA") != "1" {
		t.Fatal("disposable CI guards required")
	}
	ctx := t.Context()
	pool, err := database.Open(ctx, "postgres://gfp_migrator:gfp_ci_only@127.0.0.1:5432/gfp_ci?sslmode=disable")
	if err != nil {
		t.Fatal("disposable migration connection failed")
	}
	defer pool.Close()
	if _, err := database.Inspect(ctx, pool, "gfp_migrator", "gfp_ci"); err != nil {
		t.Fatal("disposable migration identity mismatch")
	}
	dbSQL := stdlib.OpenDBFromPool(pool)
	defer dbSQL.Close()
	source, err := fs.Sub(db.Migrations, "migrations")
	if err != nil {
		t.Fatal("migration source unavailable")
	}
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		t.Fatal("migration lock unavailable")
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, dbSQL, source,
		goose.WithTableName("app.goose_db_version"), goose.WithSessionLocker(locker),
		goose.WithDisableGlobalRegistry(true), goose.WithSlog(slog.New(slog.NewTextHandler(io.Discard, nil))))
	if err != nil {
		t.Fatal("migration provider unavailable")
	}
	current, target, err := provider.GetVersions(ctx)
	if err != nil || current != 8 || target != 8 {
		t.Fatal("round-trip only accepts exactly migration 8")
	}
	completeContributionRoundTrip(t, provider, pool)
	if _, err := provider.Down(ctx); err != nil {
		t.Fatal(database.SafeError("disposable 00007 down", err))
	}
	version, err := provider.GetDBVersion(ctx)
	if err != nil || version != 6 {
		t.Fatal("00007 down did not stop at version 6")
	}
	var preserved bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('app.resources') IS NOT NULL AND to_regclass('app.contributions') IS NULL AND to_regclass('app.contribution_contents') IS NULL AND to_regclass('app.contribution_initial_sources') IS NULL AND to_regclass('app.contribution_events') IS NULL AND to_regclass('app.contribution_review_audits') IS NULL`).Scan(&preserved); err != nil || !preserved {
		t.Fatal("00007 down did not preserve Resource Core or remove all contribution tables")
	}
	if _, err := provider.Down(ctx); err != nil {
		t.Fatal(database.SafeError("disposable 00006 down", err))
	}
	version, err = provider.GetDBVersion(ctx)
	if err != nil || version != 5 {
		t.Fatal("00006 down did not stop at version 5")
	}
	var resourceTables int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_tables WHERE schemaname='app' AND tablename IN
		('categories','category_localizations','tags','tag_localizations','resources','resource_localizations','resource_tags','resource_sources','resource_relations','resource_external_ids')`).Scan(&resourceTables); err != nil || resourceTables != 0 {
		t.Fatal("00006 down retained resource tables")
	}
	var authPreserved bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('app.users') IS NOT NULL AND to_regclass('app.sessions') IS NOT NULL AND to_regclass('app.user_roles') IS NOT NULL`).Scan(&authPreserved); err != nil || !authPreserved {
		t.Fatal("00006 down affected P0-1")
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(database.SafeError("disposable 00006 up", err))
	}
	version, err = provider.GetDBVersion(ctx)
	if err != nil || version != 8 {
		t.Fatal("00006/00007/00008 reapply failed")
	}
	var categories int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM app.categories").Scan(&categories); err != nil || categories != 0 {
		t.Fatal("migration unexpectedly seeded taxonomy")
	}
}
