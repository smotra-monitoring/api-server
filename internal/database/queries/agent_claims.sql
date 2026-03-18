-- name: CreateAgentClaim :one
INSERT INTO agent_claims (
    id,
    claim_token_hash,
    hostname,
    agent_version,
    claim_token_expires_at
) VALUES (?, ?, ?, ?, ?)
RETURNING id;

-- name: UpsertAgentClaim :one
INSERT INTO agent_claims (
    id,
    claim_token_hash,
    hostname,
    agent_version,
    claim_token_expires_at,
    last_seen_at
) VALUES (?, ?, ?, ?, ?, datetime('now'))
ON CONFLICT(id) DO UPDATE SET
    last_seen_at = datetime('now'),
    hostname = COALESCE(excluded.hostname, hostname),
    agent_version = COALESCE(excluded.agent_version, agent_version)
WHERE claimed_at IS NULL  -- Only update if not yet claimed
RETURNING id;

-- name: GetAgentClaim :one
SELECT * FROM agent_claims
WHERE id = ? LIMIT 1;

-- name: IncrementAgentClaimPollCount :exec
UPDATE agent_claims
SET poll_count = poll_count + 1
WHERE id = ?;

-- name: GetAgentClaimForClaiming :one
-- Used when user attempts to claim an agent
SELECT * FROM agent_claims
WHERE id = ?
  AND claim_token_hash = ?
  AND claim_token_expires_at > datetime('now')
  AND claimed_at IS NULL
LIMIT 1;

-- name: MarkAgentClaimClaimed :exec
UPDATE agent_claims
SET claimed_at = datetime('now'),
    claimed_by_user_id = ?,
    api_key_plaintext = ?
WHERE id = ?;

-- name: MarkAgentClaimAPIKeyDelivered :exec
UPDATE agent_claims
SET api_key_delivered = 1,
    api_key_plaintext = NULL  -- Clear plaintext key after delivery
WHERE id = ?;

-- name: GetPendingAPIKeyDelivery :one
-- Agent polls this to get API key after being claimed
SELECT 
    ac.id,
    ac.claimed_at,
    ac.api_key_plaintext
FROM agent_claims ac
WHERE ac.id = ?
  AND ac.claimed_at IS NOT NULL
  AND ac.api_key_delivered = 0
  AND ac.api_key_plaintext IS NOT NULL
LIMIT 1;

-- name: CleanupExpiredClaims :exec
DELETE FROM agent_claims 
WHERE claim_token_expires_at < datetime('now');

-- name: CleanupDeliveredClaims :exec
DELETE FROM agent_claims 
WHERE claimed_at IS NOT NULL 
  AND api_key_delivered = 1
  AND claimed_at < datetime('now', '-1 hour'); -- Keep for 1 hour after delivery

-- name: ListUnclaimedAgents :many
SELECT * FROM agent_claims
WHERE claimed_at IS NULL
  AND claim_token_expires_at > datetime('now')
ORDER BY created_at DESC;

-- name: ListPendingDeliveries :many
SELECT * FROM agent_claims
WHERE claimed_at IS NOT NULL
  AND api_key_delivered = 0
ORDER BY claimed_at ASC;
