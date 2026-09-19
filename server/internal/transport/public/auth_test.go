package public

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/identity"
	"github.com/gofiber/fiber/v3"
)

func TestCookieEnvironments(t *testing.T) {
	for _, environment := range []string{"production", "development", "test", "unknown"} {
		for _, clear := range []bool{false, true} {
			h := Handler{options: Options{Environment: environment}}
			cookie := h.cookie("temporary-test-value", time.Now().Add(auth.AbsoluteLifetime), clear)
			secure := environment != "development" && environment != "test"
			name := "tap4furry_session"
			if secure {
				name = "__Host-" + name
			}
			if cookie.Name != name || cookie.Secure != secure || !cookie.HTTPOnly || cookie.Path != "/" || cookie.Domain != "" || cookie.SameSite != "Lax" {
				t.Fatal("cookie security contract failed")
			}
			if clear && (cookie.Value != "" || cookie.MaxAge != -1 || !cookie.Expires.Before(time.Now())) {
				t.Fatal("logout cookie was not expired")
			}
			for _, provider := range []auth.Provider{auth.Google, auth.GitHub} {
				flow := h.flowCookie(provider, "temporary-test-value", clear)
				flowName := "tap4furry_oauth_" + string(provider)
				if secure {
					flowName = "__Host-" + flowName
				}
				if flow.Name != flowName || flow.Secure != secure || !flow.HTTPOnly || flow.Path != "/" || flow.Domain != "" || flow.SameSite != "Lax" {
					t.Fatal("OAuth binding cookie namespace or security contract failed")
				}
				if clear && (flow.Value != "" || flow.MaxAge != -1 || !flow.Expires.Before(time.Now())) || !clear && flow.MaxAge != 600 {
					t.Fatal("OAuth binding cookie lifetime or clearing failed")
				}
			}
			app := fiber.New()
			app.Get("/", func(c fiber.Ctx) error { c.Cookie(cookie); return c.SendStatus(204) })
			response, err := app.Test(httptest.NewRequest("GET", "/", nil))
			if err != nil {
				t.Fatal("cookie response failed")
			}
			response.Body.Close()
			cookies := response.Cookies()
			if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].Secure != secure || cookies[0].Domain != "" {
				t.Fatal("wire cookie security contract failed")
			}
		}
	}
}

func TestOriginGuard(t *testing.T) {
	h := Handler{options: Options{PublicOrigin: "https://example.com"}}
	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		for _, origin := range []string{"", "null", "https://example.com.evil.invalid", "http://example.com", "https://example.com/", "https://example.com"} {
			app := fiber.New()
			app.Use(h.originGuard)
			app.All("/", func(c fiber.Ctx) error { return c.SendStatus(204) })
			request := httptest.NewRequest(method, "/", nil)
			request.Header.Set("Origin", origin)
			response, err := app.Test(request)
			if err != nil {
				t.Fatal("origin test request failed")
			}
			response.Body.Close()
			want := 403
			if origin == "https://example.com" {
				want = 204
			}
			if response.StatusCode != want || response.Header.Get("Access-Control-Allow-Origin") != "" {
				t.Fatal("exact origin guard or CORS contract failed")
			}
		}
	}
}

func TestStableErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{identity.ErrValidation, 400, "VALIDATION_ERROR"}, {auth.ErrEmailRegistered, 409, "AUTH_EMAIL_ALREADY_REGISTERED"},
		{auth.ErrInvalidCredentials, 401, "AUTH_INVALID_CREDENTIALS"}, {auth.ErrUnauthenticated, 401, "AUTH_UNAUTHENTICATED"},
		{auth.ErrAccountDisabled, 403, "AUTH_ACCOUNT_DISABLED"}, {identity.ErrHandleUnavailable, 409, "PROFILE_HANDLE_UNAVAILABLE"},
		{identity.ErrNotFound, 404, "PROFILE_NOT_FOUND"}, {errOrigin, 403, "ORIGIN_FORBIDDEN"},
		{errors.New("private-driver-credential-detail"), 500, "INTERNAL_ERROR"},
	}
	for _, test := range cases {
		app := fiber.New()
		app.Get("/", func(c fiber.Ctx) error { return respondError(c, test.err) })
		response, err := app.Test(httptest.NewRequest("GET", "/", nil))
		if err != nil {
			t.Fatal("error mapping request failed")
		}
		var result map[string]string
		err = json.NewDecoder(response.Body).Decode(&result)
		response.Body.Close()
		if err != nil || response.StatusCode != test.status || result["code"] != test.code || len(result) != 2 || strings.Contains(result["message"], "private-driver") {
			t.Fatal("unsafe or unstable error response")
		}
	}
}
