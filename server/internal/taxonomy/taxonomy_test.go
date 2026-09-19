package taxonomy

import (
	"strings"
	"testing"
)

func TestSlugAndState(t *testing.T) {
	for _, s := range []string{"game", "creative-work", "0", strings.Repeat("a", 64)} {
		if ValidateSlug(s) != nil {
			t.Fatal("valid taxonomy slug rejected")
		}
	}
	for _, s := range []string{"", "Game", "a_b", "a--b", "-a", "a-", "a b", "兽", strings.Repeat("a", 65)} {
		if ValidateSlug(s) == nil {
			t.Fatal("invalid taxonomy slug accepted")
		}
	}
	if !Active.Valid() || !Retired.Valid() || CategoryState("").Valid() || CategoryState("removed").Valid() {
		t.Fatal("taxonomy states differ")
	}
}

func TestLocaleCanonicalization(t *testing.T) {
	for input, want := range map[string]Locale{"EN": "en", "zh-hans": "zh-Hans", "zh-hant": "zh-Hant", "JA": "ja", "en-us": "en-US", "iw": "he"} {
		got, err := ParseLocale(input)
		if err != nil || got != want {
			t.Fatalf("locale canonicalization failed for %s", input)
		}
		if again, err := ParseLocale(string(got)); err != nil || again != got {
			t.Fatal("locale canonicalization is not idempotent")
		}
	}
	for _, input := range []string{"", "a", "en_US", " en", "en--US", "en/US", strings.Repeat("a", 65)} {
		if _, err := ParseLocale(input); err == nil {
			t.Fatal("invalid locale accepted")
		}
	}
}

func TestLocalizedText(t *testing.T) {
	blank := " \n\t "
	l, err := NormalizeLocalization("EN", "  名称  ", &blank)
	if err != nil || l.Name != "名称" || l.Locale != "en" || l.Description != nil {
		t.Fatal("localization normalization failed")
	}
	name := strings.Repeat("名", 80)
	if _, err := NormalizeLocalization("en", name, nil); err != nil {
		t.Fatal("text length must count Unicode code points")
	}
	for _, name := range []string{"", " \t", "bad\x00text", string([]byte{0xff}), strings.Repeat("名", 81)} {
		if _, err := NormalizeLocalization("en", name, nil); err == nil {
			t.Fatal("invalid localized name accepted")
		}
	}
	long := strings.Repeat("a", 501)
	if _, err := NormalizeLocalization("en", "name", &long); err == nil {
		t.Fatal("taxonomy description limit not enforced")
	}
}
