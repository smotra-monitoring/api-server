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
- API versioning implementet via URL path (e.g., /api/v1/).

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
The server component is implemented in Go, leveraging its strong concurrency model and performance characteristics. It utilizes the chi for handling HTTP requests and routing. 

Database access must be implemented via interface abstractions to allow easy swapping of database backends.
- Production DB is PostgreSQL database enhanced with TimescaleDB for efficient time-series data storage and retrieval.
- Development and testing can use SQLite for simplicity.
- Database schema is managed using a migration tool go-migrate.

Server repository structure must follow standard Go project layout conventions, with clear separation of concerns between packages for handlers, services, models, and utilities.

oapi-codegen is used to generate server stubs and models from OpenAPI specifications, ensuring consistency between API documentation and implementation. 
- internal/api contains the generated code.
- cmd/server contains the main application entry point.
- OpenAPI specification is maintained in a separate repository (smotra-monitoring/openapi) and fetched during code generation.

The server must implement robust error handling and logging using a structured logging library slog. Configuration management should be handled via environment variables and configuration files, with support for different environments (development, staging, production).

Codebase must include unit tests and integration tests to ensure reliability and facilitate future development. CI/CD pipelines should be set up to automate testing, building, and deployment processes.

## Metrics and Observability

The server exposes a `/metrics` endpoint in Prometheus format for monitoring and observability. When implementing new features or modifying existing code, developers must consider and implement appropriate metrics:

### Metrics Guidelines

1. **Handler Metrics**: All new HTTP endpoints should track:
   - Request counts (total, success, failure)
   - Response times (histograms or gauges)
   - Error rates by type

2. **Database Metrics**: Database operations should expose:
   - Query counts (by operation type)
   - Query duration
   - Connection pool statistics
   - Health status

3. **Business Metrics**: Feature-specific metrics should include:
   - Agent registration/deregistration counts
   - Check execution statistics
   - Alert trigger counts
   - Data ingestion rates

4. **System Metrics**: Runtime metrics are automatically collected:
   - Go runtime statistics (goroutines, memory, GC)
   - CPU and memory usage
   - Uptime

### Metrics Implementation

The metrics handler is located in `internal/handlers/metrics/` and follows these patterns:

- Use atomic counters (`atomic.Uint64`) for concurrent-safe metric updates
- Expose metrics in Prometheus exposition format with proper HELP and TYPE comments
- Include relevant labels for dimensional metrics (e.g., status="success")
- Metrics names should follow the pattern: `smotra_<component>_<metric>_<unit>`
- Counter metrics should end with `_total` suffix
- Use gauges for values that can go up or down
- Use counters for monotonically increasing values

### Adding New Metrics

When adding features:

1. Identify what should be measured (requests, operations, resources)
2. Add metric fields to the relevant handler struct using `atomic.Uint64` for counters
3. Add increment methods that are thread-safe
4. Update the `buildPrometheusMetrics` method to expose the new metrics
5. Write tests to verify metrics are correctly incremented and formatted
6. Document the metrics in the OpenAPI spec example for `/metrics`

Example:
```go
// In handler struct
myFeatureOperationsTotal   atomic.Uint64
myFeatureOperationsSuccess atomic.Uint64
myFeatureOperationsFailure atomic.Uint64

// In buildPrometheusMetrics
output += "# HELP smotra_myfeature_operations_total Total operations\n"
output += "# TYPE smotra_myfeature_operations_total counter\n"
output += fmt.Sprintf("smotra_myfeature_operations_total %d\n", h.myFeatureOperationsTotal.Load())
```

[README.md](/README.md) describing server setup and development process

