// Package curation owns canonical Resource graph mutation transactions.
package curation

import (
	"context"
	"errors"
	"uuid"

	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/deepfurry/tap4furry/server/internal/resource"
	"github.com/deepfurry/tap4furry/server/internal/taxonomy"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrValidation    = errors.New("invalid curation input")
	ErrNotFound      = errors.New("curation entity not found")
	ErrConflict      = errors.New("canonical identity conflict")
	ErrInUse         = errors.New("taxonomy is in use")
	ErrRelationCycle = errors.New("relation would create a cycle")
)

type App struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *App { return &App{pool: pool} }

type Revision struct {
	ID      uuid.UUID
	Version int64
}

func dbID(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: true} }
func dbText(v *string) pgtype.Text {
	if v == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *v, Valid: true}
}
func textPointer(v pgtype.Text) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}
func safe(err error) error {
	if err == nil {
		return nil
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
		return database.SafeError("curation transaction", err)
	}
	if errors.Is(err, resource.ErrValidation) || errors.Is(err, resource.ErrPublishedSlug) || errors.Is(err, taxonomy.ErrValidation) {
		return ErrValidation
	}
	for _, known := range []error{ErrValidation, ErrNotFound, ErrConflict, ErrInUse, ErrRelationCycle, resource.ErrVersionConflict, auth.ErrAdminForbidden, auth.ErrAdminUnauthenticated} {
		if errors.Is(err, known) {
			return known
		}
	}
	return database.SafeError("curation operation", err)
}
func (a *App) transact(ctx context.Context, actor auth.AdminActor, cap auth.Capability, work func(*sqlc.Queries, []auth.Role) error) error {
	tx, err := a.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return database.SafeError("begin curation", err)
	}
	defer tx.Rollback(ctx)
	roles, err := auth.RequireAdminCapabilityTx(ctx, tx, actor, cap)
	if err != nil {
		return err
	}
	if err = work(sqlc.New(tx), roles); err != nil {
		return safe(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return database.SafeError("commit curation", err)
	}
	return nil
}
func lockedResource(ctx context.Context, q *sqlc.Queries, id uuid.UUID, expected int64) (sqlc.LockResourceCoreRow, error) {
	if id == uuid.Nil() || expected < 1 {
		return sqlc.LockResourceCoreRow{}, ErrValidation
	}
	row, err := q.LockResourceCore(ctx, dbID(id))
	if err != nil {
		return row, safe(err)
	}
	if row.Version != expected {
		return row, resource.ErrVersionConflict
	}
	return row, nil
}
func bump(ctx context.Context, q *sqlc.Queries, id uuid.UUID, expected int64) (Revision, error) {
	next, err := q.CurationBumpRevision(ctx, sqlc.CurationBumpRevisionParams{ID: dbID(id), Version: expected})
	if errors.Is(err, pgx.ErrNoRows) {
		return Revision{}, resource.ErrVersionConflict
	}
	return Revision{id, next}, err
}
func (a *App) mutate(ctx context.Context, actor auth.AdminActor, id uuid.UUID, expected int64, cap auth.Capability, work func(*sqlc.Queries, sqlc.LockResourceCoreRow, []auth.Role) (bool, error)) (Revision, error) {
	result := Revision{id, expected}
	err := a.transact(ctx, actor, cap, func(q *sqlc.Queries, roles []auth.Role) error {
		row, err := lockedResource(ctx, q, id, expected)
		if err != nil {
			return err
		}
		changed, err := work(q, row, roles)
		if err != nil {
			return err
		}
		if changed {
			result, err = bump(ctx, q, id, expected)
		}
		return err
	})
	return result, err
}
