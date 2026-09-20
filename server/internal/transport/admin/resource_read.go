package admin

import (
	"context"
	"errors"
	"github.com/deepfurry/tap4furry/server/internal/curation"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/deepfurry/tap4furry/server/internal/identity"
	"github.com/deepfurry/tap4furry/server/internal/resource"
	"github.com/deepfurry/tap4furry/server/internal/transport/admin/generated"
	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"math"
	"time"
	"uuid"
)

func readID(id pgtype.UUID) string { return uuid.UUID(id.Bytes).String() }
func readText(v pgtype.Text) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}
func readTime(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	return &v.Time
}

var errCanonical = errors.New("canonical localization missing")

func taxonomySummary(id pgtype.UUID, slug, locale, state string, name pgtype.Text) (generated.TaxonomySummary, error) {
	if !name.Valid {
		return generated.TaxonomySummary{}, errCanonical
	}
	return generated.TaxonomySummary{Id: readID(id), Slug: slug, DefaultLocale: locale, State: generated.TaxonomyState(state), Name: name.String}, nil
}
func (h *Handler) readSnapshot(c fiber.Ctx, work func(*sqlc.Queries) (any, error)) error {
	if h.options.ResourcePool == nil {
		return respondError(c, errors.New("admin resource reader unavailable"))
	}
	tx, err := h.options.ResourcePool.BeginTx(c.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return respondError(c, err)
	}
	defer tx.Rollback(c.Context())
	result, err := work(sqlc.New(tx))
	if errors.Is(err, pgx.ErrNoRows) {
		err = curation.ErrNotFound
	}
	if err != nil {
		return respondError(c, err)
	}
	if err = tx.Commit(c.Context()); err != nil {
		return respondError(c, err)
	}
	return c.JSON(result)
}
func (h *Handler) ListResources(c fiber.Ctx, p generated.ListResourcesParams) error {
	page, size := int64(1), int64(50)
	if p.Page != nil {
		page = *p.Page
	}
	if p.PageSize != nil {
		size = *p.PageSize
	}
	if page < 1 || size < 1 || size > 100 || page-1 > math.MaxInt64/size {
		return respondError(c, identity.ErrValidation)
	}
	args := sqlc.ListAdminResourcesParams{RowOffset: (page - 1) * size, RowLimit: size + 1}
	if p.PublicationState != nil {
		if !p.PublicationState.Valid() {
			return respondError(c, identity.ErrValidation)
		}
		args.PublicationState = pgtype.Text{String: string(*p.PublicationState), Valid: true}
	}
	if p.Slug != nil {
		if resource.ValidateSlug(*p.Slug) != nil {
			return respondError(c, identity.ErrValidation)
		}
		args.Slug = pgtype.Text{String: *p.Slug, Valid: true}
	}
	if p.CategoryId != nil {
		id, err := parseID(*p.CategoryId)
		if err != nil {
			return respondError(c, err)
		}
		args.CategoryID = pgtype.UUID{Bytes: id, Valid: true}
	}
	return h.readSnapshot(c, func(q *sqlc.Queries) (any, error) {
		rows, err := q.ListAdminResources(c.Context(), args)
		if err != nil {
			return nil, err
		}
		result := generated.ResourceList{Items: []generated.ResourceListItem{}, Page: page, PageSize: size, HasNext: int64(len(rows)) > size}
		if result.HasNext {
			rows = rows[:size]
		}
		for _, row := range rows {
			if !row.Name.Valid {
				return nil, errCanonical
			}
			category, err := taxonomySummary(row.CategoryID, row.CategorySlug, row.CategoryLocale, row.CategoryState, row.CategoryName)
			if err != nil {
				return nil, err
			}
			result.Items = append(result.Items, generated.ResourceListItem{Id: readID(row.ID), Slug: row.Slug, Name: row.Name.String, DefaultLocale: row.DefaultLocale, Category: category, PublicationState: generated.PublicationState(row.PublicationState), Lifecycle: generated.Lifecycle(row.Lifecycle), ContentRating: generated.ContentRating(row.ContentRating), Version: row.Version, PublishedAt: readTime(row.PublishedAt), UpdatedAt: row.UpdatedAt.Time})
		}
		return result, nil
	})
}
func (h *Handler) GetResource(c fiber.Ctx, raw string) error {
	id, err := parseID(raw)
	if err != nil {
		return respondError(c, err)
	}
	return h.readSnapshot(c, func(q *sqlc.Queries) (any, error) {
		return resourceSnapshot(c.Context(), q, pgtype.UUID{Bytes: id, Valid: true})
	})
}
func resourceSnapshot(ctx context.Context, q *sqlc.Queries, id pgtype.UUID) (generated.ResourceDetail, error) {
	row, err := q.GetAdminResource(ctx, id)
	if err != nil {
		return generated.ResourceDetail{}, err
	}
	category, err := taxonomySummary(row.CategoryID, row.CategorySlug, row.CategoryLocale, row.CategoryState, row.CategoryName)
	if err != nil {
		return generated.ResourceDetail{}, err
	}
	result := generated.ResourceDetail{Id: readID(row.ID), Slug: row.Slug, DefaultLocale: row.DefaultLocale, Category: category, PublicationState: generated.PublicationState(row.PublicationState), Lifecycle: generated.Lifecycle(row.Lifecycle), ContentRating: generated.ContentRating(row.ContentRating), Version: row.Version, PublishedAt: readTime(row.PublishedAt), CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time, Localizations: []generated.ResourceLocalization{}, Tags: []generated.TaxonomySummary{}, Sources: []generated.Source{}, Relations: []generated.Relation{}, ExternalIds: []generated.ExternalID{}}
	localizations, err := q.AdminResourceLocalizations(ctx, id)
	if err != nil {
		return result, err
	}
	canonical := false
	for _, l := range localizations {
		if l.Locale == row.DefaultLocale {
			canonical = true
		}
		result.Localizations = append(result.Localizations, generated.ResourceLocalization{Locale: l.Locale, Name: l.Name, Summary: readText(l.Summary), Description: readText(l.Description)})
	}
	if !canonical {
		return result, errCanonical
	}
	tags, err := q.AdminResourceTags(ctx, id)
	if err != nil {
		return result, err
	}
	for _, t := range tags {
		summary, err := taxonomySummary(t.ID, t.Slug, t.DefaultLocale, t.State, t.Name)
		if err != nil {
			return result, err
		}
		result.Tags = append(result.Tags, summary)
	}
	sources, err := q.AdminResourceSources(ctx, id)
	if err != nil {
		return result, err
	}
	for _, s := range sources {
		result.Sources = append(result.Sources, generated.Source{Id: readID(s.ID), Url: s.Url, Label: readText(s.Label), SourceType: generated.SourceType(s.SourceType), AvailabilityState: generated.AvailabilityState(s.AvailabilityState), RightsStatus: generated.RightsStatus(s.RightsStatus), IsPrimary: s.IsPrimary, CreatedAt: s.CreatedAt.Time, UpdatedAt: s.UpdatedAt.Time})
	}
	relations, err := q.AdminResourceRelations(ctx, id)
	if err != nil {
		return result, err
	}
	for _, r := range relations {
		if !r.OtherName.Valid {
			return result, errCanonical
		}
		dto := generated.Relation{Id: readID(r.ID), SourceResourceId: readID(r.SourceResourceID), TargetResourceId: readID(r.TargetResourceID), RelationType: generated.RelationType(r.RelationType), Direction: generated.RelationDirection(r.Direction)}
		dto.Other.Id = readID(r.OtherID)
		dto.Other.Slug = r.OtherSlug
		dto.Other.Name = r.OtherName.String
		dto.Other.PublicationState = generated.PublicationState(r.OtherPublicationState)
		result.Relations = append(result.Relations, dto)
	}
	external, err := q.AdminResourceExternalIDs(ctx, id)
	if err != nil {
		return result, err
	}
	for _, e := range external {
		result.ExternalIds = append(result.ExternalIds, generated.ExternalID{Namespace: e.Namespace, ExternalId: e.ExternalID})
	}
	return result, nil
}
