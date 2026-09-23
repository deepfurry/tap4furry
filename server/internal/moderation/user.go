package moderation

import (
	"context"
	"crypto/hmac"
	"errors"
	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/deepfurry/tap4furry/server/internal/governance"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"slices"
	"time"
	"uuid"
)

type Quota struct{ DailyLimit, PendingLimit, IntervalSeconds, Remaining24h, Pending, RetryAfter int64 }

func quotaView(b governance.Budget, recent, pending int64, first, last pgtype.Timestamptz, now time.Time) Quota {
	out := Quota{DailyLimit: b.Daily, PendingLimit: b.Pending, IntervalSeconds: int64(b.Interval / time.Second), Remaining24h: max(0, b.Daily-recent), Pending: pending}
	if recent >= b.Daily && first.Valid {
		out.RetryAfter = int64(seconds(first.Time.Add(24*time.Hour), now))
	} else if last.Valid && now.Before(last.Time.Add(b.Interval)) {
		out.RetryAfter = int64(seconds(last.Time.Add(b.Interval), now))
	}
	return out
}

type OwnGovernance struct {
	Restrictions                   []sqlc.GovernanceEffectiveRestrictionsRow
	ContributionQuota, ReportQuota Quota
}

func (a *App) OwnGovernance(ctx context.Context, actor auth.Actor) (OwnGovernance, error) {
	var out OwnGovernance
	err := a.transact(ctx, publicCheck(ctx, actor), func(_ pgx.Tx, q *sqlc.Queries, now time.Time) error {
		var err error
		out.Restrictions, err = q.GovernanceEffectiveRestrictions(ctx, sqlc.GovernanceEffectiveRestrictionsParams{UserID: id(actor.UserID), Now: stamp(now)})
		if err != nil {
			return err
		}
		b, err := governance.BudgetTx(ctx, q, actor.UserID)
		if err != nil {
			return err
		}
		c, err := q.ContributionQuota(ctx, sqlc.ContributionQuotaParams{AuthorID: id(actor.UserID), Now: stamp(now)})
		if err != nil {
			return err
		}
		out.ContributionQuota = quotaView(b, c.Recent, c.Pending, c.FirstAt, c.LastAt, now)
		r, err := q.ReportQuota(ctx, sqlc.ReportQuotaParams{ReporterID: id(actor.UserID), Now: stamp(now)})
		if err != nil {
			return err
		}
		out.ReportQuota = quotaView(governance.ReportBudget(), r.Recent, r.Pending, r.FirstAt, r.LastAt, now)
		return nil
	})
	return out, err
}

type UserGovernance struct {
	UserID       uuid.UUID
	Trust        string
	Revision     int64
	Restrictions []sqlc.AppUserRestriction
}

func (a *App) UserGovernance(ctx context.Context, actor auth.AdminActor, target uuid.UUID) (UserGovernance, error) {
	out := UserGovernance{UserID: target}
	err := a.snapshot(ctx, adminCheck(ctx, actor, auth.Administration), func(q *sqlc.Queries) error {
		p, err := q.GovernanceProfile(ctx, id(target))
		if err != nil {
			return err
		}
		out.Trust = p.TrustLevel
		out.Revision = p.Revision
		out.Restrictions, err = q.GovernanceAdminRestrictions(ctx, id(target))
		return err
	})
	return out, err
}
func userCheck(ctx context.Context, actor auth.AdminActor, target uuid.UUID) check {
	return func(tx pgx.Tx) (time.Time, error) {
		if target == uuid.Nil() || target == actor.UserID {
			return time.Time{}, ErrForbidden
		}
		keys := []uuid.UUID{actor.UserID, target}
		slices.SortFunc(keys, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
		for _, key := range keys {
			if _, err := sqlc.New(tx).LockUserAuthState(ctx, id(key)); err != nil {
				return time.Time{}, err
			}
		}
		return adminCheck(ctx, actor, auth.Administration)(tx)
	}
}

type UserChange struct {
	RequestID                                                               uuid.UUID
	ExpectedRevision                                                        int64
	Kind, Trust, Scope, Duration, ReasonCode, Message, Reason, InternalNote string
	RestrictionID, ReplacesID                                               uuid.UUID
}

func normalizeUserChange(in UserChange) (UserChange, error) {
	if in.RequestID == uuid.Nil() || in.ExpectedRevision < 0 {
		return in, ErrValidation
	}
	var err error
	if in.InternalNote != "" {
		in.InternalNote, err = governance.Text(in.InternalNote, 2000)
		if err != nil {
			return in, err
		}
	}
	switch in.Kind {
	case "trust":
		if _, err = governance.ContributionBudget(in.Trust); err != nil {
			return in, err
		}
		in.Reason, err = governance.Text(in.Reason, 1000)
	case "restrict":
		if !governance.Scope(in.Scope).Valid() || !slices.Contains([]string{"24h", "7d", "30d", "indefinite"}, in.Duration) || !slices.Contains([]string{"spam", "abuse", "repeated_policy_violation", "other"}, in.ReasonCode) {
			return in, ErrValidation
		}
		in.Message, err = governance.Text(in.Message, 1000)
		in.Reason = in.Message
	case "revoke_restriction":
		if in.RestrictionID == uuid.Nil() {
			return in, ErrValidation
		}
		in.Reason, err = governance.Text(in.Reason, 1000)
	default:
		return in, ErrValidation
	}
	return in, err
}
func (a *App) ChangeUser(ctx context.Context, actor auth.AdminActor, target uuid.UUID, in UserChange) (uuid.UUID, error) {
	in, err := normalizeUserChange(in)
	if err != nil {
		return uuid.Nil(), err
	}
	out := target
	hash := digest(struct {
		Target uuid.UUID
		Input  UserChange
	}{target, in})
	err = a.transact(ctx, userCheck(ctx, actor, target), func(_ pgx.Tx, q *sqlc.Queries, now time.Time) error {
		replay, e := q.GovernanceActionByRequest(ctx, sqlc.GovernanceActionByRequestParams{ActorID: id(actor.UserID), RequestID: id(in.RequestID)})
		if e == nil {
			if !hmac.Equal(replay.RequestFingerprint, hash) {
				return ErrRequestConflict
			}
			out = uid(replay.ID)
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		profile, e := q.GovernanceProfile(ctx, id(target))
		if e != nil {
			return e
		}
		if profile.Revision != in.ExpectedRevision {
			return ErrConflict
		}
		level := profile.TrustLevel
		restriction := uuid.Nil()
		audit := governance.Change{Operation: in.Kind, UserID: target, BeforeVersion: profile.Revision, AfterVersion: profile.Revision + 1, Reason: in.Reason}
		switch in.Kind {
		case "trust":
			if in.Trust == level {
				return nil
			}
			audit.Fields = []string{"trust"}
			audit.BeforeTrust = level
			audit.AfterTrust = in.Trust
			level = in.Trust
		case "restrict":
			active, e := q.GovernanceEffectiveRestrictions(ctx, sqlc.GovernanceEffectiveRestrictionsParams{UserID: id(target), Now: stamp(now)})
			if e != nil {
				return e
			}
			replaces := in.ReplacesID == uuid.Nil()
			for _, r := range active {
				if r.Scope == in.Scope {
					if uid(r.ID) != in.ReplacesID {
						return ErrConflict
					}
					replaces = true
				}
			}
			if !replaces {
				return ErrConflict
			}
			if in.ReplacesID != uuid.Nil() {
				n, e := q.GovernanceRevokeRestriction(ctx, sqlc.GovernanceRevokeRestrictionParams{ID: id(in.ReplacesID), UserID: id(target), RevokedAt: stamp(now), RevokedBy: id(actor.UserID), RevokeReason: text(in.Reason)})
				if e != nil {
					return e
				}
				if n != 1 {
					return ErrConflict
				}
			}
			expiry := pgtype.Timestamptz{}
			duration := map[string]time.Duration{"24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour, "30d": 30 * 24 * time.Hour}[in.Duration]
			if duration > 0 {
				expiry = stamp(now.Add(duration))
			}
			restriction = uuid.NewV7()
			e = q.GovernanceInsertRestriction(ctx, sqlc.GovernanceInsertRestrictionParams{ID: id(restriction), UserID: id(target), Scope: in.Scope, ReasonCode: in.ReasonCode, UserMessage: in.Message, InternalNote: text(in.InternalNote), CreatedBy: id(actor.UserID), StartsAt: stamp(now), ExpiresAt: expiry})
			if e != nil {
				return e
			}
			audit.Fields = []string{"restrictions"}
		case "revoke_restriction":
			restriction = in.RestrictionID
			n, e := q.GovernanceRevokeRestriction(ctx, sqlc.GovernanceRevokeRestrictionParams{ID: id(restriction), UserID: id(target), RevokedAt: stamp(now), RevokedBy: id(actor.UserID), RevokeReason: text(in.Reason)})
			if e != nil {
				return e
			}
			if n != 1 {
				return ErrConflict
			}
			audit.Fields = []string{"restrictions"}
		}
		if e = q.GovernanceSetProfile(ctx, sqlc.GovernanceSetProfileParams{UserID: id(target), TrustLevel: level}); e != nil {
			return e
		}
		out = uuid.NewV7()
		audit.ActionID = out
		if e = q.GovernanceInsertAction(ctx, sqlc.GovernanceInsertActionParams{ID: id(out), ActorID: id(actor.UserID), Action: in.Kind, UserID: id(target), RestrictionID: id(restriction), Reason: in.Reason, InternalNote: text(in.InternalNote), RequestID: id(in.RequestID), RequestFingerprint: hash}); e != nil {
			return e
		}
		_, e = governance.Record(ctx, q, actor.UserID, audit)
		return e
	})
	return out, err
}
