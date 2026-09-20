package admin

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/curation"
	"github.com/deepfurry/tap4furry/server/internal/identity"
	"github.com/deepfurry/tap4furry/server/internal/resource"
	"github.com/deepfurry/tap4furry/server/internal/transport/admin/generated"
	"github.com/gofiber/fiber/v3"
	"io"
	"mime"
	"slices"
	"unicode/utf8"
	"uuid"
)

func (h *Handler) adminAccessGuard(c fiber.Ctx) error {
	if _, err := h.actor(c); err != nil {
		return respondError(c, err)
	}
	return c.Next()
}
func (h *Handler) curationBoundary(c fiber.Ctx) error {
	allowed := []string{"page", "page_size", "publication_state", "category_id", "slug", "expected_version"}
	unknown := false
	c.Request().URI().QueryArgs().VisitAll(func(key, _ []byte) {
		if !slices.Contains(allowed, string(key)) {
			unknown = true
		}
	})
	if unknown {
		return respondError(c, identity.ErrValidation)
	}
	for _, key := range []string{"page", "page_size", "publication_state", "category_id", "slug", "expected_version"} {
		v := c.Request().URI().QueryArgs().PeekMulti(key)
		if len(v) > 1 || (len(v) == 1 && len(v[0]) == 0) {
			return respondError(c, identity.ErrValidation)
		}
	}
	err := c.Next()
	if err != nil {
		var fe *fiber.Error
		if errors.As(err, &fe) && fe.Code == 400 {
			return respondError(c, identity.ErrValidation)
		}
		if errors.As(err, &fe) && (fe.Code == 404 || fe.Code == 405) {
			return respondError(c, curation.ErrNotFound)
		}
		return respondError(c, err)
	}
	return nil
}

// The separate Auth decoder retains its 8 KiB ceiling. Here null is accepted only
// for explicitly nullable text, and omitted PATCH fields remain distinguishable.
func decodeCuration(c fiber.Ctx, target any, required []string) (map[string]json.RawMessage, error) {
	body := c.Body()
	media, _, err := mime.ParseMediaType(c.Get("Content-Type"))
	if err != nil || media != "application/json" || len(body) > 256*1024 || !utf8.Valid(body) {
		return nil, identity.ErrValidation
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil || fields == nil {
		return nil, identity.ErrValidation
	}
	for _, key := range required {
		if fields[key] == nil {
			return nil, identity.ErrValidation
		}
	}
	for key, value := range fields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) && !slices.Contains([]string{"label", "summary", "description"}, key) {
			return nil, identity.ErrValidation
		}
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if d.Decode(target) != nil || d.Decode(new(any)) != io.EOF {
		return nil, identity.ErrValidation
	}
	return fields, nil
}
func parseID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil || id == uuid.Nil() {
		return uuid.Nil(), identity.ErrValidation
	}
	return id, nil
}
func revisionDTO(r curation.Revision) generated.ResourceRevision {
	return generated.ResourceRevision{Id: r.ID.String(), Version: r.Version}
}
func (h *Handler) resourceCommand(c fiber.Ctx, raw string, expected int64, work func(auth.AdminActor, uuid.UUID) (curation.Revision, error)) error {
	id, err := parseID(raw)
	if err != nil || expected < 1 {
		return respondError(c, identity.ErrValidation)
	}
	actor, err := h.actor(c)
	if err != nil {
		return respondError(c, err)
	}
	if h.options.Curation == nil {
		return respondError(c, errors.New("curation unavailable"))
	}
	result, err := work(actor, id)
	if err != nil {
		return respondError(c, err)
	}
	return c.JSON(revisionDTO(result))
}
func (h *Handler) CreateResource(c fiber.Ctx, _ generated.CreateResourceParams) error {
	var input generated.CreateResource
	if _, err := decodeCuration(c, &input, []string{"slug", "default_locale", "category_id", "content_rating", "localization"}); err != nil {
		return respondError(c, err)
	}
	category, err := parseID(input.CategoryId)
	if err != nil {
		return respondError(c, err)
	}
	actor, err := h.actor(c)
	if err != nil {
		return respondError(c, err)
	}
	if h.options.Curation == nil {
		return respondError(c, errors.New("curation unavailable"))
	}
	lifecycle := resource.Unknown
	if input.Lifecycle != nil {
		lifecycle = resource.Lifecycle(*input.Lifecycle)
	}
	result, err := h.options.Curation.CreateResource(c.Context(), actor, curation.CreateInput{Slug: input.Slug, DefaultLocale: input.DefaultLocale, CategoryID: category, ContentRating: resource.ContentRating(input.ContentRating), Lifecycle: lifecycle, Localization: localizationInput(input.Localization)})
	if err != nil {
		return respondError(c, err)
	}
	return c.Status(201).JSON(revisionDTO(result))
}
func localizationInput(in generated.ResourceLocalizationInput) curation.LocalizationInput {
	return curation.LocalizationInput{Name: in.Name, Summary: in.Summary, Description: in.Description}
}
func convertPointer[A ~string, B ~string](in *A) *B {
	if in == nil {
		return nil
	}
	v := B(*in)
	return &v
}
func (h *Handler) PatchResource(c fiber.Ctx, raw string, p generated.PatchResourceParams) error {
	return h.resourceCommand(c, raw, p.ExpectedVersion, func(actor auth.AdminActor, id uuid.UUID) (curation.Revision, error) {
		var input generated.PatchResource
		if _, err := decodeCuration(c, &input, nil); err != nil {
			return curation.Revision{}, err
		}
		patch := curation.CorePatch{Slug: input.Slug, DefaultLocale: input.DefaultLocale, Lifecycle: convertPointer[generated.Lifecycle, resource.Lifecycle](input.Lifecycle), ContentRating: convertPointer[generated.ContentRating, resource.ContentRating](input.ContentRating)}
		if input.CategoryId != nil {
			category, err := parseID(*input.CategoryId)
			if err != nil {
				return curation.Revision{}, err
			}
			patch.CategoryID = &category
		}
		return h.options.Curation.PatchResource(c.Context(), actor, id, p.ExpectedVersion, patch)
	})
}
func (h *Handler) DeleteResource(c fiber.Ctx, raw string, p generated.DeleteResourceParams) error {
	return h.resourceCommand(c, raw, p.ExpectedVersion, func(actor auth.AdminActor, id uuid.UUID) (curation.Revision, error) {
		return h.options.Curation.DeleteResource(c.Context(), actor, id, p.ExpectedVersion)
	})
}
func (h *Handler) PutResourceLocalization(c fiber.Ctx, raw, locale string, p generated.PutResourceLocalizationParams) error {
	return h.resourceCommand(c, raw, p.ExpectedVersion, func(actor auth.AdminActor, id uuid.UUID) (curation.Revision, error) {
		var input generated.ResourceLocalizationInput
		if _, err := decodeCuration(c, &input, []string{"name"}); err != nil {
			return curation.Revision{}, err
		}
		return h.options.Curation.PutLocalization(c.Context(), actor, id, p.ExpectedVersion, locale, localizationInput(input))
	})
}
func (h *Handler) DeleteResourceLocalization(c fiber.Ctx, raw, locale string, p generated.DeleteResourceLocalizationParams) error {
	return h.resourceCommand(c, raw, p.ExpectedVersion, func(actor auth.AdminActor, id uuid.UUID) (curation.Revision, error) {
		return h.options.Curation.DeleteLocalization(c.Context(), actor, id, p.ExpectedVersion, locale)
	})
}
func (h *Handler) SetResourceTags(c fiber.Ctx, raw string, p generated.SetResourceTagsParams) error {
	return h.resourceCommand(c, raw, p.ExpectedVersion, func(actor auth.AdminActor, id uuid.UUID) (curation.Revision, error) {
		var input generated.SetResourceTags
		if _, err := decodeCuration(c, &input, []string{"tag_ids"}); err != nil {
			return curation.Revision{}, err
		}
		ids := make([]uuid.UUID, 0, len(input.TagIds))
		for _, raw := range input.TagIds {
			id, err := parseID(raw)
			if err != nil {
				return curation.Revision{}, err
			}
			ids = append(ids, id)
		}
		return h.options.Curation.SetTags(c.Context(), actor, id, p.ExpectedVersion, ids)
	})
}
func (h *Handler) SetResourceExternalIDs(c fiber.Ctx, raw string, p generated.SetResourceExternalIDsParams) error {
	return h.resourceCommand(c, raw, p.ExpectedVersion, func(actor auth.AdminActor, id uuid.UUID) (curation.Revision, error) {
		var input generated.SetExternalIDs
		if _, err := decodeCuration(c, &input, []string{"items"}); err != nil {
			return curation.Revision{}, err
		}
		items := make([]resource.ExternalID, 0, len(input.Items))
		for _, item := range input.Items {
			items = append(items, resource.ExternalID{Namespace: item.Namespace, Value: item.ExternalId})
		}
		return h.options.Curation.SetExternalIDs(c.Context(), actor, id, p.ExpectedVersion, items)
	})
}
func (h *Handler) CreateSource(c fiber.Ctx, raw string, p generated.CreateSourceParams) error {
	return h.resourceCommand(c, raw, p.ExpectedVersion, func(actor auth.AdminActor, id uuid.UUID) (curation.Revision, error) {
		var input generated.CreateSource
		if _, err := decodeCuration(c, &input, []string{"url", "source_type", "availability_state", "is_primary"}); err != nil {
			return curation.Revision{}, err
		}
		return h.options.Curation.CreateSource(c.Context(), actor, id, p.ExpectedVersion, curation.SourceInput{URL: input.Url, Label: input.Label, Type: resource.SourceType(input.SourceType), Availability: resource.SourceAvailabilityState(input.AvailabilityState), Primary: input.IsPrimary})
	})
}
func (h *Handler) PatchSource(c fiber.Ctx, raw, source string, p generated.PatchSourceParams) error {
	return h.resourceCommand(c, raw, p.ExpectedVersion, func(actor auth.AdminActor, id uuid.UUID) (curation.Revision, error) {
		sid, err := parseID(source)
		if err != nil {
			return curation.Revision{}, err
		}
		var input generated.PatchSource
		fields, err := decodeCuration(c, &input, nil)
		if err != nil {
			return curation.Revision{}, err
		}
		return h.options.Curation.PatchSource(c.Context(), actor, id, p.ExpectedVersion, sid, curation.SourcePatch{URL: input.Url, Label: curation.NullableTextPatch{Present: fields["label"] != nil, Value: input.Label}, Type: convertPointer[generated.SourceType, resource.SourceType](input.SourceType), Availability: convertPointer[generated.AvailabilityState, resource.SourceAvailabilityState](input.AvailabilityState), Primary: input.IsPrimary})
	})
}
func (h *Handler) SetSourceRights(c fiber.Ctx, raw, source string, p generated.SetSourceRightsParams) error {
	return h.resourceCommand(c, raw, p.ExpectedVersion, func(actor auth.AdminActor, id uuid.UUID) (curation.Revision, error) {
		sid, err := parseID(source)
		if err != nil {
			return curation.Revision{}, err
		}
		var input generated.SetSourceRights
		if _, err = decodeCuration(c, &input, []string{"rights_status"}); err != nil {
			return curation.Revision{}, err
		}
		return h.options.Curation.SetSourceRights(c.Context(), actor, id, p.ExpectedVersion, sid, resource.SourceRightsStatus(input.RightsStatus))
	})
}
func (h *Handler) AddRelation(c fiber.Ctx, raw string, p generated.AddRelationParams) error {
	return h.resourceCommand(c, raw, p.ExpectedVersion, func(actor auth.AdminActor, id uuid.UUID) (curation.Revision, error) {
		var input generated.AddRelation
		if _, err := decodeCuration(c, &input, []string{"target_resource_id", "relation_type"}); err != nil {
			return curation.Revision{}, err
		}
		target, err := parseID(input.TargetResourceId)
		if err != nil {
			return curation.Revision{}, err
		}
		return h.options.Curation.AddRelation(c.Context(), actor, id, p.ExpectedVersion, target, resource.RelationType(input.RelationType))
	})
}
func (h *Handler) DeleteRelation(c fiber.Ctx, raw, relation string, p generated.DeleteRelationParams) error {
	return h.resourceCommand(c, raw, p.ExpectedVersion, func(actor auth.AdminActor, id uuid.UUID) (curation.Revision, error) {
		rid, err := parseID(relation)
		if err != nil {
			return curation.Revision{}, err
		}
		return h.options.Curation.DeleteRelation(c.Context(), actor, id, p.ExpectedVersion, rid)
	})
}
func (h *Handler) SetPublication(c fiber.Ctx, raw string, p generated.SetPublicationParams) error {
	return h.resourceCommand(c, raw, p.ExpectedVersion, func(actor auth.AdminActor, id uuid.UUID) (curation.Revision, error) {
		var input generated.SetPublication
		if _, err := decodeCuration(c, &input, []string{"state"}); err != nil {
			return curation.Revision{}, err
		}
		return h.options.Curation.SetPublication(c.Context(), actor, id, p.ExpectedVersion, resource.PublicationState(input.State))
	})
}
