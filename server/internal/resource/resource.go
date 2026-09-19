// Package resource defines canonical Resource knowledge independently of storage.
package resource

import (
	"bytes"
	"errors"
	"regexp"
	"time"
	"uuid"

	"github.com/deepfurry/tap4furry/server/internal/taxonomy"
)

var (
	ErrValidation      = errors.New("invalid resource input")
	ErrPublishedSlug   = errors.New("published resource slug is immutable")
	ErrVersionConflict = errors.New("resource version conflict")
	slugPattern        = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	namespacePattern   = regexp.MustCompile(`^[a-z0-9]+([._-][a-z0-9]+)*$`)
)

func ValidateSlug(slug string) error {
	if len(slug) < 1 || len(slug) > 80 || !slugPattern.MatchString(slug) {
		return ErrValidation
	}
	return nil
}

type Core struct {
	ID, CategoryID         uuid.UUID
	Slug                   string
	DefaultLocale          taxonomy.Locale
	PublicationState       PublicationState
	Lifecycle              Lifecycle
	ContentRating          ContentRating
	Version                int64
	CreatedAt, UpdatedAt   time.Time
	PublishedAt, DeletedAt *time.Time
}

func (r Core) Validate() error {
	l, err := taxonomy.ParseLocale(string(r.DefaultLocale))
	if r.ID == (uuid.UUID{}) || r.CategoryID == (uuid.UUID{}) || ValidateSlug(r.Slug) != nil || err != nil || l != r.DefaultLocale || !r.PublicationState.Valid() || !r.Lifecycle.Valid() || !r.ContentRating.Valid() || r.Version < 1 || r.CreatedAt.IsZero() || r.UpdatedAt.Before(r.CreatedAt) {
		return ErrValidation
	}
	if (r.PublishedAt != nil && r.PublishedAt.Before(r.CreatedAt)) || (r.DeletedAt != nil && r.DeletedAt.Before(r.CreatedAt)) || (r.PublicationState == Published && r.PublishedAt == nil) {
		return ErrValidation
	}
	return nil
}

func (r Core) ValidateSlugChange(next string) error {
	if err := ValidateSlug(next); err != nil {
		return err
	}
	if r.PublishedAt != nil && next != r.Slug {
		return ErrPublishedSlug
	}
	return nil
}

type Localization struct {
	Locale               taxonomy.Locale
	Name                 string
	Summary, Description *string
}

func NormalizeLocalization(locale, name string, summary, description *string) (Localization, error) {
	l, err := taxonomy.ParseLocale(locale)
	if err != nil {
		return Localization{}, ErrValidation
	}
	n, err := taxonomy.RequiredText(name, 160)
	if err != nil {
		return Localization{}, ErrValidation
	}
	s, err := taxonomy.OptionalText(summary, 500)
	if err != nil {
		return Localization{}, ErrValidation
	}
	d, err := taxonomy.OptionalText(description, 0)
	if err != nil {
		return Localization{}, ErrValidation
	}
	return Localization{Locale: l, Name: n, Summary: s, Description: d}, nil
}

type Relation struct {
	Source, Target uuid.UUID
	Type           RelationType
}

func CanonicalRelation(source, target uuid.UUID, kind RelationType) (Relation, error) {
	if source == (uuid.UUID{}) || target == (uuid.UUID{}) || source == target || !kind.Valid() {
		return Relation{}, ErrValidation
	}
	if kind == RelatedTo && bytes.Compare(source[:], target[:]) > 0 {
		source, target = target, source
	}
	return Relation{Source: source, Target: target, Type: kind}, nil
}

type ExternalID struct{ Namespace, Value string }

func (id ExternalID) Normalize() (ExternalID, error) {
	if len(id.Namespace) < 1 || len(id.Namespace) > 64 || !namespacePattern.MatchString(id.Namespace) {
		return ExternalID{}, ErrValidation
	}
	value, err := taxonomy.RequiredText(id.Value, 512)
	if err != nil {
		return ExternalID{}, ErrValidation
	}
	return ExternalID{Namespace: id.Namespace, Value: value}, nil
}
