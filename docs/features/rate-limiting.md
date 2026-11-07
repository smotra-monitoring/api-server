# Rate Limiting

The server asks an external, low-latency rate-limiter service whether a client IP may proceed. The check is sequential: the request waits (at most `timeout`) for the verdict before any authentication or handler work runs. This protects the database and handlers from floods and simple brute-force attempts.

## How it works

```
request -> ClientInfoMiddleware (resolve IP) -> RateLimit -> auth middleware -> handler
```

For every request the `RateLimit` middleware (`internal/middleware/ratelimit.go`) does the following, in order:

1. Path starts with an `exempt_paths` prefix: skip the check.
2. Client IP is in `whitelist_subnets`: skip the check.
3. IP has a cached 429 (see `cache_429_sec`): respond 429 without calling the limiter.
4. Circuit breaker is open: skip the limiter and apply `fail_open`.
5. Call the limiter. 200 allows the request, 429 rejects it, anything else (error, timeout, other status) is a failure.

A failure opens the circuit breaker for `circuit_breaker_sec`, then `fail_open` decides: `true` lets the request through, `false` returns 503.

Allowed decisions are never cached, so the limiter sees every allowed request and keeps accurate counters.

## Limiter protocol

```
GET <url>
Content-Type: application/json

{"ip":"203.0.113.5"}
```

| Response | Meaning |
|---|---|
| `200` | Allow |
| `429` | Deny |
| other / timeout | Failure (see above) |

The client uses keep-alive connections. The timeout covers the whole call.

## Configuration

```yaml
rate_limiter:
    enabled: true
    url: http://rate-limiter.internal:9000/check
    timeout: 5ms
    fail_open: true
    cache_429_sec: 15
    circuit_breaker_sec: 60
    exempt_paths: ["/healthz", "/metrics"]
    whitelist_subnets: ["10.0.0.0/8"]
```

| Key | Default | Validation (when enabled) | Description |
|---|---|---|---|
| `enabled` | `false` | | Master switch. Other keys are not validated when `false`. |
| `url` | none | http(s) URL | Limiter endpoint. |
| `timeout` | `5ms` | 1ms to 20ms | Per-call timeout. |
| `fail_open` | `true` | | Allow requests when the limiter fails; otherwise respond 503. |
| `cache_429_sec` | `15` | at least 5 | How long a denied IP is rejected locally without calling the limiter. Also the `Retry-After` value. |
| `circuit_breaker_sec` | `60` | at least 5 | How long the limiter is skipped after a failed call. |
| `exempt_paths` | `/healthz`, `/metrics` | must start with `/` | Path prefixes that bypass the limiter. |
| `whitelist_subnets` | empty | CIDR or IP | Clients in these ranges bypass the limiter. |

Omitted keys keep the defaults above.

## Responses

- **429** `{"error":"too_many_requests","message":"Too many requests","request_id":...}` with a `Retry-After` header (seconds).
- **503** `{"error":"service_unavailable",...}` when the limiter failed and `fail_open` is `false`.

## Security considerations

- The client IP comes from `ClientInfoMiddleware`. `X-Forwarded-For` is honoured only when the direct peer is in `server.trusted_proxies`, so an untrusted client cannot claim a whitelisted address. Configure `trusted_proxies` correctly when running behind a proxy; otherwise every request appears to come from the proxy.
- A single failed limiter call opens the breaker for `circuit_breaker_sec`, during which `fail_open` applies to all non-whitelisted traffic. With `fail_open: true` that means no rate limiting during that window.
- The local 429 cache holds at most 100,000 IPs; when full of live entries, further denied IPs are not cached and the limiter is consulted for them.

## Metrics

Exposed on `/metrics` (see [TESTING.md](../TESTING.md)):

| Metric | Description |
|---|---|
| `smotra_ratelimiter_checks_total` | Requests evaluated (excludes exempt and whitelisted) |
| `smotra_ratelimiter_allowed_total` | Allowed by the limiter service |
| `smotra_ratelimiter_denied_total` | Denied with 429, including cache hits |
| `smotra_ratelimiter_cache_hits_total` | Denials served from the local 429 cache |
| `smotra_ratelimiter_errors_total` | Failed or timed out limiter calls |
| `smotra_ratelimiter_breaker_skips_total` | Checks that skipped the limiter while the breaker was open |
| `smotra_ratelimiter_fail_open_total` | Requests allowed because the limiter was unavailable |
| `smotra_ratelimiter_whitelisted_total` | Requests that bypassed the limiter via `whitelist_subnets` |
| `smotra_ratelimiter_blocked_entries` | Gauge: IPs currently in the local 429 cache |
| `smotra_ratelimiter_blocked_utilization_ratio` | Gauge: local 429 cache fill ratio (0-1, capacity 100,000) |
| `smotra_ratelimiter_call_duration_seconds` | Summary (`_sum`, `_count`) of limiter call durations |

Suggested alerts: sustained growth of `errors_total` or `breaker_skips_total` (limiter unhealthy), and a high `denied_total` rate (attack in progress).

## Troubleshooting

- **No rate limiting applied**: check `rate_limiter.enabled` and look for the `rate limiter enabled` log line at startup.
- **Frequent breaker trips**: the limiter is slower than `timeout`. Check limiter latency, or raise `timeout` (max 20ms).
- **Legitimate users get 429 behind a proxy**: `server.trusted_proxies` is not set, so all clients share the proxy IP.
