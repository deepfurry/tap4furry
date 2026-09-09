-- name: ListUserRoles :many
SELECT role FROM app.user_roles WHERE user_id = $1 ORDER BY role;

-- name: LockRoleOperations :exec
SELECT pg_advisory_xact_lock(72463159018201);

-- name: LockRoleUserByEmail :one
SELECT u.id, u.account_state, u.deleted_at, a.verified_at,
       EXISTS (SELECT 1 FROM app.password_credentials c WHERE c.user_id = u.id) AS has_password
FROM app.users u JOIN app.auth_identities a ON a.user_id = u.id
WHERE a.provider = 'email' AND a.provider_subject = $1
FOR UPDATE OF u;

-- name: GrantUserRole :execrows
INSERT INTO app.user_roles (user_id, role, created_at) VALUES ($1, $2, $3)
ON CONFLICT (user_id, role) DO NOTHING;

-- name: RevokeUserRole :execrows
DELETE FROM app.user_roles WHERE user_id = $1 AND role = $2;

-- name: CountActiveAdmins :one
SELECT count(*) FROM app.user_roles r JOIN app.users u ON u.id = r.user_id
WHERE r.role = 'admin' AND u.account_state = 'active' AND u.deleted_at IS NULL;

-- name: ReadAdminAccount :one
SELECT u.id, a.email FROM app.users u JOIN app.auth_identities a ON a.user_id = u.id
WHERE u.id = $1 AND a.provider = 'email' AND a.email IS NOT NULL AND a.verified_at IS NOT NULL
  AND u.account_state = 'active' AND u.deleted_at IS NULL
  AND EXISTS (SELECT 1 FROM app.password_credentials c WHERE c.user_id = u.id);

-- name: CreateAdminSession :exec
INSERT INTO app.sessions (id, user_id, kind, auth_method, token_hash, authenticated_at, created_at,
                          last_seen_at, idle_expires_at, absolute_expires_at)
VALUES (sqlc.arg(id), sqlc.arg(user_id), 'admin', 'password', sqlc.arg(token_hash), sqlc.arg(now), sqlc.arg(now), sqlc.arg(now), sqlc.arg(idle_expires_at), sqlc.arg(absolute_expires_at));

-- name: FindActiveAdminSession :one
SELECT s.id, s.user_id, s.authenticated_at, s.last_seen_at
FROM app.sessions s JOIN app.users u ON u.id = s.user_id
WHERE s.token_hash = $1 AND s.kind = 'admin' AND s.auth_method = 'password' AND s.revoked_at IS NULL
  AND s.idle_expires_at > sqlc.arg(now) AND s.absolute_expires_at > sqlc.arg(now)
  AND u.account_state = 'active' AND u.deleted_at IS NULL;

-- name: GetActiveAdminSessionByID :one
SELECT s.id, s.authenticated_at FROM app.sessions s JOIN app.users u ON u.id = s.user_id
WHERE s.id = sqlc.arg(id) AND s.user_id = sqlc.arg(user_id) AND s.kind = 'admin' AND s.auth_method = 'password'
  AND s.revoked_at IS NULL AND s.idle_expires_at > sqlc.arg(now) AND s.absolute_expires_at > sqlc.arg(now)
  AND u.account_state = 'active' AND u.deleted_at IS NULL;

-- name: TouchAdminSession :execrows
UPDATE app.sessions s SET last_seen_at = sqlc.arg(now),
    idle_expires_at = LEAST(sqlc.arg(idle_expires_at)::timestamptz, s.absolute_expires_at)
WHERE s.id = sqlc.arg(id) AND s.kind = 'admin' AND s.revoked_at IS NULL
  AND s.idle_expires_at > sqlc.arg(now) AND s.absolute_expires_at > sqlc.arg(now)
  AND s.last_seen_at <= sqlc.arg(touch_before)
  AND EXISTS (SELECT 1 FROM app.users u WHERE u.id = s.user_id AND u.account_state = 'active' AND u.deleted_at IS NULL)
  AND EXISTS (SELECT 1 FROM app.user_roles r WHERE r.user_id = s.user_id);

-- name: ListActiveAdminSessions :many
SELECT id, authenticated_at, created_at, last_seen_at, idle_expires_at, absolute_expires_at
FROM app.sessions WHERE user_id = sqlc.arg(user_id) AND kind = 'admin' AND revoked_at IS NULL
  AND idle_expires_at > sqlc.arg(now) AND absolute_expires_at > sqlc.arg(now)
ORDER BY created_at DESC, id DESC;

-- name: RevokeAdminSessionByID :execrows
UPDATE app.sessions SET revoked_at = sqlc.arg(now)
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id) AND kind = 'admin' AND revoked_at IS NULL
  AND idle_expires_at > sqlc.arg(now) AND absolute_expires_at > sqlc.arg(now);

-- name: RevokeOtherAdminSessions :exec
UPDATE app.sessions SET revoked_at = sqlc.arg(now)
WHERE user_id = sqlc.arg(user_id) AND id <> sqlc.arg(current_id) AND kind = 'admin' AND revoked_at IS NULL;

-- name: RevokeAllAdminSessions :exec
UPDATE app.sessions SET revoked_at = sqlc.arg(now)
WHERE user_id = sqlc.arg(user_id) AND kind = 'admin' AND revoked_at IS NULL;
