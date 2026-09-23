// auth-smoke exercises the real Public HTTP/application path with a temporary
// identity. Runtime DML uses gfp_api; the migrator removes only this run's fixture.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"uuid"

	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/config"
	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/deepfurry/tap4furry/server/internal/identity"
	"github.com/deepfurry/tap4furry/server/internal/mail"
	"github.com/deepfurry/tap4furry/server/internal/redisstore"
	"github.com/deepfurry/tap4furry/server/internal/transport/health"
	"github.com/deepfurry/tap4furry/server/internal/transport/public"
	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() (result error) {
	if os.Getenv("CI") != "" {
		return errors.New("real development auth smoke refuses CI")
	}
	cfg, err := config.Load("api")
	if err != nil {
		return err
	}
	if cfg.Environment != "development" {
		return errors.New("auth smoke requires development environment")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	api, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer api.Close()
	if _, err = database.Inspect(ctx, api, "gfp_api", "gfp_dev"); err != nil {
		return err
	}
	cleanupPool, err := database.Open(ctx, os.Getenv("AUTH_SMOKE_CLEANUP_URL"))
	if err != nil {
		return err
	}
	defer cleanupPool.Close()
	if _, err = database.Inspect(ctx, cleanupPool, "gfp_migrator", "gfp_dev"); err != nil {
		return err
	}
	if cfg.MailMode != "local" {
		return errors.New("development auth smoke requires private local mail capture")
	}
	runName := "smoke-" + uuid.NewV7().String()
	runDir := filepath.Join(cfg.MailLocalDir, runName)
	capture, err := mail.NewLocal(filepath.Join("..", ".local"), runDir, cfg.PublicOrigin)
	if err != nil {
		return err
	}
	defer capture.Close()
	captureFiles, err := os.OpenRoot(runDir)
	if err != nil {
		return errors.New("private smoke capture unavailable")
	}
	defer captureFiles.Close()
	defer func() {
		captureFiles.Close()
		capture.Close()
		// This random child was created by this run. Confined removal cannot reach
		// other captures, prepared configuration, or files outside the private root.
		root, err := os.OpenRoot(cfg.MailLocalDir)
		if err != nil {
			result = errors.Join(result, errors.New("smoke capture cleanup unavailable"))
			return
		}
		defer root.Close()
		if err := root.RemoveAll(runName); err != nil {
			result = errors.Join(result, errors.New("smoke capture cleanup failed"))
		}
	}()
	store, err := redisstore.Open(cfg.RedisURL, cfg.RedisKeyPrefix)
	if err != nil {
		return err
	}
	defer store.Close()
	throttle, err := store.AuthThrottle(cfg.AuthThrottleSecret)
	if err != nil {
		return err
	}
	authentication, err := auth.NewWithThrottle(api, capture, throttle)
	if err != nil {
		return err
	}
	app := fiber.New()
	public.Register(app, health.New(func(context.Context) error { return nil }, func(context.Context) error { return nil }), authentication, identity.New(api), public.Options{Environment: cfg.Environment, PublicOrigin: cfg.PublicOrigin, CSRFSecret: cfg.CSRFSecret, ResourcePool: api})
	suffix := strings.ReplaceAll(uuid.NewV7().String(), "-", "")
	email := "p01b-smoke-" + suffix + "@example.invalid"
	var random [32]byte
	rand.Read(random[:])
	password := base64.RawURLEncoding.EncodeToString(random[:])
	defer func() {
		if err := cleanup(cleanupPool, email); err != nil {
			result = errors.Join(result, err)
		}
	}()
	var request func(string, string, any, *http.Cookie, int) (map[string]any, *http.Cookie, error)
	request = func(method, path string, body any, cookie *http.Cookie, status int) (map[string]any, *http.Cookie, error) {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, nil, errors.New("auth smoke request encoding failed")
		}
		req := httptest.NewRequestWithContext(ctx, method, path, bytes.NewReader(encoded))
		req.Header.Set("Origin", cfg.PublicOrigin)
		req.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			req.AddCookie(cookie)
			if method != "GET" {
				csrf, _, err := request("GET", "/auth/csrf", nil, cookie, 200)
				if err != nil {
					return nil, nil, err
				}
				req.Header.Set("X-CSRF-Token", csrf["csrf_token"].(string))
			}
		}
		response, err := app.Test(req, fiber.TestConfig{Timeout: 10 * time.Second})
		if err != nil {
			return nil, nil, errors.New("auth smoke HTTP request failed (details withheld)")
		}
		defer response.Body.Close()
		if response.StatusCode != status {
			return nil, nil, fmt.Errorf("auth smoke %s %s: expected %d, got %d (body withheld)", method, path, status, response.StatusCode)
		}
		var bodyResult map[string]any
		if status != 204 && json.NewDecoder(response.Body).Decode(&bodyResult) != nil {
			return nil, nil, errors.New("auth smoke invalid response")
		}
		var next *http.Cookie
		if cookies := response.Cookies(); len(cookies) > 0 {
			next = cookies[0]
		}
		return bodyResult, next, nil
	}
	challenge := func(path string) (string, error) {
		files, err := fs.ReadDir(captureFiles.FS(), ".")
		if err != nil {
			return "", errors.New("capture inspection failed")
		}
		for i := len(files) - 1; i >= 0; i-- {
			data, err := captureFiles.ReadFile(files[i].Name())
			var message struct{ To, Link string }
			if err != nil || json.Unmarshal(data, &message) != nil {
				return "", errors.New("capture decode failed")
			}
			link, err := url.Parse(message.Link)
			if err != nil {
				return "", errors.New("capture link invalid")
			}
			if link.Path == path && message.To == email && link.RawQuery == "" {
				values, _ := url.ParseQuery(link.Fragment)
				if token := values.Get("token"); len(token) == 43 {
					return token, nil
				}
			}
		}
		return "", errors.New("private challenge capture missing")
	}
	credentials := map[string]string{"email": email, "password": password}
	_, registered, err := request("POST", "/auth/register", credentials, nil, 201)
	if err != nil {
		return err
	}
	if registered == nil || !registered.HttpOnly {
		return errors.New("auth smoke registration cookie missing")
	}
	if _, _, err = request("GET", "/me", nil, registered, 200); err != nil {
		return err
	}
	verifyToken, err := challenge("/verify-email")
	if err != nil {
		return err
	}
	if _, _, err = request("POST", "/auth/email/verification", map[string]string{"token": verifyToken}, nil, 204); err != nil {
		return err
	}
	verified, _, err := request("GET", "/me", nil, registered, 200)
	if err != nil {
		return err
	}
	if verified["email_verified"] != true {
		return errors.New("real email verification not visible")
	}
	_, loggedIn, err := request("POST", "/auth/login", credentials, nil, 200)
	if err != nil {
		return err
	}
	if loggedIn == nil || !loggedIn.HttpOnly {
		return errors.New("auth smoke login cookie missing")
	}
	listed, _, err := request("GET", "/me/sessions", nil, registered, 200)
	if err != nil {
		return err
	}
	if len(listed["sessions"].([]any)) != 2 {
		return errors.New("real session listing failed")
	}
	actor, err := authentication.Resolve(ctx, loggedIn.Value)
	if err != nil {
		return err
	}
	if _, _, err = request("DELETE", "/me/sessions/"+actor.SessionID.String(), nil, registered, 204); err != nil {
		return err
	}
	if _, _, err = request("GET", "/me", nil, loggedIn, 401); err != nil {
		return err
	}
	if _, _, err = request("POST", "/auth/password/reset/request", map[string]string{"email": email}, nil, 202); err != nil {
		return err
	}
	resetToken, err := challenge("/reset-password")
	if err != nil {
		return err
	}
	rand.Read(random[:])
	resetPassword := base64.RawURLEncoding.EncodeToString(random[:])
	_, resetCookie, err := request("POST", "/auth/password/reset", map[string]string{"token": resetToken, "new_password": resetPassword}, nil, 200)
	if err != nil {
		return err
	}
	if resetCookie == nil {
		return errors.New("real reset replacement cookie missing")
	}
	if _, _, err = request("GET", "/me", nil, registered, 401); err != nil {
		return err
	}
	if _, _, err = request("POST", "/auth/login", credentials, nil, 401); err != nil {
		return err
	}
	if _, _, err = request("GET", "/me", nil, resetCookie, 200); err != nil {
		return err
	}
	_, loggedIn, err = request("POST", "/auth/password/change", map[string]string{"current_password": resetPassword, "new_password": password}, resetCookie, 200)
	if err != nil {
		return err
	}
	if loggedIn == nil {
		return errors.New("real password change replacement cookie missing")
	}
	if _, _, err = request("GET", "/me", nil, resetCookie, 401); err != nil {
		return err
	}
	// Missing and old-session CSRF values must fail without revoking this session.
	oldCSRF, _, err := request("GET", "/auth/csrf", nil, loggedIn, 200)
	if err != nil {
		return err
	}
	_, reauthed, err := request("POST", "/auth/reauthenticate", map[string]string{"password": password}, loggedIn, 204)
	if err != nil {
		return err
	}
	if reauthed == nil {
		return errors.New("real reauthentication cookie missing")
	}
	if _, _, err = request("GET", "/me", nil, loggedIn, 401); err != nil {
		return err
	}
	loggedIn = reauthed
	for _, header := range []string{"", oldCSRF["csrf_token"].(string)} {
		req := httptest.NewRequestWithContext(ctx, "POST", "/auth/logout", nil)
		req.AddCookie(loggedIn)
		req.Header.Set("Origin", cfg.PublicOrigin)
		req.Header.Set("X-CSRF-Token", header)
		response, err := app.Test(req)
		if err != nil {
			return errors.New("real CSRF check failed")
		}
		response.Body.Close()
		if response.StatusCode != 403 {
			return errors.New("real CSRF enforcement failed")
		}
	}
	handle := "smoke-" + suffix[:20]
	if _, _, err = request("PATCH", "/me/profile", map[string]any{"handle": handle, "display_name": "Temporary smoke profile", "bio": nil, "search_engine_indexing": false}, loggedIn, 200); err != nil {
		return err
	}
	profile, _, err := request("GET", "/users/"+handle, nil, nil, 200)
	if err != nil {
		return err
	}
	if len(profile) != 3 || profile["handle"] != handle {
		return errors.New("auth smoke public profile contract failed")
	}
	for _, key := range []string{"handle", "display_name", "bio"} {
		if _, exists := profile[key]; !exists {
			return errors.New("auth smoke public privacy check failed")
		}
	}
	if _, _, err = request("POST", "/auth/logout", nil, loggedIn, 204); err != nil {
		return err
	}
	if _, _, err = request("GET", "/me", nil, loggedIn, 401); err != nil {
		return err
	}
	fmt.Println("Real gfp_api registration, private mail capture, verification, reset/change/reauth rotation, sessions, CSRF, profile and logout passed (all private values withheld)")
	return nil
}

func cleanup(pool *pgxpool.Pool, email string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return database.SafeError("begin temporary identity cleanup", err)
	}
	defer tx.Rollback(ctx)
	// The unguessable address was generated in this process. It is never accepted
	// as CLI input; cleanup can target no user-selected or existing account.
	for _, table := range []string{"security_events", "auth_challenges", "sessions", "password_credentials", "user_profiles"} {
		_, err = tx.Exec(ctx, "DELETE FROM app."+table+" WHERE user_id IN (SELECT user_id FROM app.auth_identities WHERE provider='email' AND provider_subject=$1)", email)
		if err != nil {
			return database.SafeError("temporary identity cleanup", err)
		}
	}
	var id string
	err = tx.QueryRow(ctx, "DELETE FROM app.auth_identities WHERE provider='email' AND provider_subject=$1 RETURNING user_id::text", email).Scan(&id)
	if err != nil {
		return database.SafeError("temporary identity lookup/cleanup", err)
	}
	if _, err = tx.Exec(ctx, "DELETE FROM app.users WHERE id=$1", id); err != nil {
		return database.SafeError("temporary account cleanup", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return database.SafeError("commit temporary identity cleanup", err)
	}
	fmt.Println("Temporary auth smoke identity and sessions removed using repository owner; no runtime privileges widened")
	return nil
}
