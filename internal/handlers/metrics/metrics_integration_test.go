package metrics

import (
	"context"
	"strings"
	"testing"

	"github.com/smotra-monitoring/server/internal/api"
	"github.com/smotra-monitoring/server/internal/testutil"
)

func TestPrometheusMetrics_Integration(t *testing.T) {
	logger, _ := testutil.NewTestLogger()
	db := testutil.NewMockDatabase()

	handler := NewHandler(logger, db, "1.0.0-integration")

	// Set some metrics
	handler.IncrementHTTPRequests(true)
	handler.IncrementHTTPRequests(true)
	handler.IncrementHTTPRequests(true)
	handler.IncrementDBQueries(true)

	ctx := context.Background()
	resp, err := handler.PrometheusMetrics(ctx, struct{}{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := string(resp.(api.PrometheusMetrics200TextResponse))

	// Verify metrics are present
	if !strings.Contains(output, "smotra_http_requests_total 3") {
		t.Error("expected http_requests_total to be 3")
	}

	if !strings.Contains(output, "smotra_db_queries_total 1") {
		t.Error("expected db_queries_total to be 1")
	}

	// Database should be healthy with real database
	if !strings.Contains(output, "smotra_db_healthy 1") {
		t.Error("expected db_healthy to be 1 with real database")
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
	db := testutil.NewMockDatabase()

	handler := NewHandler(logger, db, "1.0.0")

	// Simulate concurrent metric updates
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				handler.IncrementHTTPRequests(true)
				handler.IncrementDBQueries(true)
			}
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

	output := string(resp.(api.PrometheusMetrics200TextResponse))

	// Verify counters are correct
	if !strings.Contains(output, "smotra_http_requests_total 1000") {
		t.Errorf("expected http_requests_total to be 1000, got output: %s", output)
	}

	if !strings.Contains(output, "smotra_db_queries_total 1000") {
		t.Errorf("expected db_queries_total to be 1000, got output: %s", output)
	}
}
