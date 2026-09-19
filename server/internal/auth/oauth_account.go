package auth

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/deepfurry/tap4furry/server/internal/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var errOAuthConflict = errors.New("concurrent OAuth identity creation")

// Provider identity always wins over email. A uniqueness race is resolved from
// a new transaction, never by moving an identity or merging users.
func (a *App) oauthLogin(ctx context.Context, provider ProviderIdentity, started time.Time) (Grant, error) {
	for attempt := 0; attempt < 2; attempt++ {
		row, err := sqlc.New(a.pool).FindProviderIdentity(ctx, sqlc.FindProviderIdentityParams{Provider: string(provider.Provider), Subject: provider.Subject})
		if err == nil {
			return a.loginProviderUser(ctx, provider, row, started)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return Grant{}, database.SafeError("find OAuth identity", err)
		}
		grant, err := a.registerProviderUser(ctx, provider)
		if !errors.Is(err, errOAuthConflict) && !errors.Is(err, ErrAccountLinkRequired) {
			return grant, err
		}
	}
	return Grant{}, ErrAccountLinkRequired
}

func (a *App) loginProviderUser(ctx context.Context, provider ProviderIdentity, initial sqlc.FindProviderIdentityRow, started time.Time) (Grant, error) {
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return Grant{}, database.SafeError("begin OAuth login", err)
	}
	defer tx.Rollback(ctx)
	q, userID := sqlc.New(tx), uuid.UUID(initial.UserID.Bytes)
	if err = lockUser(ctx, q, userID); err != nil {
		return Grant{}, err
	}
	if err = checkOAuthCredentialEpoch(ctx, q, userID, started); err != nil {
		return Grant{}, err
	}
	current, err := q.FindProviderIdentityForUpdate(ctx, sqlc.FindProviderIdentityForUpdateParams{Provider: string(provider.Provider), Subject: provider.Subject})
	if errors.Is(err, pgx.ErrNoRows) {
		return Grant{}, ErrProviderNotLinked
	}
	if err != nil {
		return Grant{}, database.SafeError("recheck OAuth identity", err)
	}
	if current.ID != initial.ID || current.UserID != initial.UserID {
		return Grant{}, ErrProviderNotLinked
	}
	now := a.now().UTC()
	if err = updateProviderEmail(ctx, q, current.ID, dbID(userID), provider, now); err != nil {
		return Grant{}, err
	}
	grant, err := oauthGrant(ctx, q, userID, uuid.Nil(), string(provider.Provider), oauthLoginSucceeded, now, now)
	if err != nil {
		return Grant{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Grant{}, database.SafeError("commit OAuth login", err)
	}
	return grant, nil
}

// A reset/change that wins the User lock invalidates an older in-flight OAuth
// authorization. If OAuth wins first, reset/change revokes its inserted session.
func checkOAuthCredentialEpoch(ctx context.Context, q *sqlc.Queries, userID uuid.UUID, started time.Time) error {
	changed, err := q.GetPasswordChangeTime(ctx, dbID(userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return database.SafeError("check OAuth credential epoch", err)
	}
	if changed.Time.After(started) {
		return ErrReauthRequired
	}
	return nil
}

func (a *App) registerProviderUser(ctx context.Context, provider ProviderIdentity) (Grant, error) {
	email, err := NormalizeEmail(provider.Email)
	if err != nil || (provider.Provider == GitHub && !provider.EmailAuthoritative) {
		return Grant{}, ErrProviderInvalid
	}
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return Grant{}, database.SafeError("begin OAuth account", err)
	}
	defer tx.Rollback(ctx)
	q := sqlc.New(tx)
	if _, err = q.FindEmailIdentityBySubject(ctx, email); err == nil {
		return Grant{}, ErrAccountLinkRequired
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Grant{}, database.SafeError("check account email", err)
	}
	userID, now := uuid.NewV7(), a.now().UTC()
	if err = q.CreateUser(ctx, sqlc.CreateUserParams{ID: dbID(userID), CreatedAt: timestamp(now)}); err != nil {
		return Grant{}, database.SafeError("create OAuth user", err)
	}
	if err = q.CreateProfile(ctx, sqlc.CreateProfileParams{UserID: dbID(userID), CreatedAt: timestamp(now)}); err != nil {
		return Grant{}, database.SafeError("create OAuth profile", err)
	}
	if name := provider.DisplayName; name != "" && utf8.ValidString(name) && !strings.ContainsRune(name, 0) && utf8.RuneCountInString(name) <= 80 {
		if err = q.InitializeOAuthProfile(ctx, sqlc.InitializeOAuthProfileParams{UserID: dbID(userID), DisplayName: pgtype.Text{String: name, Valid: true}}); err != nil {
			return Grant{}, database.SafeError("initialize OAuth profile", err)
		}
	}
	verified := pgtype.Timestamptz{}
	if provider.EmailVerified && provider.EmailAuthoritative {
		verified = timestamp(now)
	}
	err = q.CreateEmailIdentityForOAuthAccount(ctx, sqlc.CreateEmailIdentityForOAuthAccountParams{ID: dbID(uuid.NewV7()), UserID: dbID(userID), Email: email, VerifiedAt: verified, Now: timestamp(now)})
	if err != nil {
		return Grant{}, oauthWriteError("create OAuth account email", err)
	}
	if err = insertProvider(ctx, q, userID, provider, now); err != nil {
		return Grant{}, err
	}
	grant, err := oauthGrant(ctx, q, userID, uuid.Nil(), string(provider.Provider), oauthLoginSucceeded, now, now)
	if err != nil {
		return Grant{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Grant{}, database.SafeError("commit OAuth account", err)
	}
	return grant, nil
}

func oauthWriteError(operation string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return errOAuthConflict
	}
	return database.SafeError(operation, err)
}
func providerEmail(provider ProviderIdentity) pgtype.Text {
	email, err := NormalizeEmail(provider.Email)
	return pgtype.Text{String: email, Valid: err == nil}
}
func insertProvider(ctx context.Context, q *sqlc.Queries, userID uuid.UUID, provider ProviderIdentity, now time.Time) error {
	verified := pgtype.Timestamptz{}
	if provider.EmailVerified {
		verified = timestamp(now)
	}
	err := q.CreateProviderIdentity(ctx, sqlc.CreateProviderIdentityParams{ID: dbID(uuid.NewV7()), UserID: dbID(userID), Provider: string(provider.Provider), ProviderSubject: provider.Subject, Email: providerEmail(provider), VerifiedAt: verified, CreatedAt: timestamp(now)})
	if err != nil {
		return oauthWriteError("create provider identity", err)
	}
	return nil
}
func updateProviderEmail(ctx context.Context, q *sqlc.Queries, id, userID pgtype.UUID, provider ProviderIdentity, now time.Time) error {
	err := q.UpdateProviderIdentityMetadata(ctx, sqlc.UpdateProviderIdentityMetadataParams{ID: id, UserID: userID, Email: providerEmail(provider), Now: timestamp(now)})
	if err != nil {
		return database.SafeError("update private provider metadata", err)
	}
	return nil
}

// Caller owns the User lock and transaction. Mutation rotations preserve the
// previous authentication time; only a verified login/reauth refreshes freshness.
func oauthGrant(ctx context.Context, q *sqlc.Queries, userID, oldSession uuid.UUID, method string, event eventType, now, authenticatedAt time.Time) (Grant, error) {
	if oldSession != uuid.Nil() {
		if err := q.RevokeSession(ctx, sqlc.RevokeSessionParams{ID: dbID(oldSession), UserID: dbID(userID), Now: timestamp(now)}); err != nil {
			return Grant{}, database.SafeError("rotate OAuth session", err)
		}
	}
	sessionID, token := uuid.NewV7(), newToken()
	if err := createSessionAuthenticatedAt(ctx, q, userID, sessionID, token, method, now, authenticatedAt); err != nil {
		return Grant{}, err
	}
	if err := recordEvent(ctx, q, event, userID, sessionID, now); err != nil {
		return Grant{}, err
	}
	me, err := identity.ReadMe(ctx, q, userID)
	if err != nil {
		return Grant{}, err
	}
	return Grant{Me: me, Token: token, ExpiresAt: now.Add(AbsoluteLifetime)}, nil
}
