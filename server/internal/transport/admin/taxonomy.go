package admin

import (
	"errors"
	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/curation"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/deepfurry/tap4furry/server/internal/taxonomy"
	"github.com/deepfurry/tap4furry/server/internal/transport/admin/generated"
	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5/pgtype"
	"uuid"
)

func (h *Handler) taxonomyCommand(c fiber.Ctx, raw string, work func(auth.AdminActor, uuid.UUID) error) error {
	id, err := parseID(raw)
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
	if err = work(actor, id); err != nil {
		return respondError(c, err)
	}
	return nil
}

func (h *Handler) ListCategories(c fiber.Ctx) error {
	return h.readSnapshot(c, func(q *sqlc.Queries) (any, error) {
		rows, err := q.ListAdminCategories(c.Context())
		if err != nil {
			return nil, err
		}
		result := generated.TaxonomyList{Items: []generated.TaxonomySummary{}}
		for _, r := range rows {
			dto, err := taxonomySummary(r.ID, r.Slug, r.DefaultLocale, r.State, r.Name)
			if err != nil {
				return nil, err
			}
			result.Items = append(result.Items, dto)
		}
		return result, nil
	})
}
func (h *Handler) GetCategory(c fiber.Ctx, raw string) error {
	id, err := parseID(raw)
	if err != nil {
		return respondError(c, err)
	}
	return h.readSnapshot(c, func(q *sqlc.Queries) (any, error) {
		row, err := q.GetAdminCategory(c.Context(), pgtype.UUID{Bytes: id, Valid: true})
		if err != nil {
			return nil, err
		}
		items, err := q.AdminCategoryLocalizations(c.Context(), row.ID)
		if err != nil {
			return nil, err
		}
		result := generated.TaxonomyDetail{Id: raw, Slug: row.Slug, State: generated.TaxonomyState(row.State), DefaultLocale: row.DefaultLocale, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time, Localizations: []generated.TaxonomyLocalization{}}
		canonical := false
		for _, l := range items {
			if l.Locale == row.DefaultLocale {
				canonical = true
			}
			result.Localizations = append(result.Localizations, generated.TaxonomyLocalization{Locale: l.Locale, Name: l.Name, Description: readText(l.Description)})
		}
		if !canonical {
			return nil, errCanonical
		}
		return result, nil
	})
}
func (h *Handler) CreateCategory(c fiber.Ctx, _ generated.CreateCategoryParams) error {
	var input generated.CreateTaxonomy
	if _, err := decodeCuration(c, &input, []string{"slug", "default_locale", "localization"}); err != nil {
		return respondError(c, err)
	}
	actor, err := h.actor(c)
	if err != nil {
		return respondError(c, err)
	}
	if h.options.Curation == nil {
		return respondError(c, errors.New("curation unavailable"))
	}
	id, err := h.options.Curation.CreateTaxonomy(c.Context(), actor, curation.Category, curation.TaxonomyInput{Slug: input.Slug, DefaultLocale: input.DefaultLocale, Name: input.Localization.Name, Description: input.Localization.Description})
	if err != nil {
		return respondError(c, err)
	}
	return c.Status(201).JSON(generated.EntityID{Id: id.String()})
}
func (h *Handler) PatchCategory(c fiber.Ctx, raw string, _ generated.PatchCategoryParams) error {
	return h.taxonomyCommand(c, raw, func(actor auth.AdminActor, id uuid.UUID) error {
		var input generated.PatchTaxonomy
		if _, err := decodeCuration(c, &input, nil); err != nil {
			return err
		}
		if err := h.options.Curation.PatchTaxonomy(c.Context(), actor, curation.Category, id, curation.TaxonomyPatch{Reason: stringValue(input.Reason), DefaultLocale: input.DefaultLocale, State: convertPointer[generated.TaxonomyState, taxonomy.CategoryState](input.State)}); err != nil {
			return err
		}
		return c.JSON(generated.EntityID{Id: id.String()})
	})
}
func (h *Handler) DeleteCategory(c fiber.Ctx, raw string, _ generated.DeleteCategoryParams) error {
	var input generated.GovernanceReason
	if _, err := decodeCuration(c, &input, []string{"reason"}); err != nil {
		return respondError(c, err)
	}
	return h.taxonomyCommand(c, raw, func(actor auth.AdminActor, id uuid.UUID) error {
		if err := h.options.Curation.DeleteTaxonomy(c.Context(), actor, curation.Category, id, input.Reason); err != nil {
			return err
		}
		return c.SendStatus(204)
	})
}
func (h *Handler) PutCategoryLocalization(c fiber.Ctx, raw, locale string, _ generated.PutCategoryLocalizationParams) error {
	return h.taxonomyCommand(c, raw, func(actor auth.AdminActor, id uuid.UUID) error {
		var input generated.TaxonomyLocalizationInput
		if _, err := decodeCuration(c, &input, []string{"name"}); err != nil {
			return err
		}
		if err := h.options.Curation.PutTaxonomyLocalization(c.Context(), actor, curation.Category, id, locale, input.Name, input.Description); err != nil {
			return err
		}
		return c.SendStatus(204)
	})
}
func (h *Handler) DeleteCategoryLocalization(c fiber.Ctx, raw, locale string, _ generated.DeleteCategoryLocalizationParams) error {
	return h.taxonomyCommand(c, raw, func(actor auth.AdminActor, id uuid.UUID) error {
		if err := h.options.Curation.DeleteTaxonomyLocalization(c.Context(), actor, curation.Category, id, locale); err != nil {
			return err
		}
		return c.SendStatus(204)
	})
}

func (h *Handler) ListTags(c fiber.Ctx) error {
	return h.readSnapshot(c, func(q *sqlc.Queries) (any, error) {
		rows, err := q.ListAdminTags(c.Context())
		if err != nil {
			return nil, err
		}
		result := generated.TaxonomyList{Items: []generated.TaxonomySummary{}}
		for _, r := range rows {
			dto, err := taxonomySummary(r.ID, r.Slug, r.DefaultLocale, r.State, r.Name)
			if err != nil {
				return nil, err
			}
			result.Items = append(result.Items, dto)
		}
		return result, nil
	})
}
func (h *Handler) GetTag(c fiber.Ctx, raw string) error {
	id, err := parseID(raw)
	if err != nil {
		return respondError(c, err)
	}
	return h.readSnapshot(c, func(q *sqlc.Queries) (any, error) {
		row, err := q.GetAdminTag(c.Context(), pgtype.UUID{Bytes: id, Valid: true})
		if err != nil {
			return nil, err
		}
		items, err := q.AdminTagLocalizations(c.Context(), row.ID)
		if err != nil {
			return nil, err
		}
		result := generated.TaxonomyDetail{Id: raw, Slug: row.Slug, State: generated.TaxonomyState(row.State), DefaultLocale: row.DefaultLocale, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time, Localizations: []generated.TaxonomyLocalization{}}
		canonical := false
		for _, l := range items {
			if l.Locale == row.DefaultLocale {
				canonical = true
			}
			result.Localizations = append(result.Localizations, generated.TaxonomyLocalization{Locale: l.Locale, Name: l.Name, Description: readText(l.Description)})
		}
		if !canonical {
			return nil, errCanonical
		}
		return result, nil
	})
}
func (h *Handler) CreateTag(c fiber.Ctx, _ generated.CreateTagParams) error {
	var input generated.CreateTaxonomy
	if _, err := decodeCuration(c, &input, []string{"slug", "default_locale", "localization"}); err != nil {
		return respondError(c, err)
	}
	actor, err := h.actor(c)
	if err != nil {
		return respondError(c, err)
	}
	if h.options.Curation == nil {
		return respondError(c, errors.New("curation unavailable"))
	}
	id, err := h.options.Curation.CreateTaxonomy(c.Context(), actor, curation.Tag, curation.TaxonomyInput{Slug: input.Slug, DefaultLocale: input.DefaultLocale, Name: input.Localization.Name, Description: input.Localization.Description})
	if err != nil {
		return respondError(c, err)
	}
	return c.Status(201).JSON(generated.EntityID{Id: id.String()})
}
func (h *Handler) PatchTag(c fiber.Ctx, raw string, _ generated.PatchTagParams) error {
	return h.taxonomyCommand(c, raw, func(actor auth.AdminActor, id uuid.UUID) error {
		var input generated.PatchTaxonomy
		if _, err := decodeCuration(c, &input, nil); err != nil {
			return err
		}
		if err := h.options.Curation.PatchTaxonomy(c.Context(), actor, curation.Tag, id, curation.TaxonomyPatch{Reason: stringValue(input.Reason), DefaultLocale: input.DefaultLocale, State: convertPointer[generated.TaxonomyState, taxonomy.CategoryState](input.State)}); err != nil {
			return err
		}
		return c.JSON(generated.EntityID{Id: id.String()})
	})
}
func (h *Handler) DeleteTag(c fiber.Ctx, raw string, _ generated.DeleteTagParams) error {
	var input generated.GovernanceReason
	if _, err := decodeCuration(c, &input, []string{"reason"}); err != nil {
		return respondError(c, err)
	}
	return h.taxonomyCommand(c, raw, func(actor auth.AdminActor, id uuid.UUID) error {
		if err := h.options.Curation.DeleteTaxonomy(c.Context(), actor, curation.Tag, id, input.Reason); err != nil {
			return err
		}
		return c.SendStatus(204)
	})
}
func (h *Handler) PutTagLocalization(c fiber.Ctx, raw, locale string, _ generated.PutTagLocalizationParams) error {
	return h.taxonomyCommand(c, raw, func(actor auth.AdminActor, id uuid.UUID) error {
		var input generated.TaxonomyLocalizationInput
		if _, err := decodeCuration(c, &input, []string{"name"}); err != nil {
			return err
		}
		if err := h.options.Curation.PutTaxonomyLocalization(c.Context(), actor, curation.Tag, id, locale, input.Name, input.Description); err != nil {
			return err
		}
		return c.SendStatus(204)
	})
}
func (h *Handler) DeleteTagLocalization(c fiber.Ctx, raw, locale string, _ generated.DeleteTagLocalizationParams) error {
	return h.taxonomyCommand(c, raw, func(actor auth.AdminActor, id uuid.UUID) error {
		if err := h.options.Curation.DeleteTaxonomyLocalization(c.Context(), actor, curation.Tag, id, locale); err != nil {
			return err
		}
		return c.SendStatus(204)
	})
}
