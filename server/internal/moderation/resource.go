package moderation

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/curation"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/deepfurry/tap4furry/server/internal/governance"
	"github.com/deepfurry/tap4furry/server/internal/resource"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"slices"
	"time"
	"uuid"
)

type DistributionInput struct {
	RequestID       uuid.UUID
	ExpectedVersion int64
	Policy, Reason  string
}

func (a *App) SetDistribution(ctx context.Context, actor auth.AdminActor, key uuid.UUID, in DistributionInput) (uuid.UUID, error) {
	if in.RequestID == uuid.Nil() || (in.Policy != "normal" && in.Policy != "excluded") {
		return uuid.Nil(), ErrValidation
	}
	reason, err := governance.Text(in.Reason, 1000)
	if err != nil {
		return uuid.Nil(), err
	}
	change := curation.GovernanceInput{Kind: "distribution", State: in.Policy, Reason: reason, RequestID: in.RequestID}
	hash := curation.GovernanceFingerprint(key, in.ExpectedVersion, change)
	out := key
	err = a.transact(ctx, adminCheck(ctx, actor, auth.Administration), func(tx pgx.Tx, q *sqlc.Queries, _ time.Time) error {
		replay, e := q.GovernanceActionByRequest(ctx, sqlc.GovernanceActionByRequestParams{ActorID: id(actor.UserID), RequestID: id(in.RequestID)})
		if e == nil {
			if !hmac.Equal(replay.RequestFingerprint, hash) {
				return ErrRequestConflict
			}
			out = key
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		result, e := curation.ApplyGovernanceTx(ctx, tx, actor, key, in.ExpectedVersion, change)
		if e != nil {
			return e
		}
		if result.AuditID != uuid.Nil() {
			out = key
		}
		return nil
	})
	return out, err
}

type SourceCheckInput struct {
	RequestID                   uuid.UUID
	ExpectedVersion             int64
	Outcome, Note, Availability string
	ObservedAt                  time.Time
}

func (a *App) RecordSourceCheck(ctx context.Context, actor auth.AdminActor, key, source uuid.UUID, in SourceCheckInput) (uuid.UUID, error) {
	var out uuid.UUID
	var err error
	in.Note, err = governance.Text(in.Note, 2000)
	if err != nil || in.RequestID == uuid.Nil() || key == uuid.Nil() || source == uuid.Nil() || in.ExpectedVersion < 1 || in.ObservedAt.IsZero() || !slices.Contains([]string{"reachable", "unreachable", "uncertain"}, in.Outcome) || (in.Availability != "" && !resource.SourceAvailabilityState(in.Availability).Valid()) {
		return out, ErrValidation
	}
	in.ObservedAt = in.ObservedAt.UTC()
	hash := digest(struct {
		Resource, Source uuid.UUID
		Input            SourceCheckInput
	}{key, source, in})
	err = a.transact(ctx, adminCheck(ctx, actor, auth.AdminAccess), func(tx pgx.Tx, q *sqlc.Queries, now time.Time) error {
		// Every static Admin role can observe; only Editorial can request a mutation.
		if in.Availability != "" {
			if _, e := auth.RequireAdminCapabilityTx(ctx, tx, actor, auth.Editorial); e != nil {
				return e
			}
		}
		replay, e := q.SourceCheckByRequest(ctx, sqlc.SourceCheckByRequestParams{ActorID: id(actor.UserID), RequestID: id(in.RequestID)})
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
		if in.ObservedAt.After(now) {
			return ErrValidation
		}
		parent, e := q.LockResourceCore(ctx, id(key))
		if e != nil {
			return e
		}
		if parent.Version != in.ExpectedVersion {
			return resource.ErrVersionConflict
		}
		row, e := q.CurationGetSource(ctx, sqlc.CurationGetSourceParams{ID: id(source), ResourceID: parent.ID})
		if e != nil {
			return e
		}
		if in.Availability != "" {
			if _, e = curation.ApplyGovernanceTx(ctx, tx, actor, key, in.ExpectedVersion, curation.GovernanceInput{Kind: "source_availability", State: in.Availability, SourceID: source, Reason: "Manual source availability update", RequestID: in.RequestID}); e != nil {
				return e
			}
		}
		urlHash := sha256.Sum256([]byte(row.Url))
		out = uuid.NewV7()
		return q.SourceCheckInsert(ctx, sqlc.SourceCheckInsertParams{ID: id(out), ResourceID: parent.ID, SourceID: row.ID, UrlFingerprint: urlHash[:], ResourceVersion: parent.Version, Outcome: in.Outcome, Note: in.Note, ActorID: id(actor.UserID), RequestID: id(in.RequestID), RequestFingerprint: hash, ObservedAt: stamp(in.ObservedAt)})
	})
	return out, err
}

type SourceHealth struct {
	Row     sqlc.SourceHealthListRow
	Check   *sqlc.AppSourceCheck
	Current bool
}
type SourceHealthList struct {
	Items    []SourceHealth
	HasNext  bool
	Page     int64
	PageSize int
}

func (a *App) SourceHealth(ctx context.Context, actor auth.AdminActor, page int64, size int, availability string, broken *bool, key, sourceID uuid.UUID) (SourceHealthList, error) {
	out := SourceHealthList{Items: []SourceHealth{}, Page: page, PageSize: size}
	offset, err := pagination(page, size)
	if err != nil || (availability != "" && !resource.SourceAvailabilityState(availability).Valid()) {
		return out, ErrValidation
	}
	err = a.snapshot(ctx, adminCheck(ctx, actor, auth.AdminAccess), func(q *sqlc.Queries) error {
		filter := pgtype.Bool{}
		if broken != nil {
			filter = pgtype.Bool{Bool: *broken, Valid: true}
		}
		rows, e := q.SourceHealthList(ctx, sqlc.SourceHealthListParams{SourceID: id(sourceID), ResourceID: id(key), AvailabilityState: text(availability), OpenBrokenReport: filter, FetchLimit: int32(size + 1), PageOffset: offset})
		if e != nil {
			return e
		}
		out.HasNext = len(rows) > size
		if out.HasNext {
			rows = rows[:size]
		}
		for _, r := range rows {
			item := SourceHealth{Row: r}
			c, e := q.SourceCheckLatest(ctx, r.SourceID)
			if e != nil && !errors.Is(e, pgx.ErrNoRows) {
				return e
			}
			if e == nil {
				item.Check = &c
				fingerprint := sha256.Sum256([]byte(r.Url))
				item.Current = hmac.Equal(c.UrlFingerprint, fingerprint[:])
			}
			out.Items = append(out.Items, item)
		}
		return nil
	})
	return out, err
}

type AuditList struct {
	Items    []sqlc.AppAuditEntry
	HasNext  bool
	Page     int64
	PageSize int
}

func (a *App) AuditList(ctx context.Context, actor auth.AdminActor, page int64, size int, resourceID, userID uuid.UUID, operation string) (AuditList, error) {
	out := AuditList{Page: page, PageSize: size}
	offset, err := pagination(page, size)
	if err != nil {
		return out, err
	}
	if operation != "" && !slices.Contains([]string{"create", "core", "localization", "tags", "source", "source_rights", "relation", "external_ids", "publication", "soft_delete", "taxonomy", "distribution", "trust", "restrict", "revoke_restriction"}, operation) {
		return out, ErrValidation
	}
	err = a.snapshot(ctx, adminCheck(ctx, actor, auth.Administration), func(q *sqlc.Queries) error {
		rows, e := q.GovernanceAuditList(ctx, sqlc.GovernanceAuditListParams{ResourceID: id(resourceID), UserID: id(userID), Operation: text(operation), FetchLimit: int32(size + 1), PageOffset: offset})
		if e != nil {
			return e
		}
		out.HasNext = len(rows) > size
		if out.HasNext {
			rows = rows[:size]
		}
		out.Items = rows
		return nil
	})
	return out, err
}
func (a *App) Audit(ctx context.Context, actor auth.AdminActor, key uuid.UUID) (sqlc.AppAuditEntry, error) {
	var out sqlc.AppAuditEntry
	err := a.snapshot(ctx, adminCheck(ctx, actor, auth.Administration), func(q *sqlc.Queries) error { var e error; out, e = q.GovernanceAudit(ctx, id(key)); return e })
	return out, err
}
