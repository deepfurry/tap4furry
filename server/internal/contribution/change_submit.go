package contribution

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/deepfurry/tap4furry/server/internal/resource"
	"github.com/deepfurry/tap4furry/server/internal/taxonomy"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"time"
	"uuid"
)

func (a *App) changeRevision(author uuid.UUID, row sqlc.ContributionResourceRow, kind string, c *Change) string {
	if a.contextSecret == "" {
		return ""
	}
	scope := struct {
		Author, Target      uuid.UUID
		Version             int64
		DefaultLocale, Kind string
		Source              uuid.UUID
		Locale              string
		Other               uuid.UUID
		OtherVersion        int64
		Direction, Type     string
	}{Author: author, Target: uid(row.ID), Version: row.Version, DefaultLocale: row.DefaultLocale, Kind: kind}
	if c != nil {
		scope.Source = c.SourceID
		if c.Translation != nil {
			scope.Locale = c.Translation.Locale
		}
		if r := c.Relation; r != nil {
			scope.Other = r.OtherID
			scope.OtherVersion = r.OtherVersion
			scope.Direction = r.Direction
			scope.Type = string(r.Type)
		}
	}
	data, _ := json.Marshal(scope)
	m := hmac.New(sha256.New, []byte(a.contextSecret))
	m.Write([]byte("tap4furry/contribution-context/v2\x00"))
	m.Write(data)
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// snapshot reads establish one MVCC view before any canonical/child projection.
// Recheck authorization in READ COMMITTED before returning: a repeatable-read
// snapshot can predate a role/session revocation that held the User lock first.
// Keep the two transactions sequential so no second pool connection is held.
func (a *App) snapshot(ctx context.Context, check func(pgx.Tx) (time.Time, error), work func(pgx.Tx, *sqlc.Queries, time.Time) error) error {
	if err := a.transactLevel(ctx, pgx.RepeatableRead, check, work); err != nil {
		return err
	}
	return a.transact(ctx, check, func(pgx.Tx, *sqlc.Queries, time.Time) error { return nil })
}
func (a *App) ContextFor(ctx context.Context, actor auth.Actor, slug string, input ContextInput) (EditContext, error) {
	if input.Kind == "" || input.Kind == Update {
		if input.SourceID != uuid.Nil() || input.Locale != "" || input.OtherSlug != "" || input.Direction != "" || input.RelationType != "" {
			return EditContext{}, ErrValidation
		}
		return a.Context(ctx, actor, slug)
	}
	var out EditContext
	if !Extended(input.Kind) || resource.ValidateSlug(slug) != nil {
		return out, ErrValidation
	}
	if (input.Kind != RemoveSource && input.SourceID != uuid.Nil()) || (input.Kind != AddTranslation && input.Locale != "") || (input.Kind != AddRelation && (input.OtherSlug != "" || input.Direction != "" || input.RelationType != "")) {
		return out, ErrValidation
	}
	err := a.snapshot(ctx, publicCheck(ctx, actor), func(_ pgx.Tx, q *sqlc.Queries, _ time.Time) error {
		row, e := q.ContributionResource(ctx, sqlc.ContributionResourceParams{Slug: str(slug)})
		if e != nil {
			return e
		}
		canonical, e := resourceContent(row)
		if e != nil {
			return e
		}
		c := &Change{}
		switch input.Kind {
		case RemoveSource:
			if input.SourceID == uuid.Nil() {
				return ErrValidation
			}
			c.SourceID = input.SourceID
		case AddRelation:
			if resource.ValidateSlug(input.OtherSlug) != nil {
				return ErrValidation
			}
			other, e := q.ContributionResource(ctx, sqlc.ContributionResourceParams{Slug: str(input.OtherSlug)})
			if e != nil {
				return e
			}
			if _, e = resourceContent(other); e != nil {
				return e
			}
			c.Relation = &RelationChange{OtherID: uid(other.ID), Type: input.RelationType, Direction: input.Direction}
			c, e = normalizeChange(AddRelation, c, false)
			if e != nil {
				return e
			}
			out.Other = &Result{uid(other.ID), other.Slug, other.Name.String}
		case AddTranslation:
			locale, e := taxonomy.ParseLocale(input.Locale)
			if e != nil {
				return ErrValidation
			}
			c.Translation = &TranslationChange{Locale: string(locale)}
		}
		base, e := changeBase(ctx, q, row, input.Kind, c)
		if e != nil {
			return e
		}
		out.ResourceID = uid(row.ID)
		out.Change = base
		if input.Kind == AddSource || input.Kind == AddTag {
			out.Change = nil
		}
		out.BaseRevision = a.changeRevision(actor.UserID, row, input.Kind, base)
		if out.BaseRevision == "" {
			return ErrCanonical
		}
		if input.Kind == AddTranslation {
			out.Reference = &canonical
		}
		return nil
	})
	return out, err
}

func changeBase(ctx context.Context, q *sqlc.Queries, row sqlc.ContributionResourceRow, kind string, c *Change) (*Change, error) {
	b := &Change{}
	switch kind {
	case AddSource: // There is no canonical Source for a new URL.
	case RemoveSource:
		s, e := q.ContributionPublicSource(ctx, sqlc.ContributionPublicSourceParams{ID: id(c.SourceID), ResourceID: row.ID})
		if e != nil {
			return nil, e
		}
		b.SourceID = uid(s.ID)
		b.Source = &Source{URL: s.Url, Label: ptr(s.Label), Type: resource.SourceType(s.SourceType), Availability: resource.SourceAvailabilityState(s.AvailabilityState)}
	case AddTag:
		b.Tags = []uuid.UUID{}
		for _, tag := range c.Tags {
			state, e := q.ContributionTagState(ctx, sqlc.ContributionTagStateParams{ID: id(tag), ResourceID: row.ID})
			if e != nil {
				return nil, e
			}
			if !state.CanonicalOk {
				return nil, ErrCanonical
			}
			if state.Bound {
				b.Tags = append(b.Tags, tag)
			}
		}
	case AddRelation:
		if c.Relation.OtherID == uid(row.ID) {
			return nil, ErrValidation
		}
		r := *c.Relation
		other, e := q.ContributionResource(ctx, sqlc.ContributionResourceParams{ID: id(r.OtherID)})
		if e != nil {
			return nil, e
		}
		if _, e = resourceContent(other); e != nil {
			return nil, e
		}
		r.OtherVersion = other.Version
		b.Relation = &r
	case AddTranslation:
		t := &TranslationChange{Locale: c.Translation.Locale, Fields: 7, Summary: NullableText{Set: true}, Description: NullableText{Set: true}}
		if t.Locale == row.DefaultLocale {
			return nil, ErrValidation
		}
		r, e := q.ContributionTranslation(ctx, sqlc.ContributionTranslationParams{ResourceID: row.ID, Locale: t.Locale})
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return nil, e
		}
		if e == nil {
			t.Exists = true
			t.Name = &r.Name
			t.Summary.Value = ptr(r.Summary)
			t.Description.Value = ptr(r.Description)
		}
		b.Translation = t
	default:
		return nil, ErrValidation
	}
	return b, nil
}
func validateChange(ctx context.Context, q *sqlc.Queries, target uuid.UUID, kind string, c *Change) error {
	switch kind {
	case AddSource:
		exists, e := q.ContributionSourceURLExists(ctx, sqlc.ContributionSourceURLExistsParams{ResourceID: id(target), Url: c.Source.URL})
		if e != nil {
			return e
		}
		if exists {
			return ErrConflict
		}
	case AddTag:
		for _, tag := range c.Tags {
			r, e := q.ContributionTagState(ctx, sqlc.ContributionTagStateParams{ID: id(tag), ResourceID: id(target)})
			if e != nil {
				return e
			}
			if !r.CanonicalOk {
				return ErrCanonical
			}
			if r.State != "active" || r.Bound {
				return ErrValidation
			}
		}
	case AddRelation:
		r, e := canonicalRelation(target, c.Relation)
		if e != nil {
			return e
		}
		exists, e := q.ContributionRelationExists(ctx, sqlc.ContributionRelationExistsParams{SourceResourceID: id(r.Source), TargetResourceID: id(r.Target), RelationType: string(r.Type)})
		if e != nil {
			return e
		}
		if exists {
			return ErrConflict
		}
	}
	return nil
}
func canonicalRelation(anchor uuid.UUID, r *RelationChange) (resource.Relation, error) {
	from, to := anchor, r.OtherID
	if r.Direction == "incoming" {
		from, to = to, from
	}
	v, e := resource.CanonicalRelation(from, to, r.Type)
	if e != nil {
		return v, ErrValidation
	}
	return v, nil
}
func mergeTranslation(base, patch *TranslationChange) (*TranslationChange, error) {
	t := *base
	t.Fields = patch.Fields
	if patch.Name != nil {
		t.Name = patch.Name
	}
	if patch.Summary.Set {
		t.Summary.Value = patch.Summary.Value
	}
	if patch.Description.Set {
		t.Description.Value = patch.Description.Value
	}
	t.Summary.Set = patch.Summary.Set
	t.Description.Set = patch.Description.Set
	if t.Name == nil {
		return nil, ErrValidation
	}
	t.Exists = true
	return &t, nil
}
func (a *App) submitChange(ctx context.Context, q *sqlc.Queries, actor auth.Actor, in SubmitInput) (uuid.UUID, error) {
	row, err := q.ContributionResource(ctx, sqlc.ContributionResourceParams{ID: id(in.TargetID)})
	if err != nil {
		return uuid.Nil(), err
	}
	if _, err = resourceContent(row); err != nil {
		return uuid.Nil(), err
	}
	base, err := changeBase(ctx, q, row, in.Kind, in.Change)
	if err != nil {
		return uuid.Nil(), err
	}
	mac := a.changeRevision(actor.UserID, row, in.Kind, base)
	if mac == "" || !hmac.Equal([]byte(mac), []byte(in.BaseRevision)) {
		return uuid.Nil(), resource.ErrVersionConflict
	}
	proposed := *in.Change
	if proposed.Relation != nil {
		r := *base.Relation
		proposed.Relation = &r
	}
	if proposed.Translation != nil {
		proposed.Translation, err = mergeTranslation(base.Translation, proposed.Translation)
		if err != nil {
			return uuid.Nil(), err
		}
		if equalChange(in.Kind, base, &proposed) {
			return uuid.Nil(), ErrValidation
		}
	}
	if err = validateChange(ctx, q, in.TargetID, in.Kind, &proposed); err != nil {
		return uuid.Nil(), err
	}
	// Public cannot lock canonical rows. Recheck the monotonic revision after
	// reading children so a concurrent mutation cannot produce a mixed baseline.
	current, err := q.ContributionResource(ctx, sqlc.ContributionResourceParams{ID: row.ID})
	if err != nil {
		return uuid.Nil(), err
	}
	if !current.CanonicalOk {
		return uuid.Nil(), ErrCanonical
	}
	if current.Version != row.Version {
		return uuid.Nil(), resource.ErrVersionConflict
	}
	if r := proposed.Relation; r != nil {
		other, e := q.ContributionResource(ctx, sqlc.ContributionResourceParams{ID: id(r.OtherID)})
		if e != nil {
			return uuid.Nil(), e
		}
		if !other.CanonicalOk {
			return uuid.Nil(), ErrCanonical
		}
		if other.Version != r.OtherVersion {
			return uuid.Nil(), resource.ErrVersionConflict
		}
	}
	// The enclosing submit transaction already serialized replay, quotas and identity.
	serialized, err := json.Marshal(in)
	if err != nil {
		return uuid.Nil(), ErrValidation
	}
	digest := sha256.Sum256(serialized)
	key := uuid.NewV7()
	_, err = q.ContributionCreate(ctx, sqlc.ContributionCreateParams{ID: id(key), AuthorID: id(actor.UserID), Kind: in.Kind, TargetResourceID: row.ID, BaseVersion: pgtype.Int8{Int64: row.Version, Valid: true}, Reason: in.Reason, PreviousID: id(in.PreviousID), RequestID: id(in.RequestID), RequestFingerprint: digest[:]})
	if err != nil {
		return uuid.Nil(), err
	}
	if err = putChange(ctx, q, key, in.Kind, "base", base); err != nil {
		return uuid.Nil(), err
	}
	if err = putChange(ctx, q, key, in.Kind, "proposed", &proposed); err != nil {
		return uuid.Nil(), err
	}
	err = q.ContributionPublicEvent(ctx, sqlc.ContributionPublicEventParams{ID: id(key), EventType: "submitted", ActorID: id(actor.UserID)})
	return key, err
}
