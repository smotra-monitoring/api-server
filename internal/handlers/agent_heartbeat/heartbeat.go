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
	if err := h.storeVitals(ctx, q, agentID, req.Body); err != nil {
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

func (h *Handler) storeVitals(ctx context.Context, q *queries.Queries, agentID string, body *api.AgentHeartbeat) error {

	params := queries.InsertAgentVitalsParams{
		ID:               uuid.Must(uuid.NewV7()).String(),
		AgentID:          agentID,
		CpuPct:           sql.NullFloat64{Float64: float64(body.Metrics.CpuUsagePercent), Valid: true},
		MemUsedMb:        sql.NullFloat64{Float64: float64(body.Metrics.MemoryUsageMb), Valid: true},
		MemTotalMb:       sql.NullFloat64{Float64: float64(body.Metrics.MemoryTotalMb), Valid: true},
		SystemUptimeSecs: sql.NullInt64{Int64: body.Metrics.SystemUptimeSecs, Valid: true},
		AgentUptimeSecs:  sql.NullInt64{Int64: body.Metrics.AgentUptimeSecs, Valid: true},
		// AgentStatus fields
		AgentVersion:      sql.NullString{String: body.AgentStatus.AgentVersion, Valid: true},
		ConfigVersion:     sql.NullInt64{Int64: int64(body.AgentStatus.ConfigVersion), Valid: true},
		IsRunning:         sql.NullInt64{Int64: boolToInt64(body.AgentStatus.IsRunning), Valid: true},
		StartedAt:         sql.NullTime{Time: body.AgentStatus.StartedAt, Valid: true},
		StoppedAt:         sql.NullTime{Time: derefTime(body.AgentStatus.StoppedAt), Valid: body.AgentStatus.StoppedAt != nil},
		ReportedAt:        body.AgentStatus.ReportedAt,
		ChecksPerformed:   sql.NullInt64{Int64: int64(body.AgentStatus.ChecksPerformed), Valid: true},
		ChecksSuccessful:  sql.NullInt64{Int64: int64(body.AgentStatus.ChecksSuccessful), Valid: true},
		ChecksFailed:      sql.NullInt64{Int64: int64(body.AgentStatus.ChecksFailed), Valid: true},
		FailedReportCount: sql.NullInt64{Int64: int64(body.AgentStatus.FailedReportCount), Valid: true},
		ServerConnected:   sql.NullInt64{Int64: boolToInt64(body.AgentStatus.ServerConnected), Valid: true},
		CacheCapacity:     sql.NullInt64{Int64: int64(body.AgentStatus.CacheStats.Capacity), Valid: true},
		CacheLen:          sql.NullInt64{Int64: int64(body.AgentStatus.CacheStats.Len), Valid: true},
	}

	return q.InsertAgentVitals(ctx, params)
}

// boolToInt64 converts a bool to 1 (true) or 0 (false) for SQLite storage.
func boolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// derefTime safely dereferences a *time.Time, returning zero value if nil.
func derefTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
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
