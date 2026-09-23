package public

import (
	"errors"
	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/curation"
	"github.com/deepfurry/tap4furry/server/internal/database/curationcheck"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/deepfurry/tap4furry/server/internal/resource"
	"github.com/deepfurry/tap4furry/server/internal/taxonomy"
	"github.com/jackc/pgx/v5/pgtype"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
	"uuid"
)

type curationFixture struct {
	*adminFixture
	actor    auth.AdminActor
	email    string
	cookie   *http.Cookie
	graph    *curationcheck.Fixture
	category uuid.UUID
}

func newCurationFixture(t *testing.T) *curationFixture {
	t.Helper()
	if os.Getenv("GFP_CURATION_INTEGRATION") != "1" {
		t.Skip("explicit disposable curation integration not enabled")
	}
	if os.Getenv("CI") != "true" || os.Getenv("GFP_DISPOSABLE_INFRA") != "1" {
		t.Fatal("disposable curation guards required")
	}
	t.Setenv("GFP_AUTH_INTEGRATION", "1")
	f := newAdminFixture(t)
	_, email, _ := f.eligible(auth.Editor)
	_, cookie := f.adminLogin(email)
	actor, err := f.adminAuth.ResolveAdmin(t.Context(), cookie.Value)
	if err != nil {
		t.Fatal("actor resolution failed")
	}
	graph := &curationcheck.Fixture{}
	t.Cleanup(func() {
		if graph.Cleanup(f.owner) != nil {
			t.Error("curation fixture cleanup failed")
		}
	})
	cat, err := f.curation.CreateTaxonomy(t.Context(), actor, curation.Category, curation.TaxonomyInput{Slug: "fixture-" + uuid.NewV7().String(), DefaultLocale: "en", Name: "Category"})
	if err != nil {
		t.Fatal(err)
	}
	graph.Categories = append(graph.Categories, cat.String())
	return &curationFixture{f, actor, email, cookie, graph, cat}
}
func (f *curationFixture) create() curation.Revision {
	f.t.Helper()
	r, err := f.curation.CreateResource(f.t.Context(), f.actor, curation.CreateInput{Slug: "fixture-" + uuid.NewV7().String(), DefaultLocale: "en", CategoryID: f.category, ContentRating: resource.General, Localization: curation.LocalizationInput{Name: "Resource"}})
	if err != nil {
		f.t.Fatal(err)
	}
	f.graph.Resources = append(f.graph.Resources, r.ID.String())
	return r
}
func (f *curationFixture) revision(id uuid.UUID) sqlc.LockResourceCoreRow {
	f.t.Helper()
	row, err := sqlc.New(f.adminPool).LockResourceCore(f.t.Context(), pgtype.UUID{Bytes: id, Valid: true})
	if err != nil {
		f.t.Fatal("read revision failed")
	}
	return row
}
func mustCuration(t *testing.T, r curation.Revision, err error) curation.Revision {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func TestIntegrationCurationHTTPAndPublicLifecycle(t *testing.T) {
	f := newCurationFixture(t)
	// Run begins with a moderator-only fixture, then uses the owner RoleOperator.
	if _, err := f.operator.Revoke(t.Context(), f.email, auth.Editor); err != nil {
		t.Fatal(err)
	}
	if err := curationcheck.Run(t.Context(), f.adminApp, f.app, f.owner, f.operator, f.email, testPassword, adminOrigin); err != nil {
		t.Fatal(err)
	}
}
func TestIntegrationCurationCASNoopRollbackAndLocalization(t *testing.T) {
	f := newCurationFixture(t)
	ctx := t.Context()
	r := f.create()
	before := f.revision(r.ID)
	unknown := resource.Unknown
	same, err := f.curation.PatchResource(ctx, f.actor, r.ID, r.Version, curation.CorePatch{Lifecycle: &unknown})
	if err != nil || same != r || !reflect.DeepEqual(before, f.revision(r.ID)) {
		t.Fatal("no-op changed canonical revision/time")
	}
	value := "  Description 🦊  "
	r, err = f.curation.PutLocalization(ctx, f.actor, r.ID, r.Version, "zh-hans", curation.LocalizationInput{Name: "资源", Description: &value})
	r = mustCuration(t, r, err)
	before = f.revision(r.ID)
	same, err = f.curation.PutLocalization(ctx, f.actor, r.ID, r.Version, "zh-Hans", curation.LocalizationInput{Name: "资源", Description: &value})
	if err != nil || same != r || !reflect.DeepEqual(before, f.revision(r.ID)) {
		t.Fatal("identical localization bumped")
	}
	if _, err = f.curation.PutLocalization(ctx, f.actor, r.ID, 1, "ja", curation.LocalizationInput{Name: "Stale"}); !errors.Is(err, resource.ErrVersionConflict) {
		t.Fatal("stale mutation accepted")
	}
	if f.scalar("SELECT count(*) FROM app.resource_localizations WHERE resource_id=$1 AND locale='ja'", r.ID.String()) != 0 {
		t.Fatal("stale child write escaped")
	}
	if _, err = f.curation.DeleteLocalization(ctx, f.actor, r.ID, r.Version, "en"); !errors.Is(err, curation.ErrValidation) {
		t.Fatal("default localization deleted")
	}
	locale := "ja"
	if _, err = f.curation.PatchResource(ctx, f.actor, r.ID, r.Version, curation.CorePatch{DefaultLocale: &locale}); !errors.Is(err, curation.ErrValidation) {
		t.Fatal("missing default accepted")
	}
	locale = "zh-hans"
	r, err = f.curation.PatchResource(ctx, f.actor, r.ID, r.Version, curation.CorePatch{DefaultLocale: &locale})
	r = mustCuration(t, r, err)
	r, err = f.curation.DeleteLocalization(ctx, f.actor, r.ID, r.Version, "en")
	r = mustCuration(t, r, err)
	large := strings.Repeat("界", 50000)
	r, err = f.curation.PutLocalization(ctx, f.actor, r.ID, r.Version, "zh-Hans", curation.LocalizationInput{Name: "资源", Description: &large})
	r = mustCuration(t, r, err)
	large += "界"
	if _, err = f.curation.PutLocalization(ctx, f.actor, r.ID, r.Version, "zh-Hans", curation.LocalizationInput{Name: "资源", Description: &large}); !errors.Is(err, curation.ErrValidation) {
		t.Fatal("description exceeded Unicode ceiling")
	}
	other := f.create()
	item := resource.ExternalID{Namespace: "fixture", Value: uuid.NewV7().String()}
	_, err = f.curation.SetExternalIDs(ctx, f.actor, other.ID, other.Version, []resource.ExternalID{item})
	if err != nil {
		t.Fatal(err)
	}
	original := resource.ExternalID{Namespace: "fixture", Value: uuid.NewV7().String()}
	r, err = f.curation.SetExternalIDs(ctx, f.actor, r.ID, r.Version, []resource.ExternalID{original})
	r = mustCuration(t, r, err)
	before = f.revision(r.ID)
	if _, err = f.curation.SetExternalIDs(ctx, f.actor, r.ID, r.Version, []resource.ExternalID{item}); !errors.Is(err, curation.ErrConflict) {
		t.Fatal("global identity conflict missing")
	}
	if !reflect.DeepEqual(before, f.revision(r.ID)) || f.scalar("SELECT count(*) FROM app.resource_external_ids WHERE resource_id=$1 AND external_id=$2", r.ID.String(), original.Value) != 1 {
		t.Fatal("child diff did not fully roll back")
	}
	same, err = f.curation.SetExternalIDs(ctx, f.actor, r.ID, r.Version, []resource.ExternalID{original, original})
	if err != nil || same != r {
		t.Fatal("identical external set bumped")
	}
}
func TestIntegrationCurationTaxonomySourceAndPublication(t *testing.T) {
	f := newCurationFixture(t)
	ctx := t.Context()
	r := f.create()
	tag, err := f.curation.CreateTaxonomy(ctx, f.actor, curation.Tag, curation.TaxonomyInput{Slug: "fixture-" + uuid.NewV7().String(), DefaultLocale: "en", Name: "Tag"})
	if err != nil {
		t.Fatal(err)
	}
	f.graph.Tags = append(f.graph.Tags, tag.String())
	r, err = f.curation.SetTags(ctx, f.actor, r.ID, r.Version, []uuid.UUID{tag})
	r = mustCuration(t, r, err)
	if _, err = f.operator.Grant(ctx, f.email, auth.Administrator); err != nil {
		t.Fatal(err)
	}
	retired := taxonomy.Retired
	for _, entry := range []struct {
		k  curation.TaxonomyKind
		id uuid.UUID
	}{{curation.Category, f.category}, {curation.Tag, tag}} {
		if err = f.curation.PatchTaxonomy(ctx, f.actor, entry.k, entry.id, curation.TaxonomyPatch{Reason: "Fixture governance reason", State: &retired}); err != nil {
			t.Fatal(err)
		}
		if err = f.curation.DeleteTaxonomy(ctx, f.actor, entry.k, entry.id, "Fixture governance reason"); !errors.Is(err, curation.ErrInUse) {
			t.Fatal("in-use taxonomy delete allowed")
		}
	}
	before := f.revision(r.ID)
	same, err := f.curation.SetTags(ctx, f.actor, r.ID, r.Version, []uuid.UUID{tag, tag})
	if err != nil || same != r || !reflect.DeepEqual(before, f.revision(r.ID)) {
		t.Fatal("retired existing tag not preserved")
	}
	if _, err = f.curation.PatchResource(ctx, f.actor, r.ID, r.Version, curation.CorePatch{CategoryID: &f.category}); err != nil {
		t.Fatal("unchanged retired category rejected")
	}
	r, err = f.curation.SetTags(ctx, f.actor, r.ID, r.Version, nil)
	r = mustCuration(t, r, err)
	if _, err = f.curation.SetTags(ctx, f.actor, r.ID, r.Version, []uuid.UUID{tag}); !errors.Is(err, curation.ErrValidation) {
		t.Fatal("retired tag rebound")
	}
	if _, err = f.curation.CreateResource(ctx, f.actor, curation.CreateInput{Slug: "rejected", DefaultLocale: "en", CategoryID: f.category, ContentRating: resource.General, Localization: curation.LocalizationInput{Name: "Rejected"}}); !errors.Is(err, curation.ErrValidation) {
		t.Fatal("new retired category binding allowed")
	}
	for _, url := range []string{"https://example.invalid/one", "HTTPS://EXAMPLE.INVALID:443/two#fragment"} {
		r, err = f.curation.CreateSource(ctx, f.actor, r.ID, r.Version, curation.SourceInput{URL: url, Type: resource.SourceOfficial, Availability: resource.SourceActive, Primary: true})
		r = mustCuration(t, r, err)
	}
	sources, err := sqlc.New(f.adminPool).AdminResourceSources(ctx, pgtype.UUID{Bytes: r.ID, Valid: true})
	if err != nil || len(sources) != 2 || sources[0].IsPrimary || !sources[1].IsPrimary {
		t.Fatal("primary auto-switch failed")
	}
	before = f.revision(r.ID)
	truth := true
	same, err = f.curation.PatchSource(ctx, f.actor, r.ID, r.Version, uuid.UUID(sources[1].ID.Bytes), curation.SourcePatch{Primary: &truth})
	if err != nil || same != r || !reflect.DeepEqual(before, f.revision(r.ID)) {
		t.Fatal("source no-op bumped")
	}
	removed := resource.SourceRemoved
	r, err = f.curation.PatchSource(ctx, f.actor, r.ID, r.Version, uuid.UUID(sources[1].ID.Bytes), curation.SourcePatch{Availability: &removed})
	r = mustCuration(t, r, err)
	if f.scalar("SELECT count(*) FROM app.resource_sources WHERE resource_id=$1", r.ID.String()) != 2 {
		t.Fatal("Source removal physically deleted row")
	}
	r, err = f.curation.SetPublication(ctx, f.actor, r.ID, r.Version, resource.Published, "Fixture governance reason")
	r = mustCuration(t, r, err)
	published := f.revision(r.ID).PublishedAt
	for _, state := range []resource.PublicationState{resource.Restricted, resource.Published, resource.Removed, resource.Published, resource.Draft, resource.Published} {
		r, err = f.curation.SetPublication(ctx, f.actor, r.ID, r.Version, state, "Fixture governance reason")
		r = mustCuration(t, r, err)
		if f.revision(r.ID).PublishedAt != published {
			t.Fatal("first publication timestamp changed")
		}
	}
	slug := "frozen-change"
	if _, err = f.curation.PatchResource(ctx, f.actor, r.ID, r.Version, curation.CorePatch{Slug: &slug}); !errors.Is(err, curation.ErrValidation) {
		t.Fatal("published slug changed")
	}
	before = f.revision(r.ID)
	same, err = f.curation.SetPublication(ctx, f.actor, r.ID, r.Version, resource.Published, "Fixture governance reason")
	if err != nil || same != r || !reflect.DeepEqual(before, f.revision(r.ID)) {
		t.Fatal("publication no-op bumped")
	}
}
func TestIntegrationCurationRelationsAndConcurrentCycle(t *testing.T) {
	f := newCurationFixture(t)
	ctx := t.Context()
	a, b, c := f.create(), f.create(), f.create()
	a, err := f.curation.AddRelation(ctx, f.actor, a.ID, a.Version, b.ID, resource.SuccessorOf)
	a = mustCuration(t, a, err)
	if f.revision(b.ID).Version != 2 {
		t.Fatal("other endpoint not bumped")
	}
	b.Version = 2
	b, err = f.curation.AddRelation(ctx, f.actor, b.ID, b.Version, c.ID, resource.SuccessorOf)
	b = mustCuration(t, b, err)
	if _, err = f.curation.AddRelation(ctx, f.actor, c.ID, f.revision(c.ID).Version, a.ID, resource.SuccessorOf); !errors.Is(err, curation.ErrRelationCycle) {
		t.Fatal("same-type cycle allowed")
	}
	if _, err = f.curation.AddRelation(ctx, f.actor, c.ID, f.revision(c.ID).Version, a.ID, resource.PartOf); err != nil {
		t.Fatal("mixed relation semantics rejected")
	}
	if _, err = f.curation.AddRelation(ctx, f.actor, a.ID, f.revision(a.ID).Version, a.ID, resource.RelatedTo); !errors.Is(err, curation.ErrValidation) {
		t.Fatal("self relation allowed")
	}
	if _, err = f.curation.AddRelation(ctx, f.actor, c.ID, f.revision(c.ID).Version, a.ID, resource.RelatedTo); err != nil {
		t.Fatal(err)
	}
	var relationID string
	if err = f.owner.QueryRow(ctx, "SELECT id::text FROM app.resource_relations WHERE relation_type='related_to' AND source_resource_id=$1 AND target_resource_id=$2", a.ID.String(), c.ID.String()).Scan(&relationID); err != nil {
		t.Fatal("symmetric UUID canonicalization failed")
	}
	beforeA, beforeC := f.revision(a.ID).Version, f.revision(c.ID).Version
	rid, _ := uuid.Parse(relationID)
	if _, err = f.curation.DeleteRelation(ctx, f.actor, c.ID, beforeC, rid); err != nil {
		t.Fatal(err)
	}
	if f.revision(a.ID).Version != beforeA+1 || f.revision(c.ID).Version != beforeC+1 {
		t.Fatal("delete must bump both endpoints")
	}
	// Separate actors avoid serializing this race on the actor lock itself.
	_, email, _ := f.eligible(auth.Editor)
	_, cookie := f.adminLogin(email)
	actor2, err := f.adminAuth.ResolveAdmin(ctx, cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	x, y := f.create(), f.create()
	start := make(chan struct{})
	done := make(chan error, 2)
	go func() {
		<-start
		_, e := f.curation.AddRelation(ctx, f.actor, x.ID, 1, y.ID, resource.DerivedFrom)
		done <- e
	}()
	go func() {
		<-start
		_, e := f.curation.AddRelation(ctx, actor2, y.ID, 1, x.ID, resource.DerivedFrom)
		done <- e
	}()
	close(start)
	one, two := <-done, <-done
	if (one == nil) == (two == nil) {
		t.Fatal("concurrent cycle race must have exactly one winner")
	}
	loser := one
	if loser == nil {
		loser = two
	}
	if !errors.Is(loser, curation.ErrRelationCycle) && !errors.Is(loser, resource.ErrVersionConflict) {
		t.Fatal("unexpected unsafe race failure")
	}
	if f.scalar("SELECT count(*) FROM app.resource_relations WHERE source_resource_id=ANY($1::uuid[])", []string{x.ID.String(), y.ID.String()}) != 1 {
		t.Fatal("concurrent cycle committed")
	}
}
func TestIntegrationCurationRoleRace(t *testing.T) {
	f := newCurationFixture(t)
	ctx := t.Context()
	r := f.create()
	if _, err := f.operator.Grant(ctx, f.email, auth.Moderator); err != nil {
		t.Fatal(err)
	}
	pause := f.gate.armQuery(t, "SELECT clock_timestamp()") // User lock is already held.
	done := make(chan error, 1)
	active := resource.Active
	go func() {
		_, err := f.curation.PatchResource(ctx, f.actor, r.ID, 1, curation.CorePatch{Lifecycle: &active})
		done <- err
	}()
	select {
	case <-pause.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("actor lock not reached")
	}
	revoked := make(chan error, 1)
	go func() { _, err := f.operator.Revoke(ctx, f.email, auth.Editor); revoked <- err }()
	// Observe the actual competing DB lock wait, not a sleep-based inference.
	deadline := time.Now().Add(4 * time.Second)
	blocked := false
	for time.Now().Before(deadline) {
		var count int
		err := f.owner.QueryRow(ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND usename='gfp_migrator' AND wait_event_type='Lock' AND query LIKE '%LockRoleUserByEmail%'").Scan(&count)
		if err != nil {
			t.Fatal("lock observation failed")
		}
		if count > 0 {
			blocked = true
			break
		}
		select {
		case <-revoked:
			t.Fatal("revoke escaped actor lock")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if !blocked {
		t.Fatal("role revocation did not wait for actor lock")
	}
	close(pause.resume)
	if err := <-done; err != nil {
		t.Fatal("authorized mutation lost its lock authority")
	}
	if err := <-revoked; err != nil {
		t.Fatal(err)
	}
	if _, err := f.curation.PatchResource(ctx, f.actor, r.ID, 2, curation.CorePatch{}); !errors.Is(err, auth.ErrAdminForbidden) {
		t.Fatal("stale actor capabilities survived role revoke")
	}
}
func TestIntegrationCurationHTTPBoundaries(t *testing.T) {
	f := newCurationFixture(t)
	r := f.create()
	path := "/resources/" + r.ID.String() + "?expected_version=1"
	for _, headers := range []map[string]string{{"Origin": "http://localhost:4321"}, {"X-CSRF-Token": "invalid"}} {
		f.adminRequest("PATCH", path, map[string]string{"lifecycle": "active"}, f.cookie, 403, headers)
	}
	f.adminRequest("GET", "/resources", nil, nil, 401)
	for _, body := range []any{map[string]any{"lifecycle": nil}, map[string]any{"publication_state": "published"}, map[string]any{"version": 20}} {
		f.adminRequest("PATCH", path, body, f.cookie, 400)
	}
	for _, query := range []string{"", "?expected_version=0", "?expected_version=1&expected_version=1", "?expected_version=garbage"} {
		f.adminRequest("PATCH", "/resources/"+r.ID.String()+query, map[string]any{}, f.cookie, 400)
	}
	large := strings.Repeat("界", 50000)
	f.adminRequest("PUT", "/resources/"+r.ID.String()+"/localizations/en?expected_version=1", map[string]string{"name": "Unicode", "description": large}, f.cookie, 200)
	f.adminRequest("POST", "/auth/reauthenticate", map[string]string{"password": strings.Repeat("a", 9000)}, f.cookie, 400)
	f.adminRequest("DELETE", "/resources/"+r.ID.String()+"/sources/"+uuid.NewV7().String()+"?expected_version=2", nil, f.cookie, 404)
	f.adminRequest("GET", "/resources?q=fuzzy", nil, f.cookie, 400)
}

func TestIntegrationCurationTaxonomyLocalizationsAndReadFilters(t *testing.T) {
	f := newCurationFixture(t)
	ctx := t.Context()
	for _, kind := range []curation.TaxonomyKind{curation.Category, curation.Tag} {
		id, err := f.curation.CreateTaxonomy(ctx, f.actor, kind, curation.TaxonomyInput{Slug: "fixture-" + uuid.NewV7().String(), DefaultLocale: "en", Name: "Canonical"})
		if err != nil {
			t.Fatal(err)
		}
		path := "/categories/" + id.String()
		if kind == curation.Category {
			f.graph.Categories = append(f.graph.Categories, id.String())
		} else {
			f.graph.Tags = append(f.graph.Tags, id.String())
			path = "/tags/" + id.String()
		}
		before, _ := f.adminRequest("GET", path, nil, f.cookie, 200)
		f.adminRequest("PUT", path+"/localizations/en", map[string]any{"name": "Canonical", "description": nil}, f.cookie, 204)
		after, _ := f.adminRequest("GET", path, nil, f.cookie, 200)
		if before["updated_at"] != after["updated_at"] {
			t.Fatal("taxonomy no-op touched parent")
		}
		f.adminRequest("DELETE", path+"/localizations/en", nil, f.cookie, 400)
		f.adminRequest("PATCH", path, map[string]string{"default_locale": "ja"}, f.cookie, 400)
		f.adminRequest("PATCH", path, map[string]string{"slug": "immutable"}, f.cookie, 400)
		f.adminRequest("PUT", path+"/localizations/zh-hans", map[string]string{"name": "本地化", "description": " 描述 "}, f.cookie, 204)
		f.adminRequest("PATCH", path, map[string]string{"default_locale": "zh-Hans"}, f.cookie, 200)
		f.adminRequest("DELETE", path+"/localizations/en", nil, f.cookie, 204)
		after, _ = f.adminRequest("GET", path, nil, f.cookie, 200)
		if after["default_locale"] != "zh-Hans" || len(after["localizations"].([]any)) != 1 {
			t.Fatal("taxonomy localization workflow failed")
		}
	}
	a, b := f.create(), f.create()
	first := f.revision(a.ID)
	exact, _ := f.adminRequest("GET", "/resources?slug="+first.Slug, nil, f.cookie, 200)
	items := exact["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["id"] != a.ID.String() {
		t.Fatal("Admin slug lookup was not exact")
	}
	page, _ := f.adminRequest("GET", "/resources?category_id="+f.category.String()+"&page_size=1", nil, f.cookie, 200)
	if page["has_next"] != true || len(page["items"].([]any)) != 1 {
		t.Fatal("Admin pagination lookahead failed")
	}
	f.adminRequest("GET", "/resources?page=9223372036854775807&page_size=100", nil, f.cookie, 400)
	if _, err := f.curation.SetPublication(ctx, f.actor, b.ID, b.Version, resource.Pending, "Fixture governance reason"); err != nil {
		t.Fatal(err)
	}
	filtered, _ := f.adminRequest("GET", "/resources?category_id="+f.category.String()+"&publication_state=pending", nil, f.cookie, 200)
	if len(filtered["items"].([]any)) != 1 || filtered["items"].([]any)[0].(map[string]any)["id"] != b.ID.String() {
		t.Fatal("Admin list filters failed")
	}
}
