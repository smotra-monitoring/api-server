---
applyTo: "internal/middleware/**,internal/handlers/**"
---

# Authentication Guidelines

## Overview

Authentication is a **two-tier system**:

1. **Middleware** (`internal/middleware/auth.go`) — populates `AuthInfo` into the request context
2. **Wrapper** (`internal/handlers/authenticated_handler.go`) — enforces auth requirements before delegating to the actual handler

**Rule: Auth checks MUST live in `AuthenticatedHandler` wrappers — never inside handler bodies.**
Handler bodies can safely cast `ctx.Value(middleware.AuthContextKey).(*middleware.AuthInfo)` without nil/ok guards because the wrapper guarantees a valid, authenticated `AuthInfo` is present.

## AuthInfo Struct

```go
type AuthInfo struct {
    // Common fields
    AuthType      string // "agent_api_key" or "oauth2"
    Authenticated bool

    // Agent-specific fields (empty for OAuth2)
    AgentID string

    // OAuth2-specific fields (empty for agent_api_key)
    UserID    string // resolved user ID from session
    SessionID string // server-managed session ID
    Provider  string // IDP provider name (e.g. "google", "github")
}
```

Context key: `middleware.AuthContextKey`

---

## Two Authentication Types

### 1. Agent API Key (`AuthType == "agent_api_key"`)

- Header: `X-Agent-API-Key`
- Middleware: `middleware.AgentAPIKeyAuth(log, db)`
- Stored as SHA-256 hash only; compared via `crypto/subtle.ConstantTimeCompare`
- Keys are never logged or returned in responses after initial delivery

**Wrapper pattern for agent endpoints** — check `Authenticated`, then verify `AgentID` matches the URL parameter:

```go
func (h *AuthenticatedHandler) MyAgentEndpoint(ctx context.Context, request api.MyRequestObject) (api.MyResponseObject, error) {
    h.authAttemptsTotal.Add(1)

    authInfo := ctx.Value(middleware.AuthContextKey)
    if authInfo == nil {
        h.authNoAuthTotal.Add(1)
        return api.MyEndpoint401JSONResponse{...}, nil
    }
    ctxInfo, ok := authInfo.(*middleware.AuthInfo)
    if !ok || !ctxInfo.Authenticated {
        h.authInvalidTotal.Add(1)
        return api.MyEndpoint401JSONResponse{...}, nil
    }
    if ctxInfo.AgentID != request.AgentId.String() {
        h.authAgentIDMismatchTotal.Add(1)
        return api.MyEndpoint503JSONResponse{...}, nil // return 503 not 403 to avoid agent ID enumeration
    }

    h.authSuccessTotal.Add(1)
    return h.APIHandler.MyAgentEndpoint(ctx, request)
}
```

### 2. OAuth2 Session (`AuthType == "oauth2"`)

- Header: `Authorization: Bearer <opaque_token>`
- Middleware: `middleware.OAuth2Auth(log, db)`
- Opaque tokens are SHA-256 hashed at rest in the `sessions` table
- Session expiry: sliding 7-day window, 90-day absolute maximum

**Wrapper pattern for OAuth2 user endpoints** — check `Authenticated` and `AuthType == "oauth2"`:

```go
func (h *AuthenticatedHandler) MyUserEndpoint(ctx context.Context, request api.MyRequestObject) (api.MyResponseObject, error) {
    h.authAttemptsTotal.Add(1)

    authInfo, ok := ctx.Value(middleware.AuthContextKey).(*middleware.AuthInfo)
    if !ok || authInfo == nil || !authInfo.Authenticated {
        h.authNoAuthTotal.Add(1)
        return api.MyEndpoint401JSONResponse{...}, nil
    }
    if authInfo.AuthType != "oauth2" {
        h.authInvalidTotal.Add(1)
        return api.MyEndpoint401JSONResponse{...}, nil
    }

    h.authSuccessTotal.Add(1)
    return h.APIHandler.MyUserEndpoint(ctx, request)
}
```

**Inside the handler body**, auth is guaranteed — use a direct cast:

```go
func (h *Handler) MyUserEndpoint(ctx context.Context, _ api.MyRequestObject) (api.MyResponseObject, error) {
    // Auth guaranteed by AuthenticatedHandler wrapper — direct cast is safe.
    authInfo := ctx.Value(middleware.AuthContextKey).(*middleware.AuthInfo)
    // Use authInfo.UserID, authInfo.SessionID, authInfo.Provider freely.
}
```

---

## Middleware Registration

Both middleware functions must be registered on the chi router **before** any route handler that requires auth:

```go
r.Use(middleware.AgentAPIKeyAuth(log, db))  // for agent endpoints
r.Use(middleware.OAuth2Auth(log, db))       // for OAuth2 user endpoints
```

---

## Agent Claiming Workflow (Three-Phase Onboarding)

### Phase 1 — Agent Self-Registration (`POST /v1/agent/register`)
- Agent generates UUIDv7 ID and cryptographically secure claim token (64+ chars)
- Sends registration with hostname, version, and SHA-256 hashed token
- Server stores claim in `agent_claims` table with expiration
- Returns poll URL and claim URL to agent

### Phase 2 — Administrator Claiming (`POST /v1/agent/claim`)
- Admin reviews pending agents in web UI
- Provides claim token, section ID, optional agent name
- Server validates token, creates agent in `agents` table
- Generates API key, stores plaintext temporarily for one-time delivery

### Phase 3 — API Key Delivery (`GET /v1/agent/{agentId}/claim-status`)
- Agent polls until claimed
- First poll after claiming returns API key (one-time only)
- Plaintext key cleared immediately after delivery
- Subsequent polls return pending status
- Agent saves key locally and begins authenticated operation

## Security Constraints

- Claim tokens: 64+ character random, SHA-256 hashed, time-limited
- API keys: 32+ byte random, one-time plaintext delivery, SHA-256 stored
- Opaque session tokens: `st_live_`/`st_test_` prefix + 32 bytes `crypto/rand` → hex; SHA-256 hash stored in DB

---

## OAuth2 / OIDC Flow

Handler: `internal/handlers/auth/` — `Handler` struct with `NewHandler()` and `NewHandlerForTesting()` constructors.

**`NewHandlerForTesting()`** disables SSRF validation (the `allowPrivateHosts` flag) so tests can use local HTTP test servers. Never use it outside tests.

The server acts as a **CORS-safe relay** — all provider credentials are server-side only. Implementation is **PKCE-only**; no `client_secret` is used anywhere.

### Endpoint Resolution

The `endpointResolver` in `internal/handlers/auth/discovery.go`:

- **`type: oidc`** — fetches `{issuerURL}/.well-known/openid-configuration` and caches the result
- **`type: static`** — uses endpoints directly from config (required for GitHub and other non-OIDC providers)

Built-in provider defaults are defined in the `defaultProviders` map in `auth.go`. Server-config values override them.

### SSRF Protection

`url_validator.go` blocks requests to private/loopback IP ranges for all IDP endpoint URLs resolved at runtime.

### OAuth2 Endpoints

| Endpoint | Handler method | Requires auth |
|---|---|---|
| `GET /v1/auth/oauth2/authorize` | `Oauth2Authorize` | No |
| `GET /v1/auth/oauth2/callback` | `Oauth2Callback` | No |
| `POST /v1/auth/oauth2/token` | `Oauth2Token` | No |
| `POST /v1/auth/oauth2/revoke` | `Oauth2Revoke` | **Yes** (oauth2) |
| `GET /v1/auth/userinfo` | `GetUserInfo` | **Yes** (oauth2) |
| `POST /v1/auth/refresh` | `AuthRefresh` | **Yes** (oauth2, SessionID required) |
| `POST /v1/auth/logout` | `Logout` | **Yes** (oauth2) |

### Session Management

- `sessions` table: stores `token_hash`, `user_id`, `oauth2_*` IDP tokens, `expires_at`, `absolute_expires_at`
- `oauth2_pending_states` table: temporary records for in-flight auth codes (10-minute TTL)
- `Oauth2Token` creates a session; `AuthRefresh` rotates (revoke-old + create-new); `Logout` and `Oauth2Revoke` revoke sessions

See [docs/features/authentication.md](../../docs/features/authentication.md) for the full configuration reference.

---

## Adding a New Protected Endpoint

1. **Add a wrapper method** in `authenticated_handler.go` following the pattern above for the appropriate auth type (agent or oauth2).
2. **Do NOT add auth checks** inside the handler body in `internal/handlers/auth/` or other handler packages.
3. **Update tests**: add a test in `authenticated_handler_test.go` (or `authenticated_handler_integration_test.go`) covering the unauthenticated path via the wrapper.

