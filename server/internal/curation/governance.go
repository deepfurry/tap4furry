package curation

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/deepfurry/tap4furry/server/internal/governance"
	"github.com/deepfurry/tap4furry/server/internal/resource"
	"github.com/jackc/pgx/v5"
	"uuid"
)

// GovernanceInput permits only the four reviewed Resource governance operations.
// ReportID may be supplied only after the caller locks that Report before Resource.
type GovernanceInput struct {
	Kind, State, Reason           string
	SourceID, RequestID, ReportID uuid.UUID
}
type GovernanceResult struct {
	Revision
	AuditID uuid.UUID
}

func GovernanceFingerprint(key uuid.UUID, expected int64, in GovernanceInput) []byte {
	b, _ := json.Marshal(struct {
		Resource uuid.UUID
		Version  int64
		Input    GovernanceInput
	}{key, expected, in})
	hash := sha256.Sum256(b)
	return hash[:]
}

func ApplyGovernanceTx(ctx context.Context, tx pgx.Tx, actor auth.AdminActor, key uuid.UUID, expected int64, in GovernanceInput) (GovernanceResult, error) {
	out := GovernanceResult{Revision: Revision{key, expected}}
	cap := auth.Editorial
	if in.Kind == "source_rights" || in.Kind == "distribution" {
		cap = auth.Administration
	}
	roles, err := auth.RequireAdminCapabilityTx(ctx, tx, actor, cap)
	if err != nil {
		return out, err
	}
	q := sqlc.New(tx)
	row, err := lockedResource(ctx, q, key, expected)
	if err != nil {
		return out, err
	}
	audit := governance.Change{Operation: in.Kind, ResourceID: key, SourceID: in.SourceID, BeforeVersion: expected, AfterVersion: expected + 1}
	changed := false
	requiresReason := in.Kind == "source_rights" || in.Kind == "distribution"
	if in.Kind == "publication" && (row.PublicationState == "restricted" || row.PublicationState == "removed" || in.State == "restricted" || in.State == "removed") {
		if !auth.HasCapability(roles, auth.Administration) {
			return out, auth.ErrAdminForbidden
		}
		requiresReason = true
	}
	if requiresReason || in.Reason != "" {
		in.Reason, err = governance.Text(in.Reason, 1000)
		if err != nil {
			return out, ErrValidation
		}
	}
	audit.Reason = in.Reason
	switch in.Kind {
	case "publication":
		if !resource.PublicationState(in.State).Valid() || in.SourceID != uuid.Nil() {
			return out, ErrValidation
		}
		audit.Fields = []string{"publication_state"}
		audit.BeforePublication = row.PublicationState
		audit.AfterPublication = in.State
		changed = row.PublicationState != in.State
		if changed {
			err = q.CurationPublish(ctx, sqlc.CurationPublishParams{ID: row.ID, State: in.State})
		}
	case "source_rights", "source_availability":
		old, e := q.CurationGetSource(ctx, sqlc.CurationGetSourceParams{ID: dbID(in.SourceID), ResourceID: row.ID})
		if e != nil {
			return out, safe(e)
		}
		audit.Fields = []string{"sources"}
		if in.Kind == "source_rights" {
			if !resource.SourceRightsStatus(in.State).Valid() {
				return out, ErrValidation
			}
			audit.BeforeRights = old.RightsStatus
			audit.AfterRights = in.State
			var n int64
			n, err = q.CurationSourceRights(ctx, sqlc.CurationSourceRightsParams{ID: old.ID, RightsStatus: in.State})
			changed = n > 0
		} else {
			if !resource.SourceAvailabilityState(in.State).Valid() {
				return out, ErrValidation
			}
			audit.Operation = "source"
			audit.BeforeAvailability = old.AvailabilityState
			audit.AfterAvailability = in.State
			var n int64
			n, err = q.CurationUpdateSource(ctx, sqlc.CurationUpdateSourceParams{ID: old.ID, Url: old.Url, Label: old.Label, SourceType: old.SourceType, AvailabilityState: in.State, IsPrimary: old.IsPrimary})
			changed = n > 0
		}
	case "distribution":
		if (in.State != "normal" && in.State != "excluded") || in.SourceID != uuid.Nil() {
			return out, ErrValidation
		}
		old, e := q.GovernanceDistribution(ctx, row.ID)
		if e != nil {
			return out, safe(e)
		}
		audit.Fields = []string{"distribution"}
		audit.BeforeDistribution = old
		audit.AfterDistribution = in.State
		changed = old != in.State
		if changed {
			err = q.GovernanceSetDistribution(ctx, sqlc.GovernanceSetDistributionParams{ResourceID: row.ID, Policy: in.State})
		}
	default:
		return out, ErrValidation
	}
	if err != nil {
		return out, safe(err)
	}
	if !changed {
		return out, nil
	}
	out.Revision, err = bump(ctx, q, key, expected)
	if err != nil {
		return out, safe(err)
	}
	// A standalone ordinary publication/availability edit needs business audit but
	// no separate moderation action. Explicit governance carries a reason and key.
	if in.Reason != "" {
		audit.ActionID = uuid.NewV7()
		if in.RequestID == uuid.Nil() {
			in.RequestID = uuid.NewV7()
		}
		b, _ := json.Marshal(struct {
			Resource uuid.UUID
			Version  int64
			Input    GovernanceInput
		}{key, expected, in})
		hash := sha256.Sum256(b)
		err = q.GovernanceInsertAction(ctx, sqlc.GovernanceInsertActionParams{ID: dbID(audit.ActionID), ActorID: dbID(actor.UserID), Action: in.Kind, ResourceID: row.ID, SourceID: governance.ID(in.SourceID), ReportID: governance.ID(in.ReportID), Reason: in.Reason, RequestID: dbID(in.RequestID), RequestFingerprint: hash[:]})
		if err != nil {
			return out, safe(err)
		}
	}
	out.AuditID, err = governance.Record(ctx, q, actor.UserID, audit)
	return out, safe(err)
}
func (a *App) governed(ctx context.Context, actor auth.AdminActor, key uuid.UUID, expected int64, in GovernanceInput) (Revision, error) {
	tx, err := a.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return Revision{}, safe(err)
	}
	defer tx.Rollback(ctx)
	out, err := ApplyGovernanceTx(ctx, tx, actor, key, expected, in)
	if err != nil {
		return Revision{}, err
	}
	return out.Revision, safe(tx.Commit(ctx))
}
func (a *App) SetPublication(ctx context.Context, actor auth.AdminActor, key uuid.UUID, expected int64, state resource.PublicationState, reason string) (Revision, error) {
	return a.governed(ctx, actor, key, expected, GovernanceInput{Kind: "publication", State: string(state), Reason: reason})
}
func (a *App) SetSourceRights(ctx context.Context, actor auth.AdminActor, key uuid.UUID, expected int64, source uuid.UUID, state resource.SourceRightsStatus, reason string) (Revision, error) {
	return a.governed(ctx, actor, key, expected, GovernanceInput{Kind: "source_rights", SourceID: source, State: string(state), Reason: reason})
}
