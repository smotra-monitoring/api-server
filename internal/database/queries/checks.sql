-- name: CheckResultExists :one
SELECT id FROM check_results WHERE id = ? LIMIT 1;

-- name: InsertCheckResult :exec
INSERT INTO check_results (
    id,
    agent_id,
    endpoint_id,
    check_type,
    success,
    checked_at
) VALUES (?, ?, ?, ?, ?, ?);

-- name: InsertPingCheckResult :exec
INSERT INTO ping_check_results (
    check_id,
    resolved_ip,
    successes,
    failures,
    avg_response_time_ms,
    success_latencies_json,
    errors_json
) VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: InsertHttpGetCheckResult :exec
INSERT INTO http_get_check_results (
    check_id,
    status_code,
    response_time_ms,
    response_size_bytes,
    error
) VALUES (?, ?, ?, ?, ?);

-- name: InsertTcpConnectCheckResult :exec
INSERT INTO tcp_connect_check_results (
    check_id,
    resolved_ip,
    connected,
    connect_time_ms,
    error
) VALUES (?, ?, ?, ?, ?);

-- name: InsertUdpConnectCheckResult :exec
INSERT INTO udp_connect_check_results (
    check_id,
    resolved_ip,
    probe_successful,
    response_time_ms,
    error
) VALUES (?, ?, ?, ?, ?);

-- name: InsertTracerouteCheckResult :exec
INSERT INTO traceroute_check_results (
    check_id,
    target_reached,
    total_time_ms,
    errors_json
) VALUES (?, ?, ?, ?);

-- name: InsertTracerouteHop :exec
INSERT INTO traceroute_hops (
    id,
    check_id,
    hop,
    address,
    hostname,
    response_time_ms
) VALUES (?, ?, ?, ?, ?, ?);

-- name: InsertPluginCheckResult :exec
INSERT INTO plugin_check_results (
    check_id,
    plugin_name,
    plugin_version,
    success,
    response_time_ms,
    error,
    data_json
) VALUES (?, ?, ?, ?, ?, ?, ?);
