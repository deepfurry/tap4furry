package curation

import (
	"context"
	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/deepfurry/tap4furry/server/internal/governance"
	"github.com/deepfurry/tap4furry/server/internal/resource"
	"slices"
	"uuid"
)

func (a *App) AddRelation(ctx context.Context, actor auth.AdminActor, anchor uuid.UUID, expected int64, target uuid.UUID, kind resource.RelationType) (Revision, error) {
	relation, err := resource.CanonicalRelation(anchor, target, kind)
	if err != nil {
		return Revision{}, ErrValidation
	}
	return a.relationMutation(ctx, actor, anchor, expected, uuid.Nil(), relation, false)
}
func (a *App) DeleteRelation(ctx context.Context, actor auth.AdminActor, anchor uuid.UUID, expected int64, relationID uuid.UUID) (Revision, error) {
	return a.relationMutation(ctx, actor, anchor, expected, relationID, resource.Relation{}, true)
}
func (a *App) relationMutation(ctx context.Context, actor auth.AdminActor, anchor uuid.UUID, expected int64, relationID uuid.UUID, relation resource.Relation, remove bool) (Revision, error) {
	result := Revision{anchor, expected}
	err := a.transact(ctx, actor, auth.Editorial, func(q *sqlc.Queries, _ []auth.Role) error {
		if expected < 1 || anchor == uuid.Nil() {
			return ErrValidation
		}
		// Every graph writer takes the same xact lock before any Resource row lock.
		if err := q.CurationLockRelationGraph(ctx); err != nil {
			return err
		}
		if remove {
			row, err := q.CurationGetRelation(ctx, dbID(relationID))
			if err != nil {
				return err
			}
			relation = resource.Relation{Source: uuid.UUID(row.SourceResourceID.Bytes), Target: uuid.UUID(row.TargetResourceID.Bytes), Type: resource.RelationType(row.RelationType)}
			if anchor != relation.Source && anchor != relation.Target {
				return ErrNotFound
			}
		}
		ids := []uuid.UUID{relation.Source, relation.Target}
		slices.SortFunc(ids, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
		rows := make([]sqlc.LockResourceCoreRow, 0, 2)
		for _, id := range ids {
			row, err := q.LockResourceCore(ctx, dbID(id))
			if err != nil {
				return err
			}
			rows = append(rows, row)
			if id == anchor && row.Version != expected {
				return resource.ErrVersionConflict
			}
		}
		var changed int64
		var err error
		if remove {
			changed, err = q.CurationDeleteRelation(ctx, dbID(relationID))
		} else {
			if relation.Type != resource.RelatedTo {
				cycle, e := q.CurationRelationCycle(ctx, sqlc.CurationRelationCycleParams{SourceID: dbID(relation.Source), TargetID: dbID(relation.Target), RelationType: string(relation.Type)})
				if e != nil {
					return e
				}
				if cycle {
					return ErrRelationCycle
				}
			}
			changed, err = q.CurationAddRelation(ctx, sqlc.CurationAddRelationParams{ID: dbID(uuid.NewV7()), SourceResourceID: dbID(relation.Source), TargetResourceID: dbID(relation.Target), RelationType: string(relation.Type)})
		}
		if err != nil {
			return err
		}
		if changed == 0 {
			return nil
		}
		for _, row := range rows {
			revision, err := bump(ctx, q, uuid.UUID(row.ID.Bytes), row.Version)
			if err != nil {
				return err
			}
			if _, err = governance.Record(ctx, q, actor.UserID, governance.Change{Operation: "relation", ResourceID: revision.ID, BeforeVersion: row.Version, AfterVersion: revision.Version, Fields: []string{"relations"}}); err != nil {
				return err
			}
			if revision.ID == anchor {
				result = revision
			}
		}
		return nil
	})
	return result, err
}
