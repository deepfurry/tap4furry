package auth

import (
	"context"
	"errors"
	"time"
	"uuid"

	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Role string

const (
	Moderator     Role = "moderator"
	Editor        Role = "editor"
	Administrator Role = "admin"
)

type Capability string

const (
	AdminAccess    Capability = "admin_access"
	Moderation     Capability = "moderation"
	Editorial      Capability = "editorial"
	Administration Capability = "administration"
)

func ValidRole(role Role) bool { return role == Moderator || role == Editor || role == Administrator }
func HasCapability(roles []Role, capability Capability) bool {
	for _, role := range roles {
		switch capability {
		case AdminAccess:
			if ValidRole(role) {
				return true
			}
		case Moderation:
			if role == Moderator || role == Administrator {
				return true
			}
		case Editorial:
			if role == Editor || role == Administrator {
				return true
			}
		case Administration:
			if role == Administrator {
				return true
			}
		}
	}
	return false
}
func readRoles(ctx context.Context, q *sqlc.Queries, userID uuid.UUID) ([]Role, error) {
	rows, err := q.ListUserRoles(ctx, dbID(userID))
	if err != nil {
		return nil, database.SafeError("read roles", err)
	}
	roles := make([]Role, 0, len(rows))
	for _, row := range rows {
		roles = append(roles, Role(row))
	}
	return roles, nil
}

var (
	ErrRoleIneligible = errors.New("role requires an active verified local account with a password")
	ErrLastAdmin      = errors.New("cannot revoke the final active admin role")
	ErrInvalidRole    = errors.New("unsupported role")
)

// RoleOperator is composed only by the owner/migrator CLI. Runtime processes
// cannot mutate roles. A fixed transaction advisory lock serializes cross-user
// last-admin decisions; it is always taken before the shared User auth lock.
type RoleOperator struct{ pool *pgxpool.Pool }

func NewRoleOperator(pool *pgxpool.Pool) *RoleOperator { return &RoleOperator{pool: pool} }
func (o *RoleOperator) Roles(ctx context.Context, email string) ([]Role, error) {
	return o.operate(ctx, email, "", false, true)
}
func (o *RoleOperator) Grant(ctx context.Context, email string, role Role) ([]Role, error) {
	return o.operate(ctx, email, role, true, false)
}
func (o *RoleOperator) Revoke(ctx context.Context, email string, role Role) ([]Role, error) {
	return o.operate(ctx, email, role, false, false)
}
func (o *RoleOperator) operate(ctx context.Context, email string, role Role, grant, list bool) ([]Role, error) {
	email, err := NormalizeEmail(email)
	if err != nil {
		return nil, err
	}
	if !list && !ValidRole(role) {
		return nil, ErrInvalidRole
	}
	tx, err := o.pool.Begin(ctx)
	if err != nil {
		return nil, database.SafeError("begin role operation", err)
	}
	defer tx.Rollback(ctx)
	q := sqlc.New(tx)
	if err = q.LockRoleOperations(ctx); err != nil {
		return nil, database.SafeError("serialize role operators", err)
	}
	row, err := q.LockRoleUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRoleIneligible
	}
	if err != nil {
		return nil, database.SafeError("lock role user", err)
	}
	id, now := uuid.UUID(row.ID.Bytes), time.Now().UTC()
	if !list {
		var n int64
		if grant {
			if row.AccountState != "active" || row.DeletedAt.Valid || !row.VerifiedAt.Valid || !row.HasPassword {
				return nil, ErrRoleIneligible
			}
			n, err = q.GrantUserRole(ctx, sqlc.GrantUserRoleParams{UserID: row.ID, Role: string(role), CreatedAt: timestamp(now)})
		} else {
			n, err = q.RevokeUserRole(ctx, sqlc.RevokeUserRoleParams{UserID: row.ID, Role: string(role)})
			if err == nil && n != 0 && role == Administrator && row.AccountState == "active" && !row.DeletedAt.Valid {
				var remaining int64
				remaining, err = q.CountActiveAdmins(ctx)
				if err == nil && remaining == 0 {
					return nil, ErrLastAdmin
				}
			}
		}
		if err != nil {
			return nil, database.SafeError("change role", err)
		}
		if n != 0 {
			event := roleEvent(role, grant)
			if err = recordEvent(ctx, q, event, id, uuid.Nil(), now); err != nil {
				return nil, err
			}
		}
	}
	roles, err := readRoles(ctx, q, id)
	if err != nil {
		return nil, err
	}
	if !list && !grant && !HasCapability(roles, AdminAccess) {
		if err = q.RevokeAllAdminSessions(ctx, sqlc.RevokeAllAdminSessionsParams{UserID: row.ID, Now: timestamp(now)}); err != nil {
			return nil, database.SafeError("revoke roleless admin sessions", err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, database.SafeError("commit role operation", err)
	}
	return roles, nil
}
func roleEvent(role Role, grant bool) eventType {
	switch role {
	case Moderator:
		if grant {
			return roleModeratorGranted
		}
		return roleModeratorRevoked
	case Editor:
		if grant {
			return roleEditorGranted
		}
		return roleEditorRevoked
	case Administrator:
		if grant {
			return roleAdminGranted
		}
		return roleAdminRevoked
	}
	panic("unvalidated role event")
}
