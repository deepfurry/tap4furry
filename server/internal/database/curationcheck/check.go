// Package curationcheck is a developer-only HTTP acceptance fixture shared by
// disposable tests and admin-smoke. Runtime binaries must never import it.
package curationcheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/deepfurry/tap4furry/server/internal/transport/admin/generated"
	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"time"
	"uuid"
)

type Fixture struct{ Resources, Categories, Tags []string }

func (f *Fixture) Cleanup(pool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return database.SafeError("begin curation fixture cleanup", err)
	}
	defer tx.Rollback(ctx)
	for _, table := range []string{"resource_relations", "resource_external_ids", "resource_sources", "resource_tags", "resource_localizations"} {
		where := "resource_id=ANY($1::uuid[])"
		if table == "resource_relations" {
			where = "source_resource_id=ANY($1::uuid[]) OR target_resource_id=ANY($1::uuid[])"
		}
		if _, err = tx.Exec(ctx, "DELETE FROM app."+table+" WHERE "+where, f.Resources); err != nil {
			return database.SafeError("remove curation fixture children", err)
		}
	}
	for _, item := range []struct {
		table, key string
		ids        []string
	}{{"resources", "id", f.Resources}, {"category_localizations", "category_id", f.Categories}, {"categories", "id", f.Categories}, {"tag_localizations", "tag_id", f.Tags}, {"tags", "id", f.Tags}} {
		if _, err = tx.Exec(ctx, "DELETE FROM app."+item.table+" WHERE "+item.key+"=ANY($1::uuid[])", item.ids); err != nil {
			return database.SafeError("remove curation fixture parents", err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return database.SafeError("commit curation fixture cleanup", err)
	}
	return nil
}

type client struct {
	ctx    context.Context
	app    *fiber.App
	origin string
	cookie *http.Cookie
	csrf   string
}

func (c *client) call(method, path string, body any, status int, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return errors.New("curation fixture input invalid")
	}
	req := httptest.NewRequestWithContext(c.ctx, method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", c.origin)
	if c.cookie != nil {
		req.AddCookie(c.cookie)
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	response, err := c.app.Test(req, fiber.TestConfig{Timeout: 10 * time.Second})
	if err != nil {
		return errors.New("curation fixture HTTP failed (details withheld)")
	}
	defer response.Body.Close()
	if response.StatusCode != status {
		return fmt.Errorf("curation fixture %s expected %d got %d (path/body withheld)", method, status, response.StatusCode)
	}
	if c.origin != "" && response.Header.Get("Cache-Control") != "no-store" {
		return errors.New("Admin curation response became cacheable")
	}
	if cookies := response.Cookies(); len(cookies) > 0 {
		c.cookie = cookies[0]
	}
	if out != nil && json.NewDecoder(response.Body).Decode(out) != nil {
		return errors.New("curation fixture response invalid")
	}
	return nil
}

// Run uses an existing temporary verified local account. Only its roles and the
// returned fixture IDs are changed; the owning caller cleans up the account.
func Run(ctx context.Context, admin, public *fiber.App, owner *pgxpool.Pool, operator *auth.RoleOperator, email, password, origin string) (result error) {
	f := &Fixture{}
	defer func() { result = errors.Join(result, f.Cleanup(owner)) }()
	if _, err := operator.Grant(ctx, email, auth.Moderator); err != nil {
		return err
	}
	c := &client{ctx: ctx, app: admin, origin: origin}
	p := &client{ctx: ctx, app: public}
	if err := c.call("POST", "/auth/login", map[string]string{"email": email, "password": password}, 200, nil); err != nil {
		return err
	}
	var csrf generated.CsrfToken
	if err := c.call("GET", "/auth/csrf", nil, 200, &csrf); err != nil {
		return err
	}
	c.csrf = csrf.CsrfToken
	var list generated.ResourceList
	if err := c.call("GET", "/resources", nil, 200, &list); err != nil {
		return err
	}
	prefix := "curation-" + uuid.NewV7().String()
	categoryBody := map[string]any{"slug": prefix, "default_locale": "en", "localization": map[string]any{"name": "Curation fixture"}}
	if err := c.call("POST", "/categories", categoryBody, 403, nil); err != nil {
		return err
	}
	if _, err := operator.Grant(ctx, email, auth.Editor); err != nil {
		return err
	}
	var cat, tag generated.EntityID
	if err := c.call("POST", "/categories", categoryBody, 201, &cat); err != nil {
		return err
	}
	f.Categories = append(f.Categories, cat.Id)
	if err := c.call("POST", "/tags", categoryBody, 201, &tag); err != nil {
		return err
	}
	f.Tags = append(f.Tags, tag.Id)
	var rev, other generated.ResourceRevision
	create := func(slug string, out *generated.ResourceRevision) error {
		return c.call("POST", "/resources", map[string]any{"slug": slug, "default_locale": "en", "category_id": cat.Id, "content_rating": "general", "localization": map[string]any{"name": "Fixture resource", "description": "Plain **Markdown**"}}, 201, out)
	}
	if err := create(prefix, &rev); err != nil {
		return err
	}
	f.Resources = append(f.Resources, rev.Id)
	if rev.Version != 1 {
		return errors.New("new Resource revision must be one")
	}
	if err := create(prefix+"-target", &other); err != nil {
		return err
	}
	f.Resources = append(f.Resources, other.Id)
	base := "/resources/" + rev.Id
	mutate := func(method, suffix string, body any, status int) error {
		path := fmt.Sprintf("%s%s?expected_version=%d", base, suffix, rev.Version)
		if status == 200 {
			var next generated.ResourceRevision
			if err := c.call(method, path, body, status, &next); err != nil {
				return err
			}
			rev = next
			return nil
		}
		return c.call(method, path, body, status, nil)
	}
	visible := func(status int) error { return p.call("GET", "/resources/"+prefix, nil, status, nil) }
	if err := visible(404); err != nil {
		return err
	}
	if err := mutate("PUT", "/localizations/zh-hans", map[string]any{"name": "资源", "summary": "", "description": nil}, 200); err != nil {
		return err
	}
	if err := mutate("PUT", "/tags", map[string]any{"tag_ids": []string{tag.Id, tag.Id}}, 200); err != nil {
		return err
	}
	source := map[string]any{"url": "https://example.invalid/curation#fragment", "label": "Official", "source_type": "official", "availability_state": "active", "is_primary": true}
	if err := mutate("POST", "/sources", source, 200); err != nil {
		return err
	}
	var detail generated.ResourceDetail
	if err := c.call("GET", base, nil, 200, &detail); err != nil {
		return err
	}
	if len(detail.Sources) != 1 || detail.Sources[0].Url != "https://example.invalid/curation" {
		return errors.New("Source normalization failed")
	}
	sid := detail.Sources[0].Id
	if err := mutate("POST", "/relations", map[string]string{"target_resource_id": other.Id, "relation_type": "successor_of"}, 200); err != nil {
		return err
	}
	if err := mutate("PUT", "/external-ids", map[string]any{"items": []any{map[string]string{"namespace": "fixture", "external_id": prefix}}}, 200); err != nil {
		return err
	}
	if err := mutate("PUT", "/publication", map[string]string{"state": "published"}, 200); err != nil {
		return err
	}
	if err := visible(200); err != nil {
		return err
	}
	for _, state := range []string{"restricted", "removed"} {
		if err := mutate("PUT", "/publication", map[string]string{"state": state}, 403); err != nil {
			return err
		}
	}
	if err := mutate("PUT", "/sources/"+sid+"/rights", map[string]string{"rights_status": "confirmed"}, 403); err != nil {
		return err
	}
	if err := mutate("DELETE", "", nil, 403); err != nil {
		return err
	}
	if err := c.call("PATCH", "/categories/"+cat.Id, map[string]string{"state": "retired"}, 403, nil); err != nil {
		return err
	}
	if _, err := operator.Grant(ctx, email, auth.Administrator); err != nil {
		return err
	}
	if err := mutate("PUT", "/sources/"+sid+"/rights", map[string]string{"rights_status": "confirmed"}, 200); err != nil {
		return err
	}
	for _, state := range []string{"restricted", "published", "removed", "published"} {
		if err := mutate("PUT", "/publication", map[string]string{"state": state}, 200); err != nil {
			return err
		}
		status := 404
		if state == "published" {
			status = 200
		}
		if err := visible(status); err != nil {
			return err
		}
	}
	if err := c.call("PATCH", "/categories/"+cat.Id, map[string]string{"state": "retired"}, 200, nil); err != nil {
		return err
	}
	if err := c.call("PATCH", "/tags/"+tag.Id, map[string]string{"state": "retired"}, 200, nil); err != nil {
		return err
	}
	if err := c.call("DELETE", "/categories/"+cat.Id, nil, 409, nil); err != nil {
		return err
	}
	if err := c.call("DELETE", "/tags/"+tag.Id, nil, 409, nil); err != nil {
		return err
	}
	if err := mutate("DELETE", "", nil, 200); err != nil {
		return err
	}
	if err := visible(404); err != nil {
		return err
	}
	if err := c.call("GET", base, nil, 404, nil); err != nil {
		return err
	}
	if err := c.call("GET", "/resources/"+other.Id, nil, 200, &detail); err != nil {
		return err
	}
	if err := c.call("DELETE", fmt.Sprintf("/resources/%s?expected_version=%d", other.Id, detail.Version), nil, 200, nil); err != nil {
		return err
	}
	if err := c.call("DELETE", "/categories/"+cat.Id, nil, 204, nil); err != nil {
		return err
	}
	if err := c.call("DELETE", "/tags/"+tag.Id, nil, 204, nil); err != nil {
		return err
	}
	var version int
	if err := owner.QueryRow(ctx, "SELECT version_id FROM app.goose_db_version ORDER BY id DESC LIMIT 1").Scan(&version); err != nil || version != 7 {
		return errors.New("Goose version preservation failed")
	}
	return nil
}
