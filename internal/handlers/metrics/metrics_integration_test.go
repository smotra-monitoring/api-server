package metrics

import (
	"context"
	"strings"
	"testing"

	healthAPI "github.com/smotra-monitoring/server/internal/api/health"
	"github.com/smotra-monitoring/server/internal/database"
	"github.com/smotra-monitoring/server/internal/testutil"
)

func TestPrometheusMetrics_Integration(t *testing.T) {
	logger, _ := testutil.NewTestLogger()
	db := testutil.NewMockDatabase()

	handler := NewHandler(logger, "1.0.0-integration")

	// Register the real DBMetrics provider (covers smotra_db_healthy, pool stats)
	handler.RegisterMetricsProvider(database.NewDBMetrics(db))

	// Register a stub provider that simulates the HTTP metrics provider
	handler.RegisterMetricsProvider(&stubMetricsProvider{
		output: "# HELP smotra_http_requests_total Total HTTP requests\n# TYPE smotra_http_requests_total counter\nsmotra_http_requests_total 3\n\n# HELP smotra_db_connections_open Open DB connections\n# TYPE smotra_db_connections_open gauge\nsmotra_db_connections_open 1\n\n",
	})

	ctx := context.Background()
	resp, err := handler.PrometheusMetrics(ctx, struct{}{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := string(resp.(healthAPI.PrometheusMetrics200TextResponse))
	// output := string(resp.(healthAPI.PrometheusMetrics200TextResponse))

	// Verify provider-supplied metrics are present
	if !strings.Contains(output, "smotra_http_requests_total 3") {
		t.Error("expected http_requests_total to be 3")
	}

	if !strings.Contains(output, "smotra_db_connections_open 1") {
		t.Error("expected db_connections_open to be 1")
	}

	// Database should be healthy with mock database
	if !strings.Contains(output, "smotra_db_healthy 1") {
		t.Error("expected db_healthy to be 1 with healthy mock database")
	}

	// Response time should be present
	if !strings.Contains(output, "smotra_db_response_time_ms") {
		t.Error("expected db_response_time_ms to be present with healthy database")
	}

	// Verify Prometheus format
	if !strings.Contains(output, "# HELP") {
		t.Error("expected HELP comments in output")
	}

	if !strings.Contains(output, "# TYPE") {
		t.Error("expected TYPE comments in output")
	}
}

func TestPrometheusMetricsConcurrency_Integration(t *testing.T) {
	logger, _ := testutil.NewTestLogger()

	handler := NewHandler(logger, "1.0.0")

	// Simulate concurrent provider registrations
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			handler.RegisterMetricsProvider(&stubMetricsProvider{output: "concurrent_metric 1\n"})
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	ctx := context.Background()
	resp, err := handler.PrometheusMetrics(ctx, struct{}{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := string(resp.(healthAPI.PrometheusMetrics200TextResponse))

	// All 10 providers should have contributed
	count := strings.Count(output, "concurrent_metric 1")
	if count != 10 {
		t.Errorf("expected concurrent_metric to appear 10 times, got %d\noutput: %s", count, output)
	}
}
