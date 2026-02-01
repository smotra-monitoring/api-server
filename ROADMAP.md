## Roadmap

### Completed
- [x] SQLite and PostgreSQL database support with interface abstraction
- [x] Health check endpoints (health, ready, live)
- [x] Prometheus metrics endpoint
- [x] Structured logging with slog
- [x] Configuration management (YAML/JSON)
- [x] Middleware (logging, request ID, recovery, CORS)
- [x] OpenAPI-based code generation
- [x] Unit and integration testing infrastructure
- [x] Update copilot-instructions.md about sqlc generations and file-structure
- [x] Add GetTitle() to MetricsProvider interface and use it in metrics.buildPrometheusMetrics in output labels
- [x] Correct OpenAPI spec. Endpoint enabled is required
- [x] Double check OpenAPI spec. AgentConfig.tags is required AgentConfig.Endpoints.tags is optional
- [x] Correct DB schema. Add optional port to the Endpoint
- [x] Database schema with support for tenants, sections, agents, endpoints, and tags
- [x] sqlc integration for type-safe database queries
- [x] Agent configuration endpoint implementation (GET /agent/{agentId}/configuration)
- [x] Database versioning triggers for automatic configuration version bumping
- [x] justfile for build automation (replacing Makefile)
- [x] Update copilot-instruct to use Strict types from api package if HTTP error should be returned. Do not use inline JSON without api.Error
- [x] Check that existing code follows api.Error struct approach
- [x] Agent API key authentication middleware implementation
- [x] Authenticated handler wrapper for protected endpoints
- [x] UUIDv7 implementation for request IDs and primary keys
- [x] Agent self-registration workflow implementation
  - [x] POST /agent/register endpoint for agent self-registration
  - [x] GET /agents/{agentId}/claim-status endpoint for polling
  - [x] POST /agents/claim endpoint for administrator claiming
  - [x] Claim token generation and secure hashing
  - [x] API key generation and one-time delivery mechanism
  - [x] Database schema for agent_claims table with delivery tracking
  - [x] Unit tests for all claiming handlers (23 tests)
  - [x] Integration tests for complete workflow (13 tests)
- [x] Agent registration and management (Server side)

### Current Work
- [ ] Web UI for agent claiming workflow
- [ ] OAuth2 user context extraction for admin endpoints
- [ ] Rate limiting for agent registration and claim status polling

### Bugfixes that are part of a current PR
- [ ] Remove CreateAgent from agent.sql, it can be safely replaced by CreateAgentFromClaim. Then `just regenerate-sqlc` and fix tests. CreateAgent only used in tests. 

- [ ] Implement OAuth2 user context extraction
- [ ] After implementing OAuth2: In claim.go Handle: add check on SectionID. SectionID must belong to the same tenant as the user.
- [ ] After implementing OAuth2: In claim_integration_test.go. Find "TODO:....." and uncomment code lines to enable "user checks".

- [ ] Update copilot-instructions.md to add metrics to any new entities that might require it
- [ ] Add metrics for agent_register, agent_claim_status, agent_claim. The way to go is to use RegisterMetricsProvider.

- [ ] Implement rate-limiting for endpoints that are using security schema AgentApiKey

### Short Term
- [ ] Web UI for claiming workflow (admin dashboard)
- [ ] Database migrations management with go-migrate or similar tool
- [ ] JWT authentication for web interface
- [ ] User management endpoints
- [ ] Docker and docker-compose setup
- [ ] Documentation for agent deployment process

### Medium Term
- [ ] OAuth2 integration
- [ ] Metrics collection from agents
- [ ] Alert configuration and notification system
- [ ] Web dashboard (frontend)
- [ ] TimescaleDB integration for time-series data

### Long Term
- [ ] Kubernetes deployment with Helm charts
- [ ] Plugin system for extensibility
- [ ] Advanced monitoring features
- [ ] Distributed tracing
- [ ] Multi-tenant support

