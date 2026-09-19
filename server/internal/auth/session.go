package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"
	"uuid"

	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	AbsoluteLifetime = 30 * 24 * time.Hour
	IdleLifetime     = 14 * 24 * time.Hour
	TouchInterval    = 10 * time.Minute
)

type Actor struct {
	UserID, SessionID uuid.UUID
	SessionKind       string
	AuthMethod        string
	AuthenticatedAt   time.Time
}

func newToken() string {
	var bytes [32]byte
	rand.Read(bytes[:])
	return base64.RawURLEncoding.EncodeToString(bytes[:])
}

func tokenHash(token string) ([32]byte, error) {
	if len(token) != 43 {
		return [32]byte{}, ErrUnauthenticated
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil || len(raw) != 32 {
		return [32]byte{}, ErrUnauthenticated
	}
	return sha256.Sum256([]byte(token)), nil
}

func (a *App) Resolve(ctx context.Context, token string) (Actor, error) {
	hash, err := tokenHash(token)
	if err != nil {
		return Actor{}, err
	}
	now := a.now().UTC()
	q := sqlc.New(a.pool)
	params := sqlc.FindActiveSessionByTokenHashParams{TokenHash: hash[:], Now: timestamp(now)}
	session, err := q.FindActiveSessionByTokenHash(ctx, params)
	if errors.Is(err, pgx.ErrNoRows) {
		return Actor{}, ErrUnauthenticated
	}
	if err != nil {
		return Actor{}, database.SafeError("resolve session", err)
	}
	if !session.LastSeenAt.Time.After(now.Add(-TouchInterval)) {
		_, err = q.TouchSession(ctx, sqlc.TouchSessionParams{ID: session.ID, Now: timestamp(now), IdleExpiresAt: timestamp(now.Add(IdleLifetime)), TouchBefore: timestamp(now.Add(-TouchInterval))})
		if err != nil {
			return Actor{}, database.SafeError("touch session", err)
		}
		// A zero-row touch can mean either a concurrent touch or revocation.
		// Recheck the active predicate before admitting the request.
		if _, err = q.FindActiveSessionByTokenHash(ctx, params); errors.Is(err, pgx.ErrNoRows) {
			return Actor{}, ErrUnauthenticated
		}
		if err != nil {
			return Actor{}, database.SafeError("recheck session", err)
		}
	}
	return Actor{UserID: uuid.UUID(session.UserID.Bytes), SessionID: uuid.UUID(session.ID.Bytes), SessionKind: session.Kind, AuthMethod: session.AuthMethod, AuthenticatedAt: session.AuthenticatedAt.Time}, nil
}

func (a *App) Logout(ctx context.Context, actor Actor) error {
	return a.revoke(ctx, actor, actor.SessionID, loggedOut)
}

func createSession(ctx context.Context, q *sqlc.Queries, userID, sessionID uuid.UUID, token, authMethod string, now time.Time) error {
	return createSessionAuthenticatedAt(ctx, q, userID, sessionID, token, authMethod, now, now)
}

func createSessionAuthenticatedAt(ctx context.Context, q *sqlc.Queries, userID, sessionID uuid.UUID, token, authMethod string, now, authenticatedAt time.Time) error {
	hash, _ := tokenHash(token)
	n, err := q.CreateSession(ctx, sqlc.CreateSessionParams{ID: dbID(sessionID), UserID: dbID(userID), TokenHash: hash[:], Now: timestamp(now), AuthMethod: authMethod,
		AuthenticatedAt: timestamp(authenticatedAt), IdleExpiresAt: timestamp(now.Add(IdleLifetime)), AbsoluteExpiresAt: timestamp(now.Add(AbsoluteLifetime))})
	if err != nil {
		return database.SafeError("create session", err)
	}
	if n != 1 {
		return ErrInvalidCredentials
	}
	return nil
}
func timestamp(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
func dbID(id uuid.UUID) pgtype.UUID            { return pgtype.UUID{Bytes: id, Valid: true} }
