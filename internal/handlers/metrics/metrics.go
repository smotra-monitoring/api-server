package metrics

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/smotra-monitoring/server/internal/api"
	"github.com/smotra-monitoring/server/internal/database"
	"github.com/smotra-monitoring/server/internal/logger"
)

// MetricsProvider defines an interface for handlers that provide metrics
type MetricsProvider interface {
	GetMetrics() map[string]uint64
	GetTitle() string
}

// Handler handles metrics endpoint
type Handler struct {
	logger     *logger.Logger
	db         database.Database
	startTime  time.Time
	appVersion string

	// Metrics counters
	httpRequestsTotal   atomic.Uint64
	httpRequestsSuccess atomic.Uint64
	httpRequestsFailure atomic.Uint64

	// Database metrics
	dbQueriesTotal   atomic.Uint64
	dbQueriesSuccess atomic.Uint64
	dbQueriesFailure atomic.Uint64

	// Agent metrics (will be populated from database)
	agentMetrics sync.Map

	// External metrics providers
	metricsProviders []MetricsProvider
}

// NewHandler creates a new metrics handler
func NewHandler(logger *logger.Logger, db database.Database, appVersion string) *Handler {
	return &Handler{
		logger:           logger.WithComponent("metrics"),
		db:               db,
		startTime:        time.Now(),
		appVersion:       appVersion,
		metricsProviders: []MetricsProvider{},
	}
}

// RegisterMetricsProvider registers a metrics provider
func (h *Handler) RegisterMetricsProvider(provider MetricsProvider) {
	h.metricsProviders = append(h.metricsProviders, provider)
}

// IncrementHTTPRequests increments HTTP request counters
func (h *Handler) IncrementHTTPRequests(success bool) {
	h.httpRequestsTotal.Add(1)
	if success {
		h.httpRequestsSuccess.Add(1)
	} else {
		h.httpRequestsFailure.Add(1)
	}
}

// IncrementDBQueries increments database query counters
func (h *Handler) IncrementDBQueries(success bool) {
	h.dbQueriesTotal.Add(1)
	if success {
		h.dbQueriesSuccess.Add(1)
	} else {
		h.dbQueriesFailure.Add(1)
	}
}

// PrometheusMetrics implements the /metrics endpoint
func (h *Handler) PrometheusMetrics(ctx context.Context, request api.PrometheusMetricsRequestObject) (api.PrometheusMetricsResponseObject, error) {
	metrics := h.buildPrometheusMetrics(ctx)
	return api.PrometheusMetrics200TextResponse(metrics), nil
}

func (h *Handler) buildPrometheusMetrics(ctx context.Context) string {
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

	// HTTP metrics
	output += "# HELP smotra_http_requests_total Total number of HTTP requests\n"
	output += "# TYPE smotra_http_requests_total counter\n"
	output += fmt.Sprintf("smotra_http_requests_total %d\n", h.httpRequestsTotal.Load())
	output += "\n"

	output += "# HELP smotra_http_requests_success_total Total number of successful HTTP requests\n"
	output += "# TYPE smotra_http_requests_success_total counter\n"
	output += fmt.Sprintf("smotra_http_requests_success_total %d\n", h.httpRequestsSuccess.Load())
	output += "\n"

	output += "# HELP smotra_http_requests_failure_total Total number of failed HTTP requests\n"
	output += "# TYPE smotra_http_requests_failure_total counter\n"
	output += fmt.Sprintf("smotra_http_requests_failure_total %d\n", h.httpRequestsFailure.Load())
	output += "\n"

	// Database metrics
	output += "# HELP smotra_db_queries_total Total number of database queries\n"
	output += "# TYPE smotra_db_queries_total counter\n"
	output += fmt.Sprintf("smotra_db_queries_total %d\n", h.dbQueriesTotal.Load())
	output += "\n"

	output += "# HELP smotra_db_queries_success_total Total number of successful database queries\n"
	output += "# TYPE smotra_db_queries_success_total counter\n"
	output += fmt.Sprintf("smotra_db_queries_success_total %d\n", h.dbQueriesSuccess.Load())
	output += "\n"

	output += "# HELP smotra_db_queries_failure_total Total number of failed database queries\n"
	output += "# TYPE smotra_db_queries_failure_total counter\n"
	output += fmt.Sprintf("smotra_db_queries_failure_total %d\n", h.dbQueriesFailure.Load())
	output += "\n"

	// Database health check
	if h.db != nil {
		dbCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()

		dbHealth, err := h.db.Health(dbCtx)
		dbHealthy := 0.0
		if err == nil {
			dbHealthy = 1.0
		}

		output += "# HELP smotra_db_healthy Database health status (1 = healthy, 0 = unhealthy)\n"
		output += "# TYPE smotra_db_healthy gauge\n"
		output += fmt.Sprintf("smotra_db_healthy %.0f\n", dbHealthy)
		output += "\n"

		if err == nil {
			output += "# HELP smotra_db_response_time_ms Database response time in milliseconds\n"
			output += "# TYPE smotra_db_response_time_ms gauge\n"
			output += fmt.Sprintf("smotra_db_response_time_ms %.2f\n", float64(dbHealth.ResponseTime.Milliseconds()))
			output += "\n"
		}
	}

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
		title := provider.GetTitle()
		metrics := provider.GetMetrics()

		if getConfigTotal, ok := metrics["get_configuration_total"]; ok {
			output += fmt.Sprintf("# HELP smotra_%s_get_total Total number of GET configuration requests\n", title)
			output += fmt.Sprintf("# TYPE smotra_%s_get_total counter\n", title)
			output += fmt.Sprintf("smotra_%s_get_total %d\n", title, getConfigTotal)
			output += "\n"
		}

		if getConfigSuccess, ok := metrics["get_configuration_success"]; ok {
			output += fmt.Sprintf("# HELP smotra_%s_get_success_total Total number of successful GET configuration requests\n", title)
			output += fmt.Sprintf("# TYPE smotra_%s_get_success_total counter\n", title)
			output += fmt.Sprintf("smotra_%s_get_success_total %d\n", title, getConfigSuccess)
			output += "\n"
		}

		if getConfigFailure, ok := metrics["get_configuration_failure"]; ok {
			output += fmt.Sprintf("# HELP smotra_%s_get_failure_total Total number of failed GET configuration requests\n", title)
			output += fmt.Sprintf("# TYPE smotra_%s_get_failure_total counter\n", title)
			output += fmt.Sprintf("smotra_%s_get_failure_total %d\n", title, getConfigFailure)
			output += "\n"
		}
	}

	return output
}
