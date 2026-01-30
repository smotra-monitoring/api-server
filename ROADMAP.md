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

### Bugfixes that are part of a current PR
- [ ] Implement rate-limiting for endpoints that are using security schema AgentApiKey

### Short Term
- [ ] Database migrations management with go-migrate or similar tool
- [ ] JWT authentication for web interface
- [ ] User management endpoints
- [ ] Agent registration and management
- [ ] Docker and docker-compose setup

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

