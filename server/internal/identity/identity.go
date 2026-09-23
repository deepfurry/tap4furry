// Package identity owns account views and profile policy, independently of HTTP.
package identity

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrValidation        = errors.New("invalid input")
	ErrHandleUnavailable = errors.New("handle unavailable")
	ErrNotFound          = errors.New("profile not found")
	ErrUnavailable       = errors.New("account unavailable")
	handlePattern        = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{2,31}$`)
)

type PublicProfile struct{ Handle, DisplayName, Bio *string }
type Me struct {
	ID                   uuid.UUID
	AccountState         string
	CreatedAt            time.Time
	Profile              PublicProfile
	SearchEngineIndexing bool
	Email                string
	EmailVerified        bool
}

// Field distinguishes omission (keep), null (clear) and a supplied value.
type Field[T any] struct {
	Set   bool
	Value *T
}
type ProfileUpdate struct {
	Handle, DisplayName, Bio Field[string]
	SearchEngineIndexing     *bool
}

func ValidateHandle(handle string) error {
	if !handlePattern.MatchString(handle) {
		return ErrValidation
	}
	return nil
}

func (p ProfileUpdate) Validate() error {
	if p.Handle.Set && p.Handle.Value != nil && ValidateHandle(*p.Handle.Value) != nil {
		return ErrValidation
	}
	for _, field := range []struct {
		value Field[string]
		limit int
	}{{p.DisplayName, 80}, {p.Bio, 500}} {
		if field.value.Set && field.value.Value != nil {
			v := *field.value.Value
			if !utf8.ValidString(v) || strings.ContainsRune(v, 0) || utf8.RuneCountInString(v) > field.limit {
				return ErrValidation
			}
		}
	}
	return nil
}

type App struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *App { return &App{pool: pool} }
func (a *App) Me(ctx context.Context, id uuid.UUID) (Me, error) {
	return ReadMe(ctx, sqlc.New(a.pool), id)
}

// ReadMe also operates inside an authentication transaction before commit.
func ReadMe(ctx context.Context, q *sqlc.Queries, id uuid.UUID) (Me, error) {
	row, err := q.GetMe(ctx, pgtype.UUID{Bytes: id, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return Me{}, ErrUnavailable
	}
	if err != nil {
		return Me{}, database.SafeError("read account", err)
	}
	return Me{ID: uuid.UUID(row.ID.Bytes), AccountState: row.AccountState, CreatedAt: row.CreatedAt.Time,
		Profile:              PublicProfile{Handle: textPointer(row.Handle), DisplayName: textPointer(row.DisplayName), Bio: textPointer(row.Bio)},
		SearchEngineIndexing: row.SearchEngineIndexing, Email: row.Email.String, EmailVerified: row.VerifiedAt.Valid}, nil
}

// ApplyProfileTx joins the caller's authenticated, User-locked transaction.
// The application owns session/restriction checks and the commit.
func ApplyProfileTx(ctx context.Context, tx pgx.Tx, id uuid.UUID, input ProfileUpdate, now time.Time) (Me, error) {
	if err := input.Validate(); err != nil {
		return Me{}, err
	}
	q := sqlc.New(tx)
	indexing := false
	if input.SearchEngineIndexing != nil {
		indexing = *input.SearchEngineIndexing
	}
	n, err := q.UpdateProfile(ctx, sqlc.UpdateProfileParams{UserID: pgtype.UUID{Bytes: id, Valid: true},
		SetHandle: input.Handle.Set, Handle: nullableText(input.Handle.Value), SetDisplayName: input.DisplayName.Set,
		DisplayName: nullableText(input.DisplayName.Value), SetBio: input.Bio.Set, Bio: nullableText(input.Bio.Value),
		SetIndexing: input.SearchEngineIndexing != nil, SearchEngineIndexing: indexing, Now: pgtype.Timestamptz{Time: now, Valid: true}})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "user_profiles_handle_key" {
		return Me{}, ErrHandleUnavailable
	}
	if err != nil {
		return Me{}, database.SafeError("update profile", err)
	}
	if n != 1 {
		return Me{}, ErrUnavailable
	}
	return ReadMe(ctx, q, id)
}

func (a *App) PublicProfile(ctx context.Context, handle string) (PublicProfile, error) {
	if ValidateHandle(handle) != nil {
		return PublicProfile{}, ErrNotFound
	}
	row, err := sqlc.New(a.pool).GetPublicProfileByHandle(ctx, pgtype.Text{String: handle, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return PublicProfile{}, ErrNotFound
	}
	if err != nil {
		return PublicProfile{}, database.SafeError("read public profile", err)
	}
	return PublicProfile{Handle: textPointer(row.Handle), DisplayName: textPointer(row.DisplayName), Bio: textPointer(row.Bio)}, nil
}

func textPointer(v pgtype.Text) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}
func nullableText(v *string) pgtype.Text {
	if v == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *v, Valid: true}
}
