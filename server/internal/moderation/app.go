// Package moderation owns report and governance application transactions.
package moderation

import (
	"context"
	"errors"
	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/curation"
	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/deepfurry/tap4furry/server/internal/governance"
	"github.com/deepfurry/tap4furry/server/internal/identity"
	"github.com/deepfurry/tap4furry/server/internal/resource"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

var (
	ErrValidation      = governance.ErrValidation
	ErrNotFound        = errors.New("governance object not found")
	ErrConflict        = errors.New("governance revision conflict")
	ErrRequestConflict = errors.New("request key reused")
	ErrForbidden       = errors.New("governance action forbidden")
	ErrVerified        = errors.New("verified email required")
	ErrCanonical       = errors.New("canonical localization missing")
)

type LimitError struct {
	Reason     string
	RetryAfter int
}

func (*LimitError) Error() string { return "report quota exceeded" }

type App struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *App { return &App{pool: pool} }
func safe(err error) error {
	if err == nil {
		return nil
	}
	var restriction *governance.RestrictedError
	var limit *LimitError
	if errors.As(err, &restriction) || errors.As(err, &limit) {
		return err
	}
	for _, e := range []error{ErrValidation, ErrNotFound, ErrConflict, ErrRequestConflict, ErrForbidden, ErrVerified, ErrCanonical, auth.ErrUnauthenticated, auth.ErrAdminUnauthenticated, auth.ErrAdminForbidden, identity.ErrValidation, identity.ErrUnavailable, identity.ErrHandleUnavailable, resource.ErrVersionConflict, curation.ErrValidation, curation.ErrNotFound, curation.ErrConflict} {
		if errors.Is(err, e) {
			return err
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var p *pgconn.PgError
	if errors.As(err, &p) {
		switch p.Code {
		case "23505", "23503":
			return ErrConflict
		case "23514", "22001", "22003":
			return ErrValidation
		}
	}
	return database.SafeError("governance operation", err)
}

type check func(pgx.Tx) (time.Time, error)

func publicCheck(ctx context.Context, actor auth.Actor) check {
	return func(tx pgx.Tx) (time.Time, error) { return auth.RequirePublicActorTx(ctx, tx, actor) }
}
func adminCheck(ctx context.Context, actor auth.AdminActor, cap auth.Capability) check {
	return func(tx pgx.Tx) (time.Time, error) {
		_, err := auth.RequireAdminCapabilityTx(ctx, tx, actor, cap)
		if err != nil {
			return time.Time{}, err
		}
		var now time.Time
		err = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now)
		return now, err
	}
}
func (a *App) transact(ctx context.Context, verify check, work func(pgx.Tx, *sqlc.Queries, time.Time) error) error {
	tx, err := a.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return safe(err)
	}
	defer tx.Rollback(ctx)
	now, err := verify(tx)
	if err != nil {
		return safe(err)
	}
	if err = work(tx, sqlc.New(tx), now); err != nil {
		return safe(err)
	}
	return safe(tx.Commit(ctx))
}

// Snapshot reads a consistent private projection, then rechecks authorization in
// a fresh transaction. Connections are sequential, including pools of size one.
func (a *App) snapshot(ctx context.Context, verify check, work func(*sqlc.Queries) error) error {
	tx, err := a.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return safe(err)
	}
	defer tx.Rollback(ctx)
	if _, err = verify(tx); err != nil {
		return safe(err)
	}
	if err = work(sqlc.New(tx)); err != nil {
		return safe(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return safe(err)
	}
	return a.transact(ctx, verify, func(pgx.Tx, *sqlc.Queries, time.Time) error { return nil })
}
func (a *App) UpdateProfile(ctx context.Context, actor auth.Actor, input identity.ProfileUpdate) (identity.Me, error) {
	var out identity.Me
	if err := input.Validate(); err != nil {
		return out, err
	}
	err := a.transact(ctx, publicCheck(ctx, actor), func(tx pgx.Tx, q *sqlc.Queries, now time.Time) error {
		if input.Handle.Set || input.DisplayName.Set || input.Bio.Set || (input.SearchEngineIndexing != nil && *input.SearchEngineIndexing) {
			if err := governance.CheckTx(ctx, q, actor.UserID, governance.PublicProfileWrite, now); err != nil {
				return err
			}
		}
		var err error
		out, err = identity.ApplyProfileTx(ctx, tx, actor.UserID, input, now)
		return err
	})
	return out, err
}
