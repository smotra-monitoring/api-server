package agent_claim

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync/atomic"

	api "github.com/smotra-monitoring/server/internal/api/v1"
	"github.com/smotra-monitoring/server/internal/database"
	"github.com/smotra-monitoring/server/internal/database/queries"
	"github.com/smotra-monitoring/server/internal/logger"
)

// Handler handles agent claiming requests from web UI
type Handler struct {
	logger *logger.Logger
	db     database.Database

	// Metrics
	claimAttemptsTotal       atomic.Uint64
	claimSuccessTotal        atomic.Uint64
	claimFailureTotal        atomic.Uint64
	claimInvalidTokenTotal   atomic.Uint64
	claimNotFoundTotal       atomic.Uint64
	claimAlreadyClaimedTotal atomic.Uint64
}

// NewHandler creates a new agent claim handler
func NewHandler(logger *logger.Logger, db database.Database) *Handler {
	return &Handler{
		logger: logger.WithComponent("agent_claim"),
		db:     db,
	}
}

// Handle processes agent claim requests
func (h *Handler) Handle(ctx context.Context, req api.ClaimAgentRequestObject) (api.ClaimAgentResponseObject, error) {
	h.claimAttemptsTotal.Add(1)

	q := queries.New(h.db.DB())

	if req.Body == nil {
		h.claimFailureTotal.Add(1)
		return api.ClaimAgent400JSONResponse{
			BadRequestJSONResponse: api.BadRequestJSONResponse{
				Error:   "bad_request",
				Message: "Request body is required",
			},
		}, nil
	}

	agentIDStr := req.Body.AgentId.String()

	// Hash the provided claim token
	claimTokenHash := hashClaimToken(req.Body.ClaimToken)

	// Get agent claimFromDB for claiming (validates token, expiration, and unclaimed status)
	claimFromDB, err := q.GetAgentClaim(ctx, agentIDStr)

	if err != nil {
		h.claimNotFoundTotal.Add(1)
		h.logger.WarnContext(ctx, "Agent claim not found or invalid",
			slog.String("agentId", agentIDStr),
			slog.String("error", err.Error()),
		)
		return api.ClaimAgent404JSONResponse(api.Error{
			Error:   "claim_not_found",
			Message: "Claim not found or invalid",
		}), nil
	}

	// Get agent claimFromDB for claiming (validates token, expiration, and unclaimed status)
	claimFromDB, err = q.GetAgentClaimForClaiming(ctx, queries.GetAgentClaimForClaimingParams{
		ID:             agentIDStr,
		ClaimTokenHash: claimTokenHash,
	})

	if err != nil {
		if err == sql.ErrNoRows {
			h.claimAlreadyClaimedTotal.Add(1)
			return api.ClaimAgent409JSONResponse(api.Error{
				Error:   "already_claimed_or_invalid",
				Message: "Agent has already been claimed or claim token is invalid/expired",
			}), nil
		}

		h.claimNotFoundTotal.Add(1)
		h.logger.WarnContext(ctx, "Agent claim not found or invalid",
			slog.String("agentId", agentIDStr),
			slog.String("error", err.Error()),
		)
		return api.ClaimAgent403JSONResponse(api.Error{
			Error:   "invalid_claim_token",
			Message: "Invalid or expired claim token",
		}), nil
	}

	// Double-check it's not already claimed (additional safety)
	if claimFromDB.ClaimedAt.Valid {
		h.claimAlreadyClaimedTotal.Add(1)
		return api.ClaimAgent409JSONResponse(api.Error{
			Error:   "already_claimed",
			Message: "Agent has already been claimed",
		}), nil
	}

	// Verify constant-time comparison of token hash (extra security)
	if subtle.ConstantTimeCompare([]byte(claimFromDB.ClaimTokenHash), []byte(claimTokenHash)) != 1 {
		h.claimInvalidTokenTotal.Add(1)
		return api.ClaimAgent403JSONResponse(api.Error{
			Error:   "invalid_claim_token",
			Message: "Invalid claim token",
		}), nil
	}

	// Generate API key for the agent
	apiKey, err := generateAPIKey()
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to generate API key",
			slog.String("agentId", agentIDStr),
			slog.String("error", err.Error()),
		)
		h.claimFailureTotal.Add(1)
		return api.ClaimAgent500JSONResponse{
			InternalServerErrorJSONResponse: api.InternalServerErrorJSONResponse{
				Error:   "internal_error",
				Message: "Failed to generate API key",
			},
		}, nil
	}

	apiKeyHash := hashAPIKey(apiKey)

	// TODO: Get user ID from auth context when OAuth2 is implemented
	// For now, we'll use a placeholder or nil
	var userID sql.NullString // Will be populated from JWT/OAuth2 context

	// Determine agent name - use hostname from claim
	agentName := claimFromDB.Hostname
	if req.Body.Name != nil && *req.Body.Name != "" {
		agentName = *req.Body.Name
	}

	// Create agent in production table
	_, err = q.CreateAgentFromClaim(ctx, queries.CreateAgentFromClaimParams{
		ID:           agentIDStr,
		SectionID:    req.Body.SectionId.String(),
		Name:         agentName,
		ApiKeyHash:   apiKeyHash,
		BaseConfig:   "{}", // Default empty config
		AgentVersion: sql.NullString{String: claimFromDB.AgentVersion, Valid: true},
	})

	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to create agent",
			slog.String("agentId", agentIDStr),
			slog.String("error", err.Error()),
		)
		h.claimFailureTotal.Add(1)
		return api.ClaimAgent500JSONResponse{
			InternalServerErrorJSONResponse: api.InternalServerErrorJSONResponse{
				Error:   "internal_error",
				Message: "Failed to create agent",
			},
		}, nil
	}

	// Mark claim as claimed and store API key for delivery
	err = q.MarkAgentClaimClaimed(ctx, queries.MarkAgentClaimClaimedParams{
		ClaimedByUserID: userID,
		ApiKeyPlaintext: sql.NullString{String: apiKey, Valid: true},
		ID:              agentIDStr,
	})

	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to mark claim as claimed",
			slog.String("agentId", agentIDStr),
			slog.String("error", err.Error()),
		)
		// Don't fail the request - agent was created successfully
	}

	h.claimSuccessTotal.Add(1)

	h.logger.InfoContext(ctx, "Agent claimed successfully",
		slog.String("agentId", agentIDStr),
		slog.String("name", agentName),
		slog.String("sectionId", req.Body.SectionId.String()),
	)

	return api.ClaimAgent200JSONResponse(api.ClaimAgentResponse{
		AgentId: req.Body.AgentId,
		Status:  "claimed",
		Message: "Agent claimed successfully. API key will be delivered on next poll.",
	}), nil
}

// hashClaimToken hashes a claim token using SHA-256
func hashClaimToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// hashAPIKey hashes an API key using SHA-256
func hashAPIKey(apiKey string) string {
	hash := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(hash[:])
}

// generateAPIKey generates a cryptographically secure API key
func generateAPIKey() (string, error) {
	// Generate 32 bytes of random data
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	// Format as sk_live_<hex>
	return fmt.Sprintf("sk_live_%s", hex.EncodeToString(bytes)), nil
}

// GetMetrics returns current handler metrics in Prometheus format
func (h *Handler) GetMetrics() string {
	metrics := ""
	metrics += "# HELP smotra_agent_claim_attempts_total Total agent claim attempts\n"
	metrics += "# TYPE smotra_agent_claim_attempts_total counter\n"
	metrics += fmt.Sprintf("smotra_agent_claim_attempts_total %d\n", h.claimAttemptsTotal.Load())
	metrics += "\n"

	metrics += "# HELP smotra_agent_claim_success_total Successful agent claims\n"
	metrics += "# TYPE smotra_agent_claim_success_total counter\n"
	metrics += fmt.Sprintf("smotra_agent_claim_success_total %d\n", h.claimSuccessTotal.Load())
	metrics += "\n"

	metrics += "# HELP smotra_agent_claim_failure_total Failed agent claims\n"
	metrics += "# TYPE smotra_agent_claim_failure_total counter\n"
	metrics += fmt.Sprintf("smotra_agent_claim_failure_total %d\n", h.claimFailureTotal.Load())
	metrics += "\n"

	metrics += "# HELP smotra_agent_claim_invalid_token_total Claims with invalid tokens\n"
	metrics += "# TYPE smotra_agent_claim_invalid_token_total counter\n"
	metrics += fmt.Sprintf("smotra_agent_claim_invalid_token_total %d\n", h.claimInvalidTokenTotal.Load())
	metrics += "\n"

	metrics += "# HELP smotra_agent_claim_not_found_total Claims for non-existent registrations\n"
	metrics += "# TYPE smotra_agent_claim_not_found_total counter\n"
	metrics += fmt.Sprintf("smotra_agent_claim_not_found_total %d\n", h.claimNotFoundTotal.Load())
	metrics += "\n"

	metrics += "# HELP smotra_agent_claim_already_claimed_total Attempts to claim already-claimed agents\n"
	metrics += "# TYPE smotra_agent_claim_already_claimed_total counter\n"
	metrics += fmt.Sprintf("smotra_agent_claim_already_claimed_total %d\n", h.claimAlreadyClaimedTotal.Load())
	metrics += "\n"

	metrics += "\n"

	return metrics
}
