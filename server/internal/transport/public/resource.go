package public

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"uuid"

	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/deepfurry/tap4furry/server/internal/identity"
	"github.com/deepfurry/tap4furry/server/internal/resource"
	"github.com/deepfurry/tap4furry/server/internal/taxonomy"
	"github.com/deepfurry/tap4furry/server/internal/transport/public/generated"
	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const PublicReadCache = "no-store"

var errResourceMissing = errors.New("public resource not found")
var errCanonicalLocalization = errors.New("canonical localization missing")
var errResourceReader = errors.New("public resource reader unavailable")

// Catch generated parameter-binding errors before Fiber's generic error handler;
// never reflect malformed user input. No actor, cookie or Origin resolution here.
func (h *Handler) publicReadBoundary(c fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	for _, key := range []string{"locale", "page", "page_size"} {
		values := c.Request().URI().QueryArgs().PeekMulti(key)
		if len(values) > 1 || (len(values) == 1 && len(values[0]) == 0) {
			return respondError(c, identity.ErrValidation)
		}
	}
	err := c.Next()
	if err != nil {
		var fe *fiber.Error
		if errors.As(err, &fe) && fe.Code == 400 {
			return respondError(c, identity.ErrValidation)
		}
		return resourceReadError(c, err)
	}
	return nil
}

func readLocale(raw *string) (string, error) {
	if raw == nil {
		return "", nil
	}
	l, err := taxonomy.ParseLocale(*raw)
	if err != nil {
		return "", identity.ErrValidation
	}
	return string(l), nil
}

func resourceReadError(c fiber.Ctx, err error) error {
	if errors.Is(err, errResourceMissing) {
		c.Set("Cache-Control", "no-store")
		return c.Status(404).JSON(generated.ApiError{Code: generated.RESOURCENOTFOUND, Message: "Resource not found."})
	}
	if errors.Is(err, errCanonicalLocalization) {
		slog.Error("public read canonical localization invariant failed")
	} else if !errors.Is(err, identity.ErrValidation) {
		slog.Error("public resource read failed")
	}
	return respondError(c, err)
}

func readID(id pgtype.UUID) string { return uuid.UUID(id.Bytes).String() }

// P0-2A rejects empty optional text. SQL uses an empty sentinel only when both
// localized values are NULL, then the public JSON field is explicitly null.
func nullableReadText(text string) *string {
	if text == "" {
		return nil
	}
	return &text
}

func readPagination(params generated.ListResourcesParams) (int64, int, int64, error) {
	page, size := int64(1), 24
	if params.Page != nil {
		page = *params.Page
	}
	if params.PageSize != nil {
		size = *params.PageSize
	}
	if page < 1 || size < 1 || size > 100 || page-1 > math.MaxInt64/int64(size) {
		return 0, 0, 0, identity.ErrValidation
	}
	return page, size, (page - 1) * int64(size), nil
}

func (h *Handler) ListResources(c fiber.Ctx, params generated.ListResourcesParams) error {
	locale, err := readLocale(params.Locale)
	if err != nil {
		return resourceReadError(c, err)
	}
	page, size, offset, err := readPagination(params)
	if err != nil {
		return resourceReadError(c, err)
	}
	if h.resources == nil {
		return resourceReadError(c, errResourceReader)
	}
	rows, err := sqlc.New(h.resources).ListPublicResources(c.Context(), sqlc.ListPublicResourcesParams{Locale: locale, FetchLimit: int64(size + 1), PageOffset: offset})
	if err != nil {
		return resourceReadError(c, err)
	}
	result := generated.ResourceList{Items: []generated.ResourceListItem{}, Page: page, PageSize: size, HasNext: len(rows) > size}
	// Check the extra pagination row too; corruption cannot silently affect has_next.
	for _, row := range rows {
		if !row.CanonicalOk {
			return resourceReadError(c, errCanonicalLocalization)
		}
	}
	if result.HasNext {
		rows = rows[:size]
	}
	for _, row := range rows {
		result.Items = append(result.Items, generated.ResourceListItem{Id: readID(row.ID), Slug: row.Slug, Name: row.Name, Summary: nullableReadText(row.Summary),
			Category:  generated.CategoryRef{Id: readID(row.CategoryID), Slug: row.CategorySlug, Name: row.CategoryName},
			Lifecycle: generated.ResourceLifecycle(row.Lifecycle), ContentRating: generated.ResourceContentRating(row.ContentRating), PublishedAt: row.PublishedAt.Time, UpdatedAt: row.UpdatedAt.Time})
	}
	c.Set("Cache-Control", PublicReadCache)
	return c.JSON(result)
}

func (h *Handler) GetResource(c fiber.Ctx, slug string, params generated.GetResourceParams) error {
	if resource.ValidateSlug(slug) != nil {
		return resourceReadError(c, errResourceMissing)
	}
	locale, err := readLocale(params.Locale)
	if err != nil {
		return resourceReadError(c, err)
	}
	if h.resources == nil {
		return resourceReadError(c, errResourceReader)
	}
	result, err := h.readResource(c.Context(), slug, locale)
	if err != nil {
		return resourceReadError(c, err)
	}
	c.Set("Cache-Control", PublicReadCache)
	return c.JSON(result)
}

func (h *Handler) readResource(ctx context.Context, slug, locale string) (generated.ResourceDetail, error) {
	result := generated.ResourceDetail{Tags: []generated.TagRef{}, Sources: []generated.ResourceSource{}, Relations: []generated.ResourceRelation{}, ExternalIds: []generated.ResourceExternalID{}}
	// All child reads see the same public parent/endpoint state, even when a
	// curator changes visibility during this request. No write/row-lock privilege.
	tx, err := h.resources.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	q := sqlc.New(tx)
	r, err := q.GetPublicResourceBySlug(ctx, sqlc.GetPublicResourceBySlugParams{Slug: slug, Locale: locale})
	if errors.Is(err, pgx.ErrNoRows) {
		return result, errResourceMissing
	}
	if err != nil {
		return result, err
	}
	if !r.CanonicalOk {
		return result, errCanonicalLocalization
	}
	result.Id, result.Slug, result.Name = readID(r.ID), r.Slug, r.Name
	result.RequestedLocale, result.DefaultLocale = nullableReadText(locale), r.DefaultLocale
	result.Summary, result.Description = nullableReadText(r.Summary), nullableReadText(r.Description)
	result.Category = generated.CategoryRef{Id: readID(r.CategoryID), Slug: r.CategorySlug, Name: r.CategoryName}
	result.Lifecycle, result.ContentRating = generated.ResourceLifecycle(r.Lifecycle), generated.ResourceContentRating(r.ContentRating)
	result.PublishedAt, result.UpdatedAt = r.PublishedAt.Time, r.UpdatedAt.Time
	if result.AvailableLocales, err = q.ListPublicResourceLocales(ctx, r.ID); err != nil {
		return result, err
	}
	tags, err := q.ListPublicResourceTags(ctx, sqlc.ListPublicResourceTagsParams{ResourceID: r.ID, Locale: locale})
	if err != nil {
		return result, err
	}
	for _, tag := range tags {
		if !tag.CanonicalOk {
			return result, errCanonicalLocalization
		}
		result.Tags = append(result.Tags, generated.TagRef{Id: readID(tag.ID), Slug: tag.Slug, Name: tag.Name})
	}
	sources, err := q.ListPublicResourceSources(ctx, r.ID)
	if err != nil {
		return result, err
	}
	for _, source := range sources {
		result.Sources = append(result.Sources, generated.ResourceSource{Id: readID(source.ID), Url: source.Url, Label: nullableReadText(source.Label.String), SourceType: generated.ResourceSourceType(source.SourceType), AvailabilityState: generated.ResourceSourceAvailability(source.AvailabilityState), IsPrimary: source.IsPrimary})
	}
	relations, err := q.ListPublicResourceRelations(ctx, sqlc.ListPublicResourceRelationsParams{ResourceID: r.ID, Locale: locale})
	if err != nil {
		return result, err
	}
	for _, relation := range relations {
		if !relation.CanonicalOk {
			return result, errCanonicalLocalization
		}
		result.Relations = append(result.Relations, generated.ResourceRelation{Type: generated.ResourceRelationType(relation.RelationType), Direction: generated.ResourceRelationDirection(relation.Direction), Resource: generated.ResourceRef{Id: readID(relation.ID), Slug: relation.Slug, Name: relation.Name}})
	}
	ids, err := q.ListPublicResourceExternalIDs(ctx, r.ID)
	if err != nil {
		return result, err
	}
	for _, id := range ids {
		result.ExternalIds = append(result.ExternalIds, generated.ResourceExternalID{Namespace: id.Namespace, ExternalId: id.ExternalID})
	}
	return result, tx.Commit(ctx)
}

func (h *Handler) ListCategories(c fiber.Ctx, params generated.ListCategoriesParams) error {
	locale, err := readLocale(params.Locale)
	if err != nil {
		return resourceReadError(c, err)
	}
	if h.resources == nil {
		return resourceReadError(c, errResourceReader)
	}
	rows, err := sqlc.New(h.resources).ListPublicCategories(c.Context(), locale)
	if err != nil {
		return resourceReadError(c, err)
	}
	result := generated.CategoryList{Items: []generated.CategoryItem{}}
	for _, row := range rows {
		if !row.CanonicalOk {
			return resourceReadError(c, errCanonicalLocalization)
		}
		result.Items = append(result.Items, generated.CategoryItem{Id: readID(row.ID), Slug: row.Slug, Name: row.Name, Description: nullableReadText(row.Description)})
	}
	c.Set("Cache-Control", PublicReadCache)
	return c.JSON(result)
}

func (h *Handler) ListTags(c fiber.Ctx, params generated.ListTagsParams) error {
	locale, err := readLocale(params.Locale)
	if err != nil {
		return resourceReadError(c, err)
	}
	if h.resources == nil {
		return resourceReadError(c, errResourceReader)
	}
	rows, err := sqlc.New(h.resources).ListPublicTags(c.Context(), locale)
	if err != nil {
		return resourceReadError(c, err)
	}
	result := generated.TagList{Items: []generated.TagItem{}}
	for _, row := range rows {
		if !row.CanonicalOk {
			return resourceReadError(c, errCanonicalLocalization)
		}
		result.Items = append(result.Items, generated.TagItem{Id: readID(row.ID), Slug: row.Slug, Name: row.Name, Description: nullableReadText(row.Description)})
	}
	c.Set("Cache-Control", PublicReadCache)
	return c.JSON(result)
}
