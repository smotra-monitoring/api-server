# Monitoring Results Submission and Agent Heartbeat

This document describes the two endpoints agents use after they have been claimed and hold a valid API key.

## Authentication

Both endpoints require the agent to authenticate using its API key:

```
X-Agent-API-Key: <api-key>
```

The agent ID in the URL path must match the agent ID associated with the API key. A mismatch returns `503` (to avoid information leakage about agent existence).

---

## POST /v1/agent/{agentId}/results

Submit a batch of monitoring check results.

### Request

```
POST /v1/agent/{agentId}/results
Content-Type: application/json
X-Agent-API-Key: <api-key>
```

```json
{
  "results": [
    {
      "id": "<uuidv7>",
      "endpointId": "<uuidv7>",
      "timestamp": "2026-05-12T10:00:00Z",
      "checkType": "ping",
      "result": { ... }
    }
  ]
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `results` | array | Yes | Array of check results (at least 1 item) |
| `results[].id` | UUIDv7 string | Yes | Client-assigned result ID — used for idempotent deduplication |
| `results[].endpointId` | UUIDv7 string | Yes | ID of the endpoint being monitored |
| `results[].timestamp` | RFC3339 | Yes | When the check was performed |
| `results[].checkType` | string | Yes | One of: `ping`, `httpget`, `tcpconnect`, `udpconnect`, `traceroute`, `plugin` |
| `results[].result` | object | Yes | Check-type-specific result object (see below) |

#### Idempotent Deduplication

The `results[].id` field must be a client-generated UUIDv7. The server uses this ID as the primary key. Submitting a batch that contains an ID already in the database causes that row to be skipped (not an error). This allows safe retries without double-counting.

### Check Result Schemas

#### `ping`

```json
{
  "successLatencies": [12.3, 11.8, 13.1],
  "packetsSent": 3,
  "packetsReceived": 3,
  "errors": []
}
```

| Field | Type | Description |
|-------|------|-------------|
| `successLatencies` | float array | Round-trip times in ms for each successful probe |
| `packetsSent` | int | Total ICMP packets sent |
| `packetsReceived` | int | Total ICMP packets received |
| `errors` | string array | Per-probe error messages (empty on full success) |

#### `httpget`

```json
{
  "statusCode": 200,
  "latencyMs": 45.2,
  "bodyByteCount": 1234,
  "error": null
}
```

#### `tcpconnect`

```json
{
  "successLatencies": [8.1, 8.3],
  "attemptsCount": 2,
  "errors": []
}
```

#### `udpconnect`

```json
{
  "successLatencies": [2.1],
  "attemptsCount": 1,
  "errors": []
}
```

#### `traceroute`

```json
{
  "hops": [
    {
      "hopNumber": 1,
      "resolvedIp": "192.168.1.1",
      "successLatencies": [1.2, 1.1, 1.3]
    },
    {
      "hopNumber": 2,
      "resolvedIp": "10.0.0.1",
      "successLatencies": []
    }
  ]
}
```

Hops with no responding router have an empty `successLatencies` array.

#### `plugin`

```json
{
  "pluginName": "custom-check",
  "success": true,
  "latencyMs": 100.0,
  "output": "{ ... }",
  "error": null
}
```

### Response

**200 OK** — batch accepted:

```json
{
  "accepted": 3,
  "duplicatesSkipped": 1
}
```

| Field | Type | Description |
|-------|------|-------------|
| `accepted` | int | Number of new results inserted |
| `duplicatesSkipped` | int | Number of results skipped because their ID already existed |

**400 Bad Request** — missing or empty body.  
**401 Unauthorized** — missing or invalid API key.  
**503 Service Unavailable** — database error or agent ID mismatch.

### Prometheus Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `smotra_submit_results_attempts_total` | counter | Total batch submission attempts |
| `smotra_submit_results_success_total` | counter | Batches accepted without error |
| `smotra_submit_results_failure_total` | counter | Batches rejected (auth or DB error) |
| `smotra_submit_results_accepted_total` | counter | Individual results inserted |
| `smotra_submit_results_duplicates_total` | counter | Individual results skipped as duplicates |

---

## POST /v1/agent/{agentId}/heartbeat

Send a vitals and status snapshot. The server always updates `last_seen_at` on the agent record and stores the heartbeat data (system metrics + agent operational status). Heartbeats should be sent at a regular interval (e.g. every 30–60 seconds).

### Request

```
POST /v1/agent/{agentId}/heartbeat
Content-Type: application/json
X-Agent-API-Key: <api-key>
```

```json
{
  "timestamp": "2024-01-15T10:30:00Z",
  "health_status": "healthy",
  "metrics": {
    "cpu_usage_percent": 12.5,
    "memory_usage_mb": 1024.0,
    "memory_total_mb": 8192.0,
    "system_uptime_secs": 86400,
    "agent_uptime_secs": 3600
  },
  "agent_status": {
    "agent_version": "0.1.0",
    "config_version": 5,
    "is_running": true,
    "started_at": "2024-01-15T09:30:00Z",
    "stopped_at": null,
    "checks_performed": 120,
    "checks_successful": 118,
    "checks_failed": 2,
    "reported_at": "2024-01-15T10:29:30Z",
    "failed_report_count": 0,
    "server_connected": true,
    "cache_stats": {
      "capacity": 1000,
      "len": 5
    }
  }
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `timestamp` | string (RFC3339) | Yes | Agent-local timestamp when heartbeat was generated |
| `health_status` | string | Yes | Agent health status: `healthy` or `degraded` |
| `metrics` | object | Yes | System resource utilisation metrics (see below) |
| `agent_status` | object | Yes | Agent operational status (see below) |

**`metrics` object:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `cpu_usage_percent` | float | Yes | CPU utilization 0.0–100.0 |
| `memory_usage_mb` | float | Yes | Resident memory currently in use (MB) |
| `memory_total_mb` | float | Yes | Total physical memory available (MB) |
| `system_uptime_secs` | int64 | Yes | OS/system uptime in seconds |
| `agent_uptime_secs` | int64 | Yes | Agent process uptime in seconds; useful for detecting crashes and restarts |

**`agent_status` object:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `agent_version` | string | Yes | Version string of the running agent binary |
| `config_version` | int | Yes | Version of the configuration currently applied |
| `is_running` | bool | Yes | Whether the agent's check loop is currently active |
| `started_at` | string (RFC3339) | Yes | Timestamp when the agent process was started |
| `stopped_at` | string (RFC3339) \| null | Yes | Timestamp when the agent was stopped; null if still running |
| `checks_performed` | int | Yes | Total checks performed since agent start |
| `checks_successful` | int | Yes | Number of checks that completed successfully |
| `checks_failed` | int | Yes | Number of checks that failed |
| `reported_at` | string (RFC3339) | Yes | Timestamp of the last report sent by the agent |
| `failed_report_count` | int | Yes | Number of consecutive failed report attempts |
| `server_connected` | bool | Yes | Whether the agent currently has a live connection to the server |
| `cache_stats` | object | Yes | Local result cache statistics |

**`cache_stats` object:**

| Field | Type | Description |
|-------|------|-------------|
| `capacity` | int | Maximum number of results the cache can hold |
| `len` | int | Number of results currently buffered |

### Storage

All heartbeat fields are stored in the `agent_vitals` time-series table alongside the system metrics. This allows historical queries to correlate resource usage with agent operational state.

### Response

**204 No Content** — heartbeat accepted.  
**400 Bad Request** — missing or invalid body.  
**401 Unauthorized** — missing or invalid API key.  
**503 Service Unavailable** — database error or agent ID mismatch.

### Prometheus Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `smotra_heartbeat_attempts_total` | counter | Total heartbeat submissions |
| `smotra_heartbeat_success_total` | counter | Heartbeats processed successfully |
| `smotra_heartbeat_failure_total` | counter | Heartbeats rejected (auth or DB error) |
| `smotra_heartbeat_vitals_stored_total` | counter | Vitals snapshots written to database |
