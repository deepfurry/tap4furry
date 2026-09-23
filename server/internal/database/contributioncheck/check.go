// Package contributioncheck contains developer-only acceptance fixtures, never runtime dependencies.
package contributioncheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/deepfurry/tap4furry/server/internal/database/curationcheck"
	admin "github.com/deepfurry/tap4furry/server/internal/transport/admin/generated"
	public "github.com/deepfurry/tap4furry/server/internal/transport/public/generated"
	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"time"
	"uuid"
)

type Fixture struct {
	IDs   []string
	Graph curationcheck.Fixture
}

func (f *Fixture) Cleanup(owner *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := owner.Begin(ctx)
	if err != nil {
		return database.SafeError("begin contribution fixture cleanup", err)
	}
	defer tx.Rollback(ctx)
	for _, table := range []string{"contribution_review_audits", "contribution_events", "contribution_initial_sources", "contribution_contents", "contributions"} {
		key := "contribution_id"
		if table == "contributions" {
			key = "id"
		}
		if _, err = tx.Exec(ctx, "DELETE FROM app."+table+" WHERE "+key+"=ANY($1::uuid[])", f.IDs); err != nil {
			return database.SafeError("remove contribution fixture", err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return database.SafeError("commit contribution fixture cleanup", err)
	}
	return f.Graph.Cleanup(owner)
}

type client struct {
	ctx          context.Context
	app          *fiber.App
	cookie       *http.Cookie
	origin, csrf string
}

func (c *client) call(method, path string, body any, status int, out any) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return errors.New("contribution fixture request invalid")
	}
	req := httptest.NewRequestWithContext(c.ctx, method, path, bytes.NewReader(encoded))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", c.origin)
	if c.cookie != nil {
		req.AddCookie(c.cookie)
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	response, err := c.app.Test(req, fiber.TestConfig{Timeout: 10 * time.Second})
	if err != nil {
		return errors.New("contribution fixture HTTP unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != status {
		return fmt.Errorf("contribution fixture %s expected %d got %d (path/body withheld)", method, status, response.StatusCode)
	}
	if c.origin != "" && response.Header.Get("Cache-Control") != "no-store" {
		return errors.New("private contribution response cacheable")
	}
	if cookies := response.Cookies(); len(cookies) > 0 {
		c.cookie = cookies[0]
	}
	if out != nil && json.NewDecoder(response.Body).Decode(out) != nil {
		return errors.New("contribution fixture response invalid")
	}
	return nil
}
func (c *client) token() error {
	var response public.CsrfToken
	if err := c.call("GET", "/auth/csrf", nil, 200, &response); err != nil {
		return err
	}
	c.csrf = response.CsrfToken
	return nil
}

// Callers own two temporary verified Public accounts and an Editorial reviewer.
// No quota bypass, clock manipulation, secret logging or shared fixture reuse.
func Run(ctx context.Context, pub, adm *fiber.App, owner *pgxpool.Pool, authors [2]*http.Cookie, reviewerEmail, password, publicOrigin, adminOrigin string) (result error) {
	if err := VerifyGrants(ctx, owner); err != nil {
		return err
	}
	f := &Fixture{}
	defer func() { result = errors.Join(result, f.Cleanup(owner)) }()
	p := &client{ctx: ctx, app: pub, cookie: authors[0], origin: publicOrigin}
	p2 := &client{ctx: ctx, app: pub, cookie: authors[1], origin: publicOrigin}
	a := &client{ctx: ctx, app: adm, origin: adminOrigin}
	anon := &client{ctx: ctx, app: pub}
	if err := a.call("POST", "/auth/login", map[string]string{"email": reviewerEmail, "password": password}, 200, nil); err != nil {
		return err
	}
	for _, c := range []*client{p, p2, a} {
		if err := c.token(); err != nil {
			return err
		}
	}
	prefix := "contribution-" + uuid.NewV7().String()
	var cat admin.EntityID
	if err := a.call("POST", "/categories", map[string]any{"slug": prefix, "default_locale": "en", "localization": map[string]string{"name": "Contribution fixture"}}, 201, &cat); err != nil {
		return err
	}
	f.Graph.Categories = append(f.Graph.Categories, cat.Id)
	content := map[string]any{"name": "Original proposal", "default_locale": "en", "category_id": cat.Id, "lifecycle": "unknown", "content_rating": "general", "summary": "Proposed summary", "description": "Plain **Markdown** <script>inert</script>", "source": map[string]string{"url": "HTTPS://EXAMPLE.INVALID:443/contribution#fragment", "source_type": "official"}}
	body := map[string]any{"kind": "create_resource", "request_id": uuid.NewV7().String(), "reason": "Fixture evidence", "content": content}
	var created public.ContributionCreated
	if err := p.call("POST", "/contributions", body, 201, &created); err != nil {
		return err
	}
	f.IDs = append(f.IDs, created.Id)
	var replay public.ContributionCreated
	if err := p.call("POST", "/contributions", body, 201, &replay); err != nil {
		return err
	}
	if replay.Id != created.Id {
		return errors.New("idempotent replay created duplicate")
	}
	var review admin.ContributionDetail
	if err := a.call("GET", "/contributions/"+created.Id, nil, 200, &review); err != nil {
		return err
	}
	review.Proposed.Name = "Reviewed resource"
	review.Proposed.Slug = &prefix
	if err := a.call("POST", "/contributions/"+created.Id+"/accept", map[string]any{"content": review.Proposed, "message": "Corrected name", "internal_note": "Private fixture note"}, 204, nil); err != nil {
		return err
	}
	if err := a.call("GET", "/contributions/"+created.Id, nil, 200, &review); err != nil {
		return err
	}
	if review.ResultResourceId == nil {
		return errors.New("accepted Resource reference missing")
	}
	rid := *review.ResultResourceId
	f.Graph.Resources = append(f.Graph.Resources, rid)
	if err := anon.call("GET", "/resources/"+prefix, nil, 404, nil); err != nil {
		return err
	}
	var own public.ContributionDetail
	if err := p.call("GET", "/me/contributions/"+created.Id, nil, 200, &own); err != nil {
		return err
	}
	if own.Proposed.Name == nil || *own.Proposed.Name != "Original proposal" || own.Accepted != nil || own.Result != nil || len(own.History) != 2 {
		return errors.New("original/privacy/draft acceptance contract failed")
	}
	if err := p2.call("GET", "/me/contributions/"+created.Id, nil, 404, nil); err != nil {
		return err
	}
	if err := a.call("PUT", "/resources/"+rid+"/publication?expected_version=1", map[string]string{"state": "published"}, 200, nil); err != nil {
		return err
	}
	if err := anon.call("GET", "/resources/"+prefix, nil, 200, nil); err != nil {
		return err
	}
	var edit public.ContributionContext
	if err := p2.call("GET", "/contributions/context/"+prefix, nil, 200, &edit); err != nil {
		return err
	}
	body = map[string]any{"kind": "update_resource", "target_resource_id": rid, "base_revision": edit.BaseRevision, "request_id": uuid.NewV7().String(), "reason": "Clear outdated summary", "content": map[string]any{"summary": nil}}
	if err := p2.call("POST", "/contributions", body, 201, &created); err != nil {
		return err
	}
	f.IDs = append(f.IDs, created.Id)
	review = admin.ContributionDetail{}
	if err := a.call("GET", "/contributions/"+created.Id, nil, 200, &review); err != nil {
		return err
	}
	if err := a.call("POST", "/contributions/"+created.Id+"/accept", map[string]any{"content": review.Proposed}, 204, nil); err != nil {
		return err
	}
	var resource public.ResourceDetail
	if err := anon.call("GET", "/resources/"+prefix, nil, 200, &resource); err != nil {
		return err
	}
	if resource.Summary != nil || resource.Name != "Reviewed resource" {
		return errors.New("accepted correction did not preserve omitted fields")
	}
	if err := a.call("POST", "/contributions/"+created.Id+"/accept", map[string]any{"content": review.Proposed}, 409, nil); err != nil {
		return err
	}
	if err := p2.call("POST", "/me/contributions/"+created.Id+"/withdraw", nil, 409, nil); err != nil {
		return err
	}
	var version, audits int
	if err := owner.QueryRow(ctx, "SELECT version FROM app.resources WHERE id=$1", rid).Scan(&version); err != nil || version != 3 {
		return errors.New("review did not bump once")
	}
	if err := owner.QueryRow(ctx, "SELECT count(*) FROM app.contribution_review_audits WHERE contribution_id=ANY($1::uuid[])", f.IDs).Scan(&audits); err != nil || audits != 2 {
		return errors.New("atomic review audit missing")
	}
	return nil
}
