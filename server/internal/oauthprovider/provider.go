// Package oauthprovider implements Auth's provider boundary. Provider tokens
// live only in a callback's stack; only a validated identity leaves this adapter.
package oauthprovider

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/config"
	"golang.org/x/oauth2"
)

const googleIssuer = "https://accounts.google.com"

type provider struct {
	kind        auth.Provider
	oauth       oauth2.Config
	client      *http.Client
	issuer, api string
	mu          sync.Mutex
	verifier    *oidc.IDTokenVerifier
}

// Endpoints are fixed by this adapter, never caller input or environment values.
// Google discovery is lazy: an IdP outage cannot disable local password auth.
func New(c config.Config) map[auth.Provider]auth.OAuthProvider {
	result := make(map[auth.Provider]auth.OAuthProvider)
	for _, pair := range []struct {
		kind        auth.Provider
		credentials config.OAuthCredentials
	}{{auth.Google, c.GoogleOAuth}, {auth.GitHub, c.GitHubOAuth}} {
		if !pair.credentials.Enabled() {
			continue
		}
		callback, _ := c.OAuthCallback(string(pair.kind))
		p := &provider{kind: pair.kind, issuer: googleIssuer, api: "https://api.github.com", client: &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
		p.oauth = oauth2.Config{ClientID: pair.credentials.ClientID, ClientSecret: pair.credentials.ClientSecret, RedirectURL: callback}
		if pair.kind == auth.Google {
			p.oauth.Endpoint = oauth2.Endpoint{AuthURL: "https://accounts.google.com/o/oauth2/v2/auth", TokenURL: "https://oauth2.googleapis.com/token", AuthStyle: oauth2.AuthStyleInParams}
			p.oauth.Scopes = []string{"openid", "email", "profile"}
		} else {
			p.oauth.Endpoint = oauth2.Endpoint{AuthURL: "https://github.com/login/oauth/authorize", TokenURL: "https://github.com/login/oauth/access_token", AuthStyle: oauth2.AuthStyleInParams}
			p.oauth.Scopes = []string{"read:user", "user:email"}
		}
		result[pair.kind] = p
	}
	return result
}

func (p *provider) AuthorizationURL(_ context.Context, state string, mode auth.OAuthMode) (auth.OAuthAuthorization, error) {
	verifier := oauth2.GenerateVerifier()
	options := []oauth2.AuthCodeOption{oauth2.S256ChallengeOption(verifier)}
	nonce := ""
	if p.kind == auth.Google {
		nonce = oauth2.GenerateVerifier()
		options = append(options, oidc.Nonce(nonce))
	}
	if mode == auth.OAuthLink || mode == auth.OAuthReauth {
		options = append(options, oauth2.SetAuthURLParam("prompt", "select_account"))
	}
	return auth.OAuthAuthorization{URL: p.oauth.AuthCodeURL(state, options...), Verifier: verifier, Nonce: nonce}, nil
}

func (p *provider) Exchange(ctx context.Context, code string, flow auth.OAuthFlow) (auth.ProviderIdentity, error) {
	if flow.Provider != p.kind || len(flow.Verifier) < 43 {
		return auth.ProviderIdentity{}, auth.ErrProviderInvalid
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, p.client)
	token, err := p.oauth.Exchange(ctx, code, oauth2.VerifierOption(flow.Verifier))
	if err != nil {
		var rejected *oauth2.RetrieveError
		if !errors.As(err, &rejected) || rejected.Response == nil || rejected.Response.StatusCode >= 500 {
			return auth.ProviderIdentity{}, auth.ErrProviderUnavailable
		}
		return auth.ProviderIdentity{}, auth.ErrProviderInvalid
	}
	// Do not use a refresh-capable TokenSource or save any returned token.
	if p.kind == auth.Google {
		raw, _ := token.Extra("id_token").(string)
		return p.googleIdentity(ctx, raw, flow.Nonce)
	}
	if !strings.EqualFold(token.TokenType, "bearer") || token.AccessToken == "" {
		return auth.ProviderIdentity{}, auth.ErrProviderInvalid
	}
	return p.githubIdentity(ctx, token.AccessToken)
}

func (p *provider) googleVerifier(ctx context.Context) (*oidc.IDTokenVerifier, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.verifier != nil {
		return p.verifier, nil
	}
	discovery, err := oidc.NewProvider(ctx, p.issuer)
	if err != nil {
		return nil, auth.ErrProviderUnavailable
	}
	p.verifier = discovery.VerifierContext(ctx, &oidc.Config{ClientID: p.oauth.ClientID, SupportedSigningAlgs: []string{oidc.RS256}})
	return p.verifier, nil
}

func (p *provider) googleIdentity(ctx context.Context, raw, nonce string) (auth.ProviderIdentity, error) {
	if raw == "" || nonce == "" {
		return auth.ProviderIdentity{}, auth.ErrProviderInvalid
	}
	verifier, err := p.googleVerifier(ctx)
	if err != nil {
		return auth.ProviderIdentity{}, err
	}
	token, err := verifier.Verify(ctx, raw)
	if err != nil || token.Subject == "" || subtle.ConstantTimeCompare([]byte(token.Nonce), []byte(nonce)) != 1 {
		return auth.ProviderIdentity{}, auth.ErrProviderInvalid
	}
	var claims struct {
		Email           string `json:"email"`
		Verified        bool   `json:"email_verified"`
		HostedDomain    string `json:"hd"`
		Name            string `json:"name"`
		AuthorizedParty string `json:"azp"`
	}
	if err = token.Claims(&claims); err != nil {
		return auth.ProviderIdentity{}, auth.ErrProviderInvalid
	}
	// Multiple audiences additionally require an authorized-party match.
	if (claims.AuthorizedParty != "" && claims.AuthorizedParty != p.oauth.ClientID) || (len(token.Audience) > 1 && claims.AuthorizedParty != p.oauth.ClientID) {
		return auth.ProviderIdentity{}, auth.ErrProviderInvalid
	}
	email, err := auth.NormalizeEmail(claims.Email)
	if err != nil {
		return auth.ProviderIdentity{}, auth.ErrProviderInvalid
	}
	authoritative := claims.Verified && (strings.HasSuffix(email, "@gmail.com") || claims.HostedDomain != "")
	return auth.ProviderIdentity{Provider: auth.Google, Subject: token.Subject, Email: email, EmailVerified: claims.Verified, EmailAuthoritative: authoritative, DisplayName: claims.Name}, nil
}

func (p *provider) getJSON(ctx context.Context, path, token string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.api+path, nil)
	if err != nil {
		return auth.ErrProviderInvalid
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	response, err := p.client.Do(req)
	if err != nil {
		return auth.ErrProviderUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return auth.ErrProviderInvalid
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20+1))
	if err != nil || len(body) > 1<<20 || json.Unmarshal(body, target) != nil {
		return auth.ErrProviderInvalid
	}
	return nil
}
