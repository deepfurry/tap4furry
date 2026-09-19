// Package config reads process environment without loading developer files.
package config

import (
	"errors"
	"net"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Environment              string
	HTTPAddr                 string
	DatabaseURL              string
	RedisURL                 string
	RedisKeyPrefix           string
	RiverSchema              string
	PublicOrigin             string
	CSRFSecret               string
	AdminOrigin              string
	AdminCSRFSecret          string
	AuthThrottleSecret       string
	MailMode                 string
	MailLocalDir             string
	ResendAPIKey             string
	MailFrom                 string
	MailReplyTo              string
	GoogleOAuth, GitHubOAuth OAuthCredentials
}

type OAuthCredentials struct{ ClientID, ClientSecret string }

func (c OAuthCredentials) Enabled() bool { return c.ClientID != "" && c.ClientSecret != "" }

func (c Config) OAuthCallback(provider string) (string, error) {
	if provider != "google" && provider != "github" {
		return "", errors.New("unsupported OAuth provider")
	}
	return c.PublicOrigin + "/api/auth/oauth/" + provider + "/callback", nil
}

const DevelopmentCSRFSecret = "tap4furry-development-only-csrf-secret"
const DevelopmentAdminCSRFSecret = "tap4furry-development-only-admin-csrf-secret"
const DevelopmentAuthThrottleSecret = "tap4furry-development-only-auth-throttle-secret"

func Load(service string) (Config, error) {
	return load(service, os.Getenv)
}

func load(service string, env func(string) string) (Config, error) {
	c := Config{Environment: env("APP_ENV"), DatabaseURL: env("DATABASE_URL")}
	if c.Environment != "development" && c.Environment != "test" && c.Environment != "production" {
		return Config{}, errors.New("APP_ENV must be development, test or production")
	}
	if !validURL(c.DatabaseURL, "postgres", "postgresql") {
		return Config{}, errors.New("DATABASE_URL is required and must be a PostgreSQL URL")
	}
	switch service {
	case "api", "admin", "worker", "migrator":
	default:
		return Config{}, errors.New("unknown service")
	}
	if service == "api" || service == "admin" {
		c.AuthThrottleSecret = env("AUTH_THROTTLE_SECRET")
		if c.AuthThrottleSecret == "" && c.Environment != "production" {
			c.AuthThrottleSecret = DevelopmentAuthThrottleSecret
		}
		if len(c.AuthThrottleSecret) < 32 || (c.Environment == "production" && c.AuthThrottleSecret == DevelopmentAuthThrottleSecret) {
			return Config{}, errors.New("AUTH_THROTTLE_SECRET must contain at least 32 bytes; production requires a private secret")
		}
		c.HTTPAddr = env("HTTP_ADDR")
		_, port, err := net.SplitHostPort(c.HTTPAddr)
		n, parseErr := strconv.Atoi(port)
		if err != nil || parseErr != nil || n < 1 || n > 65535 {
			return Config{}, errors.New("HTTP_ADDR must be host:port with a valid port")
		}
	}
	if service == "admin" {
		c.AdminOrigin = env("ADMIN_ORIGIN")
		if c.AdminOrigin == "" && c.Environment == "development" {
			c.AdminOrigin = "http://localhost:5173"
		}
		u, err := url.Parse(c.AdminOrigin)
		if err != nil || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || (u.Scheme != "http" && u.Scheme != "https") || (c.Environment == "production" && u.Scheme != "https") {
			return Config{}, errors.New("ADMIN_ORIGIN must be an exact origin (HTTPS required in production)")
		}
		c.AdminCSRFSecret = env("ADMIN_CSRF_SECRET")
		if c.AdminCSRFSecret == "" && c.Environment != "production" {
			c.AdminCSRFSecret = DevelopmentAdminCSRFSecret
		}
		if len(c.AdminCSRFSecret) < 32 || c.AdminCSRFSecret == env("CSRF_SECRET") || c.AdminCSRFSecret == DevelopmentCSRFSecret || (c.Environment == "production" && c.AdminCSRFSecret == DevelopmentAdminCSRFSecret) {
			return Config{}, errors.New("ADMIN_CSRF_SECRET requires a separate private secret of at least 32 bytes in production")
		}
	}
	if service == "api" {
		c.GoogleOAuth = OAuthCredentials{env("GOOGLE_OAUTH_CLIENT_ID"), env("GOOGLE_OAUTH_CLIENT_SECRET")}
		c.GitHubOAuth = OAuthCredentials{env("GITHUB_OAUTH_CLIENT_ID"), env("GITHUB_OAUTH_CLIENT_SECRET")}
		for _, pair := range []OAuthCredentials{c.GoogleOAuth, c.GitHubOAuth} {
			if (pair.ClientID == "") != (pair.ClientSecret == "") {
				return Config{}, errors.New("OAuth credential pairs must be both present or both absent (values withheld)")
			}
		}
		c.PublicOrigin = env("PUBLIC_ORIGIN")
		if c.PublicOrigin == "" && c.Environment == "development" {
			c.PublicOrigin = "http://localhost:4321"
		}
		u, err := url.Parse(c.PublicOrigin)
		if err != nil || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" ||
			(u.Scheme != "http" && u.Scheme != "https") || (c.Environment == "production" && u.Scheme != "https") {
			return Config{}, errors.New("PUBLIC_ORIGIN must be an exact origin (HTTPS required in production)")
		}
		c.CSRFSecret = env("CSRF_SECRET")
		if c.CSRFSecret == "" && c.Environment != "production" {
			c.CSRFSecret = DevelopmentCSRFSecret
		}
		if len(c.CSRFSecret) < 32 || (c.Environment == "production" && c.CSRFSecret == DevelopmentCSRFSecret) {
			return Config{}, errors.New("CSRF_SECRET must contain at least 32 bytes; production requires a private secret")
		}
		c.MailMode = env("MAIL_MODE")
		if c.MailMode == "" && c.Environment != "production" {
			c.MailMode = "local"
		}
		if c.Environment == "production" && c.MailMode != "resend" {
			return Config{}, errors.New("production requires explicit MAIL_MODE=resend")
		}
		if c.MailMode != "local" && c.MailMode != "disabled" && c.MailMode != "resend" {
			return Config{}, errors.New("MAIL_MODE must be local, disabled or resend")
		}
		if c.MailMode == "resend" {
			c.ResendAPIKey = env("RESEND_API_KEY")
			c.MailFrom, c.MailReplyTo = env("MAIL_FROM"), env("MAIL_REPLY_TO")
			if strings.TrimSpace(c.ResendAPIKey) == "" {
				return Config{}, errors.New("RESEND_API_KEY is required in resend mode")
			}
			for _, field := range []struct{ key, value string }{{"MAIL_FROM", c.MailFrom}, {"MAIL_REPLY_TO", c.MailReplyTo}} {
				if _, err := mail.ParseAddress(field.value); err != nil || strings.ContainsAny(field.value, "\r\n") {
					return Config{}, errors.New(field.key + " must be a valid mailbox address")
				}
			}
		}
		if c.MailMode == "local" {
			c.MailLocalDir = env("MAIL_LOCAL_DIR")
			if c.MailLocalDir == "" {
				c.MailLocalDir = filepath.Join("..", ".local", "mail")
			}
			root, _ := filepath.Abs(filepath.Join("..", ".local"))
			dir, err := filepath.Abs(c.MailLocalDir)
			rel, relErr := filepath.Rel(root, dir)
			if err != nil || relErr != nil || rel == "." || !filepath.IsLocal(rel) {
				return Config{}, errors.New("MAIL_LOCAL_DIR must be inside the repository private .local directory (launch from server)")
			}
			c.MailLocalDir = dir
		}
	}
	if service != "migrator" {
		c.RedisURL, c.RedisKeyPrefix = env("REDIS_URL"), env("REDIS_KEY_PREFIX")
		if !validURL(c.RedisURL, "redis", "rediss") {
			return Config{}, errors.New("REDIS_URL is required and must be a Redis URL")
		}
		if c.RedisKeyPrefix != "gfp:" {
			return Config{}, errors.New("REDIS_KEY_PREFIX must be gfp:")
		}
	}
	if service == "worker" || service == "migrator" {
		c.RiverSchema = env("RIVER_SCHEMA")
		if c.RiverSchema != "river" {
			return Config{}, errors.New("RIVER_SCHEMA must be river")
		}
	}
	return c, nil
}

func validURL(raw string, schemes ...string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.Fragment != "" {
		return false
	}
	for _, scheme := range schemes {
		if u.Scheme == scheme {
			return true
		}
	}
	return false
}
