package admin

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/deepfurry/gofurry-platform/server/internal/auth"
	"github.com/deepfurry/gofurry-platform/server/internal/identity"
	"github.com/deepfurry/gofurry-platform/server/internal/transport/admin/generated"
	"github.com/gofiber/fiber/v3"
)

var (
	errOrigin = errors.New("origin rejected")
	errCSRF   = errors.New("csrf invalid")
)

type actorKey struct{}

func (h *Handler) cookie(token string, expires time.Time, clear bool) *fiber.Cookie {
	secure := h.options.Environment != "development" && h.options.Environment != "test"
	name := "__Host-gofurry_admin_session"
	if !secure {
		name = "gofurry_admin_session"
	}
	cookie := &fiber.Cookie{Name: name, Value: token, Path: "/", Secure: secure, HTTPOnly: true, SameSite: "Strict", Expires: expires}
	if clear {
		cookie.Value = ""
		cookie.MaxAge = -1
		cookie.Expires = time.Unix(1, 0).UTC()
	}
	return cookie
}
func (h *Handler) actor(c fiber.Ctx) (auth.AdminActor, error) {
	if actor, ok := c.Locals(actorKey{}).(auth.AdminActor); ok {
		return actor, nil
	}
	if h.options.Auth == nil {
		return auth.AdminActor{}, auth.ErrAdminUnauthenticated
	}
	actor, err := h.options.Auth.ResolveAdmin(c.Context(), c.Cookies(h.cookie("", time.Time{}, false).Name))
	if err == nil {
		c.Locals(actorKey{}, actor)
	}
	return actor, err
}
func (h *Handler) originGuard(c fiber.Ctx) error {
	switch c.Method() {
	case "POST", "PUT", "PATCH", "DELETE":
		if h.options.AdminOrigin == "" || c.Get("Origin") != h.options.AdminOrigin {
			return respondError(c, errOrigin)
		}
	}
	return c.Next()
}
func csrfToken(secret, session string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(session))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (h *Handler) csrfGuard(c fiber.Ctx) error {
	switch c.Method() {
	case "POST", "PUT", "PATCH", "DELETE":
		if _, err := h.actor(c); err != nil {
			return respondError(c, err)
		}
		expected := csrfToken(h.options.CSRFSecret, c.Cookies(h.cookie("", time.Time{}, false).Name))
		if len(h.options.CSRFSecret) < 32 || len(c.GetReqHeaders()[http.CanonicalHeaderKey("X-CSRF-Token")]) != 1 || !hmac.Equal([]byte(expected), []byte(c.Get("X-CSRF-Token"))) {
			return respondError(c, errCSRF)
		}
	}
	return c.Next()
}
func decode(c fiber.Ctx, target any, allowed ...string) error {
	body := c.Body()
	media, _, err := mime.ParseMediaType(c.Get("Content-Type"))
	if err != nil || media != "application/json" || len(body) > 8192 || !utf8.Valid(body) {
		return identity.ErrValidation
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil || fields == nil {
		return identity.ErrValidation
	}
	if len(fields) != len(allowed) {
		return identity.ErrValidation
	}
	for _, key := range allowed {
		if fields[key] == nil || string(fields[key]) == "null" {
			return identity.ErrValidation
		}
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if d.Decode(target) != nil || d.Decode(new(any)) != io.EOF {
		return identity.ErrValidation
	}
	return nil
}
func (h *Handler) Login(c fiber.Ctx) error {
	var input generated.Credentials
	if decode(c, &input, "email", "password") != nil || input.Password == nil {
		return respondError(c, identity.ErrValidation)
	}
	if h.options.Auth == nil {
		return respondError(c, auth.ErrThrottleUnavailable)
	}
	grant, err := h.options.Auth.AdminLogin(c.Context(), input.Email, *input.Password)
	if err != nil {
		return respondError(c, err)
	}
	c.Cookie(h.cookie(grant.Token, grant.ExpiresAt, false))
	return c.JSON(meDTO(grant.Me))
}
func (h *Handler) GetMe(c fiber.Ctx) error {
	actor, err := h.actor(c)
	if err != nil {
		return respondError(c, err)
	}
	me, err := h.options.Auth.AdminMe(c.Context(), actor)
	if err != nil {
		return respondError(c, err)
	}
	return c.JSON(meDTO(me))
}
func meDTO(me auth.AdminMe) generated.AdminMe {
	roles := make([]generated.Role, 0, len(me.Roles))
	for _, role := range me.Roles {
		roles = append(roles, generated.Role(role))
	}
	return generated.AdminMe{Id: me.ID.String(), Email: me.Email, Roles: roles, AuthenticatedAt: me.AuthenticatedAt}
}
func (h *Handler) GetCsrf(c fiber.Ctx) error {
	if _, err := h.actor(c); err != nil {
		return respondError(c, err)
	}
	if len(h.options.CSRFSecret) < 32 {
		return respondError(c, errors.New("csrf unavailable"))
	}
	return c.JSON(generated.CsrfToken{CsrfToken: csrfToken(h.options.CSRFSecret, c.Cookies(h.cookie("", time.Time{}, false).Name))})
}
func (h *Handler) Logout(c fiber.Ctx, _ generated.LogoutParams) error {
	actor, err := h.actor(c)
	if err != nil {
		return respondError(c, err)
	}
	if err = h.options.Auth.AdminLogout(c.Context(), actor); err != nil {
		return respondError(c, err)
	}
	c.Cookie(h.cookie("", time.Time{}, true))
	return c.SendStatus(204)
}
func (h *Handler) Reauthenticate(c fiber.Ctx, _ generated.ReauthenticateParams) error {
	actor, err := h.actor(c)
	if err != nil {
		return respondError(c, err)
	}
	var input generated.Reauthentication
	if decode(c, &input, "password") != nil || input.Password == nil {
		return respondError(c, identity.ErrValidation)
	}
	grant, err := h.options.Auth.AdminReauthenticate(c.Context(), actor, *input.Password)
	if err != nil {
		return respondError(c, err)
	}
	c.Cookie(h.cookie(grant.Token, grant.ExpiresAt, false))
	return c.SendStatus(204)
}
func (h *Handler) ListSessions(c fiber.Ctx) error {
	actor, err := h.actor(c)
	if err != nil {
		return respondError(c, err)
	}
	rows, err := h.options.Auth.AdminSessions(c.Context(), actor)
	if err != nil {
		return respondError(c, err)
	}
	result := generated.SessionList{Sessions: make([]generated.Session, 0, len(rows))}
	for _, row := range rows {
		result.Sessions = append(result.Sessions, generated.Session{Id: row.ID.String(), AuthenticatedAt: row.AuthenticatedAt, CreatedAt: row.CreatedAt, LastSeenAt: row.LastSeenAt, IdleExpiresAt: row.IdleExpiresAt, AbsoluteExpiresAt: row.AbsoluteExpiresAt, Current: row.Current})
	}
	return c.JSON(result)
}
func (h *Handler) RevokeSession(c fiber.Ctx, sessionID string, _ generated.RevokeSessionParams) error {
	actor, err := h.actor(c)
	if err != nil {
		return respondError(c, err)
	}
	target, err := uuid.Parse(sessionID)
	if err != nil {
		return respondError(c, auth.ErrSessionNotFound)
	}
	if err = h.options.Auth.AdminRevokeSession(c.Context(), actor, target); err != nil {
		return respondError(c, err)
	}
	if target == actor.SessionID {
		c.Cookie(h.cookie("", time.Time{}, true))
	}
	return c.SendStatus(204)
}
func (h *Handler) RevokeOtherSessions(c fiber.Ctx, _ generated.RevokeOtherSessionsParams) error {
	actor, err := h.actor(c)
	if err != nil {
		return respondError(c, err)
	}
	if err = h.options.Auth.AdminRevokeOthers(c.Context(), actor); err != nil {
		return respondError(c, err)
	}
	return c.SendStatus(204)
}
func respondError(c fiber.Ctx, err error) error {
	status, code, message := 500, generated.INTERNALERROR, "The request could not be completed."
	switch {
	case errors.Is(err, identity.ErrValidation):
		status, code, message = 400, generated.VALIDATIONERROR, "Check the supplied fields."
	case errors.Is(err, auth.ErrAdminCredentials):
		status, code, message = 401, generated.ADMININVALIDCREDENTIALS, "Admin sign-in could not be verified."
	case errors.Is(err, auth.ErrAdminUnauthenticated):
		status, code, message = 401, generated.ADMINUNAUTHENTICATED, "Please sign in to Admin."
	case errors.Is(err, auth.ErrAdminForbidden):
		status, code, message = 403, generated.ADMINFORBIDDEN, "Admin access is unavailable."
	case errors.Is(err, auth.ErrSessionNotFound):
		status, code, message = 404, generated.ADMINSESSIONNOTFOUND, "Session not found."
	case errors.Is(err, errOrigin):
		status, code, message = 403, generated.ORIGINFORBIDDEN, "Request origin is not allowed."
	case errors.Is(err, errCSRF):
		status, code, message = 403, generated.CSRFINVALID, "Request verification failed."
	case errors.Is(err, auth.ErrRateLimited):
		status, code, message = 429, generated.AUTHRATELIMITED, "Too many attempts. Please try again later."
	case errors.Is(err, auth.ErrThrottleUnavailable):
		status = 503
	}
	c.Set("Cache-Control", "no-store")
	return c.Status(status).JSON(generated.ApiError{Code: code, Message: message})
}
