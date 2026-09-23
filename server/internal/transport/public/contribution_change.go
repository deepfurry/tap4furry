package public

import (
	"github.com/deepfurry/tap4furry/server/internal/contribution"
	ch "github.com/deepfurry/tap4furry/server/internal/transport/contributionhttp"
	"github.com/deepfurry/tap4furry/server/internal/transport/public/generated"
)

func publicChange(c *contribution.Change, context bool) *generated.ContributionChange {
	if c == nil {
		return nil
	}
	out := &generated.ContributionChange{SourceId: ch.IDPointer(c.SourceID)}
	if s := c.Source; s != nil {
		kind := generated.ContributionChangeSourceSourceType(s.Type)
		out.Source = &generated.ContributionChangeSource{Url: s.URL, Label: s.Label, SourceType: &kind}
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
		if t.Summary.Set {
			v.Summary = &t.Summary.Value
		}
		if t.Description.Set {
			v.Description = &t.Description.Value
		}
		if context {
			v.Exists = &t.Exists
		}
		out.Translation = v
	}
	return out
}
