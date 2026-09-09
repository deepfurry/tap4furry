-- name: CreateSession :execrows
INSERT INTO app.sessions (id, user_id, kind, auth_method, token_hash, authenticated_at, created_at,
                          last_seen_at, idle_expires_at, absolute_expires_at)
SELECT sqlc.arg(id), u.id, 'public', sqlc.arg(auth_method), sqlc.arg(token_hash), sqlc.arg(authenticated_at), sqlc.arg(now),
       sqlc.arg(now), sqlc.arg(idle_expires_at), sqlc.arg(absolute_expires_at)
FROM app.users u
WHERE u.id = sqlc.arg(user_id) AND u.account_state = 'active' AND u.deleted_at IS NULL;

-- name: FindActiveSessionByTokenHash :one
SELECT s.id, s.user_id, s.kind, s.auth_method, s.authenticated_at, s.last_seen_at, s.absolute_expires_at
FROM app.sessions s JOIN app.users u ON u.id = s.user_id
WHERE s.token_hash = $1 AND s.kind = 'public' AND s.revoked_at IS NULL
  AND s.idle_expires_at > sqlc.arg(now) AND s.absolute_expires_at > sqlc.arg(now)
  AND u.account_state = 'active' AND u.deleted_at IS NULL;

-- name: TouchSession :execrows
UPDATE app.sessions s SET last_seen_at = sqlc.arg(now),
    idle_expires_at = LEAST(sqlc.arg(idle_expires_at)::timestamptz, s.absolute_expires_at)
WHERE s.id = sqlc.arg(id) AND s.kind = 'public' AND s.revoked_at IS NULL
  AND s.idle_expires_at > sqlc.arg(now) AND s.absolute_expires_at > sqlc.arg(now)
  AND s.last_seen_at <= sqlc.arg(touch_before)
  AND EXISTS (SELECT 1 FROM app.users u WHERE u.id = s.user_id AND u.account_state = 'active' AND u.deleted_at IS NULL);

-- name: RevokeSession :exec
UPDATE app.sessions SET revoked_at = sqlc.arg(now)
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id) AND kind = 'public' AND revoked_at IS NULL;

-- name: GetActivePublicSessionByID :one
SELECT s.id, s.auth_method, s.authenticated_at FROM app.sessions s JOIN app.users u ON u.id = s.user_id
WHERE s.id = sqlc.arg(id) AND s.user_id = sqlc.arg(user_id) AND s.kind = 'public'
  AND s.revoked_at IS NULL AND s.idle_expires_at > sqlc.arg(now) AND s.absolute_expires_at > sqlc.arg(now)
  AND u.account_state = 'active' AND u.deleted_at IS NULL;

-- name: ListActivePublicSessions :many
SELECT id, auth_method, authenticated_at, created_at, last_seen_at, idle_expires_at, absolute_expires_at
FROM app.sessions WHERE user_id = sqlc.arg(user_id) AND kind = 'public' AND revoked_at IS NULL
  AND idle_expires_at > sqlc.arg(now) AND absolute_expires_at > sqlc.arg(now)
ORDER BY created_at DESC, id DESC;

-- name: RevokePublicSessionByID :execrows
UPDATE app.sessions SET revoked_at = sqlc.arg(now)
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id) AND kind = 'public' AND revoked_at IS NULL
  AND idle_expires_at > sqlc.arg(now) AND absolute_expires_at > sqlc.arg(now);

-- name: RevokeOtherPublicSessions :exec
UPDATE app.sessions SET revoked_at = sqlc.arg(now)
WHERE user_id = sqlc.arg(user_id) AND id <> sqlc.arg(current_id) AND kind = 'public' AND revoked_at IS NULL;

-- name: RevokeAllUserSessions :exec
UPDATE app.sessions SET revoked_at = sqlc.arg(now)
WHERE user_id = sqlc.arg(user_id) AND revoked_at IS NULL;
