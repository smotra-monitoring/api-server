package handlers

// api_handlers.go contains handlers for versioned API endpoints (/v1/*)
// that represent the core business logic of the monitoring system.
//
// These endpoints include:
// - Agent registration and claiming
// - Agent configuration management
// - Monitoring data submission
// - User management (future)
// - Alert management (future)
//
// The handlers are generated from OpenAPI spec using tag filtering
// (include-tags: current, exclude-tags: health) and registered under
// the /v1 route group.
//
// This separation allows:
// - Clean API versioning (future /v2 won't conflict with health endpoints)
// - Different authentication requirements per endpoint group
// - Independent evolution of monitoring vs business endpoints

import (
	"context"

	api "github.com/smotra-monitoring/server/internal/api/v1"
	"github.com/smotra-monitoring/server/internal/config"
	"github.com/smotra-monitoring/server/internal/database"
	"github.com/smotra-monitoring/server/internal/handlers/agent_claim"
	"github.com/smotra-monitoring/server/internal/handlers/agent_claim_status"
	"github.com/smotra-monitoring/server/internal/handlers/agent_configuration"
	"github.com/smotra-monitoring/server/internal/handlers/agent_register"
	"github.com/smotra-monitoring/server/internal/handlers/agent_submit_results"
	"github.com/smotra-monitoring/server/internal/handlers/metrics"
	"github.com/smotra-monitoring/server/internal/logger"
)

// APIHandler combines all handler implementations
type APIHandler struct {
	metrics               *metrics.Handler
	agent_configuration   *agent_configuration.Handler
	agent_register        *agent_register.Handler
	agent_claim_status    *agent_claim_status.Handler
	agent_claim           *agent_claim.Handler
	agent_submit_results  *agent_submit_results.Handler
}

// NewAPIHandler creates a new combined handler
func NewAPIHandler(logger *logger.Logger, db database.Database, cfg *config.Config, appVersion string, metricsHandler *metrics.Handler) *APIHandler {
	configHandler := agent_configuration.NewHandler(logger, db, appVersion)
	registerHandler := agent_register.NewHandler(logger, db, cfg)
	claimStatusHandler := agent_claim_status.NewHandler(logger, db, cfg)
	claimHandler := agent_claim.NewHandler(logger, db)
	submitResultsHandler := agent_submit_results.NewHandler(logger, db)

	// Register handlers as metrics providers
	metricsHandler.RegisterMetricsProvider(configHandler)
	metricsHandler.RegisterMetricsProvider(registerHandler)
	metricsHandler.RegisterMetricsProvider(claimStatusHandler)
	metricsHandler.RegisterMetricsProvider(claimHandler)
	metricsHandler.RegisterMetricsProvider(submitResultsHandler)

	// Note: Claim-related handlers use string metrics, not metrics provider interface
	// Their metrics are exposed through a different mechanism

	return &APIHandler{
		metrics:              metricsHandler,
		agent_configuration:  configHandler,
		agent_register:       registerHandler,
		agent_claim_status:   claimStatusHandler,
		agent_claim:          claimHandler,
		agent_submit_results: submitResultsHandler,
	}
}

// GetAgentConfiguration delegates to configuration handler
func (h *APIHandler) GetAgentConfiguration(ctx context.Context, request api.GetAgentConfigurationRequestObject) (api.GetAgentConfigurationResponseObject, error) {
	return h.agent_configuration.GetAgentConfiguration(ctx, request)
}

// RegisterAgentSelf delegates to agent register handler
func (h *APIHandler) RegisterAgentSelf(ctx context.Context, request api.RegisterAgentSelfRequestObject) (api.RegisterAgentSelfResponseObject, error) {
	return h.agent_register.Handle(ctx, request)
}

// GetAgentClaimStatus delegates to agent claim status handler
func (h *APIHandler) GetAgentClaimStatus(ctx context.Context, request api.GetAgentClaimStatusRequestObject) (api.GetAgentClaimStatusResponseObject, error) {
	return h.agent_claim_status.Handle(ctx, request)
}

// PostClaimAgent delegates to agent claim handler
func (h *APIHandler) PostClaimAgent(ctx context.Context, request api.PostClaimAgentRequestObject) (api.PostClaimAgentResponseObject, error) {
	return h.agent_claim.Handle(ctx, request)
}

// SubmitAgentResults delegates to submit results handler
func (h *APIHandler) SubmitAgentResults(ctx context.Context, request api.SubmitAgentResultsRequestObject) (api.SubmitAgentResultsResponseObject, error) {
	return h.agent_submit_results.Handle(ctx, request)
}
