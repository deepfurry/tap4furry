package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"maps"
	"time"
	"uuid"

	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/jackc/pgx/v5"
)

type Provider string

const (
	Google Provider = "google"
	GitHub Provider = "github"
)

func (p Provider) Valid() bool { return p == Google || p == GitHub }

type OAuthMode string

const (
	OAuthLogin  OAuthMode = "login"
	OAuthLink   OAuthMode = "link"
	OAuthReauth OAuthMode = "reauth"
)
const OAuthLifetime = 10 * time.Minute
const FreshAuthWindow = 15 * time.Minute

var (
	ErrProviderUnavailable   = errors.New("OAuth provider unavailable")
	ErrProviderInvalid       = errors.New("OAuth flow or identity invalid")
	ErrProviderDenied        = errors.New("OAuth authorization denied")
	ErrProviderAlreadyLinked = errors.New("OAuth provider already linked")
	ErrProviderNotLinked     = errors.New("OAuth provider not linked")
	ErrAccountLinkRequired   = errors.New("explicit account linking required")
	ErrReauthRequired        = errors.New("fresh authentication required")
	ErrLastMethod            = errors.New("cannot remove last authentication method")
)

// OAuthFlow is the complete, short-lived Redis payload. It contains no provider
// tokens, email, client secret, raw session token or CSRF value.
type OAuthFlow struct {
	Provider  Provider  `json:"provider"`
	Mode      OAuthMode `json:"mode"`
	Verifier  string    `json:"verifier"`
	Nonce     string    `json:"nonce,omitempty"`
	UserID    uuid.UUID `json:"user_id,omitzero"`
	SessionID uuid.UUID `json:"session_id,omitzero"`
	CreatedAt time.Time `json:"created_at"`
}
type OAuthFlowStore interface {
	PutOAuthFlow(context.Context, string, OAuthFlow) error
	ConsumeOAuthFlow(context.Context, string) (OAuthFlow, error)
}
type OAuthAuthorization struct{ URL, Verifier, Nonce string }
type ProviderIdentity struct {
	Provider                    Provider
	Subject, Email, DisplayName string
	EmailVerified               bool
	EmailAuthoritative          bool
}
type OAuthProvider interface {
	AuthorizationURL(context.Context, string, OAuthMode) (OAuthAuthorization, error)
	Exchange(context.Context, string, OAuthFlow) (ProviderIdentity, error)
}
type OAuthConfig struct {
	Flows     OAuthFlowStore
	Providers map[Provider]OAuthProvider
}
type OAuthStart struct{ URL, State string }
type OAuthCallback struct {
	Provider                  Provider
	State, Code, BrowserState string
	Denied                    bool
	Actor                     *Actor
}
type OAuthResult struct {
	Grant Grant
	Mode  OAuthMode
}

// The digest is used both for the Redis key and a short-lived HttpOnly browser
// binding cookie. A state issued in another browser cannot log this browser in.
func OAuthStateDigest(state string) string {
	hash := sha256.Sum256([]byte(state))
	return hex.EncodeToString(hash[:])
}

func copyOAuthConfig(config OAuthConfig) (OAuthConfig, error) {
	if len(config.Providers) > 0 && config.Flows == nil {
		return OAuthConfig{}, errors.New("OAuth flow store required")
	}
	for provider, adapter := range config.Providers {
		if !provider.Valid() || adapter == nil {
			return OAuthConfig{}, ErrProviderInvalid
		}
	}
	config.Providers = maps.Clone(config.Providers)
	return config, nil
}

func (a *App) BeginOAuth(ctx context.Context, provider Provider, mode OAuthMode, actor *Actor) (OAuthStart, error) {
	adapter, ok := a.oauth.Providers[provider]
	if !provider.Valid() {
		return OAuthStart{}, ErrProviderInvalid
	}
	if !ok || a.oauth.Flows == nil {
		return OAuthStart{}, ErrProviderUnavailable
	}
	flow := OAuthFlow{Provider: provider, Mode: mode, CreatedAt: a.now().UTC()}
	if mode != OAuthLogin {
		if mode != OAuthLink && mode != OAuthReauth {
			return OAuthStart{}, ErrProviderInvalid
		}
		if actor == nil {
			return OAuthStart{}, ErrUnauthenticated
		}
		tx, err := a.pool.Begin(ctx)
		if err != nil {
			return OAuthStart{}, database.SafeError("begin OAuth authorization", err)
		}
		defer tx.Rollback(ctx)
		q := sqlc.New(tx)
		if err = lockUser(ctx, q, actor.UserID); err != nil {
			return OAuthStart{}, err
		}
		current, err := currentActor(ctx, q, *actor, a.now().UTC(), mode == OAuthLink)
		if err != nil {
			return OAuthStart{}, err
		}
		if mode == OAuthReauth {
			methods, err := q.ListProviderIdentitiesForUser(ctx, dbID(current.UserID))
			if err != nil {
				return OAuthStart{}, database.SafeError("read linked provider", err)
			}
			linked := false
			for _, method := range methods {
				if method.Provider == string(provider) {
					linked = true
				}
			}
			if !linked {
				return OAuthStart{}, ErrProviderNotLinked
			}
		}
		flow.UserID, flow.SessionID = current.UserID, current.SessionID
		if err = tx.Commit(ctx); err != nil {
			return OAuthStart{}, database.SafeError("commit OAuth authorization", err)
		}
	}
	state := newToken()
	authorization, err := adapter.AuthorizationURL(ctx, state, mode)
	if err != nil {
		return OAuthStart{}, err
	}
	flow.Verifier, flow.Nonce = authorization.Verifier, authorization.Nonce
	if len(flow.Verifier) < 43 || (provider == Google && len(flow.Nonce) < 43) {
		return OAuthStart{}, ErrProviderInvalid
	}
	if err = a.oauth.Flows.PutOAuthFlow(ctx, state, flow); err != nil {
		return OAuthStart{}, err
	}
	return OAuthStart{URL: authorization.URL, State: state}, nil
}

func (a *App) CompleteOAuth(ctx context.Context, input OAuthCallback) (OAuthResult, error) {
	result := OAuthResult{Mode: OAuthLogin}
	if _, err := tokenHash(input.State); err != nil || !input.Provider.Valid() {
		return result, ErrProviderInvalid
	}
	adapter, ok := a.oauth.Providers[input.Provider]
	if !ok || a.oauth.Flows == nil {
		return result, ErrProviderUnavailable
	}
	flow, err := a.oauth.Flows.ConsumeOAuthFlow(ctx, input.State)
	if err != nil {
		return result, err
	}
	if flow.Mode == OAuthLink || flow.Mode == OAuthReauth {
		result.Mode = flow.Mode
	}
	now := a.now().UTC()
	if flow.Provider != input.Provider || (flow.Mode != OAuthLogin && flow.Mode != OAuthLink && flow.Mode != OAuthReauth) ||
		flow.CreatedAt.IsZero() || len(flow.Verifier) < 43 || (flow.Provider == Google && len(flow.Nonce) < 43) || !flow.CreatedAt.Add(OAuthLifetime).After(now) || flow.CreatedAt.After(now.Add(time.Second)) ||
		subtle.ConstantTimeCompare([]byte(OAuthStateDigest(input.State)), []byte(input.BrowserState)) != 1 {
		return result, ErrProviderInvalid
	}
	if input.Denied {
		return result, ErrProviderDenied
	}
	if input.Code == "" || len(input.Code) > 4096 {
		return result, ErrProviderInvalid
	}
	if flow.Mode != OAuthLogin && (input.Actor == nil || input.Actor.UserID != flow.UserID || input.Actor.SessionID != flow.SessionID) {
		return result, ErrReauthFailed
	}
	identity, err := adapter.Exchange(ctx, input.Code, flow)
	if err != nil {
		return result, err
	}
	if identity.Provider != input.Provider || identity.Subject == "" || len(identity.Subject) > 255 {
		return result, ErrProviderInvalid
	}
	if flow.Mode == OAuthLogin {
		result.Grant, err = a.oauthLogin(ctx, identity, flow.CreatedAt)
	} else {
		result.Grant, err = a.oauthMutation(ctx, identity, flow, *input.Actor)
	}
	return result, err
}

func currentActor(ctx context.Context, q *sqlc.Queries, actor Actor, now time.Time, fresh bool) (Actor, error) {
	if actor.SessionKind != "public" {
		return Actor{}, ErrUnauthenticated
	}
	row, err := q.GetActivePublicSessionByID(ctx, sqlc.GetActivePublicSessionByIDParams{ID: dbID(actor.SessionID), UserID: dbID(actor.UserID), Now: timestamp(now)})
	if errors.Is(err, pgx.ErrNoRows) {
		return Actor{}, ErrUnauthenticated
	}
	if err != nil {
		return Actor{}, database.SafeError("revalidate OAuth session", err)
	}
	actor.AuthMethod, actor.AuthenticatedAt = row.AuthMethod, row.AuthenticatedAt.Time
	if fresh && (actor.AuthenticatedAt.After(now) || actor.AuthenticatedAt.Add(FreshAuthWindow).Before(now)) {
		return Actor{}, ErrReauthRequired
	}
	return actor, nil
}
