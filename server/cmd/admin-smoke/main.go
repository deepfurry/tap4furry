// admin-smoke uses runtime roles for auth and the owner only for fixture roles
// and cleanup. Its generated account and secrets never leave the process.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"time"
	"uuid"

	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/config"
	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/deepfurry/tap4furry/server/internal/identity"
	"github.com/deepfurry/tap4furry/server/internal/mail"
	"github.com/deepfurry/tap4furry/server/internal/redisstore"
	"github.com/deepfurry/tap4furry/server/internal/transport/admin"
	"github.com/deepfurry/tap4furry/server/internal/transport/public"
	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Post-commit capture is in memory for this generated fixture only. The separate
// smoke:auth:dev command checks the real private filesystem delivery adapter.
type smokeCapture struct{ token string }

func (c *smokeCapture) SendEmailVerification(_ context.Context, _ string, token string) error {
	c.token = token
	return nil
}
func (*smokeCapture) SendPasswordReset(context.Context, string, string) error {
	return errors.New("not used by Admin smoke")
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() (result error) {
	if os.Getenv("CI") != "" {
		return errors.New("real Admin smoke refuses CI")
	}
	cfg, err := config.Load("admin")
	if err != nil {
		return err
	}
	if cfg.Environment != "development" {
		return errors.New("Admin smoke requires development")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	open := func(raw, role string) (*pgxpool.Pool, error) {
		pool, err := database.Open(ctx, raw)
		if err != nil {
			return nil, err
		}
		if _, err = database.Inspect(ctx, pool, role, "gfp_dev"); err != nil {
			pool.Close()
			return nil, err
		}
		return pool, nil
	}
	adminPool, err := open(cfg.DatabaseURL, "gfp_admin")
	if err != nil {
		return err
	}
	defer adminPool.Close()
	api, err := open(os.Getenv("ADMIN_SMOKE_API_DATABASE_URL"), "gfp_api")
	if err != nil {
		return err
	}
	defer api.Close()
	owner, err := open(os.Getenv("ADMIN_SMOKE_OWNER_DATABASE_URL"), "gfp_migrator")
	if err != nil {
		return err
	}
	defer owner.Close()
	// Inspect actual table privileges too: shared default grants must not turn
	// static operator roles into runtime-writable state.
	for _, role := range []string{"gfp_admin", "gfp_api", "gfp_worker"} {
		var bad bool
		if err = owner.QueryRow(ctx, "SELECT has_any_column_privilege($1,'app.user_roles','INSERT,UPDATE') OR has_table_privilege($1,'app.user_roles','DELETE,TRUNCATE')", role).Scan(&bad); err != nil {
			return database.SafeError("inspect role isolation", err)
		}
		if bad {
			return errors.New("unexpected runtime role mutation capability; stop for owned-object grant conflict")
		}
	}
	store, err := redisstore.Open(cfg.RedisURL, cfg.RedisKeyPrefix)
	if err != nil {
		return err
	}
	defer store.Close()
	throttle, err := store.AuthThrottle(cfg.AuthThrottleSecret)
	if err != nil {
		return err
	}
	apiStore, err := redisstore.Open(os.Getenv("ADMIN_SMOKE_API_REDIS_URL"), "gfp:")
	if err != nil {
		return err
	}
	defer apiStore.Close()
	apiThrottle, err := apiStore.AuthThrottle(cfg.AuthThrottleSecret)
	if err != nil {
		return err
	}
	capture := &smokeCapture{}
	publicAuth, err := auth.NewWithThrottle(api, capture, apiThrottle)
	if err != nil {
		return err
	}
	adminAuth, err := auth.NewWithThrottle(adminPool, mail.Disabled{}, throttle)
	if err != nil {
		return err
	}
	email := uuid.NewV7().String() + "@example.invalid"
	defer func() {
		if err := cleanup(owner, email); err != nil {
			result = errors.Join(result, err)
		}
	}()
	var random [32]byte
	rand.Read(random[:])
	password := base64.RawURLEncoding.EncodeToString(random[:])
	grant, err := publicAuth.Register(ctx, email, password)
	if err != nil {
		return err
	}
	if err = publicAuth.VerifyEmail(ctx, capture.token); err != nil {
		return err
	}
	capture.token = ""
	operator := auth.NewRoleOperator(owner)
	if _, err = operator.Grant(ctx, email, auth.Moderator); err != nil {
		return err
	}
	defer func() { _ = apiThrottle.ClearSubject(context.Background(), auth.RegistrationLimit, email) }()
	app := fiber.New()
	admin.Register(app, nil, admin.Options{Auth: adminAuth, Environment: cfg.Environment, AdminOrigin: cfg.AdminOrigin, CSRFSecret: cfg.AdminCSRFSecret})
	pub := fiber.New()
	public.Register(pub, nil, publicAuth, identity.New(api), public.Options{Environment: "development", PublicOrigin: "http://localhost:4321", CSRFSecret: config.DevelopmentCSRFSecret})
	var request func(*fiber.App, string, string, any, *http.Cookie, int) (map[string]any, *http.Cookie, error)
	request = func(target *fiber.App, method, path string, body any, cookie *http.Cookie, status int) (map[string]any, *http.Cookie, error) {
		payload, _ := json.Marshal(body)
		req := httptest.NewRequestWithContext(ctx, method, path, bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", cfg.AdminOrigin)
		if cookie != nil {
			req.AddCookie(cookie)
			if method != "GET" {
				csrf, _, err := request(target, "GET", "/auth/csrf", nil, cookie, 200)
				if err != nil {
					return nil, nil, err
				}
				value, ok := csrf["csrf_token"].(string)
				if !ok {
					return nil, nil, errors.New("Admin CSRF response invalid")
				}
				req.Header.Set("X-CSRF-Token", value)
			}
		}
		response, err := target.Test(req, fiber.TestConfig{Timeout: 10 * time.Second})
		if err != nil {
			return nil, nil, errors.New("Admin smoke HTTP request failed (details withheld)")
		}
		defer response.Body.Close()
		if response.StatusCode != status {
			return nil, nil, fmt.Errorf("Admin smoke %s %s expected %d got %d (body withheld)", method, path, status, response.StatusCode)
		}
		var decoded map[string]any
		if status != 204 && json.NewDecoder(response.Body).Decode(&decoded) != nil {
			return nil, nil, errors.New("Admin smoke response invalid")
		}
		var next *http.Cookie
		if cookies := response.Cookies(); len(cookies) > 0 {
			next = cookies[0]
		}
		return decoded, next, nil
	}
	credentials := map[string]string{"email": email, "password": password}
	me, cookie, err := request(app, "POST", "/auth/login", credentials, nil, 200)
	if err != nil {
		return err
	}
	if cookie == nil || cookie.Name != "tap4furry_admin_session" || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || len(me) != 4 {
		return errors.New("Admin smoke cookie/privacy contract failed")
	}
	if _, _, err = request(app, "GET", "/me", nil, cookie, 200); err != nil {
		return err
	}
	if _, _, err = request(app, "GET", "/auth/csrf", nil, cookie, 200); err != nil {
		return err
	}
	_, rotated, err := request(app, "POST", "/auth/reauthenticate", map[string]string{"password": password}, cookie, 204)
	if err != nil {
		return err
	}
	if rotated == nil || rotated.Value == cookie.Value {
		return errors.New("Admin smoke did not rotate session")
	}
	if _, _, err = request(app, "GET", "/me", nil, cookie, 401); err != nil {
		return err
	}
	cookie = rotated
	listed, _, err := request(app, "GET", "/me/sessions", nil, cookie, 200)
	if err != nil {
		return err
	}
	sessions, ok := listed["sessions"].([]any)
	if !ok || len(sessions) != 1 {
		return errors.New("Admin session isolation failed")
	}
	publicCookie := &http.Cookie{Name: "tap4furry_session", Value: grant.Token}
	if _, _, err = request(app, "GET", "/me", nil, publicCookie, 401); err != nil {
		return err
	}
	if _, _, err = request(pub, "GET", "/me", nil, cookie, 401); err != nil {
		return err
	}
	_, other, err := request(app, "POST", "/auth/login", credentials, nil, 200)
	if err != nil {
		return err
	}
	if _, _, err = request(app, "POST", "/me/sessions/revoke-others", nil, cookie, 204); err != nil {
		return err
	}
	if _, _, err = request(app, "GET", "/me", nil, other, 401); err != nil {
		return err
	}
	ownID, ok := sessions[0].(map[string]any)["id"].(string)
	if !ok {
		return errors.New("Admin session ID missing")
	}
	if _, _, err = request(app, "DELETE", "/me/sessions/"+ownID, nil, cookie, 204); err != nil {
		return err
	}
	_, cookie, err = request(app, "POST", "/auth/login", credentials, nil, 200)
	if err != nil {
		return err
	}
	if _, err = operator.Revoke(ctx, email, auth.Moderator); err != nil {
		return err
	}
	if _, _, err = request(app, "GET", "/me", nil, cookie, 401); err != nil {
		return err
	}
	if _, _, err = request(pub, "GET", "/me", nil, publicCookie, 200); err != nil {
		return err
	}
	fmt.Println("Real Admin identities/grants, verified fixture, login/me/CSRF/reauth, sessions, cookie isolation and final-role revocation passed (private values withheld)")
	return nil
}
func cleanup(pool *pgxpool.Pool, email string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return database.SafeError("begin Admin fixture cleanup", err)
	}
	defer tx.Rollback(ctx)
	var id string
	err = tx.QueryRow(ctx, "SELECT user_id::text FROM app.auth_identities WHERE provider='email' AND provider_subject=$1", email).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return database.SafeError("find Admin fixture", err)
	}
	for _, table := range []string{"security_events", "auth_challenges", "sessions", "user_roles", "password_credentials", "user_profiles", "auth_identities"} {
		if _, err = tx.Exec(ctx, "DELETE FROM app."+table+" WHERE user_id=$1", id); err != nil {
			return database.SafeError("remove Admin fixture rows", err)
		}
	}
	if _, err = tx.Exec(ctx, "DELETE FROM app.users WHERE id=$1", id); err != nil {
		return database.SafeError("remove Admin fixture user", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return database.SafeError("commit Admin fixture cleanup", err)
	}
	fmt.Println("Only this run's temporary Admin fixture removed by repository owner")
	return nil
}
