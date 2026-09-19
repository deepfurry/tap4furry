// Package taxonomy defines flat Category/Tag policy without persistence or transport.
package taxonomy

import (
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/language"
)

var ErrValidation = errors.New("invalid taxonomy input")

type CategoryState string

const (
	Active  CategoryState = "active"
	Retired CategoryState = "retired"
)

func (s CategoryState) Valid() bool { return s == Active || s == Retired }

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
var localePattern = regexp.MustCompile(`^[A-Za-z0-9]+(-[A-Za-z0-9]+)*$`)

func ValidateSlug(slug string) error {
	if len(slug) < 1 || len(slug) > 64 || !slugPattern.MatchString(slug) {
		return ErrValidation
	}
	return nil
}

type Locale string

func ParseLocale(raw string) (Locale, error) {
	if len(raw) < 2 || len(raw) > 64 || !localePattern.MatchString(raw) {
		return "", ErrValidation
	}
	tag, err := language.Parse(raw)
	if err != nil {
		return "", ErrValidation
	}
	canonical := tag.String()
	if len(canonical) < 2 || len(canonical) > 64 || !localePattern.MatchString(canonical) {
		return "", ErrValidation
	}
	return Locale(canonical), nil
}

// RequiredText trims outer whitespace and counts Unicode code points like SQL
// char_length. A zero limit means unbounded text, not an implicit small VARCHAR.
func RequiredText(raw string, limit int) (string, error) {
	value := strings.TrimSpace(raw)
	if !utf8.ValidString(value) || strings.ContainsRune(value, 0) || value == "" || (limit > 0 && utf8.RuneCountInString(value) > limit) {
		return "", ErrValidation
	}
	return value, nil
}

func OptionalText(raw *string, limit int) (*string, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, nil
	}
	value, err := RequiredText(*raw, limit)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

type Localization struct {
	Locale      Locale
	Name        string
	Description *string
}

func NormalizeLocalization(locale, name string, description *string) (Localization, error) {
	l, err := ParseLocale(locale)
	if err != nil {
		return Localization{}, err
	}
	n, err := RequiredText(name, 80)
	if err != nil {
		return Localization{}, err
	}
	d, err := OptionalText(description, 500)
	if err != nil {
		return Localization{}, err
	}
	return Localization{Locale: l, Name: n, Description: d}, nil
}
