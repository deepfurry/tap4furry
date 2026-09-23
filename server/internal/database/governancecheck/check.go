package governancecheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/deepfurry/tap4furry/server/internal/auth"
	admin "github.com/deepfurry/tap4furry/server/internal/transport/admin/generated"
	public "github.com/deepfurry/tap4furry/server/internal/transport/public/generated"
	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"
	"uuid"
)

type client struct {
	ctx          context.Context
	app          *fiber.App
	cookie       *http.Cookie
	origin, csrf string
}

func (c *client) call(method, path string, body any, status int, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return errors.New("governance fixture request invalid")
	}
	req := httptest.NewRequestWithContext(c.ctx, method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", c.origin)
	if c.cookie != nil {
		req.AddCookie(c.cookie)
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	res, err := c.app.Test(req, fiber.TestConfig{Timeout: 10 * time.Second})
	if err != nil {
		return errors.New("governance fixture HTTP failed")
	}
	defer res.Body.Close()
	if res.StatusCode != status {
		return fmt.Errorf("governance fixture %s %s: expected %d got %d (body withheld)", method, path, status, res.StatusCode)
	}
	if res.Header.Get("Cache-Control") != "no-store" {
		return errors.New("governance response became cacheable")
	}
	if cookies := res.Cookies(); len(cookies) > 0 {
		c.cookie = cookies[0]
	}
	if out != nil && json.NewDecoder(res.Body).Decode(out) != nil {
		return errors.New("governance fixture response invalid")
	}
	return nil
}
func (c *client) token() error {
	var v public.CsrfToken
	if err := c.call("GET", "/auth/csrf", nil, 200, &v); err != nil {
		return err
	}
	c.csrf = v.CsrfToken
	return nil
}

// Run exercises actual handlers and role pools. Its accounts are supplied by the
// caller, all other rows are owned by this fixture and removed before returning.
// It never changes a shared quota clock or reads a real user's private records.
func Run(ctx context.Context, pub, adm *fiber.App, owner *pgxpool.Pool, operator *auth.RoleOperator, authors [7]*http.Cookie, email, password, publicOrigin, adminOrigin string) (result error) {
	if err := VerifyGrants(ctx, owner); err != nil {
		return err
	}
	f := &Fixture{}
	defer func() { result = errors.Join(result, f.Cleanup(owner)) }()
	if _, err := operator.Grant(ctx, email, auth.Administrator); err != nil {
		return err
	}
	a := &client{ctx: ctx, app: adm, origin: adminOrigin}
	anon := &client{ctx: ctx, app: pub}
	if err := a.call("POST", "/auth/login", map[string]string{"email": email, "password": password}, 200, nil); err != nil {
		return err
	}
	if err := a.token(); err != nil {
		return err
	}
	var p [7]*client
	var users [7]public.Me
	for i, cookie := range authors {
		p[i] = &client{ctx: ctx, app: pub, cookie: cookie, origin: publicOrigin}
		if err := p[i].token(); err != nil {
			return err
		}
		if err := p[i].call("GET", "/me", nil, 200, &users[i]); err != nil {
			return err
		}
		f.Users = append(f.Users, users[i].Id)
	}
	staff := func(i int, role auth.Role) (*client, error) {
		if _, err := operator.Grant(ctx, users[i].Email, role); err != nil {
			return nil, err
		}
		c := &client{ctx: ctx, app: adm, origin: adminOrigin}
		if err := c.call("POST", "/auth/login", map[string]string{"email": users[i].Email, "password": password}, 200, nil); err != nil {
			return nil, err
		}
		return c, c.token()
	}
	mod, err := staff(5, auth.Moderator)
	if err != nil {
		return err
	}
	editor, err := staff(6, auth.Editor)
	if err != nil {
		return err
	}
	for _, c := range []*client{mod, editor} {
		if err = c.call("GET", "/audit", nil, 403, nil); err != nil {
			return err
		}
		if err = c.call("GET", "/users/"+users[3].Id+"/governance", nil, 403, nil); err != nil {
			return err
		}
	}
	if err = editor.call("GET", "/reports", nil, 403, nil); err != nil {
		return err
	}
	slug := "governance-" + uuid.NewV7().String()
	var cat admin.EntityID
	if err = a.call("POST", "/categories", map[string]any{"slug": slug, "default_locale": "en", "localization": map[string]string{"name": "Governance fixture"}}, 201, &cat); err != nil {
		return err
	}
	f.Graph.Categories = append(f.Graph.Categories, cat.Id)
	var rev admin.ResourceRevision
	if err = a.call("POST", "/resources", map[string]any{"slug": slug, "default_locale": "en", "category_id": cat.Id, "content_rating": "general", "lifecycle": "active", "localization": map[string]string{"name": "Governance fixture"}}, 201, &rev); err != nil {
		return err
	}
	f.Graph.Resources = append(f.Graph.Resources, rev.Id)
	base := "/resources/" + rev.Id
	mutate := func(c *client, method, suffix string, body any) error {
		return c.call(method, fmt.Sprintf("%s%s?expected_version=%d", base, suffix, rev.Version), body, 200, &rev)
	}
	if err = mutate(editor, "POST", "/sources", map[string]any{"url": "https://example.invalid/manual-only", "source_type": "official", "availability_state": "active", "is_primary": true}); err != nil {
		return err
	}
	var detail admin.ResourceDetail
	if err = a.call("GET", base, nil, 200, &detail); err != nil {
		return err
	}
	if len(detail.Sources) != 1 {
		return errors.New("fixture Source missing")
	}
	sid := detail.Sources[0].Id
	if err = mutate(a, "PUT", "/sources/"+sid+"/rights", map[string]string{"rights_status": "confirmed", "reason": "Fixture rights review"}); err != nil {
		return err
	}
	if err = mutate(editor, "PUT", "/publication", map[string]string{"state": "published"}); err != nil {
		return err
	}
	publicPath := "/resources/" + slug
	if err = anon.call("GET", publicPath, nil, 200, nil); err != nil {
		return err
	}
	report := func(c *client, reason string, source bool) (string, error) {
		body := map[string]any{"request_id": uuid.NewV7().String(), "resource_id": rev.Id, "target_kind": "resource", "reason": reason, "body": "User supplied observation"}
		if source {
			body["target_kind"] = "source"
			body["source_id"] = sid
		}
		var out, replay public.RequestReceipt
		if e := c.call("POST", "/reports", body, 200, &out); e != nil {
			return "", e
		}
		f.Reports = append(f.Reports, out.Id)
		if e := c.call("POST", "/reports", body, 200, &replay); e != nil {
			return "", e
		}
		if replay.Id != out.Id {
			return "", errors.New("report replay duplicated")
		}
		return out.Id, nil
	}
	rid, err := report(p[0], "broken_link", true)
	if err != nil {
		return err
	}
	if err = p[1].call("GET", "/me/reports/"+rid, nil, 404, nil); err != nil {
		return err
	}
	if err = mod.call("POST", "/reports/"+rid+"/triage", map[string]any{"request_id": uuid.NewV7().String(), "expected_report_version": 1, "action": "receive", "internal_note": "Internal evidence"}, 200, nil); err != nil {
		return err
	}
	body := map[string]any{"request_id": uuid.NewV7().String(), "expected_report_version": 2, "expected_resource_version": rev.Version, "mode": "source_availability", "availability_state": "removed", "reason": "Manual broken link review", "safe_message": "The link has been removed from display.", "internal_note": "Private staff observation"}
	if err = mod.call("POST", "/reports/"+rid+"/resolve", body, 403, nil); err != nil {
		return err
	}
	if err = a.call("POST", "/reports/"+rid+"/resolve", body, 200, nil); err != nil {
		return err
	}
	if err = a.call("POST", "/reports/"+rid+"/resolve", body, 200, nil); err != nil {
		return err
	}
	rev.Version++
	var own public.OwnReport
	if err = p[0].call("GET", "/me/reports/"+rid, nil, 200, &own); err != nil {
		return err
	}
	encoded, _ := json.Marshal(own)
	if own.Status != "resolved" || own.Target != nil || strings.Contains(string(encoded), "Private staff") || len(own.Events) != 3 {
		return errors.New("report safe projection failed")
	}
	var resource public.ResourceDetail
	if err = anon.call("GET", publicPath, nil, 200, &resource); err != nil {
		return err
	}
	if len(resource.Sources) != 0 {
		return errors.New("removed source leaked")
	}
	ordinary, err := report(p[1], "other", false)
	if err != nil {
		return err
	}
	if err = mod.call("POST", "/reports/"+ordinary+"/dismiss", map[string]any{"request_id": uuid.NewV7().String(), "expected_report_version": 1, "safe_message": "Unable to reproduce the issue."}, 200, nil); err != nil {
		return err
	}
	sensitive, err := report(p[2], "privacy", false)
	if err != nil {
		return err
	}
	resolution := map[string]any{"request_id": uuid.NewV7().String(), "expected_report_version": 1, "safe_message": "Reviewed; no change is required.", "mode": "no_change"}
	if err = mod.call("POST", "/reports/"+sensitive+"/resolve", resolution, 403, nil); err != nil {
		return err
	}
	if err = a.call("POST", "/reports/"+sensitive+"/resolve", resolution, 200, nil); err != nil {
		return err
	}
	check := map[string]any{"request_id": uuid.NewV7().String(), "expected_version": rev.Version, "outcome": "reachable", "observed_at": time.Now().UTC().Add(-time.Minute), "note": "Manually reviewed; no network fetch by application"}
	if err = mod.call("POST", base+"/sources/"+sid+"/checks", check, 200, nil); err != nil {
		return err
	}
	if err = mod.call("POST", base+"/sources/"+sid+"/checks", check, 200, nil); err != nil {
		return err
	}
	if err = a.call("GET", base, nil, 200, &detail); err != nil {
		return err
	}
	if detail.Version != rev.Version {
		return errors.New("observation bumped canonical revision")
	}
	check["request_id"] = uuid.NewV7().String()
	check["availability_state"] = "active"
	if err = mod.call("POST", base+"/sources/"+sid+"/checks", check, 403, nil); err != nil {
		return err
	}
	if err = editor.call("POST", base+"/sources/"+sid+"/checks", check, 200, nil); err != nil {
		return err
	}
	rev.Version++
	if err = mutate(a, "PUT", "/sources/"+sid+"/rights", map[string]string{"rights_status": "disputed", "reason": "Fixture rights hold"}); err != nil {
		return err
	}
	if err = anon.call("GET", publicPath, nil, 200, &resource); err != nil {
		return err
	}
	if len(resource.Sources) != 0 {
		return errors.New("reachable observation bypassed rights")
	}
	userPath := "/users/" + users[3].Id
	restrict := map[string]any{"request_id": uuid.NewV7().String(), "expected_revision": 0, "scope": "all_write", "duration": "24h", "reason_code": "other", "user_message": "Temporary fixture restriction", "internal_note": "Private restriction evidence"}
	if err = a.call("POST", userPath+"/restrictions", restrict, 200, nil); err != nil {
		return err
	}
	if err = p[3].call("PATCH", "/me/profile", map[string]string{"display_name": "Blocked"}, 403, nil); err != nil {
		return err
	}
	if err = p[3].call("PATCH", "/me/profile", map[string]bool{"search_engine_indexing": false}, 200, nil); err != nil {
		return err
	}
	submit := map[string]any{"kind": "create_resource", "request_id": uuid.NewV7().String(), "reason": "Fixture", "content": map[string]any{"name": "Blocked", "default_locale": "en", "category_id": cat.Id, "content_rating": "general"}}
	if err = p[3].call("POST", "/contributions", submit, 403, nil); err != nil {
		return err
	}
	for _, path := range []string{"/me", "/me/sessions", "/me/contributions", "/me/reports", "/auth/csrf"} {
		if err = p[3].call("GET", path, nil, 200, nil); err != nil {
			return err
		}
	}
	allowed, err := report(p[3], "spam", false)
	if err != nil {
		return err
	}
	if err = p[3].call("POST", "/me/reports/"+allowed+"/withdraw", map[string]string{"request_id": uuid.NewV7().String()}, 200, nil); err != nil {
		return err
	}
	if err = a.call("PUT", userPath+"/trust", map[string]any{"request_id": uuid.NewV7().String(), "expected_revision": 1, "trust_level": "established", "reason": "Reviewed contribution history"}, 200, nil); err != nil {
		return err
	}
	var status public.OwnGovernance
	if err = p[3].call("GET", "/me/governance", nil, 200, &status); err != nil {
		return err
	}
	if status.ContributionQuota.DailyLimit != 30 || status.ContributionQuota.PendingLimit != 10 || status.ReportQuota.DailyLimit != 10 || len(status.Restrictions) != 1 {
		return errors.New("fixed trust quota or restriction projection failed")
	}
	if err = a.call("POST", userPath+"/restrictions/"+status.Restrictions[0].Id+"/revoke", map[string]any{"request_id": uuid.NewV7().String(), "expected_revision": 2, "reason": "Fixture restriction concluded"}, 200, nil); err != nil {
		return err
	}
	if err = p[3].call("PATCH", "/me/profile", map[string]string{"display_name": "Allowed after revoke"}, 200, nil); err != nil {
		return err
	}
	if err = mutate(a, "PUT", "/sources/"+sid+"/rights", map[string]string{"rights_status": "confirmed", "reason": "Fixture rights review concluded"}); err != nil {
		return err
	}
	distribution := map[string]any{"request_id": uuid.NewV7().String(), "expected_version": rev.Version, "policy": "excluded", "reason": "Fixture recommendation exclusion"}
	if err = editor.call("PUT", base+"/distribution", distribution, 403, nil); err != nil {
		return err
	}
	if err = a.call("PUT", base+"/distribution", distribution, 200, nil); err != nil {
		return err
	}
	if err = a.call("PUT", base+"/distribution", distribution, 200, nil); err != nil {
		return err
	}
	rev.Version++
	if err = anon.call("GET", publicPath, nil, 200, nil); err != nil {
		return err
	}
	if err = mutate(a, "PUT", "/publication", map[string]string{"state": "restricted", "reason": "Fixture publication hold"}); err != nil {
		return err
	}
	if err = anon.call("GET", publicPath, nil, 404, nil); err != nil {
		return err
	}
	if err = p[1].call("GET", "/me/reports/"+ordinary, nil, 200, &own); err != nil {
		return err
	}
	if own.Target != nil {
		return errors.New("hidden Resource linked from report history")
	}
	var audit admin.AuditList
	if err = a.call("GET", "/audit?resource_id="+rev.Id, nil, 200, &audit); err != nil {
		return err
	}
	if len(audit.Items) < 6 {
		return errors.New("canonical governance audit incomplete")
	}
	var version int64
	if err = owner.QueryRow(ctx, "SELECT max(version_id) FROM app.goose_db_version WHERE is_applied").Scan(&version); err != nil || version != 9 {
		return errors.New("governance smoke requires Goose 9")
	}
	return nil
}
