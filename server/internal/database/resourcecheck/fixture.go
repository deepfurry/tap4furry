// Package resourcecheck contains developer-only Resource Core acceptance fixtures.
// Runtime applications must not import it; it owns no product use cases.
package resourcecheck

import (
	"context"
	"errors"
	"time"
	"uuid"

	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/deepfurry/tap4furry/server/internal/resource"
	"github.com/deepfurry/tap4furry/server/internal/taxonomy"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var Tables = []string{"categories", "category_localizations", "tags", "tag_localizations", "resources", "resource_localizations", "resource_tags", "resource_sources", "resource_relations", "resource_external_ids"}

type Fixture struct{ Category, Tag, A, B, Source, Relation uuid.UUID }

func NewFixture() Fixture {
	return Fixture{uuid.NewV7(), uuid.NewV7(), uuid.NewV7(), uuid.NewV7(), uuid.NewV7(), uuid.NewV7()}
}
func ID(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: true} }

func (f Fixture) Create(ctx context.Context, admin *pgxpool.Pool) error {
	tx, err := admin.Begin(ctx)
	if err != nil {
		return database.SafeError("begin resource fixture", err)
	}
	defer tx.Rollback(ctx)
	// Use the database clock throughout this fixture: the developer workstation
	// and shared PostgreSQL clock may differ, even by a fraction of a second.
	var now time.Time
	if err := tx.QueryRow(ctx, "SELECT transaction_timestamp()").Scan(&now); err != nil {
		return database.SafeError("read fixture creation time", err)
	}
	for _, item := range []struct {
		table, loc, key string
		id              uuid.UUID
	}{{"categories", "category_localizations", "category_id", f.Category}, {"tags", "tag_localizations", "tag_id", f.Tag}} {
		l, err := taxonomy.NormalizeLocalization("EN", " Resource smoke taxonomy ", nil)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO app."+item.table+" (id,slug,default_locale,created_at,updated_at) VALUES($1,$2,$3,$4,$4)", item.id.String(), "p02a-"+item.id.String(), string(l.Locale), now); err != nil {
			return database.SafeError("create taxonomy fixture", err)
		}
		if _, err = tx.Exec(ctx, "INSERT INTO app."+item.loc+" ("+item.key+",locale,name,created_at,updated_at) VALUES($1,$2,$3,$4,$4)", item.id.String(), string(l.Locale), l.Name, now); err != nil {
			return database.SafeError("create taxonomy localization fixture", err)
		}
	}
	for _, id := range []uuid.UUID{f.A, f.B} {
		l, err := resource.NormalizeLocalization("EN", " Resource smoke ", nil, nil)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO app.resources (id,slug,default_locale,category_id,content_rating,version,created_at,updated_at) VALUES($1,$2,$3,$4,'general',1,$5,$5)", id.String(), "p02a-"+id.String(), string(l.Locale), f.Category.String(), now); err != nil {
			return database.SafeError("create resource fixture", err)
		}
		if _, err = tx.Exec(ctx, "INSERT INTO app.resource_localizations (resource_id,locale,name,created_at,updated_at) VALUES($1,$2,$3,$4,$4)", id.String(), string(l.Locale), l.Name, now); err != nil {
			return database.SafeError("create resource localization fixture", err)
		}
	}
	u, err := resource.NormalizeURL("HTTPS://Example.INVALID:443/resource#ignored")
	if err != nil {
		return err
	}
	edge, err := resource.CanonicalRelation(f.A, f.B, resource.RelatedTo)
	if err != nil {
		return err
	}
	for _, stmt := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO app.resource_tags (resource_id,tag_id) VALUES($1,$2)", []any{f.A.String(), f.Tag.String()}},
		{"INSERT INTO app.resource_sources (id,resource_id,url,source_type,availability_state,is_primary,created_at,updated_at) VALUES($1,$2,$3,'mirror','unavailable',true,$4,$4)", []any{f.Source.String(), f.A.String(), u, now}},
		{"INSERT INTO app.resource_relations (id,source_resource_id,target_resource_id,relation_type,created_at) VALUES($1,$2,$3,$4,$5)", []any{f.Relation.String(), edge.Source.String(), edge.Target.String(), string(edge.Type), now}},
		{"INSERT INTO app.resource_external_ids (resource_id,namespace,external_id,created_at) VALUES($1,'smoke',$2,$3)", []any{f.A.String(), f.A.String(), now}},
	} {
		if _, err := tx.Exec(ctx, stmt.sql, stmt.args...); err != nil {
			return database.SafeError("create resource graph fixture", err)
		}
	}
	// Initial children are part of the creation revision, so both remain version 1.
	if err := tx.Commit(ctx); err != nil {
		return database.SafeError("commit resource fixture", err)
	}
	return nil
}

// ExerciseMutation proves a logical edit with several child statements and a
// relation mutation bumps each affected endpoint once, under ordered row locks.
func (f Fixture) ExerciseMutation(ctx context.Context, admin *pgxpool.Pool) error {
	tx, err := admin.Begin(ctx)
	if err != nil {
		return database.SafeError("begin graph fixture edit", err)
	}
	defer tx.Rollback(ctx)
	q := sqlc.New(tx)
	var now time.Time
	if err := tx.QueryRow(ctx, "SELECT transaction_timestamp()").Scan(&now); err != nil {
		return database.SafeError("read fixture mutation time", err)
	}
	edge, err := resource.CanonicalRelation(f.A, f.B, resource.RelatedTo)
	if err != nil {
		return err
	}
	for _, id := range []uuid.UUID{edge.Source, edge.Target} {
		r, err := q.LockResourceCore(ctx, ID(id))
		if err != nil {
			return database.SafeError("lock graph fixture", err)
		}
		if r.Version != 1 {
			return resource.ErrVersionConflict
		}
	}
	for _, stmt := range []struct {
		sql  string
		args []any
	}{
		{"UPDATE app.resource_localizations SET summary='Fixture edit',updated_at=now() WHERE resource_id=$1 AND locale='en'", []any{f.A.String()}},
		{"UPDATE app.resource_sources SET label='Fixture source',updated_at=now() WHERE id=$1", []any{f.Source.String()}},
		{"DELETE FROM app.resource_relations WHERE id=$1", []any{f.Relation.String()}},
		{"INSERT INTO app.resource_relations(id,source_resource_id,target_resource_id,relation_type,created_at) VALUES($1,$2,$3,'related_to',now())", []any{f.Relation.String(), edge.Source.String(), edge.Target.String()}},
	} {
		if _, err := tx.Exec(ctx, stmt.sql, stmt.args...); err != nil {
			return database.SafeError("edit graph fixture", err)
		}
	}
	for _, id := range []uuid.UUID{edge.Source, edge.Target} {
		v, err := q.CompareAndBumpResourceVersion(ctx, sqlc.CompareAndBumpResourceVersionParams{ID: ID(id), ExpectedVersion: 1, Now: pgtype.Timestamptz{Time: now, Valid: true}})
		if err != nil {
			return database.SafeError("fixture resource CAS", err)
		}
		if v != 2 {
			return resource.ErrVersionConflict
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return database.SafeError("commit graph fixture edit", err)
	}
	return nil
}

func (f Fixture) Cleanup(ctx context.Context, owner *pgxpool.Pool) error {
	var role string
	if err := owner.QueryRow(ctx, "SELECT current_user").Scan(&role); err != nil {
		return database.SafeError("fixture cleanup identity", err)
	}
	if role != "gfp_migrator" {
		return errors.New("fixture cleanup requires migrator identity")
	}
	tx, err := owner.Begin(ctx)
	if err != nil {
		return database.SafeError("begin fixture cleanup", err)
	}
	defer tx.Rollback(ctx)
	for _, stmt := range []struct {
		sql  string
		args []any
	}{
		{"DELETE FROM app.resource_external_ids WHERE resource_id IN ($1,$2)", []any{f.A.String(), f.B.String()}},
		{"DELETE FROM app.resource_relations WHERE source_resource_id IN ($1,$2) OR target_resource_id IN ($1,$2)", []any{f.A.String(), f.B.String()}},
		{"DELETE FROM app.resource_sources WHERE resource_id IN ($1,$2)", []any{f.A.String(), f.B.String()}},
		{"DELETE FROM app.resource_tags WHERE resource_id IN ($1,$2)", []any{f.A.String(), f.B.String()}},
		{"DELETE FROM app.resource_localizations WHERE resource_id IN ($1,$2)", []any{f.A.String(), f.B.String()}},
		{"DELETE FROM app.resources WHERE id IN ($1,$2)", []any{f.A.String(), f.B.String()}},
		{"DELETE FROM app.tag_localizations WHERE tag_id=$1", []any{f.Tag.String()}},
		{"DELETE FROM app.tags WHERE id=$1", []any{f.Tag.String()}},
		{"DELETE FROM app.category_localizations WHERE category_id=$1", []any{f.Category.String()}},
		{"DELETE FROM app.categories WHERE id=$1", []any{f.Category.String()}},
	} {
		if _, err := tx.Exec(ctx, stmt.sql, stmt.args...); err != nil {
			return database.SafeError("delete owned resource fixture", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return database.SafeError("commit fixture cleanup", err)
	}
	return nil
}
