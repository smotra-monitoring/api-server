package agent_configuration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	api "github.com/smotra-monitoring/server/internal/api/v1"
	"github.com/smotra-monitoring/server/internal/database"
	"github.com/smotra-monitoring/server/internal/database/queries"
	"github.com/smotra-monitoring/server/internal/logger"
	"github.com/smotra-monitoring/server/internal/middleware"
)

// Handler handles agent configuration endpoints
type Handler struct {
	logger *logger.Logger
	db     database.Database

	// Metrics
	getConfigurationTotal   atomic.Uint64
	getConfigurationSuccess atomic.Uint64
	getConfigurationFailure atomic.Uint64
	getConfigurationOAuth2  atomic.Uint64

	// Topology resolution metrics
	topologyResolutionsTotal  atomic.Uint64
	topologyEndpointsResolved atomic.Uint64
	topologyResolutionsEmpty  atomic.Uint64
}

// NewHandler creates a new configuration handler
func NewHandler(logger *logger.Logger, db database.Database, appVersion string) *Handler {
	return &Handler{
		logger: logger.WithComponent("agent_configuration"),
		db:     db,
	}
}

// GetAgentConfiguration implements the GET /agent/{agentId}/configuration endpoint
func (h *Handler) GetAgentConfiguration(ctx context.Context, request api.GetAgentConfigurationRequestObject) (api.GetAgentConfigurationResponseObject, error) {
	h.getConfigurationTotal.Add(1)

	agentID := request.AgentId.String()

	// Create queries object
	q := queries.New(h.db.DB())

	// Get the agent's base configuration
	configRow, err := q.GetAgentConfigurationBase(ctx, agentID)
	if err != nil {
		if err == sql.ErrNoRows {
			h.getConfigurationFailure.Add(1)
			h.logger.Info("Agent not found", "agent_id", agentID)
			return api.GetAgentConfiguration404JSONResponse{
				NotFoundJSONResponse: api.NotFoundJSONResponse{
					Error:   "not_found",
					Message: "Agent not found",
				},
			}, nil
		}
		h.getConfigurationFailure.Add(1)
		h.logger.Error("Failed to get agent configuration", "error", err, "agent_id", agentID)
		return nil, fmt.Errorf("failed to get agent configuration: %w", err)
	}

	// A stored JSON null decodes to a nil map, which would serialise as null instead of {}.
	var baseConfig map[string]any
	if err := json.Unmarshal([]byte(configRow.BaseConfig), &baseConfig); err != nil {
		h.getConfigurationFailure.Add(1)
		h.logger.Error("Failed to parse base configuration", "error", err, "agent_id", agentID)
		return nil, fmt.Errorf("failed to parse base configuration: %w", err)
	}
	if baseConfig == nil {
		baseConfig = make(map[string]any)
	}

	// Get agent tags
	agentTags, err := q.GetAgentTags(ctx, agentID)
	if err != nil {
		h.getConfigurationFailure.Add(1)
		h.logger.Error("Failed to get agent tags", "error", err, "agent_id", agentID)
		return nil, fmt.Errorf("failed to get agent tags: %w", err)
	}

	// nil slice marshals as JSON null; required field must be []
	if agentTags == nil {
		agentTags = []string{}
	}

	// Get agent endpoints resolved via topology membership
	endpointRows, err := q.GetEndpointsForAgent(ctx, agentID)
	h.topologyResolutionsTotal.Add(1)
	if err != nil {
		h.getConfigurationFailure.Add(1)
		h.logger.Error("Failed to get agent endpoints via topology", "error", err, "agent_id", agentID)
		return nil, fmt.Errorf("failed to get agent endpoints: %w", err)
	}
	if len(endpointRows) == 0 {
		h.topologyResolutionsEmpty.Add(1)
	} else {
		h.topologyEndpointsResolved.Add(uint64(len(endpointRows)))
	}

	// Build endpoints with their tags
	endpoints := make([]api.Endpoint, 0, len(endpointRows))
	for _, endpointRow := range endpointRows {
		endpointUUID, err := uuid.Parse(endpointRow.ID)
		if err != nil {
			h.getConfigurationFailure.Add(1)
			h.logger.Error("Failed to parse endpoint ID", "error", err, "endpoint_id", endpointRow.ID)
			return nil, fmt.Errorf("failed to parse endpoint ID: %w", err)
		}

		// Get endpoint tags
		endpointTags, err := q.GetEndpointTags(ctx, endpointRow.ID)
		if err != nil {
			h.getConfigurationFailure.Add(1)
			h.logger.Error("Failed to get endpoint tags", "error", err, "endpoint_id", endpointRow.ID)
			return nil, fmt.Errorf("failed to get endpoint tags: %w", err)
		}

		// Convert enabled from sql.NullInt64 to bool
		enabled := endpointRow.Enabled != 0

		// Convert port from sql.NullInt64 to *int
		var portPtr *int
		if endpointRow.Port.Valid {
			port := int(endpointRow.Port.Int64)
			portPtr = &port
		}

		endpoint := api.Endpoint{
			Id:      endpointUUID,
			Address: endpointRow.Address,
			Port:    portPtr,
			Enabled: enabled,
			Tags:    endpointTags,
		}
		endpoints = append(endpoints, endpoint)
	}

	// Parse agent ID to UUID
	agentUUID, err := uuid.Parse(agentID)
	if err != nil {
		h.getConfigurationFailure.Add(1)
		h.logger.Error("Failed to parse agent ID", "error", err, "agent_id", agentID)
		return nil, fmt.Errorf("failed to parse agent ID: %w", err)
	}

	workspaceUUID, err := uuid.Parse(configRow.WorkspaceID)
	if err != nil {
		h.getConfigurationFailure.Add(1)
		h.logger.Error("Failed to parse workspace ID", "error", err, "agent_id", agentID, "workspace_id", configRow.WorkspaceID)
		return nil, fmt.Errorf("failed to parse workspace ID: %w", err)
	}

	// last_seen_at tracks agent liveness, so a user read must not refresh it.
	{
		authInfo, ok := ctx.Value(middleware.AuthContextKey).(*middleware.AuthInfo)
		if !ok || authInfo == nil {
			h.getConfigurationFailure.Add(1)
			h.logger.Error("Failed to get auth info from context", "agent_id", agentID)
			return nil, fmt.Errorf("failed to get auth info from context")
		}
		switch authInfo.AuthType {
		case "oauth2":
			h.getConfigurationOAuth2.Add(1)
		case "agent_api_key":
			if err := q.UpdateAgentLastSeen(ctx, queries.UpdateAgentLastSeenParams{
				LastSeenAt: sql.NullTime{Time: time.Now(), Valid: true},
				ID:         agentID,
			}); err != nil {
				h.logger.Warn("Failed to update agent last_seen_at", "agent_id", agentID, "error", err.Error())
			}
		default:
			h.logger.Warn("Unknown auth type", "agent_id", agentID, "auth_type", authInfo.AuthType)
			h.getConfigurationFailure.Add(1)
			return nil, fmt.Errorf("unknown auth type: %s", authInfo.AuthType)
		}
	}
	h.getConfigurationSuccess.Add(1)
	h.logger.Info("Agent configuration retrieved", "agent_id", agentID, "config_version", configRow.ConfigVersionDesired)

	return api.GetAgentConfiguration200JSONResponse(api.AgentConfigPatch{
		Id:                   agentUUID,
		WorkspaceId:          workspaceUUID,
		Name:                 configRow.Name,
		ConfigVersionDesired: configRow.ConfigVersionDesired,
		BaseConfig:           baseConfig,
		Tags:                 agentTags,
		Endpoints:            endpoints,
	}), nil
}

// GetMetrics returns the metrics for this handler
func (h *Handler) GetMetrics() string {
	metrics := ""
	metrics += "# HELP smotra_agent_configuration_get_configuration_total Total get configuration requests\n"
	metrics += "# TYPE smotra_agent_configuration_get_configuration_total counter\n"
	metrics += fmt.Sprintf("smotra_agent_configuration_get_configuration_total %d\n", h.getConfigurationTotal.Load())
	metrics += "\n"

	metrics += "# HELP smotra_agent_configuration_get_configuration_success_total Successful get configuration requests\n"
	metrics += "# TYPE smotra_agent_configuration_get_configuration_success_total counter\n"
	metrics += fmt.Sprintf("smotra_agent_configuration_get_configuration_success_total %d\n", h.getConfigurationSuccess.Load())
	metrics += "\n"

	metrics += "# HELP smotra_agent_configuration_get_configuration_failure_total Failed get configuration requests\n"
	metrics += "# TYPE smotra_agent_configuration_get_configuration_failure_total counter\n"
	metrics += fmt.Sprintf("smotra_agent_configuration_get_configuration_failure_total %d\n", h.getConfigurationFailure.Load())
	metrics += "\n"

	metrics += "# HELP smotra_agent_configuration_get_configuration_oauth2_total Get configuration requests made with an OAuth2 session\n"
	metrics += "# TYPE smotra_agent_configuration_get_configuration_oauth2_total counter\n"
	metrics += fmt.Sprintf("smotra_agent_configuration_get_configuration_oauth2_total %d\n", h.getConfigurationOAuth2.Load())
	metrics += "\n"

	metrics += "# HELP smotra_agent_configuration_topology_resolutions_total Total topology resolution calls\n"
	metrics += "# TYPE smotra_agent_configuration_topology_resolutions_total counter\n"
	metrics += fmt.Sprintf("smotra_agent_configuration_topology_resolutions_total %d\n", h.topologyResolutionsTotal.Load())
	metrics += "\n"

	metrics += "# HELP smotra_agent_configuration_topology_endpoints_resolved_total Cumulative endpoints resolved across all configuration requests\n"
	metrics += "# TYPE smotra_agent_configuration_topology_endpoints_resolved_total counter\n"
	metrics += fmt.Sprintf("smotra_agent_configuration_topology_endpoints_resolved_total %d\n", h.topologyEndpointsResolved.Load())
	metrics += "\n"

	metrics += "# HELP smotra_agent_configuration_topology_resolutions_empty_total Configuration requests where the agent had no topology assignment\n"
	metrics += "# TYPE smotra_agent_configuration_topology_resolutions_empty_total counter\n"
	metrics += fmt.Sprintf("smotra_agent_configuration_topology_resolutions_empty_total %d\n", h.topologyResolutionsEmpty.Load())
	metrics += "\n"

	metrics += "\n"

	return metrics
}
