package handlers

import (
	"context"

	"github.com/smotra-monitoring/server/internal/api"
	"github.com/smotra-monitoring/server/internal/config"
	"github.com/smotra-monitoring/server/internal/database"
	"github.com/smotra-monitoring/server/internal/handlers/agent_claim"
	"github.com/smotra-monitoring/server/internal/handlers/agent_claim_status"
	"github.com/smotra-monitoring/server/internal/handlers/agent_configuration"
	"github.com/smotra-monitoring/server/internal/handlers/agent_register"
	"github.com/smotra-monitoring/server/internal/handlers/health"
	"github.com/smotra-monitoring/server/internal/handlers/metrics"
	"github.com/smotra-monitoring/server/internal/logger"
)

// CombinedHandler combines all handler implementations
type CombinedHandler struct {
	health              *health.Handler
	metrics             *metrics.Handler
	agent_configuration *agent_configuration.Handler
	agent_register      *agent_register.Handler
	agent_claim_status  *agent_claim_status.Handler
	agent_claim         *agent_claim.Handler
}

// NewCombinedHandler creates a new combined handler
func NewCombinedHandler(logger *logger.Logger, db database.Database, cfg *config.Config, appVersion string) *CombinedHandler {
	metricsHandler := metrics.NewHandler(logger, db, appVersion)
	configHandler := agent_configuration.NewHandler(logger, db, appVersion)
	registerHandler := agent_register.NewHandler(logger, db, cfg)
	claimStatusHandler := agent_claim_status.NewHandler(logger, db)
	claimHandler := agent_claim.NewHandler(logger, db)

	// Register handlers as metrics providers
	metricsHandler.RegisterMetricsProvider(configHandler)
	metricsHandler.RegisterMetricsProvider(registerHandler)
	metricsHandler.RegisterMetricsProvider(claimStatusHandler)
	metricsHandler.RegisterMetricsProvider(claimHandler)

	// Note: Claim-related handlers use string metrics, not metrics provider interface
	// Their metrics are exposed through a different mechanism

	return &CombinedHandler{
		health:              health.NewHandler(logger, db, appVersion),
		metrics:             metricsHandler,
		agent_configuration: configHandler,
		agent_register:      registerHandler,
		agent_claim_status:  claimStatusHandler,
		agent_claim:         claimHandler,
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

// RegisterAgentSelf delegates to agent register handler
func (h *CombinedHandler) RegisterAgentSelf(ctx context.Context, request api.RegisterAgentSelfRequestObject) (api.RegisterAgentSelfResponseObject, error) {
	return h.agent_register.Handle(ctx, request)
}

// GetAgentClaimStatus delegates to agent claim status handler
func (h *CombinedHandler) GetAgentClaimStatus(ctx context.Context, request api.GetAgentClaimStatusRequestObject) (api.GetAgentClaimStatusResponseObject, error) {
	return h.agent_claim_status.Handle(ctx, request)
}

// ClaimAgent delegates to agent claim handler
func (h *CombinedHandler) ClaimAgent(ctx context.Context, request api.ClaimAgentRequestObject) (api.ClaimAgentResponseObject, error) {
	return h.agent_claim.Handle(ctx, request)
}

// SetReady sets the readiness status
func (h *CombinedHandler) SetReady(ready bool) {
	h.health.SetReady(ready)
}
