-- name: CreateTopology :one
INSERT INTO topologies (id, workspace_id, name, type, enabled)
VALUES (?, ?, ?, ?, ?)
RETURNING id;

-- name: GetTopology :one
SELECT id, workspace_id, name, type, enabled, updated_at, created_at
FROM topologies
WHERE id = ?
LIMIT 1;

-- name: ListTopologiesByWorkspace :many
SELECT id, workspace_id, name, type, enabled, updated_at, created_at
FROM topologies
WHERE workspace_id = ?
ORDER BY created_at DESC;

-- name: UpdateTopology :exec
UPDATE topologies
SET name = ?, type = ?, enabled = ?
WHERE id = ?;

-- name: DeleteTopology :exec
DELETE FROM topologies WHERE id = ?;

-- name: AddTopologyMember :exec
INSERT INTO topology_members (topology_id, tag_id, role)
VALUES (?, ?, ?);

-- name: RemoveTopologyMember :exec
DELETE FROM topology_members
WHERE topology_id = ? AND tag_id = ? AND role = ?;

-- name: ListTopologyMembers :many
SELECT tm.topology_id, tm.tag_id, tm.role, t.name AS tag_name
FROM topology_members tm
JOIN tags t ON t.id = tm.tag_id
WHERE tm.topology_id = ?
ORDER BY tm.role, t.name;
