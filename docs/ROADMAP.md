## Roadmap

### Current PR
- [ ] SQL schema. What is the purpose of target_address and target_port in the check_results table? We can get them from the endpoint FK. If we keep them, we need to make sure they are consistent with the endpoint data. check_results.endpoint_id is nullable - why ? Explain the reasoning behind this design choice. If endpoint_id wouldn't be nullable, we can remove target_address and target_port fields from check_results table, as they can be obtained from the endpoints table using the endpoint_id foreign key. This would help to reduce data redundancy and ensure data consistency. 
- [ ] resolved_ip is missed in http_get_check_results. Is this on purpose? If not, we need to add it and make sure it is populated correctly in the code.
- [ ] resolved_ip renamed to address in tracerout_hops table. We should standardize on one naming convention for this field across all tables to avoid confusion.
- [ ] Inconsistency in errors_json vs error field in check results tables. We should standardize on one approach for storing error information. Is it a good idea to encode field type into field name? Though in Plugin there is field data_json. So maybe we can have errors_json in all tables for consistency, even if it is just a JSON object with a single "error" field for now. This will allow for more flexibility in the future if we want to add additional error details without changing the schema again. 
- [ ] Replace `UPDATE agents SET last_seen_at = ? WHERE id = ?;` in agent.sql (lines 44-45) with DB-trigger. This will ensure that last_seen_at is always updated whenever an agent interacts with the database, without relying on application code to do it correctly. This trigger should be used when agent submits results, but also when it fetches its configuration.  
- [ ] Endpoint could be monitoring multiple ports on the same address (for example: HTTP and HTTPS). In this case, we need to add port field to the `endpoints.sql` query `SELECT id FROM endpoints WHERE agent_id = ? AND address = ? LIMIT 1;` and make sure it is populated correctly in the code. 
- [ ] Add performance benchmarks on submit_results handler to identify potential bottlenecks and optimize database interactions. This will help ensure that the system can handle high volumes of monitoring data efficiently.
- [ ] Make duplicates_skipped in OpenAPI spec required. This will ensure that the client is aware of how many duplicate results were skipped during submission, which can be useful for monitoring and debugging purposes. Regenerate OpenAPI spec and update copilot-instructions.md accordingly.


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
- [x] Monitoring results submission endpoint (POST /agent/{agentId}/results)
  - [x] Batch ingestion with client-assigned UUIDv7 IDs for idempotent dedup
  - [x] Support for ping, httpget, tcpconnect, udpconnect, traceroute, plugin check types
  - [x] Separate `traceroute_hops` table for per-hop analytics
  - [x] Nullable endpoint FK resolved at insert time by agent+address lookup
  - [x] `last_seen_at` updated on agent after each accepted batch
  - [x] Prometheus metrics for submission attempts, success, failure, accepted, duplicates
  - [x] Authentication: agent must authenticate with matching agent ID
  - [x] Unit tests (6) and integration tests (6)

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

