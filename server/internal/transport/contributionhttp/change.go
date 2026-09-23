package contributionhttp

import (
	"encoding/json"
	"github.com/deepfurry/tap4furry/server/internal/contribution"
	"github.com/deepfurry/tap4furry/server/internal/resource"
	"uuid"
)

// ParseChange preserves explicit nulls and rejects response-only fields. The
// enclosing generated request has already passed strict JSON/duplicate checks.
func ParseChange(raw json.RawMessage, review bool) (*contribution.Change, error) {
	m, err := Object(raw, nil, []string{"source", "source_id", "tag_ids", "relation", "translation"}, nil)
	if err != nil || len(m) != 1 {
		return nil, contribution.ErrValidation
	}
	c := &contribution.Change{}
	text := func(m map[string]json.RawMessage, k string) (string, error) {
		var s string
		if m[k] == nil {
			return "", nil
		}
		if json.Unmarshal(m[k], &s) != nil {
			return "", contribution.ErrValidation
		}
		return s, nil
	}
	optional := func(m map[string]json.RawMessage, k string) (*string, error) {
		var s *string
		if m[k] != nil && json.Unmarshal(m[k], &s) != nil {
			return nil, contribution.ErrValidation
		}
		return s, nil
	}
	if raw := m["source"]; raw != nil {
		required, allowed := []string{"url"}, []string{"url", "label", "source_type"}
		if review {
			required = append(required, "availability_state")
			allowed = append(allowed, "availability_state")
		}
		s, e := Object(raw, required, allowed, []string{"label"})
		if e != nil {
			return nil, e
		}
		v := &contribution.Source{Type: "unknown"}
		v.URL, e = text(s, "url")
		if e != nil {
			return nil, e
		}
		v.Label, e = optional(s, "label")
		if e != nil {
			return nil, e
		}
		if s["source_type"] != nil {
			kind, e := text(s, "source_type")
			if e != nil {
				return nil, e
			}
			v.Type = resource.SourceType(kind)
		}
		available, e := text(s, "availability_state")
		if e != nil {
			return nil, e
		}
		v.Availability = resource.SourceAvailabilityState(available)
		c.Source = v
	}
	if raw := m["source_id"]; raw != nil {
		s, e := text(m, "source_id")
		if e != nil {
			return nil, e
		}
		c.SourceID, e = ID(s)
		if e != nil {
			return nil, e
		}
	}
	if raw := m["tag_ids"]; raw != nil {
		var ids []string
		if json.Unmarshal(raw, &ids) != nil || len(ids) < 1 || len(ids) > 10 {
			return nil, contribution.ErrValidation
		}
		c.Tags = []uuid.UUID{}
		for _, s := range ids {
			key, e := ID(s)
			if e != nil {
				return nil, e
			}
			c.Tags = append(c.Tags, key)
		}
	}
	if raw := m["relation"]; raw != nil {
		fields := []string{"other_resource_id", "relation_type", "direction"}
		r, e := Object(raw, fields, fields, nil)
		if e != nil {
			return nil, e
		}
		v := &contribution.RelationChange{}
		key, e := text(r, "other_resource_id")
		if e != nil {
			return nil, e
		}
		v.OtherID, e = ID(key)
		if e != nil {
			return nil, e
		}
		kind, e := text(r, "relation_type")
		if e != nil {
			return nil, e
		}
		v.Type = resource.RelationType(kind)
		v.Direction, e = text(r, "direction")
		if e != nil {
			return nil, e
		}
		c.Relation = v
	}
	if raw := m["translation"]; raw != nil {
		required := []string{"locale"}
		if review {
			required = append(required, "name", "summary", "description")
		}
		t, e := Object(raw, required, []string{"locale", "name", "summary", "description"}, []string{"summary", "description"})
		if e != nil {
			return nil, e
		}
		v := &contribution.TranslationChange{}
		v.Locale, e = text(t, "locale")
		if e != nil {
			return nil, e
		}
		v.Name, e = optional(t, "name")
		if e != nil {
			return nil, e
		}
		v.Summary.Set = t["summary"] != nil
		v.Summary.Value, e = optional(t, "summary")
		if e != nil {
			return nil, e
		}
		v.Description.Set = t["description"] != nil
		v.Description.Value, e = optional(t, "description")
		if e != nil {
			return nil, e
		}
		c.Translation = v
	}
	return c, nil
}
