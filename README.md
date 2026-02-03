# Smotra Monitoring Server

A distributed monitoring system designed to track reachability and performance of agents installed on various hosts.

## Prerequisites

- Go 1.23.4 or later
- For PostgreSQL: PostgreSQL 13+ with TimescaleDB extension (optional for production)
- For SQLite: No additional dependencies required (default for development)

## Quick Start

### 1. Clone and Setup

```bash
# Clone the repository
git clone https://github.com/smotra-monitoring/server.git
cd server

# Install dependencies
go mod download
```

### 2. Run with Development Configuration

The project includes a ready-to-use development configuration that uses SQLite:

```bash
# Run with the provided dev config
go run cmd/server/main.go -c configs/dev.yaml

# Or use justfile
just run
```

The server will start on `http://localhost:8080`

### 3. Test the Server

```bash
# Health check
curl http://localhost:8080/healthz

# Readiness check
curl http://localhost:8080/healthz/ready

# Liveness check
curl http://localhost:8080/healthz/live

# API info
curl http://localhost:8080/api/v1
```

### 4. Agent Claiming Workflow

The server implements a secure workflow for agent onboarding:

#### Agent Self-Registration

```bash
# Agent registers itself on first startup
curl -X POST http://localhost:8080/agent/register \
  -H "Content-Type: application/json" \
  -d '{
    "agentId": "019c1234-5678-7abc-def0-123456789abc",
    "claimTokenHash": "a3f5b2c8d1e9f4a7b6c5d2e1f8a9b4c7...",
    "hostname": "prod-web-01",
    "agentVersion": "1.0.0"
  }'

# Response includes poll URL and claim URL
# {
#   "status": "pending_claim",
#   "pollUrl": "/agents/019c1234-5678-7abc-def0-123456789abc/claim-status",
#   "claimUrl": "https://yoursite.com/claim"
# }
```

#### Agent Polls for Claim Status

```bash
# Agent polls this endpoint until claimed
curl http://localhost:8080/agents/019c1234-5678-7abc-def0-123456789abc/claim-status

# Returns pending while waiting:
# { "status": "pending" }

# Returns API key once (after admin claims):
# { "status": "claimed", "apiKey": "sk_live_..." }

# Subsequent polls return pending (key already delivered):
# { "status": "pending" }
```

#### Administrator Claims Agent (requires OAuth2 - coming soon)

```bash
# Admin claims pending agent via web UI or API
curl -X POST http://localhost:8080/agents/claim \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <admin-token>" \
  -d '{
    "agentId": "019c1234-5678-7abc-def0-123456789abc",
    "claimToken": "original-token-from-agent",
    "sectionId": "019c9876-5432-7fed-cba0-987654321fed",
    "name": "Production Web Server 01"
  }'
```

### 5. Using PostgreSQL (Production)

Edit your `config.yaml` file:

```yaml
server:
  environment: production

database_type: postgres

postgres_config:
  type: postgres
  host: localhost
  port: 5432
  username: smotra
  password: your_password
  database: smotra
  sslmode: require
  max_open_conns: 25
  max_idle_conns: 5
  conn_max_lifetime: 5m
  conn_max_idle_time: 10m
```

Then start the server:

```bash
CONFIG_FILE=config.yaml go run cmd/server/main.go
```

## Configuration

The server is configured using a YAML or JSON configuration file. The path to the configuration file is specified using the `-c` command line flag (default: `config.yaml`).

### Example Configurations

The project includes example configurations in the `configs/` directory:
- `configs/dev.yaml` - Development configuration with SQLite
- `configs/prod.yaml` - Production configuration template with PostgreSQL

### Configuration File Format

Both YAML and JSON formats are supported.

**Development Configuration (configs/dev.yaml):**
```yaml
server:
  host: 0.0.0.0
  port: 8080
  read_timeout: 15s
  write_timeout: 15s
  idle_timeout: 2m
  shutdown_timeout: 30s
  environment: development

database_type: sqlite

sqlite_config:
  config:
    type: sqlite
    maxopenconns: 25
    maxidleconns: 5
    connmaxlifetime: 15m
    connmaxidletime: 5m
  filepath: ./data/smotra.db

logging:
  level: debug
  format: json

auth:
  jwt_secret: development-secret-change-in-production
  jwt_expiration: 24h
```

**Production Configuration (configs/prod.yaml):**
```yaml
server:
  host: 0.0.0.0
  port: 8080
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 2m
  shutdown_timeout: 30s
  environment: production

database_type: postgres

postgres_config:
  config:
    type: postgres
    maxopenconns: 100
    maxidleconns: 20
    connmaxlifetime: 15m
    connmaxidletime: 5m
  host: localhost
  port: 5432
  username: smotra
  password: ""  # Set via environment variable
  database: smotra
  sslmode: require

logging:
  level: warn
  format: json

auth:
  jwt_secret: ""  # Required - set via environment variable
  jwt_expiration: 24h
```

### Configuration Fields

#### Server
- `host`: HTTP server host (default: 0.0.0.0)
- `port`: HTTP server port (default: 8080)
- `read_timeout`: Read timeout duration (default: 15s)
- `write_timeout`: Write timeout duration (default: 15s)
- `idle_timeout`: Idle timeout duration (default: 2m)
- `shutdown_timeout`: Graceful shutdown timeout (default: 30s)
- `environment`: development, staging, or production

#### Database
- `database_type`: sqlite or postgres

**For SQLite:**
- `config.type`: Must be "sqlite"
- `config.maxopenconns`: Maximum open connections (default: 25)
- `config.maxidleconns`: Maximum idle connections (default: 5)
- `config.connmaxlifetime`: Connection max lifetime (default: 15m)
- `config.connmaxidletime`: Connection max idle time (default: 5m)
- `filepath`: Database file path (default: ./data/smotra.db)

**For PostgreSQL:**
- `config.type`: Must be "postgres"
- `config.maxopenconns`: Maximum open connections (default: 100)
- `config.maxidleconns`: Maximum idle connections (default: 20)
- `config.connmaxlifetime`: Connection max lifetime (default: 15m)
- `config.connmaxidletime`: Connection max idle time (default: 5m)
- `host`: Database host
- `port`: Database port (default: 5432)
- `username`: Database username
- `password`: Database password
- `database`: Database name
- `sslmode`: SSL mode (disable, require, verify-full)

#### Logging
- `level`: debug, info, warn, or error
- `format`: json or text

#### Authentication
- `jwt_secret`: JWT signing secret (required in production)
- `jwt_expiration`: JWT token expiration duration (default: 24h)

## Building for Production

```bash
# Build the binary
just build

# Or manually
go build -ldflags "-X main.version=1.0.0" -o bin/smotra-server cmd/server/main.go

# Run the binary with config file
./bin/smotra-server -c configs/prod.yaml
```

## Available just Targets

The project includes a comprehensive justfile for common development tasks:

```bash
just help              # Display all available targets
just build             # Build the server binary
just run               # Run the server in development mode
just test              # Run tests
just test-coverage     # Run tests with coverage report
just clean             # Clean build artifacts
just generate-oapi     # Generate code from OpenAPI spec
just fmt               # Format Go code
just lint              # Run linters (go vet)
just tidy              # Tidy Go modules
just install-tools     # Install required tools (oapi-codegen)
just dev               # Run with auto-reload (requires air)
just docker-build      # Build Docker image
just docker-run        # Run Docker container
just all               # Run all build steps (clean, generate, fmt, lint, test, build)
```

## Project Structure

```
server/
├── cmd/
│   └── server/              # Main application entry point
│       └── main.go          # Server initialization and routing
├── configs/                 # Configuration files
│   ├── dev.yaml            # Development configuration (SQLite)
│   ├── dev.json            # Development configuration (JSON format)
│   └── prod.yaml           # Production configuration (PostgreSQL)
├── internal/
│   ├── api/                # Generated API code (from OpenAPI spec)
│   │   └── api.gen.go      # Generated with oapi-codegen
│   ├── config/             # Configuration management
│   │   ├── config.go       # Config loading and validation
│   │   └── types.go        # Config types and defaults
│   ├── database/           # Database interface and implementations
│   │   ├── factory.go      # Database factory pattern
│   │   ├── postgres.go     # PostgreSQL implementation
│   │   ├── sqlite.go       # SQLite implementation
│   │   ├── types.go        # Database interfaces and types
│   │   └── queries/        # sqlc-generated database queries
│   │       ├── agents.sql      # Agent-related SQL queries
│   │       ├── agents.sql.go   # Generated Go code for agent queries
│   │       ├── db.go           # Database interface
│   │       └── models.go       # Generated database models
│   ├── handlers/           # HTTP handlers
│   │   ├── handlers.go     # Combined handler implementation
│   │   ├── authenticated_handler.go # Authenticated wrapper for protected endpoints
│   │   ├── agent_configuration/ # Agent configuration handlers
│   │   │   └── configuration.go # GET /agent/{agentId}/configuration
│   │   ├── agent_register/ # Agent self-registration handlers
│   │   │   ├── register.go      # POST /agent/register
│   │   │   ├── register_test.go # Unit tests
│   │   │   └── register_integration_test.go # Integration tests
│   │   ├── agent_claim_status/ # Agent claim status handlers
│   │   │   ├── claim_status.go  # GET /agents/{agentId}/claim-status
│   │   │   ├── claim_status_test.go # Unit tests
│   │   │   └── claim_status_integration_test.go # Integration tests
│   │   ├── agent_claim/    # Administrator claim handlers
│   │   │   ├── claim.go         # POST /agents/claim
│   │   │   ├── claim_test.go    # Unit tests
│   │   │   └── claim_integration_test.go # Integration tests
│   │   ├── health/         # Health check handlers
│   │   │   └── health.go   # Health, readiness, liveness checks
│   │   └── metrics/        # Prometheus metrics handlers
│   │       └── metrics.go  # Metrics endpoint implementation
│   ├── logger/             # Structured logging
│   │   └── logger.go       # Logger implementation using slog
│   ├── middleware/         # HTTP middleware
│   │   ├── middleware.go   # RequestID, Logger, Recovery, CORS
│   │   └── auth.go         # Agent API key authentication
│   └── testutil/           # Testing utilities
│       ├── config.go       # Test configuration helpers
│       ├── database.go     # Test database helpers
│       ├── logger.go       # Test logger helpers
│       └── mock_database.go # Mock database implementation
├── api/                    # OpenAPI configuration
│   └── oapi-codegen.yaml   # oapi-codegen configuration
├── bin/                    # Compiled binaries (gitignored)
├── data/                   # Database files for SQLite (gitignored)
├── examples/               # Example code
│   └── config/             # Configuration examples
├── script/                 # Build and deployment scripts
├── go.mod                  # Go module definition
├── go.sum                  # Go module checksums
├── justfile               # Build automation (just command runner)
├── LICENSE                # License file
├── README.md              # This file
└── TESTING.md             # Testing documentation
```

## Current Features

### Server Core
- **HTTP Server**: Built with chi router for routing and middleware support
- **Configuration Management**: YAML/JSON config files with validation
- **Structured Logging**: slog-based logging with JSON/text formats
- **Graceful Shutdown**: Proper handling of SIGTERM/SIGINT signals
- **Health Checks**: `/healthz`, `/healthz/ready`, `/healthz/live` endpoints
- **Metrics Endpoint**: `/metrics` endpoint exposing Prometheus-format metrics
- **Authentication**: Agent API key authentication for protected endpoints

### Middleware
- **Request ID**: Automatic request ID generation and propagation
- **Logger**: Request/response logging with timing and status codes
- **Recovery**: Panic recovery with proper error handling
- **CORS**: Configurable CORS headers for API access
- **Authentication**: Agent API key authentication middleware for securing endpoints

### Database Support
- **SQLite**: For development and small deployments
- **PostgreSQL**: For production deployments with connection pooling
- **Interface-based**: Easy to swap database backends
- **Connection Management**: Configurable connection pools and timeouts
- **sqlc Code Generation**: Type-safe database queries generated from SQL
- **Multi-Tenant Schema**: Hierarchical structure with tenants, sections, agents, and endpoints
- **UUIDv7 Keys**: Time-ordered UUIDs for efficient indexing

### API
- **OpenAPI 3.0**: API specification maintained in separate [smotra-monitoring/openapi](https://github.com/smotra-monitoring/openapi) repository
- **Dual Router Architecture**: Separates monitoring endpoints (root) from business logic (`/api/v1`)
  - **Root Endpoints**: `/healthz`, `/metrics` - No versioning, infrastructure monitoring
  - **Versioned Endpoints**: `/api/v1/*` - Core business logic with API versioning
- **Tag-Based Code Generation**: Two oapi-codegen configs generate separate packages:
  - `api/oapi-codegen-root.yaml` → `internal/api/health/` (health tag)
  - `api/oapi-codegen-prefixed.yaml` → `internal/api/v1/` (current tag, excludes health)
- **Strict Handlers**: Uses OpenAPI strict handler pattern for type safety
- **Authentication**: Agent API key authentication via `X-Agent-API-Key` header
- **Future-Proof**: Clean separation allows adding `/api/v2` without conflicts

### Agent Configuration
- **Configuration Endpoint**: GET `/agent/{agentId}/configuration` to retrieve agent-specific configuration
- **Database-Backed**: Configuration stored in database with version tracking
- **sqlc Integration**: Type-safe database queries generated from SQL
- **Tags Support**: Agent-level and endpoint-level tags with scoping (agent, endpoint, global)
- **JSON Configuration**: Base configuration stored as JSON blob for flexibility
- **Authenticated Access**: Requires valid agent API key via `X-Agent-API-Key` header
- **Multi-Tenant Support**: Hierarchical structure with tenants, sections, and agents

## Development

### Prerequisites for Development

```bash
# Install required development tools (oapi-codegen and sqlc)
just install-tools

# Or manually
go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest

# Install just (if not already installed)
# On macOS:
brew install just
# On Linux:
cargo install just
# Or download from: https://github.com/casey/just/releases
```

### Adding New Features

1. Define API endpoints in the [smotra-monitoring/openapi](https://github.com/smotra-monitoring/openapi) repository
2. Regenerate API code: `just generate-oapi`
3. Implement handlers in `internal/handlers/` following the strict handler pattern
4. Routes are automatically registered via `api.HandlerFromMux()` in `cmd/server/main.go`
5. Add unit tests and integration tests for your handlers
6. Update metrics collection if appropriate (see Metrics section in copilot-instructions.md)
7. Run `just all` to verify everything builds

### Regenerating API Code

The OpenAPI specification is maintained in a separate repository. The server uses two code generation configs to separate endpoint types:

```bash
# Regenerate all API code (both root and versioned endpoints)
just generate-oapi

# Or manually:
# Root-level endpoints (/healthz, /metrics)
oapi-codegen -config=api/oapi-codegen-root.yaml api/openapi/api/spec.yaml

# Versioned API endpoints (/api/v1/*)
oapi-codegen -config=api/oapi-codegen-prefixed.yaml api/openapi/api/spec.yaml
```

**Generated Packages:**
- `internal/api/health/api.gen.go` - Health and metrics handlers (root level)
- `internal/api/v1/api.gen.go` - Versioned API handlers (/api/v1)

**Tag Strategy:**
- Endpoints tagged with `health` → root level package
- Endpoints tagged with `current` (excluding `health`) → versioned API package
- Future: Additional tags like `agent`, `user`, `alert` for organizational clarity

### Running Tests

```bash
# Run all tests (unit + integration)
just test

# Run unit tests only
just test-unit

# Run integration tests only
just test-integration

# Run tests with coverage
just test-coverage

# Run unit tests with coverage
just test-coverage-unit

# Run integration tests with coverage
just test-coverage-integration

# Run tests with verbose output
just test-verbose

# Run tests in watch mode (requires gotestsum)
just test-watch
```

### Code Formatting and Linting

```bash
# Format code
just fmt

# Run linter
just lint

# Tidy dependencies
just tidy
```

### Development Mode with Auto-Reload

```bash
# Install air first
go install github.com/cosmtrek/air@latest

# Run with auto-reload
just dev
```

## Troubleshooting

### Database Connection Issues

**SQLite:**
- Ensure the directory for the database file exists (`./data/` by default)
- The application will create the database file if it doesn't exist
- Check file permissions if you get write errors
- Default location: `./data/smotra.db`

**PostgreSQL:**
- Verify PostgreSQL is running: `pg_isready -h localhost -p 5432`
- Check credentials in your config file
- Ensure the database exists: `createdb smotra`
- Verify network connectivity and firewall rules
- Check SSL mode settings match your PostgreSQL configuration

### Port Already in Use

If port 8080 is already in use, change it in your config file:

```yaml
server:
  port: 8081
```

### Configuration File Not Found

If you see an error about missing configuration file:

```bash
# Specify the config file path with -c flag
go run cmd/server/main.go -c configs/dev.yaml

# Or when running the binary
./bin/smotra-server -c configs/prod.yaml
```

### Build Errors

```bash
# Clean and rebuild
just clean
just build

# Update dependencies
just tidy
go mod download
```

## Technology Stack

- **Language**: Go 1.23.4
- **Router**: chi v5
- **Logging**: slog (standard library)
- **Database Drivers**: 
  - SQLite: mattn/go-sqlite3
  - PostgreSQL: lib/pq
- **Database Query Generation**: sqlc (type-safe SQL to Go code generator)
- **API Code Generation**: oapi-codegen
- **Configuration**: YAML/JSON support via gopkg.in/yaml.v3
- **Build Tool**: just (command runner, similar to make)

## API Endpoints

### Health Checks
- `GET /healthz` - Basic health check
- `GET /healthz/ready` - Readiness check (includes database connectivity)
- `GET /healthz/live` - Liveness check

### Metrics
- `GET /metrics` - Prometheus metrics endpoint exposing server and application metrics

### API v1
- `GET /api/v1` - API version information

### Agent Configuration
- `GET /agent/{agentId}/configuration` - Retrieve agent-specific configuration including monitoring settings, endpoints, and tags (requires `X-Agent-API-Key` header)

### Future Endpoints
(To be implemented based on OpenAPI specification)
- Agent registration (`POST /agent/register`)
- Agent status reporting (`POST /agent/report`)
- Metrics submission and retrieval
- Alert configuration
- User authentication and management

## Contributing

Contributions are welcome! Please follow these guidelines:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes
4. Run tests and linting (`just all`)
5. Commit your changes (`git commit -m 'Add amazing feature'`)
6. Push to the branch (`git push origin feature/amazing-feature`)
7. Open a Pull Request

### Code Style
- Follow standard Go conventions and idioms
- Run `just fmt` before committing
- Ensure `just lint` passes
- Add tests for new functionality
- Update documentation as needed

## License

This project is source available with restrictions on SaaS usage without a commercial license. See the LICENSE file for details.

## Support

For issues and questions:
- GitHub Issues: https://github.com/smotra-monitoring/server/issues
- Documentation: https://docs.smotra.net (coming soon)

## Acknowledgments

Built with:
- [chi](https://github.com/go-chi/chi) - Lightweight router
- [oapi-codegen](https://github.com/deepmap/oapi-codegen) - OpenAPI code generation
- [slog](https://pkg.go.dev/log/slog) - Structured logging
- And other excellent open-source libraries (see go.mod)
