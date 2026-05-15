# Testing Guide

This document describes the testing strategy and how to run tests for the Smotra Monitoring Server.

## Test Structure

The project includes comprehensive unit tests and integration tests for all major packages:

### Packages with Tests

1. **internal/config** - Configuration loading and validation
   - Unit tests for YAML/JSON parsing
   - Validation logic for all configuration fields
   - Coverage: 93.6%

2. **internal/logger** - Structured logging
   - Tests for different log levels (debug, info, warn, error)
   - Tests for different output formats (JSON, text)
   - Tests for logger context and components
   - Coverage: 100%

3. **internal/database** - Database abstraction layer
   - Unit tests for factory pattern and configuration
   - Integration tests for SQLite operations (Open, Close, Ping, Health, Transactions)
   - Unit tests for `DBMetrics`: health status, connection pool stats format, unhealthy-db path, nil `*sql.DB` guard
   - Coverage: 52.4% (with integration tests)

4. **internal/middleware** - HTTP middleware
   - Unit tests for Logger, Recovery, RequestID, and CORS middleware
   - Unit tests for Agent API key authentication (`auth.go`)
   - Integration tests for authentication flow with database
   - Unit tests for `HTTPMetrics`: request counting by status code, implicit-200 path, Prometheus output format
   - Tests for responseWriter wrapper
   - Tests for chained middleware execution
   - Coverage: 100%

5. **internal/handlers/health** - Health check endpoints
   - Unit tests with mock database
   - Integration tests with real HTTP server
   - Tests for HealthCheck, ReadinessCheck, and LivenessCheck endpoints
   - Coverage: 98.1%

6. **internal/handlers/authenticated_handler** - Authentication wrapper
   - Unit tests for authentication verification
   - Integration tests for protected endpoints
   - Tests for agent ID matching and access control
   - Tests for error responses (401, 403)

7. **internal/handlers/metrics** - Prometheus metrics endpoint
   - Unit tests for `MetricsProvider` registration and output aggregation
   - Unit tests for unhealthy-DB path (via registered `DBMetrics` provider)
   - Integration tests verifying provider-supplied metrics appear in `/metrics` output
   - Concurrency test for concurrent provider registration

8. **internal/handlers/agent_configuration** - Agent configuration endpoint
   - Unit tests with mock database
   - Integration tests with real HTTP server and authentication
   - Tests for configuration retrieval with tags and endpoints

9. **internal/handlers/agent_register** - Agent self-registration
   - Unit tests for registration request validation and response format
   - Integration tests for complete registration flow with SQLite

10. **internal/handlers/agent_claim** - Administrator claiming of pending agents
    - Unit tests for token validation and agent creation
    - Integration tests for full claim workflow

11. **internal/handlers/agent_claim_status** - Agent claim status polling
    - Unit tests for pending/claimed/delivered state transitions
    - Integration tests including one-time API key delivery

12. **internal/handlers/agent_heartbeat** - Agent heartbeat and vitals submission
    - Unit tests with mock database
    - Integration tests with real HTTP server and authentication
    - Tests for `last_seen_at` update and vitals storage

13. **internal/handlers/agent_submit_results** - Monitoring results batch ingestion
    - Unit tests for all check types (ping, httpget, tcpconnect, udpconnect, traceroute, plugin)
    - Integration tests with real database and authentication
    - **Benchmark tests** (`submit_results_bench_test.go`) for database write performance
    - Tests for idempotent deduplication via client-assigned UUIDv7 IDs

14. **internal/handlers/auth** - OAuth2 relay endpoints
    - Unit tests for provider resolution, PKCE parameter construction, and redirect building
    - Tests use `NewHandlerForTesting()` to bypass SSRF validation

15. **internal/handlers** (top-level) - Route separation
    - `routes_separation_integration_test.go` verifies health endpoints are only at root (never under `/v1`) and API endpoints are only under `/v1`

16. **internal/testutil** - Test utilities and helpers
   - Mock database implementation
   - Test configuration helpers
   - Test database setup utilities

## Running Tests

### All Tests (Unit + Integration)

```bash
go test ./...
```

### Unit Tests Only

```bash
go test -short ./...
```

### Integration Tests Only

```bash
go test -tags=integration ./...
```

### With Verbose Output

```bash
go test -v ./...
```

### Coverage Reports

#### Generate Coverage Report

```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

#### View Coverage in Browser

Open `coverage.html` in your browser to see detailed coverage information.

#### Quick Coverage Summary

```bash
go test -cover ./...
```

## Makefile Commands

The project Makefile includes convenient commands for running tests:

```bash
# Run all tests
just test

# Run unit tests only
just test-unit

# Run integration tests only
just test-integration

# Run tests with coverage report
just test-coverage

# Run unit tests with coverage
just test-coverage-unit

# Run integration tests with coverage
just test-coverage-integration

# Run tests with verbose output
just test-verbose
```

## Test Organization

### Unit Tests

Unit tests are located alongside the code they test, following the Go convention:
- `package_test.go` - Unit tests for `package.go`
- Tests use table-driven tests where appropriate
- Mock objects are used to isolate dependencies

### Integration Tests

Integration tests are tagged with `// +build integration` and test real integrations:
- Database operations with actual SQLite databases
- HTTP handlers with real HTTP servers
- End-to-end workflows

Integration tests use temporary files and directories via `t.TempDir()` for isolation.

## Test Utilities

The `internal/testutil` package provides reusable test utilities:

- **MockDatabase**: Mock implementation of the `Database` interface
- **SetupTestSQLiteDB**: Creates a temporary SQLite database for testing
- **CreateTestConfigYAML**: Creates temporary YAML config files
- **CreateTestConfigJSON**: Creates temporary JSON config files
- **DefaultTestConfig**: Returns a default config suitable for testing

## Writing New Tests

### Unit Test Example

```go
func TestMyFunction(t *testing.T) {
    // Arrange
    input := "test"
    expected := "result"
    
    // Act
    result := MyFunction(input)
    
    // Assert
    if result != expected {
        t.Errorf("Expected %s, got %s", expected, result)
    }
}
```

### Integration Test Example

```go
// +build integration

func TestDatabaseIntegration(t *testing.T) {
    db := testutil.SetupTestSQLiteDB(t)
    
    // Test database operations
    err := db.Ping(context.Background())
    if err != nil {
        t.Fatalf("Ping failed: %v", err)
    }
}
```

## Coverage Goals

- **Unit Tests**: Aim for 80%+ coverage
- **Integration Tests**: Cover critical paths and edge cases
- **Overall**: Current coverage is ~52-93% depending on package

## Continuous Integration

Tests should be run in CI/CD pipelines:

```bash
# In CI pipeline
go test -v -race ./...                      # Run with race detector
go test -tags=integration -v ./...          # Run integration tests
go test -coverprofile=coverage.out ./...    # Generate coverage
```

## Best Practices

1. **Test Naming**: Use descriptive test names that explain what is being tested
2. **Table-Driven Tests**: Use table-driven tests for testing multiple scenarios
3. **Test Isolation**: Each test should be independent and not rely on other tests
4. **Cleanup**: Use `t.Cleanup()` for resource cleanup
5. **Temporary Resources**: Use `t.TempDir()` for temporary files/directories
6. **Mock Dependencies**: Use mocks for external dependencies in unit tests
7. **Integration Tests**: Tag with `// +build integration` at the top of the file
8. **Error Messages**: Provide clear error messages in assertions

## Troubleshooting

### Tests Fail on Windows

Some tests may have path-related issues on Windows. Use `filepath.ToSlash()` for cross-platform path handling.

### Integration Tests Timeout

Increase timeout for slow systems:

```bash
go test -timeout 30s -tags=integration ./...
```

### Database Locked Errors

SQLite integration tests use temporary databases. If you see "database locked" errors, ensure:
1. Tests properly close database connections
2. Only one connection is used in WAL mode
3. Tests use `t.Cleanup()` for proper cleanup

## Benchmark Tests

Benchmark tests measure handler throughput under load. Run them with:

```bash
# Run benchmarks for submit_results handler
go test -bench=. -benchmem ./internal/handlers/agent_submit_results/...

# Run all benchmarks in the project
go test -bench=. -benchmem ./...
```

Benchmarks are located alongside the handler they test (e.g. `submit_results_bench_test.go`) and do **not** use the `integration` build tag — they run in any environment.

## Future Improvements

- [ ] Add PostgreSQL integration tests
- [x] Add benchmark tests for performance-critical code (`submit_results_bench_test.go`)
- [ ] Increase coverage for edge cases
- [ ] Add mutation testing
- [ ] Add property-based testing for complex logic
