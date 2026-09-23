package moderation

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/deepfurry/tap4furry/server/internal/governance"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"math"
	"slices"
	"time"
	"uuid"
)

func id(v uuid.UUID) pgtype.UUID           { return governance.ID(v) }
func uid(v pgtype.UUID) uuid.UUID          { return uuid.UUID(v.Bytes) }
func text(v string) pgtype.Text            { return governance.Optional(v) }
func stamp(v time.Time) pgtype.Timestamptz { return governance.Timestamp(v) }
func digest(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic("invalid internal fingerprint type")
	}
	h := sha256.Sum256(b)
	return h[:]
}
func sensitive(reason string) bool {
	return slices.Contains([]string{"rights_concern", "privacy", "malicious_link"}, reason)
}
func validReason(reason string) bool {
	return slices.Contains([]string{"broken_link", "rights_concern", "malicious_link", "privacy", "content_rating", "spam", "other"}, reason)
}
func active(status string) bool { return status == "open" || status == "in_review" }
func validStatus(status string) bool {
	return status == "" || active(status) || slices.Contains([]string{"resolved", "dismissed", "withdrawn"}, status)
}
func pagination(page int64, size int) (int64, error) {
	if page < 1 || size < 1 || size > 100 || page-1 > math.MaxInt64/int64(size) {
		return 0, ErrValidation
	}
	return (page - 1) * int64(size), nil
}
func seconds(until, now time.Time) int { return max(1, int(math.Ceil(until.Sub(now).Seconds()))) }
func reportLimit(q sqlc.ReportQuotaRow, now time.Time) error {
	b := governance.ReportBudget()
	if q.Pending >= b.Pending {
		return &LimitError{Reason: "pending_limit"}
	}
	if q.Recent >= b.Daily {
		return &LimitError{Reason: "daily_limit", RetryAfter: seconds(q.FirstAt.Time.Add(24*time.Hour), now)}
	}
	if q.LastAt.Valid && now.Before(q.LastAt.Time.Add(b.Interval)) {
		return &LimitError{Reason: "submission_interval", RetryAfter: seconds(q.LastAt.Time.Add(b.Interval), now)}
	}
	return nil
}

type ReportInput struct {
	RequestID, ResourceID, SourceID uuid.UUID
	TargetKind, Reason, Body        string
}

func (a *App) SubmitReport(ctx context.Context, actor auth.Actor, in ReportInput) (uuid.UUID, error) {
	var out uuid.UUID
	var err error
	in.Body, err = governance.Text(in.Body, 4000)
	if err != nil || in.RequestID == uuid.Nil() || in.ResourceID == uuid.Nil() || !validReason(in.Reason) || (in.TargetKind != "resource" && in.TargetKind != "source") || ((in.TargetKind == "source") != (in.SourceID != uuid.Nil())) {
		return out, ErrValidation
	}
	hash := digest(in)
	err = a.transact(ctx, publicCheck(ctx, actor), func(_ pgx.Tx, q *sqlc.Queries, now time.Time) error {
		prior, e := q.ReportByRequest(ctx, sqlc.ReportByRequestParams{ReporterID: id(actor.UserID), RequestID: id(in.RequestID)})
		if e == nil {
			if !hmac.Equal(prior.RequestFingerprint, hash) {
				return ErrRequestConflict
			}
			out = uid(prior.ID)
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		verified, e := q.ContributionVerified(ctx, id(actor.UserID))
		if e != nil {
			return e
		}
		if !verified {
			return ErrVerified
		}
		quota, e := q.ReportQuota(ctx, sqlc.ReportQuotaParams{ReporterID: id(actor.UserID), Now: stamp(now)})
		if e != nil {
			return e
		}
		if e = reportLimit(quota, now); e != nil {
			return e
		}
		target, e := q.ReportPublicTarget(ctx, sqlc.ReportPublicTargetParams{ResourceID: id(in.ResourceID), SourceID: id(in.SourceID)})
		if e != nil {
			return e
		}
		if !target.CanonicalOk {
			return ErrCanonical
		}
		out = uuid.NewV7()
		queue, priority := "moderation", int16(0)
		if sensitive(in.Reason) {
			queue, priority = "administration", 1
		}
		if e = q.ReportInsert(ctx, sqlc.ReportInsertParams{ID: id(out), ReporterID: id(actor.UserID), ResourceID: id(in.ResourceID), SourceID: id(in.SourceID), TargetKind: in.TargetKind, Reason: in.Reason, Body: in.Body, RequestID: id(in.RequestID), RequestFingerprint: hash, Queue: queue, Priority: priority}); e != nil {
			return e
		}
		return q.ReportPublicEventInsert(ctx, sqlc.ReportPublicEventInsertParams{ID: id(uuid.NewV7()), ReportID: id(out), EventType: "submitted", ActorID: id(actor.UserID), RequestID: id(in.RequestID), RequestFingerprint: hash})
	})
	return out, err
}

type OwnReport struct {
	Row    sqlc.ReportOwnedRow
	Target *sqlc.ReportPublicTargetRow
	Events []sqlc.ReportPublicEventsRow
}
type OwnReportList struct {
	Items    []OwnReport
	HasNext  bool
	Page     int64
	PageSize int
}

func (a *App) OwnReport(ctx context.Context, actor auth.Actor, key uuid.UUID) (OwnReport, error) {
	out := OwnReport{Events: []sqlc.ReportPublicEventsRow{}}
	err := a.snapshot(ctx, publicCheck(ctx, actor), func(q *sqlc.Queries) error {
		var e error
		out.Row, e = q.ReportOwned(ctx, sqlc.ReportOwnedParams{ID: id(key), ReporterID: id(actor.UserID)})
		if e != nil {
			return e
		}
		out.Events, e = q.ReportPublicEvents(ctx, id(key))
		if e != nil {
			return e
		}
		target, e := q.ReportPublicTarget(ctx, sqlc.ReportPublicTargetParams{ResourceID: out.Row.ResourceID, SourceID: out.Row.SourceID})
		if errors.Is(e, pgx.ErrNoRows) {
			return nil
		}
		if e != nil {
			return e
		}
		if !target.CanonicalOk {
			return ErrCanonical
		}
		out.Target = &target
		return nil
	})
	return out, err
}
func (a *App) OwnReports(ctx context.Context, actor auth.Actor, page int64, size int, status string) (OwnReportList, error) {
	out := OwnReportList{Items: []OwnReport{}, Page: page, PageSize: size}
	offset, err := pagination(page, size)
	if err != nil || !validStatus(status) {
		return out, ErrValidation
	}
	err = a.snapshot(ctx, publicCheck(ctx, actor), func(q *sqlc.Queries) error {
		rows, e := q.ReportListOwned(ctx, sqlc.ReportListOwnedParams{ReporterID: id(actor.UserID), Status: text(status), FetchLimit: int32(size + 1), PageOffset: offset})
		if e != nil {
			return e
		}
		out.HasNext = len(rows) > size
		if out.HasNext {
			rows = rows[:size]
		}
		for _, r := range rows {
			out.Items = append(out.Items, OwnReport{Row: sqlc.ReportOwnedRow(r), Events: []sqlc.ReportPublicEventsRow{}})
		}
		return nil
	})
	return out, err
}
func (a *App) WithdrawReport(ctx context.Context, actor auth.Actor, key, request uuid.UUID) error {
	if key == uuid.Nil() || request == uuid.Nil() {
		return ErrValidation
	}
	hash := digest(struct {
		Kind string
		ID   uuid.UUID
	}{"withdraw", key})
	return a.transact(ctx, publicCheck(ctx, actor), func(_ pgx.Tx, q *sqlc.Queries, _ time.Time) error {
		replay, e := q.ReportEventReplay(ctx, sqlc.ReportEventReplayParams{ActorID: id(actor.UserID), RequestID: id(request)})
		if e == nil {
			if replay.ReportID != id(key) || !hmac.Equal(replay.RequestFingerprint, hash) {
				return ErrRequestConflict
			}
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		row, e := q.ReportLockOwned(ctx, sqlc.ReportLockOwnedParams{ID: id(key), ReporterID: id(actor.UserID)})
		if e != nil {
			return e
		}
		if !active(row.Status) {
			return ErrConflict
		}
		if e = q.ReportWithdraw(ctx, sqlc.ReportWithdrawParams{ID: id(key), ReporterID: id(actor.UserID)}); e != nil {
			return e
		}
		return q.ReportPublicEventInsert(ctx, sqlc.ReportPublicEventInsertParams{ID: id(uuid.NewV7()), ReportID: id(key), ActorID: id(actor.UserID), EventType: "withdrawn", RequestID: id(request), RequestFingerprint: hash})
	})
}

type AdminReport struct {
	Row    sqlc.AppReport
	Events []sqlc.AppReportEvent
}
type AdminReportList struct {
	Items    []AdminReport
	HasNext  bool
	Page     int64
	PageSize int
}

func (a *App) AdminReport(ctx context.Context, actor auth.AdminActor, key uuid.UUID) (AdminReport, error) {
	var out AdminReport
	err := a.snapshot(ctx, adminCheck(ctx, actor, auth.Moderation), func(q *sqlc.Queries) error {
		var e error
		out.Row, e = q.ReportAdmin(ctx, id(key))
		if e != nil {
			return e
		}
		out.Events, e = q.ReportStaffEvents(ctx, id(key))
		return e
	})
	return out, err
}
func (a *App) AdminReports(ctx context.Context, actor auth.AdminActor, page int64, size int, status, reason, queue string) (AdminReportList, error) {
	out := AdminReportList{Items: []AdminReport{}, Page: page, PageSize: size}
	offset, err := pagination(page, size)
	if err != nil || !validStatus(status) || (reason != "" && !validReason(reason)) || (queue != "" && queue != "moderation" && queue != "administration") {
		return out, ErrValidation
	}
	err = a.snapshot(ctx, adminCheck(ctx, actor, auth.Moderation), func(q *sqlc.Queries) error {
		rows, e := q.ReportListAdmin(ctx, sqlc.ReportListAdminParams{Status: text(status), Reason: text(reason), Queue: text(queue), FetchLimit: int32(size + 1), PageOffset: offset})
		if e != nil {
			return e
		}
		out.HasNext = len(rows) > size
		if out.HasNext {
			rows = rows[:size]
		}
		for _, r := range rows {
			out.Items = append(out.Items, AdminReport{r, []sqlc.AppReportEvent{}})
		}
		return nil
	})
	return out, err
}
