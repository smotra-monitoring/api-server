---
applyTo: "internal/config/**,configs/**,internal/testutil/config.go"
---

# Configuration Management Guidelines

## Core Rule

**All configurable values must be in the configuration system — not as constants in source code.**

## Config Structure

```go
type Config struct {
    Server   ServerConfig   // HTTP server settings
    Database DatabaseConfig // Database connection settings
    Logging  LoggingConfig  // Logging configuration
    Auth     AuthConfig     // Authentication settings
    Agent    AgentConfig    // Agent-specific settings
}
```

Config package: `internal/config/`
- `types.go` — struct definitions
- `config.go` — loading, `Default()`, `Validate()`

## What Belongs in Config (not constants)

- **Timeouts and intervals**: HTTP timeouts, polling intervals, expiration times, retry delays
- **URLs and endpoints**: Server URLs, external service endpoints, callback URLs
- **Limits and thresholds**: Rate limits, buffer sizes, maximum counts
- **Security settings**: Token expiration, API key lengths, password requirements
- **Feature flags**: Enable/disable features per environment
- **Resource limits**: Connection pool sizes, worker counts

## What Can Stay as Constants

- HTTP status codes and standard header names
- Date/time format strings and immutable regex patterns
- Application-specific error code strings
- Fixed validation patterns (e.g., UUID format, hash lengths)

## Example

```go
// INCORRECT — hardcoded in handler
const claimTokenExpirationHours = 24

// CORRECT — in internal/config/types.go
type AgentConfig struct {
    ClaimTokenExpirationHours int    `json:"claim_token_expiration_hours" yaml:"claim_token_expiration_hours"`
    ServerURL                 string `json:"server_url" yaml:"server_url"`
}

// Handler usage
expiresAt := time.Now().Add(time.Duration(h.config.Agent.ClaimTokenExpirationHours) * time.Hour)
```

## Adding New Config Values — Checklist

1. Add field to the appropriate struct in `internal/config/types.go`
2. Add validation in `Validate()` if needed
3. Add default in `Default*Config()` function
4. Update `configs/dev.yaml` and `configs/prod.yaml`
5. Update test config in `internal/testutil/config.go`
6. Update any existing test fixtures
7. Document the field with a comment
