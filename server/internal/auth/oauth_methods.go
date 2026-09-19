package auth

import (
	"context"
	"errors"
	"time"

	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/jackc/pgx/v5"
)

type ProviderMethod struct {
	Provider Provider
	Email    *string
	LinkedAt time.Time
}
type AuthMethods struct {
	Password  bool
	Providers []ProviderMethod
}

func (a *App) Methods(ctx context.Context, actor Actor) (AuthMethods, error) {
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return AuthMethods{}, database.SafeError("begin authentication methods", err)
	}
	defer tx.Rollback(ctx)
	q := sqlc.New(tx)
	if err = lockUser(ctx, q, actor.UserID); err != nil {
		return AuthMethods{}, err
	}
	if _, err = currentActor(ctx, q, actor, a.now().UTC(), false); err != nil {
		return AuthMethods{}, err
	}
	password, err := q.HasPasswordCredential(ctx, dbID(actor.UserID))
	if err != nil {
		return AuthMethods{}, database.SafeError("read password availability", err)
	}
	rows, err := q.ListProviderIdentitiesForUser(ctx, dbID(actor.UserID))
	if err != nil {
		return AuthMethods{}, database.SafeError("read authentication methods", err)
	}
	result := AuthMethods{Password: password, Providers: make([]ProviderMethod, 0, len(rows))}
	for _, row := range rows {
		method := ProviderMethod{Provider: Provider(row.Provider), LinkedAt: row.CreatedAt.Time}
		if row.Email.Valid {
			method.Email = &row.Email.String
		}
		result.Providers = append(result.Providers, method)
	}
	if err = tx.Commit(ctx); err != nil {
		return AuthMethods{}, database.SafeError("commit authentication methods", err)
	}
	return result, nil
}

func (a *App) oauthMutation(ctx context.Context, provider ProviderIdentity, flow OAuthFlow, actor Actor) (Grant, error) {
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return Grant{}, database.SafeError("begin OAuth mutation", err)
	}
	defer tx.Rollback(ctx)
	q := sqlc.New(tx)
	if err = lockUser(ctx, q, actor.UserID); err != nil {
		return Grant{}, err
	}
	now := a.now().UTC()
	current, err := currentActor(ctx, q, actor, now, flow.Mode == OAuthLink)
	if err != nil {
		return Grant{}, err
	}
	rows, err := q.ListProviderIdentitiesForUser(ctx, dbID(actor.UserID))
	if err != nil {
		return Grant{}, database.SafeError("read current providers", err)
	}
	var linked *sqlc.ListProviderIdentitiesForUserRow
	for i := range rows {
		if rows[i].Provider == string(provider.Provider) {
			linked = &rows[i]
		}
	}
	method, event, authenticatedAt := current.AuthMethod, oauthIdentityLinked, current.AuthenticatedAt
	if flow.Mode == OAuthReauth {
		if linked == nil || linked.ProviderSubject != provider.Subject || linked.CreatedAt.Time.After(flow.CreatedAt) {
			return Grant{}, ErrReauthFailed
		}
		method, event, authenticatedAt = string(provider.Provider), oauthReauthenticated, now
	} else {
		if linked != nil && linked.ProviderSubject != provider.Subject {
			return Grant{}, ErrProviderAlreadyLinked
		}
		owner, lookupErr := q.FindProviderIdentity(ctx, sqlc.FindProviderIdentityParams{Provider: string(provider.Provider), Subject: provider.Subject})
		if lookupErr == nil && owner.UserID != dbID(actor.UserID) {
			return Grant{}, ErrProviderAlreadyLinked
		}
		if lookupErr != nil && !errors.Is(lookupErr, pgx.ErrNoRows) {
			return Grant{}, database.SafeError("check provider ownership", lookupErr)
		}
		if linked == nil {
			if err = insertProvider(ctx, q, actor.UserID, provider, now); errors.Is(err, errOAuthConflict) {
				return Grant{}, ErrProviderAlreadyLinked
			}
			if err != nil {
				return Grant{}, err
			}
		}
	}
	grant, err := oauthGrant(ctx, q, actor.UserID, actor.SessionID, method, event, now, authenticatedAt)
	if err != nil {
		return Grant{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Grant{}, database.SafeError("commit OAuth mutation", err)
	}
	return grant, nil
}

func (a *App) UnlinkProvider(ctx context.Context, actor Actor, provider Provider) (Grant, error) {
	if !provider.Valid() {
		return Grant{}, ErrProviderInvalid
	}
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return Grant{}, database.SafeError("begin provider unlink", err)
	}
	defer tx.Rollback(ctx)
	q := sqlc.New(tx)
	if err = lockUser(ctx, q, actor.UserID); err != nil {
		return Grant{}, err
	}
	now := a.now().UTC()
	current, err := currentActor(ctx, q, actor, now, true)
	if err != nil {
		return Grant{}, err
	}
	rows, err := q.ListProviderIdentitiesForUser(ctx, dbID(actor.UserID))
	if err != nil {
		return Grant{}, database.SafeError("read remaining providers", err)
	}
	linked := false
	for _, row := range rows {
		if row.Provider == string(provider) {
			linked = true
		}
	}
	if !linked {
		return Grant{}, ErrProviderNotLinked
	}
	password, err := q.HasPasswordCredential(ctx, dbID(actor.UserID))
	if err != nil {
		return Grant{}, database.SafeError("read remaining password method", err)
	}
	count := len(rows)
	if password {
		count++
	}
	if count <= 1 {
		return Grant{}, ErrLastMethod
	}
	if current.AuthMethod == string(provider) {
		return Grant{}, ErrReauthRequired
	}
	deleted, err := q.DeleteProviderIdentity(ctx, sqlc.DeleteProviderIdentityParams{UserID: dbID(actor.UserID), Provider: string(provider)})
	if err != nil {
		return Grant{}, database.SafeError("unlink provider", err)
	}
	if deleted != 1 {
		return Grant{}, ErrProviderNotLinked
	}
	if err = q.RevokeSessionsByAuthMethod(ctx, sqlc.RevokeSessionsByAuthMethodParams{UserID: dbID(actor.UserID), AuthMethod: string(provider), Now: timestamp(now)}); err != nil {
		return Grant{}, database.SafeError("revoke provider sessions", err)
	}
	grant, err := oauthGrant(ctx, q, actor.UserID, actor.SessionID, current.AuthMethod, oauthIdentityUnlinked, now, current.AuthenticatedAt)
	if err != nil {
		return Grant{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Grant{}, database.SafeError("commit provider unlink", err)
	}
	return grant, nil
}
