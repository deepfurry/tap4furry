package public

import (
	"context"
	"errors"
	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/config"
	"github.com/deepfurry/tap4furry/server/internal/contribution"
	"github.com/deepfurry/tap4furry/server/internal/curation"
	"github.com/deepfurry/tap4furry/server/internal/database/contributioncheck"
	"github.com/deepfurry/tap4furry/server/internal/resource"
	"github.com/deepfurry/tap4furry/server/internal/taxonomy"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
	"uuid"
)

type contributionFixture struct {
	*curationFixture
	publicReview, adminReview *contribution.App
	proposals                 *contributioncheck.Fixture
	author                    auth.Actor
	authorCookie              *http.Cookie
}

func newContributionFixture(t *testing.T) *contributionFixture {
	t.Helper()
	if os.Getenv("GFP_CONTRIBUTION_INTEGRATION") != "1" {
		t.Skip("explicit disposable contribution integration not enabled")
	}
	t.Setenv("GFP_CURATION_INTEGRATION", "1")
	f := newCurationFixture(t)
	_, _, cookie := f.register()
	f.request("POST", "/auth/email/verification", map[string]string{"token": f.lastToken("email_verify")}, nil, 204)
	actor, err := f.auth.Resolve(t.Context(), cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	proposals := &contributioncheck.Fixture{}
	t.Cleanup(func() {
		if err := proposals.Cleanup(f.owner); err != nil {
			t.Error(err)
		}
	})
	return &contributionFixture{f, contribution.New(f.api, config.DevelopmentCSRFSecret), contribution.New(f.adminPool, ""), proposals, actor, cookie}
}
func cp[T any](value T) *T { return &value }
func (f *contributionFixture) input() contribution.SubmitInput {
	return contribution.SubmitInput{Kind: contribution.Create, RequestID: uuid.NewV7(), Reason: "Fixture evidence", Content: contribution.Patch{Name: cp("Original"), DefaultLocale: cp("en"), CategoryID: &f.category, ContentRating: cp(resource.General), Summary: contribution.NullableText{Set: true, Value: cp("Summary")}}}
}
func (f *contributionFixture) submit(in contribution.SubmitInput) uuid.UUID {
	f.t.Helper()
	id, err := f.publicReview.Submit(f.t.Context(), f.author, in)
	if err != nil {
		f.t.Fatal(err)
	}
	f.proposals.IDs = append(f.proposals.IDs, id.String())
	return id
}
func (f *contributionFixture) acceptInput(id uuid.UUID) contribution.AcceptInput {
	f.t.Helper()
	detail, err := f.adminReview.ReviewDetail(f.t.Context(), f.actor, id)
	if err != nil {
		f.t.Fatal(err)
	}
	content := detail.Proposed
	if detail.Kind == contribution.Create {
		content.Slug = "fixture-" + id.String()
	}
	return contribution.AcceptInput{Content: content}
}
func (f *contributionFixture) acceptedResource(id uuid.UUID) uuid.UUID {
	f.t.Helper()
	detail, err := f.adminReview.ReviewDetail(f.t.Context(), f.actor, id)
	if err != nil {
		f.t.Fatal(err)
	}
	f.graph.Resources = append(f.graph.Resources, detail.ResultID.String())
	return detail.ResultID
}
func (f *contributionFixture) cooldown() {
	f.exec("UPDATE app.contributions SET created_at=created_at-interval '61 seconds' WHERE author_id=$1", f.author.UserID.String())
}
func TestIntegrationContributionHTTPAndLifecycle(t *testing.T) {
	f := newContributionFixture(t)
	_, _, second := f.register()
	f.request("POST", "/auth/email/verification", map[string]string{"token": f.lastToken("email_verify")}, nil, 204)
	if err := contributioncheck.Run(t.Context(), f.app, f.adminApp, f.owner, [2]*http.Cookie{f.authorCookie, second}, f.email, testPassword, testOrigin, adminOrigin); err != nil {
		t.Fatal(err)
	}
}
func TestIntegrationContributionSubmissionIsolationAndQuotas(t *testing.T) {
	f := newContributionFixture(t)
	ctx := t.Context()
	in := f.input()
	id := f.submit(in)
	replay, err := f.publicReview.Submit(ctx, f.author, in)
	if err != nil || replay != id {
		t.Fatal("replay not idempotent")
	}
	changed := in
	changed.Reason = "different"
	if _, err = f.publicReview.Submit(ctx, f.author, changed); !errors.Is(err, contribution.ErrRequestConflict) {
		t.Fatal("reused key body accepted")
	}
	var limited *contribution.LimitError
	if _, err = f.publicReview.Submit(ctx, f.author, f.input()); !errors.As(err, &limited) || limited.Reason != "submission_interval" {
		t.Fatal("cooldown absent")
	}
	_, _, other := f.register()
	f.request("GET", "/me/contributions/"+id.String(), nil, other, 404)
	f.request("POST", "/contributions", map[string]any{"kind": "create_resource", "request_id": uuid.NewV7().String(), "reason": "unverified", "content": map[string]any{"default_locale": "en", "name": "A", "category_id": f.category.String(), "summary": "B", "content_rating": "general"}}, other, 403)
	f.request("GET", "/me/contributions", nil, nil, 401)
	for range 4 {
		f.cooldown()
		f.submit(f.input())
	}
	f.cooldown()
	if _, err = f.publicReview.Submit(ctx, f.author, f.input()); !errors.As(err, &limited) || limited.Reason != "pending_limit" {
		t.Fatal("pending quota absent")
	}
	for _, key := range f.proposals.IDs {
		u, _ := uuid.Parse(key)
		if err = f.publicReview.Withdraw(ctx, f.author, u); err != nil {
			t.Fatal(err)
		}
	}
	for range 5 {
		f.cooldown()
		key := f.submit(f.input())
		if err = f.publicReview.Withdraw(ctx, f.author, key); err != nil {
			t.Fatal(err)
		}
	}
	f.cooldown()
	if _, err = f.publicReview.Submit(ctx, f.author, f.input()); !errors.As(err, &limited) || limited.Reason != "daily_limit" {
		t.Fatal("rolling daily quota absent")
	}
	list, err := f.publicReview.OwnList(ctx, f.author, 1, 3, "")
	if err != nil || len(list.Items) != 3 || !list.HasNext || list.Limits.Remaining24h != 0 {
		t.Fatal("pagination/quota view wrong")
	}
	if err = f.publicReview.Withdraw(ctx, f.author, id); !errors.Is(err, contribution.ErrConflict) {
		t.Fatal("withdraw repeated")
	}
}
func TestIntegrationContributionReviewSafetyAndRollback(t *testing.T) {
	f := newContributionFixture(t)
	ctx := t.Context()
	id := f.submit(f.input())
	in := f.acceptInput(id)
	_, modEmail, _ := f.eligible(auth.Moderator)
	_, modCookie := f.adminLogin(modEmail)
	f.adminRequest("GET", "/contributions", nil, modCookie, 403)
	f.adminRequest("POST", "/contributions/"+id.String()+"/reject", map[string]any{"message": "Valid rejection"}, modCookie, 403)
	f.adminRequest("GET", "/contributions", nil, f.authorCookie, 401)
	if _, err := f.operator.Grant(ctx, modEmail, auth.Editor); err != nil {
		t.Fatal(err)
	}
	// Self-review cannot bypass the transaction by constructing a public actor.
	self := f.actor
	self.UserID = f.author.UserID
	if err := f.adminReview.Accept(ctx, self, id, in); err == nil {
		t.Fatal("forged reviewer accepted")
	}
	in.Content.Name = "Reviewer revision"
	if err := f.adminReview.Accept(ctx, f.actor, id, in); !errors.Is(err, contribution.ErrValidation) {
		t.Fatal("unexplained revision accepted")
	}
	in.Message = cp("Corrected name")
	in.InternalNote = cp("Private reviewer note")
	// Fail after canonical writes and accepted snapshot but before commit.
	f.exec("ALTER TABLE app.contribution_review_audits ADD CONSTRAINT contribution_fixture_failure CHECK (contribution_id <> '" + id.String() + "'::uuid)")
	t.Cleanup(func() {
		if _, err := f.owner.Exec(context.Background(), "ALTER TABLE app.contribution_review_audits DROP CONSTRAINT IF EXISTS contribution_fixture_failure"); err != nil {
			t.Error("remove injected constraint failed")
		}
	})
	if err := f.adminReview.Accept(ctx, f.actor, id, in); err == nil {
		t.Fatal("injected audit failure accepted")
	}
	if f.scalar("SELECT count(*) FROM app.resources WHERE slug=$1", in.Content.Slug) != 0 || f.scalar("SELECT count(*) FROM app.contribution_contents WHERE contribution_id=$1 AND content_kind='accepted'", id.String()) != 0 || f.scalar("SELECT count(*) FROM app.contribution_events WHERE contribution_id=$1", id.String()) != 1 {
		t.Fatal("failed review escaped rollback")
	}
	f.exec("ALTER TABLE app.contribution_review_audits DROP CONSTRAINT contribution_fixture_failure")
	if err := f.adminReview.Accept(ctx, f.actor, id, in); err != nil {
		t.Fatal(err)
	}
	rid := f.acceptedResource(id)
	if f.revision(rid).Version != 1 || f.revision(rid).PublicationState != "draft" {
		t.Fatal("new review must create version-one draft")
	}
	own, _ := f.publicReview.OwnDetail(ctx, f.author, id)
	if own.Proposed.Name != "Original" || own.Accepted != nil || own.Result != nil {
		t.Fatal("original overwritten or draft leaked")
	}
	if err := f.adminReview.Reject(ctx, f.actor, id, "rejected", nil); !errors.Is(err, contribution.ErrConflict) {
		t.Fatal("terminal decision changed")
	}
	f.cooldown()
	id2 := f.submit(f.input())
	in2 := f.acceptInput(id2)
	in2.Content.Slug = in.Content.Slug
	if err := f.adminReview.Accept(ctx, f.actor, id2, in2); !errors.Is(err, contribution.ErrConflict) && !errors.Is(err, curation.ErrConflict) {
		t.Fatal("slug conflict not atomic")
	}
	if err := f.adminReview.Reject(ctx, f.actor, id2, "Not suitable", cp("Internal")); err != nil {
		t.Fatal(err)
	}
	body, _ := f.request("GET", "/me/contributions/"+id2.String(), nil, f.authorCookie, 200)
	assertKeys(t, body, []string{"id", "kind", "status", "reason", "created_at", "decided_at", "proposed", "history"})
	for _, entry := range body["history"].([]any) {
		assertKeys(t, entry.(map[string]any), []string{"event_type", "message", "occurred_at"})
	}
}
func TestIntegrationContributionEditCASAndPrivacy(t *testing.T) {
	f := newContributionFixture(t)
	ctx := t.Context()
	r := f.create()
	r, err := f.curation.SetPublication(ctx, f.actor, r.ID, r.Version, resource.Published)
	if err != nil {
		t.Fatal(err)
	}
	slug := f.revision(r.ID).Slug
	edit, err := f.publicReview.Context(ctx, f.author, slug)
	if err != nil {
		t.Fatal(err)
	}
	input := contribution.SubmitInput{Kind: contribution.Update, RequestID: uuid.NewV7(), TargetID: r.ID, BaseRevision: edit.BaseRevision, Reason: "Correction", Content: contribution.Patch{Name: cp("Corrected")}}
	id := f.submit(input)
	in := f.acceptInput(id)
	if err = f.adminReview.Accept(ctx, f.actor, id, in); err != nil {
		t.Fatal(err)
	}
	if f.revision(r.ID).Version != r.Version+1 || f.revision(r.ID).PublicationState != "published" {
		t.Fatal("correction revision/publication invalid")
	}
	own, err := f.publicReview.OwnDetail(ctx, f.author, id)
	if err != nil || own.Accepted == nil || own.Accepted.Name != "Corrected" {
		t.Fatal("current accepted snapshot unavailable")
	}
	f.cooldown()
	input.RequestID = uuid.NewV7()
	if _, err = f.publicReview.Submit(ctx, f.author, input); !errors.Is(err, resource.ErrVersionConflict) {
		t.Fatal("old context accepted")
	}
	edit, err = f.publicReview.Context(ctx, f.author, slug)
	if err != nil {
		t.Fatal(err)
	}
	input.BaseRevision = edit.BaseRevision
	input.Content.Name = cp("Another")
	id2 := f.submit(input)
	in2 := f.acceptInput(id2)
	lifecycle := resource.Active
	r, err = f.curation.PatchResource(ctx, f.actor, r.ID, f.revision(r.ID).Version, curation.CorePatch{Lifecycle: &lifecycle})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.adminReview.Accept(ctx, f.actor, id2, in2); !errors.Is(err, resource.ErrVersionConflict) {
		t.Fatal("stale pending accepted")
	}
	if f.scalar("SELECT count(*) FROM app.contribution_contents WHERE contribution_id=$1 AND content_kind='accepted'", id2.String()) != 0 {
		t.Fatal("stale snapshot persisted")
	}
	own, err = f.publicReview.OwnDetail(ctx, f.author, id)
	if err != nil || own.Accepted != nil {
		t.Fatal("historical accepted snapshot resurrected")
	}
	if _, err = f.operator.Grant(ctx, f.email, auth.Administrator); err != nil {
		t.Fatal(err)
	}
	if _, err = f.curation.SetPublication(ctx, f.actor, r.ID, r.Version, resource.Restricted); err != nil {
		t.Fatal(err)
	}
	own, err = f.publicReview.OwnDetail(ctx, f.author, id)
	if err != nil || own.Result != nil || own.Accepted != nil || own.Proposed.Name != "Corrected" {
		t.Fatal("hidden Resource leaked or own content disappeared")
	}
	f.request("GET", "/contributions/context/"+slug, nil, f.authorCookie, 404)
}
func TestIntegrationContributionConcurrentDecisionsAndRevoke(t *testing.T) {
	f := newContributionFixture(t)
	ctx := t.Context()
	id := f.submit(f.input())
	in := f.acceptInput(id)
	_, email, _ := f.eligible(auth.Editor)
	_, cookie := f.adminLogin(email)
	actor2, err := f.adminAuth.ResolveAdmin(ctx, cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 2)
	go func() { done <- f.adminReview.Accept(ctx, f.actor, id, in) }()
	go func() { done <- f.adminReview.Accept(ctx, actor2, id, in) }()
	success := 0
	for range 2 {
		err := <-done
		if err == nil {
			success++
		} else if !errors.Is(err, contribution.ErrConflict) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatal("duplicate concurrent decisions")
	}
	f.acceptedResource(id)
	f.cooldown()
	id2 := f.submit(f.input())
	in2 := f.acceptInput(id2)
	go func() { done <- f.adminReview.Accept(ctx, f.actor, id2, in2) }()
	go func() { done <- f.publicReview.Withdraw(ctx, f.author, id2) }()
	success = 0
	for range 2 {
		err := <-done
		if err == nil {
			success++
		} else if !errors.Is(err, contribution.ErrConflict) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatal("withdraw/review race not serialized")
	}
	detail, err := f.adminReview.ReviewDetail(ctx, f.actor, id2)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Status == "accepted" {
		f.acceptedResource(id2)
	}
	f.cooldown()
	id3 := f.submit(f.input())
	in3 := f.acceptInput(id3)
	pause := f.gate.arm(t)
	go func() { done <- f.adminReview.Accept(ctx, f.actor, id3, in3) }()
	<-pause.entered
	if _, err = f.operator.Revoke(ctx, f.email, auth.Editor); err != nil {
		t.Fatal(err)
	}
	close(pause.resume)
	if err = <-done; !errors.Is(err, auth.ErrAdminUnauthenticated) && !errors.Is(err, auth.ErrAdminForbidden) {
		t.Fatal("revoked reviewer mutated")
	}
	if f.scalar("SELECT count(*) FROM app.contribution_review_audits WHERE contribution_id=$1", id3.String()) != 0 {
		t.Fatal("revoked review audit escaped")
	}
}
func TestIntegrationContributionGrantMatrix(t *testing.T) {
	f := newContributionFixture(t)
	ctx := t.Context()
	if err := contributioncheck.VerifyGrants(ctx, f.owner); err != nil {
		t.Fatal(err)
	}
	tables := []string{"contributions", "contribution_contents", "contribution_initial_sources", "contribution_events", "contribution_review_audits"}
	for _, table := range tables {
		for _, role := range []string{"gfp_api", "gfp_admin", "gfp_worker", "gfp_readonly"} {
			var bad bool
			query := "SELECT has_table_privilege($1,$2,'DELETE,TRUNCATE,TRIGGER,REFERENCES')"
			if role == "gfp_worker" {
				query = "SELECT has_any_column_privilege($1,$2,'SELECT,INSERT,UPDATE,REFERENCES') OR has_table_privilege($1,$2,'DELETE,TRUNCATE,TRIGGER')"
			}
			if role == "gfp_readonly" {
				query = "SELECT has_any_column_privilege($1,$2,'INSERT,UPDATE,REFERENCES') OR has_table_privilege($1,$2,'DELETE,TRUNCATE,TRIGGER')"
			}
			if err := f.owner.QueryRow(ctx, query, role, "app."+table).Scan(&bad); err != nil || bad {
				t.Fatal("excess contribution table grants")
			}
		}
	}
	for _, role := range []string{"gfp_api", "gfp_admin"} {
		for _, table := range []string{"contribution_contents", "contribution_initial_sources", "contribution_events", "contribution_review_audits"} {
			var bad bool
			if err := f.owner.QueryRow(ctx, "SELECT has_any_column_privilege($1,$2,'UPDATE')", role, "app."+table).Scan(&bad); err != nil || bad {
				t.Fatal("immutable history granted UPDATE")
			}
		}
	}
	var bad bool
	if err := f.owner.QueryRow(ctx, "SELECT has_column_privilege('gfp_api','app.contribution_events','internal_note','SELECT') OR has_any_column_privilege('gfp_api','app.resources','INSERT,UPDATE')").Scan(&bad); err != nil || bad {
		t.Fatal("Public private note/canonical write leak")
	}
	for _, raw := range []string{"javascript:bad", strings.Repeat("x", 2049)} {
		in := f.input()
		in.Content.SourceSet = true
		in.Content.Source = &contribution.Source{URL: raw, Type: "unknown", Availability: "active"}
		if _, err := f.publicReview.Submit(ctx, f.author, in); !errors.Is(err, contribution.ErrValidation) {
			t.Fatal("invalid source URL accepted")
		}
	}
}

func TestIntegrationContributionAuthorProjectionRetainsOnlySubmittedFields(t *testing.T) {
	f := newContributionFixture(t)
	ctx := t.Context()
	r := f.create()
	r, err := f.curation.SetPublication(ctx, f.actor, r.ID, r.Version, resource.Published)
	if err != nil {
		t.Fatal(err)
	}
	edit, err := f.publicReview.Context(ctx, f.author, f.revision(r.ID).Slug)
	if err != nil {
		t.Fatal(err)
	}
	id := f.submit(contribution.SubmitInput{Kind: contribution.Update, RequestID: uuid.NewV7(), TargetID: r.ID, BaseRevision: edit.BaseRevision, Reason: "Clear field", Content: contribution.Patch{Description: contribution.NullableText{Set: true}, Summary: contribution.NullableText{Set: true, Value: cp("Own summary")}}})
	if _, err = f.operator.Grant(ctx, f.email, auth.Administrator); err != nil {
		t.Fatal(err)
	}
	if _, err = f.curation.SetPublication(ctx, f.actor, r.ID, r.Version, resource.Restricted); err != nil {
		t.Fatal(err)
	}
	body, _ := f.request("GET", "/me/contributions/"+id.String(), nil, f.authorCookie, 200)
	proposed := body["proposed"].(map[string]any)
	assertKeys(t, proposed, []string{"summary", "description"})
	if proposed["description"] != nil || proposed["summary"] != "Own summary" {
		t.Fatal("explicit clear or original lost")
	}
	if _, ok := body["target"]; ok {
		t.Fatal("hidden target leaked")
	}
	list, _ := f.request("GET", "/me/contributions", nil, f.authorCookie, 200)
	if list["items"].([]any)[0].(map[string]any)["name"] != "Resource correction" {
		t.Fatal("list leaked canonical-only title")
	}
}

func TestIntegrationContributionConcurrentQuotaAndRealSelfReview(t *testing.T) {
	f := newContributionFixture(t)
	ctx := t.Context()
	input := f.input()
	type outcome struct {
		id  uuid.UUID
		err error
	}
	done := make(chan outcome, 2)
	for range 2 {
		go func() { id, err := f.publicReview.Submit(ctx, f.author, input); done <- outcome{id, err} }()
	}
	first, second := <-done, <-done
	if first.err != nil || second.err != nil || first.id != second.id {
		t.Fatal("concurrent idempotency failed")
	}
	f.proposals.IDs = append(f.proposals.IDs, first.id.String())
	f.cooldown()
	for range 2 {
		in := f.input()
		go func() { id, err := f.publicReview.Submit(ctx, f.author, in); done <- outcome{id, err} }()
	}
	success := 0
	for range 2 {
		r := <-done
		if r.err == nil {
			success++
			f.proposals.IDs = append(f.proposals.IDs, r.id.String())
		} else {
			var limit *contribution.LimitError
			if !errors.As(r.err, &limit) {
				t.Fatal(r.err)
			}
		}
	}
	if success != 1 {
		t.Fatal("concurrent quota did not serialize")
	}
	var authorEmail string
	if err := f.owner.QueryRow(ctx, "SELECT provider_subject FROM app.auth_identities WHERE user_id=$1 AND provider='email'", f.author.UserID.String()).Scan(&authorEmail); err != nil {
		t.Fatal("fixture email unavailable")
	}
	if _, err := f.operator.Grant(ctx, authorEmail, auth.Editor); err != nil {
		t.Fatal(err)
	}
	_, cookie := f.adminLogin(authorEmail)
	self, err := f.adminAuth.ResolveAdmin(ctx, cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.adminReview.Accept(ctx, self, first.id, f.acceptInput(first.id)); !errors.Is(err, contribution.ErrForbidden) {
		t.Fatal("self acceptance allowed")
	}
	if err = f.adminReview.Reject(ctx, self, first.id, "Self rejection", nil); !errors.Is(err, contribution.ErrForbidden) {
		t.Fatal("self rejection allowed")
	}
}
func TestIntegrationContributionTaxonomyLocaleAndCanonicalCorruption(t *testing.T) {
	f := newContributionFixture(t)
	ctx := t.Context()
	r := f.create()
	r, err := f.curation.SetPublication(ctx, f.actor, r.ID, r.Version, resource.Published)
	if err != nil {
		t.Fatal(err)
	}
	slug := f.revision(r.ID).Slug
	edit, err := f.publicReview.Context(ctx, f.author, slug)
	if err != nil {
		t.Fatal(err)
	}
	in := contribution.SubmitInput{Kind: contribution.Update, RequestID: uuid.NewV7(), TargetID: r.ID, BaseRevision: edit.BaseRevision, Reason: "Correction", Content: contribution.Patch{Name: cp("Corrected")}}
	noop := in
	noop.Content.Name = cp(edit.Content.Name)
	if _, err = f.publicReview.Submit(ctx, f.author, noop); !errors.Is(err, contribution.ErrValidation) {
		t.Fatal("empty edit accepted")
	}
	invalid := in
	invalid.Content.DefaultLocale = cp("ja")
	if _, err = f.publicReview.Submit(ctx, f.author, invalid); !errors.Is(err, contribution.ErrValidation) {
		t.Fatal("locale change proposed")
	}
	if _, err = f.operator.Grant(ctx, f.email, auth.Administrator); err != nil {
		t.Fatal(err)
	}
	retired := taxonomy.Retired
	if err = f.curation.PatchTaxonomy(ctx, f.actor, curation.Category, f.category, curation.TaxonomyPatch{State: &retired}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.publicReview.Submit(ctx, f.author, f.input()); !errors.Is(err, contribution.ErrValidation) {
		t.Fatal("new retired binding accepted")
	}
	id := f.submit(in)
	review := f.acceptInput(id)
	// Corruption is an internal error, not hidden/not-found or ordinary conflict.
	f.exec("DELETE FROM app.resource_localizations WHERE resource_id=$1 AND locale='en'", r.ID.String())
	f.request("GET", "/contributions/context/"+slug, nil, f.authorCookie, 500)
	if err = f.adminReview.Accept(ctx, f.actor, id, review); err == nil || errors.Is(err, contribution.ErrConflict) || errors.Is(err, curation.ErrConflict) {
		t.Fatal("corrupt canonical did not fail internally")
	}
	f.exec("INSERT INTO app.resource_localizations(resource_id,locale,name,created_at,updated_at) VALUES($1,'en','Resource',now(),now())", r.ID.String())
	if err = f.adminReview.Accept(ctx, f.actor, id, review); err != nil {
		t.Fatal("existing retired binding not preserved", err)
	}
	f.cooldown()
	edit, err = f.publicReview.Context(ctx, f.author, slug)
	if err != nil {
		t.Fatal(err)
	}
	in.RequestID = uuid.NewV7()
	in.BaseRevision = edit.BaseRevision
	in.Content.Name = cp("Second")
	id = f.submit(in)
	review = f.acceptInput(id)
	rev := f.revision(r.ID)
	next, err := f.curation.PutLocalization(ctx, f.actor, r.ID, rev.Version, "ja", curation.LocalizationInput{Name: "資料"})
	if err != nil {
		t.Fatal(err)
	}
	locale := "ja"
	if _, err = f.curation.PatchResource(ctx, f.actor, r.ID, next.Version, curation.CorePatch{DefaultLocale: &locale}); err != nil {
		t.Fatal(err)
	}
	if err = f.adminReview.Accept(ctx, f.actor, id, review); !errors.Is(err, resource.ErrVersionConflict) {
		t.Fatal("default language change did not conflict")
	}
}
func TestIntegrationContributionHTTPGuardsAndSessionRevocation(t *testing.T) {
	f := newContributionFixture(t)
	ctx := t.Context()
	id := f.submit(f.input())
	f.adminRequest("POST", "/contributions/"+id.String()+"/reject", map[string]string{"message": "Rejected"}, f.cookie, 403, map[string]string{"X-CSRF-Token": "invalid"})
	f.adminRequest("POST", "/contributions/"+id.String()+"/reject", map[string]string{"message": "Rejected"}, f.cookie, 403, map[string]string{"Origin": "https://example.invalid"})
	f.exec("UPDATE app.sessions SET idle_expires_at=$2 WHERE id=$1", f.author.SessionID.String(), time.Now().Add(-time.Minute))
	if err := f.publicReview.Withdraw(ctx, f.author, id); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("expired public actor accepted")
	}
	f.exec("UPDATE app.sessions SET revoked_at=now() WHERE id=$1", f.actor.SessionID.String())
	if err := f.adminReview.Reject(ctx, f.actor, id, "Rejected", nil); !errors.Is(err, auth.ErrAdminUnauthenticated) {
		t.Fatal("revoked Admin actor accepted")
	}
}
