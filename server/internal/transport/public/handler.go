package public

import (
	"context"
	"strings"
	"time"

	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/contribution"
	"github.com/deepfurry/tap4furry/server/internal/identity"
	"github.com/deepfurry/tap4furry/server/internal/transport/health"
	"github.com/deepfurry/tap4furry/server/internal/transport/public/generated"
	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	health        *health.Checker
	auth          *auth.App
	identity      *identity.App
	options       Options
	resources     *pgxpool.Pool
	contributions *contribution.App
}
type Options struct {
	Environment, PublicOrigin, CSRFSecret string
	ResourcePool                          *pgxpool.Pool
}

var _ generated.ServerInterface = (*Handler)(nil)

func Register(router fiber.Router, checker *health.Checker, authentication *auth.App, identities *identity.App, options Options) {
	h := &Handler{health: checker, auth: authentication, identity: identities, options: options, resources: options.ResourcePool}
	if options.ResourcePool != nil {
		h.contributions = contribution.New(options.ResourcePool, options.CSRFSecret)
	}
	for _, path := range []string{"/auth", "/me", "/users", "/resources", "/categories", "/tags", "/contributions"} {
		router.Use(path, func(c fiber.Ctx) error {
			deadline := 5 * time.Second
			if strings.HasPrefix(c.Path(), "/auth/oauth/") {
				deadline = 15 * time.Second
			}
			ctx, cancel := context.WithTimeout(c.Context(), deadline)
			defer cancel()
			c.SetContext(ctx)
			return c.Next()
		})
	}
	for _, path := range []string{"/resources", "/categories", "/tags"} {
		router.Use(path, h.publicReadBoundary)
	}
	for _, path := range []string{"/auth", "/me"} {
		router.Use(path, h.originGuard)
	}
	router.Use("/auth/oauth", h.oauthBoundary)
	for _, path := range []string{"/auth/logout", "/me/profile", "/auth/email/verification/request", "/auth/password/change", "/auth/reauthenticate", "/me/sessions", "/me/auth-methods"} {
		router.Use(path, h.csrfGuard)
	}
	router.Use("/contributions", h.contributionBoundary, h.originGuard, h.csrfGuard)
	router.Use("/me/contributions", h.contributionBoundary, h.csrfGuard)
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
