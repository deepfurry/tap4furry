package contribution

import (
	"context"
	"errors"
	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/jackc/pgx/v5"
	"math"
	"time"
	"uuid"
)

type Summary struct {
	ID                 uuid.UUID
	Kind, Status, Name string
	CreatedAt          time.Time
}
type Event struct {
	Type    string
	Message *string
	At      time.Time
}
type Result struct {
	ID         uuid.UUID
	Slug, Name string
}
type Detail struct {
	ID, PreviousID                 uuid.UUID
	Kind, Status, Reason           string
	CreatedAt                      time.Time
	DecidedAt                      *time.Time
	Proposed                       Content
	ProposedChange, AcceptedChange *Change
	SubmittedFields                int16
	Accepted                       *Content
	Result                         *Result
	Target                         *Result
	History                        []Event
}
type AdminEvent struct {
	Event
	ActorID      uuid.UUID
	InternalNote *string
}
type AdminDetail struct {
	Detail
	AuthorID, TargetID, ResultID uuid.UUID
	BaseVersion                  int64
	Base, Current                *Content
	Conflict, SelfReview         bool
	ReviewHistory                []AdminEvent
	BaseChange, CurrentChange    *Change
	ResourceChanges              []ResourceChange
}
type Limits struct {
	Remaining24h, Pending int64
	RetryAfter            int
	Reason                string
}
type List struct {
	Items    []Summary
	HasNext  bool
	Page     int64
	PageSize int
	Limits   Limits
}

func pagination(page int64, size int, status, kind string) (int64, error) {
	if page < 1 || size < 1 || size > 100 || page-1 > math.MaxInt64/int64(size) || !validStatus(status) || (kind != "" && !validKind(kind)) {
		return 0, ErrValidation
	}
	return (page - 1) * int64(size), nil
}
func baseDetail(row sqlc.AppContribution, contents map[string]Content) Detail {
	result := Detail{ID: uid(row.ID), PreviousID: uid(row.PreviousID), Kind: row.Kind, Status: row.Status, Reason: row.Reason, CreatedAt: row.CreatedAt.Time, Proposed: contents["proposed"], SubmittedFields: row.SubmittedFields, History: []Event{}}
	if row.DecidedAt.Valid {
		v := row.DecidedAt.Time
		result.DecidedAt = &v
	}
	return result
}
func (a *App) OwnDetail(ctx context.Context, actor auth.Actor, key uuid.UUID) (Detail, error) {
	var out Detail
	err := a.snapshot(ctx, publicCheck(ctx, actor), func(_ pgx.Tx, q *sqlc.Queries, _ time.Time) error {
		// Pin the mutable decision while reading its immutable snapshots/events.
		row, err := q.ContributionLockOwned(ctx, sqlc.ContributionLockOwnedParams{ID: id(key), AuthorID: id(actor.UserID)})
		if err != nil {
			return err
		}
		if Extended(row.Kind) {
			return ownChangeDetail(ctx, q, row, &out)
		}
		contents, err := readContents(ctx, q, key)
		if err != nil {
			return err
		}
		out = baseDetail(row, contents)
		if row.TargetResourceID.Valid {
			current, e := q.ContributionResource(ctx, sqlc.ContributionResourceParams{ID: row.TargetResourceID})
			if e != nil && !errors.Is(e, pgx.ErrNoRows) {
				return e
			}
			if e == nil {
				if !current.CanonicalOk {
					return ErrCanonical
				}
				out.Target = &Result{uid(current.ID), current.Slug, current.Name.String}
			}
		}
		history, err := q.ContributionPublicHistory(ctx, id(key))
		if err != nil {
			return err
		}
		for _, e := range history {
			out.History = append(out.History, Event{e.EventType, ptr(e.Message), e.OccurredAt.Time})
		}
		if row.ResultResourceID.Valid {
			current, err := q.ContributionResource(ctx, sqlc.ContributionResourceParams{ID: row.ResultResourceID})
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			if err != nil {
				return err
			}
			if !current.CanonicalOk {
				return ErrCanonical
			}
			out.Result = &Result{uid(current.ID), current.Slug, current.Name.String}
			// Later edits can remove sensitive text or restrict a Source while
			// retaining a public Resource. Never resurrect that historical data.
			if current.Version == row.ResultVersion.Int64 {
				v, ok := contents["accepted"]
				if !ok {
					return ErrCanonical
				}
				out.Accepted = &v
			}
		}
		return nil
	})
	return out, err
}
func (a *App) ReviewDetail(ctx context.Context, actor auth.AdminActor, key uuid.UUID) (AdminDetail, error) {
	var out AdminDetail
	err := a.snapshot(ctx, adminCheck(ctx, actor), func(_ pgx.Tx, q *sqlc.Queries, _ time.Time) error {
		row, err := q.ContributionLock(ctx, id(key))
		if err != nil {
			return err
		}
		if Extended(row.Kind) {
			return reviewChangeDetail(ctx, q, row, actor, &out)
		}
		contents, err := readContents(ctx, q, key)
		if err != nil {
			return err
		}
		out = AdminDetail{Detail: baseDetail(row, contents), AuthorID: uid(row.AuthorID), TargetID: uid(row.TargetResourceID), ResultID: uid(row.ResultResourceID), BaseVersion: row.BaseVersion.Int64, SelfReview: uid(row.AuthorID) == actor.UserID, ReviewHistory: []AdminEvent{}}
		if c, ok := contents["base"]; ok {
			out.Base = &c
		}
		if c, ok := contents["accepted"]; ok {
			out.Accepted = &c
		}
		if row.TargetResourceID.Valid {
			current, err := q.ContributionResource(ctx, sqlc.ContributionResourceParams{ID: row.TargetResourceID})
			if errors.Is(err, pgx.ErrNoRows) {
				out.Conflict = true
			} else if err != nil {
				return err
			} else {
				value, err := resourceContent(current)
				if err != nil {
					return err
				}
				out.Current = &value
				out.Conflict = current.Version != row.BaseVersion.Int64
			}
		}
		history, err := q.ContributionAdminHistory(ctx, id(key))
		if err != nil {
			return err
		}
		for _, e := range history {
			out.ReviewHistory = append(out.ReviewHistory, AdminEvent{Event{e.EventType, ptr(e.Message), e.OccurredAt.Time}, uid(e.ActorID), ptr(e.InternalNote)})
		}
		return nil
	})
	return out, err
}
func (a *App) OwnList(ctx context.Context, actor auth.Actor, page int64, size int, status string) (List, error) {
	out := List{Page: page, PageSize: size, Items: []Summary{}}
	offset, err := pagination(page, size, status, "")
	if err != nil {
		return out, err
	}
	err = a.transact(ctx, publicCheck(ctx, actor), func(_ pgx.Tx, q *sqlc.Queries, now time.Time) error {
		rows, err := q.ContributionListOwned(ctx, sqlc.ContributionListOwnedParams{AuthorID: id(actor.UserID), Status: str(status), FetchLimit: int32(size + 1), PageOffset: offset})
		if err != nil {
			return err
		}
		out.HasNext = len(rows) > size
		if out.HasNext {
			rows = rows[:size]
		}
		for _, r := range rows {
			out.Items = append(out.Items, Summary{uid(r.ID), r.Kind, r.Status, r.Name, r.CreatedAt.Time})
		}
		quota, err := q.ContributionQuota(ctx, sqlc.ContributionQuotaParams{AuthorID: id(actor.UserID), Now: stamp(now)})
		if err != nil {
			return err
		}
		out.Limits = Limits{Remaining24h: max(0, 10-quota.Recent), Pending: quota.Pending}
		var limit *LimitError
		if errors.As(checkQuota(quota, now), &limit) {
			out.Limits.RetryAfter = limit.RetryAfter
			out.Limits.Reason = limit.Reason
		}
		return nil
	})
	return out, err
}
func (a *App) ReviewList(ctx context.Context, actor auth.AdminActor, page int64, size int, status, kind string) (List, error) {
	out := List{Page: page, PageSize: size, Items: []Summary{}}
	offset, err := pagination(page, size, status, kind)
	if err != nil {
		return out, err
	}
	err = a.transact(ctx, adminCheck(ctx, actor), func(_ pgx.Tx, q *sqlc.Queries, _ time.Time) error {
		rows, err := q.ContributionListAdmin(ctx, sqlc.ContributionListAdminParams{Status: str(status), Kind: str(kind), FetchLimit: int32(size + 1), PageOffset: offset})
		if err != nil {
			return err
		}
		out.HasNext = len(rows) > size
		if out.HasNext {
			rows = rows[:size]
		}
		for _, r := range rows {
			out.Items = append(out.Items, Summary{uid(r.ID), r.Kind, r.Status, r.Name, r.CreatedAt.Time})
		}
		return nil
	})
	return out, err
}
