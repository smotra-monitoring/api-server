---
applyTo: "internal/api/**,internal/handlers/**,api/**,cmd/server/**"
---

# API Code Generation and Handler Guidelines

## Dual Code Generation Strategy

The server uses two oapi-codegen configurations to separate endpoint types. OpenAPI spec is maintained in the separate `smotra-monitoring/openapi` repository.

**Generation command**: `just generate-oapi` (runs both configs)

### Generated Packages

1. **Root-Level Endpoints** (`internal/api/health/`)
   - Config: `api/oapi-codegen-root.yaml`
   - Package: `api_health`
   - Filter: `include-tags: [health]`
   - Endpoints: `/healthz`, `/healthz/ready`, `/healthz/live`, `/metrics`
   - Authentication: None required
   - Handler: `internal/handlers/health_handlers.go` → `HealthHandler`

2. **Versioned API Endpoints** (`internal/api/v1/`)
   - Config: `api/oapi-codegen-prefixed.yaml`
   - Package: `api_v1`
   - Filter: `include-tags: [current], exclude-tags: [health]`
   - Endpoints: `/v1/agent/*`, etc.
   - Authentication: Required (Agent API key or OAuth2)
   - Handler: `internal/handlers/api_handlers.go` → `APIHandler`

### Routing Structure (`cmd/server/main.go`)

```go
// Root-level endpoints (no prefix)
healthHandler := handlers.NewHealthHandler(...)
healthStrictHandler := healthAPI.NewStrictHandler(healthHandler, nil)
healthAPI.HandlerFromMux(healthStrictHandler, r)  // Register at /

// Versioned API endpoints (/v1 prefix)
apiHandler := handlers.NewAuthenticatedHandler(...)
apiStrictHandler := api.NewStrictHandler(apiHandler, nil)
r.Route("/v1", func(r chi.Router) {
    api.HandlerFromMux(apiStrictHandler, r)
})
```

## Error Handling

**All HTTP error responses must use the Strict types generated from the OpenAPI spec. Never return inline JSON.**

```go
// CORRECT
errorResponse := api.Error{
    Message: "Agent not found",
    Code:    "AGENT_NOT_FOUND",
}
w.WriteHeader(http.StatusNotFound)
json.NewEncoder(w).Encode(errorResponse)

// INCORRECT — never do this
w.WriteHeader(http.StatusNotFound)
w.Write([]byte(`{"error": "Agent not found"}`))
```

## Handler Organization

Location: `internal/handlers/`

**Root-Level Handlers** (`health_handlers.go`):
- `health/` — `/healthz`, `/healthz/ready`, `/healthz/live`
- `metrics/` — `/metrics`
- No authentication; used by Kubernetes probes and Prometheus

**Versioned API Handlers** (`api_handlers.go`):
- `agent_configuration/` — `GET /v1/agent/{agentId}/configuration`
- `agent_register/` — `POST /v1/agent/register`
- `agent_claim_status/` — `GET /v1/agent/{agentId}/claim-status`
- `agent_claim/` — `POST /v1/agent/claim`
- `agent_submit_results/` — `POST /v1/agent/{agentId}/results`
- `agent_heartbeat/` — `POST /v1/agent/{agentId}/heartbeat`
- `auth/` — OAuth2 relay (`GET /v1/auth/oauth2/authorize`, `GET /v1/auth/oauth2/callback`, `POST /v1/auth/oauth2/token`, `POST /v1/auth/oauth2/revoke`, `GET /v1/auth/userinfo`, `POST /v1/auth/logout`)

Note: `auth/` handlers are registered through `APIHandler` but do **not** require authentication themselves — they are the mechanism by which users authenticate. They must **not** be wrapped with `AuthenticatedHandler` checks.

**Handler files per package:**
- `<name>.go` — implementation
- `<name>_test.go` — unit tests
- `<name>_integration_test.go` — integration tests

Each handler tracks metrics using `atomic.Uint64` counters (see metrics.instructions.md).

## Middleware Package (`internal/middleware`)

- **middleware.go**: `RequestID`, `Logger`, `Recovery`, `CORS`, `responseWriter`
- **auth.go**: `AgentAPIKeyAuth` — extracts key from `X-Agent-API-Key` header, validates against hashed DB value, injects auth info into context

All middleware must have unit tests and integration tests.

## Route Separation Tests

`routes_separation_integration_test.go` verifies:
- Health endpoints exist only at root (no `/v1` prefix)
- API endpoints exist only under `/v1`
- No duplicate route registrations
