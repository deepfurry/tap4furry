package admin

import (
	"context"
	"time"

	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/contribution"
	"github.com/deepfurry/tap4furry/server/internal/curation"
	"github.com/deepfurry/tap4furry/server/internal/transport/admin/generated"
	"github.com/deepfurry/tap4furry/server/internal/transport/health"
	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	health        *health.Checker
	options       Options
	contributions *contribution.App
}
type Options struct {
	Auth                                 *auth.App
	Curation                             *curation.App
	ResourcePool                         *pgxpool.Pool
	Environment, AdminOrigin, CSRFSecret string
}

var _ generated.ServerInterface = (*Handler)(nil)

func Register(router fiber.Router, checker *health.Checker, options ...Options) {
	var option Options
	if len(options) == 1 {
		option = options[0]
	}
	h := &Handler{health: checker, options: option}
	if option.ResourcePool != nil {
		h.contributions = contribution.New(option.ResourcePool, "")
	}
	for _, path := range []string{"/auth", "/me", "/resources", "/categories", "/tags", "/contributions"} {
		router.Use(path, func(c fiber.Ctx) error {
			ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
			defer cancel()
			c.SetContext(ctx)
			c.Set("Cache-Control", "no-store")
			return c.Next()
		}, h.originGuard)
	}
	for _, path := range []string{"/resources", "/categories", "/tags"} {
		router.Use(path, h.curationBoundary, h.adminAccessGuard, h.csrfGuard)
	}
	for _, path := range []string{"/auth/logout", "/auth/reauthenticate", "/me"} {
		router.Use(path, h.csrfGuard)
	}
	router.Use("/contributions", h.contributionBoundary, h.adminAccessGuard, h.csrfGuard)
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
