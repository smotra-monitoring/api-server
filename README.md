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

# Or use the Makefile
make run
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
make build

# Or manually
go build -ldflags "-X main.version=1.0.0" -o bin/smotra-server cmd/server/main.go

# Run the binary with config file
./bin/smotra-server -c configs/prod.yaml
```

## Available Make Targets

The project includes a comprehensive Makefile for common development tasks:

```bash
make help              # Display all available targets
make build             # Build the server binary
make run               # Run the server in development mode
make test              # Run tests
make test-coverage     # Run tests with coverage report
make clean             # Clean build artifacts
make generate-oapi     # Generate code from OpenAPI spec
make fmt               # Format Go code
make lint              # Run linters (go vet)
make tidy              # Tidy Go modules
make install-tools     # Install required tools (oapi-codegen)
make dev               # Run with auto-reload (requires air)
make docker-build      # Build Docker image
make docker-run        # Run Docker container
make all               # Run all build steps (clean, generate, fmt, lint, test, build)
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
│   ├── config/             # Configuration management
│   │   ├── config.go       # Config loading and validation
│   │   └── types.go        # Config types and defaults
│   ├── database/           # Database interface and implementations
│   │   ├── factory.go      # Database factory pattern
│   │   ├── postgres.go     # PostgreSQL implementation
│   │   ├── sqlite.go       # SQLite implementation
│   │   └── types.go        # Database interfaces and types
│   ├── handlers/           # HTTP handlers
│   │   └── health/         # Health check handlers
│   │       └── health.go   # Health, readiness, liveness checks
│   ├── logger/             # Structured logging
│   │   └── logger.go       # Logger implementation using slog
│   └── middleware/         # HTTP middleware
│       └── middleware.go   # RequestID, Logger, Recovery, CORS
├── pkg/
│   └── api/                # Generated API code (from OpenAPI spec)
│       └── api.gen.go      # Generated with oapi-codegen
├── api/                    # OpenAPI specification
│   ├── config.yaml         # oapi-codegen configuration
│   └── spec.yaml           # OpenAPI 3.0 specification
├── bin/                    # Compiled binaries (gitignored)
├── data/                   # Database files for SQLite (gitignored)
├── go.mod                  # Go module definition
├── go.sum                  # Go module checksums
├── Makefile               # Build automation
└── README.md              # This file
```

## Current Features

### Server Core
- **HTTP Server**: Built with chi router for routing and middleware support
- **Configuration Management**: YAML/JSON config files with validation
- **Structured Logging**: slog-based logging with JSON/text formats
- **Graceful Shutdown**: Proper handling of SIGTERM/SIGINT signals
- **Health Checks**: `/healthz`, `/healthz/ready`, `/healthz/live` endpoints

### Middleware
- **Request ID**: Automatic request ID generation and propagation
- **Logger**: Request/response logging with timing and status codes
- **Recovery**: Panic recovery with proper error handling
- **CORS**: Configurable CORS headers for API access

### Database Support
- **SQLite**: For development and small deployments
- **PostgreSQL**: For production deployments with connection pooling
- **Interface-based**: Easy to swap database backends
- **Connection Management**: Configurable connection pools and timeouts

### API
- **OpenAPI 3.0**: API specification in `api/spec.yaml`
- **Code Generation**: Server stubs generated with oapi-codegen
- **Versioned APIs**: API v1 routes under `/api/v1/`

## Development

### Prerequisites for Development

```bash
# Install oapi-codegen for API code generation
make install-tools

# Or manually
go install github.com/deepmap/oapi-codegen/v2/cmd/oapi-codegen@latest
```

### Adding New Features

1. Define API endpoints in `api/spec.yaml`
2. Regenerate API code: `make generate-oapi`
3. Implement handlers in `internal/handlers/`
4. Register routes in `cmd/server/main.go`
5. Add tests for your handlers
6. Run `make all` to verify everything builds

### Regenerating API Code

After modifying `api/spec.yaml`:

```bash
make generate-oapi
```

This will regenerate `pkg/api/api.gen.go` with the updated API definitions.

### Running Tests

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage
# Opens coverage.html in your browser
```

### Code Formatting and Linting

```bash
# Format code
make fmt

# Run linter
make lint

# Tidy dependencies
make tidy
```

### Development Mode with Auto-Reload

```bash
# Install air first
go install github.com/cosmtrek/air@latest

# Run with auto-reload
make dev
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
make clean
make build

# Update dependencies
make tidy
go mod download
```

## Technology Stack

- **Language**: Go 1.23.4
- **Router**: chi v5
- **Logging**: slog (standard library)
- **Database Drivers**: 
  - SQLite: mattn/go-sqlite3
  - PostgreSQL: lib/pq
- **API Code Generation**: oapi-codegen
- **Configuration**: YAML/JSON support via gopkg.in/yaml.v3

## Roadmap

### Short Term
- [ ] Database migrations with go-migrate
- [ ] JWT authentication implementation
- [ ] User management endpoints
- [ ] Agent registration and management
- [ ] Docker and docker-compose setup

### Medium Term
- [ ] OAuth2 integration
- [ ] Metrics collection from agents
- [ ] Alert configuration and notification system
- [ ] Web dashboard (frontend)
- [ ] Prometheus metrics endpoint
- [ ] TimescaleDB integration for time-series data

### Long Term
- [ ] Kubernetes deployment with Helm charts
- [ ] Plugin system for extensibility
- [ ] Advanced monitoring features
- [ ] Distributed tracing
- [ ] Multi-tenant support

## API Endpoints

### Health Checks
- `GET /healthz` - Basic health check
- `GET /healthz/ready` - Readiness check (includes database connectivity)
- `GET /healthz/live` - Liveness check

### API v1
- `GET /api/v1` - API version information

### Future Endpoints
(To be implemented based on OpenAPI specification)
- Agent registration and management
- Metrics submission and retrieval
- Alert configuration
- User authentication and management

## Contributing

Contributions are welcome! Please follow these guidelines:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes
4. Run tests and linting (`make all`)
5. Commit your changes (`git commit -m 'Add amazing feature'`)
6. Push to the branch (`git push origin feature/amazing-feature`)
7. Open a Pull Request

### Code Style
- Follow standard Go conventions and idioms
- Run `make fmt` before committing
- Ensure `make lint` passes
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
