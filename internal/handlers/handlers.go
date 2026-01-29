package handlers

import (
	"context"

	"github.com/smotra-monitoring/server/internal/api"
	"github.com/smotra-monitoring/server/internal/database"
	"github.com/smotra-monitoring/server/internal/handlers/agent_configuration"
	"github.com/smotra-monitoring/server/internal/handlers/health"
	"github.com/smotra-monitoring/server/internal/handlers/metrics"
	"github.com/smotra-monitoring/server/internal/logger"
)

// CombinedHandler combines all handler implementations
type CombinedHandler struct {
	health              *health.Handler
	metrics             *metrics.Handler
	agent_configuration *agent_configuration.Handler
}

// NewCombinedHandler creates a new combined handler
func NewCombinedHandler(logger *logger.Logger, db database.Database, appVersion string) *CombinedHandler {
	metricsHandler := metrics.NewHandler(logger, db, appVersion)
	configHandler := agent_configuration.NewHandler(logger, db, appVersion)

	// Register configuration handler as a metrics provider
	metricsHandler.RegisterMetricsProvider(configHandler)

	return &CombinedHandler{
		health:              health.NewHandler(logger, db, appVersion),
		metrics:             metricsHandler,
		agent_configuration: configHandler,
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

// GetAgentConfiguration delegates to configuration handler
func (h *CombinedHandler) GetAgentConfiguration(ctx context.Context, request api.GetAgentConfigurationRequestObject) (api.GetAgentConfigurationResponseObject, error) {
	return h.agent_configuration.GetAgentConfiguration(ctx, request)
}

// SetReady sets the readiness status
func (h *CombinedHandler) SetReady(ready bool) {
	h.health.SetReady(ready)
}
