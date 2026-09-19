package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/config"
	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/deepfurry/tap4furry/server/internal/identity"
	"github.com/deepfurry/tap4furry/server/internal/mail"
	"github.com/deepfurry/tap4furry/server/internal/oauthprovider"
	"github.com/deepfurry/tap4furry/server/internal/redisstore"
	platformruntime "github.com/deepfurry/tap4furry/server/internal/runtime"
	"github.com/deepfurry/tap4furry/server/internal/transport/health"
	"github.com/deepfurry/tap4furry/server/internal/transport/public"
	"github.com/gofiber/fiber/v3"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	c, err := config.Load("api")
	if err != nil {
		return err
	}
	logger := platformruntime.Logger("api", c.Environment)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	defer platformruntime.ShutdownDeadline(ctx, logger)()
	pool, err := database.Open(ctx, c.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	store, err := redisstore.Open(c.RedisURL, c.RedisKeyPrefix)
	if err != nil {
		return err
	}
	defer store.Close()
	checker := health.New(func(ctx context.Context) error { return database.Ready(ctx, pool) }, store.Ping)
	var mailer auth.ChallengeMailer
	switch c.MailMode {
	case "local":
		capture, err := mail.NewLocal(filepath.Join("..", ".local"), c.MailLocalDir, c.PublicOrigin)
		if err != nil {
			return err
		}
		defer capture.Close()
		mailer = capture
	case "resend":
		mailer, err = mail.NewResend(c.ResendAPIKey, c.MailFrom, c.MailReplyTo, c.PublicOrigin)
		if err != nil {
			return err
		}
	case "disabled":
		mailer = mail.Disabled{}
	}
	throttle, err := store.AuthThrottle(c.AuthThrottleSecret)
	if err != nil {
		return err
	}
	authentication, err := auth.NewWithThrottle(pool, mailer, throttle, auth.OAuthConfig{Flows: store, Providers: oauthprovider.New(c)})
	if err != nil {
		return err
	}
	app := fiber.New(fiber.Config{ReadTimeout: 5 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 30 * time.Second, BodyLimit: 8192})
	public.Register(app, checker, authentication, identity.New(pool), public.Options{Environment: c.Environment, PublicOrigin: c.PublicOrigin, CSRFSecret: c.CSRFSecret, ResourcePool: pool})
	return platformruntime.HTTP(ctx, app, c.HTTPAddr, checker, logger)
}
