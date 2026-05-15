-- name: InsertAgentVitals :exec
INSERT INTO agent_vitals (
    id,
    agent_id,
    cpu_pct,
    mem_used_mb,
    mem_total_mb,
    system_uptime_secs,
    reported_at
) VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetLatestAgentVitals :one
SELECT id, agent_id, cpu_pct, mem_used_mb, mem_total_mb, system_uptime_secs, reported_at, received_at
FROM agent_vitals
WHERE agent_id = ?
ORDER BY reported_at DESC
LIMIT 1;

-- name: GetAgentVitalsHistory :many
SELECT id, agent_id, cpu_pct, mem_used_mb, mem_total_mb, system_uptime_secs, reported_at, received_at
FROM agent_vitals
WHERE agent_id = ?
  AND reported_at >= ?
  AND reported_at <= ?
ORDER BY reported_at ASC;
