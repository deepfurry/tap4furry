// Package auth owns account authentication and canonical PostgreSQL sessions.
package auth

import (
	"context"
	"errors"
	"log/slog"
	"time"
	"uuid"

	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/deepfurry/tap4furry/server/internal/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrEmailRegistered    = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUnauthenticated    = errors.New("authentication required")
	ErrAccountDisabled    = errors.New("account disabled")
	errCredentialChanged  = errors.New("credential changed concurrently")
)

type App struct {
	pool      *pgxpool.Pool
	dummyHash string
	now       func() time.Time
	mailer    ChallengeMailer
	oauth     OAuthConfig
	throttle  Throttle
}
type Grant struct {
	Me        identity.Me
	Token     string `json:"-"`
	ExpiresAt time.Time
}

func New(pool *pgxpool.Pool, mailer ChallengeMailer, oauth ...OAuthConfig) (*App, error) {
	if mailer == nil {
		return nil, errors.New("challenge mailer is required")
	}
	// One random dummy per process, never logged or persisted.
	dummy, err := HashPassword(newToken())
	if err != nil {
		return nil, err
	}
	var providers OAuthConfig
	if len(oauth) > 1 {
		return nil, errors.New("only one OAuth configuration is allowed")
	}
	if len(oauth) == 1 {
		providers, err = copyOAuthConfig(oauth[0])
		if err != nil {
			return nil, err
		}
	}
	return &App{pool: pool, dummyHash: dummy, now: time.Now, mailer: mailer, oauth: providers}, nil
}

func (a *App) Register(ctx context.Context, email, password string) (Grant, error) {
	email, err := NormalizeEmail(email)
	if err != nil {
		return Grant{}, err
	}
	if _, err = a.throttleCheck(ctx, RegistrationLimit, email, true); err != nil {
		return Grant{}, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return Grant{}, err
	}
	userID, identityID, sessionID := uuid.NewV7(), uuid.NewV7(), uuid.NewV7()
	token, now := newToken(), a.now().UTC()
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return Grant{}, database.SafeError("begin registration", err)
	}
	defer tx.Rollback(ctx)
	q := sqlc.New(tx)
	if err = q.CreateUser(ctx, sqlc.CreateUserParams{ID: dbID(userID), CreatedAt: timestamp(now)}); err != nil {
		return Grant{}, database.SafeError("create account", err)
	}
	if err = q.CreateProfile(ctx, sqlc.CreateProfileParams{UserID: dbID(userID), CreatedAt: timestamp(now)}); err != nil {
		return Grant{}, database.SafeError("create profile", err)
	}
	err = q.CreateEmailIdentity(ctx, sqlc.CreateEmailIdentityParams{ID: dbID(identityID), UserID: dbID(userID), Email: email, Now: timestamp(now)})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "auth_identities_provider_provider_subject_key" {
		return Grant{}, ErrEmailRegistered
	}
	if err != nil {
		return Grant{}, database.SafeError("create email identity", err)
	}
	if err = q.CreatePasswordCredential(ctx, sqlc.CreatePasswordCredentialParams{UserID: dbID(userID), PasswordHash: hash, PasswordUpdatedAt: timestamp(now)}); err != nil {
		return Grant{}, database.SafeError("create password credential", err)
	}
	if err = createSession(ctx, q, userID, sessionID, token, "password", now); err != nil {
		return Grant{}, err
	}
	pending, err := issueChallenge(ctx, q, userID, identityID, email, purposeVerify, now)
	if err != nil {
		return Grant{}, err
	}
	if err = recordEvent(ctx, q, accountRegistered, userID, sessionID, now); err != nil {
		return Grant{}, err
	}
	me, err := identity.ReadMe(ctx, q, userID)
	if err != nil {
		return Grant{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Grant{}, database.SafeError("commit registration", err)
	}
	if err = a.deliver(ctx, pending); err != nil {
		slog.Warn("registration challenge delivery unavailable", "component", "auth")
	}
	return Grant{Me: me, Token: token, ExpiresAt: now.Add(AbsoluteLifetime)}, nil
}

func (a *App) login(ctx context.Context, email, password string) (Grant, error) {
	email, err := NormalizeEmail(email)
	if err != nil {
		return Grant{}, err
	}
	if err = ValidatePassword(password); err != nil {
		return Grant{}, err
	}
	for attempt := 0; attempt < 2; attempt++ {
		row, err := sqlc.New(a.pool).FindLocalCredentialByEmailSubject(ctx, email)
		if errors.Is(err, pgx.ErrNoRows) {
			_, _, _, verifyErr := verifyPassword(password, a.dummyHash)
			if verifyErr != nil {
				return Grant{}, verifyErr
			}
			return Grant{}, ErrInvalidCredentials
		}
		if err != nil {
			return Grant{}, database.SafeError("find local credential", err)
		}
		ok, replacement, upgraded, err := verifyPassword(password, row.PasswordHash)
		if err != nil {
			return Grant{}, err
		}
		if !ok || row.DeletedAt.Valid {
			return Grant{}, ErrInvalidCredentials
		}
		if row.AccountState != "active" {
			return Grant{}, ErrAccountDisabled
		}
		grant, err := a.finishLogin(ctx, row, replacement, upgraded)
		if !errors.Is(err, errCredentialChanged) {
			return grant, err
		}
		// A concurrent login upgraded the credential; verify its current value
		// outside the next transaction, rather than replacing a newer hash.
	}
	return Grant{}, ErrInvalidCredentials
}

func (a *App) finishLogin(ctx context.Context, row sqlc.FindLocalCredentialByEmailSubjectRow, replacement string, upgraded bool) (Grant, error) {
	sessionID, token, now := uuid.NewV7(), newToken(), a.now().UTC()
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return Grant{}, database.SafeError("begin login", err)
	}
	defer tx.Rollback(ctx)
	q := sqlc.New(tx)
	current, err := lockCredential(ctx, q, uuid.UUID(row.ID.Bytes))
	if err != nil {
		return Grant{}, err
	}
	if current != row.PasswordHash {
		return Grant{}, errCredentialChanged
	}
	now = a.now().UTC()
	if upgraded {
		n, err := q.CompareAndSwapPasswordHash(ctx, sqlc.CompareAndSwapPasswordHashParams{UserID: row.ID, OldHash: row.PasswordHash, NewHash: replacement, Now: timestamp(now)})
		if err != nil {
			return Grant{}, database.SafeError("upgrade password", err)
		}
		if n != 1 {
			return Grant{}, errCredentialChanged
		}
	}
	userID := uuid.UUID(row.ID.Bytes)
	if err = createSession(ctx, q, userID, sessionID, token, "password", now); err != nil {
		return Grant{}, err
	}
	if err = recordEvent(ctx, q, loginSucceeded, userID, sessionID, now); err != nil {
		return Grant{}, err
	}
	me, err := identity.ReadMe(ctx, q, userID)
	if err != nil {
		return Grant{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Grant{}, database.SafeError("commit login", err)
	}
	return Grant{Me: me, Token: token, ExpiresAt: now.Add(AbsoluteLifetime)}, nil
}
