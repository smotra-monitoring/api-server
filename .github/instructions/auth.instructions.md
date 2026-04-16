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

## Future Authentication

- OAuth2 (infrastructure partially in place)
- JWT tokens for web interface
- RBAC for different user types
