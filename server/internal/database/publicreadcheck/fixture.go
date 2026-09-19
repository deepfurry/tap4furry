// Package publicreadcheck owns developer-only Public Resource acceptance data.
// It must never enter a runtime process's dependency graph.
package publicreadcheck

import (
	"context"
	"errors"
	"time"
	"uuid"

	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/deepfurry/tap4furry/server/internal/resource"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Fixture struct {
	Prefix                      string
	Categories, Tags, Resources map[string]uuid.UUID
	VisibleSources              map[string]bool
}

func NewFixture() *Fixture {
	f := &Fixture{Prefix: "p02b-" + uuid.NewV7().String(), Categories: map[string]uuid.UUID{}, Tags: map[string]uuid.UUID{}, Resources: map[string]uuid.UUID{}, VisibleSources: map[string]bool{}}
	for _, name := range []string{"active", "retired", "deleted"} {
		f.Categories[name] = uuid.NewV7()
		f.Tags[name] = uuid.NewV7()
	}
	for _, name := range []string{"public", "other", "draft", "pending", "restricted", "removed", "deleted", "retired-category", "deleted-category"} {
		f.Resources[name] = uuid.NewV7()
	}
	return f
}
func (f *Fixture) Slug(name string) string { return f.Prefix + "-" + name }

func ownerIdentity(ctx context.Context, pool *pgxpool.Pool) error {
	var who string
	if err := pool.QueryRow(ctx, "SELECT current_user").Scan(&who); err != nil {
		return database.SafeError("public fixture identity", err)
	}
	if who != "gfp_migrator" {
		return errors.New("public fixture requires migrator identity")
	}
	return nil
}

func (f *Fixture) Create(ctx context.Context, owner *pgxpool.Pool) error {
	if err := ownerIdentity(ctx, owner); err != nil {
		return err
	}
	tx, err := owner.Begin(ctx)
	if err != nil {
		return database.SafeError("begin public fixture", err)
	}
	defer tx.Rollback(ctx)
	var now time.Time
	if err = tx.QueryRow(ctx, "SELECT transaction_timestamp()").Scan(&now); err != nil {
		return database.SafeError("public fixture clock", err)
	}
	for _, kind := range []struct {
		table, localizations, key string
		ids                       map[string]uuid.UUID
	}{{"categories", "category_localizations", "category_id", f.Categories}, {"tags", "tag_localizations", "tag_id", f.Tags}} {
		for _, state := range []string{"active", "retired", "deleted"} {
			id := kind.ids[state].String()
			storedState := state
			var deleted any
			if state == "deleted" {
				storedState = "active"
				deleted = now
			}
			if _, err = tx.Exec(ctx, "INSERT INTO app."+kind.table+"(id,slug,default_locale,state,created_at,updated_at,deleted_at) VALUES($1,$2,'en',$3,$4,$4,$5)", id, f.Slug(state), storedState, now, deleted); err != nil {
				return database.SafeError("create public taxonomy", err)
			}
			for _, locale := range []string{"en", "ja"} {
				name := "Default " + kind.table + " " + state
				var description any = "Default taxonomy description"
				if locale == "ja" {
					name = "Requested " + kind.table + " " + state
					description = nil
				}
				if _, err = tx.Exec(ctx, "INSERT INTO app."+kind.localizations+"("+kind.key+",locale,name,description,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$5)", id, locale, name, description, now); err != nil {
					return database.SafeError("create public taxonomy text", err)
				}
			}
		}
	}
	for i, name := range []string{"public", "other", "draft", "pending", "restricted", "removed", "deleted", "retired-category", "deleted-category"} {
		state, category, lifecycle, rating := "published", "active", "unknown", "general"
		var deleted any
		switch name {
		case "draft", "pending", "restricted", "removed":
			state = name
		case "deleted":
			deleted = now
		case "retired-category":
			category = "retired"
		case "deleted-category":
			category = "deleted"
		}
		if name == "public" {
			lifecycle, rating = "discontinued", "explicit"
		}
		created := now.Add(-24 * time.Hour)
		published := now.Add(-time.Duration(i) * time.Minute)
		if _, err = tx.Exec(ctx, "INSERT INTO app.resources(id,slug,default_locale,category_id,publication_state,lifecycle,content_rating,version,published_at,created_at,updated_at,deleted_at) VALUES($1,$2,'en',$3,$4,$5,$6,1,$7,$8,$9,$10)", f.Resources[name].String(), f.Slug(name), f.Categories[category].String(), state, lifecycle, rating, published, created, now, deleted); err != nil {
			return database.SafeError("create public resource", err)
		}
		for _, locale := range []string{"en", "ja"} {
			text := "Default resource " + name
			var summary any = "Default summary"
			var description any = "## Canonical description\n\nA **community** resource."
			if locale == "ja" {
				text = "Requested resource " + name
				summary = nil
				description = nil
			}
			if _, err = tx.Exec(ctx, "INSERT INTO app.resource_localizations(resource_id,locale,name,summary,description,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$6)", f.Resources[name].String(), locale, text, summary, description, now); err != nil {
				return database.SafeError("create public resource text", err)
			}
		}
	}
	for _, id := range f.Tags {
		if _, err = tx.Exec(ctx, "INSERT INTO app.resource_tags(resource_id,tag_id) VALUES($1,$2)", f.Resources["public"].String(), id.String()); err != nil {
			return database.SafeError("create public tag links", err)
		}
	}
	for _, availability := range []string{"active", "unavailable", "broken", "restricted", "removed"} {
		for _, rights := range []string{"unknown", "creator_provided", "confirmed", "disputed", "rights_review", "removed_by_request"} {
			id := uuid.NewV7()
			visible := availability != "removed" && (rights == "unknown" || rights == "creator_provided" || rights == "confirmed")
			f.VisibleSources[id.String()] = visible
			if _, err = tx.Exec(ctx, "INSERT INTO app.resource_sources(id,resource_id,url,source_type,availability_state,rights_status,is_primary,created_at,updated_at) VALUES($1,$2,$3,'mirror',$4,$5,$6,$7,$7)", id.String(), f.Resources["public"].String(), "https://example.invalid/"+id.String(), availability, rights, availability == "active" && rights == "unknown", now); err != nil {
				return database.SafeError("create public source matrix", err)
			}
		}
	}
	for name, id := range f.Resources {
		if name == "public" {
			continue
		}
		if _, err = tx.Exec(ctx, "INSERT INTO app.resource_relations(id,source_resource_id,target_resource_id,relation_type,created_at) VALUES($1,$2,$3,'part_of',$4)", uuid.NewV7().String(), f.Resources["public"].String(), id.String(), now); err != nil {
			return database.SafeError("create public relation matrix", err)
		}
	}
	for _, kind := range []resource.RelationType{resource.SuccessorOf, resource.DerivedFrom, resource.RelatedTo} {
		edge, e := resource.CanonicalRelation(f.Resources["other"], f.Resources["public"], kind)
		if e != nil {
			return e
		}
		if _, err = tx.Exec(ctx, "INSERT INTO app.resource_relations(id,source_resource_id,target_resource_id,relation_type,created_at) VALUES($1,$2,$3,$4,$5)", uuid.NewV7().String(), edge.Source.String(), edge.Target.String(), string(kind), now); err != nil {
			return database.SafeError("create public inverse relation", err)
		}
	}
	if _, err = tx.Exec(ctx, "INSERT INTO app.resource_external_ids(resource_id,namespace,external_id,created_at) VALUES($1,'fixture',$2,$3)", f.Resources["public"].String(), f.Prefix, now); err != nil {
		return database.SafeError("create public external ID", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return database.SafeError("commit public fixture", err)
	}
	return nil
}

func (f *Fixture) Cleanup(ctx context.Context, owner *pgxpool.Pool) error {
	if err := ownerIdentity(ctx, owner); err != nil {
		return err
	}
	tx, err := owner.Begin(ctx)
	if err != nil {
		return database.SafeError("begin public cleanup", err)
	}
	defer tx.Rollback(ctx)
	ids := func(m map[string]uuid.UUID) []string {
		r := []string{}
		for _, id := range m {
			r = append(r, id.String())
		}
		return r
	}
	for _, step := range []struct {
		sql    string
		values []string
	}{
		{"DELETE FROM app.resource_external_ids WHERE resource_id=ANY($1::uuid[])", ids(f.Resources)},
		{"DELETE FROM app.resource_relations WHERE source_resource_id=ANY($1::uuid[]) OR target_resource_id=ANY($1::uuid[])", ids(f.Resources)},
		{"DELETE FROM app.resource_sources WHERE resource_id=ANY($1::uuid[])", ids(f.Resources)},
		{"DELETE FROM app.resource_tags WHERE resource_id=ANY($1::uuid[])", ids(f.Resources)},
		{"DELETE FROM app.resource_localizations WHERE resource_id=ANY($1::uuid[])", ids(f.Resources)},
		{"DELETE FROM app.resources WHERE id=ANY($1::uuid[])", ids(f.Resources)},
		{"DELETE FROM app.tag_localizations WHERE tag_id=ANY($1::uuid[])", ids(f.Tags)},
		{"DELETE FROM app.tags WHERE id=ANY($1::uuid[])", ids(f.Tags)},
		{"DELETE FROM app.category_localizations WHERE category_id=ANY($1::uuid[])", ids(f.Categories)},
		{"DELETE FROM app.categories WHERE id=ANY($1::uuid[])", ids(f.Categories)},
	} {
		if _, err = tx.Exec(ctx, step.sql, step.values); err != nil {
			return database.SafeError("clean owned public fixture", err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return database.SafeError("commit public cleanup", err)
	}
	return nil
}
