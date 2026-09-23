package admin

import (
	"errors"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/deepfurry/tap4furry/server/internal/moderation"
	"github.com/deepfurry/tap4furry/server/internal/transport/admin/generated"
	mh "github.com/deepfurry/tap4furry/server/internal/transport/moderationhttp"
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
	var allowed []string
	if c.Method() == fiber.MethodGet {
		switch c.Path() {
		case "/reports":
			allowed = []string{"page", "page_size", "status", "reason", "queue"}
		case "/source-checks":
			allowed = []string{"page", "page_size", "resource_id", "source_id", "availability_state", "open_broken_report"}
		case "/audit":
			allowed = []string{"page", "page_size", "resource_id", "user_id", "operation"}
		}
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
func adminReportDTO(v moderation.AdminReport) generated.AdminReport {
	r := v.Row
	out := generated.AdminReport{Id: mh.IDString(r.ID), ReporterId: mh.IDString(r.ReporterID), ResourceId: mh.IDString(r.ResourceID), SourceId: mh.IDPointer(r.SourceID), TargetKind: generated.AdminReportTargetKind(r.TargetKind), Reason: generated.AdminReportReason(r.Reason), Body: r.Body, Status: generated.AdminReportStatus(r.Status), CreatedAt: r.CreatedAt.Time, DecidedAt: mh.Time(r.DecidedAt), Queue: generated.AdminReportQueue(r.Queue), Version: r.Version, Priority: int64(r.Priority), DuplicateOf: mh.IDPointer(r.DuplicateOf), Events: []generated.ReportStaffEvent{}}
	for _, e := range v.Events {
		out.Events = append(out.Events, generated.ReportStaffEvent{Id: mh.IDString(e.ID), ActorId: mh.IDString(e.ActorID), EventType: generated.ReportStaffEventEventType(e.EventType), SafeMessage: mh.Text[string](e.SafeMessage), InternalNote: mh.Text[string](e.InternalNote), OccurredAt: e.OccurredAt.Time, AuditId: mh.IDPointer(e.AuditID), ResolutionType: mh.Text[generated.ReportStaffEventResolutionType](e.ResolutionType)})
	}
	return out
}
func (h *Handler) ListReports(c fiber.Ctx, p generated.ListReportsParams) error {
	actor, e := h.actor(c)
	if e != nil {
		return moderationError(c, e)
	}
	page, size := mh.Pagination(p.Page, p.PageSize)
	result, e := h.moderation.AdminReports(c.Context(), actor, page, size, mh.Value(p.Status), mh.Value(p.Reason), mh.Value(p.Queue))
	if e != nil {
		return moderationError(c, e)
	}
	out := generated.AdminReportList{Items: []generated.AdminReport{}, Page: result.Page, PageSize: int64(result.PageSize), HasNext: result.HasNext}
	for _, r := range result.Items {
		out.Items = append(out.Items, adminReportDTO(r))
	}
	return c.JSON(out)
}
func (h *Handler) GetReport(c fiber.Ctx, raw string) error {
	actor, e := h.actor(c)
	if e != nil {
		return moderationError(c, e)
	}
	key, e := mh.ID(raw)
	if e != nil {
		return moderationError(c, e)
	}
	out, e := h.moderation.AdminReport(c.Context(), actor, key)
	if e != nil {
		return moderationError(c, e)
	}
	return c.JSON(adminReportDTO(out))
}
func (h *Handler) reportDecision(c fiber.Ctx, raw string, in moderation.ReportDecision) error {
	actor, e := h.actor(c)
	if e != nil {
		return moderationError(c, e)
	}
	key, e := mh.ID(raw)
	if e != nil {
		return moderationError(c, e)
	}
	if e = h.moderation.DecideReport(c.Context(), actor, key, in); e != nil {
		return moderationError(c, e)
	}
	return c.JSON(generated.RequestReceipt{Id: key.String()})
}
func (h *Handler) TriageReport(c fiber.Ctx, raw string, _ generated.TriageReportParams) error {
	var in generated.ReportTriage
	if e := mh.Decode(c, &in, []string{"request_id", "expected_report_version", "action"}, []string{"request_id", "expected_report_version", "action", "internal_note"}); e != nil {
		return moderationError(c, e)
	}
	request, e := mh.ID(in.RequestId)
	if e != nil {
		return moderationError(c, e)
	}
	return h.reportDecision(c, raw, moderation.ReportDecision{RequestID: request, ExpectedVersion: in.ExpectedReportVersion, Kind: string(in.Action), InternalNote: mh.Value(in.InternalNote)})
}
func (h *Handler) DismissReport(c fiber.Ctx, raw string, _ generated.DismissReportParams) error {
	var in generated.ReportDismiss
	if e := mh.Decode(c, &in, []string{"request_id", "expected_report_version", "safe_message"}, []string{"request_id", "expected_report_version", "safe_message", "internal_note", "duplicate_of"}); e != nil {
		return moderationError(c, e)
	}
	request, e := mh.ID(in.RequestId)
	if e != nil {
		return moderationError(c, e)
	}
	duplicate, e := mh.OptionalID(in.DuplicateOf)
	if e != nil {
		return moderationError(c, e)
	}
	return h.reportDecision(c, raw, moderation.ReportDecision{RequestID: request, ExpectedVersion: in.ExpectedReportVersion, Kind: "dismiss", SafeMessage: in.SafeMessage, InternalNote: mh.Value(in.InternalNote), DuplicateID: duplicate})
}
func (h *Handler) ResolveReport(c fiber.Ctx, raw string, _ generated.ResolveReportParams) error {
	var in generated.ReportResolve
	base := []string{"request_id", "expected_report_version", "safe_message", "mode"}
	all := append(append([]string{}, base...), "internal_note", "audit_id", "expected_resource_version", "reason", "publication_state", "availability_state", "rights_status", "policy")
	if e := mh.Decode(c, &in, base, all); e != nil {
		return moderationError(c, e)
	}
	required := append([]string{}, base...)
	allowed := append(append([]string{}, base...), "internal_note")
	switch string(in.Mode) {
	case "no_change":
	case "link_audit":
		required = append(required, "audit_id")
		allowed = append(allowed, "audit_id")
	case "publication", "source_availability", "source_rights", "distribution":
		field := map[string]string{"publication": "publication_state", "source_availability": "availability_state", "source_rights": "rights_status", "distribution": "policy"}[string(in.Mode)]
		required = append(required, "expected_resource_version", "reason", field)
		allowed = append(allowed, "expected_resource_version", "reason", field)
	default:
		return moderationError(c, moderation.ErrValidation)
	}
	if e := mh.Decode(c, &in, required, allowed); e != nil {
		return moderationError(c, e)
	}
	request, e := mh.ID(in.RequestId)
	if e != nil {
		return moderationError(c, e)
	}
	audit, e := mh.OptionalID(in.AuditId)
	if e != nil {
		return moderationError(c, e)
	}
	decision := moderation.ReportDecision{RequestID: request, ExpectedVersion: in.ExpectedReportVersion, Kind: "resolve", Mode: string(in.Mode), SafeMessage: in.SafeMessage, InternalNote: mh.Value(in.InternalNote), AuditID: audit, Reason: mh.Value(in.Reason)}
	if in.ExpectedResourceVersion != nil {
		decision.ResourceVersion = *in.ExpectedResourceVersion
	}
	switch in.Mode {
	case "publication":
		decision.State = mh.Value(in.PublicationState)
	case "source_availability":
		decision.State = mh.Value(in.AvailabilityState)
	case "source_rights":
		decision.State = mh.Value(in.RightsStatus)
	case "distribution":
		decision.State = mh.Value(in.Policy)
	}
	return h.reportDecision(c, raw, decision)
}
func (h *Handler) GetUserGovernance(c fiber.Ctx, raw string) error {
	actor, e := h.actor(c)
	if e != nil {
		return moderationError(c, e)
	}
	key, e := mh.ID(raw)
	if e != nil {
		return moderationError(c, e)
	}
	v, e := h.moderation.UserGovernance(c.Context(), actor, key)
	if e != nil {
		return moderationError(c, e)
	}
	out := generated.UserGovernance{UserId: key.String(), TrustLevel: generated.UserGovernanceTrustLevel(v.Trust), Revision: v.Revision, Restrictions: []generated.AdminRestriction{}}
	for _, r := range v.Restrictions {
		out.Restrictions = append(out.Restrictions, generated.AdminRestriction{Id: mh.IDString(r.ID), Scope: generated.AdminRestrictionScope(r.Scope), ReasonCode: generated.AdminRestrictionReasonCode(r.ReasonCode), UserMessage: r.UserMessage, InternalNote: mh.Text[string](r.InternalNote), CreatedBy: mh.IDString(r.CreatedBy), StartsAt: r.StartsAt.Time, ExpiresAt: mh.Time(r.ExpiresAt), RevokedAt: mh.Time(r.RevokedAt), RevokedBy: mh.IDPointer(r.RevokedBy), RevokeReason: mh.Text[string](r.RevokeReason)})
	}
	return c.JSON(out)
}
func (h *Handler) userChange(c fiber.Ctx, raw string, in moderation.UserChange) error {
	actor, e := h.actor(c)
	if e != nil {
		return moderationError(c, e)
	}
	key, e := mh.ID(raw)
	if e != nil {
		return moderationError(c, e)
	}
	out, e := h.moderation.ChangeUser(c.Context(), actor, key, in)
	if e != nil {
		return moderationError(c, e)
	}
	return c.JSON(generated.RequestReceipt{Id: out.String()})
}
func (h *Handler) UpdateUserTrust(c fiber.Ctx, raw string, _ generated.UpdateUserTrustParams) error {
	var in generated.TrustUpdate
	if e := mh.Decode(c, &in, []string{"request_id", "expected_revision", "trust_level", "reason"}, []string{"request_id", "expected_revision", "trust_level", "reason", "internal_note"}); e != nil {
		return moderationError(c, e)
	}
	request, e := mh.ID(in.RequestId)
	if e != nil {
		return moderationError(c, e)
	}
	return h.userChange(c, raw, moderation.UserChange{Kind: "trust", RequestID: request, ExpectedRevision: in.ExpectedRevision, Trust: string(in.TrustLevel), Reason: in.Reason, InternalNote: mh.Value(in.InternalNote)})
}
func (h *Handler) CreateRestriction(c fiber.Ctx, raw string, _ generated.CreateRestrictionParams) error {
	var in generated.RestrictionCreate
	if e := mh.Decode(c, &in, []string{"request_id", "expected_revision", "scope", "duration", "reason_code", "user_message"}, []string{"request_id", "expected_revision", "scope", "duration", "reason_code", "user_message", "internal_note", "replaces_restriction_id"}); e != nil {
		return moderationError(c, e)
	}
	request, e := mh.ID(in.RequestId)
	if e != nil {
		return moderationError(c, e)
	}
	replaces, e := mh.OptionalID(in.ReplacesRestrictionId)
	if e != nil {
		return moderationError(c, e)
	}
	return h.userChange(c, raw, moderation.UserChange{Kind: "restrict", RequestID: request, ExpectedRevision: in.ExpectedRevision, Scope: string(in.Scope), Duration: string(in.Duration), ReasonCode: string(in.ReasonCode), Message: in.UserMessage, InternalNote: mh.Value(in.InternalNote), ReplacesID: replaces})
}
func (h *Handler) RevokeRestriction(c fiber.Ctx, raw, rid string, _ generated.RevokeRestrictionParams) error {
	var in generated.RestrictionRevoke
	if e := mh.Decode(c, &in, []string{"request_id", "expected_revision", "reason"}, []string{"request_id", "expected_revision", "reason"}); e != nil {
		return moderationError(c, e)
	}
	request, e := mh.ID(in.RequestId)
	if e != nil {
		return moderationError(c, e)
	}
	restriction, e := mh.ID(rid)
	if e != nil {
		return moderationError(c, e)
	}
	return h.userChange(c, raw, moderation.UserChange{Kind: "revoke_restriction", RequestID: request, ExpectedRevision: in.ExpectedRevision, RestrictionID: restriction, Reason: in.Reason})
}
func (h *Handler) UpdateResourceDistribution(c fiber.Ctx, raw string, _ generated.UpdateResourceDistributionParams) error {
	actor, e := h.actor(c)
	if e != nil {
		return moderationError(c, e)
	}
	key, e := mh.ID(raw)
	if e != nil {
		return moderationError(c, e)
	}
	var in generated.DistributionUpdate
	if e = mh.Decode(c, &in, []string{"request_id", "expected_version", "policy", "reason"}, []string{"request_id", "expected_version", "policy", "reason"}); e != nil {
		return moderationError(c, e)
	}
	request, e := mh.ID(in.RequestId)
	if e != nil {
		return moderationError(c, e)
	}
	out, e := h.moderation.SetDistribution(c.Context(), actor, key, moderation.DistributionInput{RequestID: request, ExpectedVersion: in.ExpectedVersion, Policy: string(in.Policy), Reason: in.Reason})
	if e != nil {
		return moderationError(c, e)
	}
	return c.JSON(generated.RequestReceipt{Id: out.String()})
}
func (h *Handler) RecordSourceCheck(c fiber.Ctx, raw, sid string, _ generated.RecordSourceCheckParams) error {
	actor, e := h.actor(c)
	if e != nil {
		return moderationError(c, e)
	}
	key, e := mh.ID(raw)
	if e != nil {
		return moderationError(c, e)
	}
	source, e := mh.ID(sid)
	if e != nil {
		return moderationError(c, e)
	}
	var in generated.SourceCheckInput
	if e = mh.Decode(c, &in, []string{"request_id", "expected_version", "outcome", "observed_at", "note"}, []string{"request_id", "expected_version", "outcome", "observed_at", "note", "availability_state"}); e != nil {
		return moderationError(c, e)
	}
	request, e := mh.ID(in.RequestId)
	if e != nil {
		return moderationError(c, e)
	}
	out, e := h.moderation.RecordSourceCheck(c.Context(), actor, key, source, moderation.SourceCheckInput{RequestID: request, ExpectedVersion: in.ExpectedVersion, Outcome: string(in.Outcome), ObservedAt: in.ObservedAt, Note: in.Note, Availability: mh.Value(in.AvailabilityState)})
	if e != nil {
		return moderationError(c, e)
	}
	return c.JSON(generated.RequestReceipt{Id: out.String()})
}
func (h *Handler) ListSourceHealth(c fiber.Ctx, p generated.ListSourceHealthParams) error {
	actor, e := h.actor(c)
	if e != nil {
		return moderationError(c, e)
	}
	key, e := mh.OptionalID(p.ResourceId)
	if e != nil {
		return moderationError(c, e)
	}
	page, size := mh.Pagination(p.Page, p.PageSize)
	source, e := mh.OptionalID(p.SourceId)
	if e != nil {
		return moderationError(c, e)
	}
	v, e := h.moderation.SourceHealth(c.Context(), actor, page, size, mh.Value(p.AvailabilityState), p.OpenBrokenReport, key, source)
	if e != nil {
		return moderationError(c, e)
	}
	out := generated.SourceHealthList{Items: []generated.SourceHealth{}, Page: v.Page, PageSize: int64(v.PageSize), HasNext: v.HasNext}
	for _, r := range v.Items {
		row := r.Row
		item := generated.SourceHealth{SourceId: mh.IDString(row.SourceID), ResourceId: mh.IDString(row.ResourceID), ResourceVersion: row.ResourceVersion, ResourceSlug: row.ResourceSlug, Url: row.Url, AvailabilityState: generated.SourceHealthAvailabilityState(row.AvailabilityState), RightsStatus: generated.SourceHealthRightsStatus(row.RightsStatus), OpenBrokenReport: row.OpenBrokenReport}
		if s := r.Check; s != nil {
			item.LastCheck = &generated.SourceCheck{Id: mh.IDString(s.ID), SourceId: mh.IDString(s.SourceID), ResourceId: mh.IDString(s.ResourceID), ResourceVersion: s.ResourceVersion, Outcome: generated.SourceCheckOutcome(s.Outcome), ObservedAt: s.ObservedAt.Time, Note: s.Note, ActorId: mh.IDString(s.ActorID), Current: r.Current}
		}
		out.Items = append(out.Items, item)
	}
	return c.JSON(out)
}
func (h *Handler) ListAudit(c fiber.Ctx, p generated.ListAuditParams) error {
	actor, e := h.actor(c)
	if e != nil {
		return moderationError(c, e)
	}
	resource, e := mh.OptionalID(p.ResourceId)
	if e != nil {
		return moderationError(c, e)
	}
	user, e := mh.OptionalID(p.UserId)
	if e != nil {
		return moderationError(c, e)
	}
	page, size := mh.Pagination(p.Page, p.PageSize)
	v, e := h.moderation.AuditList(c.Context(), actor, page, size, resource, user, mh.Value(p.Operation))
	if e != nil {
		return moderationError(c, e)
	}
	out := generated.AuditList{Items: []generated.AuditEntry{}, Page: v.Page, PageSize: int64(v.PageSize), HasNext: v.HasNext}
	for _, r := range v.Items {
		out.Items = append(out.Items, auditDTO(r))
	}
	return c.JSON(out)
}
func (h *Handler) GetAudit(c fiber.Ctx, raw string) error {
	actor, e := h.actor(c)
	if e != nil {
		return moderationError(c, e)
	}
	key, e := mh.ID(raw)
	if e != nil {
		return moderationError(c, e)
	}
	v, e := h.moderation.Audit(c.Context(), actor, key)
	if e != nil {
		return moderationError(c, e)
	}
	return c.JSON(auditDTO(v))
}
func auditDTO(r sqlc.AppAuditEntry) generated.AuditEntry {
	out := generated.AuditEntry{BeforeAvailability: mh.Text[generated.AuditEntryBeforeAvailability](r.BeforeAvailability), AfterAvailability: mh.Text[generated.AuditEntryAfterAvailability](r.AfterAvailability), BeforeTaxonomyState: mh.Text[generated.AuditEntryBeforeTaxonomyState](r.BeforeTaxonomyState), AfterTaxonomyState: mh.Text[generated.AuditEntryAfterTaxonomyState](r.AfterTaxonomyState), Id: mh.IDString(r.ID), ActorId: mh.IDString(r.ActorID), Operation: generated.AuditEntryOperation(r.Operation), ResourceId: mh.IDPointer(r.ResourceID), CategoryId: mh.IDPointer(r.CategoryID), TagId: mh.IDPointer(r.TagID), UserId: mh.IDPointer(r.UserID), SourceId: mh.IDPointer(r.SourceID), ContributionId: mh.IDPointer(r.ContributionID), ModerationActionId: mh.IDPointer(r.ModerationActionID), Fields: []generated.AuditEntryFields{}, BeforeVersion: mh.Number(r.BeforeVersion), AfterVersion: mh.Number(r.AfterVersion), BeforePublication: mh.Text[generated.AuditEntryBeforePublication](r.BeforePublication), AfterPublication: mh.Text[generated.AuditEntryAfterPublication](r.AfterPublication), BeforeRights: mh.Text[generated.AuditEntryBeforeRights](r.BeforeRights), AfterRights: mh.Text[generated.AuditEntryAfterRights](r.AfterRights), BeforeDistribution: mh.Text[generated.AuditEntryBeforeDistribution](r.BeforeDistribution), AfterDistribution: mh.Text[generated.AuditEntryAfterDistribution](r.AfterDistribution), BeforeTrust: mh.Text[generated.AuditEntryBeforeTrust](r.BeforeTrust), AfterTrust: mh.Text[generated.AuditEntryAfterTrust](r.AfterTrust), Reason: mh.Text[string](r.Reason), OccurredAt: r.OccurredAt.Time}
	for _, f := range r.Fields {
		out.Fields = append(out.Fields, generated.AuditEntryFields(f))
	}
	return out
}
