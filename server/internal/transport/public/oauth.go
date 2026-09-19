package public

import (
	"errors"
	"time"

	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/transport/public/generated"
	"github.com/gofiber/fiber/v3"
)

func (h *Handler) flowCookie(provider auth.Provider, value string, clear bool) *fiber.Cookie {
	cookie := h.cookie(value, time.Now().UTC().Add(auth.OAuthLifetime), clear)
	cookie.Name = "__Host-tap4furry_oauth_" + string(provider)
	if !cookie.Secure {
		cookie.Name = "tap4furry_oauth_" + string(provider)
	}
	if !clear {
		cookie.MaxAge = int(auth.OAuthLifetime.Seconds())
	}
	return cookie
}
func oauthHeaders(c fiber.Ctx) {
	c.Set("Cache-Control", "no-store")
	c.Set("Referrer-Policy", "no-referrer")
}

// Generated query binding can fail before the callback handler runs. Keep those
// parser errors inside the same no-store, fixed-redirect transport contract.
func (h *Handler) oauthBoundary(c fiber.Ctx) error {
	oauthHeaders(c)
	if err := c.Next(); err != nil {
		return h.oauthRedirect(c, auth.OAuthLogin, auth.ErrProviderInvalid)
	}
	return nil
}
func (h *Handler) oauthRedirect(c fiber.Ctx, mode auth.OAuthMode, err error) error {
	path := "/account"
	if err != nil {
		if mode == auth.OAuthLogin {
			path = "/login"
		}
		path += "?oauth_error=" + oauthError(err)
	}
	oauthHeaders(c)
	c.Set("Location", h.options.PublicOrigin+path)
	return c.SendStatus(302)
}
func oauthError(err error) string {
	switch {
	case errors.Is(err, auth.ErrProviderDenied):
		return "provider_denied"
	case errors.Is(err, auth.ErrProviderInvalid), errors.Is(err, auth.ErrProviderNotLinked):
		return "provider_invalid"
	case errors.Is(err, auth.ErrAccountLinkRequired):
		return "account_link_required"
	case errors.Is(err, auth.ErrProviderAlreadyLinked):
		return "provider_already_linked"
	case errors.Is(err, auth.ErrReauthRequired):
		return "reauth_required"
	case errors.Is(err, auth.ErrReauthFailed), errors.Is(err, auth.ErrUnauthenticated):
		return "reauth_failed"
	default:
		return "provider_unavailable"
	}
}
func (h *Handler) StartOAuth(c fiber.Ctx, provider generated.Provider) error {
	oauthHeaders(c)
	start, err := h.auth.BeginOAuth(c.Context(), auth.Provider(provider), auth.OAuthLogin, nil)
	if err != nil {
		return h.oauthRedirect(c, auth.OAuthLogin, err)
	}
	c.Cookie(h.flowCookie(auth.Provider(provider), auth.OAuthStateDigest(start.State), false))
	c.Set("Location", start.URL)
	return c.SendStatus(302)
}
func (h *Handler) CompleteOAuth(c fiber.Ctx, provider generated.Provider, params generated.CompleteOAuthParams) error {
	oauthHeaders(c)
	if !auth.Provider(provider).Valid() {
		return h.oauthRedirect(c, auth.OAuthLogin, auth.ErrProviderInvalid)
	}
	input := auth.OAuthCallback{Provider: auth.Provider(provider), Denied: params.Error != nil}
	if params.State != nil {
		input.State = *params.State
	}
	if params.Code != nil {
		input.Code = *params.Code
	}
	input.BrowserState = c.Cookies(h.flowCookie(input.Provider, "", false).Name)
	c.Cookie(h.flowCookie(input.Provider, "", true))
	if actor, err := h.actor(c); err == nil {
		input.Actor = &actor
	}
	result, err := h.auth.CompleteOAuth(c.Context(), input)
	if err == nil {
		c.Cookie(h.cookie(result.Grant.Token, result.Grant.ExpiresAt, false))
	}
	return h.oauthRedirect(c, result.Mode, err)
}
func (h *Handler) GetAuthMethods(c fiber.Ctx) error {
	actor, err := h.actor(c)
	if err != nil {
		return respondError(c, err)
	}
	methods, err := h.auth.Methods(c.Context(), actor)
	if err != nil {
		return respondError(c, err)
	}
	result := generated.AuthMethods{Password: methods.Password, Providers: make([]generated.ProviderMethod, 0, len(methods.Providers))}
	for _, method := range methods.Providers {
		result.Providers = append(result.Providers, generated.ProviderMethod{Provider: generated.OAuthProvider(method.Provider), Email: method.Email, LinkedAt: method.LinkedAt})
	}
	return c.JSON(result)
}
func (h *Handler) startProviderMutation(c fiber.Ctx, provider generated.Provider, mode auth.OAuthMode) error {
	oauthHeaders(c)
	actor, err := h.actor(c)
	if err != nil {
		return respondError(c, err)
	}
	start, err := h.auth.BeginOAuth(c.Context(), auth.Provider(provider), mode, &actor)
	if err != nil {
		return respondError(c, err)
	}
	c.Cookie(h.flowCookie(auth.Provider(provider), auth.OAuthStateDigest(start.State), false))
	return c.JSON(generated.OAuthAuthorization{AuthorizationUrl: start.URL})
}
func (h *Handler) LinkOAuthProvider(c fiber.Ctx, provider generated.Provider, _ generated.LinkOAuthProviderParams) error {
	return h.startProviderMutation(c, provider, auth.OAuthLink)
}
func (h *Handler) ReauthenticateOAuthProvider(c fiber.Ctx, provider generated.Provider, _ generated.ReauthenticateOAuthProviderParams) error {
	return h.startProviderMutation(c, provider, auth.OAuthReauth)
}
func (h *Handler) UnlinkOAuthProvider(c fiber.Ctx, provider generated.Provider, _ generated.UnlinkOAuthProviderParams) error {
	actor, err := h.actor(c)
	if err != nil {
		return respondError(c, err)
	}
	grant, err := h.auth.UnlinkProvider(c.Context(), actor, auth.Provider(provider))
	return h.granted(c, grant, err, 200)
}
