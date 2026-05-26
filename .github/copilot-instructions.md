# Project description

This project is a distributed monitoring system designed to track reachability and performance of agents installed on various hosts. It consists of a central server that collects data from multiple agents deployed across different machines. The system provides real-time monitoring, alerting, and reporting capabilities to ensure the health and performance of the monitored infrastructure.

# Key Features
- **Agent-Based Monitoring**: Lightweight agents installed on hosts to collect metrics and send them to the central server.
- **Centralized Data Collection**: A server that aggregates data from all agents for analysis and reporting.
- **Real-Time Alerts**: Configurable alerts based on predefined thresholds to notify administrators of potential issues.
- **Performance Metrics**: Collection of various performance metrics such as reachability, response time and potentially other system metrics that can be extended via plugins.
- **Scalability**: Designed to handle a large number of agents and hosts efficiently.
- **Extensible Architecture**: Support for plugins to extend monitoring capabilities and integrate with other systems.
- **User-Friendly Interface**: A web-based dashboard for visualizing data, configuring agents, and managing alerts.
- **APIs for Integration**: RESTful APIs to allow integration with other systems and automation tools.

# Technologies Used
- Agent Development is in Rust for performance and safety.
- Server-side components are developed in Go.
- Data storage using a time-series database (PostgreSQL + TimescaleDB) for efficient metric storage and retrieval.
- Web interface built with standard web technologies (HTML, CSS, TypeScript) for a responsive user experience CSS framework (e.g. Bulima).
- Communication between agents and server using RESTful APIs over HTTP/HTTPS.
- Containerization using Docker for easy deployment and scalability.
- Orchestration using Kubernetes for managing deployments in a clustered environment.
- Monitoring and logging using Prometheus and Grafana for system health and performance visualization.
- Database is PostgreSQL with TimescaleDB extension for time-series data handling.
- Database scheme stored and managed using github repo with migrations handled by a tool like Flyway or Liquibase.

# Agent Capabilities
- Agents check reachability of other agents or predefined endpoints.
- Measure response times and log results.
- Send collected data to the central server at regular intervals.
- Support for custom plugins to extend monitoring functionality.
- Configuration management to adjust monitoring parameters remotely from the server. Must be able use local configuration if server is unreachable.
- Secure communication with the server using TLS/SSL.

Agent should be able to operate in a standalone mode if the server is unreachable, caching data locally and sending it once the connection is restored. Agents should also support auto-updates to ensure they are running the latest version. Agent use ICMP ping and traceroute for reachability checks, with options for TCP/UDP checks as plugins. 

Agent implementation should prioritize low resource usage to minimize impact on host performance. Therefore tokio async runtime is preferred for Rust implementation. 

# Server Capabilities
- Receive and store data from multiple agents.
- Provide a web-based dashboard for data visualization and management.
- Configure agents remotely, including setting monitoring intervals and thresholds.
- Generate reports based on collected data.
- Send alerts to Discord, email, or other notification systems when thresholds are breached.
- Provide RESTful APIs for data access and integration with other systems.
- Support user authentication and role-based access control for secure management.
- Implement data retention policies to manage storage usage effectively.
- Support for horizontal scaling to handle increased load as the number of agents grows.
- Server endpoints must be generated using OpenAPI/Swagger for easy integration and documentation.
- Authentication should use JWT tokens for API access and session management for web interface.
- User authentication should support OAuth2 for integration with existing identity providers.

# Endpoints
- RESTful API endpoints for agent data submission, configuration management, and data retrieval.
- WebSocket endpoints for real-time data updates to the dashboard.
- Authentication endpoints for user login and management.
- /metrics endpoint for Prometheus monitoring.
- /healthz endpoint for server status monitoring.
- API versioning implementet via URL path (e.g., /v1/).

# Deployment
- Use Docker for containerization of the server components.
- Use Kubernetes for orchestration and management of server deployments.
- Provide Helm charts for easy deployment in Kubernetes environments.
- Include CI/CD pipelines for automated testing, building, and deployment of both agents and server components.
- Provide documentation for installation, configuration, and usage of the system.

# Documentation
- Comprehensive documentation covering installation, configuration, usage, and troubleshooting.
- API documentation generated using OpenAPI/Swagger.
- Guides for developing custom plugins for agents.
- Best practices for deploying and scaling the system in production environments.

# Community and Support
- Encourage community contributions through GitHub.
- Provide support channels such as a discussion forum or Discord server for users to seek help and share knowledge.
- Regular updates and maintenance to ensure the system remains secure and up-to-date with the latest technologies
- Roadmap for future features and improvements based on user feedback and industry trends.

# Licensing
- Source available prohibiting SaaS usage without a commercial license.
- Use a permissive open-source license for non-SaaS usage (e.g., MIT, Apache 2.0).
- Clearly define terms for commercial usage and contributions.
- Include a CONTRIBUTING.md file to guide contributors on how to participate in the project.

# Frontend implementation
The web interface is built using standard web technologies (HTML, CSS, TypeScript) and employs a CSS framework like Bulma for responsive design. The server exposes RESTful APIs for agent communication and data retrieval, with endpoints documented using OpenAPI/Swagger.

# Server implementation
The server is implemented in Go using the chi router. HTTP framework: chi. Database: PostgreSQL + TimescaleDB (production), SQLite (dev/test). Database access via interface abstractions.

Server repository structure follows standard Go project layout. Use `slog` for structured logging. Configuration via environment variables and YAML config files.

Codebase must include unit tests and integration tests. CI/CD pipelines automate testing, building, and deployment.

**Detailed implementation rules** (auto-loaded by Copilot when editing relevant files):
- Database access, sqlc, schema, migrations → `.github/instructions/database.instructions.md`
- API codegen, routing, error handling, handlers → `.github/instructions/api.instructions.md`
- Authentication, API keys, agent claiming, OAuth2 session flow → `.github/instructions/auth.instructions.md`

**Critical auth rule**: Auth checks always live in `AuthenticatedHandler` wrapper methods (`internal/handlers/authenticated_handler.go`). Handler bodies in `internal/handlers/auth/` and other packages must **never** contain inline auth guards — they may cast `ctx.Value(middleware.AuthContextKey).(*middleware.AuthInfo)` directly since the wrapper guarantees auth.
- Configuration management, config structs → `.github/instructions/config.instructions.md`
- Metrics, Prometheus, observability → `.github/instructions/metrics.instructions.md`

Documentation is organized in the docs/ folder:
- **docs/README.md** - This is the GitHub repository homepage (no README.md in root folder)
- **docs/features/GUIDE.md** - Comprehensive server setup and development documentation
- **docs/TESTING.md** - Testing strategy and how to run tests
- **docs/ROADMAP.md** - Planned features and improvements for future releases

## Metrics implementation

Metrics **must always** be implemented when new features are added, according to the guidelines in `.github/instructions/metrics.instructions.md`. This includes:
- Defining relevant metrics for server performance, API usage, and agent interactions
- Implementing Prometheus instrumentation in the server code
- Exposing a /metrics endpoint for Prometheus scraping
- Documenting the metrics in docs/TESTING.md for monitoring and alerting purposes

## Testing Requirements

Tests **must always** be written or updated alongside code changes:

- **New features**: add unit tests and integration tests covering the new behaviour
- **Bug fixes**: add a regression test that fails without the fix before adding the fix
- Each handler package must include `<name>_test.go` (unit) and `<name>_integration_test.go` (integration)
- Tests live next to the code they test — no separate `tests/` top-level folder

## Documentation Guidelines

When implementing significant new features or workflows, they must be documented in the docs/ folder:

1. **New Features**: Update docs/features/ with:
   - Feature description and purpose
   - Configuration options
   - API endpoints (if applicable)
   - Usage examples
   - Integration instructions

2. **Complex Workflows**: Document multi-step processes with:
   - Step-by-step instructions
   - Example requests/responses
   - Security considerations
   - Error handling and troubleshooting

3. **Testing**: Update docs/TESTING.md when adding:
   - New testing patterns or utilities
   - Integration test requirements
   - Performance or load testing procedures

4. **Future Plans**: Update docs/ROADMAP.md for:
   - Planned features and improvements
   - Breaking changes
   - Deprecation notices

**Important**: Keep docs/README.md concise as it serves as the GitHub repository homepage. There should be no README.md file in the project root. Detailed documentation belongs in docs/features/GUIDE.md.
