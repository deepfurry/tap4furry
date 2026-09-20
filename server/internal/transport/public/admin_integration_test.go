package public

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"uuid"

	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/config"
	"github.com/deepfurry/tap4furry/server/internal/curation"
	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/deepfurry/tap4furry/server/internal/mail"
	"github.com/deepfurry/tap4furry/server/internal/redisstore"
	"github.com/deepfurry/tap4furry/server/internal/transport/admin"
	"github.com/deepfurry/tap4furry/server/internal/transport/health"
	"github.com/gofiber/fiber/v3"
	"github.com/gofurry/easyhash"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const adminOrigin = "http://localhost:5173"

func (f *adminFixture) scalar(query string, args ...any) int {
	f.t.Helper()
	var n int
	if err := f.owner.QueryRow(f.t.Context(), query, args...).Scan(&n); err != nil {
		f.t.Fatal(database.SafeError("Admin fixture assertion", err))
	}
	return n
}

type authQueryGate struct{ next atomic.Pointer[queryPause] }
type queryPause struct {
	entered, resume chan struct{}
	query           string
}

func (g *authQueryGate) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if pause := g.next.Load(); pause != nil && strings.Contains(data.SQL, pause.query) {
		if g.next.CompareAndSwap(pause, nil) {
			close(pause.entered)
			select {
			case <-pause.resume:
			case <-ctx.Done():
			}
		}
	}
	return ctx
}
func (*authQueryGate) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}
func (g *authQueryGate) arm(t *testing.T) *queryPause {
	return g.armQuery(t, "-- name: LockUserAuthState")
}
func (g *authQueryGate) armQuery(t *testing.T, query string) *queryPause {
	pause := &queryPause{entered: make(chan struct{}), resume: make(chan struct{}), query: query}
	g.next.Store(pause)
	t.Cleanup(func() {
		select {
		case <-pause.resume:
		default:
			close(pause.resume)
		}
	})
	return pause
}

type adminFixture struct {
	*fixture
	adminApp  *fiber.App
	adminAuth *auth.App
	operator  *auth.RoleOperator
	store     *redisstore.Store
	throttle  *redisstore.AuthThrottle
	gate      *authQueryGate
	curation  *curation.App
	adminPool *pgxpool.Pool
}

func newAdminFixture(t *testing.T) *adminFixture {
	f := newFixture(t)
	cfg, err := pgxpool.ParseConfig("postgres://gfp_admin:gfp_ci_only@127.0.0.1:5432/gfp_ci?sslmode=disable")
	if err != nil {
		t.Fatal("Admin pool configuration failed")
	}
	gate := &authQueryGate{}
	cfg.ConnConfig.Tracer = gate
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal("Admin pool failed")
	}
	t.Cleanup(pool.Close)
	if _, err = database.Inspect(t.Context(), pool, "gfp_admin", "gfp_ci"); err != nil {
		t.Fatal("Admin role identity failed")
	}
	store, err := redisstore.Open("redis://gfp_runtime:gfp_ci_only@127.0.0.1:6379/0", "gfp:")
	if err != nil {
		t.Fatal("throttle fixture failed")
	}
	t.Cleanup(func() { _ = store.Close() })
	throttle, err := store.AuthThrottle(uuid.NewV7().String())
	if err != nil {
		t.Fatal("throttle secret fixture failed")
	}
	aa, err := auth.NewWithThrottle(pool, mail.Disabled{}, throttle)
	if err != nil {
		t.Fatal("Admin auth fixture failed")
	}
	f.auth, err = auth.NewWithThrottle(f.api, f.mail, throttle)
	if err != nil {
		t.Fatal("Public throttle fixture failed")
	}
	f.app = fiber.New()
	Register(f.app, nil, f.auth, f.identity, Options{Environment: "test", PublicOrigin: testOrigin, CSRFSecret: config.DevelopmentCSRFSecret, ResourcePool: f.api})
	app := fiber.New(fiber.Config{BodyLimit: 256 * 1024})
	curator := curation.New(pool)
	admin.Register(app, health.New(func(context.Context) error { return nil }, store.Ping), admin.Options{Auth: aa, Curation: curator, ResourcePool: pool, Environment: "test", AdminOrigin: adminOrigin, CSRFSecret: config.DevelopmentAdminCSRFSecret})
	return &adminFixture{fixture: f, adminApp: app, adminAuth: aa, operator: auth.NewRoleOperator(f.owner), store: store, throttle: throttle, gate: gate, curation: curator, adminPool: pool}
}
func (f *adminFixture) eligible(role auth.Role) (string, string, *http.Cookie) {
	id, email, cookie := f.register()
	f.request("POST", "/auth/email/verification", map[string]string{"token": f.lastToken("email_verify")}, nil, 204)
	if _, err := f.operator.Grant(f.t.Context(), email, role); err != nil {
		f.t.Fatal("operator grant failed")
	}
	// Disposable fixture owns exactly this User's roles, including a last-admin fixture.
	f.t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := f.owner.Exec(ctx, "DELETE FROM app.user_roles WHERE user_id=$1", id); err != nil {
			f.t.Error("fixture role cleanup failed")
		}
	})
	return id, email, cookie
}
func adminCSRF(cookie *http.Cookie) string {
	mac := hmac.New(sha256.New, []byte(config.DevelopmentAdminCSRFSecret))
	mac.Write([]byte(cookie.Value))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (f *adminFixture) adminRequest(method, path string, body any, cookie *http.Cookie, status int, override ...map[string]string) (map[string]any, *http.Cookie) {
	f.t.Helper()
	encoded, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	req.Header.Set("Origin", adminOrigin)
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
		req.Header.Set("X-CSRF-Token", adminCSRF(cookie))
	}
	for _, headers := range override {
		for key, value := range headers {
			req.Header.Set(key, value)
		}
	}
	response, err := f.adminApp.Test(req, fiber.TestConfig{Timeout: 10 * time.Second})
	if err != nil {
		f.t.Fatal("Admin HTTP request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != status {
		f.t.Fatalf("Admin %s %s: status %d expected %d (body withheld)", method, path, response.StatusCode, status)
	}
	if response.Header.Get("Cache-Control") != "no-store" {
		f.t.Fatal("Admin private response cacheable")
	}
	var result map[string]any
	if status != 204 && json.NewDecoder(response.Body).Decode(&result) != nil {
		f.t.Fatal("invalid Admin JSON")
	}
	var next *http.Cookie
	if cookies := response.Cookies(); len(cookies) > 0 {
		next = cookies[0]
	}
	return result, next
}
func (f *adminFixture) adminLogin(email string) (map[string]any, *http.Cookie) {
	return f.adminRequest("POST", "/auth/login", map[string]string{"email": email, "password": testPassword}, nil, 200)
}

func TestIntegrationAdminRolesAndEligibility(t *testing.T) {
	f := newAdminFixture(t)
	for _, role := range []auth.Role{auth.Moderator, auth.Editor, auth.Administrator} {
		_, email, _ := f.eligible(role)
		me, cookie := f.adminLogin(email)
		assertKeys(t, me, []string{"id", "email", "roles", "authenticated_at"})
		if cookie == nil || cookie.Name != "tap4furry_admin_session" || !cookie.HttpOnly || cookie.Secure || cookie.Path != "/" || cookie.Domain != "" || cookie.SameSite != http.SameSiteStrictMode {
			t.Fatal("Admin cookie contract failed")
		}
		if !reflect.DeepEqual(me["roles"], []any{string(role)}) {
			t.Fatal("Admin role response incorrect")
		}
		f.adminRequest("GET", "/me", nil, cookie, 200)
	}
	var expected map[string]any
	for _, kind := range []string{"unknown", "wrong", "normal", "unverified", "disabled", "deleted", "oauth-only"} {
		email := emailFixture()
		password := testPassword
		if kind != "unknown" {
			var id, created string
			if kind == "normal" {
				id, created, _ = f.register()
			} else {
				id, created, _ = f.eligible(auth.Moderator)
			}
			email = created
			switch kind {
			case "wrong":
				password = "a deliberately wrong password"
			case "normal":
				f.exec("UPDATE app.auth_identities SET verified_at=now() WHERE user_id=$1", id)
			case "unverified":
				f.exec("UPDATE app.auth_identities SET verified_at=NULL WHERE user_id=$1 AND provider='email'", id)
			case "disabled":
				f.exec("UPDATE app.users SET account_state='disabled' WHERE id=$1", id)
			case "deleted":
				f.exec("UPDATE app.users SET deleted_at=now() WHERE id=$1", id)
			case "oauth-only":
				f.exec("DELETE FROM app.password_credentials WHERE user_id=$1", id)
				f.exec("INSERT INTO app.auth_identities(id,user_id,provider,provider_subject,email,created_at,updated_at) VALUES($1,$2,'google',$3,$4,now(),now())", uuid.NewV7().String(), id, uuid.NewV7().String(), email)
			}
		}
		body, cookie := f.adminRequest("POST", "/auth/login", map[string]string{"email": email, "password": password}, nil, 401)
		if body["code"] != "ADMIN_INVALID_CREDENTIALS" || cookie != nil {
			t.Fatal("eligibility disclosed")
		}
		if expected != nil && !reflect.DeepEqual(body, expected) {
			t.Fatal("ineligible credentials have different bodies")
		}
		expected = body
	}
	id, email, _ := f.eligible(auth.Moderator)
	old, err := easyhash.Hash(testPassword, easyhash.WithBcryptCost(4))
	if err != nil {
		t.Fatal("legacy KDF fixture failed")
	}
	f.exec("UPDATE app.password_credentials SET password_hash=$1 WHERE user_id=$2", old, id)
	f.adminLogin(email)
	var hash string
	var unchanged bool
	if err = f.owner.QueryRow(t.Context(), "SELECT password_hash,password_updated_at=created_at FROM app.password_credentials WHERE user_id=$1", id).Scan(&hash, &unchanged); err != nil {
		t.Fatal("KDF upgrade inspection failed")
	}
	algorithm, _ := easyhash.Identify(hash)
	if algorithm != easyhash.AlgorithmArgon2id || !unchanged {
		t.Fatal("Admin CAS changed password epoch or failed upgrade")
	}
}

func TestIntegrationAdminSessionsAndCSRF(t *testing.T) {
	f := newAdminFixture(t)
	id, email, publicCookie := f.eligible(auth.Moderator)
	_, cookie := f.adminLogin(email)
	f.adminRequest("GET", "/me", nil, publicCookie, 401)
	f.adminRequest("GET", "/auth/csrf", nil, publicCookie, 401)
	renamed := *publicCookie
	renamed.Name = cookie.Name
	f.adminRequest("GET", "/me", nil, &renamed, 401)
	f.request("GET", "/me", nil, cookie, 401)
	renamed = *cookie
	renamed.Name = publicCookie.Name
	f.request("GET", "/me", nil, &renamed, 401)
	csrf, _ := f.adminRequest("GET", "/auth/csrf", nil, cookie, 200)
	duplicate := httptest.NewRequest("POST", "/auth/logout", nil)
	duplicate.AddCookie(cookie)
	duplicate.Header.Set("Origin", adminOrigin)
	duplicate.Header.Add("X-CSRF-Token", adminCSRF(cookie))
	duplicate.Header.Add("X-CSRF-Token", adminCSRF(cookie))
	duplicateResponse, err := f.adminApp.Test(duplicate)
	if err != nil {
		t.Fatal("duplicate CSRF request failed")
	}
	var duplicateBody map[string]any
	decodeErr := json.NewDecoder(duplicateResponse.Body).Decode(&duplicateBody)
	duplicateResponse.Body.Close()
	if duplicateResponse.StatusCode != 403 || decodeErr != nil || duplicateBody["code"] != "CSRF_INVALID" {
		t.Fatal("duplicate CSRF bypassed stable JSON rejection")
	}
	if csrf["csrf_token"] != adminCSRF(cookie) {
		t.Fatal("Admin CSRF derivation failed")
	}
	_, other := f.adminLogin(email)
	for _, headers := range []map[string]string{{"Origin": ""}, {"Origin": testOrigin}, {"X-CSRF-Token": ""}, {"X-CSRF-Token": csrfToken(config.DevelopmentCSRFSecret, cookie.Value)}, {"X-CSRF-Token": adminCSRF(other)}} {
		f.adminRequest("POST", "/auth/logout", nil, cookie, 403, headers)
	}
	listed, _ := f.adminRequest("GET", "/me/sessions", nil, cookie, 200)
	rows := listed["sessions"].([]any)
	if len(rows) != 2 {
		t.Fatal("Admin list included Public sessions")
	}
	for _, row := range rows {
		assertKeys(t, row.(map[string]any), []string{"id", "authenticated_at", "created_at", "last_seen_at", "idle_expires_at", "absolute_expires_at", "current"})
	}
	var absolute, idle float64
	if err := f.owner.QueryRow(t.Context(), "SELECT extract(epoch FROM absolute_expires_at-created_at),extract(epoch FROM idle_expires_at-created_at) FROM app.sessions WHERE user_id=$1 AND kind='admin' LIMIT 1", id).Scan(&absolute, &idle); err != nil || absolute != 28800 || idle != 3600 {
		t.Fatal("Admin lifetime differs")
	}
	publicActor, err := f.auth.Resolve(t.Context(), publicCookie.Value)
	if err != nil {
		t.Fatal("Public fixture lost")
	}
	f.adminRequest("DELETE", "/me/sessions/"+publicActor.SessionID.String(), nil, cookie, 404)
	_, foreignEmail, _ := f.eligible(auth.Editor)
	_, foreignCookie := f.adminLogin(foreignEmail)
	foreignActor, err := f.adminAuth.ResolveAdmin(t.Context(), foreignCookie.Value)
	if err != nil {
		t.Fatal("foreign Admin fixture failed")
	}
	f.adminRequest("DELETE", "/me/sessions/"+foreignActor.SessionID.String(), nil, cookie, 404)
	f.adminRequest("GET", "/me", nil, foreignCookie, 200)
	adminActor, err := f.adminAuth.ResolveAdmin(t.Context(), cookie.Value)
	if err != nil {
		t.Fatal("Admin fixture lost")
	}
	f.request("DELETE", "/me/sessions/"+adminActor.SessionID.String(), nil, publicCookie, 404)
	f.request("POST", "/me/sessions/revoke-others", nil, publicCookie, 204)
	f.adminRequest("GET", "/me", nil, other, 200)
	_, rotated := f.adminRequest("POST", "/auth/reauthenticate", map[string]string{"password": testPassword}, cookie, 204)
	if rotated == nil || rotated.Value == cookie.Value {
		t.Fatal("Admin reauthentication failed to rotate")
	}
	f.adminRequest("GET", "/me", nil, cookie, 401)
	f.adminRequest("POST", "/auth/logout", nil, rotated, 403, map[string]string{"X-CSRF-Token": adminCSRF(cookie)})
	f.adminRequest("POST", "/me/sessions/revoke-others", nil, rotated, 204)
	f.adminRequest("GET", "/me", nil, other, 401)
	f.request("GET", "/me", nil, publicCookie, 200)
	f.adminRequest("POST", "/auth/logout", nil, rotated, 204)
	f.request("GET", "/me", nil, publicCookie, 200)
	_, fresh := f.adminLogin(email)
	f.request("POST", "/auth/logout", nil, publicCookie, 204)
	f.adminRequest("GET", "/me", nil, fresh, 200)
	_, freshPublic := f.request("POST", "/auth/login", map[string]string{"email": email, "password": testPassword}, nil, 200)
	publicCookie = freshPublic
	_, deletable := f.adminLogin(email)
	deleteActor, err := f.adminAuth.ResolveAdmin(t.Context(), deletable.Value)
	if err != nil {
		t.Fatal("session deletion fixture failed")
	}
	_, cleared := f.adminRequest("DELETE", "/me/sessions/"+deleteActor.SessionID.String(), nil, deletable, 204)
	if cleared == nil || cleared.MaxAge != -1 {
		t.Fatal("current Admin revocation did not clear cookie")
	}
	f.adminRequest("GET", "/me", nil, fresh, 200)
	f.request("GET", "/me", nil, publicCookie, 200)
	for _, event := range []string{"admin_login_succeeded", "admin_logout", "admin_reauthenticated", "admin_session_revoked", "admin_other_sessions_revoked"} {
		if f.scalar("SELECT count(*) FROM app.security_events WHERE user_id=$1 AND event_type=$2", id, event) == 0 {
			t.Fatal("Admin security event missing")
		}
	}
	f.exec("UPDATE app.sessions SET last_seen_at=now()-interval '6 minutes',idle_expires_at=now()+interval '10 minutes' WHERE user_id=$1 AND kind='admin' AND revoked_at IS NULL", id)
	f.adminRequest("GET", "/me", nil, fresh, 200)
	if f.scalar("SELECT count(*) FROM app.sessions WHERE user_id=$1 AND kind='admin' AND revoked_at IS NULL AND last_seen_at>now()-interval '1 minute' AND idle_expires_at>now()+interval '55 minutes'", id) != 1 {
		t.Fatal("Admin touch failed")
	}
	f.exec("UPDATE app.sessions SET idle_expires_at=now()-interval '1 second' WHERE user_id=$1 AND kind='admin'", id)
	f.adminRequest("GET", "/me", nil, fresh, 401)
	_, fresh = f.adminLogin(email)
	f.exec("UPDATE app.sessions SET absolute_expires_at=created_at WHERE user_id=$1 AND kind='admin' AND revoked_at IS NULL", id)
	f.adminRequest("GET", "/me", nil, fresh, 401)
}

func TestIntegrationAdminRoleOperator(t *testing.T) {
	f := newAdminFixture(t)
	ctx := t.Context()
	id, email, publicCookie := f.eligible(auth.Moderator)
	_, cookie := f.adminLogin(email)
	if _, err := f.operator.Grant(ctx, email, auth.Moderator); err != nil {
		t.Fatal("idempotent grant failed")
	}
	if f.scalar("SELECT count(*) FROM app.security_events WHERE user_id=$1 AND event_type='role_moderator_granted'", id) != 1 {
		t.Fatal("idempotent grant duplicated event")
	}
	if _, err := f.operator.Grant(ctx, email, auth.Editor); err != nil {
		t.Fatal("additive grant failed")
	}
	me, _ := f.adminRequest("GET", "/me", nil, cookie, 200)
	if len(me["roles"].([]any)) != 2 {
		t.Fatal("session cached old roles")
	}
	if _, err := f.operator.Revoke(ctx, email, auth.Moderator); err != nil {
		t.Fatal("role revoke failed")
	}
	f.adminRequest("GET", "/me", nil, cookie, 200)
	if _, err := f.operator.Revoke(ctx, email, auth.Editor); err != nil {
		t.Fatal("final privilege revoke failed")
	}
	f.adminRequest("GET", "/me", nil, cookie, 401)
	f.request("GET", "/me", nil, publicCookie, 200)
	if f.scalar("SELECT count(*) FROM app.sessions WHERE user_id=$1 AND kind='admin' AND revoked_at IS NULL", id) != 0 {
		t.Fatal("final privilege left Admin sessions")
	}
	_, unverified, _ := f.register()
	for _, event := range []string{"role_moderator_granted", "role_moderator_revoked", "role_editor_granted", "role_editor_revoked"} {
		if f.scalar("SELECT count(*) FROM app.security_events WHERE user_id=$1 AND event_type=$2", id, event) != 1 {
			t.Fatal("role event absent or duplicated")
		}
	}
	if _, err := f.operator.Grant(ctx, unverified, auth.Moderator); !errors.Is(err, auth.ErrRoleIneligible) {
		t.Fatal("unverified bootstrap allowed")
	}
	if _, err := f.operator.Grant(ctx, email, "owner"); !errors.Is(err, auth.ErrInvalidRole) {
		t.Fatal("unknown role accepted")
	}
	firstID, first, _ := f.eligible(auth.Administrator)
	secondID, second, _ := f.eligible(auth.Administrator)
	var results [2]error
	var wg sync.WaitGroup
	gate := make(chan struct{})
	for i, target := range []string{first, second} {
		wg.Go(func() { <-gate; _, results[i] = f.operator.Revoke(ctx, target, auth.Administrator) })
	}
	close(gate)
	wg.Wait()
	failures := 0
	for _, err := range results {
		if errors.Is(err, auth.ErrLastAdmin) {
			failures++
		} else if err != nil {
			t.Fatal("concurrent role revoke failed")
		}
	}
	if failures != 1 || f.scalar("SELECT count(*) FROM app.user_roles WHERE user_id IN ($1,$2) AND role='admin'", firstID, secondID) != 1 {
		t.Fatal("concurrent last-admin invariant failed")
	}
}

func TestIntegrationAdminMinimalGrants(t *testing.T) {
	f := newAdminFixture(t)
	ctx := t.Context()
	for _, test := range []struct {
		role, table, privilege string
		want                   bool
	}{
		{"gfp_admin", "users", "SELECT", true}, {"gfp_admin", "auth_identities", "SELECT", true}, {"gfp_admin", "password_credentials", "SELECT", true}, {"gfp_admin", "user_roles", "SELECT", true}, {"gfp_admin", "sessions", "SELECT,INSERT", true}, {"gfp_admin", "security_events", "INSERT", true},
		{"gfp_admin", "user_roles", "INSERT,UPDATE,DELETE", false}, {"gfp_api", "user_roles", "SELECT,INSERT,UPDATE,DELETE", false}, {"gfp_worker", "user_roles", "SELECT,INSERT,UPDATE,DELETE", false}, {"gfp_readonly", "user_roles", "SELECT", true},
		{"gfp_admin", "user_profiles", "SELECT,INSERT,UPDATE,DELETE", false}, {"gfp_admin", "auth_challenges", "SELECT,INSERT,UPDATE,DELETE", false}, {"gfp_admin", "auth_identities", "INSERT,UPDATE,DELETE", false}, {"gfp_admin", "sessions", "DELETE,TRUNCATE", false},
	} {
		var got bool
		if err := f.owner.QueryRow(ctx, "SELECT has_table_privilege($1,$2,$3)", test.role, "app."+test.table, test.privilege).Scan(&got); err != nil || got != test.want {
			t.Fatal("runtime table privileges differ")
		}
	}
	for _, test := range []struct {
		table, column string
		want          bool
	}{{"users", "updated_at", true}, {"users", "account_state", false}, {"password_credentials", "password_hash", true}, {"password_credentials", "updated_at", true}, {"password_credentials", "password_updated_at", false}, {"sessions", "revoked_at", true}, {"sessions", "last_seen_at", true}, {"sessions", "idle_expires_at", true}, {"sessions", "kind", false}, {"sessions", "token_hash", false}, {"sessions", "authenticated_at", false}, {"sessions", "absolute_expires_at", false}} {
		var got bool
		if err := f.owner.QueryRow(ctx, "SELECT has_column_privilege('gfp_admin',$1,$2,'UPDATE')", "app."+test.table, test.column).Scan(&got); err != nil || got != test.want {
			t.Fatal("runtime column privileges differ")
		}
	}
}

func TestIntegrationAdminAndPublicThrottle(t *testing.T) {
	f := newAdminFixture(t)
	ctx := t.Context()
	id, email, _ := f.eligible(auth.Moderator)
	wrong := map[string]string{"email": strings.ToUpper(email), "password": "a wrong password for testing"}
	for range 5 {
		f.adminRequest("POST", "/auth/login", wrong, nil, 401)
	}
	limited, _ := f.adminRequest("POST", "/auth/login", wrong, nil, 429)
	if limited["code"] != "AUTH_RATE_LIMITED" {
		t.Fatal("Admin rate error differs")
	}
	if f.scalar("SELECT count(*) FROM app.security_events WHERE user_id=$1 AND event_type='admin_login_failed'", id) != 5 {
		t.Fatal("Admin failures not bounded")
	}
	for range 10 {
		f.request("POST", "/auth/login", wrong, nil, 401)
	}
	f.request("POST", "/auth/login", wrong, nil, 429)
	if f.scalar("SELECT count(*) FROM app.security_events WHERE user_id=$1 AND event_type='login_failed'", id) != 10 {
		t.Fatal("Public failures not bounded")
	}
	if err := f.throttle.ClearSubject(ctx, auth.AdminLoginLimit, email); err != nil {
		t.Fatal("fixture clear failed")
	}
	_, cookie := f.adminLogin(email)
	for range 5 {
		f.adminRequest("POST", "/auth/reauthenticate", map[string]string{"password": "wrong reauthentication password"}, cookie, 401)
	}
	f.adminRequest("POST", "/auth/reauthenticate", map[string]string{"password": testPassword}, cookie, 429)
	_, clearEmail, _ := f.eligible(auth.Editor)
	clearWrong := map[string]string{"email": clearEmail, "password": "an incorrect fixture password"}
	f.adminRequest("POST", "/auth/login", clearWrong, nil, 401)
	f.adminLogin(clearEmail)
	for range 5 {
		f.adminRequest("POST", "/auth/login", clearWrong, nil, 401)
	}
	f.adminRequest("POST", "/auth/login", clearWrong, nil, 429)
	f.request("POST", "/auth/login", clearWrong, nil, 401)
	f.request("POST", "/auth/login", map[string]string{"email": clearEmail, "password": testPassword}, nil, 200)
	for range 10 {
		f.request("POST", "/auth/login", clearWrong, nil, 401)
	}
	f.request("POST", "/auth/login", clearWrong, nil, 429)
	freshEmail := emailFixture()
	registration := map[string]string{"email": freshEmail, "password": testPassword}
	f.request("POST", "/auth/register", registration, nil, 201)
	f.request("POST", "/auth/register", registration, nil, 409)
	f.request("POST", "/auth/register", registration, nil, 409)
	f.request("POST", "/auth/register", registration, nil, 429)
	var accepted map[string]any
	for range 4 {
		body, _ := f.request("POST", "/auth/password/reset/request", map[string]string{"email": email}, nil, 202)
		if accepted != nil && !reflect.DeepEqual(body, accepted) {
			t.Fatal("limited reset response disclosed state")
		}
		accepted = body
		f.exec("UPDATE app.auth_challenges SET created_at=created_at-interval '61 seconds' WHERE user_id=$1 AND purpose='password_reset'", id)
	}
	if f.scalar("SELECT count(*) FROM app.auth_challenges WHERE user_id=$1 AND purpose='password_reset'", id) != 3 {
		t.Fatal("limited reset issued challenge")
	}
	f.store.Close()
	f.adminRequest("POST", "/auth/login", map[string]string{"email": email, "password": testPassword}, nil, 503)
	f.adminRequest("GET", "/me", nil, cookie, 200) // Existing canonical session survives cache outage.
	f.request("POST", "/auth/login", map[string]string{"email": email, "password": testPassword}, nil, 200)
	f.request("POST", "/auth/register", map[string]string{"email": emailFixture(), "password": testPassword}, nil, 201)
	f.request("POST", "/auth/password/reset/request", map[string]string{"email": email}, nil, 202)
	if f.scalar("SELECT count(*) FROM app.auth_challenges WHERE user_id=$1 AND purpose='password_reset'", id) != 4 {
		t.Fatal("Public reset failed closed")
	}
	f.request("POST", "/auth/login", wrong, nil, 401)
	if f.scalar("SELECT count(*) FROM app.security_events WHERE user_id=$1 AND event_type='login_failed'", id) != 10 {
		t.Fatal("cache outage permitted unbounded failure events")
	}
}

func TestIntegrationAdminPasswordAndRoleRaces(t *testing.T) {
	t.Run("role-revoke-during-request", func(t *testing.T) {
		f := newAdminFixture(t)
		_, email, _ := f.eligible(auth.Moderator)
		_, cookie := f.adminLogin(email)
		pause := f.gate.armQuery(t, "-- name: ListUserRoles")
		done := make(chan error, 1)
		go func() { _, err := f.adminAuth.ResolveAdmin(t.Context(), cookie.Value); done <- err }()
		select {
		case <-pause.entered:
		case <-time.After(5 * time.Second):
			t.Fatal("role read was not reached")
		}
		if _, err := f.operator.Revoke(t.Context(), email, auth.Moderator); err != nil {
			t.Fatal("competing role removal failed")
		}
		close(pause.resume)
		err := <-done
		if !errors.Is(err, auth.ErrAdminForbidden) && !errors.Is(err, auth.ErrAdminUnauthenticated) {
			t.Fatal("in-flight request retained stale role")
		}
	})
	for _, operation := range []string{"login-reset", "login-change", "reauth-reset", "login-role", "reauth-role"} {
		t.Run(operation, func(t *testing.T) {
			f := newAdminFixture(t)
			ctx := t.Context()
			id, email, publicCookie := f.eligible(auth.Moderator)
			_, adminCookie := f.adminLogin(email)
			actor, err := f.adminAuth.ResolveAdmin(ctx, adminCookie.Value)
			if err != nil {
				t.Fatal("Admin actor failed")
			}
			f.auth.RequestPasswordReset(ctx, email)
			resetToken := f.lastToken("password_reset")
			pause := f.gate.arm(t)
			type outcome struct {
				grant auth.AdminGrant
				err   error
			}
			done := make(chan outcome, 1)
			go func() {
				var grant auth.AdminGrant
				var err error
				if strings.HasPrefix(operation, "reauth") {
					grant, err = f.adminAuth.AdminReauthenticate(ctx, actor, testPassword)
				} else {
					grant, err = f.adminAuth.AdminLogin(ctx, email, testPassword)
				}
				done <- outcome{grant, err}
			}()
			select {
			case <-pause.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("Admin did not reach transaction after KDF")
			}
			switch {
			case strings.HasSuffix(operation, "role"):
				_, err = f.operator.Revoke(ctx, email, auth.Moderator)
			case strings.HasSuffix(operation, "change"):
				var publicActor auth.Actor
				publicActor, err = f.auth.Resolve(ctx, publicCookie.Value)
				if err == nil {
					_, err = f.auth.ChangePassword(ctx, publicActor, testPassword, "a replacement password for racing")
				}
			default:
				_, err = f.auth.ResetPassword(ctx, resetToken, "a replacement password for racing")
			}
			if err != nil {
				t.Fatal("competing mutation failed while Admin KDF held no User lock")
			}
			close(pause.resume)
			result := <-done
			if result.err == nil {
				t.Fatal("in-flight Admin escaped password/role revocation")
			}
			if f.scalar("SELECT count(*) FROM app.sessions WHERE user_id=$1 AND kind='admin' AND revoked_at IS NULL", id) != 0 {
				t.Fatal("Admin session escaped revocation")
			}
			f.adminRequest("GET", "/me", nil, adminCookie, 401)
		})
	}
	// Admin-first ordering: reset/change revoke a successfully committed Admin
	// session and leave exactly one replacement Public session.
	for _, change := range []bool{false, true} {
		f := newAdminFixture(t)
		ctx := t.Context()
		id, email, cookie := f.eligible(auth.Moderator)
		_, adminCookie := f.adminLogin(email)
		var err error
		if change {
			actor, e := f.auth.Resolve(ctx, cookie.Value)
			if e != nil {
				t.Fatal("Public actor failed")
			}
			_, err = f.auth.ChangePassword(ctx, actor, testPassword, "a new password in admin first order")
		} else {
			f.auth.RequestPasswordReset(ctx, email)
			_, err = f.auth.ResetPassword(ctx, f.lastToken("password_reset"), "a new password in admin first order")
		}
		if err != nil {
			t.Fatal("password mutation failed")
		}
		f.adminRequest("GET", "/me", nil, adminCookie, 401)
		if f.scalar("SELECT count(*) FROM app.sessions WHERE user_id=$1 AND revoked_at IS NULL", id) != 1 || f.scalar("SELECT count(*) FROM app.sessions WHERE user_id=$1 AND kind='public' AND revoked_at IS NULL", id) != 1 {
			t.Fatal("password mutation left wrong replacement session")
		}
	}
}
