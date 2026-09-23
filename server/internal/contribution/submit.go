package contribution

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"math"
	"time"
	"uuid"

	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/deepfurry/tap4furry/server/internal/governance"
	"github.com/deepfurry/tap4furry/server/internal/resource"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type EditContext struct {
	ResourceID   uuid.UUID
	BaseRevision string
	Content      Content
	Change       *Change
	Reference    *Content
	Other        *Result
}

func (a *App) Context(ctx context.Context, actor auth.Actor, slug string) (EditContext, error) {
	var out EditContext
	if resource.ValidateSlug(slug) != nil {
		return out, ErrValidation
	}
	err := a.snapshot(ctx, publicCheck(ctx, actor), func(_ pgx.Tx, q *sqlc.Queries, _ time.Time) error {
		row, err := q.ContributionResource(ctx, sqlc.ContributionResourceParams{Slug: str(slug)})
		if err != nil {
			return err
		}
		out.Content, err = resourceContent(row)
		if err != nil {
			return err
		}
		out.ResourceID = uid(row.ID)
		out.BaseRevision = a.revision(actor.UserID, row)
		if out.BaseRevision == "" {
			return ErrCanonical
		}
		return nil
	})
	return out, err
}
func retry(t, now time.Time) int { return max(1, int(math.Ceil(t.Sub(now).Seconds()))) }
func checkQuota(quota sqlc.ContributionQuotaRow, now time.Time, budget governance.Budget) error {
	if quota.Pending >= budget.Pending {
		return &LimitError{Reason: "pending_limit"}
	}
	if quota.Recent >= budget.Daily {
		return &LimitError{Reason: "daily_limit", RetryAfter: retry(quota.FirstAt.Time.Add(24*time.Hour), now)}
	}
	if quota.LastAt.Valid && now.Before(quota.LastAt.Time.Add(budget.Interval)) {
		return &LimitError{Reason: "submission_interval", RetryAfter: retry(quota.LastAt.Time.Add(budget.Interval), now)}
	}
	return nil
}
func (a *App) Submit(ctx context.Context, actor auth.Actor, input SubmitInput) (uuid.UUID, error) {
	var result uuid.UUID
	in, err := normalizeInput(input)
	if err != nil {
		return result, err
	}
	serialized, err := json.Marshal(in)
	if err != nil {
		return result, ErrValidation
	}
	fingerprint := sha256.Sum256(serialized)
	err = a.transact(ctx, publicCheck(ctx, actor), func(_ pgx.Tx, q *sqlc.Queries, now time.Time) error {
		verified, err := q.ContributionVerified(ctx, id(actor.UserID))
		if err != nil {
			return err
		}
		if !verified {
			return ErrVerified
		}
		prior, err := q.ContributionByRequest(ctx, sqlc.ContributionByRequestParams{AuthorID: id(actor.UserID), RequestID: id(in.RequestID)})
		if err == nil {
			if !hmac.Equal(prior.RequestFingerprint, fingerprint[:]) {
				return ErrRequestConflict
			}
			result = uid(prior.ID)
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err = governance.CheckTx(ctx, q, actor.UserID, governance.ContributionSubmit, now); err != nil {
			return err
		}
		budget, err := governance.BudgetTx(ctx, q, actor.UserID)
		if err != nil {
			return err
		}
		quota, err := q.ContributionQuota(ctx, sqlc.ContributionQuotaParams{AuthorID: id(actor.UserID), Now: stamp(now)})
		if err != nil {
			return err
		}
		if err = checkQuota(quota, now, budget); err != nil {
			return err
		}
		if in.PreviousID != uuid.Nil() {
			if _, err = q.ContributionOwned(ctx, sqlc.ContributionOwnedParams{ID: id(in.PreviousID), AuthorID: id(actor.UserID)}); err != nil {
				return err
			}
		}
		if Extended(in.Kind) {
			result, err = a.submitChange(ctx, q, actor, in)
			return err
		}
		base := Content{Lifecycle: resource.Unknown}
		var version pgtype.Int8
		if in.Kind == Update {
			row, err := q.ContributionResource(ctx, sqlc.ContributionResourceParams{ID: id(in.TargetID)})
			if err != nil {
				return err
			}
			base, err = resourceContent(row)
			if err != nil {
				return err
			}
			revision := a.revision(actor.UserID, row)
			if revision == "" || !hmac.Equal([]byte(revision), []byte(in.BaseRevision)) {
				return resource.ErrVersionConflict
			}
			version = pgtype.Int8{Int64: row.Version, Valid: true}
		}
		proposed, err := apply(base, in.Content)
		if err != nil {
			return err
		}
		if in.Kind == Update && (proposed.DefaultLocale != base.DefaultLocale || equal(base, proposed)) {
			return ErrValidation
		}
		if in.Kind == Create && proposed.Source == nil && proposed.Summary == nil {
			return ErrValidation
		}
		if err = categoryOK(ctx, q, proposed.CategoryID, base.CategoryID); err != nil {
			return err
		}
		result = uuid.NewV7()
		_, err = q.ContributionCreate(ctx, sqlc.ContributionCreateParams{ID: id(result), AuthorID: id(actor.UserID), Kind: in.Kind, TargetResourceID: id(in.TargetID), BaseVersion: version, Reason: in.Reason, PreviousID: id(in.PreviousID), RequestID: id(in.RequestID), RequestFingerprint: fingerprint[:], SubmittedFields: submittedFields(in)})
		if err != nil {
			return err
		}
		if in.Kind == Update {
			if err = putContent(ctx, q, result, "base", base); err != nil {
				return err
			}
		}
		if err = putContent(ctx, q, result, "proposed", proposed); err != nil {
			return err
		}
		return q.ContributionPublicEvent(ctx, sqlc.ContributionPublicEventParams{ID: id(result), EventType: "submitted", ActorID: id(actor.UserID)})
	})
	return result, err
}
func (a *App) Withdraw(ctx context.Context, actor auth.Actor, key uuid.UUID) error {
	return a.transact(ctx, publicCheck(ctx, actor), func(_ pgx.Tx, q *sqlc.Queries, _ time.Time) error {
		row, err := q.ContributionLockOwned(ctx, sqlc.ContributionLockOwnedParams{ID: id(key), AuthorID: id(actor.UserID)})
		if err != nil {
			return err
		}
		if row.Status != "pending" {
			return ErrConflict
		}
		n, err := q.ContributionWithdraw(ctx, sqlc.ContributionWithdrawParams{ID: id(key), AuthorID: id(actor.UserID)})
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrConflict
		}
		return q.ContributionPublicEvent(ctx, sqlc.ContributionPublicEventParams{ID: id(key), EventType: "withdrawn", ActorID: id(actor.UserID)})
	})
}
