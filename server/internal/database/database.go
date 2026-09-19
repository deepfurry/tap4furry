// Package database constructs explicit PostgreSQL pools and system queries.
package database

import (
	"context"
	"errors"
	"time"

	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Open(ctx context.Context, rawURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(rawURL)
	if err != nil {
		return nil, errors.New("invalid PostgreSQL configuration (details withheld)")
	}
	cfg.ConnConfig.ConnectTimeout = 5 * time.Second
	cfg.MaxConns = 6
	cfg.MinConns = 0
	cfg.MaxConnLifetime = 30 * time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, SafeError("construct PostgreSQL pool", err)
	}
	return pool, nil
}

func Ready(ctx context.Context, pool *pgxpool.Pool) error {
	value, err := sqlc.New(pool).CheckReadiness(ctx)
	if err != nil {
		return SafeError("PostgreSQL readiness", err)
	}
	if value != 1 {
		return errors.New("unexpected PostgreSQL readiness response")
	}
	return nil
}

// SafeError retains only SQLSTATE or cancellation, never DSNs/hosts/server messages.
func SafeError(operation string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return errors.New(operation + " failed (SQLSTATE " + pgErr.Code + ")")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New(operation + " timed out")
	}
	if errors.Is(err, context.Canceled) {
		return errors.New(operation + " canceled")
	}
	return errors.New(operation + " failed (details withheld)")
}
