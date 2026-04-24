# Variables
binary_name := "smotra-server"
binary_path := "bin/" + binary_name
main_path   := "cmd/server/main.go"
config_file := "configs/dev.yaml"
version     := "0.0.1"

# Run all build steps
all: clean generate-oapi generate-sqlc fmt lint test build

# Display available commands
help:
    @just --list

# Build the server binary
build:
    @echo "Building {{binary_name}}..."
    mkdir -p bin
    go build -ldflags "-X main.version={{version}}" -o {{binary_path}} {{main_path}}
    @echo "Binary built: {{binary_path}}"

# Run the server (development mode)
run config=config_file:
    @echo "Running server with {{config}}..."
    go run {{main_path}} -c {{config}}

# Run all tests (unit + integration)
test:
    @echo "Running all tests..."
    go test -v ./...

# Run unit tests only
test-unit:
    @echo "Running unit tests..."
    go test -v -short ./...

# Run integration tests only
test-integration:
    @echo "Running integration tests..."
    go test -v -tags=integration ./...

# Run tests with coverage report
test-coverage:
    @echo "Running tests with coverage..."
    go test -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out -o coverage.html
    @echo "Coverage report generated: coverage.html"

# Run unit tests with coverage
test-coverage-unit:
    @echo "Running unit tests with coverage..."
    go test -short -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out -o coverage.html
    @echo "Unit test coverage report generated: coverage.html"

# Run integration tests with coverage
test-coverage-integration:
    @echo "Running integration tests with coverage..."
    go test -tags=integration -coverprofile=coverage-integration.out ./...
    go tool cover -html=coverage-integration.out -o coverage-integration.html
    @echo "Integration test coverage report generated: coverage-integration.html"

# Run tests with verbose output
test-verbose:
    @echo "Running tests with verbose output..."
    go test -v -cover ./...

# Run tests in watch mode (requires gotestsum)
test-watch:
    @if command -v gotestsum > /dev/null; then \
        gotestsum --watch -- -v ./...; \
    else \
        echo "gotestsum not found. Install it with: go install gotest.tools/gotestsum@latest"; \
    fi

test-benchmark:
    @echo "Running benchmarks..."
    go test -bench=. -benchmem ./...

# Clean build artifacts
clean:
    @echo "Cleaning..."
    rm -rf bin/
    rm -f coverage.out coverage.html
    rm -f {{binary_name}}
    @echo "Clean complete"

# Generate code from OpenAPI spec
generate-oapi:
    @echo "Generating API code from OpenAPI spec..."
    oapi-codegen -config=api/oapi-codegen-root.yaml api/openapi/api/spec.yaml
    oapi-codegen -config=api/oapi-codegen-prefixed.yaml api/openapi/api/spec.yaml
    @echo "Code generation complete"

# Generate database code from SQL queries using sqlc
generate-sqlc:
    @echo "Generating database code from SQL queries..."
    sqlc generate -f data/db/dev/sqlc/sqlc.yaml
    @echo "sqlc code generation complete"

# Format Go code
fmt:
    @echo "Formatting code..."
    go fmt ./...
    @echo "Formatting complete"

# Run linters
lint:
    @echo "Running linters..."
    go vet ./...
    @echo "Linting complete"

# Tidy Go modules
tidy:
    @echo "Tidying modules..."
    go mod tidy
    @echo "Modules tidied"

# Install required tools
install-tools:
    @echo "Installing tools..."
    go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest
    go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
    @echo "Tools installed"

# Run in development mode with auto-reload (requires air)
dev:
    @if command -v air > /dev/null; then \
        air; \
    else \
        echo "air not found. Install it with: go install github.com/cosmtrek/air@latest"; \
        echo "Falling back to regular run..."; \
        just run; \
    fi

# Build Docker image
docker-build:
    @echo "Building Docker image..."
    docker build -t smotra-server:{{version}} .
    @echo "Docker image built: smotra-server:{{version}}"

# Run Docker container
docker-run:
    @echo "Running Docker container..."
    docker run -p 8080:8080 --env-file .env smotra-server:{{version}}