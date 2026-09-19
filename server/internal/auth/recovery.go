package auth

import (
	"context"
	"errors"
	"time"
	"uuid"

	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/deepfurry/tap4furry/server/internal/identity"
	"github.com/jackc/pgx/v5"
)

func (a *App) ResetPassword(ctx context.Context, token, password string) (Grant, error) {
	initial, err := a.initialChallenge(ctx, token, purposeReset)
	if err != nil {
		return Grant{}, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return Grant{}, err
	}
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return Grant{}, database.SafeError("begin password reset", err)
	}
	defer tx.Rollback(ctx)
	q, userID := sqlc.New(tx), uuid.UUID(initial.UserID.Bytes)
	if _, err = lockCredential(ctx, q, userID); errors.Is(err, ErrUnauthenticated) {
		return Grant{}, ErrChallengeInvalid
	}
	if err != nil {
		return Grant{}, err
	}
	now := a.now().UTC()
	challenge, err := lockChallenge(ctx, q, token, purposeReset, now)
	if err != nil {
		return Grant{}, err
	}
	if err = q.SetPasswordHashForReset(ctx, sqlc.SetPasswordHashForResetParams{UserID: dbID(userID), NewHash: hash, Now: timestamp(now)}); err != nil {
		return Grant{}, database.SafeError("reset password credential", err)
	}
	if err = consumeChallenge(ctx, q, uuid.UUID(challenge.ID.Bytes), userID, purposeReset, now); err != nil {
		return Grant{}, err
	}
	grant, err := replaceSessions(ctx, q, userID, uuid.Nil(), "password_reset", resetCompleted, now)
	if err != nil {
		return Grant{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Grant{}, database.SafeError("commit password reset", err)
	}
	return grant, nil
}

func (a *App) ChangePassword(ctx context.Context, actor Actor, currentPassword, newPassword string) (Grant, error) {
	if err := ValidatePassword(newPassword); err != nil {
		return Grant{}, err
	}
	verified, err := a.verifyCurrent(ctx, actor, currentPassword)
	if err != nil {
		return Grant{}, err
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return Grant{}, err
	}
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return Grant{}, database.SafeError("begin password change", err)
	}
	defer tx.Rollback(ctx)
	q := sqlc.New(tx)
	current, err := lockCredential(ctx, q, actor.UserID)
	if err != nil {
		return Grant{}, err
	}
	now := a.now().UTC()
	if err = requireActor(ctx, q, actor, now); err != nil {
		return Grant{}, err
	}
	if current != verified {
		return Grant{}, ErrReauthFailed
	}
	count, err := q.ChangePasswordHash(ctx, sqlc.ChangePasswordHashParams{UserID: dbID(actor.UserID), OldHash: verified, NewHash: hash, Now: timestamp(now)})
	if err != nil {
		return Grant{}, database.SafeError("change password credential", err)
	}
	if count != 1 {
		return Grant{}, ErrReauthFailed
	}
	if err = invalidateChallenges(ctx, q, actor.UserID, purposeReset, now); err != nil {
		return Grant{}, err
	}
	grant, err := replaceSessions(ctx, q, actor.UserID, uuid.Nil(), "password", passwordChanged, now)
	if err != nil {
		return Grant{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Grant{}, database.SafeError("commit password change", err)
	}
	return grant, nil
}

func (a *App) Reauthenticate(ctx context.Context, actor Actor, password string) (Grant, error) {
	verified, err := a.verifyCurrent(ctx, actor, password)
	if err != nil {
		return Grant{}, err
	}
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return Grant{}, database.SafeError("begin reauthentication", err)
	}
	defer tx.Rollback(ctx)
	q := sqlc.New(tx)
	current, err := lockCredential(ctx, q, actor.UserID)
	if err != nil {
		return Grant{}, err
	}
	now := a.now().UTC()
	if err = requireActor(ctx, q, actor, now); err != nil {
		return Grant{}, err
	}
	if current != verified {
		return Grant{}, ErrReauthFailed
	}
	grant, err := replaceSessions(ctx, q, actor.UserID, actor.SessionID, "password", reauthenticated, now)
	if err != nil {
		return Grant{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Grant{}, database.SafeError("commit reauthentication", err)
	}
	return grant, nil
}

func (a *App) verifyCurrent(ctx context.Context, actor Actor, password string) (string, error) {
	if err := requireActor(ctx, sqlc.New(a.pool), actor, a.now().UTC()); err != nil {
		return "", err
	}
	subject := actor.UserID.String()
	healthy, err := a.throttleCheck(ctx, PublicReauthLimit, subject, false)
	if err != nil {
		return "", err
	}
	verified, err := a.verifyCurrentPassword(ctx, actor.UserID, password)
	if err == nil {
		a.throttleSuccess(ctx, PublicReauthLimit, subject)
	} else if healthy && errors.Is(err, ErrReauthFailed) {
		a.throttleFailure(ctx, PublicReauthLimit, subject, uuid.Nil(), loginFailed)
	}
	return verified, err
}

func (a *App) verifyCurrentPassword(ctx context.Context, userID uuid.UUID, password string) (string, error) {
	if ValidatePassword(password) != nil {
		return "", ErrReauthFailed
	}
	row, err := sqlc.New(a.pool).GetPasswordCredentialByUser(ctx, dbID(userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrReauthFailed
	}
	if err != nil {
		return "", database.SafeError("read current credential", err)
	}
	ok, _, _, err := verifyPassword(password, row.PasswordHash)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrReauthFailed
	}
	return row.PasswordHash, nil
}

// With the User auth lock held, nil superseded means reset/change (revoke all),
// otherwise reauthentication replaces exactly the current session. All writes
// and the event belong to the caller's transaction; only the caller may commit.
func replaceSessions(ctx context.Context, q *sqlc.Queries, userID, superseded uuid.UUID, method string, event eventType, now time.Time) (Grant, error) {
	var err error
	if superseded == uuid.Nil() {
		err = q.RevokeAllUserSessions(ctx, sqlc.RevokeAllUserSessionsParams{UserID: dbID(userID), Now: timestamp(now)})
	} else {
		err = q.RevokeSession(ctx, sqlc.RevokeSessionParams{ID: dbID(superseded), UserID: dbID(userID), Now: timestamp(now)})
	}
	if err != nil {
		return Grant{}, database.SafeError("revoke superseded sessions", err)
	}
	sessionID, token := uuid.NewV7(), newToken()
	if err = createSession(ctx, q, userID, sessionID, token, method, now); err != nil {
		return Grant{}, err
	}
	if err = recordEvent(ctx, q, event, userID, sessionID, now); err != nil {
		return Grant{}, err
	}
	me, err := identity.ReadMe(ctx, q, userID)
	if err != nil {
		return Grant{}, err
	}
	return Grant{Me: me, Token: token, ExpiresAt: now.Add(AbsoluteLifetime)}, nil
}
