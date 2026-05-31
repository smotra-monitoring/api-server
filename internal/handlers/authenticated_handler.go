package handlers

import (
	"context"
	"fmt"
	"sync/atomic"

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

	// Authentication metrics counters
	authAttemptsTotal        atomic.Uint64
	authNoAuthTotal          atomic.Uint64
	authInvalidTotal         atomic.Uint64
	authAgentIDMismatchTotal atomic.Uint64
	authSuccessTotal         atomic.Uint64
}

// NewAuthenticatedHandler creates a new authenticated handler wrapper
func NewAuthenticatedHandler(logger *logger.Logger, db database.Database, cfg *config.Config, appVersion string, metricsHandler *metrics.Handler) *AuthenticatedHandler {
	h := &AuthenticatedHandler{
		APIHandler: NewAPIHandler(logger, db, cfg, appVersion, metricsHandler),
		logger:     logger,
		db:         db,
	}
	metricsHandler.RegisterMetricsProvider(h)
	return h
}

// GetAgentConfiguration wraps the configuration handler with authentication
func (h *AuthenticatedHandler) GetAgentConfiguration(ctx context.Context, request api.GetAgentConfigurationRequestObject) (api.GetAgentConfigurationResponseObject, error) {
	h.authAttemptsTotal.Add(1)

	// Check if authentication is present in context
	authInfo := ctx.Value(middleware.AuthContextKey)
	if authInfo == nil {
		h.authNoAuthTotal.Add(1)
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
		h.authInvalidTotal.Add(1)
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
		h.authAgentIDMismatchTotal.Add(1)
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
	h.authSuccessTotal.Add(1)
	return h.APIHandler.GetAgentConfiguration(ctx, request)
}

// SubmitAgentResults wraps the submit results handler with authentication
func (h *AuthenticatedHandler) SubmitAgentResults(ctx context.Context, request api.SubmitAgentResultsRequestObject) (api.SubmitAgentResultsResponseObject, error) {
	h.authAttemptsTotal.Add(1)

	authInfo := ctx.Value(middleware.AuthContextKey)
	if authInfo == nil {
		h.authNoAuthTotal.Add(1)
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
		h.authInvalidTotal.Add(1)
		h.logger.Warn("Invalid authentication for submit results endpoint", "agent", request.AgentId.String())
		return api.SubmitAgentResults401JSONResponse{
			UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
				Error:   "unauthorized",
				Message: "Invalid authentication",
			},
		}, nil
	}

	if ctxInfo.AgentID != request.AgentId.String() {
		h.authAgentIDMismatchTotal.Add(1)
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

	h.authSuccessTotal.Add(1)
	return h.APIHandler.SubmitAgentResults(ctx, request)
}

// SendAgentHeartbeat wraps the heartbeat handler with authentication
func (h *AuthenticatedHandler) SendAgentHeartbeat(ctx context.Context, request api.SendAgentHeartbeatRequestObject) (api.SendAgentHeartbeatResponseObject, error) {
	h.authAttemptsTotal.Add(1)

	authInfo := ctx.Value(middleware.AuthContextKey)
	if authInfo == nil {
		h.authNoAuthTotal.Add(1)
		h.logger.Warn("No authentication provided for heartbeat endpoint", "agent", request.AgentId.String())
		return api.SendAgentHeartbeat401JSONResponse{
			UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
				Error:   "unauthorized",
				Message: "No authentication provided",
			},
		}, nil
	}

	ctxInfo, ok := authInfo.(*middleware.AuthInfo)
	if !ok || !ctxInfo.Authenticated {
		h.authInvalidTotal.Add(1)
		h.logger.Warn("Invalid authentication for heartbeat endpoint", "agent", request.AgentId.String())
		return api.SendAgentHeartbeat401JSONResponse{
			UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
				Error:   "unauthorized",
				Message: "Invalid authentication",
			},
		}, nil
	}

	if ctxInfo.AgentID != request.AgentId.String() {
		h.authAgentIDMismatchTotal.Add(1)
		h.logger.Warn("Agent ID mismatch in heartbeat authentication",
			"authenticated_agent", ctxInfo.AgentID,
			"requested_agent", request.AgentId.String(),
		)
		return api.SendAgentHeartbeat503JSONResponse{
			InternalServerErrorJSONResponse: api.InternalServerErrorJSONResponse{
				Error:   "forbidden",
				Message: "Access denied due to internal server error",
			},
		}, nil
	}

	h.authSuccessTotal.Add(1)
	return h.APIHandler.SendAgentHeartbeat(ctx, request)
}

// Oauth2Revoke wraps the revoke handler with authentication.
func (h *AuthenticatedHandler) Oauth2Revoke(ctx context.Context, request api.Oauth2RevokeRequestObject) (api.Oauth2RevokeResponseObject, error) {
	h.authAttemptsTotal.Add(1)

	authInfo, ok := ctx.Value(middleware.AuthContextKey).(*middleware.AuthInfo)
	if !ok || authInfo == nil || !authInfo.Authenticated {
		h.authNoAuthTotal.Add(1)
		return api.Oauth2Revoke401JSONResponse{
			UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
				Error:   "unauthorized",
				Message: "Valid session required",
			},
		}, nil
	}

	if authInfo.AuthType != "oauth2" {
		h.authInvalidTotal.Add(1)
		h.logger.Warn("Invalid authentication type for oauth2 revoke", "auth_type", authInfo.AuthType)
		return api.Oauth2Revoke401JSONResponse{
			UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
				Error:   "unauthorized",
				Message: "Valid session required",
			},
		}, nil
	}

	if authInfo.SessionID == "" {
		h.authInvalidTotal.Add(1)
		h.logger.Warn("Missing session ID in authentication info for oauth2 revoke")
		return api.Oauth2Revoke401JSONResponse{
			UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
				Error:   "unauthorized",
				Message: "Valid session required",
			},
		}, nil
	}

	if authInfo.UserID == "" {
		h.authInvalidTotal.Add(1)
		h.logger.Warn("Missing user ID in authentication info for oauth2 revoke")
		return api.Oauth2Revoke401JSONResponse{
			UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
				Error:   "unauthorized",
				Message: "Valid session required",
			},
		}, nil
	}

	h.authSuccessTotal.Add(1)
	return h.APIHandler.Oauth2Revoke(ctx, request)
}

// AuthRefresh wraps the refresh handler with authentication.
func (h *AuthenticatedHandler) AuthRefresh(ctx context.Context, request api.AuthRefreshRequestObject) (api.AuthRefreshResponseObject, error) {
	h.authAttemptsTotal.Add(1)

	authInfo, ok := ctx.Value(middleware.AuthContextKey).(*middleware.AuthInfo)
	if !ok || authInfo == nil || !authInfo.Authenticated || authInfo.SessionID == "" {
		h.authNoAuthTotal.Add(1)
		return api.AuthRefresh401JSONResponse{
			UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
				Error:   "unauthorized",
				Message: "Valid session required",
			},
		}, nil
	}

	if authInfo.AuthType != "oauth2" {
		h.authInvalidTotal.Add(1)
		h.logger.Warn("Invalid authentication type for refresh", "auth_type", authInfo.AuthType)
		return api.AuthRefresh401JSONResponse{
			UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
				Error:   "unauthorized",
				Message: "Valid session required",
			},
		}, nil
	}

	if authInfo.SessionID == "" {
		h.authInvalidTotal.Add(1)
		h.logger.Warn("Missing session ID in authentication info for refresh")
		return api.AuthRefresh401JSONResponse{
			UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
				Error:   "unauthorized",
				Message: "Valid session required",
			},
		}, nil
	}

	if authInfo.UserID == "" {
		h.authInvalidTotal.Add(1)
		h.logger.Warn("Missing user ID in authentication info for refresh")
		return api.AuthRefresh401JSONResponse{
			UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
				Error:   "unauthorized",
				Message: "Valid session required",
			},
		}, nil
	}

	h.authSuccessTotal.Add(1)
	return h.APIHandler.AuthRefresh(ctx, request)
}

// Logout wraps the logout handler with authentication.
func (h *AuthenticatedHandler) Logout(ctx context.Context, request api.LogoutRequestObject) (api.LogoutResponseObject, error) {
	h.authAttemptsTotal.Add(1)

	authInfo, ok := ctx.Value(middleware.AuthContextKey).(*middleware.AuthInfo)
	if !ok || authInfo == nil || !authInfo.Authenticated {
		h.authNoAuthTotal.Add(1)
		return api.Logout401JSONResponse{
			UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
				Error:   "unauthorized",
				Message: "Valid session required",
			},
		}, nil
	}

	if authInfo.AuthType != "oauth2" {
		h.authInvalidTotal.Add(1)
		h.logger.Warn("Invalid authentication type for logout", "auth_type", authInfo.AuthType)
		return api.Logout401JSONResponse{
			UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
				Error:   "unauthorized",
				Message: "Valid session required",
			},
		}, nil
	}

	if authInfo.SessionID == "" {
		h.authInvalidTotal.Add(1)
		h.logger.Warn("Missing session ID in authentication info for logout")
		return api.Logout401JSONResponse{
			UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
				Error:   "unauthorized",
				Message: "Valid session required",
			},
		}, nil
	}

	if authInfo.UserID == "" {
		h.authInvalidTotal.Add(1)
		h.logger.Warn("Missing user ID in authentication info for logout")
		return api.Logout401JSONResponse{
			UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
				Error:   "unauthorized",
				Message: "Valid session required",
			},
		}, nil
	}

	h.authSuccessTotal.Add(1)
	return h.APIHandler.Logout(ctx, request)
}

// GetUserInfo wraps the userinfo handler with authentication.
func (h *AuthenticatedHandler) GetUserInfo(ctx context.Context, request api.GetUserInfoRequestObject) (api.GetUserInfoResponseObject, error) {
	h.authAttemptsTotal.Add(1)

	authInfo, ok := ctx.Value(middleware.AuthContextKey).(*middleware.AuthInfo)
	if !ok || authInfo == nil || !authInfo.Authenticated {
		h.authNoAuthTotal.Add(1)
		return api.GetUserInfo401JSONResponse{
			UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
				Error:   "unauthorized",
				Message: "Valid session required",
			},
		}, nil
	}

	if authInfo.AuthType != "oauth2" {
		h.authInvalidTotal.Add(1)
		h.logger.Warn("Invalid authentication type for userinfo", "auth_type", authInfo.AuthType)
		return api.GetUserInfo401JSONResponse{
			UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
				Error:   "unauthorized",
				Message: "Valid session required",
			},
		}, nil
	}

	if authInfo.SessionID == "" {
		h.authInvalidTotal.Add(1)
		h.logger.Warn("Missing session ID in authentication info for userinfo")
		return api.GetUserInfo401JSONResponse{
			UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
				Error:   "unauthorized",
				Message: "Valid session required",
			},
		}, nil
	}

	if authInfo.UserID == "" {
		h.authInvalidTotal.Add(1)
		h.logger.Warn("Missing user ID in authentication info for userinfo")
		return api.GetUserInfo401JSONResponse{
			UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
				Error:   "unauthorized",
				Message: "Valid session required",
			},
		}, nil
	}

	h.authSuccessTotal.Add(1)
	return h.APIHandler.GetUserInfo(ctx, request)
}

// ListAgents wraps the agent list handler with authentication.
// Only OAuth2-authenticated users (web sessions) may access this endpoint.
// Agent API keys are explicitly rejected.
func (h *AuthenticatedHandler) ListAgents(ctx context.Context, request api.ListAgentsRequestObject) (api.ListAgentsResponseObject, error) {
	h.authAttemptsTotal.Add(1)

	authInfo, ok := ctx.Value(middleware.AuthContextKey).(*middleware.AuthInfo)
	if !ok || authInfo == nil || !authInfo.Authenticated {
		h.authNoAuthTotal.Add(1)
		return api.ListAgents401JSONResponse{
			UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
				Error:   "unauthorized",
				Message: "Valid session required",
			},
		}, nil
	}

	if authInfo.AuthType != "oauth2" {
		h.authInvalidTotal.Add(1)
		h.logger.Warn("Invalid authentication type for agent list", "auth_type", authInfo.AuthType)
		return api.ListAgents401JSONResponse{
			UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
				Error:   "unauthorized",
				Message: "Valid session required",
			},
		}, nil
	}

	if authInfo.UserID == "" {
		h.authInvalidTotal.Add(1)
		h.logger.Warn("Missing user ID in authentication info for agent list")
		return api.ListAgents401JSONResponse{
			UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
				Error:   "unauthorized",
				Message: "Valid session required",
			},
		}, nil
	}

	h.authSuccessTotal.Add(1)
	return h.APIHandler.ListAgents(ctx, request)
}

// GetMetrics returns current authentication metrics in Prometheus format
func (h *AuthenticatedHandler) GetMetrics() string {
	out := ""

	out += "# HELP smotra_auth_attempts_total Total authentication attempts across all authenticated endpoints\n"
	out += "# TYPE smotra_auth_attempts_total counter\n"
	out += fmt.Sprintf("smotra_auth_attempts_total %d\n", h.authAttemptsTotal.Load())
	out += "\n"

	out += "# HELP smotra_auth_no_auth_total Authentication attempts with no credentials provided\n"
	out += "# TYPE smotra_auth_no_auth_total counter\n"
	out += fmt.Sprintf("smotra_auth_no_auth_total %d\n", h.authNoAuthTotal.Load())
	out += "\n"

	out += "# HELP smotra_auth_invalid_total Authentication attempts with invalid credentials\n"
	out += "# TYPE smotra_auth_invalid_total counter\n"
	out += fmt.Sprintf("smotra_auth_invalid_total %d\n", h.authInvalidTotal.Load())
	out += "\n"

	out += "# HELP smotra_auth_agent_id_mismatch_total Authentication attempts where agent ID does not match token\n"
	out += "# TYPE smotra_auth_agent_id_mismatch_total counter\n"
	out += fmt.Sprintf("smotra_auth_agent_id_mismatch_total %d\n", h.authAgentIDMismatchTotal.Load())
	out += "\n"

	out += "# HELP smotra_auth_success_total Successful authentications\n"
	out += "# TYPE smotra_auth_success_total counter\n"
	out += fmt.Sprintf("smotra_auth_success_total %d\n", h.authSuccessTotal.Load())
	out += "\n"

	return out
}
