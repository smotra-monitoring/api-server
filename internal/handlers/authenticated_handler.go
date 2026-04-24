package handlers

import (
	"context"

	api "github.com/smotra-monitoring/server/internal/api/v1"
	"github.com/smotra-monitoring/server/internal/config"
	"github.com/smotra-monitoring/server/internal/database"
	"github.com/smotra-monitoring/server/internal/handlers/metrics"
	"github.com/smotra-monitoring/server/internal/logger"
	"github.com/smotra-monitoring/server/internal/middleware"
)

// AuthenticatedHandler wraps the CombinedHandler to add authentication checks where needed
type AuthenticatedHandler struct {
	*APIHandler
	logger *logger.Logger
	db     database.Database
}

// NewAuthenticatedHandler creates a new authenticated handler wrapper
func NewAuthenticatedHandler(logger *logger.Logger, db database.Database, cfg *config.Config, apiVersion string, metricsHandler *metrics.Handler) *AuthenticatedHandler {
	return &AuthenticatedHandler{
		APIHandler: NewAPIHandler(logger, db, cfg, apiVersion, metricsHandler),
		logger:     logger,
		db:         db,
	}
}

// GetAgentConfiguration wraps the configuration handler with authentication
func (h *AuthenticatedHandler) GetAgentConfiguration(ctx context.Context, request api.GetAgentConfigurationRequestObject) (api.GetAgentConfigurationResponseObject, error) {
	// Check if authentication is present in context
	authInfo := ctx.Value(middleware.AuthContextKey)
	if authInfo == nil {
		h.logger.Warn("No authentication provided for agent configuration endpoint", "agent", request.AgentId.String())
		return api.GetAgentConfiguration401JSONResponse{
			UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
				Error:   "unauthorized",
				Message: "No authentication provided",
			},
		}, nil
	}

	ctxInfo, ok := authInfo.(*middleware.AuthInfo)
	if !ok || !ctxInfo.Authenticated {
		h.logger.Warn("Invalid authentication for agent configuration endpoint", "agent", request.AgentId.String())
		return api.GetAgentConfiguration401JSONResponse{
			UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
				Error:   "unauthorized",
				Message: "Invalid authentication",
			},
		}, nil
	}

	// Verify that the authenticated agent matches the requested agent
	reqAgentID := request.AgentId.String()
	if ctxInfo.AgentID != reqAgentID {
		h.logger.Warn("Agent ID mismatch in authentication",
			"authenticated_agent", ctxInfo.AgentID,
			"requested_agent", reqAgentID,
		)
		return api.GetAgentConfiguration503JSONResponse{
			InternalServerErrorJSONResponse: api.InternalServerErrorJSONResponse{
				Error:   "forbidden",
				Message: "Access denied due to internal server error",
			},
		}, nil
	}

	// Authentication successful, delegate to the actual handler
	return h.APIHandler.GetAgentConfiguration(ctx, request)
}

// SubmitAgentResults wraps the submit results handler with authentication
func (h *AuthenticatedHandler) SubmitAgentResults(ctx context.Context, request api.SubmitAgentResultsRequestObject) (api.SubmitAgentResultsResponseObject, error) {
	authInfo := ctx.Value(middleware.AuthContextKey)
	if authInfo == nil {
		h.logger.Warn("No authentication provided for submit results endpoint", "agent", request.AgentId.String())
		return api.SubmitAgentResults401JSONResponse{
			UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
				Error:   "unauthorized",
				Message: "No authentication provided",
			},
		}, nil
	}

	ctxInfo, ok := authInfo.(*middleware.AuthInfo)
	if !ok || !ctxInfo.Authenticated {
		h.logger.Warn("Invalid authentication for submit results endpoint", "agent", request.AgentId.String())
		return api.SubmitAgentResults401JSONResponse{
			UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
				Error:   "unauthorized",
				Message: "Invalid authentication",
			},
		}, nil
	}

	if ctxInfo.AgentID != request.AgentId.String() {
		h.logger.Warn("Agent ID mismatch in submit results authentication",
			"authenticated_agent", ctxInfo.AgentID,
			"requested_agent", request.AgentId.String(),
		)
		return api.SubmitAgentResults503JSONResponse{
			InternalServerErrorJSONResponse: api.InternalServerErrorJSONResponse{
				Error:   "forbidden",
				Message: "Access denied due to internal server error",
			},
		}, nil
	}

	return h.APIHandler.SubmitAgentResults(ctx, request)
}
