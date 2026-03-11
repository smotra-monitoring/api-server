package agent_register

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	api "github.com/smotra-monitoring/server/internal/api/v1"
	"github.com/smotra-monitoring/server/internal/config"
	"github.com/smotra-monitoring/server/internal/database"
	"github.com/smotra-monitoring/server/internal/database/queries"
	"github.com/smotra-monitoring/server/internal/logger"
)

// Handler handles agent self-registration requests
type Handler struct {
	logger *logger.Logger
	db     database.Database
	config *config.Config

	// Metrics
	registrationAttemptsTotal   atomic.Uint64
	registrationSuccessTotal    atomic.Uint64
	registrationFailureTotal    atomic.Uint64
	registrationIdempotentTotal atomic.Uint64
}

// NewHandler creates a new agent registration handler
func NewHandler(logger *logger.Logger, db database.Database, cfg *config.Config) *Handler {
	return &Handler{
		logger: logger.WithComponent("agent_register"),
		db:     db,
		config: cfg,
	}
}

// Handle processes agent self-registration requests
func (h *Handler) Handle(ctx context.Context, req api.RegisterAgentSelfRequestObject) (api.RegisterAgentSelfResponseObject, error) {
	h.registrationAttemptsTotal.Add(1)

	if req.Body == nil {
		h.registrationFailureTotal.Add(1)
		return api.RegisterAgentSelf400JSONResponse{
			BadRequestJSONResponse: api.BadRequestJSONResponse{
				Error:   "request_body_required",
				Message: "This is unexpected error, pay attention to logs. If \"body\" is null, then validation in generated API layer should have caught it earlier.",
			},
		}, nil
	}

	// Validate request
	if err := h.validateRequest(req.Body); err != nil {
		h.registrationFailureTotal.Add(1)
		return api.RegisterAgentSelf400JSONResponse{
			BadRequestJSONResponse: api.BadRequestJSONResponse{
				Error:   "validation_error",
				Message: err.Error(),
			},
		}, nil
	}

	q := queries.New(h.db.DB())

	// Convert UUID to string
	agentIDStr := req.Body.AgentId.String()

	// Check if agent claim already exists
	existingClaim, err := q.GetAgentClaim(ctx, agentIDStr)
	if err == nil && existingClaim.ClaimedAt.Valid {
		// Agent already claimed
		h.registrationFailureTotal.Add(1)
		return api.RegisterAgentSelf400JSONResponse{
			BadRequestJSONResponse: api.BadRequestJSONResponse{
				Error:   "already_claimed",
				Message: "Agent has already been claimed",
			},
		}, nil
	}

	// Calculate expiration time
	expiresAt := time.Now().UTC().Add(time.Duration(h.config.Agent.ClaimTokenExpirationHours) * time.Hour)

	// Upsert agent claim (idempotent)
	_, err = q.UpsertAgentClaim(ctx, queries.UpsertAgentClaimParams{
		ID:                  agentIDStr,
		ClaimTokenHash:      req.Body.ClaimTokenHash,
		Hostname:            req.Body.Hostname,
		AgentVersion:        req.Body.AgentVersion,
		ClaimTokenExpiresAt: expiresAt,
	})

	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to upsert agent claim",
			slog.String("agentId", agentIDStr),
			slog.String("error", err.Error()),
		)
		h.registrationFailureTotal.Add(1)
		return api.RegisterAgentSelf500JSONResponse{
			InternalServerErrorJSONResponse: api.InternalServerErrorJSONResponse{
				Error:   "internal_error",
				Message: "Failed to register agent",
			},
		}, nil
	}

	// Check if this was an update (idempotent call)
	statusCode := 201 // Created
	if existingClaim.ID != "" {
		h.registrationIdempotentTotal.Add(1)
		statusCode = 200 // OK (idempotent update)
	} else {
		h.registrationSuccessTotal.Add(1)
	}

	response := api.AgentRegistrationResponse{
		Status:    "pending_claim",
		PollUrl:   fmt.Sprintf("/agent/%s/claim-status", agentIDStr),
		ClaimUrl:  fmt.Sprintf("%s/claim", h.config.Agent.ServerURL),
		ExpiresAt: expiresAt,
	}

	h.logger.InfoContext(ctx, "Agent registration successful",
		slog.String("agentId", agentIDStr),
		slog.String("hostname", req.Body.Hostname),
		slog.Int("statusCode", statusCode),
	)

	if statusCode == 201 {
		return api.RegisterAgentSelf201JSONResponse(response), nil
	}
	return api.RegisterAgentSelf200JSONResponse(response), nil
}

// validateRequest validates the registration request
func (h *Handler) validateRequest(req *api.AgentSelfRegistration) error {
	agentIDStr := req.AgentId.String()
	if agentIDStr == "" || agentIDStr == "00000000-0000-0000-0000-000000000000" {
		return fmt.Errorf("agentId is required and must be a valid UUID")
	}

	if req.ClaimTokenHash == "" {
		return fmt.Errorf("claimTokenHash is required")
	}

	if len(req.ClaimTokenHash) != 64 {
		return fmt.Errorf("claimTokenHash must be a 64-character SHA-256 hash")
	}

	// Validate it's hex
	if _, err := hex.DecodeString(req.ClaimTokenHash); err != nil {
		return fmt.Errorf("claimTokenHash must be a valid hexadecimal string")
	}

	if req.Hostname == "" {
		return fmt.Errorf("hostname is required")
	}

	if req.AgentVersion == "" {
		return fmt.Errorf("agentVersion is required")
	}

	return nil
}

// GetMetrics returns current handler metrics in Prometheus format
func (h *Handler) GetMetrics() string {
	metrics := ""
	metrics += "# HELP smotra_agent_registration_attempts_total Total agent registration attempts\n"
	metrics += "# TYPE smotra_agent_registration_attempts_total counter\n"
	metrics += fmt.Sprintf("smotra_agent_registration_attempts_total %d\n", h.registrationAttemptsTotal.Load())
	metrics += "\n"

	metrics += "# HELP smotra_agent_registration_success_total Successful agent registrations\n"
	metrics += "# TYPE smotra_agent_registration_success_total counter\n"
	metrics += fmt.Sprintf("smotra_agent_registration_success_total %d\n", h.registrationSuccessTotal.Load())
	metrics += "\n"

	metrics += "# HELP smotra_agent_registration_failure_total Failed agent registrations\n"
	metrics += "# TYPE smotra_agent_registration_failure_total counter\n"
	metrics += fmt.Sprintf("smotra_agent_registration_failure_total %d\n", h.registrationFailureTotal.Load())
	metrics += "\n"

	metrics += "# HELP smotra_agent_registration_idempotent_total Idempotent registration updates\n"
	metrics += "# TYPE smotra_agent_registration_idempotent_total counter\n"
	metrics += fmt.Sprintf("smotra_agent_registration_idempotent_total %d\n", h.registrationIdempotentTotal.Load())
	metrics += "\n"

	metrics += "\n"

	return metrics
}

// GenerateClaimToken generates a random claim token for testing purposes
func GenerateClaimToken() (string, error) {
	bytes := make([]byte, 8) // 8 bytes = 16 hex characters
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
