# Smotra Monitoring Server - Quick Start Guide

This guide will help you get the server running quickly.

## Prerequisites

- Go 1.23 or later
- For PostgreSQL: PostgreSQL 13+ with TimescaleDB extension (optional for production)
- For SQLite: No additional dependencies required (default for development)

## Quick Start

### 1. Clone and Setup

```bash
# Clone the repository
git clone https://github.com/smotra-monitoring/server.git
cd server

# Copy the example environment file
cp .env.example .env

# Edit .env with your configuration (optional, defaults work for development)
# nano .env
```

### 2. Run with SQLite (Development)

The default configuration uses SQLite, which requires no additional setup:

```bash
# Build and run
go run cmd/server/main.go
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

### 4. Using PostgreSQL (Production)

Edit your `.env` file:

```bash
DB_TYPE=postgres
DB_HOST=localhost
DB_PORT=5432
DB_USERNAME=smotra
DB_PASSWORD=your_password
DB_DATABASE=smotra
DB_SSLMODE=disable  # Use 'require' or 'verify-full' in production
```

Then start the server:

```bash
go run cmd/server/main.go
```

## Configuration Options

All configuration is done through environment variables. See `.env.example` for all available options.

### Key Configuration

- `SERVER_PORT`: HTTP server port (default: 8080)
- `ENVIRONMENT`: development, staging, or production
- `DB_TYPE`: sqlite or postgres
- `LOG_LEVEL`: debug, info, warn, or error
- `LOG_FORMAT`: json or text

## Building for Production

```bash
# Build the binary
go build -o server cmd/server/main.go

# Run the binary
./server
```

Or with custom output location:

```bash
go build -o bin/smotra-server cmd/server/main.go
./bin/smotra-server
```

## Docker Support (Coming Soon)

Docker support with docker-compose configurations will be added soon.

## Project Structure

```
server/
├── cmd/
│   └── server/          # Main application entry point
│       └── main.go
├── internal/
│   ├── config/          # Configuration management
│   ├── database/        # Database interface and implementations
│   ├── handlers/        # HTTP handlers
│   │   └── health/      # Health check handlers
│   ├── logger/          # Logging setup
│   └── middleware/      # HTTP middleware
├── pkg/
│   └── api/             # Generated API code (from OpenAPI spec)
└── oapi-codegen/        # OpenAPI specification
    ├── config.yaml
    └── spec.yaml
```

## Development

### Adding New Features

1. Define API endpoints in `oapi-codegen/spec.yaml`
2. Regenerate API code: `make generate` (or run oapi-codegen manually)
3. Implement handlers in `internal/handlers/`
4. Register routes in `cmd/server/main.go`

### Running Tests

```bash
go test ./...
```

### Code Formatting

```bash
go fmt ./...
```

## Troubleshooting

### Database Connection Issues

**SQLite:**
- Ensure the directory for the database file exists or the application has write permissions
- Default location: `./data/smotra.db`

**PostgreSQL:**
- Verify PostgreSQL is running: `pg_isready -h localhost -p 5432`
- Check credentials and database exists
- Verify network connectivity and firewall rules

### Port Already in Use

If port 8080 is already in use, change it in `.env`:

```bash
SERVER_PORT=8081
```

## Next Steps

- Review the full API documentation in `oapi-codegen/spec.yaml`
- Set up database migrations (coming soon)
- Configure OAuth2 authentication
- Deploy using Docker/Kubernetes

## Support

For issues and questions:
- GitHub Issues: https://github.com/smotra-monitoring/server/issues
- Documentation: https://docs.smotra.net (coming soon)
