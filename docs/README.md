# Smotra Server

A distributed monitoring system designed to track reachability and performance of agents installed on various hosts. The server collects data from deployed agents, provides real-time monitoring, alerting, and reporting capabilities.

## Features

- 🔍 **Agent-Based Monitoring** - Lightweight agents collect metrics and send them to the central server
- 📊 **Real-Time Metrics** - Prometheus-format metrics endpoint for monitoring and observability
- 🔐 **Secure Authentication** - Agent API key authentication with secure claiming workflow
- 🗄️ **Flexible Database** - SQLite for development, PostgreSQL with TimescaleDB for production
- 🏷️ **Multi-Tenant Support** - Hierarchical structure with tenants, sections, and agents
- 🌐 **RESTful API** - OpenAPI 3.0 specification with automatic code generation
- 📈 **Extensible Architecture** - Plugin support for custom monitoring capabilities

## Quick Start

### Prerequisites

- Go 1.23.4 or later
- SQLite (for development) or PostgreSQL 13+ with TimescaleDB (for production)

### Installation

```bash
# Clone the repository
git clone https://github.com/smotra-monitoring/server.git
cd server

# Install dependencies
go mod download

# Run with development configuration
go run cmd/server/main.go -c configs/dev.yaml
```

The server will start on `http://localhost:8080`

### Verify Installation

```bash
# Health check
curl http://localhost:8080/healthz

# View metrics
curl http://localhost:8080/metrics
```

## Agent Claiming Workflow

The server implements a secure three-phase workflow for agent onboarding:

1. **Agent Self-Registration** - Agent generates ID and claim token, registers with server
2. **Administrator Claiming** - Admin reviews and claims pending agents via web UI
3. **API Key Delivery** - Agent polls for claim status and receives API key one-time

See the [detailed guide](GUIDE.md#4-agent-claiming-workflow) for complete examples.

## Configuration

Configuration is managed via YAML or JSON files. Example configurations are provided:

- `configs/dev.yaml` - Development setup with SQLite
- `configs/prod.yaml` - Production template with PostgreSQL

```yaml
server:
  host: 0.0.0.0
  port: 8080
  environment: development

database_type: sqlite

sqlite_config:
  filepath: ./data/smotra.db

logging:
  level: debug
  format: json
```

For detailed configuration options, see the [Configuration Guide](GUIDE.md#configuration).

## Development

### Using justfile

The project includes a justfile for common development tasks:

```bash
just run              # Run server in development mode
just test             # Run tests
just test-coverage    # Run tests with coverage
just build            # Build production binary
just generate-oapi    # Regenerate API code from OpenAPI spec
just lint             # Run linters
just all              # Run all build steps
```

### Project Structure

```
server/
├── cmd/server/              # Main application entry point
├── configs/                 # Configuration files
├── internal/
│   ├── api/                # Generated API code (OpenAPI)
│   ├── config/             # Configuration management
│   ├── database/           # Database interface and implementations
│   ├── handlers/           # HTTP request handlers
│   ├── logger/             # Structured logging
│   ├── middleware/         # HTTP middleware (auth, logging, etc)
│   └── testutil/           # Testing utilities
├── data/                   # Database files (SQLite)
└── api/                    # OpenAPI configuration
```

For complete project structure, see [GUIDE.md](GUIDE.md#project-structure).

## API Documentation

The API is defined using OpenAPI 3.0 specification maintained in the [smotra-monitoring/openapi](https://github.com/smotra-monitoring/openapi) repository.

### Key Endpoints

- `GET /healthz` - Health check
- `GET /healthz/ready` - Readiness check (includes DB connectivity)
- `GET /healthz/live` - Liveness check
- `GET /metrics` - Prometheus metrics
- `GET /v1/agent/{agentId}/configuration` - Agent configuration (authenticated)
- `POST /v1/agent/register` - Agent self-registration
- `GET /v1/agents/{agentId}/claim-status` - Agent claim status polling
- `POST /v1/agents/claim` - Administrator claims agent

## Testing

```bash
# Run all tests
just test

# Run with coverage
just test-coverage

# Run specific test types
just test-unit              # Unit tests only
just test-integration       # Integration tests only
```

See [TESTING.md](TESTING.md) for detailed testing documentation.

## Building for Production

```bash
# Build binary
just build

# Or manually
go build -ldflags "-X main.version=1.0.0" -o bin/smotra-server cmd/server/main.go

# Run with production config
./bin/smotra-server -c configs/prod.yaml
```

## Technology Stack

- **Language**: Go 1.23.4
- **Router**: chi v5
- **Database**: PostgreSQL (with TimescaleDB) / SQLite
- **Query Generation**: sqlc (type-safe SQL to Go)
- **API Generation**: oapi-codegen (from OpenAPI spec)
- **Logging**: slog (standard library)

## Documentation

- [GUIDE.md](GUIDE.md) - Comprehensive setup and development guide
- [TESTING.md](TESTING.md) - Testing strategy and examples
- [ROADMAP.md](ROADMAP.md) - Planned features and improvements

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes and add tests
4. Run `just all` to verify everything passes
5. Commit your changes (`git commit -m 'Add amazing feature'`)
6. Push to the branch (`git push origin feature/amazing-feature`)
7. Open a Pull Request

## License

Source available with restrictions on SaaS usage without a commercial license. See the [LICENSE](../LICENSE) file for details.

## Support

- **Issues**: [GitHub Issues](https://github.com/smotra-monitoring/server/issues)
- **Documentation**: See [GUIDE.md](GUIDE.md) for detailed documentation

---

Built with ❤️ using [chi](https://github.com/go-chi/chi), [oapi-codegen](https://github.com/deepmap/oapi-codegen), and other excellent open-source libraries.
