package public

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"
	"uuid"

	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/identity"
	"github.com/deepfurry/tap4furry/server/internal/transport/public/generated"
	"github.com/gofiber/fiber/v3"
)

var errCSRF = errors.New("csrf invalid")

func csrfToken(secret, session string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(session))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
func (h *Handler) csrfGuard(c fiber.Ctx) error {
	switch c.Method() {
	case "POST", "PUT", "PATCH", "DELETE":
		if _, err := h.actor(c); err != nil {
			return respondError(c, err)
		}
		expected := csrfToken(h.options.CSRFSecret, c.Cookies(h.cookie("", time.Time{}, false).Name))
		if len(h.options.CSRFSecret) < 32 || !hmac.Equal([]byte(expected), []byte(c.Get("X-CSRF-Token"))) {
			return respondError(c, errCSRF)
		}
	}
	return c.Next()
}
func (h *Handler) GetCsrf(c fiber.Ctx) error {
	if _, err := h.actor(c); err != nil {
		return respondError(c, err)
	}
	if len(h.options.CSRFSecret) < 32 {
		return respondError(c, errors.New("csrf not configured"))
	}
	return c.JSON(generated.CsrfToken{CsrfToken: csrfToken(h.options.CSRFSecret, c.Cookies(h.cookie("", time.Time{}, false).Name))})
}
func (h *Handler) RequestEmailVerification(c fiber.Ctx, _ generated.RequestEmailVerificationParams) error {
	actor, err := h.actor(c)
	if err != nil {
		return respondError(c, err)
	}
	if err = h.auth.RequestVerification(c.Context(), actor); err != nil {
		return respondError(c, err)
	}
	return accepted(c)
}
func (h *Handler) VerifyEmail(c fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	var input generated.ChallengeToken
	if _, err := decodeBody(c, &input, "token"); err != nil || input.Token == nil {
		return respondError(c, identity.ErrValidation)
	}
	if err := h.auth.VerifyEmail(c.Context(), *input.Token); err != nil {
		return respondError(c, err)
	}
	return c.SendStatus(204)
}
func accepted(c fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	return c.Status(202).JSON(generated.Accepted{Message: "If this request is eligible, an email will be sent. Please check your inbox."})
}
func (h *Handler) RequestPasswordReset(c fiber.Ctx) error {
	var input generated.ResetRequest
	fields, err := decodeBody(c, &input, "email")
	if err != nil || fields["email"] == nil || string(fields["email"]) == "null" {
		return respondError(c, identity.ErrValidation)
	}
	h.auth.RequestPasswordReset(c.Context(), input.Email)
	return accepted(c)
}
func (h *Handler) ResetPassword(c fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	var input generated.PasswordReset
	if _, err := decodeBody(c, &input, "token", "new_password"); err != nil || input.Token == nil || input.NewPassword == nil {
		return respondError(c, identity.ErrValidation)
	}
	grant, err := h.auth.ResetPassword(c.Context(), *input.Token, *input.NewPassword)
	return h.granted(c, grant, err, 200)
}
func (h *Handler) ChangePassword(c fiber.Ctx, _ generated.ChangePasswordParams) error {
	actor, err := h.actor(c)
	if err != nil {
		return respondError(c, err)
	}
	var input generated.PasswordChange
	if _, err = decodeBody(c, &input, "current_password", "new_password"); err != nil || input.CurrentPassword == nil || input.NewPassword == nil {
		return respondError(c, identity.ErrValidation)
	}
	grant, err := h.auth.ChangePassword(c.Context(), actor, *input.CurrentPassword, *input.NewPassword)
	return h.granted(c, grant, err, 200)
}
func (h *Handler) Reauthenticate(c fiber.Ctx, _ generated.ReauthenticateParams) error {
	actor, err := h.actor(c)
	if err != nil {
		return respondError(c, err)
	}
	var input generated.Reauthentication
	if _, err = decodeBody(c, &input, "password"); err != nil || input.Password == nil {
		return respondError(c, identity.ErrValidation)
	}
	grant, err := h.auth.Reauthenticate(c.Context(), actor, *input.Password)
	return h.granted(c, grant, err, 204)
}
func (h *Handler) granted(c fiber.Ctx, grant auth.Grant, err error, status int) error {
	if err != nil {
		return respondError(c, err)
	}
	c.Cookie(h.cookie(grant.Token, grant.ExpiresAt, false))
	if status == 204 {
		return c.SendStatus(204)
	}
	return c.Status(status).JSON(meDTO(grant.Me))
}
func (h *Handler) ListSessions(c fiber.Ctx) error {
	actor, err := h.actor(c)
	if err != nil {
		return respondError(c, err)
	}
	sessions, err := h.auth.Sessions(c.Context(), actor)
	if err != nil {
		return respondError(c, err)
	}
	result := generated.SessionList{Sessions: make([]generated.Session, 0, len(sessions))}
	for _, session := range sessions {
		result.Sessions = append(result.Sessions, generated.Session{Id: session.ID.String(), AuthMethod: generated.SessionAuthMethod(session.AuthMethod),
			AuthenticatedAt: session.AuthenticatedAt, CreatedAt: session.CreatedAt, LastSeenAt: session.LastSeenAt,
			IdleExpiresAt: session.IdleExpiresAt, AbsoluteExpiresAt: session.AbsoluteExpiresAt, Current: session.Current})
	}
	return c.JSON(result)
}
func (h *Handler) RevokeSession(c fiber.Ctx, sessionID string, _ generated.RevokeSessionParams) error {
	actor, err := h.actor(c)
	if err != nil {
		return respondError(c, err)
	}
	id, err := uuid.Parse(sessionID)
	if err != nil {
		return respondError(c, auth.ErrSessionNotFound)
	}
	if err = h.auth.RevokeSession(c.Context(), actor, id); err != nil {
		return respondError(c, err)
	}
	if id == actor.SessionID {
		c.Cookie(h.cookie("", time.Time{}, true))
	}
	return c.SendStatus(204)
}
func (h *Handler) RevokeOtherSessions(c fiber.Ctx, _ generated.RevokeOtherSessionsParams) error {
	actor, err := h.actor(c)
	if err != nil {
		return respondError(c, err)
	}
	if err = h.auth.RevokeOthers(c.Context(), actor); err != nil {
		return respondError(c, err)
	}
	return c.SendStatus(204)
}
