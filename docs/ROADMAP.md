## Roadmap

### Implementation of cache check results
- [X] Update OpenAPI schema BatchMonitoringResult and then update the code and DB schema to support the change. The main change is that we will replace the `target` field to the endpoint_id mandatory field. This will allow us to have more structured data and avoid issues with unrecognized check types. The `target` should be removed from the OpenAPI schema and replaced with `endpoint_id` which will be a string representing the UUID of the endpoint. The code should be updated to validate the `endpoint_id` and ensure that it corresponds to an existing endpoint in the database. Additionally, the database schema should be updated to include an `endpoint_id` column in the `check_results` table, and appropriate foreign key constraints should be added to maintain data integrity.
- [X] OpenAPI spec Endpoint.id made required. Before it was optional. Check that `just generate-openapi` is working correctly and run go test to make sure there are no issues with the generated code. Update copilot-instructions.md to reflect this change and provide guidance on how to handle the required endpoint_id field when constructing monitoring results in the client code.
- [X] `resolved_ip` is missed in `check_results_http_get`. Is this on purpose? If not, we need to add it and make sure it is populated correctly in the code.
      Solution: resolved_ip is not needed due to http requests will be using reqwest library which will resolve the hostname by itself. Which means, if we will resolve it as well it might be resolved to different IPs if the hostname has multiple A records. 
- [X] Inconsistency in errors_json vs error field in check results tables. We should standardize on one approach for storing error information, even if it is just a JSON object with a single "error" field for now. This will allow for more flexibility in the future if we want to add additional error details without changing the schema again. Modify the OpenAPI schema to reflect this change and update the code accordingly to ensure that error information is stored consistently across all check results tables.
- [X] Replace `UPDATE agents SET last_seen_at = ? WHERE id = ?;` in agent.sql (lines 44-45) with DB-trigger. This will ensure that last_seen_at is always updated whenever an agent interacts with the database, without relying on application code to do it correctly. This trigger should be used when agent submits results, but also when it fetches its configuration.  
   Solution: DB trigger can't be fired on select, so when agent requesting config from server DB will not fire the update. So we will keep the update in the code explicitly. 
- [X] Endpoint could be monitoring multiple ports on the same address (for example: HTTP and HTTPS). In this case, we need to add port field to the `endpoints.sql` query `SELECT id FROM endpoints WHERE agent_id = ? AND address = ? LIMIT 1;` and make sure it is populated correctly in the code. 
   Solution: not relevant anymore, now wr are saving `endpoint_id` in the `monitoring_result`, so we don't need to query endpoint by agent_id and address anymore.
- [X] Add performance benchmarks on submit_results handler to identify potential bottlenecks and optimize database interactions. This will help ensure that the system can handle high volumes of monitoring data efficiently.
- [X] Make `duplicates_skipped` in OpenAPI `/agent/{agent_id}/results` response required. This will ensure that the client is aware of how many duplicate results were skipped during submission, which can be useful for monitoring and debugging purposes. Regenerate OpenAPI spec and update copilot-instructions.md accordingly.
   Solution: decided not to implement it, optional is just fine. 
- [X] What is the reasoning of having Average Response Time in the Ping response instead of just SuccessLatenciesArray ? This is a design choice that should be justified. If we keep Average Response Time, we need to make sure it is calculated correctly and consistently with the SuccessLatenciesArray. Alternatively, we could remove Average Response Time and let the client calculate it from the SuccessLatenciesArray if needed, which would simplify the schema and reduce potential sources of inconsistency.
- [X] Rename DB scheme so that all check_results tables have the same prefix (for example: check_results_ping, check_results_http_get, check_results_tcp_connect, etc.) for consistency and easier identification of related tables. 
- [X] traceroute_hops table rename to check_results_traceroute_hops for consistency with other check results tables. This will make it clear that this table is related to traceroute check results and follows the same naming convention as other check results tables, improving overall clarity and organization of the database schema.
- [X] traceroute_hops table has field named "address". For consistency with other tables, we should consider renaming it to "resolved_ip". This will help maintain a consistent naming convention across the database schema and reduce confusion when working with the data. Update all relevant code and OpenAPI schema to reflect this change, and make sure to test thoroughly to ensure that there are no issues with the renaming.
- [ ] BIGGIE !!! Separate endpoint from agent. Currently, endpoint is tightly coupled with agent through the `agent_id` foreign key. This means that if we want to monitor the same endpoint from multiple agents (for example: monitoring a web server from different geographic locations), we would need to create duplicate endpoint entries for each agent. This is not ideal and can lead to data redundancy and inconsistency. Instead, we should separate endpoint into its own table with a unique identifier (endpoint_id) and then create a many-to-many relationship between agents and endpoints through a junction table (for example: agent_endpoints). This way, we can have a single entry for each unique endpoint and associate it with multiple agents as needed, improving data integrity and reducing redundancy in the database schema.

- [X] Add `hostname` to the `endpoints` table. This will allow us to store the original hostname that the agent is monitoring, which can be useful for reporting and debugging purposes. The address field can still store the resolved IP address for efficient querying and analysis, while the hostname field preserves the original monitoring target information.
- [X] IMPORTANT!!! Revert previous change of adding `hostname` to the `endpoints` table. After further consideration, it may be better to 2`keep the `endpoints` object storing only `address` and specific checks will resolve IP if needed. For example:
    - For ping and tcp connect checks, we can resolve the IP address. And it will resolve to different IPs if the hostname has multiple A records, so it is better to store only `hostname` / `address`
    - HTTP GET checks will be using reqwest library which will resolve the hostname by itself, so it is better to store only `hostname` / `address`
    - For traceroute checks, it is better to store only `hostname` / `address` in the `endpoints` table and then resolve IPs for each hop in the `check

    
- [X] Brainstorm if we need to add `agent_id` to the checks.sql `SELECT id FROM check_results WHERE id = ? LIMIT 1;` query. This would allow us to ensure that the check result being queried belongs to the correct agent, which can enhance security and data integrity. However, it may also require additional changes to the codebase to pass the agent_id parameter through the necessary functions and handlers, so we need to carefully consider the trade-offs before implementing this change. And it will increase load on the database, so we need to make sure it is optimized properly (for example: by adding appropriate indexes).
     Answer: check_results.id is the Primary Key, it is impossible to get duplicate id from another agent. So it is not necessary to add agent_id to the query.
- [X] Table `check_results_traceroute_hops` rename `response_time_ms` to `success_latencies_json TEXT NOT NULL DEFAULT '[]'` and make it a JSON array of response times for each attempt, similar to how we store latencies in the ping check results. This will allow us to capture more detailed information about the traceroute hops, including variability in response times across multiple attempts, which can be valuable for performance analysis and troubleshooting.
This change will also require change in OpenAPI `TracerouteCheck.result.total_time_ms` field to `TracerouteCheck.result.latencies` and make it an array of latencies for each attempt. 

### Completed
- [x] SQLite and PostgreSQL database support with interface abstraction
- [x] Health check endpoints (health, ready, live)
- [x] Prometheus metrics endpoint
- [x] Refactored HTTP and DB metrics to `MetricsProvider` interface — `HTTPMetrics` middleware counts requests by status code, `DBMetrics` exposes `sql.DB.Stats()` pool metrics and db health; `db` dependency removed from `metrics.Handler`
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
  - [x] Benchmark tests (`submit_results_bench_test.go`)
- [x] Agent heartbeat endpoint (POST /agent/{agentId}/heartbeat)
  - [x] Stores CPU/memory vitals snapshot on every heartbeat
  - [x] Updates `last_seen_at` on the agent record
  - [x] Prometheus metrics (attempts, success, failure, vitals stored)
  - [x] Authentication: agent must authenticate with matching agent ID
  - [x] Unit tests and integration tests
- [x] OAuth2 / OIDC relay implementation
  - [x] GET /v1/auth/oauth2/authorize — PKCE authorization redirect
  - [x] GET /v1/auth/oauth2/callback — relay code/error to frontend
  - [x] POST /v1/auth/oauth2/token — proxy token exchange (injects client_id)
  - [x] POST /v1/auth/oauth2/revoke — proxy token revocation
  - [x] GET /v1/auth/userinfo — proxy userinfo request
  - [x] POST /v1/auth/logout — redirect to IDP end-session
  - [x] OIDC discovery for type=oidc providers
  - [x] Static endpoint config for type=static providers (GitHub)
  - [x] SSRF protection via IP-range validation
  - [x] Unit tests for all endpoints

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
- [x] Add metrics for agent_register, agent_claim_status, agent_claim. The way to go is to use RegisterMetricsProvider.

- [ ] Implement rate-limiting for endpoints that are using security schema AgentApiKey

### Short Term
- [ ] Web UI for claiming workflow (admin dashboard)
- [ ] Database migrations management with go-migrate or similar tool
- [ ] JWT authentication for web interface
- [ ] User management endpoints
- [ ] Docker and docker-compose setup
- [ ] Documentation for agent deployment process

### Medium Term
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

