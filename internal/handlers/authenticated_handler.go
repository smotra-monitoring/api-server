package handlers

import (
	"context"

	"github.com/smotra-monitoring/server/internal/api"
	"github.com/smotra-monitoring/server/internal/database"
	"github.com/smotra-monitoring/server/internal/logger"
	"github.com/smotra-monitoring/server/internal/middleware"
)

// AuthenticatedHandler wraps the CombinedHandler to add authentication checks where needed
type AuthenticatedHandler struct {
	*CombinedHandler
	logger *logger.Logger
	db     database.Database
}

// NewAuthenticatedHandler creates a new authenticated handler wrapper
func NewAuthenticatedHandler(logger *logger.Logger, db database.Database, apiVersion string) *AuthenticatedHandler {
	return &AuthenticatedHandler{
		CombinedHandler: NewCombinedHandler(logger, db, apiVersion),
		logger:          logger,
		db:              db,
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

	info, ok := authInfo.(*middleware.AuthInfo)
	if !ok || !info.Authenticated {
		h.logger.Warn("Invalid authentication for agent configuration endpoint", "agent", request.AgentId.String())
		return api.GetAgentConfiguration401JSONResponse{
			UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
				Error:   "unauthorized",
				Message: "Invalid authentication",
			},
		}, nil
	}

	// Verify that the authenticated agent matches the requested agent
	agentID := request.AgentId.String()
	if info.AgentID != agentID {
		h.logger.Warn("Agent ID mismatch in authentication",
			"authenticated_agent", info.AgentID,
			"requested_agent", agentID,
		)
		return api.GetAgentConfiguration503JSONResponse{
			InternalServerErrorJSONResponse: api.InternalServerErrorJSONResponse{
				Error:   "forbidden",
				Message: "Access denied due to internal server error",
			},
		}, nil
	}

	// Authentication successful, delegate to the actual handler
	return h.CombinedHandler.GetAgentConfiguration(ctx, request)
}
