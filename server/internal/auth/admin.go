package auth

import (
	"context"
	"errors"
	"time"
	"uuid"

	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/jackc/pgx/v5"
)

const (
	AdminAbsoluteLifetime = 8 * time.Hour
	AdminIdleLifetime     = time.Hour
	AdminTouchInterval    = 5 * time.Minute
)

var (
	ErrAdminCredentials     = errors.New("invalid admin credentials")
	ErrAdminUnauthenticated = errors.New("admin authentication required")
	ErrAdminForbidden       = errors.New("admin access forbidden")
)

type AdminActor struct {
	UserID, SessionID uuid.UUID
	AuthenticatedAt   time.Time
	Roles             []Role
}
type AdminMe struct {
	ID              uuid.UUID
	Email           string
	Roles           []Role
	AuthenticatedAt time.Time
}
type AdminGrant struct {
	Me        AdminMe
	Token     string `json:"-"`
	ExpiresAt time.Time
}

// RequireAdminCapabilityTx shares the User-first lock order with authentication
// and RoleOperator. Callers must keep this transaction open through their write;
// roles carried in a previously resolved actor are deliberately not trusted.
func RequireAdminCapabilityTx(ctx context.Context, tx pgx.Tx, actor AdminActor, capability Capability) ([]Role, error) {
	q := sqlc.New(tx)
	if err := lockUser(ctx, q, actor.UserID); err != nil {
		return nil, adminActorError(err)
	}
	var now time.Time
	if err := tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); err != nil {
		return nil, database.SafeError("read admin authorization time", err)
	}
	me, err := requireAdminActor(ctx, q, actor, now)
	if err != nil {
		return nil, err
	}
	if !HasCapability(me.Roles, capability) {
		return nil, ErrAdminForbidden
	}
	return me.Roles, nil
}

func readAdminMe(ctx context.Context, q *sqlc.Queries, userID uuid.UUID, authenticatedAt time.Time) (AdminMe, error) {
	account, err := q.ReadAdminAccount(ctx, dbID(userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return AdminMe{}, ErrAdminUnauthenticated
	}
	if err != nil {
		return AdminMe{}, database.SafeError("read admin account", err)
	}
	roles, err := readRoles(ctx, q, userID)
	if err != nil {
		return AdminMe{}, err
	}
	if !HasCapability(roles, AdminAccess) {
		return AdminMe{}, ErrAdminForbidden
	}
	return AdminMe{ID: userID, Email: account.Email.String, Roles: roles, AuthenticatedAt: authenticatedAt}, nil
}
func createAdminSession(ctx context.Context, q *sqlc.Queries, userID, sessionID uuid.UUID, token string, now time.Time) error {
	hash, _ := tokenHash(token)
	err := q.CreateAdminSession(ctx, sqlc.CreateAdminSessionParams{ID: dbID(sessionID), UserID: dbID(userID), TokenHash: hash[:], Now: timestamp(now), IdleExpiresAt: timestamp(now.Add(AdminIdleLifetime)), AbsoluteExpiresAt: timestamp(now.Add(AdminAbsoluteLifetime))})
	if err != nil {
		return database.SafeError("create admin session", err)
	}
	return nil
}
func (a *App) AdminLogin(ctx context.Context, email, password string) (AdminGrant, error) {
	email, err := NormalizeEmail(email)
	if err != nil {
		return AdminGrant{}, ErrAdminCredentials
	}
	healthy, err := a.throttleCheck(ctx, AdminLoginLimit, email, false)
	if err != nil {
		return AdminGrant{}, err
	}
	var known uuid.UUID
	fail := func() (AdminGrant, error) {
		if healthy {
			a.throttleFailure(ctx, AdminLoginLimit, email, known, adminLoginFailed)
		}
		return AdminGrant{}, ErrAdminCredentials
	}
	if ValidatePassword(password) != nil {
		return fail()
	}
	for attempt := 0; attempt < 2; attempt++ {
		row, err := sqlc.New(a.pool).FindLocalCredentialByEmailSubject(ctx, email)
		if errors.Is(err, pgx.ErrNoRows) {
			_, _, _, err = verifyPassword(password, a.dummyHash)
			if err != nil {
				return AdminGrant{}, err
			}
			return fail()
		}
		if err != nil {
			return AdminGrant{}, database.SafeError("find admin credential", err)
		}
		known = uuid.UUID(row.ID.Bytes)
		ok, replacement, upgraded, err := verifyPassword(password, row.PasswordHash)
		if err != nil {
			return AdminGrant{}, err
		}
		if !ok || row.AccountState != "active" || row.DeletedAt.Valid {
			return fail()
		}
		grant, err := a.finishAdminLogin(ctx, row, replacement, upgraded)
		if errors.Is(err, errCredentialChanged) {
			continue
		}
		if errors.Is(err, ErrUnauthenticated) || errors.Is(err, ErrAdminUnauthenticated) || errors.Is(err, ErrAdminForbidden) {
			return fail()
		}
		if err == nil {
			a.throttleSuccess(ctx, AdminLoginLimit, email)
		}
		return grant, err
	}
	return fail()
}
func (a *App) finishAdminLogin(ctx context.Context, row sqlc.FindLocalCredentialByEmailSubjectRow, replacement string, upgraded bool) (AdminGrant, error) {
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return AdminGrant{}, database.SafeError("begin admin login", err)
	}
	defer tx.Rollback(ctx)
	q, userID := sqlc.New(tx), uuid.UUID(row.ID.Bytes)
	current, err := lockCredential(ctx, q, userID)
	if err != nil {
		return AdminGrant{}, err
	}
	if current != row.PasswordHash {
		return AdminGrant{}, errCredentialChanged
	}
	now := a.now().UTC()
	me, err := readAdminMe(ctx, q, userID, now)
	if err != nil {
		return AdminGrant{}, err
	}
	if upgraded {
		n, err := q.CompareAndSwapPasswordHash(ctx, sqlc.CompareAndSwapPasswordHashParams{UserID: row.ID, OldHash: row.PasswordHash, NewHash: replacement, Now: timestamp(now)})
		if err != nil {
			return AdminGrant{}, database.SafeError("upgrade admin credential", err)
		}
		if n != 1 {
			return AdminGrant{}, errCredentialChanged
		}
	}
	sessionID, token := uuid.NewV7(), newToken()
	if err = createAdminSession(ctx, q, userID, sessionID, token, now); err != nil {
		return AdminGrant{}, err
	}
	if err = recordEvent(ctx, q, adminLoginSucceeded, userID, sessionID, now); err != nil {
		return AdminGrant{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return AdminGrant{}, database.SafeError("commit admin login", err)
	}
	return AdminGrant{Me: me, Token: token, ExpiresAt: now.Add(AdminAbsoluteLifetime)}, nil
}
func (a *App) ResolveAdmin(ctx context.Context, token string) (AdminActor, error) {
	hash, err := tokenHash(token)
	if err != nil {
		return AdminActor{}, ErrAdminUnauthenticated
	}
	q, now := sqlc.New(a.pool), a.now().UTC()
	params := sqlc.FindActiveAdminSessionParams{TokenHash: hash[:], Now: timestamp(now)}
	session, err := q.FindActiveAdminSession(ctx, params)
	if errors.Is(err, pgx.ErrNoRows) {
		return AdminActor{}, ErrAdminUnauthenticated
	}
	if err != nil {
		return AdminActor{}, database.SafeError("resolve admin session", err)
	}
	if !session.LastSeenAt.Time.After(now.Add(-AdminTouchInterval)) {
		_, err = q.TouchAdminSession(ctx, sqlc.TouchAdminSessionParams{ID: session.ID, Now: timestamp(now), IdleExpiresAt: timestamp(now.Add(AdminIdleLifetime)), TouchBefore: timestamp(now.Add(-AdminTouchInterval))})
		if err != nil {
			return AdminActor{}, database.SafeError("touch admin session", err)
		}
		if _, err = q.FindActiveAdminSession(ctx, params); errors.Is(err, pgx.ErrNoRows) {
			return AdminActor{}, ErrAdminUnauthenticated
		}
		if err != nil {
			return AdminActor{}, database.SafeError("recheck admin session", err)
		}
	}
	me, err := readAdminMe(ctx, q, uuid.UUID(session.UserID.Bytes), session.AuthenticatedAt.Time)
	if err != nil {
		return AdminActor{}, err
	}
	return AdminActor{UserID: me.ID, SessionID: uuid.UUID(session.ID.Bytes), AuthenticatedAt: me.AuthenticatedAt, Roles: me.Roles}, nil
}
func requireAdminActor(ctx context.Context, q *sqlc.Queries, actor AdminActor, now time.Time) (AdminMe, error) {
	session, err := q.GetActiveAdminSessionByID(ctx, sqlc.GetActiveAdminSessionByIDParams{ID: dbID(actor.SessionID), UserID: dbID(actor.UserID), Now: timestamp(now)})
	if errors.Is(err, pgx.ErrNoRows) {
		return AdminMe{}, ErrAdminUnauthenticated
	}
	if err != nil {
		return AdminMe{}, database.SafeError("revalidate admin session", err)
	}
	return readAdminMe(ctx, q, actor.UserID, session.AuthenticatedAt.Time)
}
func (a *App) AdminMe(ctx context.Context, actor AdminActor) (AdminMe, error) {
	return requireAdminActor(ctx, sqlc.New(a.pool), actor, a.now().UTC())
}
func (a *App) AdminSessions(ctx context.Context, actor AdminActor) ([]SessionView, error) {
	q, now := sqlc.New(a.pool), a.now().UTC()
	if _, err := requireAdminActor(ctx, q, actor, now); err != nil {
		return nil, err
	}
	rows, err := q.ListActiveAdminSessions(ctx, sqlc.ListActiveAdminSessionsParams{UserID: dbID(actor.UserID), Now: timestamp(now)})
	if err != nil {
		return nil, database.SafeError("list admin sessions", err)
	}
	result := make([]SessionView, 0, len(rows))
	for _, row := range rows {
		id := uuid.UUID(row.ID.Bytes)
		result = append(result, SessionView{ID: id, AuthMethod: "password", AuthenticatedAt: row.AuthenticatedAt.Time, CreatedAt: row.CreatedAt.Time, LastSeenAt: row.LastSeenAt.Time, IdleExpiresAt: row.IdleExpiresAt.Time, AbsoluteExpiresAt: row.AbsoluteExpiresAt.Time, Current: id == actor.SessionID})
	}
	return result, nil
}
func (a *App) AdminLogout(ctx context.Context, actor AdminActor) error {
	return a.adminRevoke(ctx, actor, actor.SessionID, adminLoggedOut)
}
func (a *App) AdminRevokeSession(ctx context.Context, actor AdminActor, target uuid.UUID) error {
	return a.adminRevoke(ctx, actor, target, adminSessionRevoked)
}
func (a *App) AdminRevokeOthers(ctx context.Context, actor AdminActor) error {
	return a.adminRevoke(ctx, actor, actor.SessionID, adminOtherSessionsRevoked)
}
func (a *App) adminRevoke(ctx context.Context, actor AdminActor, target uuid.UUID, event eventType) error {
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return database.SafeError("begin admin revocation", err)
	}
	defer tx.Rollback(ctx)
	q := sqlc.New(tx)
	if err = lockUser(ctx, q, actor.UserID); err != nil {
		return adminActorError(err)
	}
	now := a.now().UTC()
	if _, err = requireAdminActor(ctx, q, actor, now); err != nil {
		return err
	}
	if event == adminOtherSessionsRevoked {
		err = q.RevokeOtherAdminSessions(ctx, sqlc.RevokeOtherAdminSessionsParams{UserID: dbID(actor.UserID), CurrentID: dbID(actor.SessionID), Now: timestamp(now)})
	} else {
		var n int64
		n, err = q.RevokeAdminSessionByID(ctx, sqlc.RevokeAdminSessionByIDParams{ID: dbID(target), UserID: dbID(actor.UserID), Now: timestamp(now)})
		if err == nil && n != 1 {
			return ErrSessionNotFound
		}
	}
	if err != nil {
		return database.SafeError("revoke admin sessions", err)
	}
	if err = recordEvent(ctx, q, event, actor.UserID, target, now); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return database.SafeError("commit admin revocation", err)
	}
	return nil
}
func adminActorError(err error) error {
	if errors.Is(err, ErrUnauthenticated) {
		return ErrAdminUnauthenticated
	}
	return err
}
func (a *App) AdminReauthenticate(ctx context.Context, actor AdminActor, password string) (AdminGrant, error) {
	if _, err := a.AdminMe(ctx, actor); err != nil {
		return AdminGrant{}, err
	}
	subject := actor.UserID.String()
	healthy, err := a.throttleCheck(ctx, AdminReauthLimit, subject, false)
	if err != nil {
		return AdminGrant{}, err
	}
	verified, err := a.verifyCurrentPassword(ctx, actor.UserID, password)
	if err != nil {
		if healthy && errors.Is(err, ErrReauthFailed) {
			a.throttleFailure(ctx, AdminReauthLimit, subject, uuid.Nil(), adminLoginFailed)
		}
		if errors.Is(err, ErrReauthFailed) {
			return AdminGrant{}, ErrAdminCredentials
		}
		return AdminGrant{}, err
	}
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return AdminGrant{}, database.SafeError("begin admin reauthentication", err)
	}
	defer tx.Rollback(ctx)
	q := sqlc.New(tx)
	current, err := lockCredential(ctx, q, actor.UserID)
	if err != nil {
		return AdminGrant{}, adminActorError(err)
	}
	now := a.now().UTC()
	me, err := requireAdminActor(ctx, q, actor, now)
	if err != nil {
		return AdminGrant{}, err
	}
	if current != verified {
		return AdminGrant{}, ErrAdminCredentials
	}
	if _, err = q.RevokeAdminSessionByID(ctx, sqlc.RevokeAdminSessionByIDParams{ID: dbID(actor.SessionID), UserID: dbID(actor.UserID), Now: timestamp(now)}); err != nil {
		return AdminGrant{}, database.SafeError("rotate admin session", err)
	}
	sessionID, token := uuid.NewV7(), newToken()
	if err = createAdminSession(ctx, q, actor.UserID, sessionID, token, now); err != nil {
		return AdminGrant{}, err
	}
	if err = recordEvent(ctx, q, adminReauthenticated, actor.UserID, sessionID, now); err != nil {
		return AdminGrant{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return AdminGrant{}, database.SafeError("commit admin reauthentication", err)
	}
	a.throttleSuccess(ctx, AdminReauthLimit, subject)
	me.AuthenticatedAt = now
	return AdminGrant{Me: me, Token: token, ExpiresAt: now.Add(AdminAbsoluteLifetime)}, nil
}
