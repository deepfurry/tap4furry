package contribution

import (
	"context"
	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/curation"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"time"
	"uuid"
)

func (a *App) Accept(ctx context.Context, actor auth.AdminActor, key uuid.UUID, input AcceptInput) error {
	final, err := normalizeContent(input.Content)
	if err != nil {
		return err
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
		row, err := q.ContributionLock(ctx, id(key))
		if err != nil {
			return err
		}
		if uid(row.AuthorID) == actor.UserID {
			return ErrForbidden
		}
		if row.Status != "pending" {
			return ErrConflict
		}
		contents, err := readContents(ctx, q, key)
		if err != nil {
			return err
		}
		proposed := contents["proposed"]
		if !equal(proposed, final) && message == nil {
			return ErrValidation
		}
		base := contents["base"]
		if row.Kind == Update {
			if final.DefaultLocale != base.DefaultLocale || final.Slug != base.Slug || final.Source != nil || equal(base, final) {
				return ErrValidation
			}
		} else if final.Source == nil && final.Summary == nil {
			return ErrValidation
		}
		var source *curation.SourceInput
		if final.Source != nil {
			source = &curation.SourceInput{URL: final.Source.URL, Label: final.Source.Label, Type: final.Source.Type, Availability: final.Source.Availability, Primary: true}
		}
		revision, err := curation.ApplyReviewedTx(ctx, tx, actor, uid(row.TargetResourceID), row.BaseVersion.Int64, base.DefaultLocale, curation.CreateInput{Slug: final.Slug, DefaultLocale: final.DefaultLocale, CategoryID: final.CategoryID, Lifecycle: final.Lifecycle, ContentRating: final.ContentRating, Localization: curation.LocalizationInput{Name: final.Name, Summary: final.Summary, Description: final.Description}}, source)
		if err != nil {
			return err
		}
		if err = putContent(ctx, q, key, "accepted", final); err != nil {
			return err
		}
		if _, err = q.ContributionDecide(ctx, sqlc.ContributionDecideParams{ID: id(key), Status: "accepted", ResultResourceID: id(revision.ID), ResultVersion: pgtype.Int8{Int64: revision.Version, Valid: true}}); err != nil {
			return err
		}
		if err = q.ContributionReviewEvent(ctx, sqlc.ContributionReviewEventParams{ID: id(key), EventType: "accepted", ActorID: id(actor.UserID), Message: txt(message), InternalNote: txt(note)}); err != nil {
			return err
		}
		return q.ContributionAudit(ctx, sqlc.ContributionAuditParams{ID: id(key), ActorID: id(actor.UserID), ResourceID: id(revision.ID), BeforeVersion: row.BaseVersion, AfterVersion: pgtype.Int8{Int64: revision.Version, Valid: true}})
	})
}
func (a *App) Reject(ctx context.Context, actor auth.AdminActor, key uuid.UUID, message string, internalNote *string) error {
	msg, err := optional(&message, 2000)
	if err != nil || msg == nil {
		return ErrValidation
	}
	note, err := optional(internalNote, 2000)
	if err != nil {
		return err
	}
	return a.transact(ctx, adminCheck(ctx, actor), func(_ pgx.Tx, q *sqlc.Queries, _ time.Time) error {
		row, err := q.ContributionLock(ctx, id(key))
		if err != nil {
			return err
		}
		if uid(row.AuthorID) == actor.UserID {
			return ErrForbidden
		}
		if row.Status != "pending" {
			return ErrConflict
		}
		if _, err = q.ContributionDecide(ctx, sqlc.ContributionDecideParams{ID: id(key), Status: "rejected"}); err != nil {
			return err
		}
		if err = q.ContributionReviewEvent(ctx, sqlc.ContributionReviewEventParams{ID: id(key), EventType: "rejected", ActorID: id(actor.UserID), Message: txt(msg), InternalNote: txt(note)}); err != nil {
			return err
		}
		return q.ContributionAudit(ctx, sqlc.ContributionAuditParams{ID: id(key), ActorID: id(actor.UserID), ResourceID: row.TargetResourceID, BeforeVersion: row.BaseVersion})
	})
}
