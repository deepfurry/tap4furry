package public

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"time"
	"unicode/utf8"

	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/identity"
	"github.com/deepfurry/tap4furry/server/internal/transport/public/generated"
	"github.com/gofiber/fiber/v3"
)

var errOrigin = errors.New("origin rejected")

type actorKey struct{}

func (h *Handler) originGuard(c fiber.Ctx) error {
	switch c.Method() {
	case "POST", "PUT", "PATCH", "DELETE":
		if h.options.PublicOrigin == "" || c.Get("Origin") != h.options.PublicOrigin {
			return respondError(c, errOrigin)
		}
	}
	return c.Next()
}

func (h *Handler) cookie(token string, expires time.Time, clear bool) *fiber.Cookie {
	// Unknown environments fail closed. Only explicit local environments may
	// use the separate insecure development cookie.
	secure := h.options.Environment != "development" && h.options.Environment != "test"
	name := "__Host-tap4furry_session"
	if !secure {
		name = "tap4furry_session"
	}
	cookie := &fiber.Cookie{Name: name, Value: token, Path: "/", Secure: secure, HTTPOnly: true, SameSite: "Lax", Expires: expires}
	if clear {
		cookie.Value = ""
		cookie.MaxAge = -1
		cookie.Expires = time.Unix(1, 0).UTC()
	}
	return cookie
}

func (h *Handler) actor(c fiber.Ctx) (auth.Actor, error) {
	c.Set("Cache-Control", "no-store")
	if actor, ok := c.Locals(actorKey{}).(auth.Actor); ok {
		return actor, nil
	}
	actor, err := h.auth.Resolve(c.Context(), c.Cookies(h.cookie("", time.Time{}, false).Name))
	if err == nil {
		c.Locals(actorKey{}, actor)
	}
	return actor, err
}

// Decode only JSON objects, with the contract's exact property names. The map
// retains nullable PATCH field presence; generated DTOs still own value types.
func decodeBody(c fiber.Ctx, target any, allowed ...string) (map[string]json.RawMessage, error) {
	body := c.Body()
	mediaType, _, err := mime.ParseMediaType(c.Get("Content-Type"))
	if err != nil || mediaType != "application/json" || len(body) > 8192 || !utf8.Valid(body) {
		return nil, identity.ErrValidation
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil || fields == nil {
		return nil, identity.ErrValidation
	}
	for key := range fields {
		found := false
		for _, name := range allowed {
			if key == name {
				found = true
			}
		}
		if !found {
			return nil, identity.ErrValidation
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil || decoder.Decode(new(any)) != io.EOF {
		return nil, identity.ErrValidation
	}
	return fields, nil
}

func (h *Handler) Register(c fiber.Ctx) error { return h.credentials(c, true) }
func (h *Handler) Login(c fiber.Ctx) error    { return h.credentials(c, false) }
func (h *Handler) credentials(c fiber.Ctx, registration bool) error {
	c.Set("Cache-Control", "no-store")
	var input generated.Credentials
	if _, err := decodeBody(c, &input, "email", "password"); err != nil || input.Password == nil {
		return respondError(c, identity.ErrValidation)
	}
	var grant auth.Grant
	var err error
	code := fiber.StatusOK
	if registration {
		grant, err = h.auth.Register(c.Context(), input.Email, *input.Password)
		code = fiber.StatusCreated
	} else {
		grant, err = h.auth.Login(c.Context(), input.Email, *input.Password)
	}
	if err != nil {
		return respondError(c, err)
	}
	c.Cookie(h.cookie(grant.Token, grant.ExpiresAt, false))
	return c.Status(code).JSON(meDTO(grant.Me))
}

func (h *Handler) Logout(c fiber.Ctx, _ generated.LogoutParams) error {
	actor, err := h.actor(c)
	if err != nil {
		return respondError(c, err)
	}
	if err = h.auth.Logout(c.Context(), actor); err != nil {
		return respondError(c, err)
	}
	c.Cookie(h.cookie("", time.Time{}, true))
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) GetMe(c fiber.Ctx) error {
	actor, err := h.actor(c)
	if err != nil {
		return respondError(c, err)
	}
	me, err := h.identity.Me(c.Context(), actor.UserID)
	if err != nil {
		return respondError(c, err)
	}
	return c.JSON(meDTO(me))
}

func (h *Handler) UpdateProfile(c fiber.Ctx, _ generated.UpdateProfileParams) error {
	actor, err := h.actor(c)
	if err != nil {
		return respondError(c, err)
	}
	var input generated.ProfileUpdate
	fields, err := decodeBody(c, &input, "handle", "display_name", "bio", "search_engine_indexing")
	if err != nil {
		return respondError(c, err)
	}
	_, setHandle := fields["handle"]
	_, setName := fields["display_name"]
	_, setBio := fields["bio"]
	if _, setIndexing := fields["search_engine_indexing"]; setIndexing && input.SearchEngineIndexing == nil {
		return respondError(c, identity.ErrValidation)
	}
	me, err := h.identity.UpdateProfile(c.Context(), actor.UserID, identity.ProfileUpdate{
		Handle: identity.Field[string]{Set: setHandle, Value: input.Handle}, DisplayName: identity.Field[string]{Set: setName, Value: input.DisplayName},
		Bio: identity.Field[string]{Set: setBio, Value: input.Bio}, SearchEngineIndexing: input.SearchEngineIndexing})
	if err != nil {
		return respondError(c, err)
	}
	return c.JSON(meDTO(me))
}

func (h *Handler) GetPublicProfile(c fiber.Ctx, handle string) error {
	profile, err := h.identity.PublicProfile(c.Context(), handle)
	if err != nil {
		return respondError(c, err)
	}
	return c.JSON(generated.PublicProfile{Handle: *profile.Handle, DisplayName: profile.DisplayName, Bio: profile.Bio})
}

func meDTO(me identity.Me) generated.Me {
	return generated.Me{Id: me.ID.String(), AccountState: generated.MeAccountState(me.AccountState), CreatedAt: me.CreatedAt,
		Email: me.Email, EmailVerified: me.EmailVerified, Profile: generated.Profile{Handle: me.Profile.Handle,
			DisplayName: me.Profile.DisplayName, Bio: me.Profile.Bio, SearchEngineIndexing: me.SearchEngineIndexing}}
}

func respondError(c fiber.Ctx, err error) error {
	status, code, message := fiber.StatusInternalServerError, generated.INTERNALERROR, "The request could not be completed."
	switch {
	case errors.Is(err, auth.ErrRateLimited):
		status, code, message = 429, generated.AUTHRATELIMITED, "Too many attempts. Please try again later."
	case errors.Is(err, identity.ErrValidation):
		status, code, message = 400, generated.VALIDATIONERROR, "Check the supplied fields."
	case errors.Is(err, auth.ErrEmailRegistered):
		status, code, message = 409, generated.AUTHEMAILALREADYREGISTERED, "This email is already registered."
	case errors.Is(err, auth.ErrInvalidCredentials):
		status, code, message = 401, generated.AUTHINVALIDCREDENTIALS, "Email or password is incorrect."
	case errors.Is(err, auth.ErrUnauthenticated), errors.Is(err, identity.ErrUnavailable):
		status, code, message = 401, generated.AUTHUNAUTHENTICATED, "Please log in."
	case errors.Is(err, auth.ErrAccountDisabled):
		status, code, message = 403, generated.AUTHACCOUNTDISABLED, "This account is disabled."
	case errors.Is(err, identity.ErrHandleUnavailable):
		status, code, message = 409, generated.PROFILEHANDLEUNAVAILABLE, "This handle is unavailable."
	case errors.Is(err, identity.ErrNotFound):
		status, code, message = 404, generated.PROFILENOTFOUND, "Profile not found."
	case errors.Is(err, errOrigin):
		status, code, message = 403, generated.ORIGINFORBIDDEN, "Request origin is not allowed."
	case errors.Is(err, errCSRF):
		status, code, message = 403, generated.CSRFINVALID, "Request verification failed."
	case errors.Is(err, auth.ErrChallengeInvalid):
		status, code, message = 400, generated.AUTHCHALLENGEINVALID, "This link is invalid or expired."
	case errors.Is(err, auth.ErrReauthFailed):
		status, code, message = 401, generated.AUTHREAUTHFAILED, "Authentication verification failed."
	case errors.Is(err, auth.ErrProviderUnavailable):
		status, code, message = 503, generated.AUTHPROVIDERUNAVAILABLE, "This sign-in provider is unavailable."
	case errors.Is(err, auth.ErrProviderInvalid), errors.Is(err, auth.ErrProviderDenied):
		status, code, message = 400, generated.AUTHPROVIDERINVALID, "Provider authorization is invalid or expired."
	case errors.Is(err, auth.ErrProviderAlreadyLinked):
		status, code, message = 409, generated.AUTHPROVIDERALREADYLINKED, "This provider account cannot be linked."
	case errors.Is(err, auth.ErrProviderNotLinked):
		status, code, message = 409, generated.AUTHPROVIDERNOTLINKED, "This provider is not linked."
	case errors.Is(err, auth.ErrAccountLinkRequired):
		status, code, message = 409, generated.AUTHACCOUNTLINKREQUIRED, "Sign in to your existing account to link this provider."
	case errors.Is(err, auth.ErrReauthRequired):
		status, code, message = 403, generated.AUTHREAUTHREQUIRED, "Reauthenticate with a remaining sign-in method to continue."
	case errors.Is(err, auth.ErrLastMethod):
		status, code, message = 409, generated.AUTHLASTMETHOD, "Keep at least one sign-in method."
	case errors.Is(err, auth.ErrSessionNotFound):
		status, code, message = 404, generated.AUTHSESSIONNOTFOUND, "Session not found."
	case errors.Is(err, auth.ErrMailUnavailable):
		status, code, message = 503, generated.MAILUNAVAILABLE, "Email delivery is unavailable. Please try again later."
	}
	c.Set("Cache-Control", "no-store")
	return c.Status(status).JSON(generated.ApiError{Code: code, Message: message})
}
