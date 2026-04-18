---
applyTo: "internal/handlers/**"
---

# Metrics and Observability Guidelines

## Requirement

Every new HTTP endpoint or significant feature must expose appropriate Prometheus metrics.

## Naming Convention

```
smotra_<component>_<metric>_<unit>
```

- Counter metrics (monotonically increasing) end with `_total`
- Gauges for values that can go up or down
- Histograms/gauges for durations

## Implementation Pattern

Use `atomic.Uint64` for concurrent-safe counter fields in handler structs:

```go
// In handler struct
myFeatureRequestsTotal   atomic.Uint64
myFeatureRequestsSuccess atomic.Uint64
myFeatureRequestsFailure atomic.Uint64

// In buildPrometheusMetrics
output += "# HELP smotra_myfeature_requests_total Total requests\n"
output += "# TYPE smotra_myfeature_requests_total counter\n"
output += fmt.Sprintf("smotra_myfeature_requests_total %d\n", h.myFeatureRequestsTotal.Load())
```

## Required Metrics per New Endpoint

1. **Request counts**: total, success, failure
2. **Response times**: histogram or gauge
3. **Error rates by type** (where applicable)

## Required Metrics for DB Operations

- Query counts by operation type
- Query duration
- Connection pool statistics
- Health status

## Business Metrics to Track

- Agent registration/deregistration counts
- Check execution statistics
- Alert trigger counts
- Data ingestion rates

## Metrics Handler Location

`internal/handlers/metrics/metrics.go` — `buildPrometheusMetrics` method aggregates all metrics from all handlers and formats them in Prometheus exposition format.

## Testing

Tests must verify:
- Counters increment correctly on each code path
- Prometheus output includes correct HELP, TYPE, and value lines
- Concurrent increments are safe (atomic operations)
