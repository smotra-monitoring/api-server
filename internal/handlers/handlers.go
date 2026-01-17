package handlers

import (
	"context"

	"github.com/smotra-monitoring/server/internal/api"
	"github.com/smotra-monitoring/server/internal/database"
	"github.com/smotra-monitoring/server/internal/handlers/health"
	"github.com/smotra-monitoring/server/internal/handlers/metrics"
	"github.com/smotra-monitoring/server/internal/logger"
)

// CombinedHandler combines all handler implementations
type CombinedHandler struct {
	health  *health.Handler
	metrics *metrics.Handler
}

// NewCombinedHandler creates a new combined handler
func NewCombinedHandler(logger *logger.Logger, db database.Database, version string) *CombinedHandler {
	return &CombinedHandler{
		health:  health.NewHandler(logger, db, version),
		metrics: metrics.NewHandler(logger, db, version),
	}
}

// HealthCheck delegates to health handler
func (h *CombinedHandler) HealthCheck(ctx context.Context, request api.HealthCheckRequestObject) (api.HealthCheckResponseObject, error) {
	return h.health.HealthCheck(ctx, request)
}

// LivenessCheck delegates to health handler
func (h *CombinedHandler) LivenessCheck(ctx context.Context, request api.LivenessCheckRequestObject) (api.LivenessCheckResponseObject, error) {
	return h.health.LivenessCheck(ctx, request)
}

// ReadinessCheck delegates to health handler
func (h *CombinedHandler) ReadinessCheck(ctx context.Context, request api.ReadinessCheckRequestObject) (api.ReadinessCheckResponseObject, error) {
	return h.health.ReadinessCheck(ctx, request)
}

// PrometheusMetrics delegates to metrics handler
func (h *CombinedHandler) PrometheusMetrics(ctx context.Context, request api.PrometheusMetricsRequestObject) (api.PrometheusMetricsResponseObject, error) {
	return h.metrics.PrometheusMetrics(ctx, request)
}

// SetReady sets the readiness status
func (h *CombinedHandler) SetReady(ready bool) {
	h.health.SetReady(ready)
}
