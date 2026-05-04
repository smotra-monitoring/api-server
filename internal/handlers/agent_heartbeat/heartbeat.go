package agent_heartbeat

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	api "github.com/smotra-monitoring/server/internal/api/v1"
	"github.com/smotra-monitoring/server/internal/database"
	"github.com/smotra-monitoring/server/internal/database/queries"
	"github.com/smotra-monitoring/server/internal/logger"
)

// Handler handles agent heartbeat submissions.
type Handler struct {
	logger *logger.Logger
	db     database.Database

	// Metrics
	heartbeatAttemptsTotal atomic.Uint64
	heartbeatSuccessTotal  atomic.Uint64
	heartbeatFailureTotal  atomic.Uint64
	vitalsStoredTotal      atomic.Uint64
}

// NewHandler creates a new heartbeat handler.
func NewHandler(log *logger.Logger, db database.Database) *Handler {
	return &Handler{
		logger: log.WithComponent("agent_heartbeat"),
		db:     db,
	}
}

// Handle processes a heartbeat from an agent.
// It always updates agent.last_seen_at and stores a vitals snapshot.
func (h *Handler) Handle(ctx context.Context, req api.SendAgentHeartbeatRequestObject) (api.SendAgentHeartbeatResponseObject, error) {
	h.heartbeatAttemptsTotal.Add(1)

	if req.Body == nil {
		h.heartbeatFailureTotal.Add(1)
		return api.SendAgentHeartbeat400JSONResponse{
			BadRequestJSONResponse: api.BadRequestJSONResponse{
				Error:   "request_body_required",
				Message: "Request body is required",
			},
		}, nil
	}

	agentID := req.AgentId.String()
	receivedAt := time.Now().UTC()

	q := queries.New(h.db.DB())

	// Always update last_seen_at — non-fatal if it fails.
	if err := q.UpdateAgentLastSeen(ctx, queries.UpdateAgentLastSeenParams{
		LastSeenAt: sql.NullTime{Time: receivedAt, Valid: true},
		ID:         agentID,
	}); err != nil {
		h.logger.WarnContext(ctx, "Failed to update agent last_seen_at",
			slog.String("agent_id", agentID),
			slog.String("error", err.Error()),
		)
	}

	// Always store vitals — cpu/memory are required fields.
	if err := h.storeVitals(ctx, q, agentID, req.Body, receivedAt); err != nil {
		h.heartbeatFailureTotal.Add(1)
		h.logger.ErrorContext(ctx, "Failed to store vitals snapshot",
			slog.String("agent_id", agentID),
			slog.String("error", err.Error()),
		)
		return api.SendAgentHeartbeat503JSONResponse{
			InternalServerErrorJSONResponse: api.InternalServerErrorJSONResponse{
				Error:   "database_error",
				Message: "Failed to store vitals snapshot",
			},
		}, nil
	}
	h.vitalsStoredTotal.Add(1)

	h.heartbeatSuccessTotal.Add(1)
	return api.SendAgentHeartbeat204Response{}, nil
}

func (h *Handler) storeVitals(ctx context.Context, q *queries.Queries, agentID string, body *api.AgentHeartbeat, receivedAt time.Time) error {
	reportedAt := body.Timestamp
	if reportedAt.IsZero() {
		reportedAt = receivedAt
	}

	params := queries.InsertAgentVitalsParams{
		ID:         uuid.Must(uuid.NewV7()).String(),
		AgentID:    agentID,
		ReportedAt: reportedAt,
		CpuPct:           sql.NullFloat64{Float64: float64(body.CpuUsagePercent), Valid: true},
		MemUsedMb:        sql.NullFloat64{Float64: float64(body.MemoryUsageMb), Valid: true},
		MemTotalMb:       sql.NullFloat64{Float64: float64(body.MemoryTotalMb), Valid: true},
		SystemUptimeSecs: sql.NullInt64{Int64: body.SystemUptimeSecs, Valid: true},
	}

	return q.InsertAgentVitals(ctx, params)
}

// GetMetrics returns Prometheus-formatted metrics for this handler.
func (h *Handler) GetMetrics() string {
	out := ""
	out += "# HELP smotra_agent_heartbeat_attempts_total Total heartbeat attempts\n"
	out += "# TYPE smotra_agent_heartbeat_attempts_total counter\n"
	out += fmt.Sprintf("smotra_agent_heartbeat_attempts_total %d\n", h.heartbeatAttemptsTotal.Load())

	out += "# HELP smotra_agent_heartbeat_success_total Successful heartbeats processed\n"
	out += "# TYPE smotra_agent_heartbeat_success_total counter\n"
	out += fmt.Sprintf("smotra_agent_heartbeat_success_total %d\n", h.heartbeatSuccessTotal.Load())

	out += "# HELP smotra_agent_heartbeat_failure_total Failed heartbeat submissions\n"
	out += "# TYPE smotra_agent_heartbeat_failure_total counter\n"
	out += fmt.Sprintf("smotra_agent_heartbeat_failure_total %d\n", h.heartbeatFailureTotal.Load())

	out += "# HELP smotra_agent_vitals_stored_total Vitals snapshots stored from heartbeats\n"
	out += "# TYPE smotra_agent_vitals_stored_total counter\n"
	out += fmt.Sprintf("smotra_agent_vitals_stored_total %d\n", h.vitalsStoredTotal.Load())

	return out
}
