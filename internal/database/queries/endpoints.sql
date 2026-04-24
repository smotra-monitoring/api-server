-- name: GetEndpointByIDAndAgentID :one
SELECT id FROM endpoints WHERE id = ? AND agent_id = ? LIMIT 1;
