package admin

import (
	"github.com/deepfurry/tap4furry/server/internal/contribution"
	"github.com/deepfurry/tap4furry/server/internal/transport/admin/generated"
	ch "github.com/deepfurry/tap4furry/server/internal/transport/contributionhttp"
)

func adminChange(c *contribution.Change) *generated.ContributionChange {
	if c == nil {
		return nil
	}
	out := &generated.ContributionChange{SourceId: ch.IDPointer(c.SourceID)}
	if s := c.Source; s != nil {
		kind := generated.ContributionChangeSourceSourceType(s.Type)
		out.Source = &generated.ContributionChangeSource{Url: s.URL, Label: s.Label, SourceType: &kind}
		if s.Availability != "" {
			available := generated.ContributionChangeSourceAvailabilityState(s.Availability)
			out.Source.AvailabilityState = &available
		}
	}
	if c.Tags != nil {
		ids := []string{}
		for _, key := range c.Tags {
			ids = append(ids, key.String())
		}
		out.TagIds = &ids
	}
	if r := c.Relation; r != nil {
		out.Relation = &generated.ContributionChangeRelation{OtherResourceId: r.OtherID.String(), RelationType: generated.ContributionChangeRelationRelationType(r.Type), Direction: generated.ContributionChangeRelationDirection(r.Direction)}
	}
	if t := c.Translation; t != nil {
		v := &generated.ContributionChangeTranslation{Locale: t.Locale, Name: t.Name}
		// Editorial snapshots include the merged canonical values; only author
		// originals use supplied-field projection.
		v.Summary = &t.Summary.Value
		v.Description = &t.Description.Value
		v.Exists = &t.Exists
		out.Translation = v
	}
	return out
}
