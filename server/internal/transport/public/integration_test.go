package public

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/config"
	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/deepfurry/tap4furry/server/internal/identity"
	"github.com/deepfurry/tap4furry/server/internal/moderation"
	"github.com/deepfurry/tap4furry/server/internal/transport/admin"
	"github.com/deepfurry/tap4furry/server/internal/transport/health"
	"github.com/gofiber/fiber/v3"
	"github.com/gofurry/easyhash"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const testOrigin = "http://localhost:4321"
const testPassword = "a long test password 🦊"

type fixture struct {
	t          *testing.T
	app        *fiber.App
	api, owner *pgxpool.Pool
	auth       *auth.App
	identity   *identity.App
	mail       *fakeMailer
}

func newFixture(t *testing.T, oauth ...auth.OAuthConfig) *fixture {
	t.Helper()
	if os.Getenv("GFP_AUTH_INTEGRATION") != "1" {
		t.Skip("explicit disposable auth integration not enabled")
	}
	if os.Getenv("CI") != "true" || os.Getenv("GFP_DISPOSABLE_INFRA") != "1" {
		t.Fatal("auth integration requires disposable CI guards")
	}
	// Deliberately fixed loopback fixtures. Never read local files or developer URLs.
	api, err := database.Open(t.Context(), "postgres://gfp_api:gfp_ci_only@127.0.0.1:5432/gfp_ci?sslmode=disable")
	if err != nil {
		t.Fatal("disposable API connection failed")
	}
	t.Cleanup(api.Close)
	owner, err := database.Open(t.Context(), "postgres://gfp_migrator:gfp_ci_only@127.0.0.1:5432/gfp_ci?sslmode=disable")
	if err != nil {
		t.Fatal("disposable migrator connection failed")
	}
	t.Cleanup(owner.Close)
	if err = database.Ready(t.Context(), api); err != nil {
		t.Fatal("disposable database is not ready")
	}
	mailer := &fakeMailer{}
	authentication, err := auth.New(api, mailer, oauth...)
	if err != nil {
		t.Fatal("authentication initialization failed")
	}
	identities := identity.New(api)
	checker := health.New(func(ctx context.Context) error { return database.Ready(ctx, api) }, func(context.Context) error { return nil })
	app := fiber.New()
	Register(app, checker, authentication, identities, Options{Environment: "test", PublicOrigin: testOrigin, CSRFSecret: config.DevelopmentCSRFSecret, ResourcePool: api})
	return &fixture{t: t, app: app, api: api, owner: owner, auth: authentication, identity: identities, mail: mailer}
}

func (f *fixture) request(method, path string, body any, cookie *http.Cookie, status int) (map[string]any, *http.Cookie) {
	f.t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		f.t.Fatal("encode request failed")
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	req.Header.Set("Origin", testOrigin)
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
		req.Header.Set("X-CSRF-Token", csrfToken(config.DevelopmentCSRFSecret, cookie.Value))
	}
	response, err := f.app.Test(req, fiber.TestConfig{Timeout: 10 * time.Second})
	if err != nil {
		f.t.Fatal("HTTP integration request failed (details withheld)")
	}
	defer response.Body.Close()
	if response.StatusCode != status {
		f.t.Fatalf("%s %s: status %d; expected %d (body withheld)", method, path, response.StatusCode, status)
	}
	var result map[string]any
	if status != 204 && json.NewDecoder(response.Body).Decode(&result) != nil {
		f.t.Fatal("invalid JSON response")
	}
	if strings.HasPrefix(path, "/auth/") || path == "/me" || path == "/me/profile" {
		if response.Header.Get("Cache-Control") != "no-store" {
			f.t.Fatal("private response is cacheable")
		}
	}
	cookies := response.Cookies()
	var next *http.Cookie
	if len(cookies) > 0 {
		next = cookies[0]
	}
	return result, next
}

func assertMePrivacy(t *testing.T, body map[string]any) {
	t.Helper()
	assertKeys(t, body, []string{"id", "account_state", "created_at", "email", "email_verified", "profile"})
	profile, ok := body["profile"].(map[string]any)
	if !ok {
		t.Fatal("missing profile")
	}
	assertKeys(t, profile, []string{"handle", "display_name", "bio", "search_engine_indexing"})
}
func assertKeys(t *testing.T, body map[string]any, want []string) {
	t.Helper()
	keys := make([]string, 0, len(body))
	for key := range body {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	sort.Strings(want)
	if !reflect.DeepEqual(keys, want) {
		t.Fatal("response exposes unexpected fields or lacks required fields (values withheld)")
	}
}
func (f *fixture) exec(query string, args ...any) {
	f.t.Helper()
	if _, err := f.owner.Exec(f.t.Context(), query, args...); err != nil {
		f.t.Fatal(database.SafeError("disposable fixture statement", err))
	}
}
func emailFixture() string { return uuid.NewV7().String() + "@example.invalid" }

func TestIntegrationHTTPAndPrivacy(t *testing.T) {
	f := newFixture(t)
	email := emailFixture()
	credentials := map[string]string{"email": email, "password": testPassword}
	me, cookie := f.request("POST", "/auth/register", credentials, nil, 201)
	assertMePrivacy(t, me)
	if cookie == nil || !cookie.HttpOnly || cookie.Secure || cookie.Name != "tap4furry_session" || cookie.Path != "/" || cookie.Domain != "" || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatal("registration cookie contract failed")
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(cookie.Value)
	if err != nil || len(raw) != 32 {
		t.Fatal("cookie token lacks required entropy encoding")
	}
	id := me["id"].(string)
	parsed, err := uuid.Parse(id)
	if err != nil || parsed[6]>>4 != 7 || me["email_verified"] != false || me["profile"].(map[string]any)["search_engine_indexing"] != false {
		t.Fatal("registration defaults or UUIDv7 failed")
	}
	var storedHash []byte
	var passwordHash, provider, subject string
	var verified *time.Time
	err = f.owner.QueryRow(t.Context(), `SELECT s.token_hash,c.password_hash,a.provider,a.provider_subject,a.verified_at FROM app.sessions s JOIN app.password_credentials c ON c.user_id=s.user_id JOIN app.auth_identities a ON a.user_id=s.user_id WHERE s.user_id=$1`, id).Scan(&storedHash, &passwordHash, &provider, &subject, &verified)
	expectedHash := sha256.Sum256([]byte(cookie.Value))
	if err != nil || !bytes.Equal(storedHash, expectedHash[:]) || provider != "email" || subject != email || verified != nil {
		t.Fatal("private persistence contract failed")
	}
	if algorithm, _ := easyhash.Identify(passwordHash); algorithm != easyhash.AlgorithmArgon2id {
		t.Fatal("registration persisted non-Argon2id password")
	}
	if strings.Contains(passwordHash, testPassword) {
		t.Fatal("plaintext password persisted")
	}

	duplicate, _ := f.request("POST", "/auth/register", map[string]string{"email": strings.ToUpper(email), "password": testPassword}, nil, 409)
	if duplicate["code"] != "AUTH_EMAIL_ALREADY_REGISTERED" {
		t.Fatal("duplicate email mapping failed")
	}
	anonymous, _ := f.request("GET", "/me", nil, nil, 401)
	if anonymous["code"] != "AUTH_UNAUTHENTICATED" {
		t.Fatal("anonymous me accepted")
	}
	current, _ := f.request("GET", "/me", nil, cookie, 200)
	assertMePrivacy(t, current)
	handle := "u" + strings.ReplaceAll(uuid.NewV7().String(), "-", "")[1:24]
	profile := map[string]any{"handle": handle, "display_name": "Furry 🦊", "bio": "Hello ecosystem", "search_engine_indexing": true}
	updated, _ := f.request("PATCH", "/me/profile", profile, cookie, 200)
	assertMePrivacy(t, updated)
	publicView, _ := f.request("GET", "/users/"+handle, nil, nil, 200)
	assertKeys(t, publicView, []string{"handle", "display_name", "bio"})
	if publicView["handle"] != handle || publicView["display_name"] != profile["display_name"] {
		t.Fatal("public profile values failed")
	}
	cleared, _ := f.request("PATCH", "/me/profile", map[string]any{"bio": nil}, cookie, 200)
	clearedProfile := cleared["profile"].(map[string]any)
	if clearedProfile["bio"] != nil || clearedProfile["handle"] != handle || clearedProfile["display_name"] != profile["display_name"] {
		t.Fatal("PATCH omission/null semantics failed")
	}
	for _, invalid := range []map[string]any{{"email": "other@example.invalid"}, {"password": "ignored"}, {"account_state": "disabled"}, {"handle": "UPPER"}, {"display_name": strings.Repeat("a", 81)}, {"bio": strings.Repeat("界", 501)}, {"search_engine_indexing": nil}} {
		f.request("PATCH", "/me/profile", invalid, cookie, 400)
	}
	f.request("GET", "/users/not-present", nil, nil, 404)
	second, secondCookie := f.request("POST", "/auth/register", map[string]string{"email": emailFixture(), "password": testPassword}, nil, 201)
	conflict, _ := f.request("PATCH", "/me/profile", map[string]any{"handle": handle}, secondCookie, 409)
	if conflict["code"] != "PROFILE_HANDLE_UNAVAILABLE" {
		t.Fatal("handle conflict mapping failed")
	}
	_ = second
	wrong, _ := f.request("POST", "/auth/login", map[string]string{"email": email, "password": "wrong password value"}, nil, 401)
	unknown, _ := f.request("POST", "/auth/login", map[string]string{"email": emailFixture(), "password": "wrong password value"}, nil, 401)
	if !reflect.DeepEqual(wrong, unknown) || wrong["code"] != "AUTH_INVALID_CREDENTIALS" {
		t.Fatal("login reveals identity existence")
	}
	loggedIn, loginCookie := f.request("POST", "/auth/login", credentials, nil, 200)
	assertMePrivacy(t, loggedIn)
	if loginCookie == nil || loginCookie.Value == cookie.Value {
		t.Fatal("login did not issue a new session")
	}
	_, expiredCookie := f.request("POST", "/auth/logout", nil, loginCookie, 204)
	if expiredCookie == nil || expiredCookie.Value != "" || expiredCookie.MaxAge != -1 || !expiredCookie.Expires.Before(time.Now()) {
		t.Fatal("logout did not clear cookie")
	}
	f.request("GET", "/me", nil, loginCookie, 401)
	f.request("POST", "/auth/logout", nil, nil, 401)
	for _, path := range []string{"/auth/register", "/auth/login", "/auth/logout", "/me/profile"} {
		method := "POST"
		if path == "/me/profile" {
			method = "PATCH"
		}
		for _, origin := range []string{"", "https://attacker.invalid"} {
			req := httptest.NewRequest(method, path, nil)
			req.Header.Set("Origin", origin)
			req.AddCookie(cookie)
			response, err := f.app.Test(req)
			if err != nil {
				t.Fatal("origin HTTP check failed")
			}
			response.Body.Close()
			if response.StatusCode != 403 {
				t.Fatal("unsafe endpoint bypasses Origin guard")
			}
		}
	}
	f.request("GET", "/health/live", nil, nil, 200)
	f.request("GET", "/health/ready", nil, nil, 200)
	adminApp := fiber.New()
	admin.Register(adminApp, health.New(func(context.Context) error { return nil }, func(context.Context) error { return nil }))
	req := httptest.NewRequest("GET", "/me", nil)
	req.AddCookie(cookie)
	response, err := adminApp.Test(req)
	if err != nil {
		t.Fatal("admin separation check failed")
	}
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatal("Admin accepted a Public session")
	}

	// Soft-deleted profiles disappear, while their unique handles remain reserved.
	f.exec("UPDATE app.users SET deleted_at=now() WHERE id=$1", id)
	f.request("GET", "/users/"+handle, nil, nil, 404)
	f.request("PATCH", "/me/profile", map[string]any{"handle": handle}, secondCookie, 409)
	f.request("GET", "/me", nil, cookie, 401)
	f.request("POST", "/auth/login", credentials, nil, 401)
}

func TestIntegrationSessionLifecycle(t *testing.T) {
	f := newFixture(t)
	email := emailFixture()
	credentials := map[string]string{"email": email, "password": testPassword}
	me, cookie := f.request("POST", "/auth/register", credentials, nil, 201)
	id := me["id"].(string)
	for _, column := range []string{"idle_expires_at", "absolute_expires_at"} {
		f.exec("UPDATE app.sessions SET "+column+"=now()-interval '1 second' WHERE user_id=$1", id)
		f.request("GET", "/me", nil, cookie, 401)
		f.exec("UPDATE app.sessions SET idle_expires_at=now()+interval '14 days',absolute_expires_at=now()+interval '30 days' WHERE user_id=$1", id)
	}
	f.exec("UPDATE app.users SET account_state='disabled' WHERE id=$1", id)
	f.request("GET", "/me", nil, cookie, 401)
	disabled, _ := f.request("POST", "/auth/login", credentials, nil, 403)
	if disabled["code"] != "AUTH_ACCOUNT_DISABLED" {
		t.Fatal("disabled account login mapping failed")
	}
	f.exec("UPDATE app.users SET account_state='active' WHERE id=$1", id)
	f.exec("UPDATE app.sessions SET kind='admin' WHERE user_id=$1", id)
	f.request("GET", "/me", nil, cookie, 401)
	f.exec("UPDATE app.sessions SET kind='public',last_seen_at=now()-interval '11 minutes',idle_expires_at=now()+interval '1 hour',absolute_expires_at=now()+interval '10 days' WHERE user_id=$1", id)
	f.request("GET", "/me", nil, cookie, 200)
	var first, last, idle, absolute time.Time
	if err := f.owner.QueryRow(t.Context(), "SELECT last_seen_at,idle_expires_at,absolute_expires_at FROM app.sessions WHERE user_id=$1", id).Scan(&first, &idle, &absolute); err != nil {
		t.Fatal("session inspection failed")
	}
	if !idle.Equal(absolute) || time.Since(first) > time.Minute {
		t.Fatal("touch did not extend idle lifetime bounded by absolute expiry")
	}
	f.request("GET", "/me", nil, cookie, 200)
	if err := f.owner.QueryRow(t.Context(), "SELECT last_seen_at FROM app.sessions WHERE user_id=$1", id).Scan(&last); err != nil || !last.Equal(first) {
		t.Fatal("session touch was not throttled")
	}
}

func TestIntegrationUpgradeAndConcurrency(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	email := emailFixture()
	grant, err := f.auth.Register(ctx, email, testPassword)
	if err != nil {
		t.Fatal("registration fixture failed")
	}
	legacy, err := easyhash.Hash(testPassword, easyhash.WithBcryptCost(4))
	if err != nil {
		t.Fatal("bcrypt fixture failed")
	}
	f.exec("UPDATE app.password_credentials SET password_hash=$1 WHERE user_id=$2", legacy, grant.Me.ID.String())
	var passwordUpdated time.Time
	if err := f.owner.QueryRow(ctx, "SELECT password_updated_at FROM app.password_credentials WHERE user_id=$1", grant.Me.ID.String()).Scan(&passwordUpdated); err != nil {
		t.Fatal("credential inspection failed")
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Go(func() { _, err := f.auth.Login(ctx, email, testPassword); results <- err })
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal("concurrent upgrade login failed")
		}
	}
	row, err := sqlc.New(f.api).FindLocalCredentialByEmailSubject(ctx, email)
	if err != nil {
		t.Fatal("credential lookup failed")
	}
	if algorithm, _ := easyhash.Identify(row.PasswordHash); algorithm != easyhash.AlgorithmArgon2id {
		t.Fatal("bcrypt login upgrade was not persisted")
	}
	count, err := sqlc.New(f.api).CompareAndSwapPasswordHash(ctx, sqlc.CompareAndSwapPasswordHashParams{UserID: row.ID, OldHash: legacy, NewHash: legacy, Now: pgtype.Timestamptz{Time: time.Now(), Valid: true}})
	if err != nil || count != 0 {
		t.Fatal("stale CAS overwrote a current hash")
	}
	var after time.Time
	if err := f.owner.QueryRow(ctx, "SELECT password_updated_at FROM app.password_credentials WHERE user_id=$1", grant.Me.ID.String()).Scan(&after); err != nil || !after.Equal(passwordUpdated) {
		t.Fatal("rehash changed password-change timestamp")
	}
	stronger := easyhash.DefaultArgon2()
	stronger.TimeCost++
	strongHash, err := easyhash.Hash(testPassword, easyhash.WithArgon2idConfig(stronger))
	if err != nil {
		t.Fatal("strong credential fixture failed")
	}
	f.exec("UPDATE app.password_credentials SET password_hash=$1 WHERE user_id=$2", strongHash, grant.Me.ID.String())
	if _, err := f.auth.Login(ctx, email, testPassword); err != nil {
		t.Fatal("strong credential login failed")
	}
	row, err = sqlc.New(f.api).FindLocalCredentialByEmailSubject(ctx, email)
	if err != nil || row.PasswordHash != strongHash {
		t.Fatal("strong credential was downgraded")
	}
	dupeEmail := emailFixture()
	duplicateResults := make(chan error, 2)
	for range 2 {
		wg.Go(func() { _, err := f.auth.Register(ctx, dupeEmail, testPassword); duplicateResults <- err })
	}
	wg.Wait()
	close(duplicateResults)
	successes, conflicts := 0, 0
	for err := range duplicateResults {
		if err == nil {
			successes++
		} else if errors.Is(err, auth.ErrEmailRegistered) {
			conflicts++
		} else {
			t.Fatal("unsafe duplicate registration error")
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatal("concurrent email uniqueness failed")
	}
	var orphanCount int
	if err := f.owner.QueryRow(ctx, `SELECT count(*) FROM app.users u WHERE NOT EXISTS (SELECT 1 FROM app.auth_identities a WHERE a.user_id=u.id AND a.provider='email') OR NOT EXISTS (SELECT 1 FROM app.user_profiles p WHERE p.user_id=u.id) OR (NOT EXISTS (SELECT 1 FROM app.password_credentials c WHERE c.user_id=u.id) AND NOT EXISTS (SELECT 1 FROM app.auth_identities a WHERE a.user_id=u.id AND a.provider IN ('google','github')))`).Scan(&orphanCount); err != nil || orphanCount != 0 {
		t.Fatal("registration left partial records")
	}
	other, err := f.auth.Register(ctx, emailFixture(), testPassword)
	if err != nil {
		t.Fatal("second account fixture failed")
	}
	handle := "race" + strings.ReplaceAll(uuid.NewV7().String(), "-", "")[:20]
	handleResults := make(chan error, 2)
	for _, grant := range []auth.Grant{grant, other} {
		wg.Go(func() {
			actor, err := f.auth.Resolve(ctx, grant.Token)
			if err != nil {
				handleResults <- err
				return
			}
			_, err = moderation.New(f.api).UpdateProfile(ctx, actor, identity.ProfileUpdate{Handle: identity.Field[string]{Set: true, Value: &handle}})
			handleResults <- err
		})
	}
	wg.Wait()
	close(handleResults)
	successes, conflicts = 0, 0
	for err := range handleResults {
		if err == nil {
			successes++
		} else if errors.Is(err, identity.ErrHandleUnavailable) {
			conflicts++
		} else {
			t.Fatal("unsafe concurrent handle error")
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatal("concurrent handle uniqueness failed")
	}
}

func TestIntegrationSchemaAndPrivileges(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	var tables string
	if err := f.owner.QueryRow(ctx, `SELECT string_agg(tablename,',' ORDER BY tablename) FROM pg_tables WHERE schemaname='app'`).Scan(&tables); err != nil || tables != "audit_entries,auth_challenges,auth_identities,categories,category_localizations,contribution_contents,contribution_events,contribution_initial_sources,contribution_localization_changes,contribution_relation_changes,contribution_review_audits,contribution_review_resource_changes,contribution_source_changes,contribution_tag_changes,contributions,goose_db_version,moderation_actions,password_credentials,report_events,reports,resource_distribution_policies,resource_external_ids,resource_localizations,resource_relations,resource_sources,resource_tags,resources,security_events,sessions,source_checks,tag_localizations,tags,user_governance_profiles,user_profiles,user_restrictions,user_roles,users" {
		t.Fatal("unexpected application schema or future tables")
	}
	var version int
	if err := f.owner.QueryRow(ctx, "SELECT max(version_id) FROM app.goose_db_version WHERE is_applied").Scan(&version); err != nil || version != 9 {
		t.Fatal("fresh migration chain failed")
	}
	for _, role := range []string{"gfp_api", "gfp_admin", "gfp_worker"} {
		var ddl, owner bool
		if err := f.owner.QueryRow(ctx, "SELECT has_schema_privilege($1,'app','CREATE'), EXISTS(SELECT 1 FROM pg_tables WHERE schemaname='app' AND tableowner=$1)", role).Scan(&ddl, &owner); err != nil || ddl || owner {
			t.Fatal("runtime owns schema/DDL")
		}
	}
	for _, role := range []string{"gfp_worker"} {
		for _, table := range []string{"users", "user_profiles", "auth_identities", "password_credentials", "sessions", "auth_challenges", "security_events", "user_roles"} {
			var anyPrivilege bool
			if err := f.owner.QueryRow(ctx, "SELECT has_any_column_privilege($1,$2,'SELECT,INSERT,UPDATE,REFERENCES') OR has_table_privilege($1,$2,'DELETE,TRUNCATE,TRIGGER')", role, "app."+table).Scan(&anyPrivilege); err != nil || anyPrivilege {
				t.Fatal("unneeded identity privileges granted")
			}
		}
	}
	grant, err := f.auth.Register(ctx, emailFixture(), testPassword)
	if err != nil {
		t.Fatal("constraint account fixture failed")
	}
	id := grant.Me.ID.String()
	for index, test := range []struct {
		query, code string
		args        []any
	}{
		{"UPDATE app.users SET account_state='other' WHERE id=$1", "23514", []any{id}},
		{"UPDATE app.user_profiles SET handle='UPPER' WHERE user_id=$1", "23514", []any{id}},
		{"UPDATE app.user_profiles SET bio=repeat('x',501) WHERE user_id=$1", "23514", []any{id}},
		{"UPDATE app.sessions SET token_hash='short'::bytea WHERE user_id=$1", "23514", []any{id}},
		{"UPDATE app.sessions SET kind='other' WHERE user_id=$1", "23514", []any{id}},
		{"UPDATE app.sessions SET auth_method='future-provider' WHERE user_id=$1", "23514", []any{id}},
		{"DELETE FROM app.users WHERE id=$1", "23001", []any{id}},
		{"INSERT INTO app.users(created_at,updated_at) VALUES(now(),now())", "23502", nil},
	} {
		_, err := f.owner.Exec(ctx, test.query, test.args...)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != test.code {
			t.Fatalf("database invariant %d was not enforced: %v", index, database.SafeError("constraint check", err))
		}
	}
	// Provider+subject is unique; the presentation email is not globally unique.
	f.exec("INSERT INTO app.auth_identities(id,user_id,provider,provider_subject,email,created_at,updated_at) VALUES($1,$2,'github',$3,$4,now(),now())", uuid.NewV7().String(), id, uuid.NewV7().String(), grant.Me.Email)
}
