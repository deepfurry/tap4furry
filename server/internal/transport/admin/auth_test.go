package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deepfurry/gofurry-platform/server/internal/config"
	"github.com/gofiber/fiber/v3"
)

func TestAdminCookieAndCSRFDomains(t *testing.T) {
	for _, environment := range []string{"development", "test", "production", ""} {
		h := &Handler{options: Options{Environment: environment}}
		cookie := h.cookie("fixture", time.Now().Add(time.Hour), false)
		secure := environment != "development" && environment != "test"
		if cookie.Secure != secure || cookie.SameSite != "Strict" || !cookie.HTTPOnly || cookie.Path != "/" || cookie.Domain != "" {
			t.Fatal("Admin cookie flags failed")
		}
		name := "gofurry_admin_session"
		if secure {
			name = "__Host-" + name
		}
		if cookie.Name != name {
			t.Fatal("Admin cookie namespace failed")
		}
		cleared := h.cookie("", time.Time{}, true)
		if cleared.MaxAge != -1 || !cleared.Expires.Before(time.Now()) || cleared.Name != name || cleared.Secure != secure {
			t.Fatal("cookie clearing failed")
		}
	}
	first := csrfToken(config.DevelopmentAdminCSRFSecret, "session-one")
	if first == csrfToken(config.DevelopmentCSRFSecret, "session-one") || first == csrfToken(config.DevelopmentAdminCSRFSecret, "session-two") {
		t.Fatal("CSRF crossed secret/session boundary")
	}
}
func TestAdminOriginBeforeBody(t *testing.T) {
	app := fiber.New()
	Register(app, nil, Options{Environment: "test", AdminOrigin: "http://localhost:5173"})
	for _, origin := range []string{"", "null", "http://localhost:4321", "http://localhost:5173.evil.invalid"} {
		req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(`{"email":"fixture@example.invalid","password":"do not expose this"}`))
		req.Header.Set("Origin", origin)
		req.Header.Set("Content-Type", "application/json")
		response, err := app.Test(req)
		if err != nil {
			t.Fatal("Origin request failed")
		}
		response.Body.Close()
		if response.StatusCode != http.StatusForbidden {
			t.Fatal("Admin accepted a foreign origin")
		}
	}
}
