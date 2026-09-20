package curation

import (
	"context"
	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
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
			state = string(*input.State)
		}
		if locale == row.DefaultLocale && state == row.State {
			return nil
		}
		if kind == Category {
			return q.CurationUpdateCategory(ctx, sqlc.CurationUpdateCategoryParams{ID: row.ID, DefaultLocale: locale, State: state})
		}
		return q.CurationUpdateTag(ctx, sqlc.CurationUpdateTagParams{ID: row.ID, DefaultLocale: locale, State: state})
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
		return touchTaxonomy(ctx, q, kind, id)
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
		return touchTaxonomy(ctx, q, kind, id)
	})
}
func (a *App) DeleteTaxonomy(ctx context.Context, actor auth.AdminActor, kind TaxonomyKind, id uuid.UUID) error {
	return a.transact(ctx, actor, auth.Administration, func(q *sqlc.Queries, _ []auth.Role) error {
		row, err := lockTaxonomy(ctx, q, kind, id)
		if err != nil {
			return err
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
			return q.CurationSoftDeleteCategory(ctx, row.ID)
		}
		return q.CurationSoftDeleteTag(ctx, row.ID)
	})
}
