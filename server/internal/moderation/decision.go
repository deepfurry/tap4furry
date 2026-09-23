package moderation

import (
	"context"
	"crypto/hmac"
	"errors"
	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/curation"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/deepfurry/tap4furry/server/internal/governance"
	"github.com/jackc/pgx/v5"
	"slices"
	"time"
	"uuid"
)

type ReportDecision struct {
	RequestID, DuplicateID, AuditID                      uuid.UUID
	ExpectedVersion, ResourceVersion                     int64
	Kind, Mode, SafeMessage, InternalNote, Reason, State string
}

func (a *App) DecideReport(ctx context.Context, actor auth.AdminActor, key uuid.UUID, in ReportDecision) error {
	if in.RequestID == uuid.Nil() || key == uuid.Nil() || in.ExpectedVersion < 1 || !slices.Contains([]string{"receive", "escalate", "note", "resolve", "dismiss"}, in.Kind) {
		return ErrValidation
	}
	var err error
	if in.InternalNote != "" {
		in.InternalNote, err = governance.Text(in.InternalNote, 2000)
		if err != nil {
			return err
		}
	}
	if in.Kind == "note" && in.InternalNote == "" {
		return ErrValidation
	}
	if in.Kind == "resolve" || in.Kind == "dismiss" {
		in.SafeMessage, err = governance.Text(in.SafeMessage, 1000)
		if err != nil {
			return err
		}
	}
	if in.Kind == "resolve" && !slices.Contains([]string{"no_change", "link_audit", "publication", "source_availability", "source_rights", "distribution"}, in.Mode) {
		return ErrValidation
	}
	if in.Mode == "link_audit" && in.AuditID == uuid.Nil() {
		return ErrValidation
	}
	if in.Kind == "resolve" && in.Mode != "no_change" && in.Mode != "link_audit" {
		in.Reason, err = governance.Text(in.Reason, 1000)
		if err != nil || in.ResourceVersion < 1 {
			return ErrValidation
		}
	}
	hash := digest(struct {
		ID    uuid.UUID
		Input ReportDecision
	}{key, in})
	return a.transact(ctx, adminCheck(ctx, actor, auth.Moderation), func(tx pgx.Tx, q *sqlc.Queries, _ time.Time) error {
		replay, e := q.ReportEventReplay(ctx, sqlc.ReportEventReplayParams{ActorID: id(actor.UserID), RequestID: id(in.RequestID)})
		if e == nil {
			if replay.ReportID != id(key) || !hmac.Equal(replay.RequestFingerprint, hash) {
				return ErrRequestConflict
			}
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		roles, e := auth.RequireAdminCapabilityTx(ctx, tx, actor, auth.Moderation)
		if e != nil {
			return e
		}
		// The duplicate FK can acquire a lock on the other report. Lock both in
		// the same order before either mutation so reciprocal links cannot deadlock.
		if in.DuplicateID != uuid.Nil() {
			if in.DuplicateID == key {
				return ErrValidation
			}
			keys := []uuid.UUID{key, in.DuplicateID}
			slices.SortFunc(keys, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
			for _, k := range keys {
				if _, e = q.ReportLock(ctx, id(k)); e != nil {
					return e
				}
			}
		}
		row, e := q.ReportLock(ctx, id(key))
		if e != nil {
			return e
		}
		if uid(row.ReporterID) == actor.UserID {
			return ErrForbidden
		}
		if row.Version != in.ExpectedVersion || !active(row.Status) {
			return ErrConflict
		}
		if (in.Kind == "resolve" || in.Kind == "dismiss") && (sensitive(row.Reason) || row.Queue == "administration") && !auth.HasCapability(roles, auth.Administration) {
			return auth.ErrAdminForbidden
		}
		status, queue, event := row.Status, row.Queue, ""
		audit := uuid.Nil()
		switch in.Kind {
		case "receive":
			if row.Status == "in_review" {
				return nil
			}
			status, event = "in_review", "triaged"
		case "escalate":
			if queue == "administration" && row.Status == "in_review" {
				return nil
			}
			status, queue, event = "in_review", "administration", "escalated"
		case "note":
			event = "noted"
		case "dismiss":
			status, event = "dismissed", "dismissed"
			if in.DuplicateID != uuid.Nil() {
				if in.DuplicateID == key {
					return ErrValidation
				}
				other, e := q.ReportAdmin(ctx, id(in.DuplicateID))
				if e != nil {
					return e
				}
				if other.ResourceID != row.ResourceID || other.SourceID != row.SourceID {
					return ErrValidation
				}
			}
		case "resolve":
			status, event = "resolved", "resolved"
			switch in.Mode {
			case "no_change":
			case "link_audit":
				prior, e := q.GovernanceAudit(ctx, id(in.AuditID))
				if e != nil {
					return e
				}
				if prior.ResourceID != row.ResourceID {
					return ErrValidation
				}
				if row.SourceID.Valid && prior.SourceID != row.SourceID && !slices.Contains([]string{"publication", "soft_delete", "distribution"}, prior.Operation) {
					return ErrValidation
				}
				audit = in.AuditID
			default:
				source := uuid.Nil()
				if in.Mode == "source_rights" || in.Mode == "source_availability" {
					if !row.SourceID.Valid {
						return ErrValidation
					}
					source = uid(row.SourceID)
				}
				result, e := curation.ApplyGovernanceTx(ctx, tx, actor, uid(row.ResourceID), in.ResourceVersion, curation.GovernanceInput{Kind: in.Mode, State: in.State, SourceID: source, Reason: in.Reason, RequestID: in.RequestID, ReportID: key})
				if e != nil {
					return e
				}
				if result.AuditID == uuid.Nil() {
					return ErrConflict
				}
				audit = result.AuditID
			}
		}
		n, e := q.ReportDecide(ctx, sqlc.ReportDecideParams{ID: row.ID, Status: status, Queue: queue, DuplicateOf: id(in.DuplicateID), ExpectedVersion: in.ExpectedVersion})
		if e != nil {
			return e
		}
		if n != 1 {
			return ErrConflict
		}
		return q.ReportStaffEventInsert(ctx, sqlc.ReportStaffEventInsertParams{ID: id(uuid.NewV7()), ReportID: row.ID, EventType: event, ActorID: id(actor.UserID), RequestID: id(in.RequestID), RequestFingerprint: hash, SafeMessage: text(in.SafeMessage), InternalNote: text(in.InternalNote), AuditID: id(audit), ResolutionType: text(in.Mode)})
	})
}
