// Package contributionhttp contains only shared HTTP parsing for the two transports.
package contributionhttp

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/deepfurry/tap4furry/server/internal/contribution"
	"github.com/deepfurry/tap4furry/server/internal/curation"
	"github.com/deepfurry/tap4furry/server/internal/resource"
	"github.com/gofiber/fiber/v3"
	"io"
	"mime"
	"slices"
	"unicode/utf8"
	"uuid"
)

func ID(s string) (uuid.UUID, error) {
	v, err := uuid.Parse(s)
	if err != nil || v == uuid.Nil() {
		return uuid.Nil(), contribution.ErrValidation
	}
	return v, nil
}
func OptionalID(s *string) (uuid.UUID, error) {
	if s == nil {
		return uuid.Nil(), nil
	}
	return ID(*s)
}
func Value(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
func IDPointer(v uuid.UUID) *string {
	if v == uuid.Nil() {
		return nil
	}
	s := v.String()
	return &s
}
func Object(body []byte, required, allowed, nullables []string) (map[string]json.RawMessage, error) {
	var m map[string]json.RawMessage
	if json.Unmarshal(body, &m) != nil || m == nil {
		return nil, contribution.ErrValidation
	}
	for _, key := range required {
		if m[key] == nil {
			return nil, contribution.ErrValidation
		}
	}
	for key, v := range m {
		if !slices.Contains(allowed, key) || (bytes.Equal(bytes.TrimSpace(v), []byte("null")) && !slices.Contains(nullables, key)) {
			return nil, contribution.ErrValidation
		}
	}
	return m, nil
}

// Reject duplicate object keys recursively before decoding generated structs.
func unique(d *json.Decoder) error {
	token, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			s, ok := key.(string)
			if !ok || seen[s] {
				return contribution.ErrValidation
			}
			seen[s] = true
			if err = unique(d); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err = unique(d); err != nil {
				return err
			}
		}
	default:
		return contribution.ErrValidation
	}
	_, err = d.Token()
	return err
}
func Decode(c fiber.Ctx, target any, required, allowed, nullables []string) (map[string]json.RawMessage, error) {
	body := c.Body()
	media, _, err := mime.ParseMediaType(c.Get("Content-Type"))
	if err != nil || media != "application/json" || len(body) > 256*1024 || !utf8.Valid(body) {
		return nil, contribution.ErrValidation
	}
	d := json.NewDecoder(bytes.NewReader(body))
	if unique(d) != nil {
		return nil, contribution.ErrValidation
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, contribution.ErrValidation
	}
	fields, err := Object(body, required, allowed, nullables)
	if err != nil {
		return nil, err
	}
	d = json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if d.Decode(target) != nil || d.Decode(new(any)) != io.EOF {
		return nil, contribution.ErrValidation
	}
	return fields, nil
}
func Query(c fiber.Ctx, allowed []string) error {
	invalid := false
	c.Request().URI().QueryArgs().VisitAll(func(k, v []byte) {
		if !slices.Contains(allowed, string(k)) || len(v) == 0 || len(c.Request().URI().QueryArgs().PeekMulti(string(k))) != 1 {
			invalid = true
		}
	})
	if invalid {
		return contribution.ErrValidation
	}
	return nil
}
func Error(err error) (int, string, string, int) {
	var limit *contribution.LimitError
	switch {
	case errors.As(err, &limit):
		return 429, "CONTRIBUTION_LIMITED", "Contribution limit reached: " + limit.Reason + ".", limit.RetryAfter
	case errors.Is(err, contribution.ErrValidation), errors.Is(err, curation.ErrValidation):
		return 400, "VALIDATION_ERROR", "Check the contribution fields and review reason.", 0
	case errors.Is(err, contribution.ErrNotFound), errors.Is(err, curation.ErrNotFound):
		return 404, "CONTRIBUTION_NOT_FOUND", "Contribution or resource not found.", 0
	case errors.Is(err, contribution.ErrForbidden):
		return 403, "CONTRIBUTION_FORBIDDEN", "This contribution cannot be reviewed by this account.", 0
	case errors.Is(err, contribution.ErrVerified):
		return 403, "CONTRIBUTION_VERIFICATION_REQUIRED", "Verify your email before submitting.", 0
	case errors.Is(err, contribution.ErrConflict), errors.Is(err, curation.ErrConflict):
		return 409, "CONTRIBUTION_CONFLICT", "The contribution was processed or a canonical identifier is already in use. Reload before retrying.", 0
	case errors.Is(err, curation.ErrRelationCycle):
		return 409, "RESOURCE_RELATION_CYCLE", "This relation would create a cycle.", 0
	case errors.Is(err, contribution.ErrRequestConflict):
		return 409, "CONTRIBUTION_REQUEST_CONFLICT", "This request ID belongs to different content.", 0
	case errors.Is(err, resource.ErrVersionConflict):
		return 409, "RESOURCE_VERSION_CONFLICT", "The resource has changed. Reload its current content and submit a new proposal.", 0
	}
	return 0, "", "", 0
}
