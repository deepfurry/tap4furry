package contribution

import (
	"context"
	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/curation"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/deepfurry/tap4furry/server/internal/resource"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"reflect"
	"time"
	"uuid"
)

func (a *App) acceptChange(ctx context.Context, actor auth.AdminActor, key uuid.UUID, input AcceptInput) error {
	if !reflect.DeepEqual(input.Content, Content{}) {
		return ErrValidation
	}
	message, err := optional(input.Message, 2000)
	if err != nil {
		return err
	}
	note, err := optional(input.InternalNote, 2000)
	if err != nil {
		return err
	}
	return a.transact(ctx, adminCheck(ctx, actor), func(tx pgx.Tx, q *sqlc.Queries, _ time.Time) error {
		row, e := q.ContributionLock(ctx, id(key))
		if e != nil {
			return e
		}
		if uid(row.AuthorID) == actor.UserID {
			return ErrForbidden
		}
		if row.Status != "pending" {
			return ErrConflict
		}
		if !Extended(row.Kind) {
			return ErrValidation
		}
		final, e := normalizeChange(row.Kind, input.Change, true)
		if e != nil {
			return e
		}
		snapshots, e := readChanges(ctx, q, key, row.Kind)
		if e != nil {
			return e
		}
		proposed := snapshots["proposed"]
		if !equalChange(row.Kind, proposed, final) && message == nil {
			return ErrValidation
		}
		if row.Kind == RemoveSource && final.SourceID != proposed.SourceID {
			return ErrValidation
		}
		if row.Kind == AddTranslation && final.Translation.Locale != proposed.Translation.Locale {
			return ErrValidation
		}
		if row.Kind == AddRelation {
			r, p := final.Relation, proposed.Relation
			if r.OtherID != p.OtherID || r.Direction != p.Direction || (r.Type == resource.RelatedTo) != (p.Type == resource.RelatedTo) {
				return ErrValidation
			}
			r.OtherVersion = p.OtherVersion
		}
		canonical := curation.ReviewedOwnedChange{Kind: row.Kind, SourceID: final.SourceID, Tags: final.Tags}
		if s := final.Source; s != nil {
			canonical.Source = &curation.SourceInput{URL: s.URL, Label: s.Label, Type: s.Type, Availability: s.Availability}
		}
		if r := final.Relation; r != nil {
			edge, e := canonicalRelation(uid(row.TargetResourceID), r)
			if e != nil {
				return e
			}
			canonical.Relation = &edge
			canonical.OtherID = r.OtherID
			canonical.OtherVersion = r.OtherVersion
		}
		if t := final.Translation; t != nil {
			canonical.Locale = t.Locale
			canonical.Localization = &curation.LocalizationInput{Name: *t.Name, Summary: t.Summary.Value, Description: t.Description.Value}
		}
		result, e := curation.ApplyReviewedOwnedTx(ctx, tx, actor, uid(row.TargetResourceID), row.BaseVersion.Int64, canonical)
		if e != nil {
			return e
		}
		var mainVersion int64
		for _, rev := range result.Revisions {
			if rev.ID == uid(row.TargetResourceID) {
				mainVersion = rev.Version
			}
			if final.Relation != nil {
				if rev.ID == final.Relation.OtherID {
					final.Relation.OtherResultVersion = rev.Version
				} else {
					final.Relation.AnchorResultVersion = rev.Version
				}
			}
		}
		if mainVersion == 0 {
			return ErrCanonical
		}
		if row.Kind == AddSource {
			final.SourceID = result.SourceID
		}
		if final.Translation != nil {
			final.Translation.Exists = true
		}
		if e = putChange(ctx, q, key, row.Kind, "accepted", final); e != nil {
			return e
		}
		if _, e = q.ContributionDecide(ctx, sqlc.ContributionDecideParams{ID: id(key), Status: "accepted", ResultResourceID: row.TargetResourceID, ResultVersion: pgtype.Int8{Int64: mainVersion, Valid: true}}); e != nil {
			return e
		}
		if e = q.ContributionReviewEvent(ctx, sqlc.ContributionReviewEventParams{ID: id(key), EventType: "accepted", ActorID: id(actor.UserID), Message: txt(message), InternalNote: txt(note)}); e != nil {
			return e
		}
		if e = q.ContributionAudit(ctx, sqlc.ContributionAuditParams{ID: id(key), ActorID: id(actor.UserID), ResourceID: row.TargetResourceID, BeforeVersion: row.BaseVersion, AfterVersion: pgtype.Int8{Int64: mainVersion, Valid: true}}); e != nil {
			return e
		}
		for _, rev := range result.Revisions {
			if e = q.ContributionAuditResource(ctx, sqlc.ContributionAuditResourceParams{ContributionID: id(key), ResourceID: id(rev.ID), BeforeVersion: rev.Version - 1, AfterVersion: rev.Version}); e != nil {
				return e
			}
		}
		return nil
	})
}
