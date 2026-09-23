package contribution

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/curation"
	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/deepfurry/tap4furry/server/internal/resource"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type App struct {
	pool          *pgxpool.Pool
	contextSecret string
}

func New(pool *pgxpool.Pool, contextSecret string) *App { return &App{pool, contextSecret} }
func id(v uuid.UUID) pgtype.UUID                        { return pgtype.UUID{Bytes: v, Valid: v != uuid.Nil()} }
func uid(v pgtype.UUID) uuid.UUID                       { return uuid.UUID(v.Bytes) }
func txt(v *string) pgtype.Text {
	if v == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *v, Valid: true}
}
func ptr(v pgtype.Text) *string {
	if !v.Valid {
		return nil
	}
	s := v.String
	return &s
}
func str(v string) pgtype.Text             { return pgtype.Text{String: v, Valid: v != ""} }
func stamp(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
func safe(err error) error {
	if err == nil {
		return nil
	}
	var limit *LimitError
	if errors.As(err, &limit) {
		return err
	}
	for _, known := range []error{ErrValidation, ErrNotFound, ErrForbidden, ErrVerified, ErrConflict, ErrRequestConflict, ErrCanonical, auth.ErrUnauthenticated, auth.ErrAdminUnauthenticated, auth.ErrAdminForbidden, resource.ErrVersionConflict, curation.ErrConflict, curation.ErrValidation, curation.ErrNotFound, curation.ErrRelationCycle} {
		if errors.Is(err, known) {
			return err
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgerr *pgconn.PgError
	if errors.As(err, &pgerr) {
		switch pgerr.Code {
		case "23505", "23503":
			return ErrConflict
		case "23514", "22001", "22003":
			return ErrValidation
		}
	}
	return database.SafeError("contribution operation", err)
}
func (a *App) transact(ctx context.Context, check func(pgx.Tx) (time.Time, error), work func(pgx.Tx, *sqlc.Queries, time.Time) error) error {
	return a.transactLevel(ctx, pgx.ReadCommitted, check, work)
}
func (a *App) transactLevel(ctx context.Context, level pgx.TxIsoLevel, check func(pgx.Tx) (time.Time, error), work func(pgx.Tx, *sqlc.Queries, time.Time) error) error {
	tx, err := a.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: level})
	if err != nil {
		return safe(err)
	}
	defer tx.Rollback(ctx)
	now, err := check(tx)
	if err != nil {
		return safe(err)
	}
	if err = work(tx, sqlc.New(tx), now); err != nil {
		return safe(err)
	}
	return safe(tx.Commit(ctx))
}
func publicCheck(ctx context.Context, actor auth.Actor) func(pgx.Tx) (time.Time, error) {
	return func(tx pgx.Tx) (time.Time, error) { return auth.RequirePublicActorTx(ctx, tx, actor) }
}
func adminCheck(ctx context.Context, actor auth.AdminActor) func(pgx.Tx) (time.Time, error) {
	return func(tx pgx.Tx) (time.Time, error) {
		_, err := auth.RequireAdminCapabilityTx(ctx, tx, actor, auth.Editorial)
		return time.Time{}, err
	}
}

func (a *App) revision(author uuid.UUID, row sqlc.ContributionResourceRow) string {
	if a.contextSecret == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(a.contextSecret))
	fmt.Fprintf(mac, "tap4furry/contribution-context/v1\x00%s\x00%s\x00%d\x00%s", author.String(), uid(row.ID).String(), row.Version, row.DefaultLocale)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func resourceContent(row sqlc.ContributionResourceRow) (Content, error) {
	if !row.CanonicalOk || !row.Name.Valid {
		return Content{}, ErrCanonical
	}
	return Content{DefaultLocale: row.DefaultLocale, CategoryID: uid(row.CategoryID), Name: row.Name.String, Summary: ptr(row.Summary), Description: ptr(row.Description), Lifecycle: resource.Lifecycle(row.Lifecycle), ContentRating: resource.ContentRating(row.ContentRating), Slug: row.Slug}, nil
}
func readContents(ctx context.Context, q *sqlc.Queries, key uuid.UUID) (map[string]Content, error) {
	rows, err := q.ContributionContents(ctx, id(key))
	if err != nil {
		return nil, err
	}
	result := map[string]Content{}
	for _, r := range rows {
		c := Content{DefaultLocale: r.DefaultLocale, CategoryID: uid(r.CategoryID), Name: r.Name, Summary: ptr(r.Summary), Description: ptr(r.Description), Lifecycle: resource.Lifecycle(r.Lifecycle), ContentRating: resource.ContentRating(r.ContentRating), Slug: r.Slug.String}
		if r.Url.Valid {
			c.Source = &Source{URL: r.Url.String, Label: ptr(r.Label), Type: resource.SourceType(r.SourceType.String), Availability: resource.SourceAvailabilityState(r.AvailabilityState.String)}
		}
		result[r.ContentKind] = c
	}
	if _, ok := result["proposed"]; !ok {
		return nil, ErrCanonical
	}
	return result, nil
}
func putContent(ctx context.Context, q *sqlc.Queries, key uuid.UUID, kind string, c Content) error {
	err := q.ContributionPutContent(ctx, sqlc.ContributionPutContentParams{ContributionID: id(key), ContentKind: kind, DefaultLocale: c.DefaultLocale, CategoryID: id(c.CategoryID), Name: c.Name, Summary: txt(c.Summary), Description: txt(c.Description), Lifecycle: string(c.Lifecycle), ContentRating: string(c.ContentRating), Slug: str(c.Slug)})
	if err != nil || c.Source == nil {
		return err
	}
	s := c.Source
	return q.ContributionPutSource(ctx, sqlc.ContributionPutSourceParams{ContributionID: id(key), ContentKind: kind, Url: s.URL, Label: txt(s.Label), SourceType: string(s.Type), AvailabilityState: string(s.Availability)})
}
func categoryOK(ctx context.Context, q *sqlc.Queries, category, previous uuid.UUID) error {
	state, err := q.ContributionCategory(ctx, id(category))
	if err != nil {
		return err
	}
	if state != "active" && category != previous {
		return ErrValidation
	}
	return nil
}
