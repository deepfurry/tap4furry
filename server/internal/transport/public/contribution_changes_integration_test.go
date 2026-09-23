package public

import (
	"context"
	"errors"
	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/contribution"
	"github.com/deepfurry/tap4furry/server/internal/curation"
	"github.com/deepfurry/tap4furry/server/internal/resource"
	"github.com/deepfurry/tap4furry/server/internal/taxonomy"
	"reflect"
	"strings"
	"testing"
	"time"
	"uuid"
)

func (f *contributionFixture) published() curation.Revision {
	f.t.Helper()
	r := f.create()
	r, e := f.curation.SetPublication(f.t.Context(), f.actor, r.ID, r.Version, resource.Published, "Fixture governance reason")
	if e != nil {
		f.t.Fatal(e)
	}
	return r
}
func (f *contributionFixture) changeInput(r curation.Revision, kind string, c *contribution.Change) contribution.SubmitInput {
	f.t.Helper()
	opts := contribution.ContextInput{Kind: kind, SourceID: c.SourceID}
	if c.Translation != nil {
		opts.Locale = c.Translation.Locale
	}
	if rel := c.Relation; rel != nil {
		opts.OtherSlug = f.revision(rel.OtherID).Slug
		opts.Direction = rel.Direction
		opts.RelationType = rel.Type
	}
	base, e := f.publicReview.ContextFor(f.t.Context(), f.author, f.revision(r.ID).Slug, opts)
	if e != nil {
		f.t.Fatal(e)
	}
	return contribution.SubmitInput{Kind: kind, RequestID: uuid.NewV7(), TargetID: r.ID, BaseRevision: base.BaseRevision, Reason: "Fixture evidence", Change: c}
}
func (f *contributionFixture) changeAccept(key uuid.UUID) contribution.AcceptInput {
	f.t.Helper()
	v, e := f.adminReview.ReviewDetail(f.t.Context(), f.actor, key)
	if e != nil {
		f.t.Fatal(e)
	}
	c := v.ProposedChange
	if c.Source != nil {
		c.Source.Availability = "active"
	}
	if c.Translation != nil {
		c.Translation.Summary.Set = true
		c.Translation.Description.Set = true
	}
	return contribution.AcceptInput{Change: c}
}
func TestIntegrationContributionChangesTranslation(t *testing.T) {
	f := newContributionFixture(t)
	ctx := t.Context()
	r := f.published()
	r, e := f.curation.PutLocalization(ctx, f.actor, r.ID, r.Version, "ja", curation.LocalizationInput{Name: "Canonical-only translated name", Summary: cp("Old summary"), Description: cp("Untouched translated description")})
	if e != nil {
		t.Fatal(e)
	}
	in := f.changeInput(r, contribution.AddTranslation, &contribution.Change{Translation: &contribution.TranslationChange{Locale: "ja", Summary: contribution.NullableText{Set: true}}})
	key := f.submit(in)
	own, e := f.publicReview.OwnDetail(ctx, f.author, key)
	if e != nil {
		t.Fatal(e)
	}
	if tr := own.ProposedChange.Translation; tr.Name != nil || tr.Description.Set || tr.Description.Value != nil || !tr.Summary.Set || tr.Summary.Value != nil {
		t.Fatal("partial translation leaked baseline or lost explicit null")
	}
	accepted := f.changeAccept(key)
	if e = f.adminReview.Accept(ctx, f.actor, key, accepted); e != nil {
		t.Fatal(e)
	}
	var name, description string
	var summary *string
	if e = f.owner.QueryRow(ctx, "SELECT name,summary,description FROM app.resource_localizations WHERE resource_id=$1 AND locale='ja'", r.ID.String()).Scan(&name, &summary, &description); e != nil || name != "Canonical-only translated name" || summary != nil || description != "Untouched translated description" {
		t.Fatal("partial translation overwrote omitted fields")
	}
	f.cooldown()
	same := f.changeInput(r, contribution.AddTranslation, &contribution.Change{Translation: &contribution.TranslationChange{Locale: "ja", Summary: contribution.NullableText{Set: true}}})
	before := f.revision(r.ID)
	if _, e = f.publicReview.Submit(ctx, f.author, same); !errors.Is(e, contribution.ErrValidation) {
		t.Fatal("translation no-op submitted")
	}
	if !reflect.DeepEqual(before, f.revision(r.ID)) {
		t.Fatal("no-op bumped resource")
	}
	missing, e := f.publicReview.ContextFor(ctx, f.author, before.Slug, contribution.ContextInput{Kind: contribution.AddTranslation, Locale: "fr"})
	if e != nil || missing.Change.Translation.Exists || missing.Change.Translation.Name != nil || missing.Reference == nil {
		t.Fatal("missing translation copied fallback")
	}
	if _, e = f.publicReview.ContextFor(ctx, f.author, before.Slug, contribution.ContextInput{Kind: contribution.AddTranslation, Locale: "en"}); !errors.Is(e, contribution.ErrValidation) {
		t.Fatal("default translation bypass")
	}
	// Default-language corruption fails closed on the private context as well.
	f.exec("DELETE FROM app.resource_localizations WHERE resource_id=$1 AND locale='en'", r.ID.String())
	f.request("GET", "/contributions/context/"+before.Slug+"?kind=add_translation&locale=fr", nil, f.authorCookie, 500)
}
func TestIntegrationContributionChangesSourcePrivacyAndStale(t *testing.T) {
	f := newContributionFixture(t)
	ctx := t.Context()
	r := f.published()
	in := f.changeInput(r, contribution.AddSource, &contribution.Change{Source: &contribution.Source{URL: "HTTPS://EXAMPLE.INVALID:443/source#fragment", Type: "official"}})
	key := f.submit(in)
	final := f.changeAccept(key)
	final.Change.Source.Label = cp("Reviewer-added label")
	if e := f.adminReview.Accept(ctx, f.actor, key, final); !errors.Is(e, contribution.ErrValidation) {
		t.Fatal("unexplained source edit accepted")
	}
	final.Message = cp("Corrected source label")
	if e := f.adminReview.Accept(ctx, f.actor, key, final); e != nil {
		t.Fatal(e)
	}
	var source string
	var primary bool
	var rights string
	if e := f.owner.QueryRow(ctx, "SELECT id::text,is_primary,rights_status FROM app.resource_sources WHERE resource_id=$1", r.ID.String()).Scan(&source, &primary, &rights); e != nil || primary || rights != "unknown" {
		t.Fatal("source defaults violated")
	}
	sid, _ := uuid.Parse(source)
	f.cooldown()
	remove := f.changeInput(r, contribution.RemoveSource, &contribution.Change{SourceID: sid})
	removal := f.submit(remove)
	original, e := f.publicReview.OwnDetail(ctx, f.author, removal)
	if e != nil || original.ProposedChange.Source != nil {
		t.Fatal("removal original leaked canonical URL")
	}
	if e = f.adminReview.Accept(ctx, f.actor, removal, f.changeAccept(removal)); e != nil {
		t.Fatal(e)
	}
	if f.scalar("SELECT count(*) FROM app.resource_sources WHERE id=$1 AND availability_state='removed'", source) != 1 {
		t.Fatal("source was deleted rather than retained")
	}
	original, e = f.publicReview.OwnDetail(ctx, f.author, removal)
	if e != nil || original.AcceptedChange.Source != nil {
		t.Fatal("removed URL leaked")
	}
	old, e := f.publicReview.OwnDetail(ctx, f.author, key)
	if e != nil || old.AcceptedChange != nil {
		t.Fatal("historical reviewer source resurrected")
	}
	f.cooldown()
	again := f.changeInput(r, contribution.AddSource, in.Change)
	if _, e = f.publicReview.Submit(ctx, f.author, again); !errors.Is(e, contribution.ErrConflict) {
		t.Fatal("removed URL duplicated")
	}
	next := f.changeInput(r, contribution.AddSource, &contribution.Change{Source: &contribution.Source{URL: "https://example.invalid/next", Type: "unknown"}})
	stale := f.submit(next)
	final = f.changeAccept(stale)
	if _, e = f.curation.PatchResource(ctx, f.actor, r.ID, f.revision(r.ID).Version, curation.CorePatch{Lifecycle: cp(resource.Active)}); e != nil {
		t.Fatal(e)
	}
	if e = f.adminReview.Accept(ctx, f.actor, stale, final); !errors.Is(e, resource.ErrVersionConflict) {
		t.Fatal("stale source accepted")
	}
	if f.scalar("SELECT count(*) FROM app.resource_sources WHERE resource_id=$1", r.ID.String()) != 1 {
		t.Fatal("stale child escaped rollback")
	}
}
func TestIntegrationContributionChangesTagsAtomic(t *testing.T) {
	f := newContributionFixture(t)
	ctx := t.Context()
	r := f.published()
	ids := []uuid.UUID{}
	for range 3 {
		tag, e := f.curation.CreateTaxonomy(ctx, f.actor, curation.Tag, curation.TaxonomyInput{Slug: "tag-" + uuid.NewV7().String(), DefaultLocale: "en", Name: "Tag"})
		if e != nil {
			t.Fatal(e)
		}
		ids = append(ids, tag)
		f.graph.Tags = append(f.graph.Tags, tag.String())
	}
	r, e := f.curation.SetTags(ctx, f.actor, r.ID, r.Version, ids[:1])
	if e != nil {
		t.Fatal(e)
	}
	key := f.submit(f.changeInput(r, contribution.AddTag, &contribution.Change{Tags: ids[1:]}))
	detail, e := f.adminReview.ReviewDetail(ctx, f.actor, key)
	if e != nil || detail.BaseChange == nil || len(detail.BaseChange.Tags) != 0 || detail.CurrentChange == nil || len(detail.CurrentChange.Tags) != 0 {
		t.Fatal("unbound additions appeared in canonical baseline", e)
	}
	final := f.changeAccept(key)
	if _, e = f.operator.Grant(ctx, f.email, auth.Administrator); e != nil {
		t.Fatal(e)
	}
	if e = f.curation.PatchTaxonomy(ctx, f.actor, curation.Tag, ids[2], curation.TaxonomyPatch{Reason: "Fixture governance reason", State: cp(taxonomy.Retired)}); e != nil {
		t.Fatal(e)
	}
	before := f.revision(r.ID)
	if e = f.adminReview.Accept(ctx, f.actor, key, final); !errors.Is(e, curation.ErrValidation) {
		t.Fatal("retired tag binding accepted")
	}
	if !reflect.DeepEqual(before, f.revision(r.ID)) || f.scalar("SELECT count(*) FROM app.resource_tags WHERE resource_id=$1", r.ID.String()) != 1 {
		t.Fatal("partial tag write escaped")
	}
	final.Change.Tags = ids[1:2]
	final.Message = cp("Omitted retired tag")
	if e = f.adminReview.Accept(ctx, f.actor, key, final); e != nil {
		t.Fatal(e)
	}
	if f.revision(r.ID).Version != before.Version+1 || f.scalar("SELECT count(*) FROM app.resource_tags WHERE resource_id=$1", r.ID.String()) != 2 {
		t.Fatal("unrelated binding lost or multi-bump")
	}
	detail, e = f.adminReview.ReviewDetail(ctx, f.actor, key)
	if e != nil || len(detail.BaseChange.Tags) != 0 || !reflect.DeepEqual(detail.CurrentChange.Tags, ids[1:2]) {
		t.Fatal("tag baseline changed or current binding missing", e)
	}
}
func TestIntegrationContributionChangesRelationCASAuditAndPrivacy(t *testing.T) {
	f := newContributionFixture(t)
	ctx := t.Context()
	a, b := f.published(), f.published()
	change := &contribution.Change{Relation: &contribution.RelationChange{OtherID: b.ID, Type: resource.PartOf, Direction: "outgoing"}}
	key := f.submit(f.changeInput(a, contribution.AddRelation, change))
	final := f.changeAccept(key)
	if _, e := f.curation.PatchResource(ctx, f.actor, b.ID, b.Version, curation.CorePatch{Lifecycle: cp(resource.Active)}); e != nil {
		t.Fatal(e)
	}
	if e := f.adminReview.Accept(ctx, f.actor, key, final); !errors.Is(e, resource.ErrVersionConflict) {
		t.Fatal("other endpoint stale was ignored")
	}
	f.cooldown()
	key = f.submit(f.changeInput(a, contribution.AddRelation, change))
	final = f.changeAccept(key)
	f.exec("ALTER TABLE app.contribution_review_resource_changes ADD CONSTRAINT contribution_change_failure CHECK (contribution_id <> '" + key.String() + "'::uuid)")
	t.Cleanup(func() {
		if _, e := f.owner.Exec(context.Background(), "ALTER TABLE app.contribution_review_resource_changes DROP CONSTRAINT IF EXISTS contribution_change_failure"); e != nil {
			t.Error("fault cleanup failed")
		}
	})
	va, vb := f.revision(a.ID), f.revision(b.ID)
	if e := f.adminReview.Accept(ctx, f.actor, key, final); e == nil {
		t.Fatal("injected dual audit failure accepted")
	}
	if !reflect.DeepEqual(va, f.revision(a.ID)) || !reflect.DeepEqual(vb, f.revision(b.ID)) || f.scalar("SELECT count(*) FROM app.resource_relations WHERE source_resource_id=$1", a.ID.String()) != 0 || f.scalar("SELECT count(*) FROM app.contribution_relation_changes WHERE contribution_id=$1 AND snapshot_kind='accepted'", key.String()) != 0 {
		t.Fatal("partial relation transaction escaped")
	}
	f.exec("ALTER TABLE app.contribution_review_resource_changes DROP CONSTRAINT contribution_change_failure")
	if e := f.adminReview.Accept(ctx, f.actor, key, final); e != nil {
		t.Fatal(e)
	}
	if f.revision(a.ID).Version != va.Version+1 || f.revision(b.ID).Version != vb.Version+1 || f.scalar("SELECT count(*) FROM app.contribution_review_resource_changes WHERE contribution_id=$1", key.String()) != 2 {
		t.Fatal("dual revision/audit incomplete")
	}
	own, e := f.publicReview.OwnDetail(ctx, f.author, key)
	if e != nil || own.AcceptedChange == nil {
		t.Fatal("current relation result missing")
	}
	f.exec("UPDATE app.resources SET publication_state='restricted' WHERE id=$1", b.ID.String())
	own, e = f.publicReview.OwnDetail(ctx, f.author, key)
	if e != nil || own.AcceptedChange != nil {
		t.Fatal("hidden endpoint leaked through result")
	}
	body, _ := f.request("GET", "/me/contributions/"+key.String(), nil, f.authorCookie, 200)
	if body["accepted_change"] != nil {
		t.Fatal("hidden relation serialized")
	}
}
func TestIntegrationContributionChangesRelationCycleRace(t *testing.T) {
	f := newContributionFixture(t)
	ctx := t.Context()
	a, b, c := f.published(), f.published(), f.published()
	if _, e := f.curation.AddRelation(ctx, f.actor, a.ID, a.Version, b.ID, resource.PartOf); e != nil {
		t.Fatal(e)
	}
	forward := f.submit(f.changeInput(b, contribution.AddRelation, &contribution.Change{Relation: &contribution.RelationChange{OtherID: c.ID, Type: resource.PartOf, Direction: "outgoing"}}))
	in1 := f.changeAccept(forward)
	f.cooldown()
	back := f.submit(f.changeInput(c, contribution.AddRelation, &contribution.Change{Relation: &contribution.RelationChange{OtherID: a.ID, Type: resource.PartOf, Direction: "outgoing"}}))
	in2 := f.changeAccept(back)
	_, email, _ := f.eligible(auth.Editor)
	_, cookie := f.adminLogin(email)
	actor, e := f.adminAuth.ResolveAdmin(ctx, cookie.Value)
	if e != nil {
		t.Fatal(e)
	}
	start := make(chan struct{})
	done := make(chan error, 2)
	go func() { <-start; done <- f.adminReview.Accept(ctx, f.actor, forward, in1) }()
	go func() { <-start; done <- f.adminReview.Accept(ctx, actor, back, in2) }()
	close(start)
	successes := 0
	for range 2 {
		e := <-done
		if e == nil {
			successes++
		} else if !errors.Is(e, resource.ErrVersionConflict) && !errors.Is(e, curation.ErrRelationCycle) {
			t.Fatal(e)
		}
	}
	if successes != 1 {
		t.Fatal("concurrent cycle was accepted")
	}
	// A fresh, same-type closing edge is rejected. A different type is independent.
	var from, to curation.Revision
	if f.scalar("SELECT count(*) FROM app.resource_relations WHERE source_resource_id=$1 AND target_resource_id=$2", b.ID.String(), c.ID.String()) == 1 {
		from, to = c, a
	} else {
		from, to = b, c
	}
	f.cooldown()
	closeKey := f.submit(f.changeInput(from, contribution.AddRelation, &contribution.Change{Relation: &contribution.RelationChange{OtherID: to.ID, Type: resource.PartOf, Direction: "outgoing"}}))
	if e = f.adminReview.Accept(ctx, f.actor, closeKey, f.changeAccept(closeKey)); !errors.Is(e, curation.ErrRelationCycle) {
		t.Fatal("same-type cycle not rejected")
	}
	f.cooldown()
	different := f.submit(f.changeInput(from, contribution.AddRelation, &contribution.Change{Relation: &contribution.RelationChange{OtherID: to.ID, Type: resource.DerivedFrom, Direction: "outgoing"}}))
	if e = f.adminReview.Accept(ctx, f.actor, different, f.changeAccept(different)); e != nil {
		t.Fatal(e)
	}
}
func TestIntegrationContributionChangesRevokeAndContextScope(t *testing.T) {
	f := newContributionFixture(t)
	ctx := t.Context()
	r := f.published()
	in := f.changeInput(r, contribution.AddSource, &contribution.Change{Source: &contribution.Source{URL: "https://example.invalid/source", Type: "unknown"}})
	old := in
	old.Kind = contribution.AddTranslation
	old.Change = &contribution.Change{Translation: &contribution.TranslationChange{Locale: "ja", Name: cp("翻訳")}}
	if _, e := f.publicReview.Submit(ctx, f.author, old); !errors.Is(e, resource.ErrVersionConflict) {
		t.Fatal("context reused across kinds")
	}
	key := f.submit(in)
	final := f.changeAccept(key)
	pause := f.gate.arm(t)
	done := make(chan error, 1)
	go func() { done <- f.adminReview.Accept(ctx, f.actor, key, final) }()
	<-pause.entered
	if _, e := f.operator.Revoke(ctx, f.email, auth.Editor); e != nil {
		t.Fatal(e)
	}
	close(pause.resume)
	if e := <-done; !errors.Is(e, auth.ErrAdminForbidden) && !errors.Is(e, auth.ErrAdminUnauthenticated) {
		t.Fatal("revoked role accepted extended proposal")
	}
	if f.scalar("SELECT count(*) FROM app.resource_sources WHERE resource_id=$1", r.ID.String()) != 0 {
		t.Fatal("revoked source write escaped")
	}
	// Public responses never reveal the private revision MAC as a raw version.
	body, _ := f.request("GET", "/me/contributions/"+key.String(), nil, f.authorCookie, 200)
	for field := range body {
		if strings.Contains(field, "version") || strings.Contains(field, "internal") {
			t.Fatal("private field serialized")
		}
	}
}

func TestIntegrationContributionChangesTagRetirementRace(t *testing.T) {
	f := newContributionFixture(t)
	ctx := t.Context()
	r := f.published()
	tag, err := f.curation.CreateTaxonomy(ctx, f.actor, curation.Tag, curation.TaxonomyInput{Slug: "tag-" + uuid.NewV7().String(), DefaultLocale: "en", Name: "Race"})
	if err != nil {
		t.Fatal(err)
	}
	f.graph.Tags = append(f.graph.Tags, tag.String())
	key := f.submit(f.changeInput(r, contribution.AddTag, &contribution.Change{Tags: []uuid.UUID{tag}}))
	final := f.changeAccept(key)
	_, email, _ := f.eligible(auth.Administrator)
	_, cookie := f.adminLogin(email)
	governor, err := f.adminAuth.ResolveAdmin(ctx, cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	pause := f.gate.armQuery(t, "-- name: CurationLockTag")
	done := make(chan error, 1)
	go func() { done <- f.adminReview.Accept(ctx, f.actor, key, final) }()
	select {
	case <-pause.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("tag lock not reached")
	}
	if err = f.curation.PatchTaxonomy(ctx, governor, curation.Tag, tag, curation.TaxonomyPatch{Reason: "Fixture governance reason", State: cp(taxonomy.Retired)}); err != nil {
		t.Fatal(err)
	}
	close(pause.resume)
	if err = <-done; !errors.Is(err, curation.ErrValidation) {
		t.Fatal("retirement race passed stale tag eligibility", err)
	}
	if f.revision(r.ID).Version != r.Version || f.scalar("SELECT count(*) FROM app.resource_tags WHERE resource_id=$1", r.ID.String()) != 0 {
		t.Fatal("retirement race partially committed")
	}
}

func TestIntegrationContributionChangesDirectRelationRace(t *testing.T) {
	f := newContributionFixture(t)
	ctx := t.Context()
	a, b := f.published(), f.published()
	key := f.submit(f.changeInput(a, contribution.AddRelation, &contribution.Change{Relation: &contribution.RelationChange{OtherID: b.ID, Type: resource.PartOf, Direction: "outgoing"}}))
	final := f.changeAccept(key)
	_, email, _ := f.eligible(auth.Editor)
	_, cookie := f.adminLogin(email)
	actor, err := f.adminAuth.ResolveAdmin(ctx, cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	start, done := make(chan struct{}), make(chan error, 2)
	go func() { <-start; done <- f.adminReview.Accept(ctx, f.actor, key, final) }()
	go func() {
		<-start
		_, e := f.curation.AddRelation(ctx, actor, b.ID, b.Version, a.ID, resource.PartOf)
		done <- e
	}()
	close(start)
	successes := 0
	for range 2 {
		if err = <-done; err == nil {
			successes++
		} else if !errors.Is(err, resource.ErrVersionConflict) && !errors.Is(err, curation.ErrRelationCycle) {
			t.Fatal(err)
		}
	}
	if successes != 1 || f.scalar("SELECT count(*) FROM app.resource_relations WHERE source_resource_id=ANY($1::uuid[])", []string{a.ID.String(), b.ID.String()}) != 1 {
		t.Fatal("direct curation and proposal did not share graph serialization")
	}
}

func TestIntegrationContributionChangesRejectWithdrawAndSharedQuota(t *testing.T) {
	for _, kind := range []string{contribution.AddSource, contribution.RemoveSource, contribution.AddTag, contribution.AddRelation, contribution.AddTranslation} {
		t.Run(kind, func(t *testing.T) {
			f := newContributionFixture(t)
			ctx := t.Context()
			r := f.published()
			change := &contribution.Change{}
			switch kind {
			case contribution.AddSource:
				change.Source = &contribution.Source{URL: "https://example.invalid/lifecycle", Type: "unknown"}
			case contribution.RemoveSource:
				var err error
				r, err = f.curation.CreateSource(ctx, f.actor, r.ID, r.Version, curation.SourceInput{URL: "https://example.invalid/retained", Type: "official", Availability: "active", Primary: true})
				if err != nil {
					t.Fatal(err)
				}
				var source string
				if err = f.owner.QueryRow(ctx, "SELECT id::text FROM app.resource_sources WHERE resource_id=$1", r.ID.String()).Scan(&source); err != nil {
					t.Fatal(err)
				}
				change.SourceID, _ = uuid.Parse(source)
			case contribution.AddTag:
				tag, err := f.curation.CreateTaxonomy(ctx, f.actor, curation.Tag, curation.TaxonomyInput{Slug: "tag-" + uuid.NewV7().String(), DefaultLocale: "en", Name: "Tag"})
				if err != nil {
					t.Fatal(err)
				}
				f.graph.Tags = append(f.graph.Tags, tag.String())
				change.Tags = []uuid.UUID{tag}
			case contribution.AddRelation:
				other := f.published()
				change.Relation = &contribution.RelationChange{OtherID: other.ID, Type: resource.RelatedTo, Direction: "symmetric"}
			case contribution.AddTranslation:
				change.Translation = &contribution.TranslationChange{Locale: "fr", Name: cp("Traduction")}
			}
			key := f.submit(f.changeInput(r, kind, change))
			var limited *contribution.LimitError
			if _, err := f.publicReview.Submit(ctx, f.author, f.input()); !errors.As(err, &limited) || limited.Reason != "submission_interval" {
				t.Fatal("old and new kinds did not share quota")
			}
			if err := f.adminReview.Reject(ctx, f.actor, key, "Please provide better evidence", cp("Private review note")); err != nil {
				t.Fatal(err)
			}
			own, err := f.publicReview.OwnDetail(ctx, f.author, key)
			if err != nil || own.Status != "rejected" || own.ProposedChange == nil || own.AcceptedChange != nil {
				t.Fatal("new-kind rejection history broken", err)
			}
			f.cooldown()
			key = f.submit(f.changeInput(r, kind, change))
			if err = f.publicReview.Withdraw(ctx, f.author, key); err != nil {
				t.Fatal(err)
			}
			own, err = f.publicReview.OwnDetail(ctx, f.author, key)
			if err != nil || own.Status != "withdrawn" || f.revision(r.ID).Version != r.Version {
				t.Fatal("withdrawal changed canonical state", err)
			}
		})
	}
}

func TestIntegrationContributionChangesSnapshotRevoke(t *testing.T) {
	for _, reviewer := range []bool{false, true} {
		t.Run(map[bool]string{false: "public-session", true: "editorial-role"}[reviewer], func(t *testing.T) {
			f := newContributionFixture(t)
			ctx := t.Context()
			key := f.submit(f.changeInput(f.published(), contribution.AddSource, &contribution.Change{Source: &contribution.Source{URL: "https://example.invalid/private", Type: "unknown"}}))
			user, role := f.author.UserID, "gfp_api"
			if reviewer {
				user, role = f.actor.UserID, "gfp_admin"
				if _, err := f.operator.Grant(ctx, f.email, auth.Moderator); err != nil {
					t.Fatal(err)
				}
			}
			// Reproduce an Auth/owner mutation holding User while a snapshot read
			// starts. Neither session nor role revocation changes the User row.
			tx, err := f.owner.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if _, err = tx.Exec(ctx, "SELECT id FROM app.users WHERE id=$1 FOR UPDATE", user.String()); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() {
				if reviewer {
					_, e := f.adminReview.ReviewDetail(ctx, f.actor, key)
					done <- e
				} else {
					_, e := f.publicReview.OwnDetail(ctx, f.author, key)
					done <- e
				}
			}()
			blocked := false
			deadline := time.Now().Add(4 * time.Second)
			for time.Now().Before(deadline) {
				var n int
				if err = f.owner.QueryRow(ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND usename=$1 AND $2::integer=ANY(pg_blocking_pids(pid))", role, int32(tx.Conn().PgConn().PID())).Scan(&n); err != nil {
					t.Fatal(err)
				}
				if n > 0 {
					blocked = true
					break
				}
				select {
				case <-done:
					t.Fatal("snapshot escaped User lock")
				case <-time.After(10 * time.Millisecond):
				}
			}
			if !blocked {
				t.Fatal("snapshot lock wait was not observed")
			}
			if reviewer {
				_, err = tx.Exec(ctx, "DELETE FROM app.user_roles WHERE user_id=$1 AND role='editor'", user.String())
			} else {
				_, err = tx.Exec(ctx, "UPDATE app.sessions SET revoked_at=clock_timestamp() WHERE id=$1", f.author.SessionID.String())
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			want := auth.ErrUnauthenticated
			if reviewer {
				want = auth.ErrAdminForbidden
			}
			if err = <-done; !errors.Is(err, want) {
				t.Fatal("private snapshot outlived current authorization", err)
			}
		})
	}
}
