package curation

import (
	"context"
	"errors"
	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/deepfurry/tap4furry/server/internal/resource"
	"github.com/jackc/pgx/v5"
	"uuid"
)

// ApplyReviewedTx joins a review transaction without committing it. Authorization
// is checked here too: a caller cannot manufacture an already-authorized actor.
// A nil target creates a draft; otherwise the existing public resource is locked
// and its default language and revision must still equal the submitted baseline.
func ApplyReviewedTx(ctx context.Context, tx pgx.Tx, actor auth.AdminActor, target uuid.UUID, expected int64, baseLocale string, input CreateInput, source *SourceInput) (Revision, error) {
	if _, err := auth.RequireAdminCapabilityTx(ctx, tx, actor, auth.Editorial); err != nil {
		return Revision{}, err
	}
	q := sqlc.New(tx)
	if target == uuid.Nil() {
		result, err := createResource(ctx, q, input)
		if err != nil {
			return Revision{}, safe(err)
		}
		if source != nil {
			s, err := normalizeSource(*source)
			if err != nil {
				return Revision{}, err
			}
			err = q.CurationCreateSource(ctx, sqlc.CurationCreateSourceParams{ID: dbID(uuid.NewV7()), ResourceID: dbID(result.ID), Url: s.URL, Label: dbText(s.Label), SourceType: string(s.Type), AvailabilityState: string(s.Availability), IsPrimary: true})
			if err != nil {
				return Revision{}, safe(err)
			}
		}
		return result, nil
	}
	row, err := lockedResource(ctx, q, target, expected)
	if err != nil {
		return Revision{}, err
	}
	if row.PublicationState != "published" || row.DefaultLocale != baseLocale || input.DefaultLocale != baseLocale || source != nil {
		return Revision{}, resource.ErrVersionConflict
	}
	canonical, err := q.ContributionResource(ctx, sqlc.ContributionResourceParams{ID: row.ID})
	if err != nil {
		return Revision{}, safe(err)
	}
	if !canonical.CanonicalOk {
		return Revision{}, errors.New("canonical Resource localization missing")
	}
	l, err := normalizeLocalization(input.DefaultLocale, input.Localization)
	if err != nil || !input.Lifecycle.Valid() || !input.ContentRating.Valid() {
		return Revision{}, ErrValidation
	}
	category, err := q.CurationLockCategory(ctx, dbID(input.CategoryID))
	if err != nil {
		return Revision{}, safe(err)
	}
	if category.State != "active" && input.CategoryID != uuid.UUID(row.CategoryID.Bytes) {
		return Revision{}, ErrValidation
	}
	changed := row.CategoryID != category.ID || row.Lifecycle != string(input.Lifecycle) || row.ContentRating != string(input.ContentRating)
	n, err := putLocalization(ctx, q, target, l)
	if err != nil {
		return Revision{}, safe(err)
	}
	if !changed && n == 0 {
		return Revision{}, ErrValidation
	}
	if err = q.CurationUpdateCore(ctx, sqlc.CurationUpdateCoreParams{ID: row.ID, Slug: row.Slug, DefaultLocale: row.DefaultLocale, CategoryID: category.ID, Lifecycle: string(input.Lifecycle), ContentRating: string(input.ContentRating)}); err != nil {
		return Revision{}, safe(err)
	}
	result, err := bump(ctx, q, target, expected)
	return result, safe(err)
}
