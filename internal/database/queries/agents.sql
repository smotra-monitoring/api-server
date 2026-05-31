-- name: GetAgentConfigurationBase :one
SELECT id, config_version, name, base_config FROM agents WHERE id = ?
LIMIT 1;

-- name: CreateAgent :one
INSERT INTO agents (id, section_id, name, api_key_hash, base_config) VALUES 
(?, ?, ?, ?, ?)
RETURNING id;

-- name: CreateAgentFromClaim :one
-- Creates an agent after successful claim
-- name param should be initialized from agent_claims.hostname
INSERT INTO agents (
    id,
    section_id,
    name,
    api_key_hash,
    base_config,
    agent_version,
    ip_addresses_json
) VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: GetAgentTags :many
SELECT t.name FROM agent_tags at
JOIN tags t ON at.tag_id = t.id
WHERE at.agent_id = ? AND t.scope IN ('agent', 'global');

-- name: UpdateAgentConfiguration :exec
UPDATE agents
SET config_version = ?, base_config = ?
WHERE id = ?;

-- name: GetEndpointTags :many
SELECT t.name FROM endpoint_tags et
JOIN tags t ON et.tag_id = t.id
WHERE et.endpoint_id = ? AND t.scope IN ('endpoint', 'global');

-- name: VerifyAgentAPIKey :one
SELECT id, api_key_hash FROM agents WHERE id = ?;

-- name: UpdateAgentLastSeen :exec
UPDATE agents SET last_seen_at = ? WHERE id = ?;

-- name: UpdateAgentLastResultSubmittedAt :exec
UPDATE agents SET last_result_submitted_at = ? WHERE id = ?;

-- name: ListAgentsByTenant :many
-- Returns paginated agents belonging to all sections of a given tenant.
SELECT a.id, a.section_id, a.name, a.config_version, a.agent_version,
       a.ip_addresses_json, a.last_seen_at, a.last_result_submitted_at,
       a.updated_at, a.created_at
FROM agents a
JOIN sections s ON a.section_id = s.id
WHERE s.tenant_id = ?
ORDER BY a.created_at DESC
LIMIT ? OFFSET ?;

-- name: CountAgentsByTenant :one
-- Returns total count of agents belonging to all sections of a given tenant.
SELECT COUNT(*) FROM agents a
JOIN sections s ON a.section_id = s.id
WHERE s.tenant_id = ?;
