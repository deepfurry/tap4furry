package curation

import (
	"context"
	"slices"
	"uuid"

	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/deepfurry/tap4furry/server/internal/governance"
	"github.com/deepfurry/tap4furry/server/internal/resource"
	"github.com/deepfurry/tap4furry/server/internal/taxonomy"
)

type LocalizationInput struct {
	Name                 string
	Summary, Description *string
}

func normalizeLocalization(locale string, input LocalizationInput) (resource.Localization, error) {
	description, err := taxonomy.OptionalText(input.Description, 50000)
	if err != nil {
		return resource.Localization{}, ErrValidation
	}
	return resource.NormalizeLocalization(locale, input.Name, input.Summary, description)
}
func putLocalization(ctx context.Context, q *sqlc.Queries, id uuid.UUID, l resource.Localization) (int64, error) {
	return q.CurationPutLocalization(ctx, sqlc.CurationPutLocalizationParams{ResourceID: dbID(id), Locale: string(l.Locale), Name: l.Name, Summary: dbText(l.Summary), Description: dbText(l.Description)})
}

type CreateInput struct {
	Slug, DefaultLocale string
	CategoryID          uuid.UUID
	Lifecycle           resource.Lifecycle
	ContentRating       resource.ContentRating
	Localization        LocalizationInput
}

func (a *App) CreateResource(ctx context.Context, actor auth.AdminActor, input CreateInput) (Revision, error) {
	var result Revision
	err := a.transact(ctx, actor, auth.Editorial, func(q *sqlc.Queries, _ []auth.Role) error {
		var err error
		result, err = createResource(ctx, q, input)
		if err == nil {
			_, err = governance.Record(ctx, q, actor.UserID, governance.Change{Operation: "create", ResourceID: result.ID, AfterVersion: 1, Fields: []string{"core", "localization"}})
		}
		return err
	})
	return result, err
}

func createResource(ctx context.Context, q *sqlc.Queries, input CreateInput) (Revision, error) {
	result := Revision{uuid.NewV7(), 1}
	err := func() error {
		if resource.ValidateSlug(input.Slug) != nil || input.CategoryID == uuid.Nil() || !input.ContentRating.Valid() {
			return ErrValidation
		}
		if input.Lifecycle == "" {
			input.Lifecycle = resource.Unknown
		}
		if !input.Lifecycle.Valid() {
			return ErrValidation
		}
		l, err := normalizeLocalization(input.DefaultLocale, input.Localization)
		if err != nil {
			return err
		}
		category, err := q.CurationLockCategory(ctx, dbID(input.CategoryID))
		if err != nil {
			return err
		}
		if category.State != "active" {
			return ErrValidation
		}
		if err = q.CurationCreateResource(ctx, sqlc.CurationCreateResourceParams{ID: dbID(result.ID), Slug: input.Slug, DefaultLocale: string(l.Locale), CategoryID: dbID(input.CategoryID), Lifecycle: string(input.Lifecycle), ContentRating: string(input.ContentRating)}); err != nil {
			return err
		}
		_, err = putLocalization(ctx, q, result.ID, l)
		return err
	}()
	return result, err
}

type CorePatch struct {
	Slug, DefaultLocale *string
	CategoryID          *uuid.UUID
	Lifecycle           *resource.Lifecycle
	ContentRating       *resource.ContentRating
}

func (a *App) PatchResource(ctx context.Context, actor auth.AdminActor, id uuid.UUID, expected int64, input CorePatch) (Revision, error) {
	return a.mutate(ctx, actor, id, expected, auth.Editorial, governance.Change{Operation: "core", Fields: []string{"core"}}, func(q *sqlc.Queries, row sqlc.LockResourceCoreRow, _ []auth.Role) (bool, error) {
		next := row
		if input.Slug != nil {
			core := resource.Core{Slug: row.Slug}
			if row.PublishedAt.Valid {
				core.PublishedAt = &row.PublishedAt.Time
			}
			if err := core.ValidateSlugChange(*input.Slug); err != nil {
				return false, err
			}
			next.Slug = *input.Slug
		}
		if input.DefaultLocale != nil {
			locale, err := taxonomy.ParseLocale(*input.DefaultLocale)
			if err != nil {
				return false, err
			}
			ok, err := q.ResourceLocalizationExists(ctx, sqlc.ResourceLocalizationExistsParams{ResourceID: row.ID, Locale: string(locale)})
			if err != nil {
				return false, err
			}
			if !ok {
				return false, ErrValidation
			}
			next.DefaultLocale = string(locale)
		}
		if input.CategoryID != nil && *input.CategoryID != uuid.UUID(row.CategoryID.Bytes) {
			category, err := q.CurationLockCategory(ctx, dbID(*input.CategoryID))
			if err != nil {
				return false, err
			}
			if category.State != "active" {
				return false, ErrValidation
			}
			next.CategoryID = category.ID
		}
		if input.Lifecycle != nil {
			if !input.Lifecycle.Valid() {
				return false, ErrValidation
			}
			next.Lifecycle = string(*input.Lifecycle)
		}
		if input.ContentRating != nil {
			if !input.ContentRating.Valid() {
				return false, ErrValidation
			}
			next.ContentRating = string(*input.ContentRating)
		}
		changed := next.Slug != row.Slug || next.DefaultLocale != row.DefaultLocale || next.CategoryID != row.CategoryID || next.Lifecycle != row.Lifecycle || next.ContentRating != row.ContentRating
		if !changed {
			return false, nil
		}
		return true, q.CurationUpdateCore(ctx, sqlc.CurationUpdateCoreParams{ID: row.ID, Slug: next.Slug, DefaultLocale: next.DefaultLocale, CategoryID: next.CategoryID, Lifecycle: next.Lifecycle, ContentRating: next.ContentRating})
	})
}
func (a *App) PutLocalization(ctx context.Context, actor auth.AdminActor, id uuid.UUID, expected int64, locale string, input LocalizationInput) (Revision, error) {
	return a.mutate(ctx, actor, id, expected, auth.Editorial, governance.Change{Operation: "localization", Fields: []string{"localization"}}, func(q *sqlc.Queries, _ sqlc.LockResourceCoreRow, _ []auth.Role) (bool, error) {
		l, err := normalizeLocalization(locale, input)
		if err != nil {
			return false, err
		}
		n, err := putLocalization(ctx, q, id, l)
		return n > 0, err
	})
}
func (a *App) DeleteLocalization(ctx context.Context, actor auth.AdminActor, id uuid.UUID, expected int64, locale string) (Revision, error) {
	return a.mutate(ctx, actor, id, expected, auth.Editorial, governance.Change{Operation: "localization", Fields: []string{"localization"}}, func(q *sqlc.Queries, row sqlc.LockResourceCoreRow, _ []auth.Role) (bool, error) {
		l, err := taxonomy.ParseLocale(locale)
		if err != nil {
			return false, err
		}
		if string(l) == row.DefaultLocale {
			return false, ErrValidation
		}
		n, err := q.CurationDeleteLocalization(ctx, sqlc.CurationDeleteLocalizationParams{ResourceID: row.ID, Locale: string(l)})
		return n > 0, err
	})
}
func (a *App) SetTags(ctx context.Context, actor auth.AdminActor, id uuid.UUID, expected int64, ids []uuid.UUID) (Revision, error) {
	return a.mutate(ctx, actor, id, expected, auth.Editorial, governance.Change{Operation: "tags", Fields: []string{"tags"}}, func(q *sqlc.Queries, row sqlc.LockResourceCoreRow, _ []auth.Role) (bool, error) {
		current, err := q.CurationCurrentTags(ctx, row.ID)
		if err != nil {
			return false, err
		}
		old := map[uuid.UUID]bool{}
		for _, v := range current {
			old[uuid.UUID(v.Bytes)] = true
		}
		ids = slices.Clone(ids)
		slices.SortFunc(ids, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
		ids = slices.Compact(ids)
		next := map[uuid.UUID]bool{}
		changed := false
		for _, tag := range ids {
			next[tag] = true
			if old[tag] {
				continue
			}
			t, err := q.CurationLockTag(ctx, dbID(tag))
			if err != nil {
				return false, err
			}
			if t.State != "active" {
				return false, ErrValidation
			}
			if err = q.CurationAddTag(ctx, sqlc.CurationAddTagParams{ResourceID: row.ID, TagID: t.ID}); err != nil {
				return false, err
			}
			changed = true
		}
		for _, tag := range current {
			if !next[uuid.UUID(tag.Bytes)] {
				if err = q.CurationRemoveTag(ctx, sqlc.CurationRemoveTagParams{ResourceID: row.ID, TagID: tag}); err != nil {
					return false, err
				}
				changed = true
			}
		}
		return changed, nil
	})
}
func (a *App) SetExternalIDs(ctx context.Context, actor auth.AdminActor, id uuid.UUID, expected int64, items []resource.ExternalID) (Revision, error) {
	return a.mutate(ctx, actor, id, expected, auth.Editorial, governance.Change{Operation: "external_ids", Fields: []string{"external_ids"}}, func(q *sqlc.Queries, row sqlc.LockResourceCoreRow, _ []auth.Role) (bool, error) {
		current, err := q.AdminResourceExternalIDs(ctx, row.ID)
		if err != nil {
			return false, err
		}
		next := map[resource.ExternalID]bool{}
		old := map[resource.ExternalID]bool{}
		for _, item := range items {
			n, err := item.Normalize()
			if err != nil {
				return false, err
			}
			next[n] = true
		}
		changed := false
		for _, item := range current {
			key := resource.ExternalID{Namespace: item.Namespace, Value: item.ExternalID}
			old[key] = true
			if !next[key] {
				if err = q.CurationRemoveExternalID(ctx, sqlc.CurationRemoveExternalIDParams{ResourceID: row.ID, Namespace: key.Namespace, ExternalID: key.Value}); err != nil {
					return false, err
				}
				changed = true
			}
		}
		for key := range next {
			if !old[key] {
				if err = q.CurationAddExternalID(ctx, sqlc.CurationAddExternalIDParams{ResourceID: row.ID, Namespace: key.Namespace, ExternalID: key.Value}); err != nil {
					return false, err
				}
				changed = true
			}
		}
		return changed, nil
	})
}
func (a *App) DeleteResource(ctx context.Context, actor auth.AdminActor, id uuid.UUID, expected int64, reason string) (Revision, error) {
	if _, err := governance.Text(reason, 1000); err != nil {
		return Revision{}, ErrValidation
	}
	return a.mutate(ctx, actor, id, expected, auth.Administration, governance.Change{Operation: "soft_delete", Fields: []string{"deleted_at"}, Reason: reason}, func(q *sqlc.Queries, row sqlc.LockResourceCoreRow, _ []auth.Role) (bool, error) {
		return true, q.CurationSoftDeleteResource(ctx, row.ID)
	})
}
