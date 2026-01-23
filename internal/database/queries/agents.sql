-- name: GetAgent :one
SELECT * FROM agents WHERE id = ?
LIMIT 1;

-- name: ListAgents :many
SELECT * FROM agents
ORDER BY id
LIMIT ? OFFSET ?;

-- name: CreateAgent :one
INSERT INTO agents (id, section_id, name, api_key_hash, base_config) VALUES 
(?, ?, ?, ?, ?)
RETURNING id;

-- name: UpdateAgent :one
UPDATE agents
SET section_id = ?, name = ?, api_key_hash = ?, base_config = ?
WHERE id = ?
RETURNING *;

-- name: DeleteAgent :exec
DELETE FROM agents WHERE id = ?;

-- name: ListAgentsBySection :many
SELECT * FROM agents
WHERE section_id = ?
ORDER BY id
LIMIT ? OFFSET ?;


--  remove above lines , those might be deleted


-- name: GetAgentConfigurationBase :one
SELECT id, version, name, base_config FROM agents WHERE id = ?
LIMIT 1;

-- name: GetAgentTags :many
SELECT t.name FROM agent_tags at
JOIN tags t ON at.tag_id = t.id
WHERE at.agent_id = ? AND t.scope IN ('agent', 'global');

-- name: UpdateAgentConfiguration :exec
UPDATE agents
SET version = ?, base_config = ?
WHERE id = ?;

-- name: GetAgentEndpoints :many
SELECT id, address, enabled FROM endpoints WHERE agent_id = ?;

-- name: GetEndpointTags :many
SELECT t.name FROM endpoint_tags et
JOIN tags t ON et.tag_id = t.id
WHERE et.endpoint_id = ? AND t.scope IN ('endpoint', 'global');
