package resourcecheck

import (
	"context"
	"errors"
	"slices"

	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var adminUpdates = map[string][]string{
	"categories":             {"default_locale", "state", "updated_at", "deleted_at"},
	"tags":                   {"default_locale", "state", "updated_at", "deleted_at"},
	"category_localizations": {"name", "description", "updated_at"},
	"tag_localizations":      {"name", "description", "updated_at"},
	"resources":              {"slug", "default_locale", "category_id", "publication_state", "lifecycle", "content_rating", "version", "published_at", "updated_at", "deleted_at"},
	"resource_localizations": {"name", "summary", "description", "updated_at"},
	"resource_sources":       {"url", "label", "source_type", "availability_state", "rights_status", "is_primary", "updated_at"},
}
var adminDeletes = []string{"category_localizations", "tag_localizations", "resource_localizations", "resource_tags", "resource_relations", "resource_external_ids"}

// VerifyRole checks effective privileges, not only explicit ACL text. It also
// executes SQL under each real identity; denied operations must return 42501.
func VerifyRole(ctx context.Context, pool *pgxpool.Pool, role string, fixture Fixture) error {
	if !slices.Contains([]string{"api", "admin", "worker", "readonly"}, role) {
		return errors.New("unsupported resource check role")
	}
	for _, table := range Tables {
		var read, insert, remove, update, truncate, references, trigger, maintain bool
		// OID lookup avoids requiring schema USAGE merely to inspect denied grants.
		err := pool.QueryRow(ctx, `SELECT has_table_privilege(current_user,c.oid,'SELECT'),has_table_privilege(current_user,c.oid,'INSERT'),has_table_privilege(current_user,c.oid,'DELETE'),has_table_privilege(current_user,c.oid,'UPDATE'),has_table_privilege(current_user,c.oid,'TRUNCATE'),has_table_privilege(current_user,c.oid,'REFERENCES'),has_table_privilege(current_user,c.oid,'TRIGGER'),has_table_privilege(current_user,c.oid,'MAINTAIN') FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='app' AND c.relname=$1`, table).Scan(&read, &insert, &remove, &update, &truncate, &references, &trigger, &maintain)
		if err != nil {
			return database.SafeError("inspect resource table privileges", err)
		}
		if read != (role != "worker") || insert != (role == "admin") || remove != (role == "admin" && slices.Contains(adminDeletes, table)) || update || truncate || references || trigger || maintain {
			return errors.New("resource table privilege contract differs: " + role + "/" + table)
		}
		rows, err := pool.Query(ctx, `SELECT a.attname,has_column_privilege(current_user,c.oid,a.attnum,'UPDATE') FROM pg_attribute a JOIN pg_class c ON c.oid=a.attrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='app' AND c.relname=$1 AND a.attnum>0 AND NOT a.attisdropped`, table)
		if err != nil {
			return database.SafeError("inspect resource column privileges", err)
		}
		columns := []string{}
		for rows.Next() {
			var column string
			var allowed bool
			if err := rows.Scan(&column, &allowed); err != nil {
				rows.Close()
				return database.SafeError("read resource column privileges", err)
			}
			if allowed != (role == "admin" && slices.Contains(adminUpdates[table], column)) {
				rows.Close()
				return errors.New("resource column privilege contract differs: " + role + "/" + table)
			}
			columns = append(columns, column)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return database.SafeError("inspect resource columns", err)
		}
		rows.Close()
		if len(columns) == 0 {
			return errors.New("resource table columns missing")
		}
		_, err = pool.Exec(ctx, "SELECT 1 FROM app."+table+" LIMIT 0")
		if err := expectPermission(err, role != "worker"); err != nil {
			return err
		}
		if role != "admin" {
			_, err = pool.Exec(ctx, "INSERT INTO app."+table+" DEFAULT VALUES")
			if err := expectPermission(err, false); err != nil {
				return err
			}
		}
		_, err = pool.Exec(ctx, "DELETE FROM app."+table+" WHERE false")
		if err := expectPermission(err, role == "admin" && slices.Contains(adminDeletes, table)); err != nil {
			return err
		}
		for _, column := range columns {
			_, err = pool.Exec(ctx, "UPDATE app."+table+" SET "+column+"="+column+" WHERE false")
			if err := expectPermission(err, role == "admin" && slices.Contains(adminUpdates[table], column)); err != nil {
				return err
			}
		}
	}
	if role != "worker" {
		r, err := sqlc.New(pool).GetResourceRevision(ctx, ID(fixture.A))
		if err != nil {
			return database.SafeError("read resource fixture through sqlc", err)
		}
		if r.Version < 1 {
			return errors.New("resource fixture revision missing")
		}
	}
	return nil
}

func expectPermission(err error, allowed bool) error {
	if allowed {
		if err != nil {
			return database.SafeError("allowed resource operation", err)
		}
		return nil
	}
	var p *pgconn.PgError
	if !errors.As(err, &p) || p.Code != "42501" {
		return errors.New("forbidden resource operation did not fail with SQLSTATE 42501")
	}
	return nil
}
