// oauth-smoke checks prepared configuration and ephemeral Redis capabilities;
// it never exchanges a real authorization code or prints authorization URLs.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/config"
	"github.com/deepfurry/tap4furry/server/internal/oauthprovider"
	"github.com/deepfurry/tap4furry/server/internal/redisstore"
	"golang.org/x/oauth2"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	if os.Getenv("CI") != "" {
		return errors.New("real OAuth smoke refuses CI")
	}
	c, err := config.Load("api")
	if err != nil {
		return err
	}
	if c.Environment != "development" || c.PublicOrigin != "http://localhost:4321" {
		return errors.New("OAuth smoke requires the frozen development origin")
	}
	if !c.GoogleOAuth.Enabled() || !c.GitHubOAuth.Enabled() {
		return errors.New("STOP: both real development OAuth credential pairs must be present (values withheld)")
	}
	store, err := redisstore.Open(c.RedisURL, c.RedisKeyPrefix)
	if err != nil {
		return err
	}
	defer store.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for kind, provider := range oauthprovider.New(c) {
		state := oauth2.GenerateVerifier()
		authorization, err := provider.AuthorizationURL(ctx, state, auth.OAuthLogin)
		if err != nil {
			return errors.New("provider authorization preparation failed")
		}
		parsed, err := url.Parse(authorization.URL)
		if err != nil {
			return errors.New("provider authorization URL invalid")
		}
		callback, _ := c.OAuthCallback(string(kind))
		query := parsed.Query()
		if query.Get("redirect_uri") != callback || query.Get("code_challenge_method") != "S256" || query.Get("state") != state || query.Get("client_secret") != "" || (kind == auth.Google && query.Get("nonce") != authorization.Nonce) {
			return errors.New("provider authorization contract failed (values withheld)")
		}
		flow := auth.OAuthFlow{Provider: kind, Mode: auth.OAuthLogin, Verifier: authorization.Verifier, Nonce: authorization.Nonce, CreatedAt: time.Now().UTC()}
		if err = store.PutOAuthFlow(ctx, state, flow); err != nil {
			return errors.New("STOP: development Redis OAuth SET NX EX capability unavailable (values withheld)")
		}
		// A failed consume leaves only this random flow, which expires in ten minutes.
		if _, err = store.ConsumeOAuthFlow(ctx, state); err != nil {
			return errors.New("STOP: development Redis OAuth GETDEL capability unavailable (values withheld)")
		}
		if _, err = store.ConsumeOAuthFlow(ctx, state); !errors.Is(err, auth.ErrProviderInvalid) {
			return errors.New("OAuth state replay was not rejected")
		}
		fmt.Printf("%s: credential pair present; fixed callback and S256 authorization URL valid; Redis one-use flow PASS (values withheld)\n", kind)
	}
	return nil
}
