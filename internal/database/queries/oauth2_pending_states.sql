-- name: CreatePendingState :exec
INSERT INTO oauth2_pending_states (
    id,
    state,
    provider,
    expires_at
) VALUES (?, ?, ?, ?);

-- name: SetPendingStateAuthCode :exec
-- Called at /callback when IdP returns the authorization code.
UPDATE oauth2_pending_states
SET auth_code = ?
WHERE state = ?;

-- name: GetPendingStateByAuthCode :one
-- Called at /token to retrieve provider; filters expired records.
SELECT * FROM oauth2_pending_states
WHERE auth_code = ?
  AND expires_at > datetime('now')
LIMIT 1;

-- name: DeletePendingState :exec
-- One-time use: delete after consuming at /token to prevent replay.
DELETE FROM oauth2_pending_states
WHERE id = ?;

-- name: DeleteExpiredPendingStates :exec
-- Housekeeping: remove stale records older than their TTL.
DELETE FROM oauth2_pending_states
WHERE expires_at <= datetime('now');
