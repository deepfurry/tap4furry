package auth

import (
	"context"
	"errors"
	"log/slog"
	"time"
	"uuid"

	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ThrottleOperation string

const (
	PublicLoginLimit   ThrottleOperation = "public_login"
	AdminLoginLimit    ThrottleOperation = "admin_login"
	RegistrationLimit  ThrottleOperation = "registration"
	PasswordResetLimit ThrottleOperation = "password_reset"
	PublicReauthLimit  ThrottleOperation = "public_reauth"
	AdminReauthLimit   ThrottleOperation = "admin_reauth"
)

type ThrottlePolicy struct {
	Subject, Global int
	Window          time.Duration
}

func PolicyFor(operation ThrottleOperation) (ThrottlePolicy, bool) {
	switch operation {
	case PublicLoginLimit:
		return ThrottlePolicy{10, 500, 15 * time.Minute}, true
	case AdminLoginLimit:
		return ThrottlePolicy{5, 100, 15 * time.Minute}, true
	case RegistrationLimit, PasswordResetLimit:
		return ThrottlePolicy{3, 200, time.Hour}, true
	case PublicReauthLimit:
		return ThrottlePolicy{5, 500, 15 * time.Minute}, true
	case AdminReauthLimit:
		return ThrottlePolicy{5, 100, 15 * time.Minute}, true
	}
	return ThrottlePolicy{}, false
}

type ThrottleDecision struct {
	Allowed    bool
	RetryAfter time.Duration
}

// Auth owns this consumer boundary. The adapter retains only HMAC fingerprints,
// bounded counters and TTLs; no IP or forwarded request headers are inputs.
type Throttle interface {
	Check(context.Context, ThrottleOperation, string) (ThrottleDecision, error)
	Record(context.Context, ThrottleOperation, string) (ThrottleDecision, error)
	ClearSubject(context.Context, ThrottleOperation, string) error
}

var (
	ErrRateLimited         = errors.New("authentication rate limited")
	ErrThrottleUnavailable = errors.New("authentication throttle unavailable")
)

func NewWithThrottle(pool *pgxpool.Pool, mailer ChallengeMailer, throttle Throttle, oauth ...OAuthConfig) (*App, error) {
	app, err := New(pool, mailer, oauth...)
	if err != nil {
		return nil, err
	}
	app.throttle = throttle
	return app, nil
}
func (a *App) throttleCheck(ctx context.Context, op ThrottleOperation, subject string, record bool) (bool, error) {
	var decision ThrottleDecision
	err := ErrThrottleUnavailable
	if a.throttle != nil {
		if record {
			decision, err = a.throttle.Record(ctx, op, subject)
		} else {
			decision, err = a.throttle.Check(ctx, op, subject)
		}
	}
	if err != nil {
		slog.WarnContext(ctx, "auth throttle unavailable (details withheld)", "operation", string(op))
		if op == AdminLoginLimit || op == AdminReauthLimit {
			return false, ErrThrottleUnavailable
		}
		return false, nil // PostgreSQL local auth remains available; skip failure events.
	}
	if !decision.Allowed {
		return true, ErrRateLimited
	}
	return true, nil
}
func (a *App) throttleSuccess(ctx context.Context, op ThrottleOperation, subject string) {
	if a.throttle != nil && a.throttle.ClearSubject(ctx, op, subject) != nil {
		slog.WarnContext(ctx, "auth throttle clear unavailable (details withheld)", "operation", string(op))
	}
}
func (a *App) throttleFailure(ctx context.Context, op ThrottleOperation, subject string, known uuid.UUID, event eventType) {
	if a.throttle == nil {
		return
	}
	decision, err := a.throttle.Record(ctx, op, subject)
	if err != nil {
		slog.WarnContext(ctx, "auth throttle record unavailable (details withheld)", "operation", string(op))
		return
	}
	// Saturating atomic counters bound persistent events, including concurrent
	// failures. Unknown subjects and fail-open outages never create audit rows.
	if decision.Allowed && known != uuid.Nil() {
		if recordEvent(ctx, sqlc.New(a.pool), event, known, uuid.Nil(), a.now().UTC()) != nil {
			slog.WarnContext(ctx, "auth failure event unavailable (details withheld)")
		}
	}
}
func (a *App) Login(ctx context.Context, email, password string) (Grant, error) {
	normalized, err := NormalizeEmail(email)
	if err != nil {
		return Grant{}, err
	}
	if err = ValidatePassword(password); err != nil {
		return Grant{}, err
	}
	healthy, err := a.throttleCheck(ctx, PublicLoginLimit, normalized, false)
	if err != nil {
		return Grant{}, err
	}
	grant, err := a.login(ctx, normalized, password)
	if err == nil {
		a.throttleSuccess(ctx, PublicLoginLimit, normalized)
	} else if healthy && (errors.Is(err, ErrInvalidCredentials) || errors.Is(err, ErrAccountDisabled)) {
		var known uuid.UUID
		row, lookupErr := sqlc.New(a.pool).FindLocalCredentialByEmailSubject(ctx, normalized)
		if lookupErr == nil {
			known = uuid.UUID(row.ID.Bytes)
		}
		a.throttleFailure(ctx, PublicLoginLimit, normalized, known, loginFailed)
	}
	return grant, err
}
