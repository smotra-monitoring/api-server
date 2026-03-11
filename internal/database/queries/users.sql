-- name: CreateUser :one
INSERT INTO users (
    id,
    tenant_id,
    oauth_provider,
    oauth_subject,
    display_name
) VALUES (?, ?, ?, ?, ?)
RETURNING id;

-- name: GetUserByID :one
SELECT * FROM users
WHERE id = ? LIMIT 1;

-- name: GetUserByOAuth :one
SELECT * FROM users
WHERE oauth_provider = ? AND oauth_subject = ?
LIMIT 1;

-- name: UpsertUserByOAuth :one
-- Creates or updates user on OAuth login
INSERT INTO users (
    id,
    tenant_id,
    oauth_provider,
    oauth_subject,
    display_name,
    last_login_at
) VALUES (?, ?, ?, ?, ?, datetime('now'))
ON CONFLICT(oauth_provider, oauth_subject) DO UPDATE SET
    display_name = COALESCE(excluded.display_name, display_name),
    last_login_at = datetime('now'),
    updated_at = datetime('now')
RETURNING id;

-- name: UpdateUserDisplayName :exec
UPDATE users
SET display_name = ?,
    updated_at = datetime('now')
WHERE id = ?;

-- name: UpdateUserLastLogin :exec
UPDATE users
SET last_login_at = datetime('now')
WHERE id = ?;

-- name: ListUsersByTenant :many
SELECT * FROM users
WHERE tenant_id = ?
ORDER BY created_at DESC;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = ?;
