package metrics

import (
	"context"
	"strings"
	"testing"
	"time"

	healthAPI "github.com/smotra-monitoring/server/internal/api/health"
	"github.com/smotra-monitoring/server/internal/database"
	"github.com/smotra-monitoring/server/internal/testutil"
)

func TestNewHandler(t *testing.T) {
	logger, _ := testutil.NewTestLogger()
	appVersion := "1.0.0-test"

	handler := NewHandler(logger, appVersion)

	if handler == nil {
		t.Fatal("handler is nil")
	}

	if handler.appVersion != appVersion {
		t.Errorf("expected fake version %s, got %s", appVersion, handler.appVersion)
	}

	if handler.logger == nil {
		t.Error("logger is nil")
	}
}

func TestPrometheusMetrics(t *testing.T) {
	logger, _ := testutil.NewTestLogger()
	handler := NewHandler(logger, "1.0.0-test")

	// Register stub providers to simulate HTTP and DB metrics
	handler.RegisterMetricsProvider(&stubMetricsProvider{
		output: "# HELP smotra_http_requests_total Total HTTP requests\n# TYPE smotra_http_requests_total counter\nsmotra_http_requests_total 3\n\n",
	})
	handler.RegisterMetricsProvider(&stubMetricsProvider{
		output: "# HELP smotra_db_connections_open Open DB connections\n# TYPE smotra_db_connections_open gauge\nsmotra_db_connections_open 1\n\n",
	})

	// Sleep a bit to get uptime > 0
	time.Sleep(10 * time.Millisecond)

	ctx := context.Background()
	resp, err := handler.PrometheusMetrics(ctx, struct{}{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := string(resp.(healthAPI.PrometheusMetrics200TextResponse))

	// Verify required core metrics are present
	requiredMetrics := []string{
		"smotra_info",
		"smotra_uptime_seconds",
		"smotra_go_goroutines",
		"smotra_go_memory_alloc_bytes",
		"smotra_agents_registered_total",
		// provider-supplied metrics
		"smotra_http_requests_total 3",
		"smotra_db_connections_open 1",
	}

	for _, metric := range requiredMetrics {
		if !strings.Contains(output, metric) {
			t.Errorf("expected metric %q not found in output", metric)
		}
	}

	if !strings.Contains(output, `version="1.0.0-test"`) {
		t.Error("expected version label to be 1.0.0-test")
	}
}

func TestPrometheusMetricsWithUnhealthyDB(t *testing.T) {
	logger, _ := testutil.NewTestLogger()
	handler := NewHandler(logger, "1.0.0")

	// Simulate database error via DBMetrics provider
	mockDB := testutil.NewMockDatabase()
	mockDB.ShouldFail = true
	handler.RegisterMetricsProvider(database.NewDBMetrics(mockDB))

	ctx := context.Background()
	resp, err := handler.PrometheusMetrics(ctx, struct{}{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := string(resp.(healthAPI.PrometheusMetrics200TextResponse))

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
	handler := NewHandler(logger, "1.0.0")

	ctx := context.Background()
	resp, err := handler.PrometheusMetrics(ctx, struct{}{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := string(resp.(healthAPI.PrometheusMetrics200TextResponse))
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

// ─── RegisterMetricsProvider ──────────────────────────────────────────────────

type stubMetricsProvider struct {
	output string
}

func (s *stubMetricsProvider) GetMetrics() string {
	return s.output
}

func TestRegisterMetricsProvider_OutputIncludedInPrometheus(t *testing.T) {
	logger, _ := testutil.NewTestLogger()
	handler := NewHandler(logger, "1.0.0")

	stub := &stubMetricsProvider{
		output: "# HELP custom_counter Custom test counter\n# TYPE custom_counter counter\ncustom_counter 42\n",
	}
	handler.RegisterMetricsProvider(stub)

	resp, err := handler.PrometheusMetrics(context.Background(), struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := string(resp.(healthAPI.PrometheusMetrics200TextResponse))
	if !strings.Contains(output, "custom_counter 42") {
		t.Errorf("expected provider metrics in output, got:\n%s", output)
	}
}

func TestRegisterMetricsProvider_MultipleProviders(t *testing.T) {
	logger, _ := testutil.NewTestLogger()
	handler := NewHandler(logger, "1.0.0")

	handler.RegisterMetricsProvider(&stubMetricsProvider{output: "provider_a 1\n"})
	handler.RegisterMetricsProvider(&stubMetricsProvider{output: "provider_b 2\n"})

	resp, err := handler.PrometheusMetrics(context.Background(), struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := string(resp.(healthAPI.PrometheusMetrics200TextResponse))
	if !strings.Contains(output, "provider_a 1") {
		t.Errorf("expected provider_a in output")
	}
	if !strings.Contains(output, "provider_b 2") {
		t.Errorf("expected provider_b in output")
	}
}
