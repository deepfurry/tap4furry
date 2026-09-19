package oauthprovider

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/config"
	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"golang.org/x/oauth2"
)

func TestAuthorizationContract(t *testing.T) {
	for _, origin := range []string{"http://localhost:4321", "https://tap4furry.com"} {
		providers := New(config.Config{PublicOrigin: origin, GoogleOAuth: config.OAuthCredentials{ClientID: "fake-google", ClientSecret: "fixture-secret"}, GitHubOAuth: config.OAuthCredentials{ClientID: "fake-github", ClientSecret: "fixture-secret"}})
		for kind, p := range providers {
			state := oauth2.GenerateVerifier()
			start, err := p.AuthorizationURL(t.Context(), state, auth.OAuthReauth)
			if err != nil {
				t.Fatal("authorization failed")
			}
			parsed, _ := url.Parse(start.URL)
			q := parsed.Query()
			if q.Get("state") != state || q.Get("redirect_uri") != origin+"/api/auth/oauth/"+string(kind)+"/callback" || q.Get("code_challenge") != oauth2.S256ChallengeFromVerifier(start.Verifier) || q.Get("code_challenge_method") != "S256" || q.Get("client_secret") != "" || q.Get("access_type") == "offline" || q.Get("prompt") != "select_account" {
				t.Fatal("authorization security contract failed")
			}
			scope := "read:user user:email"
			if kind == auth.Google {
				scope = "openid email profile"
				if q.Get("nonce") != start.Nonce || len(start.Nonce) != 43 {
					t.Fatal("nonce missing")
				}
			}
			if q.Get("scope") != scope {
				t.Fatal("unneeded provider scope")
			}
		}
	}
}

func TestGoogleOIDCValidationAndPKCE(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal("fixture key generation failed")
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		t.Fatal("fixture signer failed")
	}
	var claims map[string]any
	var server *httptest.Server
	var challenge string
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": server.URL, "authorization_endpoint": server.URL + "/authorize", "token_endpoint": server.URL + "/token", "jwks_uri": server.URL + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/keys":
			_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, Algorithm: "RS256", Use: "sig"}}})
		case "/token":
			if r.ParseForm() != nil || r.Form.Get("client_id") != "fake-google" || r.Form.Get("client_secret") != "fixture-secret" || r.Form.Get("redirect_uri") != "http://localhost:4321/api/auth/oauth/google/callback" || r.Form.Get("code_verifier") == "" || oauth2.S256ChallengeFromVerifier(r.Form.Get("code_verifier")) != challenge {
				w.WriteHeader(400)
				_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
				return
			}
			raw, signErr := jwt.Signed(signer).Claims(claims).Serialize()
			if signErr != nil {
				w.WriteHeader(500)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "transient-access-fixture", "refresh_token": "transient-refresh-fixture", "token_type": "Bearer", "expires_in": 3600, "id_token": raw})
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	p := New(config.Config{PublicOrigin: "http://localhost:4321", GoogleOAuth: config.OAuthCredentials{ClientID: "fake-google", ClientSecret: "fixture-secret"}})[auth.Google].(*provider)
	p.issuer, p.oauth.Endpoint.TokenURL, p.client = server.URL, server.URL+"/token", server.Client()
	ctx := context.WithValue(t.Context(), oauth2.HTTPClient, p.client)
	start, _ := p.AuthorizationURL(ctx, oauth2.GenerateVerifier(), auth.OAuthLogin)
	challenge = oauth2.S256ChallengeFromVerifier(start.Verifier)
	flow := auth.OAuthFlow{Provider: auth.Google, Verifier: start.Verifier, Nonce: start.Nonce}
	valid := func() map[string]any {
		return map[string]any{"iss": server.URL, "aud": "fake-google", "sub": "stable-google-subject", "exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(), "nonce": start.Nonce, "email": "fixture@gmail.com", "email_verified": true, "name": "Friendly fox"}
	}
	claims = valid()
	got, err := p.Exchange(ctx, "fixture-code", flow)
	if err != nil || got.Subject != "stable-google-subject" || !got.EmailAuthoritative {
		t.Fatal("valid signed OIDC callback rejected")
	}
	for _, test := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"issuer", func(c map[string]any) { c["iss"] = "https://wrong.invalid" }},
		{"audience", func(c map[string]any) { c["aud"] = "other-client" }},
		{"expired", func(c map[string]any) { c["exp"] = time.Now().Add(-time.Hour).Unix() }},
		{"nonce", func(c map[string]any) { c["nonce"] = "wrong" }},
		{"subject", func(c map[string]any) { delete(c, "sub") }},
		{"email", func(c map[string]any) { delete(c, "email") }},
		{"azp", func(c map[string]any) { c["azp"] = "other-client" }},
		{"multiple-audiences", func(c map[string]any) { c["aud"] = []string{"fake-google", "other"} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			claims = valid()
			test.change(claims)
			if _, err = p.Exchange(ctx, "fixture-code", flow); err == nil {
				t.Fatal("invalid OIDC identity accepted")
			}
		})
	}
	claims = valid()
	claims["email"] = "third-party@example.invalid"
	got, err = p.Exchange(ctx, "fixture-code", flow)
	if err != nil || got.EmailAuthoritative || !got.EmailVerified {
		t.Fatal("third-party email incorrectly authoritative")
	}
	claims["hd"] = "example.invalid"
	got, err = p.Exchange(ctx, "fixture-code", flow)
	if err != nil || !got.EmailAuthoritative {
		t.Fatal("verified workspace not authoritative")
	}
	claims["email_verified"] = false
	got, err = p.Exchange(ctx, "fixture-code", flow)
	if err != nil || got.EmailAuthoritative {
		t.Fatal("unverified email trusted")
	}
	for _, verifier := range []string{"", oauth2.GenerateVerifier()} {
		bad := flow
		bad.Verifier = verifier
		if _, err = p.Exchange(ctx, "fixture-code", bad); err == nil {
			t.Fatal("missing/wrong PKCE verifier accepted")
		}
	}
	claims = valid()
	raw, err := jwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		t.Fatal("sign fixture")
	}
	parts := strings.Split(raw, ".")
	parts[2] = strings.Repeat("A", len(parts[2]))
	if _, err = p.googleIdentity(ctx, strings.Join(parts, "."), flow.Nonce); err == nil {
		t.Fatal("invalid signature accepted")
	}
}

func TestGitHubExchangeAndIdentity(t *testing.T) {
	var emails any = []map[string]any{{"email": "unverified@example.invalid", "primary": true, "verified": false}, {"email": "verified@example.invalid", "verified": true}}
	var user any = map[string]any{"id": int64(9007199254740993), "login": "mutable-login", "email": "untrusted@example.invalid", "name": "Fox"}
	var challenge string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/token" {
			_ = r.ParseForm()
			if r.Form.Get("code_verifier") == "" || oauth2.S256ChallengeFromVerifier(r.Form.Get("code_verifier")) != challenge || r.Form.Get("client_secret") != "fixture-secret" {
				w.WriteHeader(400)
				_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "transient-access-fixture", "refresh_token": "transient-refresh-fixture", "token_type": "bearer"})
			return
		}
		if r.Header.Get("Authorization") != "Bearer transient-access-fixture" || r.Header.Get("X-GitHub-Api-Version") != "2026-03-10" {
			w.WriteHeader(401)
			return
		}
		if r.URL.Path == "/user" {
			_ = json.NewEncoder(w).Encode(user)
		} else if r.URL.Path == "/user/emails" {
			_ = json.NewEncoder(w).Encode(emails)
		} else {
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	p := New(config.Config{PublicOrigin: "http://localhost:4321", GitHubOAuth: config.OAuthCredentials{ClientID: "fake-github", ClientSecret: "fixture-secret"}})[auth.GitHub].(*provider)
	p.api, p.oauth.Endpoint.TokenURL, p.client = server.URL, server.URL+"/token", server.Client()
	start, _ := p.AuthorizationURL(t.Context(), oauth2.GenerateVerifier(), auth.OAuthLogin)
	challenge = oauth2.S256ChallengeFromVerifier(start.Verifier)
	flow := auth.OAuthFlow{Provider: auth.GitHub, Verifier: start.Verifier}
	got, err := p.Exchange(t.Context(), "fixture-code", flow)
	if err != nil || got.Subject != "9007199254740993" || got.Email != "verified@example.invalid" || !got.EmailAuthoritative {
		t.Fatal("numeric ID or verified-email selection failed")
	}
	emails = []map[string]any{{"email": "secondary@example.invalid", "verified": true}, {"email": "primary@example.invalid", "primary": true, "verified": true}}
	got, err = p.Exchange(t.Context(), "fixture-code", flow)
	if err != nil || got.Email != "primary@example.invalid" {
		t.Fatal("primary verified email not preferred")
	}
	emails = []map[string]any{{"email": "unverified@example.invalid", "primary": true, "verified": false}}
	got, err = p.Exchange(t.Context(), "fixture-code", flow)
	if err != nil || got.Email != "" {
		t.Fatal("unverified GitHub email used")
	}
	for _, id := range []any{nil, 0, -1, "123", 12.5} {
		user = map[string]any{"id": id}
		if _, err = p.Exchange(t.Context(), "fixture-code", flow); err == nil {
			t.Fatal("invalid numeric identity accepted")
		}
	}
	for _, verifier := range []string{"", oauth2.GenerateVerifier()} {
		bad := flow
		bad.Verifier = verifier
		if _, err = p.Exchange(t.Context(), "fixture-code", bad); err == nil {
			t.Fatal("invalid GitHub PKCE accepted")
		}
	}
}
