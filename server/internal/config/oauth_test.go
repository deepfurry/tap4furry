package config

import (
	"strings"
	"testing"
)

func TestOAuthCredentialPairsAndCallbacks(t *testing.T) {
	base := map[string]string{"APP_ENV": "development", "DATABASE_URL": "postgres://localhost/gfp_ci", "REDIS_URL": "redis://localhost:6379", "REDIS_KEY_PREFIX": "gfp:", "HTTP_ADDR": "127.0.0.1:8080"}
	for _, prefix := range []string{"GOOGLE", "GITHUB"} {
		for _, suffix := range []string{"_OAUTH_CLIENT_ID", "_OAUTH_CLIENT_SECRET"} {
			base[prefix+suffix] = "private-fixture"
			if _, err := load("api", func(k string) string { return base[k] }); err == nil || strings.Contains(err.Error(), "private-fixture") {
				t.Fatal("partial credential pair accepted or disclosed")
			}
			delete(base, prefix+suffix)
		}
	}
	for _, origin := range []string{"http://localhost:4321", "https://tap4furry.com"} {
		base["PUBLIC_ORIGIN"] = origin
		c, err := load("api", func(k string) string { return base[k] })
		if err != nil || c.GoogleOAuth.Enabled() || c.GitHubOAuth.Enabled() {
			t.Fatal("disabled providers break local auth")
		}
		for _, provider := range []string{"google", "github"} {
			callback, err := c.OAuthCallback(provider)
			if err != nil || callback != origin+"/api/auth/oauth/"+provider+"/callback" {
				t.Fatal("callback contract differs")
			}
		}
		if _, err = c.OAuthCallback("other"); err == nil {
			t.Fatal("unsupported callback accepted")
		}
	}
}
