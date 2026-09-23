package contribution

import (
	"github.com/deepfurry/tap4furry/server/internal/resource"
	"github.com/deepfurry/tap4furry/server/internal/taxonomy"
	"reflect"
	"slices"
	"uuid"
)

const (
	AddSource      = "add_source"
	RemoveSource   = "remove_broken_source"
	AddTag         = "add_tag"
	AddRelation    = "add_relation"
	AddTranslation = "add_translation"
)

func Extended(kind string) bool {
	return slices.Contains([]string{AddSource, RemoveSource, AddTag, AddRelation, AddTranslation}, kind)
}
func validKind(kind string) bool { return kind == Create || kind == Update || Extended(kind) }

// Change is a closed application union, never arbitrary properties or transport DTOs.
type Change struct {
	SourceID    uuid.UUID
	Source      *Source
	Tags        []uuid.UUID
	Relation    *RelationChange
	Translation *TranslationChange
}
type RelationChange struct {
	OtherID                                               uuid.UUID
	Type                                                  resource.RelationType
	Direction                                             string
	OtherVersion, AnchorResultVersion, OtherResultVersion int64
}
type TranslationChange struct {
	Locale               string
	Exists               bool
	Fields               int16
	Name                 *string
	Summary, Description NullableText
}
type ResourceChange struct {
	ID            uuid.UUID
	Before, After int64
}
type ContextInput struct {
	Kind, Locale, OtherSlug, Direction string
	SourceID                           uuid.UUID
	RelationType                       resource.RelationType
}

func normalizeChange(kind string, input *Change, review bool) (*Change, error) {
	if input == nil || !Extended(kind) {
		return nil, ErrValidation
	}
	c := *input
	fields := 0
	if c.Source != nil {
		fields++
	}
	if c.SourceID != uuid.Nil() {
		fields++
	}
	if c.Tags != nil {
		fields++
	}
	if c.Relation != nil {
		fields++
	}
	if c.Translation != nil {
		fields++
	}
	if fields != 1 {
		return nil, ErrValidation
	}
	var err error
	switch kind {
	case AddSource:
		if c.Source == nil {
			return nil, ErrValidation
		}
		s := *c.Source
		if s.Type == "" {
			s.Type = "unknown"
		}
		if (!review && s.Availability != "") || (review && !slices.Contains([]resource.SourceAvailabilityState{"active", "unavailable", "broken"}, s.Availability)) {
			return nil, ErrValidation
		}
		saved := s.Availability
		s.Availability = "active"
		c.Source, err = normalizeSource(&s)
		if err != nil {
			return nil, err
		}
		c.Source.Availability = saved
	case RemoveSource:
		if c.SourceID == uuid.Nil() {
			return nil, ErrValidation
		}
	case AddTag:
		if len(c.Tags) < 1 || len(c.Tags) > 10 {
			return nil, ErrValidation
		}
		c.Tags = slices.Clone(c.Tags)
		slices.SortFunc(c.Tags, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
		c.Tags = slices.Compact(c.Tags)
		if slices.Contains(c.Tags, uuid.Nil()) {
			return nil, ErrValidation
		}
	case AddRelation:
		if c.Relation == nil {
			return nil, ErrValidation
		}
		r := *c.Relation
		c.Relation = &r
		if r.OtherID == uuid.Nil() || !r.Type.Valid() || !slices.Contains([]string{"outgoing", "incoming", "symmetric"}, r.Direction) || (r.Type == resource.RelatedTo) != (r.Direction == "symmetric") {
			return nil, ErrValidation
		}
		// Versions are server-owned and must not participate in client request identity.
		r.OtherVersion = 0
		r.AnchorResultVersion = 0
		r.OtherResultVersion = 0
		c.Relation = &r
	case AddTranslation:
		if c.Translation == nil {
			return nil, ErrValidation
		}
		t := *c.Translation
		c.Translation = &t
		l, e := taxonomy.ParseLocale(t.Locale)
		if e != nil {
			return nil, ErrValidation
		}
		t.Locale = string(l)
		if t.Name != nil {
			n, e := taxonomy.RequiredText(*t.Name, 160)
			if e != nil {
				return nil, ErrValidation
			}
			t.Name = &n
		}
		t.Summary.Value, err = optional(t.Summary.Value, 500)
		if err != nil {
			return nil, err
		}
		t.Description.Value, err = optional(t.Description.Value, 50000)
		if err != nil {
			return nil, err
		}
		if t.Name == nil && !t.Summary.Set && !t.Description.Set {
			return nil, ErrValidation
		}
		if review && (t.Name == nil || !t.Summary.Set || !t.Description.Set) {
			return nil, ErrValidation
		}
		t.Fields = 0
		if t.Name != nil {
			t.Fields |= 1
		}
		if t.Summary.Set {
			t.Fields |= 2
		}
		if t.Description.Set {
			t.Fields |= 4
		}
		t.Exists = false
		c.Translation = &t
	}
	return &c, nil
}
func originalChange(kind string, c *Change) *Change {
	if c == nil {
		return nil
	}
	v := *c
	if kind == AddSource && v.Source != nil {
		s := *v.Source
		s.Availability = ""
		v.Source = &s
	}
	if kind == RemoveSource {
		v.Source = nil
	}
	if v.Relation != nil {
		r := *v.Relation
		r.OtherVersion = 0
		r.AnchorResultVersion = 0
		r.OtherResultVersion = 0
		v.Relation = &r
	}
	if v.Translation != nil {
		t := *v.Translation
		t.Exists = false
		if t.Fields&1 == 0 {
			t.Name = nil
		}
		if !t.Summary.Set {
			t.Summary.Value = nil
		}
		if !t.Description.Set {
			t.Description.Value = nil
		}
		v.Translation = &t
	}
	return &v
}
func equalChange(kind string, a, b *Change) bool {
	if a == nil || b == nil {
		return a == b
	}
	x, y := *a, *b
	if kind == AddSource {
		x = *originalChange(kind, a)
		y = *originalChange(kind, b)
	}
	if x.Relation != nil {
		r := *x.Relation
		r.OtherVersion = 0
		r.AnchorResultVersion = 0
		r.OtherResultVersion = 0
		x.Relation = &r
	}
	if y.Relation != nil {
		r := *y.Relation
		r.OtherVersion = 0
		r.AnchorResultVersion = 0
		r.OtherResultVersion = 0
		y.Relation = &r
	}
	if x.Translation != nil {
		t := *x.Translation
		t.Exists = false
		t.Fields = 0
		t.Summary.Set = true
		t.Description.Set = true
		x.Translation = &t
	}
	if y.Translation != nil {
		t := *y.Translation
		t.Exists = false
		t.Fields = 0
		t.Summary.Set = true
		t.Description.Set = true
		y.Translation = &t
	}
	return reflect.DeepEqual(x, y)
}
