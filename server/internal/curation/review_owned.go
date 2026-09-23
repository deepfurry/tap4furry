package curation

import (
	"context"
	"errors"
	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/deepfurry/tap4furry/server/internal/resource"
	"github.com/jackc/pgx/v5"
	"slices"
	"uuid"
)

// ReviewedOwnedChange is deliberately closed; callers cannot supply SQL or work callbacks.
type ReviewedOwnedChange struct {
	Kind         string
	Source       *SourceInput
	SourceID     uuid.UUID
	Tags         []uuid.UUID
	Relation     *resource.Relation
	OtherID      uuid.UUID
	OtherVersion int64
	Locale       string
	Localization *LocalizationInput
}
type ReviewedOwnedResult struct {
	Revisions []Revision
	SourceID  uuid.UUID
}

// ApplyReviewedOwnedTx never commits. Canonical writes and the proposal decision
// share the caller's transaction, but authorization remains mandatory here.
func ApplyReviewedOwnedTx(ctx context.Context, tx pgx.Tx, actor auth.AdminActor, anchor uuid.UUID, expected int64, in ReviewedOwnedChange) (ReviewedOwnedResult, error) {
	var out ReviewedOwnedResult
	if _, err := auth.RequireAdminCapabilityTx(ctx, tx, actor, auth.Editorial); err != nil {
		return out, err
	}
	q := sqlc.New(tx)
	ids := []uuid.UUID{anchor}
	versions := map[uuid.UUID]int64{anchor: expected}
	if in.Kind == "add_relation" {
		if in.Relation == nil || in.OtherID == anchor || in.OtherID == uuid.Nil() || in.OtherVersion < 1 {
			return out, ErrValidation
		}
		if !((in.Relation.Source == anchor && in.Relation.Target == in.OtherID) || (in.Relation.Target == anchor && in.Relation.Source == in.OtherID)) {
			return out, ErrValidation
		}
		if err := q.CurationLockRelationGraph(ctx); err != nil {
			return out, safe(err)
		}
		ids = append(ids, in.OtherID)
		versions[in.OtherID] = in.OtherVersion
	}
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	categories := []uuid.UUID{}
	for _, key := range ids {
		r, err := lockedResource(ctx, q, key, versions[key])
		if err != nil {
			return out, err
		}
		if r.PublicationState != "published" {
			return out, resource.ErrVersionConflict
		}
		categories = append(categories, uuid.UUID(r.CategoryID.Bytes))
	}
	slices.SortFunc(categories, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	categories = slices.Compact(categories)
	for _, key := range categories {
		if _, err := q.CurationLockCategory(ctx, dbID(key)); err != nil {
			return out, safe(err)
		}
	}
	for _, key := range ids {
		r, err := q.ContributionResource(ctx, sqlc.ContributionResourceParams{ID: dbID(key)})
		if err != nil {
			return out, safe(err)
		}
		if !r.CanonicalOk {
			return out, errors.New("canonical localization missing")
		}
		if in.Kind == "add_translation" && r.DefaultLocale == in.Locale {
			return out, ErrValidation
		}
	}
	var err error
	switch in.Kind {
	case "add_source":
		if in.Source == nil || in.Source.Primary || !slices.Contains([]resource.SourceAvailabilityState{"active", "unavailable", "broken"}, in.Source.Availability) {
			return out, ErrValidation
		}
		s, e := normalizeSource(*in.Source)
		if e != nil {
			return out, safe(e)
		}
		out.SourceID = uuid.NewV7()
		err = q.CurationCreateSource(ctx, sqlc.CurationCreateSourceParams{ID: dbID(out.SourceID), ResourceID: dbID(anchor), Url: s.URL, Label: dbText(s.Label), SourceType: string(s.Type), AvailabilityState: string(s.Availability)})
	case "remove_broken_source":
		if _, err = q.ContributionPublicSource(ctx, sqlc.ContributionPublicSourceParams{ID: dbID(in.SourceID), ResourceID: dbID(anchor)}); err != nil {
			return out, safe(err)
		}
		old, e := q.CurationGetSource(ctx, sqlc.CurationGetSourceParams{ID: dbID(in.SourceID), ResourceID: dbID(anchor)})
		if e != nil {
			return out, safe(e)
		}
		var n int64
		n, err = q.CurationUpdateSource(ctx, sqlc.CurationUpdateSourceParams{ID: old.ID, Url: old.Url, Label: old.Label, SourceType: old.SourceType, AvailabilityState: "removed", IsPrimary: old.IsPrimary})
		if err == nil && n == 0 {
			return out, ErrValidation
		}
		out.SourceID = in.SourceID
	case "add_tag":
		tags := slices.Clone(in.Tags)
		slices.SortFunc(tags, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
		tags = slices.Compact(tags)
		if len(tags) < 1 || len(tags) > 10 {
			return out, ErrValidation
		}
		for _, tag := range tags {
			t, e := q.CurationLockTag(ctx, dbID(tag))
			if e != nil {
				return out, safe(e)
			}
			if t.State != "active" {
				return out, ErrValidation
			}
			state, e := q.ContributionTagState(ctx, sqlc.ContributionTagStateParams{ID: t.ID, ResourceID: dbID(anchor)})
			if e != nil {
				return out, safe(e)
			}
			if !state.CanonicalOk {
				return out, errors.New("canonical localization missing")
			}
			if state.Bound {
				return out, ErrValidation
			}
			if err = q.CurationAddTag(ctx, sqlc.CurationAddTagParams{ResourceID: dbID(anchor), TagID: t.ID}); err != nil {
				return out, safe(err)
			}
		}
	case "add_relation":
		r, e := resource.CanonicalRelation(in.Relation.Source, in.Relation.Target, in.Relation.Type)
		if e != nil {
			return out, ErrValidation
		}
		if r.Type != resource.RelatedTo {
			cycle, e := q.CurationRelationCycle(ctx, sqlc.CurationRelationCycleParams{SourceID: dbID(r.Source), TargetID: dbID(r.Target), RelationType: string(r.Type)})
			if e != nil {
				return out, safe(e)
			}
			if cycle {
				return out, ErrRelationCycle
			}
		}
		var n int64
		n, err = q.CurationAddRelation(ctx, sqlc.CurationAddRelationParams{ID: dbID(uuid.NewV7()), SourceResourceID: dbID(r.Source), TargetResourceID: dbID(r.Target), RelationType: string(r.Type)})
		if err == nil && n == 0 {
			return out, ErrConflict
		}
	case "add_translation":
		if in.Localization == nil {
			return out, ErrValidation
		}
		l, e := normalizeLocalization(in.Locale, *in.Localization)
		if e != nil {
			return out, safe(e)
		}
		var n int64
		n, err = putLocalization(ctx, q, anchor, l)
		if err == nil && n == 0 {
			return out, ErrValidation
		}
	default:
		return out, ErrValidation
	}
	if err != nil {
		return out, safe(err)
	}
	for _, key := range ids {
		rev, e := bump(ctx, q, key, versions[key])
		if e != nil {
			return out, safe(e)
		}
		out.Revisions = append(out.Revisions, rev)
	}
	return out, nil
}
