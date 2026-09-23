// Package moderationhttp shares strict parsing and safe errors, never application decisions.
package moderationhttp

import (
	"errors"
	"github.com/deepfurry/tap4furry/server/internal/contribution"
	"github.com/deepfurry/tap4furry/server/internal/curation"
	"github.com/deepfurry/tap4furry/server/internal/governance"
	"github.com/deepfurry/tap4furry/server/internal/moderation"
	"github.com/deepfurry/tap4furry/server/internal/resource"
	ch "github.com/deepfurry/tap4furry/server/internal/transport/contributionhttp"
	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5/pgtype"
	"time"
	"uuid"
)

func ID(raw string) (uuid.UUID, error) {
	v, e := ch.ID(raw)
	if e != nil {
		return v, moderation.ErrValidation
	}
	return v, nil
}
func OptionalID(raw *string) (uuid.UUID, error) {
	if raw == nil {
		return uuid.Nil(), nil
	}
	return ID(*raw)
}
func Value[T ~string](v *T) string {
	if v == nil {
		return ""
	}
	return string(*v)
}
func Text[T ~string](v pgtype.Text) *T {
	if !v.Valid {
		return nil
	}
	s := T(v.String)
	return &s
}
func IDPointer(v pgtype.UUID) *string {
	if !v.Valid {
		return nil
	}
	s := uuid.UUID(v.Bytes).String()
	return &s
}
func IDString(v pgtype.UUID) string { return uuid.UUID(v.Bytes).String() }
func Time(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}
func Number(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	n := v.Int64
	return &n
}
func Pagination(page *int64, size *int) (int64, int) {
	p, s := int64(1), 20
	if page != nil {
		p = *page
	}
	if size != nil {
		s = *size
	}
	return p, s
}
func Decode(c fiber.Ctx, v any, required, allowed []string) error {
	_, e := ch.Decode(c, v, required, allowed, nil)
	if e != nil {
		return moderation.ErrValidation
	}
	return nil
}
func Query(c fiber.Ctx, allowed []string) error {
	if ch.Query(c, allowed) != nil {
		return moderation.ErrValidation
	}
	return nil
}
func Error(err error) (int, string, string, int) {
	var limit *moderation.LimitError
	var restricted *governance.RestrictedError
	switch {
	case errors.As(err, &restricted):
		return 403, "BUSINESS_RESTRICTED", restricted.Message, 0
	case errors.As(err, &limit):
		return 429, "REPORT_LIMITED", "Report limit reached: " + limit.Reason + ".", limit.RetryAfter
	case errors.Is(err, moderation.ErrValidation), errors.Is(err, contribution.ErrValidation), errors.Is(err, curation.ErrValidation):
		return 400, "VALIDATION_ERROR", "Check the supplied fields.", 0
	case errors.Is(err, moderation.ErrNotFound), errors.Is(err, curation.ErrNotFound):
		return 404, "REPORT_NOT_FOUND", "The requested object is unavailable.", 0
	case errors.Is(err, moderation.ErrForbidden):
		return 403, "REPORT_FORBIDDEN", "This account cannot perform this action.", 0
	case errors.Is(err, moderation.ErrVerified):
		return 403, "REPORT_VERIFICATION_REQUIRED", "Verify your email before reporting.", 0
	case errors.Is(err, moderation.ErrConflict), errors.Is(err, curation.ErrConflict):
		return 409, "GOVERNANCE_CONFLICT", "The object changed or was processed. Reload explicitly before trying again.", 0
	case errors.Is(err, moderation.ErrRequestConflict):
		return 409, "GOVERNANCE_REQUEST_CONFLICT", "This request key belongs to different input.", 0
	case errors.Is(err, resource.ErrVersionConflict):
		return 409, "RESOURCE_VERSION_CONFLICT", "The resource changed. Reload its current version; your input has been kept.", 0
	}
	return 0, "", "", 0
}
