// Package governancecheck contains developer-only acceptance fixtures. Runtime
// binaries must never import it; shared smoke only touches its generated IDs.
package governancecheck

import (
	"context"
	"errors"
	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
)

var tables = []string{"reports", "report_events", "user_governance_profiles", "user_restrictions", "resource_distribution_policies", "source_checks", "moderation_actions", "audit_entries"}
var apiRead = map[string]string{
	"reports":                        "id reporter_id resource_id source_id target_kind reason body request_id request_fingerprint status version created_at decided_at",
	"report_events":                  "id report_id event_type safe_message occurred_at actor_id request_id request_fingerprint",
	"user_governance_profiles":       "user_id trust_level revision",
	"user_restrictions":              "id user_id scope reason_code user_message starts_at expires_at revoked_at",
	"resource_distribution_policies": "resource_id policy",
}
var apiInsert = map[string]string{"reports": "id reporter_id resource_id source_id target_kind reason body request_id request_fingerprint queue priority", "report_events": "id report_id event_type actor_id request_id request_fingerprint safe_message"}
var apiUpdate = map[string]string{"reports": "status version updated_at decided_at"}
var adminInsert = map[string]string{
	"report_events":                  "id report_id event_type actor_id request_id request_fingerprint safe_message internal_note audit_id resolution_type",
	"user_governance_profiles":       "user_id trust_level revision",
	"user_restrictions":              "id user_id scope reason_code user_message internal_note created_by starts_at expires_at",
	"resource_distribution_policies": "resource_id policy",
	"source_checks":                  "id resource_id source_id url_fingerprint resource_version outcome note actor_id request_id request_fingerprint observed_at",
	"moderation_actions":             "id actor_id action resource_id source_id user_id restriction_id report_id reason internal_note request_id request_fingerprint",
	"audit_entries":                  "id actor_id operation resource_id category_id tag_id user_id source_id contribution_id moderation_action_id fields before_version after_version before_publication after_publication before_rights after_rights before_distribution after_distribution before_trust after_trust reason before_availability after_availability before_taxonomy_state after_taxonomy_state",
}
var adminUpdate = map[string]string{"reports": "status version updated_at decided_at queue duplicate_of", "user_governance_profiles": "trust_level revision updated_at", "user_restrictions": "revoked_at revoked_by revoke_reason", "resource_distribution_policies": "policy"}

func contains(columns, column string) bool { return strings.Contains(" "+columns+" ", " "+column+" ") }
func VerifyGrants(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx, `SELECT role,c.relname,a.attname,
 has_column_privilege(role,c.oid,a.attnum,'SELECT'),has_column_privilege(role,c.oid,a.attnum,'INSERT'),has_column_privilege(role,c.oid,a.attnum,'UPDATE'),
 has_column_privilege(role,c.oid,a.attnum,'REFERENCES') OR has_table_privilege(role,c.oid,'DELETE,TRUNCATE,TRIGGER,MAINTAIN')
 FROM unnest(ARRAY['gfp_api','gfp_admin','gfp_worker','gfp_readonly']) role
 CROSS JOIN pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace JOIN pg_attribute a ON a.attrelid=c.oid
 WHERE n.nspname='app' AND c.relname=ANY($1::text[]) AND a.attnum>0 AND NOT a.attisdropped`, tables)
	if err != nil {
		return database.SafeError("inspect governance grants", err)
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var role, table, col string
		var read, insert, update, other bool
		if err = rows.Scan(&role, &table, &col, &read, &insert, &update, &other); err != nil {
			return database.SafeError("read governance grants", err)
		}
		seen[table] = true
		wantRead := role == "gfp_admin" || role == "gfp_readonly" || (role == "gfp_api" && contains(apiRead[table], col))
		wantInsert := (role == "gfp_admin" && contains(adminInsert[table], col)) || (role == "gfp_api" && contains(apiInsert[table], col))
		wantUpdate := (role == "gfp_admin" && contains(adminUpdate[table], col)) || (role == "gfp_api" && contains(apiUpdate[table], col))
		if read != wantRead || insert != wantInsert || update != wantUpdate || other {
			return errors.New("governance minimum column grant matrix mismatch")
		}
	}
	if err = rows.Err(); err != nil {
		return database.SafeError("inspect governance grant rows", err)
	}
	if len(seen) != 8 {
		return errors.New("governance schema incomplete")
	}
	return nil
}
