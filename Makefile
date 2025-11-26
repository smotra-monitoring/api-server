.PHONY: help build run test clean generate fmt lint docker-build

# Variables
BINARY_NAME=smotra-server
BINARY_PATH=bin/$(BINARY_NAME)
MAIN_PATH=cmd/server/main.go
CONFIG_FILE?=configs/dev.yaml
VERSION?=0.0.1

help: ## Display this help message
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ## Build the server binary
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p bin
	@go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY_PATH) $(MAIN_PATH)
	@echo "Binary built: $(BINARY_PATH)"

run: ## Run the server (development mode)
	@echo "Running server with $(CONFIG_FILE)..."
	@go run $(MAIN_PATH) -c $(CONFIG_FILE)

test: ## Run all tests (unit + integration)
	@echo "Running all tests..."
	@go test -v ./...

test-unit: ## Run unit tests only
	@echo "Running unit tests..."
	@go test -v -short ./...

test-integration: ## Run integration tests only
	@echo "Running integration tests..."
	@go test -v -tags=integration ./...

test-coverage: ## Run tests with coverage report
	@echo "Running tests with coverage..."
	@go test -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

test-coverage-unit: ## Run unit tests with coverage
	@echo "Running unit tests with coverage..."
	@go test -short -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Unit test coverage report generated: coverage.html"

test-coverage-integration: ## Run integration tests with coverage
	@echo "Running integration tests with coverage..."
	@go test -tags=integration -coverprofile=coverage-integration.out ./...
	@go tool cover -html=coverage-integration.out -o coverage-integration.html
	@echo "Integration test coverage report generated: coverage-integration.html"

test-verbose: ## Run tests with verbose output
	@echo "Running tests with verbose output..."
	@go test -v -cover ./...

test-watch: ## Run tests in watch mode (requires gotestsum)
	@if command -v gotestsum > /dev/null; then \
		gotestsum --watch -- -v ./...; \
	else \
		echo "gotestsum not found. Install it with: go install gotest.tools/gotestsum@latest"; \
	fi

clean: ## Clean build artifacts
	@echo "Cleaning..."
	@rm -rf bin/
	@rm -f coverage.out coverage.html
	@rm -f $(BINARY_NAME)
	@echo "Clean complete"

generate-oapi: ## Generate code from OpenAPI spec
	@echo "Generating API code from OpenAPI spec..."
	@oapi-codegen -config api/oapi-codegen.yaml api/spec.yaml
	@echo "Code generation complete"

fmt: ## Format Go code
	@echo "Formatting code..."
	@go fmt ./...
	@echo "Formatting complete"

lint: ## Run linters
	@echo "Running linters..."
	@go vet ./...
	@echo "Linting complete"

tidy: ## Tidy Go modules
	@echo "Tidying modules..."
	@go mod tidy
	@echo "Modules tidied"

install-tools: ## Install required tools
	@echo "Installing tools..."
	@go install github.com/deepmap/oapi-codegen/v2/cmd/oapi-codegen@latest
	@echo "Tools installed"

dev: ## Run in development mode with auto-reload (requires air)
	@if command -v air > /dev/null; then \
		air; \
	else \
		echo "air not found. Install it with: go install github.com/cosmtrek/air@latest"; \
		echo "Falling back to regular run..."; \
		$(MAKE) run; \
	fi

docker-build: ## Build Docker image
	@echo "Building Docker image..."
	@docker build -t smotra-server:$(VERSION) .
	@echo "Docker image built: smotra-server:$(VERSION)"

docker-run: ## Run Docker container
	@echo "Running Docker container..."
	@docker run -p 8080:8080 --env-file .env smotra-server:$(VERSION)

all: clean generate-oapi fmt lint test build ## Run all build steps
