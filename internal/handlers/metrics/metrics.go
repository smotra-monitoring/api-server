package metrics

import (
	"context"
	"fmt"
	"runtime"
	"time"

	healthAPI "github.com/smotra-monitoring/server/internal/api/health"
	"github.com/smotra-monitoring/server/internal/logger"
)

// MetricsProvider defines an interface for handlers that provide metrics
type MetricsProvider interface {
	GetMetrics() string
}

// Handler handles metrics endpoint
type Handler struct {
	logger     *logger.Logger
	startTime  time.Time
	appVersion string

	// External metrics providers
	metricsProviders []MetricsProvider
}

// NewHandler creates a new metrics handler
func NewHandler(logger *logger.Logger, appVersion string) *Handler {
	return &Handler{
		logger:           logger.WithComponent("metrics"),
		startTime:        time.Now(),
		appVersion:       appVersion,
		metricsProviders: []MetricsProvider{},
	}
}

// RegisterMetricsProvider registers a metrics provider
func (h *Handler) RegisterMetricsProvider(provider MetricsProvider) {
	h.metricsProviders = append(h.metricsProviders, provider)
}

// PrometheusMetrics implements the /metrics endpoint
func (h *Handler) PrometheusMetrics(ctx context.Context, request healthAPI.PrometheusMetricsRequestObject) (healthAPI.PrometheusMetricsResponseObject, error) {
	metrics := h.buildPrometheusMetrics()
	return healthAPI.PrometheusMetrics200TextResponse(metrics), nil
}

func (h *Handler) buildPrometheusMetrics() string {
	var output string

	// Server info
	output += "# HELP smotra_info Server information\n"
	output += "# TYPE smotra_info gauge\n"
	output += fmt.Sprintf("smotra_info{version=\"%s\"} 1\n", h.appVersion)
	output += "\n"

	// Uptime
	uptime := time.Since(h.startTime).Seconds()
	output += "# HELP smotra_uptime_seconds Server uptime in seconds\n"
	output += "# TYPE smotra_uptime_seconds counter\n"
	output += fmt.Sprintf("smotra_uptime_seconds %.2f\n", uptime)
	output += "\n"

	// Go runtime metrics
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	output += "# HELP smotra_go_goroutines Number of goroutines\n"
	output += "# TYPE smotra_go_goroutines gauge\n"
	output += fmt.Sprintf("smotra_go_goroutines %d\n", runtime.NumGoroutine())
	output += "\n"

	output += "# HELP smotra_go_threads Number of OS threads\n"
	output += "# TYPE smotra_go_threads gauge\n"
	output += fmt.Sprintf("smotra_go_threads %d\n", runtime.GOMAXPROCS(0))
	output += "\n"

	output += "# HELP smotra_go_memory_alloc_bytes Bytes of allocated heap objects\n"
	output += "# TYPE smotra_go_memory_alloc_bytes gauge\n"
	output += fmt.Sprintf("smotra_go_memory_alloc_bytes %d\n", m.Alloc)
	output += "\n"

	output += "# HELP smotra_go_memory_total_alloc_bytes Cumulative bytes allocated for heap objects\n"
	output += "# TYPE smotra_go_memory_total_alloc_bytes counter\n"
	output += fmt.Sprintf("smotra_go_memory_total_alloc_bytes %d\n", m.TotalAlloc)
	output += "\n"

	output += "# HELP smotra_go_memory_sys_bytes Total bytes of memory obtained from OS\n"
	output += "# TYPE smotra_go_memory_sys_bytes gauge\n"
	output += fmt.Sprintf("smotra_go_memory_sys_bytes %d\n", m.Sys)
	output += "\n"

	output += "# HELP smotra_go_gc_runs_total Total number of GC runs\n"
	output += "# TYPE smotra_go_gc_runs_total counter\n"
	output += fmt.Sprintf("smotra_go_gc_runs_total %d\n", m.NumGC)
	output += "\n"

	// Agent metrics (placeholder - will be populated from database in future)
	output += "# HELP smotra_agents_registered_total Total number of registered agents\n"
	output += "# TYPE smotra_agents_registered_total gauge\n"
	output += "smotra_agents_registered_total 0\n"
	output += "\n"

	output += "# HELP smotra_checks_total Total number of checks performed\n"
	output += "# TYPE smotra_checks_total counter\n"
	output += "smotra_checks_total{status=\"success\"} 0\n"
	output += "smotra_checks_total{status=\"failure\"} 0\n"
	output += "\n"

	// Configuration handler metrics
	for _, provider := range h.metricsProviders {
		output += provider.GetMetrics()
	}

	return output
}
