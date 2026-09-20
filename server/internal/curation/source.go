package curation

import (
	"context"
	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/deepfurry/tap4furry/server/internal/resource"
	"github.com/deepfurry/tap4furry/server/internal/taxonomy"
	"uuid"
)

type SourceInput struct {
	URL          string
	Label        *string
	Type         resource.SourceType
	Availability resource.SourceAvailabilityState
	Primary      bool
}

// Present distinguishes omitted nullable label from an explicit null in PATCH.
type NullableTextPatch struct {
	Present bool
	Value   *string
}
type SourcePatch struct {
	URL          *string
	Label        NullableTextPatch
	Type         *resource.SourceType
	Availability *resource.SourceAvailabilityState
	Primary      *bool
}

func normalizeSource(input SourceInput) (SourceInput, error) {
	if !input.Type.Valid() || !input.Availability.Valid() {
		return input, ErrValidation
	}
	url, err := resource.NormalizeURL(input.URL)
	if err != nil {
		return input, err
	}
	label, err := taxonomy.OptionalText(input.Label, 80)
	if err != nil {
		return input, err
	}
	input.URL = url
	input.Label = label
	return input, nil
}
func (a *App) CreateSource(ctx context.Context, actor auth.AdminActor, id uuid.UUID, expected int64, input SourceInput) (Revision, error) {
	return a.mutate(ctx, actor, id, expected, auth.Editorial, func(q *sqlc.Queries, row sqlc.LockResourceCoreRow, _ []auth.Role) (bool, error) {
		n, err := normalizeSource(input)
		if err != nil {
			return false, err
		}
		sourceID := dbID(uuid.NewV7())
		if n.Primary {
			if err = q.CurationClearPrimary(ctx, sqlc.CurationClearPrimaryParams{ResourceID: row.ID, ID: sourceID}); err != nil {
				return false, err
			}
		}
		return true, q.CurationCreateSource(ctx, sqlc.CurationCreateSourceParams{ID: sourceID, ResourceID: row.ID, Url: n.URL, Label: dbText(n.Label), SourceType: string(n.Type), AvailabilityState: string(n.Availability), IsPrimary: n.Primary})
	})
}
func (a *App) PatchSource(ctx context.Context, actor auth.AdminActor, id uuid.UUID, expected int64, sourceID uuid.UUID, input SourcePatch) (Revision, error) {
	return a.mutate(ctx, actor, id, expected, auth.Editorial, func(q *sqlc.Queries, row sqlc.LockResourceCoreRow, _ []auth.Role) (bool, error) {
		old, err := q.CurationGetSource(ctx, sqlc.CurationGetSourceParams{ID: dbID(sourceID), ResourceID: row.ID})
		if err != nil {
			return false, err
		}
		next := SourceInput{URL: old.Url, Label: textPointer(old.Label), Type: resource.SourceType(old.SourceType), Availability: resource.SourceAvailabilityState(old.AvailabilityState), Primary: old.IsPrimary}
		if input.URL != nil {
			next.URL = *input.URL
		}
		if input.Label.Present {
			next.Label = input.Label.Value
		}
		if input.Type != nil {
			next.Type = *input.Type
		}
		if input.Availability != nil {
			next.Availability = *input.Availability
		}
		if input.Primary != nil {
			next.Primary = *input.Primary
		}
		next, err = normalizeSource(next)
		if err != nil {
			return false, err
		}
		if next.Primary && !old.IsPrimary {
			if err = q.CurationClearPrimary(ctx, sqlc.CurationClearPrimaryParams{ResourceID: row.ID, ID: old.ID}); err != nil {
				return false, err
			}
		}
		n, err := q.CurationUpdateSource(ctx, sqlc.CurationUpdateSourceParams{ID: old.ID, Url: next.URL, Label: dbText(next.Label), SourceType: string(next.Type), AvailabilityState: string(next.Availability), IsPrimary: next.Primary})
		return n > 0, err
	})
}
func (a *App) SetSourceRights(ctx context.Context, actor auth.AdminActor, id uuid.UUID, expected int64, sourceID uuid.UUID, state resource.SourceRightsStatus) (Revision, error) {
	return a.mutate(ctx, actor, id, expected, auth.Administration, func(q *sqlc.Queries, row sqlc.LockResourceCoreRow, _ []auth.Role) (bool, error) {
		if !state.Valid() {
			return false, ErrValidation
		}
		old, err := q.CurationGetSource(ctx, sqlc.CurationGetSourceParams{ID: dbID(sourceID), ResourceID: row.ID})
		if err != nil {
			return false, err
		}
		n, err := q.CurationSourceRights(ctx, sqlc.CurationSourceRightsParams{ID: old.ID, RightsStatus: string(state)})
		return n > 0, err
	})
}
