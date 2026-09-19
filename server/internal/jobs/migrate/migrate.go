package migrate

import (
	"context"
	"errors"
	"log/slog"

	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

// Migrate uses official River SQL and explicit schema substitution. Never called
// at runtime startup. Advisory locking spans the independently committed steps.
func Up(ctx context.Context, pool *pgxpool.Pool, schema string, logger *slog.Logger) error {
	if schema != "river" {
		return errors.New("River schema must be river")
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return database.SafeError("River migration lock connection", err)
	}
	defer conn.Release()
	// Session lock is released by closing the dedicated connection, even on cancel.
	defer conn.Conn().Close(context.Background())
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock(715606301234)"); err != nil {
		return database.SafeError("River migration lock", err)
	}
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), &rivermigrate.Config{Schema: schema, Logger: logger})
	if err != nil {
		return database.SafeError("River migrator", err)
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return database.SafeError("official River migrate-up", err)
	}
	// Grants are limited to River v0.47.0 runtime objects. No role/cluster changes,
	// no migration-table writes, schema CREATE, ownership or Public/Admin enqueue.
	_, err = pool.Exec(ctx, `
		REVOKE ALL ON SCHEMA river FROM PUBLIC;
		REVOKE ALL ON ALL TABLES IN SCHEMA river FROM PUBLIC;
		REVOKE ALL ON ALL SEQUENCES IN SCHEMA river FROM PUBLIC;
		REVOKE ALL ON ALL FUNCTIONS IN SCHEMA river FROM PUBLIC;
		GRANT USAGE ON SCHEMA river TO gfp_worker;
		GRANT SELECT, INSERT, UPDATE, DELETE ON river.river_job, river.river_leader,
			river.river_queue, river.river_notification TO gfp_worker;
		GRANT USAGE ON SEQUENCE river.river_job_id_seq, river.river_notification_id_seq TO gfp_worker;
		GRANT EXECUTE ON FUNCTION river.river_job_state_in_bitmask(bit, river.river_job_state) TO gfp_worker;
	`)
	if err != nil {
		return database.SafeError("River runtime object grants", err)
	}
	return nil
}
