package admin

import (
	"context"
	"time"

	"github.com/deepfurry/gofurry-platform/server/internal/auth"
	"github.com/deepfurry/gofurry-platform/server/internal/transport/admin/generated"
	"github.com/deepfurry/gofurry-platform/server/internal/transport/health"
	"github.com/gofiber/fiber/v3"
)

type Handler struct {
	health  *health.Checker
	options Options
}
type Options struct {
	Auth                                 *auth.App
	Environment, AdminOrigin, CSRFSecret string
}

var _ generated.ServerInterface = (*Handler)(nil)

func Register(router fiber.Router, checker *health.Checker, options ...Options) {
	var option Options
	if len(options) == 1 {
		option = options[0]
	}
	h := &Handler{health: checker, options: option}
	for _, path := range []string{"/auth", "/me"} {
		router.Use(path, func(c fiber.Ctx) error {
			ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
			defer cancel()
			c.SetContext(ctx)
			c.Set("Cache-Control", "no-store")
			return c.Next()
		}, h.originGuard)
	}
	for _, path := range []string{"/auth/logout", "/auth/reauthenticate", "/me"} {
		router.Use(path, h.csrfGuard)
	}
	generated.RegisterHandlers(router, h)
}
func (*Handler) GetLive(c fiber.Ctx) error { return c.JSON(generated.Live{Status: "alive"}) }
func (h *Handler) GetReady(c fiber.Ctx) error {
	state := h.health.Ready(c.Context())
	code := fiber.StatusOK
	if state.Status == "unavailable" {
		code = fiber.StatusServiceUnavailable
	}
	return c.Status(code).JSON(generated.Ready{Status: generated.ReadyStatus(state.Status), Postgres: generated.ReadyPostgres(state.Postgres), Redis: generated.ReadyRedis(state.Redis)})
}
