package handlers

// health_handlers.go contains handlers for root-level endpoints (/healthz, /metrics)
// that should NOT be prefixed with API version.
//
// These endpoints are typically used by:
// - Kubernetes liveness/readiness probes
// - Prometheus metrics scraping
// - Load balancer health checks
// - Monitoring systems
//
// The handlers are generated from OpenAPI spec using tag filtering (include-tags: health)
// and registered at the root level of the router, not under /v1.

import (
	"context"

	healthAPI "github.com/smotra-monitoring/server/internal/api/health"
	"github.com/smotra-monitoring/server/internal/config"
	"github.com/smotra-monitoring/server/internal/database"
	"github.com/smotra-monitoring/server/internal/handlers/health"
	"github.com/smotra-monitoring/server/internal/handlers/metrics"
	"github.com/smotra-monitoring/server/internal/logger"
)

// NewMetricsHandler creates a new metrics handler
func NewMetricsHandler(logger *logger.Logger, appVersion string) *metrics.Handler {
	return metrics.NewHandler(logger, appVersion)
}

// HealthHandler combines all handler implementations
type HealthHandler struct {
	health  *health.Handler
	metrics *metrics.Handler
}

// NewHealthHandler creates a new health handler
func NewHealthHandler(logger *logger.Logger, db database.Database, cfg *config.Config, appVersion string, metricsHandler *metrics.Handler) *HealthHandler {
	return &HealthHandler{
		health:  health.NewHandler(logger, db, appVersion),
		metrics: metricsHandler,
	}
}

// HealthCheck delegates to health handler
func (h *HealthHandler) HealthCheck(ctx context.Context, request healthAPI.HealthCheckRequestObject) (healthAPI.HealthCheckResponseObject, error) {
	return h.health.HealthCheck(ctx, request)
}

// LivenessCheck delegates to health handler
func (h *HealthHandler) LivenessCheck(ctx context.Context, request healthAPI.LivenessCheckRequestObject) (healthAPI.LivenessCheckResponseObject, error) {
	return h.health.LivenessCheck(ctx, request)
}

// ReadinessCheck delegates to health handler
func (h *HealthHandler) ReadinessCheck(ctx context.Context, request healthAPI.ReadinessCheckRequestObject) (healthAPI.ReadinessCheckResponseObject, error) {
	return h.health.ReadinessCheck(ctx, request)
}

// PrometheusMetrics delegates to metrics handler
func (h *HealthHandler) PrometheusMetrics(ctx context.Context, request healthAPI.PrometheusMetricsRequestObject) (healthAPI.PrometheusMetricsResponseObject, error) {
	return h.metrics.PrometheusMetrics(ctx, request)
}

// SetReady sets the readiness status
func (h *HealthHandler) SetReady(ready bool) {
	h.health.SetReady(ready)
}
