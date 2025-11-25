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

test: ## Run tests
	@echo "Running tests..."
	@go test -v ./...

test-coverage: ## Run tests with coverage
	@echo "Running tests with coverage..."
	@go test -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

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
