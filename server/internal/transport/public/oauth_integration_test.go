package public

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/identity"
	"github.com/deepfurry/tap4furry/server/internal/redisstore"
	"github.com/gofiber/fiber/v3"
	"golang.org/x/oauth2"
)

type fakeAuthorization struct {
	challenge, nonce string
	identity         auth.ProviderIdentity
}
type fakeOAuthProvider struct {
	kind    auth.Provider
	mu      sync.Mutex
	codes   map[string]fakeAuthorization
	entered chan struct{}
	resume  chan struct{}
}

func (p *fakeOAuthProvider) AuthorizationURL(_ context.Context, state string, _ auth.OAuthMode) (auth.OAuthAuthorization, error) {
	verifier, nonce := oauth2.GenerateVerifier(), ""
	if p.kind == auth.Google {
		nonce = oauth2.GenerateVerifier()
	}
	q := url.Values{"state": {state}, "code_challenge": {oauth2.S256ChallengeFromVerifier(verifier)}, "nonce": {nonce}}
	return auth.OAuthAuthorization{URL: "https://provider.example.invalid/authorize?" + q.Encode(), Verifier: verifier, Nonce: nonce}, nil
}
func (p *fakeOAuthProvider) issue(start string, identity auth.ProviderIdentity) string {
	parsed, _ := url.Parse(start)
	q := parsed.Query()
	code := oauth2.GenerateVerifier()
	p.mu.Lock()
	defer p.mu.Unlock()
	p.codes[code] = fakeAuthorization{q.Get("code_challenge"), q.Get("nonce"), identity}
	return code
}
func (p *fakeOAuthProvider) Exchange(ctx context.Context, code string, flow auth.OAuthFlow) (auth.ProviderIdentity, error) {
	p.mu.Lock()
	issued, ok := p.codes[code]
	delete(p.codes, code)
	entered, resume := p.entered, p.resume
	p.mu.Unlock()
	if !ok || oauth2.S256ChallengeFromVerifier(flow.Verifier) != issued.challenge || flow.Nonce != issued.nonce {
		return auth.ProviderIdentity{}, auth.ErrProviderInvalid
	}
	if entered != nil {
		close(entered)
		select {
		case <-resume:
		case <-ctx.Done():
			return auth.ProviderIdentity{}, auth.ErrProviderUnavailable
		}
	}
	return issued.identity, nil
}

type oauthFixture struct {
	*fixture
	store     *redisstore.Store
	providers map[auth.Provider]*fakeOAuthProvider
}

func newOAuthFixture(t *testing.T) *oauthFixture {
	// Establish the standard guards before connecting Redis.
	baseline := newFixture(t)
	store, err := redisstore.Open("redis://gfp_runtime:gfp_ci_only@127.0.0.1:6379/0", "gfp:")
	if err != nil {
		t.Fatal("disposable Redis unavailable")
	}
	t.Cleanup(func() { _ = store.Close() })
	providers := map[auth.Provider]*fakeOAuthProvider{}
	adapters := map[auth.Provider]auth.OAuthProvider{}
	for _, kind := range []auth.Provider{auth.Google, auth.GitHub} {
		p := &fakeOAuthProvider{kind: kind, codes: map[string]fakeAuthorization{}}
		providers[kind] = p
		adapters[kind] = p
	}
	authentication, err := auth.New(baseline.api, baseline.mail, auth.OAuthConfig{Flows: store, Providers: adapters})
	if err != nil {
		t.Fatal("fake OAuth initialization failed")
	}
	baseline.auth = authentication
	baseline.app = fiber.New()
	Register(baseline.app, nil, authentication, baseline.identity, Options{Environment: "test", PublicOrigin: testOrigin, CSRFSecret: "tap4furry-development-only-csrf-secret"})
	return &oauthFixture{baseline, store, providers}
}
func fakeIdentity(kind auth.Provider) auth.ProviderIdentity {
	return auth.ProviderIdentity{Provider: kind, Subject: oauth2.GenerateVerifier(), Email: emailFixture(), DisplayName: "Provider fox", EmailVerified: true, EmailAuthoritative: true}
}
func (f *oauthFixture) begin(kind auth.Provider, mode auth.OAuthMode, actor *auth.Actor) auth.OAuthStart {
	f.t.Helper()
	start, err := f.auth.BeginOAuth(f.t.Context(), kind, mode, actor)
	if err != nil {
		f.t.Fatal("begin fake authorization failed")
	}
	return start
}
func (f *oauthFixture) complete(start auth.OAuthStart, identity auth.ProviderIdentity, actor *auth.Actor) (auth.OAuthResult, error) {
	code := f.providers[identity.Provider].issue(start.URL, identity)
	return f.auth.CompleteOAuth(f.t.Context(), auth.OAuthCallback{Provider: identity.Provider, State: start.State, BrowserState: auth.OAuthStateDigest(start.State), Code: code, Actor: actor})
}
func (f *oauthFixture) login(identity auth.ProviderIdentity) (auth.Grant, auth.Actor) {
	f.t.Helper()
	result, err := f.complete(f.begin(identity.Provider, auth.OAuthLogin, nil), identity, nil)
	if err != nil {
		f.t.Fatalf("fake provider login failed: %v", err)
	}
	return result.Grant, f.actorFor(result.Grant)
}
func (f *oauthFixture) actorFor(grant auth.Grant) auth.Actor {
	f.t.Helper()
	actor, err := f.auth.Resolve(f.t.Context(), grant.Token)
	if err != nil {
		f.t.Fatal("session resolution failed")
	}
	return actor
}
func (f *oauthFixture) scalar(query string, args ...any) int {
	f.t.Helper()
	var n int
	if err := f.owner.QueryRow(f.t.Context(), query, args...).Scan(&n); err != nil {
		f.t.Fatal("fixture scalar failed")
	}
	return n
}

func TestIntegrationOAuthIdentityAndExistingAuth(t *testing.T) {
	f := newOAuthFixture(t)
	ctx := t.Context()
	for _, kind := range []auth.Provider{auth.Google, auth.GitHub} {
		identity := fakeIdentity(kind)
		grant, actor := f.login(identity)
		if actor.AuthMethod != string(kind) || !grant.Me.EmailVerified || grant.Me.Profile.Handle != nil || grant.Me.Profile.Bio != nil || grant.Me.SearchEngineIndexing {
			t.Fatal("OAuth account defaults differ")
		}
		if f.scalar("SELECT count(*) FROM app.password_credentials WHERE user_id=$1", grant.Me.ID.String()) != 0 {
			t.Fatal("fake password credential created")
		}
		if f.scalar("SELECT count(*) FROM app.auth_identities WHERE user_id=$1", grant.Me.ID.String()) != 2 {
			t.Fatal("OAuth email identity missing")
		}
		methods, err := f.auth.Methods(ctx, actor)
		if err != nil || methods.Password || len(methods.Providers) != 1 {
			t.Fatal("OAuth-only methods differ")
		}
		name := "User-edited name"
		if _, err = f.identity.UpdateProfile(ctx, actor.UserID, identityProfileName(name)); err != nil {
			t.Fatal("OAuth-only profile update failed")
		}
		changed := identity
		changed.Email = emailFixture()
		changed.DisplayName = "Changed upstream"
		again, _ := f.login(changed)
		if again.Me.ID != grant.Me.ID || again.Me.Email != grant.Me.Email || *again.Me.Profile.DisplayName != name {
			t.Fatal("provider metadata overwrote canonical account")
		}
		other := identity
		other.Subject = oauth2.GenerateVerifier()
		if _, err = f.complete(f.begin(kind, auth.OAuthLogin, nil), other, nil); !errors.Is(err, auth.ErrAccountLinkRequired) {
			t.Fatal("same email auto-linked")
		}
		if _, err = f.auth.Reauthenticate(ctx, actor, testPassword); !errors.Is(err, auth.ErrReauthFailed) {
			t.Fatal("OAuth-only password reauth unsafe")
		}
		before := f.scalar("SELECT count(*) FROM app.auth_challenges WHERE user_id=$1", grant.Me.ID.String())
		f.auth.RequestPasswordReset(ctx, grant.Me.Email)
		if f.scalar("SELECT count(*) FROM app.auth_challenges WHERE user_id=$1", grant.Me.ID.String()) != before {
			t.Fatal("OAuth-only reset created password path")
		}
		if err = f.auth.RevokeOthers(ctx, actor); err != nil {
			t.Fatal("OAuth-only revoke others failed")
		}
		if _, err = f.auth.Resolve(ctx, again.Token); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatal("OAuth-only session remained active")
		}
		if err = f.auth.Logout(ctx, actor); err != nil {
			t.Fatal("OAuth-only logout failed")
		}
	}
	google := fakeIdentity(auth.Google)
	google.EmailAuthoritative = false
	grant, actor := f.login(google)
	if grant.Me.EmailVerified {
		t.Fatal("third-party Google email trusted locally")
	}
	if err := f.auth.RequestVerification(ctx, actor); err != nil {
		t.Fatal("OAuth-only email verification request failed")
	}
	if err := f.auth.VerifyEmail(ctx, f.lastToken("email_verify")); err != nil {
		t.Fatal("OAuth-only email verification failed")
	}
	me, err := f.identity.Me(ctx, actor.UserID)
	if err != nil || !me.EmailVerified {
		t.Fatal("OAuth-only email not verified")
	}
	github := fakeIdentity(auth.GitHub)
	github.Email = ""
	github.EmailAuthoritative = false
	if _, err = f.complete(f.begin(auth.GitHub, auth.OAuthLogin, nil), github, nil); !errors.Is(err, auth.ErrProviderInvalid) {
		t.Fatal("GitHub account created without verified email")
	}
	local, err := f.auth.Register(ctx, emailFixture(), testPassword)
	if err != nil {
		t.Fatal("local fixture failed")
	}
	collision := fakeIdentity(auth.Google)
	collision.Email = local.Me.Email
	if _, err = f.complete(f.begin(auth.Google, auth.OAuthLogin, nil), collision, nil); !errors.Is(err, auth.ErrAccountLinkRequired) {
		t.Fatal("local account auto-linked")
	}
	if f.scalar("SELECT count(*) FROM app.auth_identities WHERE user_id=$1", local.Me.ID.String()) != 1 {
		t.Fatal("email collision mutated local account")
	}
}

func identityProfileName(name string) identity.ProfileUpdate {
	return identity.ProfileUpdate{DisplayName: identity.Field[string]{Set: true, Value: &name}}
}

func TestIntegrationOAuthOnlySecondMethodAndHTTPRotation(t *testing.T) {
	f := newOAuthFixture(t)
	ctx := t.Context()
	github := fakeIdentity(auth.GitHub)
	initial, actor := f.login(github)
	session := &http.Cookie{Name: "tap4furry_session", Value: initial.Token}
	f.request("GET", "/me", nil, session, 200)
	f.request("PATCH", "/me/profile", map[string]any{"display_name": "My own name"}, session, 200)
	f.request("GET", "/me/sessions", nil, session, 200)
	body, binding := f.request("POST", "/me/auth-methods/google/link", nil, session, 200)
	startURL := body["authorization_url"].(string)
	parsed, _ := url.Parse(startURL)
	google := fakeIdentity(auth.Google)
	code := f.providers[auth.Google].issue(startURL, google)
	req := httptest.NewRequest("GET", "/auth/oauth/google/callback?"+url.Values{"state": {parsed.Query().Get("state")}, "code": {code}}.Encode(), nil)
	req.AddCookie(session)
	req.AddCookie(binding)
	response, err := f.app.Test(req)
	if err != nil {
		t.Fatal("link HTTP callback failed")
	}
	_ = response.Body.Close()
	if response.Header.Get("Location") != testOrigin+"/account" {
		t.Fatal("link callback did not return account")
	}
	for _, cookie := range response.Cookies() {
		if cookie.Name == "tap4furry_session" {
			session = cookie
		}
	}
	rotated, err := f.auth.Resolve(ctx, session.Value)
	if err != nil || rotated.AuthMethod != "github" || rotated.SessionID == actor.SessionID {
		t.Fatal("OAuth-only link did not preserve current method")
	}
	req = httptest.NewRequest("POST", "/me/auth-methods/google/reauthenticate", nil)
	req.AddCookie(session)
	req.Header.Set("Origin", testOrigin)
	req.Header.Set("X-CSRF-Token", csrfToken("tap4furry-development-only-csrf-secret", initial.Token))
	response, err = f.app.Test(req)
	if err != nil {
		t.Fatal("old CSRF test failed")
	}
	_ = response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("old CSRF survived link rotation")
	}
	reauthed, err := f.complete(f.begin(auth.GitHub, auth.OAuthReauth, &rotated), github, &rotated)
	if err != nil {
		t.Fatal("GitHub reauth failed")
	}
	unlinked, err := f.auth.UnlinkProvider(ctx, f.actorFor(reauthed.Grant), auth.Google)
	if err != nil {
		t.Fatal("two-provider unlink failed")
	}
	if f.actorFor(unlinked).AuthMethod != "github" {
		t.Fatal("remaining OAuth method changed")
	}
	if _, err = f.auth.UnlinkProvider(ctx, f.actorFor(unlinked), auth.GitHub); !errors.Is(err, auth.ErrLastMethod) {
		t.Fatal("only GitHub removed")
	}
	_, googleActor := f.login(fakeIdentity(auth.Google))
	if _, err = f.auth.UnlinkProvider(ctx, googleActor, auth.Google); !errors.Is(err, auth.ErrLastMethod) {
		t.Fatal("only Google removed")
	}
}

func TestIntegrationOAuthLinkReauthUnlink(t *testing.T) {
	f := newOAuthFixture(t)
	ctx := t.Context()
	local, err := f.auth.Register(ctx, emailFixture(), testPassword)
	if err != nil {
		t.Fatal("local fixture failed")
	}
	actor := f.actorFor(local)
	google := fakeIdentity(auth.Google)
	linked, err := f.complete(f.begin(auth.Google, auth.OAuthLink, &actor), google, &actor)
	if err != nil || linked.Grant.Me.ID != local.Me.ID || linked.Grant.Me.Email != local.Me.Email {
		t.Fatal("explicit link failed or changed email")
	}
	current := f.actorFor(linked.Grant)
	if current.AuthMethod != "password" || !current.AuthenticatedAt.Equal(actor.AuthenticatedAt) || current.SessionID == actor.SessionID {
		t.Fatal("link rotation/freshness differs")
	}
	if _, err = f.auth.Resolve(ctx, local.Token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("link retained old cookie")
	}
	if _, err = f.complete(f.begin(auth.Google, auth.OAuthReauth, &current), fakeIdentity(auth.Google), &current); !errors.Is(err, auth.ErrReauthFailed) {
		t.Fatal("reauth accepted unlinked subject")
	}
	reauthed, err := f.complete(f.begin(auth.Google, auth.OAuthReauth, &current), google, &current)
	if err != nil {
		t.Fatal("provider reauth failed")
	}
	googleActor := f.actorFor(reauthed.Grant)
	if googleActor.AuthMethod != "google" || !googleActor.AuthenticatedAt.After(current.AuthenticatedAt) {
		t.Fatal("provider reauth did not refresh")
	}
	if _, err = f.auth.UnlinkProvider(ctx, googleActor, auth.Google); !errors.Is(err, auth.ErrReauthRequired) {
		t.Fatal("current provider self-unlinked")
	}
	otherGoogle, _ := f.login(google)
	passwordGrant, err := f.auth.Reauthenticate(ctx, googleActor, testPassword)
	if err != nil {
		t.Fatal("remaining password reauth failed")
	}
	passwordActor := f.actorFor(passwordGrant)
	unlinked, err := f.auth.UnlinkProvider(ctx, passwordActor, auth.Google)
	if err != nil {
		t.Fatal("unlink after remaining-method reauth failed")
	}
	if _, err = f.auth.Resolve(ctx, otherGoogle.Token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("removed-provider session remained")
	}
	if f.actorFor(unlinked).AuthMethod != "password" {
		t.Fatal("unlink changed remaining method")
	}
	methods, err := f.auth.Methods(ctx, f.actorFor(unlinked))
	if err != nil || !methods.Password || len(methods.Providers) != 0 {
		t.Fatal("unlink methods differ")
	}
	github := fakeIdentity(auth.GitHub)
	only, onlyActor := f.login(github)
	if _, err = f.auth.UnlinkProvider(ctx, onlyActor, auth.GitHub); !errors.Is(err, auth.ErrLastMethod) {
		t.Fatal("last method removed")
	}
	foreign := fakeIdentity(auth.Google)
	f.login(foreign)
	if _, err = f.complete(f.begin(auth.Google, auth.OAuthLink, &onlyActor), foreign, &onlyActor); !errors.Is(err, auth.ErrProviderAlreadyLinked) {
		t.Fatal("foreign provider identity moved")
	}
	f.exec("UPDATE app.sessions SET authenticated_at=now()-interval '16 minutes' WHERE id=$1", onlyActor.SessionID.String())
	if _, err = f.auth.BeginOAuth(ctx, auth.Google, auth.OAuthLink, &onlyActor); !errors.Is(err, auth.ErrReauthRequired) {
		t.Fatal("stale link allowed")
	}
	if _, err = f.auth.UnlinkProvider(ctx, onlyActor, auth.GitHub); !errors.Is(err, auth.ErrReauthRequired) {
		t.Fatal("stale unlink allowed")
	}
	if _, err = f.auth.Resolve(ctx, only.Token); err != nil {
		t.Fatal("stale freshness broke normal session")
	}
	if _, err = f.auth.BeginOAuth(ctx, auth.Google, auth.OAuthLink, &onlyActor); !errors.Is(err, auth.ErrReauthRequired) {
		t.Fatal("activity refreshed freshness")
	}
	refreshed, err := f.complete(f.begin(auth.GitHub, auth.OAuthReauth, &onlyActor), github, &onlyActor)
	if err != nil {
		t.Fatal("stale session could not reauthenticate")
	}
	if f.actorFor(refreshed.Grant).AuthenticatedAt.Before(time.Now().Add(-time.Minute)) {
		t.Fatal("reauth timestamp stale")
	}
}

func TestIntegrationOAuthHTTPStateAndCSRF(t *testing.T) {
	f := newOAuthFixture(t)
	kind := auth.Google
	req := httptest.NewRequest("GET", "/auth/oauth/google/start?return_to=https://evil.invalid", nil)
	response, err := f.app.Test(req)
	if err != nil {
		t.Fatal("OAuth HTTP start failed")
	}
	_ = response.Body.Close()
	if response.StatusCode != 302 || response.Header.Get("Referrer-Policy") != "no-referrer" || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("OAuth start headers differ")
	}
	cookies := response.Cookies()
	if len(cookies) != 1 || cookies[0].Name != "tap4furry_oauth_google" || !cookies[0].HttpOnly || cookies[0].MaxAge != 600 || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatal("flow cookie unsafe")
	}
	parsed, _ := url.Parse(response.Header.Get("Location"))
	state := parsed.Query().Get("state")
	identity := fakeIdentity(kind)
	code := f.providers[kind].issue(response.Header.Get("Location"), identity)
	callback := "/auth/oauth/google/callback?" + url.Values{"state": {state}, "code": {code}, "return_to": {"https://evil.invalid"}}.Encode()
	req = httptest.NewRequest("GET", callback, nil)
	req.AddCookie(cookies[0])
	response, err = f.app.Test(req)
	if err != nil {
		t.Fatal("OAuth HTTP callback failed")
	}
	_ = response.Body.Close()
	if response.StatusCode != 302 || response.Header.Get("Location") != testOrigin+"/account" || response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("callback did not use safe fixed redirect")
	}
	var session *http.Cookie
	for _, cookie := range response.Cookies() {
		if cookie.Name == "tap4furry_session" {
			session = cookie
		}
	}
	if session == nil || !session.HttpOnly {
		t.Fatal("callback session missing")
	}
	methods, _ := f.request("GET", "/me/auth-methods", nil, session, 200)
	assertKeys(t, methods, []string{"password", "providers"})
	method := methods["providers"].([]any)[0].(map[string]any)
	assertKeys(t, method, []string{"provider", "email", "linked_at"})
	response, err = f.app.Test(req)
	if err != nil {
		t.Fatal("replay HTTP failed")
	}
	_ = response.Body.Close()
	if response.Header.Get("Location") != testOrigin+"/login?oauth_error=provider_invalid" {
		t.Fatal("callback replay not rejected safely")
	}
	for _, path := range []string{"/me/auth-methods/github/link", "/me/auth-methods/google/reauthenticate", "/me/auth-methods/google"} {
		verb := "POST"
		if path == "/me/auth-methods/google" {
			verb = "DELETE"
		}
		for _, origin := range []string{"https://evil.invalid", testOrigin} {
			req := httptest.NewRequest(verb, path, nil)
			req.AddCookie(session)
			req.Header.Set("Origin", origin)
			response, err := f.app.Test(req)
			if err != nil {
				t.Fatal("CSRF fixture failed")
			}
			_ = response.Body.Close()
			if response.StatusCode != 403 {
				t.Fatal("Origin/CSRF guard bypassed")
			}
		}
	}
	for _, failure := range []string{"browser", "provider", "expired", "denied"} {
		start := f.begin(kind, auth.OAuthLogin, nil)
		input := auth.OAuthCallback{Provider: kind, State: start.State, Code: f.providers[kind].issue(start.URL, identity), BrowserState: auth.OAuthStateDigest(start.State)}
		expected := auth.ErrProviderInvalid
		switch failure {
		case "browser":
			input.BrowserState = ""
		case "provider":
			input.Provider = auth.GitHub
		case "expired":
			flow, e := f.store.ConsumeOAuthFlow(t.Context(), start.State)
			if e != nil {
				t.Fatal("flow fixture failed")
			}
			flow.CreatedAt = time.Now().Add(-11 * time.Minute)
			if e = f.store.PutOAuthFlow(t.Context(), start.State, flow); e != nil {
				t.Fatal("expired fixture failed")
			}
		case "denied":
			input.Denied = true
			expected = auth.ErrProviderDenied
		}
		if _, err = f.auth.CompleteOAuth(t.Context(), input); !errors.Is(err, expected) {
			t.Fatal("invalid OAuth flow accepted")
		}
		if _, err = f.store.ConsumeOAuthFlow(t.Context(), start.State); !errors.Is(err, auth.ErrProviderInvalid) {
			t.Fatal("invalid flow was not consumed")
		}
	}
	start := f.begin(kind, auth.OAuthLogin, nil)
	req = httptest.NewRequest("GET", "/auth/oauth/google/callback?"+url.Values{"state": {start.State}, "error": {"access_denied"}, "error_description": {"untrusted-provider-detail"}}.Encode(), nil)
	req.AddCookie(&http.Cookie{Name: cookies[0].Name, Value: auth.OAuthStateDigest(start.State)})
	response, err = f.app.Test(req)
	if err != nil {
		t.Fatal("denial HTTP failed")
	}
	_ = response.Body.Close()
	if response.Header.Get("Location") != testOrigin+"/login?oauth_error=provider_denied" {
		t.Fatal("upstream error forwarded")
	}
	if err = f.store.Close(); err != nil {
		t.Fatal("Redis outage fixture failed")
	}
	if _, err = f.auth.BeginOAuth(t.Context(), kind, auth.OAuthLogin, nil); !errors.Is(err, auth.ErrProviderUnavailable) {
		t.Fatal("Redis outage did not fail OAuth")
	}
	if _, err = f.auth.Register(t.Context(), emailFixture(), testPassword); err != nil {
		t.Fatal("Redis outage broke local auth")
	}
}

// Prevent tests from accidentally treating opaque provider subjects or tokens as
// public JSON. SQL reads only the disposable database and never prints contents.
func TestIntegrationOAuthPersistencePrivacy(t *testing.T) {
	f := newOAuthFixture(t)
	identity := fakeIdentity(auth.Google)
	start := f.begin(auth.Google, auth.OAuthLogin, nil)
	code := f.providers[auth.Google].issue(start.URL, identity)
	result, err := f.auth.CompleteOAuth(t.Context(), auth.OAuthCallback{Provider: auth.Google, State: start.State, Code: code, BrowserState: auth.OAuthStateDigest(start.State)})
	if err != nil {
		t.Fatal("privacy callback failed")
	}
	for _, table := range []string{"users", "user_profiles", "auth_identities", "password_credentials", "sessions", "auth_challenges", "security_events"} {
		rows, err := f.owner.Query(t.Context(), "SELECT row_to_json(t)::text FROM app."+table+" t")
		if err != nil {
			t.Fatal("privacy read failed")
		}
		for rows.Next() {
			var body string
			if rows.Scan(&body) != nil {
				t.Fatal("privacy scan failed")
			}
			for _, value := range []string{start.State, code, result.Grant.Token, "transient-access-fixture", "transient-refresh-fixture"} {
				if strings.Contains(body, value) {
					t.Error("raw token persisted")
				}
			}
		}
		rows.Close()
	}
	actor := f.actorFor(result.Grant)
	methods, err := f.auth.Methods(t.Context(), actor)
	if err != nil {
		t.Fatal("methods failed")
	}
	raw, err := json.Marshal(methods)
	if err != nil || strings.Contains(string(raw), identity.Subject) {
		t.Fatal("subject escaped private owner view")
	}
	if f.scalar("SELECT count(*) FROM app.security_events WHERE user_id=$1 AND event_type='oauth_login_succeeded'", actor.UserID.String()) != 1 {
		t.Fatal("OAuth security event missing")
	}
}
