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
INSERT INTO check_results_ping (
    check_id,
    resolved_ip,
    successes,
    failures,
    success_latencies_json,
    errors_json
) VALUES (?, ?, ?, ?, ?, ?);

-- name: InsertHttpGetCheckResult :exec
INSERT INTO check_results_http_get (
    check_id,
    status_code,
    response_time_ms,
    response_size_bytes,
    errors_json
) VALUES (?, ?, ?, ?, ?);

-- name: InsertTcpConnectCheckResult :exec
INSERT INTO check_results_tcp_connect (
    check_id,
    resolved_ip,
    connected,
    connect_time_ms,
    errors_json
) VALUES (?, ?, ?, ?, ?);

-- name: InsertUdpConnectCheckResult :exec
INSERT INTO check_results_udp_connect (
    check_id,
    resolved_ip,
    probe_successful,
    response_time_ms,
    errors_json
) VALUES (?, ?, ?, ?, ?);

-- name: InsertTracerouteCheckResult :exec
INSERT INTO check_results_traceroute (
    check_id,
    target_reached,
    errors_json
) VALUES (?, ?, ?);

-- name: InsertTracerouteHop :exec
INSERT INTO check_results_traceroute_hops (
    id,
    check_id,
    hop,
    resolved_ip,
    hostname,
    success_latencies_json
) VALUES (?, ?, ?, ?, ?, ?);

-- name: InsertPluginCheckResult :exec
INSERT INTO check_results_plugin (
    check_id,
    plugin_name,
    plugin_version,
    success,
    response_time_ms,
    errors_json,
    data_json
) VALUES (?, ?, ?, ?, ?, ?, ?);
