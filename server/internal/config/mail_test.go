package config

import (
	"strings"
	"testing"
)

func TestMailEnvironmentContract(t *testing.T) {
	for _, environment := range []string{"development", "test", "production"} {
		for _, mode := range []string{"", "local", "disabled", "resend", "invalid-private-value"} {
			t.Run(environment+"/"+mode, func(t *testing.T) {
				env := map[string]string{
					"APP_ENV": environment, "MAIL_MODE": mode,
					"DATABASE_URL": "postgres://localhost/gfp_ci", "REDIS_URL": "redis://localhost:6379", "REDIS_KEY_PREFIX": "gfp:",
					"HTTP_ADDR": "127.0.0.1:8080", "PUBLIC_ORIGIN": "https://example.invalid",
					"CSRF_SECRET": strings.Repeat("public-fixture", 4), "AUTH_THROTTLE_SECRET": strings.Repeat("throttle-fixture", 4),
				}
				if mode == "resend" {
					env["RESEND_API_KEY"], env["MAIL_FROM"], env["MAIL_REPLY_TO"] = "private-key-fixture", "Tap4Furry <sender@example.invalid>", "reply@example.invalid"
					// Resend must not validate or open the local capture directory.
					env["MAIL_LOCAL_DIR"] = "../public-capture"
				}
				cfg, err := load("api", func(k string) string { return env[k] })
				valid := mode != "invalid-private-value" && (environment != "production" || mode == "resend")
				if (err == nil) != valid {
					t.Fatal("mail environment contract differs")
				}
				if err != nil && strings.Contains(err.Error(), "private-value") {
					t.Fatal("mail mode error leaked input")
				}
				if valid && mode == "" && cfg.MailMode != "local" {
					t.Fatal("development/test no longer default to local capture")
				}
				if valid && mode == "resend" && (cfg.ResendAPIKey != env["RESEND_API_KEY"] || cfg.MailFrom != env["MAIL_FROM"] || cfg.MailReplyTo != env["MAIL_REPLY_TO"] || cfg.MailLocalDir != "") {
					t.Fatal("resend config lost values or requires local capture")
				}
				if mode != "resend" {
					return
				}
				for _, field := range []string{"RESEND_API_KEY", "MAIL_FROM", "MAIL_REPLY_TO"} {
					badValues := []string{"", "   "}
					if field != "RESEND_API_KEY" {
						badValues = append(badValues, "private-value", "first@example.invalid, private-value@example.invalid", "private-value@example.invalid\r\nBcc: other@example.invalid", "private-value@example.invalid>")
					}
					for _, bad := range badValues {
						_, err := load("api", func(k string) string {
							if k == field {
								return bad
							}
							return env[k]
						})
						if err == nil || strings.Contains(err.Error(), "private-value") || strings.Contains(err.Error(), "private-key-fixture") {
							t.Fatal("invalid resend config accepted or disclosed")
						}
					}
				}
			})
		}
	}
}

func TestNonAPIServicesDoNotReadMailConfiguration(t *testing.T) {
	env := map[string]string{
		"APP_ENV": "production", "HTTP_ADDR": "127.0.0.1:8081", "ADMIN_ORIGIN": "https://admin.example.invalid",
		"ADMIN_CSRF_SECRET": strings.Repeat("admin-fixture", 4), "AUTH_THROTTLE_SECRET": strings.Repeat("throttle-fixture", 4),
		"DATABASE_URL": "postgres://localhost/gfp_ci", "REDIS_URL": "redis://localhost:6379", "REDIS_KEY_PREFIX": "gfp:", "RIVER_SCHEMA": "river",
	}
	for _, service := range []string{"admin", "worker", "migrator"} {
		if _, err := load(service, func(k string) string {
			if strings.HasPrefix(k, "MAIL_") || strings.HasPrefix(k, "RESEND_") {
				t.Fatal("non-Public process read mail configuration")
			}
			return env[k]
		}); err != nil {
			t.Fatal("non-Public process requires mail configuration")
		}
	}
}
