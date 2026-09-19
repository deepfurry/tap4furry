package resourcecheck

import (
	"context"
	"errors"
	"testing"
	"uuid"

	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/deepfurry/tap4furry/server/internal/taxonomy"
	"github.com/jackc/pgx/v5/pgxpool"
)

var errDefaultLocalization = errors.New("current default localization cannot be deleted")
var errMissingLocalization = errors.New("target localization does not exist")

// This is an executable transaction pattern, not a premature CRUD use case.
// P0-2C must serialize child/default edits on the parent and use the same tx for CAS.
func editDefaultLocalization(ctx context.Context, pool *pgxpool.Pool, table string, id uuid.UUID, raw string, remove bool) error {
	locale, err := taxonomy.ParseLocale(raw)
	if err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := sqlc.New(tx)
	var current, key, localizations string
	var exists bool
	var version int64
	switch table {
	case "categories":
		row, e := q.LockCategoryLocalizationParent(ctx, ID(id))
		if e != nil {
			return e
		}
		current = row.DefaultLocale
		key, localizations = "category_id", "category_localizations"
		exists, err = q.CategoryLocalizationExists(ctx, sqlc.CategoryLocalizationExistsParams{CategoryID: ID(id), Locale: string(locale)})
	case "tags":
		row, e := q.LockTagLocalizationParent(ctx, ID(id))
		if e != nil {
			return e
		}
		current = row.DefaultLocale
		key, localizations = "tag_id", "tag_localizations"
		exists, err = q.TagLocalizationExists(ctx, sqlc.TagLocalizationExistsParams{TagID: ID(id), Locale: string(locale)})
	case "resources":
		row, e := q.LockResourceCore(ctx, ID(id))
		if e != nil {
			return e
		}
		current, version = row.DefaultLocale, row.Version
		key, localizations = "resource_id", "resource_localizations"
		exists, err = q.ResourceLocalizationExists(ctx, sqlc.ResourceLocalizationExistsParams{ResourceID: ID(id), Locale: string(locale)})
	default:
		return errors.New("unsupported localization fixture entity")
	}
	if err != nil {
		return err
	}
	if !exists {
		return errMissingLocalization
	}
	if remove && current == string(locale) {
		return errDefaultLocalization
	}
	if !remove && current == string(locale) {
		return nil
	}
	if remove {
		_, err = tx.Exec(ctx, "DELETE FROM app."+localizations+" WHERE "+key+"=$1 AND locale=$2", id.String(), string(locale))
	} else {
		_, err = tx.Exec(ctx, "UPDATE app."+table+" SET default_locale=$2,updated_at=now() WHERE id=$1", id.String(), string(locale))
	}
	if err != nil {
		return err
	}
	if table == "resources" {
		if _, err = bump(ctx, q, id, version); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func TestIntegrationLocalizationTransactionPatterns(t *testing.T) {
	f := setup(t)
	p := f.pools["admin"]
	for _, item := range []struct {
		table, localizations, key string
		id                        uuid.UUID
	}{{"categories", "category_localizations", "category_id", f.Category}, {"tags", "tag_localizations", "tag_id", f.Tag}, {"resources", "resource_localizations", "resource_id", f.A}} {
		t.Run(item.table, func(t *testing.T) {
			var complete bool
			if err := p.QueryRow(t.Context(), "SELECT EXISTS(SELECT 1 FROM app."+item.table+" p JOIN app."+item.localizations+" l ON l."+item.key+"=p.id AND l.locale=p.default_locale WHERE p.id=$1)", item.id.String()).Scan(&complete); err != nil || !complete {
				t.Fatal("creation did not commit entity plus default localization")
			}
			// A failing default localization must roll back the preceding entity insert.
			newID := uuid.NewV7()
			tx, err := p.Begin(t.Context())
			if err != nil {
				t.Fatal("atomic creation transaction unavailable")
			}
			defer tx.Rollback(t.Context())
			if item.table == "resources" {
				_, err = tx.Exec(t.Context(), "INSERT INTO app.resources(id,slug,default_locale,category_id,content_rating,version,created_at,updated_at) VALUES($1,$2,'en',$3,'general',1,now(),now())", newID.String(), "p02a-"+newID.String(), f.Category.String())
			} else {
				_, err = tx.Exec(t.Context(), "INSERT INTO app."+item.table+"(id,slug,default_locale,created_at,updated_at) VALUES($1,$2,'en',now(),now())", newID.String(), "p02a-"+newID.String())
			}
			if err != nil {
				t.Fatal("atomic creation first statement failed")
			}
			_, err = tx.Exec(t.Context(), "INSERT INTO app."+item.localizations+"("+item.key+",locale,name,created_at,updated_at) VALUES($1,'en','',now(),now())", newID.String())
			if err == nil {
				t.Fatal("invalid default localization accepted")
			}
			if err = tx.Rollback(t.Context()); err != nil {
				t.Fatal("atomic creation rollback failed")
			}
			var exists bool
			if err = p.QueryRow(t.Context(), "SELECT EXISTS(SELECT 1 FROM app."+item.table+" WHERE id=$1)", newID.String()).Scan(&exists); err != nil || exists {
				t.Fatal("failed localization left an orphan entity")
			}
			if err = editDefaultLocalization(t.Context(), p, item.table, item.id, "JA", false); !errors.Is(err, errMissingLocalization) {
				t.Fatal("default locale switched to a missing translation")
			}
			if err = editDefaultLocalization(t.Context(), p, item.table, item.id, "EN", true); !errors.Is(err, errDefaultLocalization) {
				t.Fatal("current default localization deletion was allowed")
			}
			tx, err = p.Begin(t.Context())
			if err != nil {
				t.Fatal("translation transaction failed")
			}
			defer tx.Rollback(t.Context())
			if _, err = tx.Exec(t.Context(), "INSERT INTO app."+item.localizations+"("+item.key+",locale,name,created_at,updated_at) VALUES($1,'ja','日本語',now(),now())", item.id.String()); err != nil {
				t.Fatal("add translation fixture failed")
			}
			if item.table == "resources" {
				if _, err = bump(t.Context(), sqlc.New(tx), item.id, 1); err != nil {
					t.Fatal("translation revision failed")
				}
			}
			if err = tx.Commit(t.Context()); err != nil {
				t.Fatal("translation commit failed")
			}
			if err = editDefaultLocalization(t.Context(), p, item.table, item.id, "JA", false); err != nil {
				t.Fatal("existing target translation was rejected")
			}
			if err = editDefaultLocalization(t.Context(), p, item.table, item.id, "EN", true); err != nil {
				t.Fatal("non-default translation cannot be removed")
			}
			if err = editDefaultLocalization(t.Context(), p, item.table, item.id, "ja", true); !errors.Is(err, errDefaultLocalization) {
				t.Fatal("new default can be deleted")
			}
			var locale string
			if err = p.QueryRow(t.Context(), "SELECT default_locale FROM app."+item.table+" WHERE id=$1", item.id.String()).Scan(&locale); err != nil || locale != "ja" {
				t.Fatal("default locale not canonical or not committed")
			}
		})
	}
	r, err := sqlc.New(p).GetResourceRevision(t.Context(), ID(f.A))
	if err != nil || r.Version != 4 {
		t.Fatal("localization logical transactions did not each bump once")
	}
}
