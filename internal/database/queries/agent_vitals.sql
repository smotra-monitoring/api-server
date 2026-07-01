-- name: InsertAgentVitals :exec
INSERT INTO agent_vitals (
    id,
    agent_id,
    cpu_pct,
    mem_used_mb,
    mem_total_mb,
    system_uptime_secs,
    agent_uptime_secs,
    reported_at,
    agent_version,
    config_version,
    is_running,
    started_at,
    stopped_at,
    checks_performed,
    checks_successful,
    checks_failed,
    last_report_at,
    failed_report_count,
    server_connected,
    cache_capacity,
    cache_len
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetLatestAgentVitals :one
SELECT id, agent_id, cpu_pct, mem_used_mb, mem_total_mb, system_uptime_secs, agent_uptime_secs, reported_at, received_at,
       agent_version, config_version, is_running, started_at, stopped_at,
       checks_performed, checks_successful, checks_failed, last_report_at,
       failed_report_count, server_connected, cache_capacity, cache_len
FROM agent_vitals
WHERE agent_id = ?
ORDER BY reported_at DESC
LIMIT 1;

-- name: GetAgentVitalsHistory :many
SELECT id, agent_id, cpu_pct, mem_used_mb, mem_total_mb, system_uptime_secs, agent_uptime_secs, reported_at, received_at,
       agent_version, config_version, is_running, started_at, stopped_at,
       checks_performed, checks_successful, checks_failed, last_report_at,
       failed_report_count, server_connected, cache_capacity, cache_len
FROM agent_vitals
WHERE agent_id = ?
  AND reported_at >= ?
  AND reported_at <= ?
ORDER BY reported_at ASC;
