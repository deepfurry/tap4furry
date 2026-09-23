package curation

import (
	"context"
	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/deepfurry/tap4furry/server/internal/governance"
	"github.com/deepfurry/tap4furry/server/internal/taxonomy"
	"uuid"
)

// Category and Tag share localization/state policy, but use explicit SQL tables.
type TaxonomyKind string

const (
	Category TaxonomyKind = "category"
	Tag      TaxonomyKind = "tag"
)

func (k TaxonomyKind) valid() bool { return k == Category || k == Tag }

type TaxonomyInput struct {
	Slug, DefaultLocale, Name string
	Description               *string
}
type TaxonomyPatch struct {
	Reason        string
	DefaultLocale *string
	State         *taxonomy.CategoryState
}

func lockTaxonomy(ctx context.Context, q *sqlc.Queries, kind TaxonomyKind, id uuid.UUID) (sqlc.AppCategory, error) {
	if !kind.valid() || id == uuid.Nil() {
		return sqlc.AppCategory{}, ErrValidation
	}
	if kind == Category {
		return q.CurationLockCategory(ctx, dbID(id))
	}
	t, err := q.CurationLockTag(ctx, dbID(id))
	return sqlc.AppCategory(t), err
}
func taxonomyLocalizationExists(ctx context.Context, q *sqlc.Queries, kind TaxonomyKind, id uuid.UUID, locale string) (bool, error) {
	if kind == Category {
		return q.CategoryLocalizationExists(ctx, sqlc.CategoryLocalizationExistsParams{CategoryID: dbID(id), Locale: locale})
	}
	return q.TagLocalizationExists(ctx, sqlc.TagLocalizationExistsParams{TagID: dbID(id), Locale: locale})
}
func putTaxonomyLocalization(ctx context.Context, q *sqlc.Queries, kind TaxonomyKind, id uuid.UUID, l taxonomy.Localization) (int64, error) {
	if kind == Category {
		return q.CurationPutCategoryLocalization(ctx, sqlc.CurationPutCategoryLocalizationParams{CategoryID: dbID(id), Locale: string(l.Locale), Name: l.Name, Description: dbText(l.Description)})
	}
	return q.CurationPutTagLocalization(ctx, sqlc.CurationPutTagLocalizationParams{TagID: dbID(id), Locale: string(l.Locale), Name: l.Name, Description: dbText(l.Description)})
}
func touchTaxonomy(ctx context.Context, q *sqlc.Queries, kind TaxonomyKind, id uuid.UUID) error {
	if kind == Category {
		return q.CurationTouchCategory(ctx, dbID(id))
	}
	return q.CurationTouchTag(ctx, dbID(id))
}
func (a *App) CreateTaxonomy(ctx context.Context, actor auth.AdminActor, kind TaxonomyKind, input TaxonomyInput) (uuid.UUID, error) {
	id := uuid.NewV7()
	err := a.transact(ctx, actor, auth.Editorial, func(q *sqlc.Queries, _ []auth.Role) error {
		if !kind.valid() || taxonomy.ValidateSlug(input.Slug) != nil {
			return ErrValidation
		}
		l, err := taxonomy.NormalizeLocalization(input.DefaultLocale, input.Name, input.Description)
		if err != nil {
			return err
		}
		if kind == Category {
			err = q.CurationCreateCategory(ctx, sqlc.CurationCreateCategoryParams{ID: dbID(id), Slug: input.Slug, DefaultLocale: string(l.Locale)})
		} else {
			err = q.CurationCreateTag(ctx, sqlc.CurationCreateTagParams{ID: dbID(id), Slug: input.Slug, DefaultLocale: string(l.Locale)})
		}
		if err != nil {
			return err
		}
		_, err = putTaxonomyLocalization(ctx, q, kind, id, l)
		if err == nil {
			err = recordTaxonomy(ctx, q, actor.UserID, kind, id, "create", "")
		}
		return err
	})
	return id, err
}
func (a *App) PatchTaxonomy(ctx context.Context, actor auth.AdminActor, kind TaxonomyKind, id uuid.UUID, input TaxonomyPatch) error {
	return a.transact(ctx, actor, auth.Editorial, func(q *sqlc.Queries, roles []auth.Role) error {
		row, err := lockTaxonomy(ctx, q, kind, id)
		if err != nil {
			return err
		}
		locale, state := row.DefaultLocale, row.State
		if input.DefaultLocale != nil {
			l, err := taxonomy.ParseLocale(*input.DefaultLocale)
			if err != nil {
				return err
			}
			exists, err := taxonomyLocalizationExists(ctx, q, kind, id, string(l))
			if err != nil {
				return err
			}
			if !exists {
				return ErrValidation
			}
			locale = string(l)
		}
		if input.State != nil {
			if !input.State.Valid() {
				return ErrValidation
			}
			if !auth.HasCapability(roles, auth.Administration) {
				return auth.ErrAdminForbidden
			}
			if _, err = governance.Text(input.Reason, 1000); err != nil {
				return ErrValidation
			}
			state = string(*input.State)
		}
		if locale == row.DefaultLocale && state == row.State {
			return nil
		}
		if kind == Category {
			err = q.CurationUpdateCategory(ctx, sqlc.CurationUpdateCategoryParams{ID: row.ID, DefaultLocale: locale, State: state})
		} else {
			err = q.CurationUpdateTag(ctx, sqlc.CurationUpdateTagParams{ID: row.ID, DefaultLocale: locale, State: state})
		}
		if err == nil {
			err = recordTaxonomy(ctx, q, actor.UserID, kind, id, "taxonomy", input.Reason, row.State, state)
		}
		return err
	})
}
func (a *App) PutTaxonomyLocalization(ctx context.Context, actor auth.AdminActor, kind TaxonomyKind, id uuid.UUID, locale, name string, description *string) error {
	return a.transact(ctx, actor, auth.Editorial, func(q *sqlc.Queries, _ []auth.Role) error {
		if _, err := lockTaxonomy(ctx, q, kind, id); err != nil {
			return err
		}
		l, err := taxonomy.NormalizeLocalization(locale, name, description)
		if err != nil {
			return err
		}
		n, err := putTaxonomyLocalization(ctx, q, kind, id, l)
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		if err = touchTaxonomy(ctx, q, kind, id); err != nil {
			return err
		}
		return recordTaxonomy(ctx, q, actor.UserID, kind, id, "localization", "")
	})
}
func (a *App) DeleteTaxonomyLocalization(ctx context.Context, actor auth.AdminActor, kind TaxonomyKind, id uuid.UUID, locale string) error {
	return a.transact(ctx, actor, auth.Editorial, func(q *sqlc.Queries, _ []auth.Role) error {
		row, err := lockTaxonomy(ctx, q, kind, id)
		if err != nil {
			return err
		}
		l, err := taxonomy.ParseLocale(locale)
		if err != nil {
			return err
		}
		if string(l) == row.DefaultLocale {
			return ErrValidation
		}
		var n int64
		if kind == Category {
			n, err = q.CurationDeleteCategoryLocalization(ctx, sqlc.CurationDeleteCategoryLocalizationParams{CategoryID: row.ID, Locale: string(l)})
		} else {
			n, err = q.CurationDeleteTagLocalization(ctx, sqlc.CurationDeleteTagLocalizationParams{TagID: row.ID, Locale: string(l)})
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		if err = touchTaxonomy(ctx, q, kind, id); err != nil {
			return err
		}
		return recordTaxonomy(ctx, q, actor.UserID, kind, id, "localization", "")
	})
}
func (a *App) DeleteTaxonomy(ctx context.Context, actor auth.AdminActor, kind TaxonomyKind, id uuid.UUID, reason string) error {
	return a.transact(ctx, actor, auth.Administration, func(q *sqlc.Queries, _ []auth.Role) error {
		row, err := lockTaxonomy(ctx, q, kind, id)
		if err != nil {
			return err
		}
		if _, err = governance.Text(reason, 1000); err != nil {
			return ErrValidation
		}
		var used bool
		if kind == Category {
			used, err = q.CurationCategoryInUse(ctx, row.ID)
		} else {
			used, err = q.CurationTagInUse(ctx, row.ID)
		}
		if err != nil {
			return err
		}
		if used {
			return ErrInUse
		}
		if kind == Category {
			err = q.CurationSoftDeleteCategory(ctx, row.ID)
		} else {
			err = q.CurationSoftDeleteTag(ctx, row.ID)
		}
		if err == nil {
			err = recordTaxonomy(ctx, q, actor.UserID, kind, id, "soft_delete", reason)
		}
		return err
	})
}

func recordTaxonomy(ctx context.Context, q *sqlc.Queries, actor uuid.UUID, kind TaxonomyKind, id uuid.UUID, op, reason string, states ...string) error {
	c := governance.Change{Operation: op, Fields: []string{"taxonomy"}, Reason: reason}
	if len(states) == 2 {
		c.BeforeTaxonomyState = states[0]
		c.AfterTaxonomyState = states[1]
	}
	if op == "localization" {
		c.Fields = []string{"localization"}
	}
	if op == "soft_delete" {
		c.Fields = []string{"deleted_at"}
	}
	if kind == Category {
		c.CategoryID = id
	} else {
		c.TagID = id
	}
	_, err := governance.Record(ctx, q, actor, c)
	return err
}
