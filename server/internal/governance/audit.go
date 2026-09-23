package governance

import (
	"context"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/jackc/pgx/v5/pgtype"
	"uuid"
)

// Change is a closed, typed audit record. No caller-provided SQL, payload map,
// secret, URL or body is accepted. A zero version pair means an unversioned object.
type Change struct {
	BeforeAvailability, AfterAvailability, BeforeTaxonomyState, AfterTaxonomyState string
	Operation                                                                      string
	ResourceID, CategoryID, TagID, UserID, SourceID, ContributionID, ActionID      uuid.UUID
	Fields                                                                         []string
	BeforeVersion, AfterVersion                                                    int64
	BeforePublication, AfterPublication, BeforeRights, AfterRights                 string
	BeforeDistribution, AfterDistribution, BeforeTrust, AfterTrust, Reason         string
}

func Record(ctx context.Context, q *sqlc.Queries, actor uuid.UUID, c Change) (uuid.UUID, error) {
	key := uuid.NewV7()
	p := sqlc.GovernanceInsertAuditParams{BeforeAvailability: Optional(c.BeforeAvailability), AfterAvailability: Optional(c.AfterAvailability), BeforeTaxonomyState: Optional(c.BeforeTaxonomyState), AfterTaxonomyState: Optional(c.AfterTaxonomyState), ID: ID(key), ActorID: ID(actor), Operation: c.Operation, ResourceID: ID(c.ResourceID), CategoryID: ID(c.CategoryID), TagID: ID(c.TagID), UserID: ID(c.UserID), SourceID: ID(c.SourceID), ContributionID: ID(c.ContributionID), ModerationActionID: ID(c.ActionID), Fields: c.Fields,
		BeforePublication: Optional(c.BeforePublication), AfterPublication: Optional(c.AfterPublication), BeforeRights: Optional(c.BeforeRights), AfterRights: Optional(c.AfterRights), BeforeDistribution: Optional(c.BeforeDistribution), AfterDistribution: Optional(c.AfterDistribution), BeforeTrust: Optional(c.BeforeTrust), AfterTrust: Optional(c.AfterTrust), Reason: Optional(c.Reason)}
	if c.AfterVersion > 0 {
		p.BeforeVersion = pgtype.Int8{Int64: c.BeforeVersion, Valid: true}
		p.AfterVersion = pgtype.Int8{Int64: c.AfterVersion, Valid: true}
	}
	return key, q.GovernanceInsertAudit(ctx, p)
}
