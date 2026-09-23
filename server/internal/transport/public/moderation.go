package public

import (
	"errors"
	"github.com/deepfurry/tap4furry/server/internal/moderation"
	mh "github.com/deepfurry/tap4furry/server/internal/transport/moderationhttp"
	"github.com/deepfurry/tap4furry/server/internal/transport/public/generated"
	"github.com/gofiber/fiber/v3"
	"strconv"
)

func moderationError(c fiber.Ctx, err error) error {
	c.Set("Cache-Control", "no-store")
	status, code, message, retry := mh.Error(err)
	if status == 0 {
		return respondError(c, err)
	}
	if retry > 0 {
		c.Set("Retry-After", strconv.Itoa(retry))
	}
	return c.Status(status).JSON(generated.ApiError{Code: generated.ApiErrorCode(code), Message: message})
}
func (h *Handler) moderationBoundary(c fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	var allowed []string
	if c.Method() == fiber.MethodGet && c.Path() == "/me/reports" {
		allowed = []string{"page", "page_size", "status"}
	}
	if e := mh.Query(c, allowed); e != nil {
		return moderationError(c, e)
	}
	if h.moderation == nil {
		return moderationError(c, errors.New("governance unavailable"))
	}
	err := c.Next()
	var fe *fiber.Error
	if errors.As(err, &fe) {
		if fe.Code == 400 {
			err = moderation.ErrValidation
		} else if fe.Code == 404 || fe.Code == 405 {
			err = moderation.ErrNotFound
		}
	}
	if err != nil {
		return moderationError(c, err)
	}
	return nil
}
func ownReportDTO(v moderation.OwnReport) generated.OwnReport {
	r := v.Row
	out := generated.OwnReport{Id: mh.IDString(r.ID), ResourceId: mh.IDString(r.ResourceID), SourceId: mh.IDPointer(r.SourceID), TargetKind: generated.OwnReportTargetKind(r.TargetKind), Reason: generated.OwnReportReason(r.Reason), Body: r.Body, Status: generated.OwnReportStatus(r.Status), CreatedAt: r.CreatedAt.Time, DecidedAt: mh.Time(r.DecidedAt), Events: []generated.ReportPublicEvent{}}
	if t := v.Target; t != nil {
		out.Target = &generated.ReportPublicTarget{ResourceId: mh.IDString(t.ID), Slug: t.Slug, Name: t.Name.String, SourceId: mh.IDPointer(t.SourceID)}
	}
	for _, e := range v.Events {
		out.Events = append(out.Events, generated.ReportPublicEvent{EventType: generated.ReportPublicEventEventType(e.EventType), SafeMessage: mh.Text[string](e.SafeMessage), OccurredAt: e.OccurredAt.Time})
	}
	return out
}
func (h *Handler) SubmitReport(c fiber.Ctx, _ generated.SubmitReportParams) error {
	actor, err := h.actor(c)
	if err != nil {
		return moderationError(c, err)
	}
	var input generated.ReportInput
	if err = mh.Decode(c, &input, []string{"request_id", "resource_id", "target_kind", "reason", "body"}, []string{"request_id", "resource_id", "source_id", "target_kind", "reason", "body"}); err != nil {
		return moderationError(c, err)
	}
	request, err := mh.ID(input.RequestId)
	if err != nil {
		return moderationError(c, err)
	}
	resource, err := mh.ID(input.ResourceId)
	if err != nil {
		return moderationError(c, err)
	}
	source, err := mh.OptionalID(input.SourceId)
	if err != nil {
		return moderationError(c, err)
	}
	key, err := h.moderation.SubmitReport(c.Context(), actor, moderation.ReportInput{RequestID: request, ResourceID: resource, SourceID: source, TargetKind: string(input.TargetKind), Reason: string(input.Reason), Body: input.Body})
	if err != nil {
		return moderationError(c, err)
	}
	return c.JSON(generated.RequestReceipt{Id: key.String()})
}
func (h *Handler) ListOwnReports(c fiber.Ctx, p generated.ListOwnReportsParams) error {
	actor, err := h.actor(c)
	if err != nil {
		return moderationError(c, err)
	}
	page, size := mh.Pagination(p.Page, p.PageSize)
	result, err := h.moderation.OwnReports(c.Context(), actor, page, size, mh.Value(p.Status))
	if err != nil {
		return moderationError(c, err)
	}
	out := generated.OwnReportList{Items: []generated.OwnReport{}, Page: result.Page, PageSize: int64(result.PageSize), HasNext: result.HasNext}
	for _, r := range result.Items {
		out.Items = append(out.Items, ownReportDTO(r))
	}
	return c.JSON(out)
}
func (h *Handler) GetOwnReport(c fiber.Ctx, raw string) error {
	actor, err := h.actor(c)
	if err != nil {
		return moderationError(c, err)
	}
	key, err := mh.ID(raw)
	if err != nil {
		return moderationError(c, err)
	}
	out, err := h.moderation.OwnReport(c.Context(), actor, key)
	if err != nil {
		return moderationError(c, err)
	}
	return c.JSON(ownReportDTO(out))
}
func (h *Handler) WithdrawReport(c fiber.Ctx, raw string, _ generated.WithdrawReportParams) error {
	actor, err := h.actor(c)
	if err != nil {
		return moderationError(c, err)
	}
	key, err := mh.ID(raw)
	if err != nil {
		return moderationError(c, err)
	}
	var in generated.ReportWithdrawal
	if err = mh.Decode(c, &in, []string{"request_id"}, []string{"request_id"}); err != nil {
		return moderationError(c, err)
	}
	request, err := mh.ID(in.RequestId)
	if err != nil {
		return moderationError(c, err)
	}
	if err = h.moderation.WithdrawReport(c.Context(), actor, key, request); err != nil {
		return moderationError(c, err)
	}
	return c.JSON(generated.RequestReceipt{Id: key.String()})
}
func quotaDTO(q moderation.Quota) generated.BusinessQuota {
	return generated.BusinessQuota{DailyLimit: q.DailyLimit, PendingLimit: q.PendingLimit, IntervalSeconds: q.IntervalSeconds, Remaining24h: q.Remaining24h, Pending: q.Pending, RetryAfter: q.RetryAfter}
}
func (h *Handler) GetOwnGovernance(c fiber.Ctx) error {
	actor, err := h.actor(c)
	if err != nil {
		return moderationError(c, err)
	}
	result, err := h.moderation.OwnGovernance(c.Context(), actor)
	if err != nil {
		return moderationError(c, err)
	}
	out := generated.OwnGovernance{Restrictions: []generated.ActiveRestriction{}, ContributionQuota: quotaDTO(result.ContributionQuota), ReportQuota: quotaDTO(result.ReportQuota)}
	for _, r := range result.Restrictions {
		out.Restrictions = append(out.Restrictions, generated.ActiveRestriction{Id: mh.IDString(r.ID), Scope: generated.ActiveRestrictionScope(r.Scope), ReasonCode: generated.ActiveRestrictionReasonCode(r.ReasonCode), UserMessage: r.UserMessage, StartsAt: r.StartsAt.Time, ExpiresAt: mh.Time(r.ExpiresAt)})
	}
	return c.JSON(out)
}
