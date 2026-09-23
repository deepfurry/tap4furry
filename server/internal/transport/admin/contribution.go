package admin

import (
	"errors"
	"github.com/deepfurry/tap4furry/server/internal/contribution"
	"github.com/deepfurry/tap4furry/server/internal/resource"
	"github.com/deepfurry/tap4furry/server/internal/transport/admin/generated"
	ch "github.com/deepfurry/tap4furry/server/internal/transport/contributionhttp"
	"github.com/gofiber/fiber/v3"
	"strconv"
)

func contributionError(c fiber.Ctx, err error) error {
	status, code, message, retry := ch.Error(err)
	if status == 0 {
		return respondError(c, err)
	}
	c.Set("Cache-Control", "no-store")
	if retry > 0 {
		c.Set("Retry-After", strconv.Itoa(retry))
	}
	return c.Status(status).JSON(generated.ApiError{Code: generated.ApiErrorCode(code), Message: message})
}
func (h *Handler) contributionBoundary(c fiber.Ctx) error {
	if err := ch.Query(c, []string{"page", "page_size", "status", "kind"}); err != nil {
		return contributionError(c, err)
	}
	if h.contributions == nil {
		return contributionError(c, errors.New("contribution unavailable"))
	}
	err := c.Next()
	var fe *fiber.Error
	if errors.As(err, &fe) {
		if fe.Code == 400 {
			return contributionError(c, contribution.ErrValidation)
		}
		if fe.Code == 404 || fe.Code == 405 {
			return contributionError(c, contribution.ErrNotFound)
		}
	}
	if err != nil {
		return contributionError(c, err)
	}
	return nil
}
func adminContributionContent(v contribution.Content) generated.ContributionContent {
	out := generated.ContributionContent{DefaultLocale: v.DefaultLocale, CategoryId: v.CategoryID.String(), Name: v.Name, Summary: v.Summary, Description: v.Description, Lifecycle: generated.ContributionContentLifecycle(v.Lifecycle), ContentRating: generated.ContributionContentContentRating(v.ContentRating)}
	if v.Slug != "" {
		s := v.Slug
		out.Slug = &s
	}
	if s := v.Source; s != nil {
		out.Source = &generated.ContributionSource{Url: s.URL, Label: s.Label, SourceType: generated.ContributionSourceSourceType(s.Type), AvailabilityState: generated.ContributionSourceAvailabilityState(s.Availability)}
	}
	return out
}
func (h *Handler) ListContributions(c fiber.Ctx, p generated.ListContributionsParams) error {
	actor, err := h.actor(c)
	if err != nil {
		return contributionError(c, err)
	}
	page, size, status, kind := int64(1), 20, "", ""
	if p.Page != nil {
		page = *p.Page
	}
	if p.PageSize != nil {
		size = *p.PageSize
	}
	if p.Status != nil {
		status = string(*p.Status)
	}
	if p.Kind != nil {
		kind = string(*p.Kind)
	}
	result, err := h.contributions.ReviewList(c.Context(), actor, page, size, status, kind)
	if err != nil {
		return contributionError(c, err)
	}
	out := generated.ContributionList{Items: []generated.ContributionSummary{}, Page: result.Page, PageSize: result.PageSize, HasNext: result.HasNext}
	for _, r := range result.Items {
		out.Items = append(out.Items, generated.ContributionSummary{Id: r.ID.String(), Kind: generated.ContributionKind(r.Kind), Status: generated.ContributionStatus(r.Status), Name: r.Name, CreatedAt: r.CreatedAt})
	}
	return c.JSON(out)
}
func (h *Handler) GetContribution(c fiber.Ctx, raw string) error {
	actor, err := h.actor(c)
	if err != nil {
		return contributionError(c, err)
	}
	key, err := ch.ID(raw)
	if err != nil {
		return contributionError(c, err)
	}
	r, err := h.contributions.ReviewDetail(c.Context(), actor, key)
	if err != nil {
		return contributionError(c, err)
	}
	out := generated.ContributionDetail{Id: r.ID.String(), AuthorId: r.AuthorID.String(), Kind: generated.ContributionKind(r.Kind), Status: generated.ContributionStatus(r.Status), Reason: r.Reason, PreviousId: ch.IDPointer(r.PreviousID), CreatedAt: r.CreatedAt, DecidedAt: r.DecidedAt, ProposedChange: adminChange(r.ProposedChange), BaseChange: adminChange(r.BaseChange), CurrentChange: adminChange(r.CurrentChange), AcceptedChange: adminChange(r.AcceptedChange), TargetResourceId: ch.IDPointer(r.TargetID), ResultResourceId: ch.IDPointer(r.ResultID), Conflict: r.Conflict, SelfReview: r.SelfReview, History: []generated.ContributionEvent{}}
	if !contribution.Extended(r.Kind) {
		v := adminContributionContent(r.Proposed)
		out.Proposed = &v
	}
	changes := []generated.ContributionResourceChange{}
	for _, v := range r.ResourceChanges {
		changes = append(changes, generated.ContributionResourceChange{ResourceId: v.ID.String(), BeforeVersion: v.Before, AfterVersion: v.After})
	}
	out.ResourceChanges = &changes
	if r.BaseVersion > 0 {
		out.BaseVersion = &r.BaseVersion
	}
	if r.Base != nil {
		v := adminContributionContent(*r.Base)
		out.Base = &v
	}
	if r.Current != nil {
		v := adminContributionContent(*r.Current)
		out.Current = &v
	}
	if r.Accepted != nil {
		v := adminContributionContent(*r.Accepted)
		out.Accepted = &v
	}
	for _, e := range r.ReviewHistory {
		out.History = append(out.History, generated.ContributionEvent{ActorId: e.ActorID.String(), EventType: generated.ContributionEventEventType(e.Type), Message: e.Message, InternalNote: e.InternalNote, OccurredAt: e.At})
	}
	return c.JSON(out)
}
func (h *Handler) AcceptContribution(c fiber.Ctx, raw string, _ generated.AcceptContributionParams) error {
	actor, err := h.actor(c)
	if err != nil {
		return contributionError(c, err)
	}
	key, err := ch.ID(raw)
	if err != nil {
		return contributionError(c, err)
	}
	var body generated.AcceptContribution
	fields, err := ch.Decode(c, &body, nil, []string{"content", "change", "message", "internal_note"}, []string{"message", "internal_note"})
	if err != nil {
		return contributionError(c, err)
	}
	if body.Change != nil {
		if body.Content != nil {
			return contributionError(c, contribution.ErrValidation)
		}
		change, e := ch.ParseChange(fields["change"], true)
		if e != nil {
			return contributionError(c, e)
		}
		if e = h.contributions.Accept(c.Context(), actor, key, contribution.AcceptInput{Change: change, Message: body.Message, InternalNote: body.InternalNote}); e != nil {
			return contributionError(c, e)
		}
		return c.SendStatus(204)
	}
	if body.Content == nil {
		return contributionError(c, contribution.ErrValidation)
	}
	fields, err = ch.Object(fields["content"], []string{"default_locale", "category_id", "name", "summary", "description", "lifecycle", "content_rating"}, []string{"default_locale", "category_id", "name", "summary", "description", "lifecycle", "content_rating", "slug", "source"}, []string{"summary", "description"})
	if err != nil {
		return contributionError(c, err)
	}
	p := body.Content
	category, err := ch.ID(p.CategoryId)
	if err != nil {
		return contributionError(c, err)
	}
	in := contribution.AcceptInput{Content: contribution.Content{DefaultLocale: p.DefaultLocale, CategoryID: category, Name: p.Name, Summary: p.Summary, Description: p.Description, Lifecycle: resource.Lifecycle(p.Lifecycle), ContentRating: resource.ContentRating(p.ContentRating), Slug: ch.Value(p.Slug)}, Message: body.Message, InternalNote: body.InternalNote}
	if s := p.Source; s != nil {
		if _, err = ch.Object(fields["source"], []string{"url", "label", "source_type", "availability_state"}, []string{"url", "label", "source_type", "availability_state"}, []string{"label"}); err != nil {
			return contributionError(c, err)
		}
		in.Content.Source = &contribution.Source{URL: s.Url, Label: s.Label, Type: resource.SourceType(s.SourceType), Availability: resource.SourceAvailabilityState(s.AvailabilityState)}
	}
	if err = h.contributions.Accept(c.Context(), actor, key, in); err != nil {
		return contributionError(c, err)
	}
	return c.SendStatus(204)
}
func (h *Handler) RejectContribution(c fiber.Ctx, raw string, _ generated.RejectContributionParams) error {
	actor, err := h.actor(c)
	if err != nil {
		return contributionError(c, err)
	}
	key, err := ch.ID(raw)
	if err != nil {
		return contributionError(c, err)
	}
	var body generated.RejectContribution
	if _, err = ch.Decode(c, &body, []string{"message"}, []string{"message", "internal_note"}, []string{"internal_note"}); err != nil {
		return contributionError(c, err)
	}
	if err = h.contributions.Reject(c.Context(), actor, key, body.Message, body.InternalNote); err != nil {
		return contributionError(c, err)
	}
	return c.SendStatus(204)
}
