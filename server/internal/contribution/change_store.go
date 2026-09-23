package contribution

import (
	"context"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/deepfurry/tap4furry/server/internal/resource"
	"github.com/jackc/pgx/v5/pgtype"
	"uuid"
)

func putChange(ctx context.Context, q *sqlc.Queries, key uuid.UUID, kind, snapshot string, c *Change) error {
	switch kind {
	case AddSource, RemoveSource:
		p := sqlc.ContributionPutSourceChangeParams{ContributionID: id(key), SnapshotKind: snapshot, SourceID: id(c.SourceID)}
		if s := c.Source; s != nil {
			p.Url = str(s.URL)
			p.Label = txt(s.Label)
			p.SourceType = str(string(s.Type))
			p.AvailabilityState = str(string(s.Availability))
		}
		return q.ContributionPutSourceChange(ctx, p)
	case AddTag:
		for _, tag := range c.Tags {
			if err := q.ContributionPutTagChange(ctx, sqlc.ContributionPutTagChangeParams{ContributionID: id(key), SnapshotKind: snapshot, TagID: id(tag), WasBound: snapshot != "proposed"}); err != nil {
				return err
			}
		}
		return nil
	case AddRelation:
		r := c.Relation
		if snapshot == "accepted" {
			return q.ContributionPutAcceptedRelation(ctx, sqlc.ContributionPutAcceptedRelationParams{ContributionID: id(key), OtherResourceID: id(r.OtherID), RelationType: string(r.Type), Direction: r.Direction, OtherBaseVersion: r.OtherVersion, AnchorResultVersion: pgtype.Int8{Int64: r.AnchorResultVersion, Valid: true}, OtherResultVersion: pgtype.Int8{Int64: r.OtherResultVersion, Valid: true}})
		}
		return q.ContributionPutRelationChange(ctx, sqlc.ContributionPutRelationChangeParams{ContributionID: id(key), SnapshotKind: snapshot, OtherResourceID: id(r.OtherID), RelationType: string(r.Type), Direction: r.Direction, OtherBaseVersion: r.OtherVersion})
	case AddTranslation:
		t := c.Translation
		bits := t.Fields
		return q.ContributionPutLocalizationChange(ctx, sqlc.ContributionPutLocalizationChangeParams{ContributionID: id(key), SnapshotKind: snapshot, Locale: t.Locale, RowExists: t.Exists, Name: txt(t.Name), Summary: txt(t.Summary.Value), Description: txt(t.Description.Value), SuppliedFields: bits})
	}
	return ErrValidation
}
func readChanges(ctx context.Context, q *sqlc.Queries, key uuid.UUID, kind string) (map[string]*Change, error) {
	out := map[string]*Change{}
	switch kind {
	case AddSource, RemoveSource:
		rows, err := q.ContributionSourceChanges(ctx, id(key))
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			c := &Change{SourceID: uid(r.SourceID)}
			if r.Url.Valid {
				c.Source = &Source{URL: r.Url.String, Label: ptr(r.Label), Type: resource.SourceType(r.SourceType.String), Availability: resource.SourceAvailabilityState(r.AvailabilityState.String)}
			}
			out[r.SnapshotKind] = c
		}
	case AddTag:
		// An empty baseline means none of the proposed additions was bound.
		out["base"] = &Change{Tags: []uuid.UUID{}}
		rows, err := q.ContributionTagChanges(ctx, id(key))
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			if r.SnapshotKind == "base" && !r.WasBound {
				continue
			}
			if out[r.SnapshotKind] == nil {
				out[r.SnapshotKind] = &Change{Tags: []uuid.UUID{}}
			}
			out[r.SnapshotKind].Tags = append(out[r.SnapshotKind].Tags, uid(r.TagID))
		}
	case AddRelation:
		rows, err := q.ContributionRelationChanges(ctx, id(key))
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out[r.SnapshotKind] = &Change{Relation: &RelationChange{OtherID: uid(r.OtherResourceID), Type: resource.RelationType(r.RelationType), Direction: r.Direction, OtherVersion: r.OtherBaseVersion, AnchorResultVersion: r.AnchorResultVersion.Int64, OtherResultVersion: r.OtherResultVersion.Int64}}
		}
	case AddTranslation:
		rows, err := q.ContributionLocalizationChanges(ctx, id(key))
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out[r.SnapshotKind] = &Change{Translation: &TranslationChange{Locale: r.Locale, Exists: r.RowExists, Fields: r.SuppliedFields, Name: ptr(r.Name), Summary: NullableText{Set: r.SuppliedFields&2 != 0, Value: ptr(r.Summary)}, Description: NullableText{Set: r.SuppliedFields&4 != 0, Value: ptr(r.Description)}}}
		}
	}
	if out["proposed"] == nil || out["base"] == nil {
		return nil, ErrCanonical
	}
	return out, nil
}
