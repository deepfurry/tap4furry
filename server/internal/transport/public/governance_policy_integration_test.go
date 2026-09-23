package public

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/contribution"
	"github.com/deepfurry/tap4furry/server/internal/curation"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/deepfurry/tap4furry/server/internal/governance"
	"github.com/deepfurry/tap4furry/server/internal/identity"
	"github.com/deepfurry/tap4furry/server/internal/moderation"
	"github.com/deepfurry/tap4furry/server/internal/resource"
	"github.com/jackc/pgx/v5"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"
)

func (f *governanceFixture) published() (curation.Revision, uuid.UUID) {
	f.t.Helper()
	ctx := f.t.Context()
	r := f.create()
	active := resource.Active
	next, err := f.curation.PatchResource(ctx, f.actor, r.ID, r.Version, curation.CorePatch{Lifecycle: &active})
	r = mustCuration(f.t, next, err)
	next, err = f.curation.CreateSource(ctx, f.actor, r.ID, r.Version, curation.SourceInput{URL: "https://example.invalid/observed", Type: resource.SourceOfficial, Availability: resource.SourceActive})
	r = mustCuration(f.t, next, err)
	sources, err := sqlc.New(f.adminPool).AdminResourceSources(ctx, governance.ID(r.ID))
	if err != nil || len(sources) != 1 {
		f.t.Fatal("fixture source missing")
	}
	sid := uuid.UUID(sources[0].ID.Bytes)
	next, err = f.curation.SetPublication(ctx, f.actor, r.ID, r.Version, resource.Published, "")
	return mustCuration(f.t, next, err), sid
}
func (f *governanceFixture) report(in moderation.ReportInput) uuid.UUID {
	f.t.Helper()
	key, err := f.pubGov.SubmitReport(f.t.Context(), f.author, in)
	if err != nil {
		f.t.Fatal(err)
	}
	f.records.Reports = append(f.records.Reports, key.String())
	return key
}
func reportInput(r uuid.UUID) moderation.ReportInput {
	return moderation.ReportInput{RequestID: uuid.NewV7(), ResourceID: r, TargetKind: "resource", Reason: "other", Body: "Original 🦊 observation"}
}
func (f *governanceFixture) change(in moderation.UserChange) uuid.UUID {
	f.t.Helper()
	key, err := f.adminGov.ChangeUser(f.t.Context(), f.actor, f.author.UserID, in)
	if err != nil {
		f.t.Fatal(err)
	}
	return key
}
func restriction(rev int64) moderation.UserChange {
	return moderation.UserChange{RequestID: uuid.NewV7(), ExpectedRevision: rev, Kind: "restrict", Scope: "all_write", Duration: "24h", ReasonCode: "other", Message: "Business writes paused", InternalNote: "Staff evidence"}
}

func TestIntegrationGovernanceReportPrivacyQuotaAndReplay(t *testing.T) {
	f := newGovernanceFixture(t)
	ctx := t.Context()
	r, sid := f.published()
	in := reportInput(r.ID)
	in.SourceID = uuid.NewV7()
	in.TargetKind = "source"
	if _, err := f.pubGov.SubmitReport(ctx, f.author, in); !errors.Is(err, moderation.ErrNotFound) {
		t.Fatal("foreign source accepted", err)
	}
	in.SourceID = sid
	in.Body = strings.Repeat("界", 4001)
	if _, err := f.pubGov.SubmitReport(ctx, f.author, in); !errors.Is(err, moderation.ErrValidation) {
		t.Fatal("oversized body accepted")
	}
	in.Body = "Original 🦊 observation"
	key := f.report(in)
	if replay, err := f.pubGov.SubmitReport(ctx, f.author, in); err != nil || replay != key {
		t.Fatal("replay failed")
	}
	in.Body = "Changed"
	if _, err := f.pubGov.SubmitReport(ctx, f.author, in); !errors.Is(err, moderation.ErrRequestConflict) {
		t.Fatal("request fingerprint mismatch accepted")
	}
	in.Body = "Original 🦊 observation"
	another := reportInput(r.ID)
	_, err := f.pubGov.SubmitReport(ctx, f.author, another)
	var limit *moderation.LimitError
	if !errors.As(err, &limit) || limit.Reason != "submission_interval" {
		t.Fatal("report interval missing", err)
	}
	f.exec("UPDATE app.reports SET created_at=created_at-interval '61 seconds' WHERE id=$1", key.String())
	duplicate := in
	duplicate.RequestID = uuid.NewV7()
	if _, err = f.pubGov.SubmitReport(ctx, f.author, duplicate); !errors.Is(err, moderation.ErrConflict) {
		t.Fatal("duplicate active target accepted", err)
	}
	for _, kind := range []string{"receive", "note"} {
		if err = f.adminGov.DecideReport(ctx, f.actor, key, moderation.ReportDecision{RequestID: uuid.NewV7(), ExpectedVersion: int64(f.scalar("SELECT version FROM app.reports WHERE id=$1", key.String())), Kind: kind, InternalNote: "Never disclose staff evidence"}); err != nil {
			t.Fatal(err)
		}
	}
	dto, _ := f.request("GET", "/me/reports/"+key.String(), nil, f.authorCookie, 200)
	b, _ := json.Marshal(dto)
	for _, forbidden := range []string{"Never disclose", "internal_note", "actor_id", "version", "priority", "queue", "request_fingerprint", "url", "duplicate_of"} {
		if strings.Contains(string(b), forbidden) {
			t.Fatal("private report field leaked")
		}
	}
	if err = f.adminGov.DecideReport(ctx, f.actor, key, moderation.ReportDecision{RequestID: uuid.NewV7(), ExpectedVersion: 1, Kind: "dismiss", SafeMessage: "Stale"}); !errors.Is(err, moderation.ErrConflict) {
		t.Fatal("stale decision accepted")
	}
	if _, err = f.curation.SetPublication(ctx, f.actor, r.ID, r.Version, resource.Restricted, "Visibility test"); err != nil {
		t.Fatal(err)
	}
	own, err := f.pubGov.OwnReport(ctx, f.author, key)
	if err != nil || own.Target != nil || own.Row.Body != in.Body {
		t.Fatal("hidden target report projection failed", err)
	}
	if _, err = f.pubGov.SubmitReport(ctx, f.author, in); err != nil {
		t.Fatal("replay failed after target hidden")
	}
	withdrawn := uuid.NewV7()
	if err = f.pubGov.WithdrawReport(ctx, f.author, key, withdrawn); err != nil {
		t.Fatal(err)
	}
	if err = f.pubGov.WithdrawReport(ctx, f.author, key, withdrawn); err != nil {
		t.Fatal("withdraw replay failed")
	}
	if err = f.adminGov.DecideReport(ctx, f.actor, key, moderation.ReportDecision{RequestID: uuid.NewV7(), ExpectedVersion: 4, Kind: "dismiss", SafeMessage: "Too late"}); !errors.Is(err, moderation.ErrConflict) {
		t.Fatal("terminal report reopened")
	}
}

func TestIntegrationGovernanceTrustRestrictionsAndProtectedWrites(t *testing.T) {
	f := newGovernanceFixture(t)
	ctx := t.Context()
	proposal := f.input()
	prior := f.submit(proposal)
	initial, err := f.adminGov.UserGovernance(ctx, f.actor, f.author.UserID)
	if err != nil || initial.Trust != "new" || initial.Revision != 0 {
		t.Fatal("default governance profile wrong")
	}
	noop := moderation.UserChange{RequestID: uuid.NewV7(), Kind: "trust", Trust: "new", Reason: "Already default"}
	f.change(noop)
	if f.scalar("SELECT count(*) FROM app.user_governance_profiles WHERE user_id=$1", f.author.UserID.String()) != 0 {
		t.Fatal("no-op created profile")
	}
	rev := int64(0)
	for _, level := range []string{"established", "trusted", "new"} {
		b, _ := governance.ContributionBudget(level)
		in := moderation.UserChange{RequestID: uuid.NewV7(), Kind: "trust", ExpectedRevision: rev, Trust: level, Reason: "Reviewed history"}
		key := f.change(in)
		if f.change(in) != key {
			t.Fatal("trust replay mismatch")
		}
		rev++
		own, e := f.pubGov.OwnGovernance(ctx, f.author)
		if e != nil || own.ContributionQuota.DailyLimit != b.Daily || own.ContributionQuota.PendingLimit != b.Pending || own.ContributionQuota.IntervalSeconds != int64(b.Interval/time.Second) || own.ReportQuota.DailyLimit != 10 {
			t.Fatal("quota tier mismatch", e)
		}
		list, e := f.publicReview.OwnList(ctx, f.author, 1, 20, "")
		if e != nil || list.Limits.PendingLimit != b.Pending {
			t.Fatal("contribution quota display stale", e)
		}
	}
	if _, err = f.adminGov.ChangeUser(ctx, f.actor, f.actor.UserID, restriction(0)); !errors.Is(err, moderation.ErrForbidden) {
		t.Fatal("self restriction allowed")
	}
	in := restriction(rev)
	f.change(in)
	rev++
	original, err := f.identity.Me(ctx, f.author.UserID)
	if err != nil {
		t.Fatal(err)
	}
	mixed := identity.ProfileUpdate{DisplayName: identity.Field[string]{Set: true, Value: cp("Blocked")}, SearchEngineIndexing: cp(false)}
	_, err = f.pubGov.UpdateProfile(ctx, f.author, mixed)
	var blocked *governance.RestrictedError
	if !errors.As(err, &blocked) {
		t.Fatal("restricted profile accepted", err)
	}
	after, _ := f.identity.Me(ctx, f.author.UserID)
	if !reflect.DeepEqual(original, after) {
		t.Fatal("mixed privacy request partially applied")
	}
	if _, err = f.pubGov.UpdateProfile(ctx, f.author, identity.ProfileUpdate{SearchEngineIndexing: cp(false)}); err != nil {
		t.Fatal("privacy opt-out blocked", err)
	}
	if replay, err := f.publicReview.Submit(ctx, f.author, proposal); err != nil || replay != prior {
		t.Fatal("restricted replay blocked", err)
	}

	for _, kind := range []string{contribution.Create, contribution.Update, contribution.AddSource, contribution.RemoveSource, contribution.AddTag, contribution.AddRelation, contribution.AddTranslation} {
		next := f.input()
		next.Kind = kind
		if kind != contribution.Create {
			next.TargetID = uuid.NewV7()
			next.BaseRevision = "opaque"
		}
		if contribution.Extended(kind) {
			next.Content = contribution.Patch{}
			switch kind {
			case contribution.AddSource:
				next.Change = &contribution.Change{Source: &contribution.Source{URL: "https://example.invalid/new"}}
			case contribution.RemoveSource:
				next.Change = &contribution.Change{SourceID: uuid.NewV7()}
			case contribution.AddTag:
				next.Change = &contribution.Change{Tags: []uuid.UUID{uuid.NewV7()}}
			case contribution.AddRelation:
				next.Change = &contribution.Change{Relation: &contribution.RelationChange{OtherID: uuid.NewV7(), Type: resource.RelatedTo, Direction: "symmetric"}}
			case contribution.AddTranslation:
				next.Change = &contribution.Change{Translation: &contribution.TranslationChange{Locale: "ja", Name: cp("Name")}}
			}
		}
		_, e := f.publicReview.Submit(ctx, f.author, next)
		if !errors.As(e, &blocked) {
			t.Fatal("contribution kind bypassed restriction", kind, e)
		}
	}
	if err = f.publicReview.Withdraw(ctx, f.author, prior); err != nil {
		t.Fatal("restricted withdrawal blocked", err)
	}
	view, _ := f.adminGov.UserGovernance(ctx, f.actor, f.author.UserID)
	old := uuid.UUID(view.Restrictions[0].ID.Bytes)
	replacement := restriction(rev)
	replacement.Duration = "7d"
	replacement.ReplacesID = old
	f.change(replacement)
	rev++
	view, _ = f.adminGov.UserGovernance(ctx, f.actor, f.author.UserID)
	own, _ := f.pubGov.OwnGovernance(ctx, f.author)
	if len(view.Restrictions) != 2 || len(own.Restrictions) != 1 {
		t.Fatal("restriction replacement lost history or overlapped")
	}
	current := uuid.UUID(own.Restrictions[0].ID.Bytes)
	f.change(moderation.UserChange{Kind: "revoke_restriction", RequestID: uuid.NewV7(), ExpectedRevision: rev, RestrictionID: current, Reason: "Restriction ended"})
	rev++
	if _, err = f.pubGov.UpdateProfile(ctx, f.author, mixed); err != nil {
		t.Fatal("revoke did not restore business write", err)
	}
	f.change(restriction(rev))
	f.exec("UPDATE app.user_restrictions SET starts_at=clock_timestamp()-interval '2 days',expires_at=clock_timestamp()-interval '1 day' WHERE user_id=$1 AND revoked_at IS NULL", f.author.UserID.String())
	if _, err = f.pubGov.UpdateProfile(ctx, f.author, mixed); err != nil {
		t.Fatal("expired restriction remained effective", err)
	}
	if f.scalar("SELECT count(*) FROM app.user_restrictions WHERE user_id=$1 AND expires_at<clock_timestamp() AND revoked_at IS NULL", f.author.UserID.String()) != 1 {
		t.Fatal("expiry rewrote immutable history")
	}
}

func TestIntegrationGovernanceSourceRevisionAndEligibility(t *testing.T) {
	f := newGovernanceFixture(t)
	ctx := t.Context()
	r, sid := f.published()
	q := sqlc.New(f.api)
	eligible := func(want bool) {
		t.Helper()
		got, e := q.GovernanceRecommendationEligible(ctx, governance.ID(r.ID))
		if e != nil || got != want {
			t.Fatal("SQL eligibility disagrees with policy", e)
		}
	}
	eligible(false) // Unknown rights remain displayable, never recommended.
	next, err := f.curation.SetSourceRights(ctx, f.actor, r.ID, r.Version, sid, resource.RightsConfirmed, "Rights established")
	r = mustCuration(t, next, err)
	eligible(true)
	check := moderation.SourceCheckInput{RequestID: uuid.NewV7(), ExpectedVersion: r.Version, Outcome: "reachable", Note: "Manual observation", ObservedAt: time.Now().Add(-time.Minute)}
	id, err := f.adminGov.RecordSourceCheck(ctx, f.actor, r.ID, sid, check)
	if err != nil {
		t.Fatal(err)
	}
	if again, e := f.adminGov.RecordSourceCheck(ctx, f.actor, r.ID, sid, check); e != nil || again != id {
		t.Fatal("observation replay duplicated")
	}
	if f.revision(r.ID).Version != r.Version {
		t.Fatal("observation bumped Resource")
	}
	check.RequestID = uuid.NewV7()
	check.ExpectedVersion--
	if _, err = f.adminGov.RecordSourceCheck(ctx, f.actor, r.ID, sid, check); !errors.Is(err, resource.ErrVersionConflict) {
		t.Fatal("stale observation accepted")
	}
	next, err = f.curation.PatchSource(ctx, f.actor, r.ID, r.Version, sid, curation.SourcePatch{URL: cp("https://example.invalid/new-address")})
	r = mustCuration(t, next, err)
	health, err := f.adminGov.SourceHealth(ctx, f.actor, 1, 10, "", nil, r.ID, sid)
	if err != nil || len(health.Items) != 1 || health.Items[0].Current || health.Items[0].Check == nil {
		t.Fatal("old URL observation shown as current", err)
	}
	in := moderation.DistributionInput{RequestID: uuid.NewV7(), ExpectedVersion: r.Version, Policy: "excluded", Reason: "Editorial recommendation exclusion"}
	if _, err = f.adminGov.SetDistribution(ctx, f.actor, r.ID, in); err != nil {
		t.Fatal(err)
	}
	r.Version++
	eligible(false)
	in.RequestID = uuid.NewV7()
	in.ExpectedVersion = r.Version
	before := f.revision(r.ID)
	n := f.scalar("SELECT count(*) FROM app.audit_entries WHERE resource_id=$1", r.ID.String())
	if _, err = f.adminGov.SetDistribution(ctx, f.actor, r.ID, in); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, f.revision(r.ID)) || n != f.scalar("SELECT count(*) FROM app.audit_entries WHERE resource_id=$1", r.ID.String()) {
		t.Fatal("distribution no-op manufactured audit")
	}
	in.RequestID = uuid.NewV7()
	in.Policy = "normal"
	if _, err = f.adminGov.SetDistribution(ctx, f.actor, r.ID, in); err != nil {
		t.Fatal(err)
	}
	r.Version++
	eligible(true)
	// The pure policy and SQL agree for each independently disqualifying state.
	for _, tc := range []struct {
		sql, undo string
		e         governance.RecommendationEligibility
	}{
		{"UPDATE app.resources SET content_rating='explicit' WHERE id=$1", "UPDATE app.resources SET content_rating='general' WHERE id=$1", governance.RecommendationEligibility{Public: true, Canonical: true, Active: true, CategoryActive: true, ConfirmedActiveSource: true}},
		{"UPDATE app.resources SET lifecycle='inactive' WHERE id=$1", "UPDATE app.resources SET lifecycle='active' WHERE id=$1", governance.RecommendationEligibility{Public: true, Canonical: true, General: true, CategoryActive: true, ConfirmedActiveSource: true}},
		{"UPDATE app.resources SET publication_state='restricted' WHERE id=$1", "UPDATE app.resources SET publication_state='published' WHERE id=$1", governance.RecommendationEligibility{Canonical: true, General: true, Active: true, CategoryActive: true, ConfirmedActiveSource: true}},
	} {
		f.exec(tc.sql, r.ID.String())
		eligible(tc.e.Eligible())
		f.exec(tc.undo, r.ID.String())
	}
	f.exec("DELETE FROM app.resource_localizations WHERE resource_id=$1", r.ID.String())
	eligible(false)
	if _, err = f.pubGov.SubmitReport(ctx, f.author, reportInput(r.ID)); !errors.Is(err, moderation.ErrCanonical) {
		t.Fatal("canonical corruption did not fail closed", err)
	}
}

func waitForGovernanceBlock(t *testing.T, f *governanceFixture, tx pgx.Tx, role string, done <-chan error) {
	t.Helper()
	until := time.Now().Add(4 * time.Second)
	for time.Now().Before(until) {
		var n int
		if err := f.owner.QueryRow(t.Context(), "SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND usename=$1 AND $2::integer=ANY(pg_blocking_pids(pid))", role, int32(tx.Conn().PgConn().PID())).Scan(&n); err != nil {
			t.Fatal("lock observer failed")
		}
		if n > 0 {
			return
		}
		select {
		case <-done:
			t.Fatal("operation escaped lock")
		case <-time.After(10 * time.Millisecond):
		}
	}
	t.Fatal("expected transaction lock wait not observed")
}
func TestIntegrationGovernanceAuthorizationRaces(t *testing.T) {
	for _, mode := range []string{"profile", "private_report", "staff_decision"} {
		t.Run(mode, func(t *testing.T) {
			f := newGovernanceFixture(t)
			ctx := t.Context()
			r, _ := f.published()
			report := f.report(reportInput(r.ID))
			user := f.author.UserID
			role := "gfp_api"
			if mode == "staff_decision" {
				user = f.actor.UserID
				role = "gfp_admin"
			}
			tx, err := f.owner.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if _, err = tx.Exec(ctx, "SELECT id FROM app.users WHERE id=$1 FOR UPDATE", user.String()); err != nil {
				t.Fatal("lock actor failed")
			}
			done := make(chan error, 1)
			go func() {
				var e error
				switch mode {
				case "profile":
					_, e = f.pubGov.UpdateProfile(ctx, f.author, identity.ProfileUpdate{DisplayName: identity.Field[string]{Set: true, Value: cp("Forbidden race")}})
				case "private_report":
					_, e = f.pubGov.OwnReport(ctx, f.author, report)
				case "staff_decision":
					e = f.adminGov.DecideReport(ctx, f.actor, report, moderation.ReportDecision{RequestID: uuid.NewV7(), ExpectedVersion: 1, Kind: "dismiss", SafeMessage: "Must not commit"})
				}
				done <- e
			}()
			waitForGovernanceBlock(t, f, tx, role, done)
			if mode == "staff_decision" {
				_, err = tx.Exec(ctx, "DELETE FROM app.user_roles WHERE user_id=$1 AND role='admin'", user.String())
			} else {
				_, err = tx.Exec(ctx, "UPDATE app.sessions SET revoked_at=clock_timestamp() WHERE id=$1", f.author.SessionID.String())
			}
			if err != nil {
				t.Fatal("revocation failed")
			}
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			want := auth.ErrUnauthenticated
			if mode == "staff_decision" {
				want = auth.ErrAdminForbidden
			}
			if err = <-done; !errors.Is(err, want) {
				t.Fatal("operation outlived current authorization", err)
			}
			if f.scalar("SELECT version FROM app.reports WHERE id=$1", report.String()) != 1 {
				t.Fatal("revoked actor changed report")
			}
		})
	}
}

func TestIntegrationGovernanceCompositeRollbackAndTerminalRace(t *testing.T) {
	f := newGovernanceFixture(t)
	ctx := t.Context()
	r, _ := f.published()
	report := f.report(reportInput(r.ID))
	in := moderation.ReportDecision{RequestID: uuid.NewV7(), ExpectedVersion: 1, ResourceVersion: r.Version - 1, Kind: "resolve", Mode: "publication", State: "restricted", Reason: "Explicit policy hold", SafeMessage: "Resource display restricted"}
	if err := f.adminGov.DecideReport(ctx, f.actor, report, in); !errors.Is(err, resource.ErrVersionConflict) {
		t.Fatal("stale canonical version accepted", err)
	}
	if f.scalar("SELECT version FROM app.reports WHERE id=$1", report.String()) != 1 || f.revision(r.ID).Version != r.Version {
		t.Fatal("stale composite partially committed")
	}
	in.ResourceVersion = r.Version
	tx, err := f.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "LOCK TABLE app.audit_entries IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal("audit failure fixture failed")
	}
	deadline, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- f.adminGov.DecideReport(deadline, f.actor, report, in) }()
	waitForGovernanceBlock(t, f, tx, "gfp_admin", done)
	cancel()
	if err = <-done; err == nil {
		t.Fatal("cancelled audit committed")
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if f.revision(r.ID).Version != r.Version || f.scalar("SELECT version FROM app.reports WHERE id=$1", report.String()) != 1 || f.scalar("SELECT count(*) FROM app.moderation_actions WHERE report_id=$1", report.String()) != 0 {
		t.Fatal("audit failure escaped rollback")
	}
	// Two reviewers/author operations contend on the same report; one terminal wins.
	var results [2]error
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() { <-start; results[0] = f.adminGov.DecideReport(ctx, f.actor, report, in) })
	wg.Go(func() { <-start; results[1] = f.pubGov.WithdrawReport(ctx, f.author, report, uuid.NewV7()) })
	close(start)
	wg.Wait()
	wins := 0
	for _, e := range results {
		if e == nil {
			wins++
		} else if !errors.Is(e, moderation.ErrConflict) {
			t.Fatal("unexpected terminal race error", e)
		}
	}
	if wins != 1 || f.scalar("SELECT count(*) FROM app.report_events WHERE report_id=$1 AND event_type IN ('resolved','dismissed','withdrawn')", report.String()) != 1 {
		t.Fatal("multiple terminal results")
	}
}
