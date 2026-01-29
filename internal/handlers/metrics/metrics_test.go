package metrics

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/smotra-monitoring/server/internal/api"
	"github.com/smotra-monitoring/server/internal/testutil"
)

func TestNewHandler(t *testing.T) {
	logger, _ := testutil.NewTestLogger()
	db := testutil.NewMockDatabase()
	appVersion := "1.0.0-test"

	handler := NewHandler(logger, db, appVersion)

	if handler == nil {
		t.Fatal("handler is nil")
	}

	if handler.appVersion != appVersion {
		t.Errorf("expected fake version %s, got %s", appVersion, handler.appVersion)
	}

	if handler.logger == nil {
		t.Error("logger is nil")
	}

	if handler.db == nil {
		t.Error("db is nil")
	}
}

func TestIncrementHTTPRequests(t *testing.T) {
	logger, _ := testutil.NewTestLogger()
	db := testutil.NewMockDatabase()
	handler := NewHandler(logger, db, "1.0.0")

	// Test successful requests
	handler.IncrementHTTPRequests(true)
	handler.IncrementHTTPRequests(true)
	handler.IncrementHTTPRequests(true)

	if handler.httpRequestsTotal.Load() != 3 {
		t.Errorf("expected total requests 3, got %d", handler.httpRequestsTotal.Load())
	}

	if handler.httpRequestsSuccess.Load() != 3 {
		t.Errorf("expected successful requests 3, got %d", handler.httpRequestsSuccess.Load())
	}

	if handler.httpRequestsFailure.Load() != 0 {
		t.Errorf("expected failed requests 0, got %d", handler.httpRequestsFailure.Load())
	}

	// Test failed requests
	handler.IncrementHTTPRequests(false)
	handler.IncrementHTTPRequests(false)

	if handler.httpRequestsTotal.Load() != 5 {
		t.Errorf("expected total requests 5, got %d", handler.httpRequestsTotal.Load())
	}

	if handler.httpRequestsSuccess.Load() != 3 {
		t.Errorf("expected successful requests 3, got %d", handler.httpRequestsSuccess.Load())
	}

	if handler.httpRequestsFailure.Load() != 2 {
		t.Errorf("expected failed requests 2, got %d", handler.httpRequestsFailure.Load())
	}
}

func TestIncrementDBQueries(t *testing.T) {
	logger, _ := testutil.NewTestLogger()
	db := testutil.NewMockDatabase()
	handler := NewHandler(logger, db, "1.0.0")

	// Test successful queries
	handler.IncrementDBQueries(true)
	handler.IncrementDBQueries(true)

	if handler.dbQueriesTotal.Load() != 2 {
		t.Errorf("expected total queries 2, got %d", handler.dbQueriesTotal.Load())
	}

	if handler.dbQueriesSuccess.Load() != 2 {
		t.Errorf("expected successful queries 2, got %d", handler.dbQueriesSuccess.Load())
	}

	if handler.dbQueriesFailure.Load() != 0 {
		t.Errorf("expected failed queries 0, got %d", handler.dbQueriesFailure.Load())
	}

	// Test failed queries
	handler.IncrementDBQueries(false)

	if handler.dbQueriesTotal.Load() != 3 {
		t.Errorf("expected total queries 3, got %d", handler.dbQueriesTotal.Load())
	}

	if handler.dbQueriesSuccess.Load() != 2 {
		t.Errorf("expected successful queries 2, got %d", handler.dbQueriesSuccess.Load())
	}

	if handler.dbQueriesFailure.Load() != 1 {
		t.Errorf("expected failed queries 1, got %d", handler.dbQueriesFailure.Load())
	}
}

func TestPrometheusMetrics(t *testing.T) {
	logger, _ := testutil.NewTestLogger()
	db := testutil.NewMockDatabase()
	handler := NewHandler(logger, db, "1.0.0-test")

	// Set some metrics
	handler.IncrementHTTPRequests(true)
	handler.IncrementHTTPRequests(true)
	handler.IncrementHTTPRequests(false)
	handler.IncrementDBQueries(true)
	handler.IncrementDBQueries(false)

	// Sleep a bit to get uptime > 0
	time.Sleep(10 * time.Millisecond)

	ctx := context.Background()
	resp, err := handler.PrometheusMetrics(ctx, struct{}{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := string(resp.(api.PrometheusMetrics200TextResponse))

	// Verify required metrics are present
	requiredMetrics := []string{
		"smotra_info",
		"smotra_uptime_seconds",
		"smotra_http_requests_total",
		"smotra_http_requests_success_total",
		"smotra_http_requests_failure_total",
		"smotra_db_queries_total",
		"smotra_db_queries_success_total",
		"smotra_db_queries_failure_total",
		"smotra_db_healthy",
		"smotra_go_goroutines",
		"smotra_go_memory_alloc_bytes",
		"smotra_agents_registered_total",
	}

	for _, metric := range requiredMetrics {
		if !strings.Contains(output, metric) {
			t.Errorf("expected metric %s not found in output", metric)
		}
	}

	// Verify metric values
	if !strings.Contains(output, "smotra_http_requests_total 3") {
		t.Error("expected http_requests_total to be 3")
	}

	if !strings.Contains(output, "smotra_http_requests_success_total 2") {
		t.Error("expected http_requests_success_total to be 2")
	}

	if !strings.Contains(output, "smotra_http_requests_failure_total 1") {
		t.Error("expected http_requests_failure_total to be 1")
	}

	if !strings.Contains(output, "smotra_db_queries_total 2") {
		t.Error("expected db_queries_total to be 2")
	}

	if !strings.Contains(output, `version="1.0.0-test"`) {
		t.Error("expected version label to be 1.0.0-test")
	}

	// Verify HELP and TYPE comments
	if !strings.Contains(output, "# HELP smotra_http_requests_total") {
		t.Error("expected HELP comment for http_requests_total")
	}

	if !strings.Contains(output, "# TYPE smotra_http_requests_total counter") {
		t.Error("expected TYPE comment for http_requests_total")
	}
}

func TestPrometheusMetricsWithUnhealthyDB(t *testing.T) {
	logger, _ := testutil.NewTestLogger()
	db := testutil.NewMockDatabase()

	// Simulate database error
	db.ShouldFail = true

	handler := NewHandler(logger, db, "1.0.0")

	ctx := context.Background()
	resp, err := handler.PrometheusMetrics(ctx, struct{}{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := string(resp.(api.PrometheusMetrics200TextResponse))

	// Database should be marked as unhealthy
	if !strings.Contains(output, "smotra_db_healthy 0") {
		t.Error("expected db_healthy to be 0 when database is unhealthy")
	}

	// Response time metric should not be present when DB is unhealthy
	if strings.Contains(output, "smotra_db_response_time_ms") {
		t.Error("expected db_response_time_ms to not be present when database is unhealthy")
	}
}

func TestPrometheusMetricsFormat(t *testing.T) {
	logger, _ := testutil.NewTestLogger()
	db := testutil.NewMockDatabase()
	handler := NewHandler(logger, db, "1.0.0")

	ctx := context.Background()
	resp, err := handler.PrometheusMetrics(ctx, struct{}{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := string(resp.(api.PrometheusMetrics200TextResponse))
	lines := strings.Split(output, "\n")

	// Verify format: each metric should have HELP and TYPE comments
	helpCount := 0
	typeCount := 0
	metricCount := 0

	for _, line := range lines {
		if strings.HasPrefix(line, "# HELP") {
			helpCount++
		} else if strings.HasPrefix(line, "# TYPE") {
			typeCount++
		} else if line != "" && !strings.HasPrefix(line, "#") {
			metricCount++
		}
	}

	// We should have roughly equal HELP and TYPE comments
	if helpCount == 0 {
		t.Error("expected at least one HELP comment")
	}

	if typeCount == 0 {
		t.Error("expected at least one TYPE comment")
	}

	if metricCount == 0 {
		t.Error("expected at least one metric")
	}

	// HELP and TYPE counts should be close (within 1-2 of each other)
	if abs(helpCount-typeCount) > 2 {
		t.Errorf("expected similar HELP (%d) and TYPE (%d) counts", helpCount, typeCount)
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
