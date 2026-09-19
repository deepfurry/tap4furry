package oauthprovider

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/config"
	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/deepfurry/tap4furry/server/internal/identity"
	"github.com/deepfurry/tap4furry/server/internal/mail"
	"github.com/deepfurry/tap4furry/server/internal/redisstore"
	"github.com/deepfurry/tap4furry/server/internal/transport/public"
	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/gofiber/fiber/v3"
	"golang.org/x/oauth2"
)

// Runs the real transport, Auth, Redis, provider adapters, OIDC verifier and SQL
// together. Only remote IdPs are replaced with local HTTP/JWKS fixtures.
func TestIntegrationOAuthProviderE2E(t *testing.T) {
	if os.Getenv("GFP_AUTH_INTEGRATION") != "1" {
		t.Skip("disposable integration not enabled")
	}
	if os.Getenv("CI") != "true" || os.Getenv("GFP_DISPOSABLE_INFRA") != "1" {
		t.Fatal("disposable guards required")
	}
	pool, err := database.Open(t.Context(), "postgres://gfp_api:gfp_ci_only@127.0.0.1:5432/gfp_ci?sslmode=disable")
	if err != nil {
		t.Fatal("disposable API pool failed")
	}
	defer pool.Close()
	owner, err := database.Open(t.Context(), "postgres://gfp_migrator:gfp_ci_only@127.0.0.1:5432/gfp_ci?sslmode=disable")
	if err != nil {
		t.Fatal("disposable owner pool failed")
	}
	defer owner.Close()
	store, err := redisstore.Open("redis://gfp_runtime:gfp_ci_only@127.0.0.1:6379/0", "gfp:")
	if err != nil {
		t.Fatal("disposable Redis failed")
	}
	defer store.Close()
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer slog.SetDefault(previous)
	for _, kind := range []auth.Provider{auth.Google, auth.GitHub} {
		t.Run(string(kind), func(t *testing.T) {
			key, err := rsa.GenerateKey(rand.Reader, 2048)
			if err != nil {
				t.Fatal("ephemeral signing key failed")
			}
			signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, nil)
			if err != nil {
				t.Fatal("signer failed")
			}
			code, access, refresh := oauth2.GenerateVerifier(), oauth2.GenerateVerifier(), oauth2.GenerateVerifier()
			email := uuid.NewV7().String() + "@example.invalid"
			subject := uuid.NewV7().String()
			numericID := time.Now().UnixNano()
			var nonce, challenge, idToken string
			var upstream *httptest.Server
			upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/.well-known/openid-configuration":
					_ = json.NewEncoder(w).Encode(map[string]any{"issuer": upstream.URL, "authorization_endpoint": upstream.URL + "/authorize", "token_endpoint": upstream.URL + "/token", "jwks_uri": upstream.URL + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}})
				case "/keys":
					_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, Use: "sig", Algorithm: "RS256"}}})
				case "/token":
					_ = r.ParseForm()
					if r.Form.Get("code") != code || r.Form.Get("client_secret") != "fixture-secret" || oauth2.S256ChallengeFromVerifier(r.Form.Get("code_verifier")) != challenge {
						w.WriteHeader(400)
						return
					}
					tokenResponse := map[string]any{"token_type": "bearer", "access_token": access, "refresh_token": refresh, "expires_in": 3600}
					if kind == auth.Google {
						var signErr error
						idToken, signErr = jwt.Signed(signer).Claims(map[string]any{"iss": upstream.URL, "aud": "fixture-client", "sub": subject, "email": email, "email_verified": true, "nonce": nonce, "exp": time.Now().Add(time.Hour).Unix()}).Serialize()
						if signErr != nil {
							w.WriteHeader(500)
							return
						}
						tokenResponse["id_token"] = idToken
					}
					_ = json.NewEncoder(w).Encode(tokenResponse)
				case "/user":
					if r.Header.Get("Authorization") != "Bearer "+access {
						w.WriteHeader(401)
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"id": numericID, "login": "presentation-only"})
				case "/user/emails":
					if r.Header.Get("Authorization") != "Bearer "+access {
						w.WriteHeader(401)
						return
					}
					_ = json.NewEncoder(w).Encode([]map[string]any{{"email": email, "verified": true, "primary": true}})
				default:
					w.WriteHeader(404)
				}
			}))
			defer upstream.Close()
			c := config.Config{PublicOrigin: "http://localhost:4321", GoogleOAuth: config.OAuthCredentials{ClientID: "fixture-client", ClientSecret: "fixture-secret"}, GitHubOAuth: config.OAuthCredentials{ClientID: "fixture-client", ClientSecret: "fixture-secret"}}
			adapter := New(c)[kind].(*provider)
			adapter.issuer, adapter.api, adapter.oauth.Endpoint.TokenURL = upstream.URL, upstream.URL, upstream.URL+"/token"
			// All changes are private fixture fields; production endpoints remain frozen.
			authentication, err := auth.New(pool, mail.Disabled{}, auth.OAuthConfig{Flows: store, Providers: map[auth.Provider]auth.OAuthProvider{kind: adapter}})
			if err != nil {
				t.Fatal("authentication setup failed")
			}
			app := fiber.New()
			public.Register(app, nil, authentication, identity.New(pool), public.Options{Environment: "test", PublicOrigin: c.PublicOrigin, CSRFSecret: config.DevelopmentCSRFSecret})
			response, err := app.Test(httptest.NewRequest("GET", "/auth/oauth/"+string(kind)+"/start", nil))
			if err != nil {
				t.Fatal("start failed")
			}
			_ = response.Body.Close()
			if response.StatusCode != 302 || len(response.Cookies()) != 1 {
				t.Fatal("start contract failed")
			}
			authorization, _ := url.Parse(response.Header.Get("Location"))
			query := authorization.Query()
			nonce, challenge = query.Get("nonce"), query.Get("code_challenge")
			state := query.Get("state")
			request := httptest.NewRequest("GET", "/auth/oauth/"+string(kind)+"/callback?"+url.Values{"state": {state}, "code": {code}}.Encode(), nil)
			request.AddCookie(response.Cookies()[0])
			response, err = app.Test(request, fiber.TestConfig{Timeout: 10 * time.Second})
			if err != nil {
				t.Fatal("callback failed")
			}
			_ = response.Body.Close()
			if response.StatusCode != 302 || response.Header.Get("Location") != c.PublicOrigin+"/account" {
				t.Fatal("full OAuth callback rejected")
			}
			var session *http.Cookie
			for _, cookie := range response.Cookies() {
				if cookie.Name == "tap4furry_session" {
					session = cookie
				}
			}
			if session == nil {
				t.Fatal("callback cookie missing")
			}
			actor, err := authentication.Resolve(t.Context(), session.Value)
			if err != nil || actor.AuthMethod != string(kind) {
				t.Fatal("canonical provider session missing")
			}
			if _, err = store.ConsumeOAuthFlow(t.Context(), state); !errors.Is(err, auth.ErrProviderInvalid) {
				t.Fatal("flow persisted after callback")
			}
			secrets := []string{state, code, access, refresh, idToken, session.Value, nonce}
			for _, table := range []string{"users", "user_profiles", "auth_identities", "password_credentials", "sessions", "auth_challenges", "security_events"} {
				column := "user_id"
				if table == "users" {
					column = "id"
				}
				var rows string
				if err = owner.QueryRow(t.Context(), "SELECT coalesce(json_agg(t)::text,'[]') FROM app."+table+" t WHERE "+column+"=$1", actor.UserID.String()).Scan(&rows); err != nil {
					t.Fatal("persistence inspection failed")
				}
				assertNoOAuthSecrets(t, rows, secrets)
			}
			for _, path := range []string{"/me", "/me/auth-methods", "/me/sessions"} {
				request := httptest.NewRequest("GET", path, nil)
				request.AddCookie(session)
				response, err := app.Test(request)
				if err != nil || response.StatusCode != 200 {
					t.Fatal("owner API failed")
				}
				var body json.RawMessage
				if json.NewDecoder(response.Body).Decode(&body) != nil {
					t.Fatal("owner API malformed")
				}
				_ = response.Body.Close()
				assertNoOAuthSecrets(t, string(body), secrets)
			}
			assertNoOAuthSecrets(t, logs.String(), secrets)
		})
	}
}
func assertNoOAuthSecrets(t *testing.T, body string, values []string) {
	t.Helper()
	for _, value := range values {
		if value != "" && strings.Contains(body, value) {
			t.Fatal("raw OAuth material escaped its boundary (values withheld)")
		}
	}
}
