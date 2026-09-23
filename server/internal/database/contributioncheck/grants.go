package contributioncheck

import (
	"context"
	"errors"
	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/jackc/pgx/v5/pgxpool"
	"slices"
)

// VerifyGrants checks every column, including privileges inherited through roles.
func VerifyGrants(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx, `SELECT role,c.relname,a.attname,
 has_column_privilege(role,c.oid,a.attnum,'SELECT'),has_column_privilege(role,c.oid,a.attnum,'INSERT'),has_column_privilege(role,c.oid,a.attnum,'UPDATE'),
 has_column_privilege(role,c.oid,a.attnum,'REFERENCES') OR has_table_privilege(role,c.oid,'DELETE,TRUNCATE,TRIGGER,MAINTAIN')
 FROM unnest(ARRAY['gfp_api','gfp_admin','gfp_worker','gfp_readonly']) role
 CROSS JOIN pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace JOIN pg_attribute a ON a.attrelid=c.oid
 WHERE n.nspname='app' AND c.relname IN ('contributions','contribution_contents','contribution_initial_sources','contribution_events','contribution_review_audits') AND a.attnum>0 AND NOT a.attisdropped`)
	if err != nil {
		return database.SafeError("inspect contribution grants", err)
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var role, table, column string
		var read, insert, update, other bool
		if err = rows.Scan(&role, &table, &column, &read, &insert, &update, &other); err != nil {
			return database.SafeError("read contribution grants", err)
		}
		seen[table] = true
		wantRead := role == "gfp_readonly" || role == "gfp_admin" || role == "gfp_api" && table != "contribution_review_audits" && (table != "contribution_events" || slices.Contains([]string{"contribution_id", "event_type", "message", "occurred_at"}, column))
		wantInsert := role == "gfp_admin" && table != "contributions"
		if role == "gfp_api" {
			switch table {
			case "contributions":
				wantInsert = slices.Contains([]string{"id", "author_id", "kind", "target_resource_id", "base_version", "reason", "previous_id", "request_id", "request_fingerprint", "submitted_fields"}, column)
			case "contribution_contents", "contribution_initial_sources":
				wantInsert = true
			case "contribution_events":
				wantInsert = column != "internal_note"
			}
		}
		wantUpdate := table == "contributions" && (role == "gfp_api" && slices.Contains([]string{"status", "decided_at"}, column) || role == "gfp_admin" && slices.Contains([]string{"status", "decided_at", "result_resource_id", "result_version"}, column))
		if read != wantRead || insert != wantInsert || update != wantUpdate || other {
			return errors.New("contribution grant matrix differs from minimum contract")
		}
	}
	if rows.Err() != nil {
		return database.SafeError("inspect contribution grants", rows.Err())
	}
	if len(seen) != 5 {
		return errors.New("contribution schema incomplete")
	}
	return nil
}
