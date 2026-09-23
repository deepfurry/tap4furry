package governancecheck

import (
	"context"
	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/deepfurry/tap4furry/server/internal/database/curationcheck"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type Fixture struct {
	Reports, Users []string
	Graph          curationcheck.Fixture
}

func (f *Fixture) Cleanup(pool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return database.SafeError("begin governance fixture cleanup", err)
	}
	defer tx.Rollback(ctx)
	operations := []struct {
		sql  string
		args []any
	}{
		{"DELETE FROM app.report_events WHERE report_id=ANY($1::uuid[])", []any{f.Reports}},
		{"DELETE FROM app.audit_entries WHERE user_id=ANY($1::uuid[]) OR moderation_action_id IN (SELECT id FROM app.moderation_actions WHERE report_id=ANY($2::uuid[]))", []any{f.Users, f.Reports}},
		{"DELETE FROM app.moderation_actions WHERE user_id=ANY($1::uuid[]) OR report_id=ANY($2::uuid[])", []any{f.Users, f.Reports}},
		{"DELETE FROM app.reports WHERE id=ANY($1::uuid[])", []any{f.Reports}},
		{"DELETE FROM app.user_restrictions WHERE user_id=ANY($1::uuid[])", []any{f.Users}},
		{"DELETE FROM app.user_governance_profiles WHERE user_id=ANY($1::uuid[])", []any{f.Users}},
	}
	for _, op := range operations {
		if _, err = tx.Exec(ctx, op.sql, op.args...); err != nil {
			return database.SafeError("remove owned governance fixture", err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return database.SafeError("commit governance cleanup", err)
	}
	return f.Graph.Cleanup(pool)
}
