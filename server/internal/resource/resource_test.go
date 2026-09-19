package resource

import (
	"strings"
	"testing"
	"time"
	"uuid"
)

func TestURLNormalization(t *testing.T) {
	for input, want := range map[string]string{
		" HTTPS://Example.COM:443/path?q=1#fragment ":            "https://example.com/path?q=1",
		"http://Example.COM:80/":                                 "http://example.com/",
		"https://example.com/path?a=1&b=2":                       "https://example.com/path?a=1&b=2",
		"https://EXAMPLE.com:8443/a%2Fb/../c//?a=%2f&a=+&&z=1#x": "https://example.com:8443/a%2Fb/../c//?a=%2f&a=+&&z=1",
		"https://example.com?":                                   "https://example.com?",
		"http://[2001:DB8::1]:80/path":                           "http://[2001:db8::1]/path",
		"https://[2001:DB8::1]:8443/":                            "https://[2001:db8::1]:8443/",
	} {
		got, err := NormalizeURL(input)
		if err != nil || got != want {
			t.Fatalf("URL normalization differs for synthetic input %q", input)
		}
		if again, err := NormalizeURL(got); err != nil || again != got {
			t.Fatal("URL normalization is not idempotent")
		}
	}
	for _, input := range []string{"https://user:password@example.invalid", "ftp://example.invalid", "https:///missing", "https://:443", "relative/path", "https://example.invalid:99999/", "https://example.invalid:abc/", "https://example.invalid/" + strings.Repeat("x", 2048)} {
		if _, err := NormalizeURL(input); err != ErrValidation {
			t.Fatal("invalid URL accepted or raw error exposed")
		}
	}
}

func TestIndependentResourceDimensionsAndSlug(t *testing.T) {
	now := time.Now().UTC()
	r := Core{ID: uuid.NewV7(), CategoryID: uuid.NewV7(), Slug: "resource", DefaultLocale: "en", Version: 1, CreatedAt: now, UpdatedAt: now, PublishedAt: &now}
	for _, publication := range []PublicationState{Draft, Pending, Published, Restricted, Removed} {
		for _, lifecycle := range []Lifecycle{Active, Inactive, Discontinued, Delisted, Archived, Unknown} {
			for _, rating := range []ContentRating{General, Mature, Explicit} {
				r.PublicationState, r.Lifecycle, r.ContentRating = publication, lifecycle, rating
				if r.Validate() != nil {
					t.Fatal("independent valid dimensions rejected")
				}
			}
		}
	}
	if r.ValidateSlugChange("another") != ErrPublishedSlug || r.ValidateSlugChange(r.Slug) != nil {
		t.Fatal("first publication did not freeze slug")
	}
	r.PublishedAt = nil
	if r.ValidateSlugChange("another") != nil {
		t.Fatal("unpublished slug cannot change")
	}
	r.PublicationState = Published
	if r.Validate() == nil {
		t.Fatal("published without timestamp accepted")
	}
	r.PublicationState = Draft
	for _, mutate := range []func(*Core){func(r *Core) { r.Version = 0 }, func(r *Core) { r.ContentRating = "" }, func(r *Core) { r.Lifecycle = "invalid" }, func(r *Core) { r.PublicationState = "invalid" }, func(r *Core) { r.CategoryID = uuid.UUID{} }, func(r *Core) { r.DefaultLocale = "EN" }, func(r *Core) { r.UpdatedAt = now.Add(-time.Second) }, func(r *Core) { past := now.Add(-time.Second); r.DeletedAt = &past }} {
		bad := r
		mutate(&bad)
		if bad.Validate() == nil {
			t.Fatal("invalid core invariant accepted")
		}
	}
	for _, s := range []string{"", "UPPER", "under_score", "a--b", "兽", strings.Repeat("a", 81)} {
		if ValidateSlug(s) == nil {
			t.Fatal("invalid resource slug accepted")
		}
	}
	if ValidateSlug(strings.Repeat("a", 80)) != nil {
		t.Fatal("80-character resource slug rejected")
	}
}

func TestRelationsSourcesAndExternalIDs(t *testing.T) {
	a, b := uuid.NewV7(), uuid.NewV7()
	for _, kind := range []RelationType{PartOf, SuccessorOf, DerivedFrom, RelatedTo} {
		forward, err := CanonicalRelation(a, b, kind)
		if err != nil {
			t.Fatal("valid relation rejected")
		}
		reverse, err := CanonicalRelation(b, a, kind)
		if err != nil {
			t.Fatal("valid inverse input rejected")
		}
		if kind == RelatedTo && forward != reverse {
			t.Fatal("symmetric edge has duplicate storage form")
		}
		if kind != RelatedTo && (reverse.Source != b || reverse.Target != a) {
			t.Fatal("directed relation lost direction")
		}
		if _, err := CanonicalRelation(a, a, kind); err != ErrValidation {
			t.Fatal("self-edge accepted")
		}
	}
	if _, err := CanonicalRelation(a, b, "contains"); err == nil {
		t.Fatal("future relation type accepted")
	}
	for _, kind := range []SourceType{SourceOfficial, SourceStore, SourceArchive, SourceMirror, SourceCommunity, SourceExternal, SourceUnknown} {
		for _, availability := range []SourceAvailabilityState{SourceActive, SourceUnavailable, SourceBroken, SourceRemoved, SourceRestricted} {
			for _, rights := range []SourceRightsStatus{RightsUnknown, CreatorProvided, RightsConfirmed, RightsDisputed, RightsReview, RemovedByRequest} {
				if _, err := (Source{URL: "https://example.invalid", Type: kind, Availability: availability, Rights: rights, Primary: true}).Normalize(); err != nil {
					t.Fatal("source axes incorrectly coupled")
				}
			}
		}
	}
	if SourceType("creator_provided").Valid() || SourceAvailabilityState("disputed").Valid() || SourceRightsStatus("legal").Valid() {
		t.Fatal("source dimensions mixed")
	}
	id, err := (ExternalID{Namespace: "github_repo", Value: " Owner/CaseSensitive "}).Normalize()
	if err != nil || id.Value != "Owner/CaseSensitive" {
		t.Fatal("opaque external identity changed")
	}
	for _, id := range []ExternalID{{"Steam", "123"}, {"bad..namespace", "123"}, {"steam_app", " "}, {"steam_app", strings.Repeat("x", 513)}} {
		if _, err := id.Normalize(); err == nil {
			t.Fatal("invalid external ID accepted")
		}
	}
}

func TestResourceLocalization(t *testing.T) {
	blank := " \t\n"
	description := strings.Repeat("长", 10000)
	l, err := NormalizeLocalization("zh-hans", "  资源  ", &blank, &description)
	if err != nil || l.Locale != "zh-Hans" || l.Name != "资源" || l.Summary != nil || l.Description == nil || *l.Description != description {
		t.Fatal("resource localization lost text or imposed small description limit")
	}
	long := strings.Repeat("s", 501)
	if _, err := NormalizeLocalization("en", "name", &long, nil); err == nil {
		t.Fatal("summary limit absent")
	}
	if _, err := NormalizeLocalization("en", strings.Repeat("名", 161), nil, nil); err == nil {
		t.Fatal("name limit absent")
	}
}
