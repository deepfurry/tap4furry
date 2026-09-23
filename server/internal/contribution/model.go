// Package contribution owns typed knowledge proposals and their review transactions.
package contribution

import (
	"errors"
	"reflect"
	"uuid"

	"github.com/deepfurry/tap4furry/server/internal/resource"
	"github.com/deepfurry/tap4furry/server/internal/taxonomy"
)

var (
	ErrValidation      = errors.New("invalid contribution")
	ErrNotFound        = errors.New("contribution not found")
	ErrForbidden       = errors.New("contribution operation forbidden")
	ErrVerified        = errors.New("verified email required")
	ErrConflict        = errors.New("contribution state conflict")
	ErrRequestConflict = errors.New("request ID already used for different content")
	ErrCanonical       = errors.New("canonical resource localization missing")
)

const (
	Create = "create_resource"
	Update = "update_resource"
)

type LimitError struct {
	Reason     string
	RetryAfter int
}

func (*LimitError) Error() string { return "contribution limit reached" }

type Source struct {
	URL          string
	Label        *string
	Type         resource.SourceType
	Availability resource.SourceAvailabilityState
}
type Content struct {
	DefaultLocale        string
	CategoryID           uuid.UUID
	Name                 string
	Summary, Description *string
	Lifecycle            resource.Lifecycle
	ContentRating        resource.ContentRating
	Slug                 string
	Source               *Source
}
type NullableText struct {
	Set   bool
	Value *string
}
type Patch struct {
	DefaultLocale, Name  *string
	CategoryID           *uuid.UUID
	Summary, Description NullableText
	Lifecycle            *resource.Lifecycle
	ContentRating        *resource.ContentRating
	SourceSet            bool
	Source               *Source
}
type SubmitInput struct {
	RequestID, TargetID, PreviousID uuid.UUID
	Kind, Reason, BaseRevision      string
	Content                         Patch
	Change                          *Change
}
type AcceptInput struct {
	Content               Content
	Change                *Change
	Message, InternalNote *string
}

func optional(value *string, limit int) (*string, error) {
	result, err := taxonomy.OptionalText(value, limit)
	if err != nil {
		return nil, ErrValidation
	}
	return result, nil
}
func normalizeSource(in *Source) (*Source, error) {
	if in == nil {
		return nil, nil
	}
	result := *in
	var err error
	result.URL, err = resource.NormalizeURL(in.URL)
	if err != nil || !in.Type.Valid() || !in.Availability.Valid() {
		return nil, ErrValidation
	}
	result.Label, err = optional(in.Label, 80)
	return &result, err
}
func normalizeContent(in Content) (Content, error) {
	locale, err := resource.NormalizeLocalization(in.DefaultLocale, in.Name, in.Summary, in.Description)
	if err != nil || in.CategoryID == uuid.Nil() || !in.Lifecycle.Valid() || !in.ContentRating.Valid() {
		return Content{}, ErrValidation
	}
	in.DefaultLocale = string(locale.Locale)
	in.Name = locale.Name
	in.Summary = locale.Summary
	in.Description, err = optional(in.Description, 50000)
	if err != nil {
		return Content{}, err
	}
	in.Source, err = normalizeSource(in.Source)
	return in, err
}

// Normalize the request itself before fingerprinting. BaseRevision is compared
// as an opaque MAC, never persisted except as part of this one-way digest.
func normalizeInput(in SubmitInput) (SubmitInput, error) {
	if in.RequestID == uuid.Nil() || !validKind(in.Kind) {
		return in, ErrValidation
	}
	if (in.Kind == Create && (in.TargetID != uuid.Nil() || in.BaseRevision != "")) || (in.Kind != Create && (in.TargetID == uuid.Nil() || in.BaseRevision == "")) {
		return in, ErrValidation
	}
	var err error
	in.Reason, err = taxonomy.RequiredText(in.Reason, 2000)
	if err != nil {
		return in, ErrValidation
	}
	if Extended(in.Kind) {
		if !reflect.DeepEqual(in.Content, Patch{}) {
			return in, ErrValidation
		}
		in.Change, err = normalizeChange(in.Kind, in.Change, false)
		return in, err
	}
	if in.Change != nil {
		return in, ErrValidation
	}
	p := &in.Content
	if p.Name != nil {
		s, e := taxonomy.RequiredText(*p.Name, 160)
		if e != nil {
			return in, ErrValidation
		}
		p.Name = &s
	}
	if p.DefaultLocale != nil {
		l, e := taxonomy.ParseLocale(*p.DefaultLocale)
		if e != nil {
			return in, ErrValidation
		}
		s := string(l)
		p.DefaultLocale = &s
	}
	if p.CategoryID != nil && *p.CategoryID == uuid.Nil() {
		return in, ErrValidation
	}
	if p.Lifecycle != nil && !p.Lifecycle.Valid() || p.ContentRating != nil && !p.ContentRating.Valid() {
		return in, ErrValidation
	}
	p.Summary.Value, err = optional(p.Summary.Value, 500)
	if err != nil {
		return in, err
	}
	p.Description.Value, err = optional(p.Description.Value, 50000)
	if err != nil {
		return in, err
	}
	p.Source, err = normalizeSource(p.Source)
	if err != nil {
		return in, err
	}
	if in.Kind == Update && p.SourceSet {
		return in, ErrValidation
	}
	if in.Kind == Create && (p.Name == nil || p.DefaultLocale == nil || p.CategoryID == nil || p.ContentRating == nil) {
		return in, ErrValidation
	}
	return in, nil
}
func apply(base Content, p Patch) (Content, error) {
	if p.Name != nil {
		base.Name = *p.Name
	}
	if p.DefaultLocale != nil {
		base.DefaultLocale = *p.DefaultLocale
	}
	if p.CategoryID != nil {
		base.CategoryID = *p.CategoryID
	}
	if p.Lifecycle != nil {
		base.Lifecycle = *p.Lifecycle
	}
	if p.ContentRating != nil {
		base.ContentRating = *p.ContentRating
	}
	if p.Summary.Set {
		base.Summary = p.Summary.Value
	}
	if p.Description.Set {
		base.Description = p.Description.Value
	}
	if p.SourceSet {
		base.Source = p.Source
	}
	return normalizeContent(base)
}
func equal(a, b Content) bool { a.Slug = ""; b.Slug = ""; return reflect.DeepEqual(a, b) }
func validStatus(s string) bool {
	return s == "" || s == "pending" || s == "accepted" || s == "rejected" || s == "withdrawn"
}

// Fixed field presence is stored separately from the complete proposed snapshot.
// Author DTOs must never expose canonical fields supplied only by the server.
const (
	FieldName int16 = 1 << iota
	FieldSummary
	FieldDescription
	FieldCategory
	FieldLifecycle
	FieldRating
	FieldLocale
)

func submittedFields(in SubmitInput) int16 {
	if in.Kind == Create {
		return 127
	}
	p := in.Content
	var bits int16
	if p.Name != nil {
		bits |= FieldName
	}
	if p.Summary.Set {
		bits |= FieldSummary
	}
	if p.Description.Set {
		bits |= FieldDescription
	}
	if p.CategoryID != nil {
		bits |= FieldCategory
	}
	if p.Lifecycle != nil {
		bits |= FieldLifecycle
	}
	if p.ContentRating != nil {
		bits |= FieldRating
	}
	if p.DefaultLocale != nil {
		bits |= FieldLocale
	}
	return bits
}
