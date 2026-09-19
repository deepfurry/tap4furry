package public

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/deepfurry/tap4furry/server/internal/database/publicreadcheck"
	"github.com/deepfurry/tap4furry/server/internal/transport/public/generated"
	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5/pgxpool"
)

type publicFixture struct {
	*publicreadcheck.Fixture
	app        *fiber.App
	owner, api *pgxpool.Pool
}

func newPublicFixture(t *testing.T) *publicFixture {
	t.Helper()
	if os.Getenv("GFP_PUBLIC_READ_INTEGRATION") != "1" {
		t.Skip("explicit public read disposable integration not enabled")
	}
	if os.Getenv("CI") != "true" || os.Getenv("GFP_DISPOSABLE_INFRA") != "1" {
		t.Fatal("public read integration requires disposable guards")
	}
	f := &publicFixture{Fixture: publicreadcheck.NewFixture(), app: fiber.New()}
	for _, role := range []string{"api", "migrator"} {
		p, err := database.Open(t.Context(), "postgres://gfp_"+role+":gfp_ci_only@127.0.0.1:5432/gfp_ci?sslmode=disable")
		if err != nil {
			t.Fatal("disposable public pool failed")
		}
		t.Cleanup(p.Close)
		if _, err = database.Inspect(t.Context(), p, "gfp_"+role, "gfp_ci"); err != nil {
			t.Fatal("disposable public identity failed")
		}
		if role == "api" {
			f.api = p
		} else {
			f.owner = p
		}
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := f.Cleanup(ctx, f.owner); err != nil {
			t.Error(err)
		}
	})
	if err := f.Create(t.Context(), f.owner); err != nil {
		t.Fatal(err)
	}
	// Nil Auth/Identity/Health and no Redis prove anonymous Resource independence.
	Register(f.app, nil, nil, nil, Options{ResourcePool: f.api})
	return f
}
func (f *publicFixture) request(t *testing.T, path string, status int, target any) {
	t.Helper()
	if err := publicreadcheck.Request(f.app, path, status, target); err != nil {
		t.Fatalf("%s: %s", path, err)
	}
}
func (f *publicFixture) sql(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := f.owner.Exec(t.Context(), sql, args...); err != nil {
		t.Fatal(database.SafeError("disposable public fixture mutation", err))
	}
}

func TestIntegrationPublicReadMatrix(t *testing.T) {
	f := newPublicFixture(t)
	if err := f.Verify(f.app); err != nil {
		t.Fatal(err)
	}
	for _, lifecycle := range []string{"active", "inactive", "discontinued", "delisted", "archived", "unknown"} {
		f.sql(t, "UPDATE app.resources SET lifecycle=$2 WHERE id=$1", f.Resources["public"].String(), lifecycle)
		var detail generated.ResourceDetail
		f.request(t, "/resources/"+f.Slug("public"), 200, &detail)
		if string(detail.Lifecycle) != lifecycle {
			t.Fatal("lifecycle controls public visibility")
		}
	}
	var list generated.ResourceList
	f.request(t, "/resources?page=1&page_size=2&locale=JA", 200, &list)
	if !list.HasNext || len(list.Items) != 2 || list.Items[0].Slug != f.Slug("public") || list.Items[1].Slug != f.Slug("other") || list.Items[0].Name != "Requested resource public" {
		t.Fatal("list ordering, locale or lookahead differs")
	}
	f.request(t, "/resources?page=2&page_size=2", 200, &list)
	if list.HasNext || len(list.Items) != 1 || list.Items[0].Slug != f.Slug("retired-category") {
		t.Fatal("final page differs")
	}
	f.request(t, "/resources?page=3&page_size=2", 200, &list)
	if list.HasNext || list.Items == nil || len(list.Items) != 0 {
		t.Fatal("empty page must contain []")
	}
	for _, path := range []string{"/resources?locale=", "/resources?locale=en_US", "/categories?locale=en--US", "/tags?locale=a", "/resources?locale=en&locale=ja", "/resources?page=0", "/resources?page=-1", "/resources?page=abc", "/resources?page=", "/resources?page=9223372036854775808", "/resources?page=9223372036854775807&page_size=100", "/resources?page_size=0", "/resources?page_size=101", "/resources?page_size=abc", "/resources?page_size=", "/resources?page=1&page=2"} {
		var body generated.ApiError
		f.request(t, path, 400, &body)
		if body.Code != generated.VALIDATIONERROR {
			t.Fatal("query error not stable")
		}
	}
	for _, slug := range []string{"UPPER", "a--b", "not-present"} {
		var body generated.ApiError
		f.request(t, "/resources/"+slug, 404, &body)
		if body.Code != generated.RESOURCENOTFOUND {
			t.Fatal("slug privacy differs")
		}
	}
	var raw map[string]any
	f.request(t, "/resources/"+f.Slug("public"), 200, &raw)
	assertKeys(t, raw, []string{"id", "slug", "requested_locale", "default_locale", "available_locales", "name", "summary", "description", "category", "tags", "lifecycle", "content_rating", "sources", "relations", "external_ids", "published_at", "updated_at"})
	for _, source := range raw["sources"].([]any) {
		assertKeys(t, source.(map[string]any), []string{"id", "url", "label", "source_type", "availability_state", "is_primary"})
	}
	for _, relation := range raw["relations"].([]any) {
		assertKeys(t, relation.(map[string]any), []string{"type", "direction", "resource"})
		assertKeys(t, relation.(map[string]any)["resource"].(map[string]any), []string{"id", "slug", "name"})
	}
	raw = nil // json.Unmarshal otherwise retains keys from the preceding detail map.
	f.request(t, "/resources", 200, &raw)
	assertKeys(t, raw, []string{"items", "page", "page_size", "has_next"})
	for _, item := range raw["items"].([]any) {
		assertKeys(t, item.(map[string]any), []string{"id", "slug", "name", "summary", "category", "lifecycle", "content_rating", "published_at", "updated_at"})
	}
	// Only owned fixture rows are mutated, even in disposable tests.
	for _, id := range f.Resources {
		f.sql(t, "UPDATE app.resources SET publication_state='draft' WHERE id=$1", id.String())
	}
	f.request(t, "/resources", 200, &list)
	if list.Items == nil || len(list.Items) != 0 || list.HasNext {
		t.Fatal("empty resource list differs")
	}
}

func TestIntegrationPublicCanonicalCorruption(t *testing.T) {
	f := newPublicFixture(t)
	for _, test := range []struct {
		table, id string
		paths     []string
	}{
		{"resources", f.Resources["public"].String(), []string{"/resources", "/resources/" + f.Slug("public") + "?locale=ja"}},
		{"categories", f.Categories["active"].String(), []string{"/categories?locale=ja", "/resources", "/resources/" + f.Slug("public")}},
		{"tags", f.Tags["active"].String(), []string{"/tags?locale=ja", "/resources/" + f.Slug("public")}},
		{"tags", f.Tags["retired"].String(), []string{"/resources/" + f.Slug("public")}},
		{"resources", f.Resources["other"].String(), []string{"/resources/" + f.Slug("public")}},
	} {
		t.Run(test.table+"-"+test.id, func(t *testing.T) {
			f.sql(t, "UPDATE app."+test.table+" SET default_locale='fr' WHERE id=$1", test.id)
			defer f.sql(t, "UPDATE app."+test.table+" SET default_locale='en' WHERE id=$1", test.id)
			for _, path := range test.paths {
				var body generated.ApiError
				f.request(t, path, 500, &body)
				if body.Code != generated.INTERNALERROR {
					t.Fatal("canonical corruption was hidden")
				}
			}
		})
	}
}

func TestIntegrationPublicFieldFallbackAndStableOrder(t *testing.T) {
	f := newPublicFixture(t)
	id := f.Resources["public"].String()
	var detail generated.ResourceDetail
	f.sql(t, "UPDATE app.resource_localizations SET summary='Requested summary' WHERE resource_id=$1 AND locale='ja'", id)
	f.request(t, "/resources/"+f.Slug("public")+"?locale=ja", 200, &detail)
	if detail.Summary == nil || *detail.Summary != "Requested summary" || detail.Description == nil || *detail.Description != "## Canonical description\n\nA **community** resource." {
		t.Fatal("requested summary disabled default description fallback")
	}
	f.sql(t, "UPDATE app.resource_localizations SET summary=NULL,description='Requested description' WHERE resource_id=$1 AND locale='ja'", id)
	f.request(t, "/resources/"+f.Slug("public")+"?locale=ja", 200, &detail)
	if detail.Summary == nil || *detail.Summary != "Default summary" || detail.Description == nil || *detail.Description != "Requested description" {
		t.Fatal("requested description disabled default summary fallback")
	}
	f.sql(t, "UPDATE app.resource_localizations SET summary=NULL,description=NULL WHERE resource_id=$1", id)
	f.request(t, "/resources/"+f.Slug("public")+"?locale=ja", 200, &detail)
	if detail.Summary != nil || detail.Description != nil {
		t.Fatal("nullable fields were not serialized as null")
	}
	// A missing default on a deleted linked Tag must not reveal or invalidate it.
	f.sql(t, "UPDATE app.tags SET default_locale='fr' WHERE id=$1", f.Tags["deleted"].String())
	f.request(t, "/resources/"+f.Slug("public"), 200, nil)
	f.sql(t, "UPDATE app.resources SET published_at=(SELECT published_at FROM app.resources WHERE id=$1) WHERE id=$2", id, f.Resources["other"].String())
	var list generated.ResourceList
	f.request(t, "/resources?page_size=2", 200, &list)
	if len(list.Items) != 2 || list.Items[0].Id <= list.Items[1].Id {
		t.Fatal("equal publication timestamps require descending UUID tie-break")
	}
}

func TestIntegrationPublicReadDeadline(t *testing.T) {
	f := newPublicFixture(t)
	tx, err := f.owner.Begin(t.Context())
	if err != nil {
		t.Fatal("fixture lock begin failed")
	}
	defer tx.Rollback(t.Context())
	if _, err = tx.Exec(t.Context(), "LOCK TABLE app.resources IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal("fixture lock failed")
	}
	start := time.Now()
	var body generated.ApiError
	f.request(t, "/resources", 500, &body)
	if time.Since(start) < 4*time.Second || time.Since(start) > 6500*time.Millisecond || body.Code != generated.INTERNALERROR {
		t.Fatal("public database read did not respect five-second deadline")
	}
}
