package auth

import (
	"context"
	"time"

	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/jackc/pgx/v5"
)

// RequirePublicActorTx serializes authenticated application writes with account,
// password and session changes. Keep the transaction open until the write commits.
func RequirePublicActorTx(ctx context.Context, tx pgx.Tx, actor Actor) (time.Time, error) {
	q := sqlc.New(tx)
	if err := lockUser(ctx, q, actor.UserID); err != nil {
		return time.Time{}, err
	}
	var now time.Time
	if err := tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); err != nil {
		return now, database.SafeError("read authorization time", err)
	}
	return now, requireActor(ctx, q, actor, now)
}
