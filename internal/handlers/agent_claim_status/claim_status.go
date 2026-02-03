package agent_claim_status

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"

	api "github.com/smotra-monitoring/server/internal/api/v1"
	"github.com/smotra-monitoring/server/internal/database"
	"github.com/smotra-monitoring/server/internal/database/queries"
	"github.com/smotra-monitoring/server/internal/logger"
)

// Handler handles agent claim status polling requests
type Handler struct {
	logger *logger.Logger
	db     database.Database

	// Metrics
	pollAttemptsTotal         atomic.Uint64
	pollPendingTotal          atomic.Uint64
	pollFailedTotal           atomic.Uint64
	pollAlreadyDeliveredTotal atomic.Uint64
	pollNotFoundTotal         atomic.Uint64
	apiKeyDeliveryTotal       atomic.Uint64
}

// NewHandler creates a new agent claim status handler
func NewHandler(logger *logger.Logger, db database.Database) *Handler {
	return &Handler{
		logger: logger.WithComponent("agent_claim_status"),
		db:     db,
	}
}

// Handle processes agent claim status polling requests
func (h *Handler) Handle(ctx context.Context, req api.GetAgentClaimStatusRequestObject) (api.GetAgentClaimStatusResponseObject, error) {
	h.pollAttemptsTotal.Add(1)

	q := queries.New(h.db.DB())

	agentIDStr := req.AgentId.String()

	// Get agent claim
	claim, err := q.GetAgentClaim(ctx, agentIDStr)
	if err != nil {
		if err == sql.ErrNoRows {
			h.pollNotFoundTotal.Add(1)
			return api.GetAgentClaimStatus404JSONResponse(api.Error{
				Error:   "not_found",
				Message: "Agent registration not found or has expired",
			}), nil
		}

		h.logger.ErrorContext(ctx, "Failed to get agent claim",
			slog.String("agentId", agentIDStr),
			slog.String("error", err.Error()),
		)
		h.pollFailedTotal.Add(1)

		return api.GetAgentClaimStatus500JSONResponse{
			InternalServerErrorJSONResponse: api.InternalServerErrorJSONResponse{
				Error:   "internal_error",
				Message: "Failed to retrieve claim status",
			},
		}, nil
	}

	// Check if claimed
	if !claim.ClaimedAt.Valid {
		// Still pending
		h.pollPendingTotal.Add(1)

		pending := api.ClaimStatusPending{
			Status: "pending_claim",
		}

		return newClaimStatus200Response(pending)
	}

	// Agent has been claimed - check if API key was already delivered
	if claim.ApiKeyDelivered != 0 {
		h.pollAlreadyDeliveredTotal.Add(1)
		h.logger.InfoContext(ctx, "Agent claim status checked - already delivered",
			slog.String("agentId", agentIDStr),
		)

		// API key already delivered, return pending status
		// (agent should stop polling after receiving the key once)
		pending := api.ClaimStatusPending{
			Status: "pending_claim",
		}
		return newClaimStatus200Response(pending)
	}

	//
	// Agent claimed, but API key not yet delivered
	//

	// Get the agent record to retrieve API key
	pendingDelivery, err := q.GetPendingAPIKeyDelivery(ctx, agentIDStr)
	if err != nil {
		if err == sql.ErrNoRows {
			// API key already delivered or not yet ready
			h.pollFailedTotal.Add(1)
			return api.GetAgentClaimStatus500JSONResponse{
				InternalServerErrorJSONResponse: api.InternalServerErrorJSONResponse{
					Error:   "internal_error",
					Message: "Failed to retrieve API key",
				},
			}, nil
		}

		h.pollFailedTotal.Add(1)
		h.logger.ErrorContext(ctx, "Failed to retrieve pending API key for delivery",
			slog.String("agentId", agentIDStr),
			slog.String("error", err.Error()),
		)
		return api.GetAgentClaimStatus500JSONResponse{
			InternalServerErrorJSONResponse: api.InternalServerErrorJSONResponse{
				Error:   "internal_error",
				Message: "Failed to retrieve API key",
			},
		}, nil
	}

	// Validate we have a plaintext key
	if !pendingDelivery.ApiKeyPlaintext.Valid || pendingDelivery.ApiKeyPlaintext.String == "" {
		h.pollFailedTotal.Add(1)
		h.logger.ErrorContext(ctx, "API key plaintext is missing",
			slog.String("agentId", agentIDStr),
		)
		return api.GetAgentClaimStatus500JSONResponse{
			InternalServerErrorJSONResponse: api.InternalServerErrorJSONResponse{
				Error:   "internal_error",
				Message: "API key not available",
			},
		}, nil
	}

	// Mark API key as delivered (this will clear the plaintext)
	err = q.MarkAgentClaimAPIKeyDelivered(ctx, agentIDStr)
	if err != nil {
		h.pollFailedTotal.Add(1)
		h.logger.ErrorContext(ctx, "Failed to mark API key as delivered",
			slog.String("agentId", agentIDStr),
			slog.String("error", err.Error()),
		)
		// Continue anyway - the key will still be delivered
	}

	h.apiKeyDeliveryTotal.Add(1)

	claimed := api.ClaimStatusClaimed{
		Status:    "claimed",
		ApiKey:    pendingDelivery.ApiKeyPlaintext.String,
		ConfigUrl: fmt.Sprintf("/agents/%s/configuration", agentIDStr),
	}

	h.logger.InfoContext(ctx, "API key delivered to agent",
		slog.String("agentId", agentIDStr),
	)

	return newClaimStatus200Response(claimed)
}

// newClaimStatus200Response creates a GetAgentClaimStatus200JSONResponse from either
// ClaimStatusPending or ClaimStatusClaimed by marshaling to JSON
func newClaimStatus200Response(status interface{}) (api.GetAgentClaimStatusResponseObject, error) {
	jsonData, err := json.Marshal(status)
	if err != nil {
		return api.GetAgentClaimStatus500JSONResponse{
			InternalServerErrorJSONResponse: api.InternalServerErrorJSONResponse{
				Error:   "internal_error",
				Message: "Failed to marshal response",
			},
		}, nil
	}

	// Return the custom response wrapper that implements the interface
	return &claimStatusResponse{data: jsonData}, nil
}

// claimStatusResponse implements GetAgentClaimStatusResponseObject
type claimStatusResponse struct {
	data json.RawMessage
}

func (r *claimStatusResponse) VisitGetAgentClaimStatusResponse(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	_, err := w.Write(r.data)
	return err
}

// GetMetrics returns current handler metrics in Prometheus format
func (h *Handler) GetMetrics() string {
	metrics := ""
	metrics += "# HELP smotra_agent_claim_status_poll_attempts_total Total claim status poll attempts\n"
	metrics += "# TYPE smotra_agent_claim_status_poll_attempts_total counter\n"
	metrics += fmt.Sprintf("smotra_agent_claim_status_poll_attempts_total %d\n", h.pollAttemptsTotal.Load())
	metrics += "\n"

	metrics += "# HELP smotra_agent_claim_status_poll_failed_total Failed claim status polls\n"
	metrics += "# TYPE smotra_agent_claim_status_poll_failed_total counter\n"
	metrics += fmt.Sprintf("smotra_agent_claim_status_poll_failed_total %d\n", h.pollFailedTotal.Load())
	metrics += "\n"

	metrics += "# HELP smotra_agent_claim_status_pending_total Polls returning pending status\n"
	metrics += "# TYPE smotra_agent_claim_status_pending_total counter\n"
	metrics += fmt.Sprintf("smotra_agent_claim_status_pending_total %d\n", h.pollPendingTotal.Load())
	metrics += "\n"

	metrics += "# HELP smotra_agent_claim_status_already_delivered_total Polls returning claimed status\n"
	metrics += "# TYPE smotra_agent_claim_status_already_delivered_total counter\n"
	metrics += fmt.Sprintf("smotra_agent_claim_status_already_delivered_total %d\n", h.pollAlreadyDeliveredTotal.Load())
	metrics += "\n"

	metrics += "# HELP smotra_agent_claim_status_not_found_total Polls for non-existent claims\n"
	metrics += "# TYPE smotra_agent_claim_status_not_found_total counter\n"
	metrics += fmt.Sprintf("smotra_agent_claim_status_not_found_total %d\n", h.pollNotFoundTotal.Load())
	metrics += "\n"

	metrics += "# HELP smotra_agent_api_key_delivery_total API keys delivered to agents\n"
	metrics += "# TYPE smotra_agent_api_key_delivery_total counter\n"
	metrics += fmt.Sprintf("smotra_agent_api_key_delivery_total %d\n", h.apiKeyDeliveryTotal.Load())
	metrics += "\n"

	metrics += "\n"

	return metrics
}
