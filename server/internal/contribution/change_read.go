package contribution

import (
	"context"
	"errors"
	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/jackc/pgx/v5"
)

func changeDetail(row sqlc.AppContribution, snapshots map[string]*Change) Detail {
	out := baseDetail(row, nil)
	out.ProposedChange = snapshots["proposed"]
	return out
}
func currentPublic(ctx context.Context, q *sqlc.Queries, row sqlc.AppContribution) (*sqlc.ContributionResourceRow, error) {
	r, err := q.ContributionResource(ctx, sqlc.ContributionResourceParams{ID: row.TargetResourceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if _, err = resourceContent(r); err != nil {
		return nil, err
	}
	return &r, nil
}
func ownChangeDetail(ctx context.Context, q *sqlc.Queries, row sqlc.AppContribution, out *Detail) error {
	snapshots, err := readChanges(ctx, q, uid(row.ID), row.Kind)
	if err != nil {
		return err
	}
	*out = changeDetail(row, snapshots)
	out.ProposedChange = originalChange(row.Kind, out.ProposedChange)
	history, err := q.ContributionPublicHistory(ctx, row.ID)
	if err != nil {
		return err
	}
	for _, e := range history {
		out.History = append(out.History, Event{e.EventType, ptr(e.Message), e.OccurredAt.Time})
	}
	current, err := currentPublic(ctx, q, row)
	if err != nil || current == nil {
		return err
	}
	out.Target = &Result{uid(current.ID), current.Slug, current.Name.String}
	if row.Status != "accepted" {
		return nil
	}
	out.Result = out.Target
	if current.Version != row.ResultVersion.Int64 {
		return nil
	}
	accepted := snapshots["accepted"]
	if accepted == nil {
		return ErrCanonical
	}
	switch row.Kind {
	case AddSource:
		if _, err = q.ContributionPublicSource(ctx, sqlc.ContributionPublicSourceParams{ID: id(accepted.SourceID), ResourceID: row.TargetResourceID}); errors.Is(err, pgx.ErrNoRows) {
			return nil
		} else if err != nil {
			return err
		}
	case AddTag:
		for _, tag := range accepted.Tags {
			r, e := q.ContributionTagState(ctx, sqlc.ContributionTagStateParams{ID: id(tag), ResourceID: row.TargetResourceID})
			if errors.Is(e, pgx.ErrNoRows) {
				return nil
			}
			if e != nil {
				return e
			}
			if !r.CanonicalOk {
				return ErrCanonical
			}
			if !r.Bound {
				return nil
			}
		}
	case AddRelation:
		r := accepted.Relation
		other, e := q.ContributionResource(ctx, sqlc.ContributionResourceParams{ID: id(r.OtherID)})
		if errors.Is(e, pgx.ErrNoRows) {
			return nil
		}
		if e != nil {
			return e
		}
		if _, e = resourceContent(other); e != nil {
			return e
		}
		if other.Version != r.OtherResultVersion || current.Version != r.AnchorResultVersion {
			return nil
		}
	}
	out.AcceptedChange = accepted
	return nil
}
func reviewChangeDetail(ctx context.Context, q *sqlc.Queries, row sqlc.AppContribution, actor auth.AdminActor, out *AdminDetail) error {
	snapshots, err := readChanges(ctx, q, uid(row.ID), row.Kind)
	if err != nil {
		return err
	}
	*out = AdminDetail{Detail: changeDetail(row, snapshots), AuthorID: uid(row.AuthorID), TargetID: uid(row.TargetResourceID), ResultID: uid(row.ResultResourceID), BaseVersion: row.BaseVersion.Int64, SelfReview: uid(row.AuthorID) == actor.UserID, BaseChange: snapshots["base"], ResourceChanges: []ResourceChange{}}
	out.AcceptedChange = snapshots["accepted"]
	current, err := currentPublic(ctx, q, row)
	if err != nil {
		return err
	}
	out.Conflict = current == nil || (current != nil && current.Version != row.BaseVersion.Int64)
	if current != nil {
		out.CurrentChange, err = changeBase(ctx, q, *current, row.Kind, snapshots["proposed"])
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, ErrValidation) {
			out.Conflict = true
			err = nil
		}
		if err != nil {
			return err
		}
		if row.Kind == AddRelation && out.CurrentChange != nil && out.CurrentChange.Relation.OtherVersion != snapshots["proposed"].Relation.OtherVersion {
			out.Conflict = true
		}
		if e := validateChange(ctx, q, uid(row.TargetResourceID), row.Kind, snapshots["proposed"]); e != nil {
			if errors.Is(e, ErrValidation) || errors.Is(e, ErrConflict) || errors.Is(e, pgx.ErrNoRows) {
				out.Conflict = true
			} else {
				return e
			}
		}
	}
	history, err := q.ContributionAdminHistory(ctx, row.ID)
	if err != nil {
		return err
	}
	for _, e := range history {
		out.ReviewHistory = append(out.ReviewHistory, AdminEvent{Event{e.EventType, ptr(e.Message), e.OccurredAt.Time}, uid(e.ActorID), ptr(e.InternalNote)})
	}
	changes, err := q.ContributionResourceAudits(ctx, row.ID)
	if err != nil {
		return err
	}
	for _, r := range changes {
		out.ResourceChanges = append(out.ResourceChanges, ResourceChange{uid(r.ResourceID), r.BeforeVersion, r.AfterVersion})
	}
	return nil
}
