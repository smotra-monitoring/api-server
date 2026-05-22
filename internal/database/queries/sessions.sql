-- name: CreateSession :one
INSERT INTO sessions (
    id,
    user_id,
    token_hash,
    sliding_expires_at,
    expires_at,
    oauth2_provider,
    oauth2_access_token,
    oauth2_refresh_token,
    oauth2_token_expiry,
    oauth2_id_token,
    oauth2_scope,
    oauth2_token_type
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetSessionByTokenHash :one
-- Returns a valid, non-revoked, non-expired session by token hash.
-- Does NOT filter on oauth2_token_expiry; middleware handles transparent IDP refresh.
SELECT * FROM sessions
WHERE token_hash = ?
  AND revoked = 0
  AND sliding_expires_at > datetime('now')
  AND expires_at > datetime('now')
LIMIT 1;

-- name: GetSessionByID :one
SELECT * FROM sessions
WHERE id = ?
LIMIT 1;

-- name: UpdateSessionLastUsed :exec
UPDATE sessions
SET last_used_at = datetime('now')
WHERE id = ?;

-- name: RevokeSession :exec
UPDATE sessions
SET revoked = 1
WHERE id = ?;

-- name: UpdateSessionOAuth2Tokens :exec
-- Updates IDP tokens after a transparent refresh.
-- Slides sliding_expires_at forward by 7 days, capped at expires_at (hard cap).
UPDATE sessions
SET oauth2_access_token          = ?,
    oauth2_refresh_token         = ?,
    oauth2_token_expiry          = ?,
    oauth2_id_token              = ?,
    oauth2_token_refresh_count   = oauth2_token_refresh_count + 1,
    oauth2_token_refresh_last_at = datetime('now'),
    sliding_expires_at           = min(datetime('now', '+7 days'), expires_at)
WHERE id = ?;
