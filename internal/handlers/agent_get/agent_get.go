package agent_get

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/google/uuid"
	api "github.com/smotra-monitoring/server/internal/api/v1"
	"github.com/smotra-monitoring/server/internal/database"
	"github.com/smotra-monitoring/server/internal/database/queries"
	"github.com/smotra-monitoring/server/internal/logger"
	"github.com/smotra-monitoring/server/internal/middleware"
)

var sessionUnauthorized = api.UnauthorizedJSONResponse{
	Error:   "unauthorized",
	Message: "Valid session required",
}

// Handler handles requests to retrieve a single agent for an authenticated user's workspace.
type Handler struct {
	logger *logger.Logger
	db     database.Database

	// Metrics
	getAttemptsTotal        atomic.Uint64
	getSuccessTotal         atomic.Uint64
	getFailureTotal         atomic.Uint64
	getNotFoundTotal        atomic.Uint64
	getWorkspaceDeniedTotal atomic.Uint64
}

// NewHandler creates a new single-agent get handler.
func NewHandler(log *logger.Logger, db database.Database) *Handler {
	return &Handler{
		logger: log.WithComponent("agent_get"),
		db:     db,
	}
}

// Handle returns all non-sensitive fields for a single agent belonging to the authenticated user's workspace.
func (h *Handler) Handle(ctx context.Context, req api.GetAgentRequestObject) (api.GetAgentResponseObject, error) {
	h.getAttemptsTotal.Add(1)

	// Auth is guaranteed by AuthenticatedHandler; cast directly.
	authInfo := ctx.Value(middleware.AuthContextKey).(*middleware.AuthInfo)
	userID := authInfo.UserID
	agentID := req.AgentId.String()

	q := queries.New(h.db.DB())

	// Fetch the agent first; unknown agent returns 404 before the ownership check.
	row, err := q.GetAgentByID(ctx, agentID)
	if err != nil {
		h.getFailureTotal.Add(1)
		if errors.Is(err, sql.ErrNoRows) {
			h.getNotFoundTotal.Add(1)
			h.logger.WarnContext(ctx, "Agent not found", slog.String("agent_id", agentID), slog.String("user_id", userID))
			return api.GetAgent404JSONResponse{
				NotFoundJSONResponse: api.NotFoundJSONResponse{
					Error:   "not_found",
					Message: "Agent not found",
				},
			}, nil
		}
		h.logger.ErrorContext(ctx, "Failed to retrieve agent", slog.String("agent_id", agentID), slog.String("error", err.Error()))
		return api.GetAgent500JSONResponse{
			InternalServerErrorJSONResponse: api.InternalServerErrorJSONResponse{
				Error:   "database_error",
				Message: "Failed to retrieve agent",
			},
		}, nil
	}

	// Verify workspace ownership: a missing user or foreign workspace returns 401
	// so the response never confirms the agent exists to unauthorized workspaces.
	user, err := q.GetUserByID(ctx, userID)
	if err != nil {
		h.getFailureTotal.Add(1)
		if errors.Is(err, sql.ErrNoRows) {
			h.logger.WarnContext(ctx, "User not found for get agent", slog.String("user_id", userID), slog.String("agent_id", agentID))
			return api.GetAgent401JSONResponse{UnauthorizedJSONResponse: sessionUnauthorized}, nil
		}
		h.logger.ErrorContext(ctx, "Failed to retrieve user", slog.String("user_id", userID), slog.String("agent_id", agentID), slog.String("error", err.Error()))
		return api.GetAgent500JSONResponse{
			InternalServerErrorJSONResponse: api.InternalServerErrorJSONResponse{
				Error:   "database_error",
				Message: "Failed to authorize request",
			},
		}, nil
	}

	if user.WorkspaceID != row.WorkspaceID {
		h.getFailureTotal.Add(1)
		h.getWorkspaceDeniedTotal.Add(1)
		h.logger.WarnContext(ctx, "OAuth2 user denied access to agent outside workspace", slog.String("user_id", userID), slog.String("agent_id", agentID))
		return api.GetAgent401JSONResponse{UnauthorizedJSONResponse: sessionUnauthorized}, nil
	}

	h.getSuccessTotal.Add(1)
	return api.GetAgent200JSONResponse(h.rowToItem(row)), nil
}

// rowToItem converts a database row to an API response item.
func (h *Handler) rowToItem(row queries.GetAgentByIDRow) api.Agent {
	item := api.Agent{
		Id:                   api.UUIDv7(uuid.MustParse(row.ID)),
		WorkspaceId:          api.UUIDv7(uuid.MustParse(row.WorkspaceID)),
		Name:                 row.Name,
		ConfigVersionDesired: row.ConfigVersionDesired,
		CreatedAt:            row.CreatedAt,
		UpdatedAt:            row.UpdatedAt,
	}

	if row.ConfigVersionCurrent.Valid {
		current := row.ConfigVersionCurrent.Int64
		item.ConfigVersionCurrent = &current
	}

	if row.AgentVersion.Valid {
		item.AgentVersion = &row.AgentVersion.String
	}

	if row.LastSeenAt.Valid {
		item.LastSeenAt = &row.LastSeenAt.Time
	}

	if row.LastResultSubmittedAt.Valid {
		item.LastResultSubmittedAt = &row.LastResultSubmittedAt.Time
	}

	// Parse ip_addresses_json into structured slice; silently skip on error.
	if row.IpAddressesJson != "" && row.IpAddressesJson != "[]" {
		var ifaces []api.AgentNetworkInterface
		if err := json.Unmarshal([]byte(row.IpAddressesJson), &ifaces); err == nil && len(ifaces) > 0 {
			item.IpAddresses = &ifaces
		}
	}

	return item
}

// GetMetrics returns current handler metrics in Prometheus format.
func (h *Handler) GetMetrics() string {
	out := ""

	out += "# HELP smotra_agent_get_attempts_total Total requests to get a single agent\n"
	out += "# TYPE smotra_agent_get_attempts_total counter\n"
	out += fmt.Sprintf("smotra_agent_get_attempts_total %d\n", h.getAttemptsTotal.Load())
	out += "\n"

	out += "# HELP smotra_agent_get_success_total Successful get agent responses\n"
	out += "# TYPE smotra_agent_get_success_total counter\n"
	out += fmt.Sprintf("smotra_agent_get_success_total %d\n", h.getSuccessTotal.Load())
	out += "\n"

	out += "# HELP smotra_agent_get_failure_total Failed get agent responses\n"
	out += "# TYPE smotra_agent_get_failure_total counter\n"
	out += fmt.Sprintf("smotra_agent_get_failure_total %d\n", h.getFailureTotal.Load())
	out += "\n"

	out += "# HELP smotra_agent_get_not_found_total Get agent requests where agent was not found\n"
	out += "# TYPE smotra_agent_get_not_found_total counter\n"
	out += fmt.Sprintf("smotra_agent_get_not_found_total %d\n", h.getNotFoundTotal.Load())
	out += "\n"

	out += "# HELP smotra_agent_get_workspace_denied_total Get agent requests denied due to workspace mismatch\n"
	out += "# TYPE smotra_agent_get_workspace_denied_total counter\n"
	out += fmt.Sprintf("smotra_agent_get_workspace_denied_total %d\n", h.getWorkspaceDeniedTotal.Load())

	return out
}
