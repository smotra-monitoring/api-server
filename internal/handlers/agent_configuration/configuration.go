package agent_configuration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/smotra-monitoring/server/internal/api"
	"github.com/smotra-monitoring/server/internal/database"
	"github.com/smotra-monitoring/server/internal/database/queries"
	"github.com/smotra-monitoring/server/internal/logger"
)

// Handler handles agent configuration endpoints
type Handler struct {
	logger  *logger.Logger
	db      database.Database
	version string

	// Metrics
	getConfigurationTotal   atomic.Uint64
	getConfigurationSuccess atomic.Uint64
	getConfigurationFailure atomic.Uint64
}

// NewHandler creates a new configuration handler
func NewHandler(logger *logger.Logger, db database.Database, version string) *Handler {
	return &Handler{
		logger:  logger.WithComponent("agent_configuration"),
		db:      db,
		version: version,
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

	// Parse the base_config JSON into the expected structure
	var baseConfig struct {
		Monitoring api.MonitoringConfig `json:"monitoring"`
		Server     api.ServerConfig     `json:"server"`
		Storage    api.StorageConfig    `json:"storage"`
	}

	if err := json.Unmarshal([]byte(configRow.BaseConfig), &baseConfig); err != nil {
		h.getConfigurationFailure.Add(1)
		h.logger.Error("Failed to parse base configuration", "error", err, "agent_id", agentID)
		return nil, fmt.Errorf("failed to parse base configuration: %w", err)
	}

	// Get agent tags
	agentTags, err := q.GetAgentTags(ctx, agentID)
	if err != nil {
		h.getConfigurationFailure.Add(1)
		h.logger.Error("Failed to get agent tags", "error", err, "agent_id", agentID)
		return nil, fmt.Errorf("failed to get agent tags: %w", err)
	}

	// Get agent endpoints
	endpointRows, err := q.GetAgentEndpoints(ctx, agentID)
	if err != nil {
		h.getConfigurationFailure.Add(1)
		h.logger.Error("Failed to get agent endpoints", "error", err, "agent_id", agentID)
		return nil, fmt.Errorf("failed to get agent endpoints: %w", err)
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

		var endpointTagsPtr *[]string
		if len(endpointTags) > 0 {
			endpointTagsPtr = &endpointTags
		}

		// Convert enabled from sql.NullInt64 to *bool
		var enabledPtr *bool
		if endpointRow.Enabled.Valid {
			enabled := endpointRow.Enabled.Int64 != 0
			enabledPtr = &enabled
		}

		endpoint := api.Endpoint{
			Id:      endpointUUID,
			Address: endpointRow.Address,
			Enabled: enabledPtr,
			Tags:    endpointTagsPtr,
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

	// Build the response
	config := api.AgentConfig{
		Version:    int32(configRow.Version),
		AgentId:    agentUUID,
		AgentName:  configRow.Name,
		Tags:       agentTags,
		Monitoring: baseConfig.Monitoring,
		Server:     baseConfig.Server,
		Storage:    baseConfig.Storage,
		Endpoints:  endpoints,
	}

	h.getConfigurationSuccess.Add(1)
	h.logger.Info("Agent configuration retrieved", "agent_id", agentID, "version", configRow.Version)

	return api.GetAgentConfiguration200JSONResponse(config), nil
}

// GetMetrics returns the metrics for this handler
func (h *Handler) GetMetrics() map[string]uint64 {
	return map[string]uint64{
		"get_configuration_total":   h.getConfigurationTotal.Load(),
		"get_configuration_success": h.getConfigurationSuccess.Load(),
		"get_configuration_failure": h.getConfigurationFailure.Load(),
	}
}
