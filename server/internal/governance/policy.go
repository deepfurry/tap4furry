// Package governance provides the fixed business policies and transactional
// primitives shared by contribution, curation and moderation. It does not own Auth.
package governance

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/jackc/pgx/v5/pgtype"
)

var ErrValidation = errors.New("invalid governance input")

type Scope string

const (
	ContributionSubmit Scope = "contribution_submit"
	PublicProfileWrite Scope = "public_profile_write"
	AllWrite           Scope = "all_write"
)

func (s Scope) Valid() bool {
	return s == ContributionSubmit || s == PublicProfileWrite || s == AllWrite
}
func (s Scope) Blocks(action Scope) bool {
	return (action == ContributionSubmit || action == PublicProfileWrite) && (s == action || s == AllWrite)
}

type Budget struct {
	Daily, Pending int64
	Interval       time.Duration
}

func ContributionBudget(level string) (Budget, error) {
	switch level {
	case "new":
		return Budget{10, 5, time.Minute}, nil
	case "established":
		return Budget{30, 10, 30 * time.Second}, nil
	case "trusted":
		return Budget{60, 20, 15 * time.Second}, nil
	default:
		return Budget{}, ErrValidation
	}
}
func ReportBudget() Budget { return Budget{10, 5, time.Minute} }
func BudgetTx(ctx context.Context, q *sqlc.Queries, user uuid.UUID) (Budget, error) {
	row, err := q.GovernanceProfile(ctx, ID(user))
	if err != nil {
		return Budget{}, err
	}
	return ContributionBudget(row.TrustLevel)
}

type RestrictedError struct {
	Scope   Scope
	Message string
	Until   *time.Time
}

func (*RestrictedError) Error() string { return "business action restricted" }
func CheckTx(ctx context.Context, q *sqlc.Queries, user uuid.UUID, action Scope, now time.Time) error {
	if action != ContributionSubmit && action != PublicProfileWrite {
		return ErrValidation
	}
	rows, err := q.GovernanceEffectiveRestrictions(ctx, sqlc.GovernanceEffectiveRestrictionsParams{UserID: ID(user), Now: Timestamp(now)})
	if err != nil {
		return err
	}
	for _, row := range rows {
		if Scope(row.Scope).Blocks(action) {
			e := &RestrictedError{Scope: Scope(row.Scope), Message: row.UserMessage}
			if row.ExpiresAt.Valid {
				t := row.ExpiresAt.Time
				e.Until = &t
			}
			return e
		}
	}
	return nil
}
func Text(v string, limit int) (string, error) {
	v = strings.TrimSpace(v)
	if !utf8.ValidString(v) || strings.ContainsRune(v, 0) || utf8.RuneCountInString(v) < 1 || utf8.RuneCountInString(v) > limit {
		return "", ErrValidation
	}
	return v, nil
}
func ID(v uuid.UUID) pgtype.UUID               { return pgtype.UUID{Bytes: v, Valid: v != uuid.Nil()} }
func Timestamp(v time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: v, Valid: true} }
func Optional(v string) pgtype.Text            { return pgtype.Text{String: v, Valid: v != ""} }

// RecommendationEligibility expresses eligibility, never a safety guarantee.
type RecommendationEligibility struct{ Public, Canonical, General, Active, CategoryActive, Excluded, ConfirmedActiveSource bool }

func (e RecommendationEligibility) Eligible() bool {
	return e.Public && e.Canonical && e.General && e.Active && e.CategoryActive && !e.Excluded && e.ConfirmedActiveSource
}
