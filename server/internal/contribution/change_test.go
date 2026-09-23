package contribution

import (
	"github.com/deepfurry/tap4furry/server/internal/resource"
	"reflect"
	"strings"
	"testing"
	"uuid"
)

func TestTypedChangeNormalizationAndOriginal(t *testing.T) {
	for _, c := range []*Change{nil, {}, {Source: &Source{URL: "https://example.invalid"}, Tags: []uuid.UUID{uuid.NewV7()}}, {Source: &Source{URL: "https://example.invalid", Availability: "active"}}} {
		if _, e := normalizeChange(AddSource, c, false); e == nil {
			t.Fatal("invalid source union/availability accepted")
		}
	}
	s, e := normalizeChange(AddSource, &Change{Source: &Source{URL: "HTTPS://EXAMPLE.INVALID:443/a#fragment"}}, false)
	if e != nil || s.Source.URL != "https://example.invalid/a" || s.Source.Type != "unknown" || s.Source.Availability != "" {
		t.Fatal("source normalization incorrect")
	}
	a, b := uuid.NewV7(), uuid.NewV7()
	x, e := normalizeChange(AddTag, &Change{Tags: []uuid.UUID{b, a, b}}, false)
	if e != nil {
		t.Fatal(e)
	}
	y, e := normalizeChange(AddTag, &Change{Tags: []uuid.UUID{a, b}}, false)
	if e != nil || !reflect.DeepEqual(x, y) {
		t.Fatal("tag request identity depends on order")
	}
	for _, c := range []*Change{{Relation: &RelationChange{OtherID: a, Type: resource.PartOf, Direction: "symmetric"}}, {Translation: &TranslationChange{Locale: "ja", Description: NullableText{Set: true, Value: pointer(strings.Repeat("🦊", 50001))}}}} {
		kind := AddRelation
		if c.Translation != nil {
			kind = AddTranslation
		}
		if _, e := normalizeChange(kind, c, false); e == nil {
			t.Fatal("invalid relation or Unicode overflow accepted")
		}
	}
	base := &TranslationChange{Locale: "ja", Exists: true, Fields: 7, Name: pointer("Server name"), Summary: NullableText{Set: true, Value: pointer("Before")}, Description: NullableText{Set: true, Value: pointer("Private baseline field")}}
	patch, e := normalizeChange(AddTranslation, &Change{Translation: &TranslationChange{Locale: "ja", Summary: NullableText{Set: true}}}, false)
	if e != nil {
		t.Fatal(e)
	}
	merged, e := mergeTranslation(base, patch.Translation)
	if e != nil {
		t.Fatal(e)
	}
	original := originalChange(AddTranslation, &Change{Translation: merged}).Translation
	if original.Name != nil || original.Description.Set || original.Description.Value != nil || !original.Summary.Set || original.Summary.Value != nil {
		t.Fatal("author projection leaks omitted baseline")
	}
	final := *merged
	final.Fields = 7
	final.Summary.Set = true
	final.Description.Set = true
	if !equalChange(AddTranslation, &Change{Translation: merged}, &Change{Translation: &final}) {
		t.Fatal("field presence mistaken for an editorial revision")
	}
}
