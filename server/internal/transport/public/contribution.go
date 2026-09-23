package public

import (
	"errors"
	"github.com/deepfurry/tap4furry/server/internal/contribution"
	"github.com/deepfurry/tap4furry/server/internal/resource"
	ch "github.com/deepfurry/tap4furry/server/internal/transport/contributionhttp"
	"github.com/deepfurry/tap4furry/server/internal/transport/public/generated"
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
	c.Set("Cache-Control", "no-store")
	if err := ch.Query(c, []string{"page", "page_size", "status"}); err != nil {
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
func publicContributionContent(v contribution.Content) generated.ContributionContent {
	out := generated.ContributionContent{DefaultLocale: v.DefaultLocale, CategoryId: v.CategoryID.String(), Name: v.Name, Summary: v.Summary, Description: v.Description, Lifecycle: generated.ContributionContentLifecycle(v.Lifecycle), ContentRating: generated.ContributionContentContentRating(v.ContentRating)}
	if s := v.Source; s != nil {
		out.Source = &generated.ContributionSource{Url: s.URL, Label: s.Label, SourceType: generated.ContributionSourceSourceType(s.Type), AvailabilityState: generated.ContributionSourceAvailabilityState(s.Availability)}
	}
	return out
}
func originalContribution(v contribution.Detail) generated.ContributionOriginal {
	p := v.Proposed
	bits := v.SubmittedFields
	out := generated.ContributionOriginal{}
	if bits&contribution.FieldName != 0 {
		out.Name = &p.Name
	}
	if bits&contribution.FieldLocale != 0 {
		out.DefaultLocale = &p.DefaultLocale
	}
	if bits&contribution.FieldSummary != 0 {
		out.Summary = &p.Summary
	}
	if bits&contribution.FieldDescription != 0 {
		out.Description = &p.Description
	}
	if bits&contribution.FieldCategory != 0 {
		s := p.CategoryID.String()
		out.CategoryId = &s
	}
	if bits&contribution.FieldLifecycle != 0 {
		s := generated.ContributionOriginalLifecycle(p.Lifecycle)
		out.Lifecycle = &s
	}
	if bits&contribution.FieldRating != 0 {
		s := generated.ContributionOriginalContentRating(p.ContentRating)
		out.ContentRating = &s
	}
	if p.Source != nil {
		s := generated.ContributionSourceInputSourceType(p.Source.Type)
		out.Source = &generated.ContributionSourceInput{Url: p.Source.URL, Label: p.Source.Label, SourceType: &s}
	}
	return out
}
func (h *Handler) GetContributionContext(c fiber.Ctx, slug string) error {
	actor, err := h.actor(c)
	if err != nil {
		return contributionError(c, err)
	}
	result, err := h.contributions.Context(c.Context(), actor, slug)
	if err != nil {
		return contributionError(c, err)
	}
	return c.JSON(generated.ContributionContext{ResourceId: result.ResourceID.String(), BaseRevision: result.BaseRevision, Content: publicContributionContent(result.Content)})
}
func (h *Handler) SubmitContribution(c fiber.Ctx, _ generated.SubmitContributionParams) error {
	actor, err := h.actor(c)
	if err != nil {
		return contributionError(c, err)
	}
	var body generated.SubmitContribution
	fields, err := ch.Decode(c, &body, []string{"kind", "request_id", "reason", "content"}, []string{"kind", "request_id", "target_resource_id", "base_revision", "previous_id", "reason", "content"}, nil)
	if err != nil {
		return contributionError(c, err)
	}
	content, err := ch.Object(fields["content"], nil, []string{"default_locale", "category_id", "name", "summary", "description", "lifecycle", "content_rating", "source"}, []string{"summary", "description"})
	if err != nil {
		return contributionError(c, err)
	}
	in := contribution.SubmitInput{Kind: string(body.Kind), Reason: body.Reason, BaseRevision: ch.Value(body.BaseRevision)}
	in.RequestID, err = ch.ID(body.RequestId)
	if err != nil {
		return contributionError(c, err)
	}
	in.TargetID, err = ch.OptionalID(body.TargetResourceId)
	if err != nil {
		return contributionError(c, err)
	}
	in.PreviousID, err = ch.OptionalID(body.PreviousId)
	if err != nil {
		return contributionError(c, err)
	}
	p := body.Content
	in.Content = contribution.Patch{DefaultLocale: p.DefaultLocale, Name: p.Name, Summary: contribution.NullableText{Set: content["summary"] != nil, Value: p.Summary}, Description: contribution.NullableText{Set: content["description"] != nil, Value: p.Description}, SourceSet: content["source"] != nil}
	if p.CategoryId != nil {
		v, e := ch.ID(*p.CategoryId)
		if e != nil {
			return contributionError(c, e)
		}
		in.Content.CategoryID = &v
	}
	if p.Lifecycle != nil {
		v := resource.Lifecycle(*p.Lifecycle)
		in.Content.Lifecycle = &v
	}
	if p.ContentRating != nil {
		v := resource.ContentRating(*p.ContentRating)
		in.Content.ContentRating = &v
	}
	if p.Source != nil {
		if _, err = ch.Object(content["source"], []string{"url"}, []string{"url", "label", "source_type"}, []string{"label"}); err != nil {
			return contributionError(c, err)
		}
		kind := resource.SourceType("unknown")
		if p.Source.SourceType != nil {
			kind = resource.SourceType(*p.Source.SourceType)
		}
		in.Content.Source = &contribution.Source{URL: p.Source.Url, Label: p.Source.Label, Type: kind, Availability: "active"}
	}
	result, err := h.contributions.Submit(c.Context(), actor, in)
	if err != nil {
		return contributionError(c, err)
	}
	return c.Status(201).JSON(generated.ContributionCreated{Id: result.String()})
}
func (h *Handler) ListMyContributions(c fiber.Ctx, p generated.ListMyContributionsParams) error {
	actor, err := h.actor(c)
	if err != nil {
		return contributionError(c, err)
	}
	page, size, status := int64(1), 20, ""
	if p.Page != nil {
		page = *p.Page
	}
	if p.PageSize != nil {
		size = *p.PageSize
	}
	if p.Status != nil {
		status = string(*p.Status)
	}
	result, err := h.contributions.OwnList(c.Context(), actor, page, size, status)
	if err != nil {
		return contributionError(c, err)
	}
	out := generated.ContributionList{Items: []generated.ContributionSummary{}, Page: result.Page, PageSize: result.PageSize, HasNext: result.HasNext, Limits: generated.ContributionLimits{Remaining24h: result.Limits.Remaining24h, PendingCount: result.Limits.Pending, PendingLimit: 5, Reason: result.Limits.Reason, RetryAfterSeconds: result.Limits.RetryAfter}}
	for _, r := range result.Items {
		out.Items = append(out.Items, generated.ContributionSummary{Id: r.ID.String(), Kind: generated.ContributionKind(r.Kind), Status: generated.ContributionStatus(r.Status), Name: r.Name, CreatedAt: r.CreatedAt})
	}
	return c.JSON(out)
}
func (h *Handler) GetMyContribution(c fiber.Ctx, raw string) error {
	actor, err := h.actor(c)
	if err != nil {
		return contributionError(c, err)
	}
	key, err := ch.ID(raw)
	if err != nil {
		return contributionError(c, err)
	}
	result, err := h.contributions.OwnDetail(c.Context(), actor, key)
	if err != nil {
		return contributionError(c, err)
	}
	out := generated.ContributionDetail{Id: result.ID.String(), Kind: generated.ContributionKind(result.Kind), Status: generated.ContributionStatus(result.Status), Reason: result.Reason, PreviousId: ch.IDPointer(result.PreviousID), CreatedAt: result.CreatedAt, DecidedAt: result.DecidedAt, Proposed: originalContribution(result), History: []generated.ContributionEvent{}}
	if result.Accepted != nil {
		v := publicContributionContent(*result.Accepted)
		out.Accepted = &v
	}
	if result.Result != nil {
		r := result.Result
		out.Result = &generated.ContributionResult{Id: r.ID.String(), Slug: r.Slug, Name: r.Name}
	}
	if result.Target != nil {
		r := result.Target
		out.Target = &generated.ContributionResult{Id: r.ID.String(), Slug: r.Slug, Name: r.Name}
	}
	for _, e := range result.History {
		out.History = append(out.History, generated.ContributionEvent{EventType: generated.ContributionEventEventType(e.Type), Message: e.Message, OccurredAt: e.At})
	}
	return c.JSON(out)
}
func (h *Handler) WithdrawContribution(c fiber.Ctx, raw string, _ generated.WithdrawContributionParams) error {
	actor, err := h.actor(c)
	if err != nil {
		return contributionError(c, err)
	}
	key, err := ch.ID(raw)
	if err != nil {
		return contributionError(c, err)
	}
	if err = h.contributions.Withdraw(c.Context(), actor, key); err != nil {
		return contributionError(c, err)
	}
	return c.SendStatus(204)
}
