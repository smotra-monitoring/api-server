---
applyTo: "internal/middleware/**,internal/handlers/**"
---

# Authentication Guidelines

## Overview

Authentication is handled through middleware and authenticated handler wrappers:
- `internal/middleware/auth.go` — Agent API key middleware
- `internal/handlers/authenticated_handler.go` — Wrapper for protected endpoints

## Authentication Flow

1. Agent passes API key via `X-Agent-API-Key` header
2. Middleware validates key against SHA-256 hash stored in DB using constant-time comparison
3. On success, `AuthInfo` is injected into the request context
4. Protected endpoints use `AuthenticatedHandler` wrapper to verify authentication
5. Authenticated agent ID must match the requested agent ID in the URL

## Authentication Context

```go
type AuthInfo struct {
    AgentID       string
    AuthType      string // "agent_api_key" or "oauth2"
    Authenticated bool
    BearerToken   string // raw "Authorization: Bearer <token>" value (OAuth2 only; not yet validated)
}
```

Context key: `AuthContextKey`

## API Key Security

- Stored as SHA-256 hash only — plaintext never persisted after delivery
- Comparison via `crypto/subtle.ConstantTimeCompare` to prevent timing attacks
- Keys are never logged or exposed in responses
- DB column: `api_key_hash`

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
- Rate limiting recommended for registration and polling endpoints

## OAuth2 Relay Implementation

The server acts as a **CORS-safe stateless relay** for OAuth2/OIDC flows. All provider credentials are held server-side — browsers never see them. Implementation is **PKCE-only**; no `client_secret` is used anywhere.

Handler: `internal/handlers/auth/` — `Handler` struct with `NewHandler()` and `NewHandlerForTesting()` constructors.

**`NewHandlerForTesting()`** disables SSRF validation (the `allowPrivateHosts` flag) so tests can use local HTTP test servers without triggering the IP-range block. Never use it in production code.

### Endpoint Resolution

The `endpointResolver` in `internal/handlers/auth/discovery.go` resolves provider endpoints:

- **`type: oidc`** — fetches `{issuerURL}/.well-known/openid-configuration` and caches the result
- **`type: static`** — uses endpoints directly from config (required for GitHub and other non-OIDC providers)

Built-in provider defaults are defined in the `defaultProviders` map in `auth.go`. Server-config values override them.

### SSRF Protection

The `url_validator.go` file blocks requests to private/loopback IP ranges to prevent server-side request forgery through attacker-controlled provider URLs. This check is applied to all IDP endpoint URLs resolved at runtime.

### OAuth2 Flow

1. `GET /v1/auth/oauth2/authorize` — resolve provider, build IDP auth URL with PKCE params, return `302`
2. `GET /v1/auth/oauth2/callback` — relay code/error back to the frontend callback URL (fixed in config — not overridable by caller)
3. `POST /v1/auth/oauth2/token` — proxy token request to IDP, injecting `client_id` from server config
4. `POST /v1/auth/oauth2/revoke` — proxy revocation; no-op with `warning` for providers without revocation (e.g. GitHub)
5. `GET /v1/auth/userinfo` — proxy to IDP userinfo endpoint, forwarding `Authorization: Bearer` header
6. `POST /v1/auth/logout` — redirect to IDP end-session if supported; `200` otherwise

See [docs/features/authentication.md](../../docs/features/authentication.md) for the full configuration reference.

## Future Authentication

- **OAuth2 user context extraction** — bearer tokens received by admin endpoints (`/v1/agent/claim`, etc.) are stored in `AuthInfo.BearerToken` but not yet validated. When implemented, admin endpoints will call the userinfo endpoint to establish user identity and tenant membership before processing requests.
- JWT tokens for web interface
- RBAC for different user types
