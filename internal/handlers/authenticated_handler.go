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

const (
	authTypeAgentAPIKey = "agent_api_key"
	authTypeOAuth2      = "oauth2"
)

var (
	noAuth             = api.UnauthorizedJSONResponse{Error: "unauthorized", Message: "No authentication provided"}
	agentInvalidAuth   = api.UnauthorizedJSONResponse{Error: "unauthorized", Message: "Invalid authentication"}
	sessionWrongAuth   = api.UnauthorizedJSONResponse{Error: "unauthorized", Message: "OAuth2 session required"}
	sessionInvalidUser = api.UnauthorizedJSONResponse{Error: "unauthorized", Message: "Session has no associated user"}
	sessionInvalidID   = api.UnauthorizedJSONResponse{Error: "unauthorized", Message: "Session has no identifier"}
	agentIDMismatch    = api.InternalServerErrorJSONResponse{Error: "forbidden", Message: "Access denied due to internal server error"}
)

// authTypeOf returns the auth type in ctx, or "" when no AuthInfo is present.
func authTypeOf(ctx context.Context) string {
	if authInfo, ok := ctx.Value(middleware.AuthContextKey).(*middleware.AuthInfo); ok && authInfo != nil {
		return authInfo.AuthType
	}
	return ""
}

// rejectAuthType records the failed attempt for a missing or unsupported auth type and returns the 401 body.
func (h *AuthenticatedHandler) rejectAuthType(ctx context.Context, endpoint string) api.UnauthorizedJSONResponse {
	h.authAttemptsTotal.Add(1)

	if authType := authTypeOf(ctx); authType != "" {
		h.authInvalidTotal.Add(1)
		h.logger.Warn("Unsupported authentication type", "endpoint", endpoint, "auth_type", authType)
		return agentInvalidAuth
	}

	h.authNoAuthTotal.Add(1)
	h.logger.Warn("No authentication provided", "endpoint", endpoint)
	return noAuth
}

// requireAgent checks that ctx carries authentication for agentID, recording auth metrics and logging failures.
// It returns the 401 body, or the 503 body for an agent ID mismatch; both are nil on success.
func (h *AuthenticatedHandler) requireAgent(ctx context.Context, endpoint, agentID string) (*api.UnauthorizedJSONResponse, *api.InternalServerErrorJSONResponse) {
	h.authAttemptsTotal.Add(1)

	raw := ctx.Value(middleware.AuthContextKey)
	if raw == nil {
		h.authNoAuthTotal.Add(1)
		h.logger.Warn("No authentication provided", "endpoint", endpoint, "agent", agentID)
		return &noAuth, nil
	}

	authInfo, ok := raw.(*middleware.AuthInfo)
	if !ok || authInfo == nil || !authInfo.Authenticated {
		h.authInvalidTotal.Add(1)
		h.logger.Warn("Invalid authentication", "endpoint", endpoint, "agent", agentID)
		return &agentInvalidAuth, nil
	}

	if authInfo.AuthType != authTypeAgentAPIKey {
		h.authInvalidTotal.Add(1)
		h.logger.Warn("Invalid authentication type (expected "+authTypeAgentAPIKey+")", "endpoint", endpoint, "agent", agentID, "auth_type", authInfo.AuthType)
		return &agentInvalidAuth, nil
	}

	if authInfo.AgentID != agentID {
		h.authAgentIDMismatchTotal.Add(1)
		h.logger.Warn("Agent ID mismatch in authentication",
			"endpoint", endpoint,
			"authenticated_agent", authInfo.AgentID,
			"requested_agent", agentID,
		)
		return nil, &agentIDMismatch
	}

	h.authSuccessTotal.Add(1)
	return nil, nil
}

// requireOAuth2User checks that ctx carries an authenticated OAuth2 session with a user ID,
// recording auth metrics and logging failures. It returns the 401 body, or nil on success.
func (h *AuthenticatedHandler) requireOAuth2User(ctx context.Context, endpoint string) *api.UnauthorizedJSONResponse {
	h.authAttemptsTotal.Add(1)

	authInfo, ok := ctx.Value(middleware.AuthContextKey).(*middleware.AuthInfo)
	if !ok || authInfo == nil || !authInfo.Authenticated {
		h.authNoAuthTotal.Add(1)
		h.logger.Warn("No authentication provided", "endpoint", endpoint)
		return &noAuth
	}
	if authInfo.AuthType != authTypeOAuth2 {
		h.authInvalidTotal.Add(1)
		h.logger.Warn("Invalid authentication type (expected "+authTypeOAuth2+")", "endpoint", endpoint, "auth_type", authInfo.AuthType)
		return &sessionWrongAuth
	}
	if authInfo.UserID == "" {
		h.authInvalidTotal.Add(1)
		h.logger.Warn("Missing user ID in authentication info", "endpoint", endpoint)
		return &sessionInvalidUser
	}

	if authInfo.SessionID == "" {
		h.authInvalidTotal.Add(1)
		h.logger.Warn("Missing session ID in authentication info", "endpoint", endpoint)
		return &sessionInvalidID
	}

	h.authSuccessTotal.Add(1)
	return nil
}

// ---- Either auth type (Agent API Key or OAuth2)

// GetAgentConfiguration wraps the configuration handler with authentication.
// Each supported auth type has its own case; any other type is rejected with 401.
// OAuth2 sessions are additionally scoped to a workspace by the handler.
func (h *AuthenticatedHandler) GetAgentConfiguration(ctx context.Context, request api.GetAgentConfigurationRequestObject) (api.GetAgentConfigurationResponseObject, error) {
	switch authTypeOf(ctx) {
	case authTypeAgentAPIKey:
		if unauth, serverErr := h.requireAgent(ctx, "agent_configuration", request.AgentId.String()); unauth != nil {
			return api.GetAgentConfiguration401JSONResponse{UnauthorizedJSONResponse: *unauth}, nil
		} else if serverErr != nil {
			return api.GetAgentConfiguration503JSONResponse{InternalServerErrorJSONResponse: *serverErr}, nil
		}
	case authTypeOAuth2:
		if unauth := h.requireOAuth2User(ctx, "agent_configuration"); unauth != nil {
			return api.GetAgentConfiguration401JSONResponse{UnauthorizedJSONResponse: *unauth}, nil
		}
	default:
		return api.GetAgentConfiguration401JSONResponse{UnauthorizedJSONResponse: h.rejectAuthType(ctx, "agent_configuration")}, nil
	}
	return h.APIHandler.GetAgentConfiguration(ctx, request)
}

// --- Agent API Key auth only

// SubmitAgentResults wraps the submit results handler with authentication
func (h *AuthenticatedHandler) SubmitAgentResults(ctx context.Context, request api.SubmitAgentResultsRequestObject) (api.SubmitAgentResultsResponseObject, error) {
	if unauth, serverErr := h.requireAgent(ctx, "submit_results", request.AgentId.String()); unauth != nil {
		return api.SubmitAgentResults401JSONResponse{UnauthorizedJSONResponse: *unauth}, nil
	} else if serverErr != nil {
		return api.SubmitAgentResults503JSONResponse{InternalServerErrorJSONResponse: *serverErr}, nil
	}
	return h.APIHandler.SubmitAgentResults(ctx, request)
}

// SendAgentHeartbeat wraps the heartbeat handler with authentication
func (h *AuthenticatedHandler) SendAgentHeartbeat(ctx context.Context, request api.SendAgentHeartbeatRequestObject) (api.SendAgentHeartbeatResponseObject, error) {
	if unauth, serverErr := h.requireAgent(ctx, "heartbeat", request.AgentId.String()); unauth != nil {
		return api.SendAgentHeartbeat401JSONResponse{UnauthorizedJSONResponse: *unauth}, nil
	} else if serverErr != nil {
		return api.SendAgentHeartbeat503JSONResponse{InternalServerErrorJSONResponse: *serverErr}, nil
	}
	return h.APIHandler.SendAgentHeartbeat(ctx, request)
}

// --- OAuth2 auth only

// Oauth2Revoke wraps the revoke handler with authentication.
func (h *AuthenticatedHandler) Oauth2Revoke(ctx context.Context, request api.Oauth2RevokeRequestObject) (api.Oauth2RevokeResponseObject, error) {
	if unauth := h.requireOAuth2User(ctx, "oauth2_revoke"); unauth != nil {
		return api.Oauth2Revoke401JSONResponse{UnauthorizedJSONResponse: *unauth}, nil
	}
	return h.APIHandler.Oauth2Revoke(ctx, request)
}

// AuthRefresh wraps the refresh handler with authentication.
func (h *AuthenticatedHandler) AuthRefresh(ctx context.Context, request api.AuthRefreshRequestObject) (api.AuthRefreshResponseObject, error) {
	if unauth := h.requireOAuth2User(ctx, "auth_refresh"); unauth != nil {
		return api.AuthRefresh401JSONResponse{UnauthorizedJSONResponse: *unauth}, nil
	}
	return h.APIHandler.AuthRefresh(ctx, request)
}

// Logout wraps the logout handler with authentication.
func (h *AuthenticatedHandler) Logout(ctx context.Context, request api.LogoutRequestObject) (api.LogoutResponseObject, error) {
	if unauth := h.requireOAuth2User(ctx, "logout"); unauth != nil {
		return api.Logout401JSONResponse{UnauthorizedJSONResponse: *unauth}, nil
	}
	return h.APIHandler.Logout(ctx, request)
}

// GetUserInfo wraps the userinfo handler with authentication.
func (h *AuthenticatedHandler) GetUserInfo(ctx context.Context, request api.GetUserInfoRequestObject) (api.GetUserInfoResponseObject, error) {
	if unauth := h.requireOAuth2User(ctx, "userinfo"); unauth != nil {
		return api.GetUserInfo401JSONResponse{UnauthorizedJSONResponse: *unauth}, nil
	}
	return h.APIHandler.GetUserInfo(ctx, request)
}

// PostClaimAgent wraps the agent claim handler with authentication.
// Only OAuth2-authenticated users (web sessions) may claim agents.
// Agent API keys are explicitly rejected.
func (h *AuthenticatedHandler) PostClaimAgent(ctx context.Context, request api.PostClaimAgentRequestObject) (api.PostClaimAgentResponseObject, error) {
	if unauth := h.requireOAuth2User(ctx, "claim_agent"); unauth != nil {
		return api.PostClaimAgent401JSONResponse{UnauthorizedJSONResponse: *unauth}, nil
	}
	return h.APIHandler.PostClaimAgent(ctx, request)
}

// ListAgents wraps the agent list handler with authentication.
// Only OAuth2-authenticated users (web sessions) may access this endpoint.
// Agent API keys are explicitly rejected.
func (h *AuthenticatedHandler) ListAgents(ctx context.Context, request api.ListAgentsRequestObject) (api.ListAgentsResponseObject, error) {
	if unauth := h.requireOAuth2User(ctx, "list_agents"); unauth != nil {
		return api.ListAgents401JSONResponse{UnauthorizedJSONResponse: *unauth}, nil
	}
	return h.APIHandler.ListAgents(ctx, request)
}

// GetAgent wraps the single-agent get handler with authentication.
// Only OAuth2-authenticated users (web sessions) may access this endpoint.
// Agent API keys are explicitly rejected.
func (h *AuthenticatedHandler) GetAgent(ctx context.Context, request api.GetAgentRequestObject) (api.GetAgentResponseObject, error) {
	if unauth := h.requireOAuth2User(ctx, "get_agent"); unauth != nil {
		return api.GetAgent401JSONResponse{UnauthorizedJSONResponse: *unauth}, nil
	}
	return h.APIHandler.GetAgent(ctx, request)
}

// ListSessions wraps the session list handler; OAuth2 sessions only.
func (h *AuthenticatedHandler) ListSessions(ctx context.Context, request api.ListSessionsRequestObject) (api.ListSessionsResponseObject, error) {
	if unauth := h.requireOAuth2User(ctx, "list_sessions"); unauth != nil {
		return api.ListSessions401JSONResponse{UnauthorizedJSONResponse: *unauth}, nil
	}
	return h.APIHandler.ListSessions(ctx, request)
}

// PurgeSession wraps the single-session purge handler; OAuth2 sessions only.
func (h *AuthenticatedHandler) PurgeSession(ctx context.Context, request api.PurgeSessionRequestObject) (api.PurgeSessionResponseObject, error) {
	if unauth := h.requireOAuth2User(ctx, "purge_session"); unauth != nil {
		return api.PurgeSession401JSONResponse{UnauthorizedJSONResponse: *unauth}, nil
	}
	return h.APIHandler.PurgeSession(ctx, request)
}

// PurgeSessions wraps the purge-all handler; OAuth2 sessions only.
func (h *AuthenticatedHandler) PurgeSessions(ctx context.Context, request api.PurgeSessionsRequestObject) (api.PurgeSessionsResponseObject, error) {
	if unauth := h.requireOAuth2User(ctx, "purge_sessions"); unauth != nil {
		return api.PurgeSessions401JSONResponse{UnauthorizedJSONResponse: *unauth}, nil
	}
	return h.APIHandler.PurgeSessions(ctx, request)
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
