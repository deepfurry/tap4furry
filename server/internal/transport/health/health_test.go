package health_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/deepfurry/gofurry-platform/server/internal/transport/admin"
	"github.com/deepfurry/gofurry-platform/server/internal/transport/health"
	"github.com/deepfurry/gofurry-platform/server/internal/transport/public"
	"github.com/gofiber/fiber/v3"
)

func TestPublicAndAdminHealthContract(t *testing.T) {
	registerPublic := func(router fiber.Router, checker *health.Checker) {
		public.Register(router, checker, nil, nil, public.Options{Environment: "test", PublicOrigin: "http://localhost:4321"})
	}
	registerAdmin := func(router fiber.Router, checker *health.Checker) { admin.Register(router, checker) }
	for name, register := range map[string]func(fiber.Router, *health.Checker){"public": registerPublic, "admin": registerAdmin} {
		t.Run(name, func(t *testing.T) {
			for _, scenario := range []struct {
				name            string
				pg, redis, stop bool
				code            int
				status          string
			}{
				{"ready", true, true, false, 200, "ready"}, {"redis-degraded", true, false, false, 200, "degraded"},
				{"postgres-down", false, true, false, 503, "unavailable"}, {"both-down", false, false, false, 503, "unavailable"},
				{"stopping", true, true, true, 503, "unavailable"},
			} {
				t.Run(scenario.name, func(t *testing.T) {
					calls := 0
					check := func(up bool) health.Check {
						return func(ctx context.Context) error {
							calls++
							if _, ok := ctx.Deadline(); !ok {
								t.Error("dependency check lacks timeout")
							}
							if !up {
								return errors.New("private-driver-message")
							}
							return nil
						}
					}
					checker := health.New(check(scenario.pg), check(scenario.redis))
					if scenario.stop {
						checker.Stop()
					}
					app := fiber.New()
					register(app, checker)
					live, err := app.Test(httptest.NewRequest("GET", "/health/live", nil))
					if err != nil {
						t.Fatal(err)
					}
					defer live.Body.Close()
					if live.StatusCode != 200 || calls != 0 {
						t.Fatal("liveness fanned out or failed")
					}
					ready, err := app.Test(httptest.NewRequest("GET", "/health/ready", nil))
					if err != nil {
						t.Fatal(err)
					}
					defer ready.Body.Close()
					var body map[string]string
					if err := json.NewDecoder(ready.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					if ready.StatusCode != scenario.code || body["status"] != scenario.status || len(body) != 3 {
						t.Fatalf("unexpected health response: %v / %d", body, ready.StatusCode)
					}
					if scenario.stop && calls != 0 {
						t.Fatal("shutdown readiness probed dependencies")
					}
				})
			}
		})
	}
}
