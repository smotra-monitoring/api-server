package agent_submit_results

import (
	"context"
	"database/sql"
	"encoding/json"
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

// Handler handles agent monitoring result submission
type Handler struct {
	logger *logger.Logger
	db     database.Database

	// Metrics
	submissionAttemptsTotal atomic.Uint64
	submissionSuccessTotal  atomic.Uint64
	submissionFailureTotal  atomic.Uint64
	resultsAccepted         atomic.Uint64
	resultsDuplicates       atomic.Uint64
}

// NewHandler creates a new submit results handler
func NewHandler(log *logger.Logger, db database.Database) *Handler {
	return &Handler{
		logger: log.WithComponent("agent_submit_results"),
		db:     db,
	}
}

// Handle processes a batch of monitoring results submitted by an agent
func (h *Handler) Handle(ctx context.Context, req api.SubmitAgentResultsRequestObject) (api.SubmitAgentResultsResponseObject, error) {
	h.submissionAttemptsTotal.Add(1)

	if req.Body == nil {
		h.submissionFailureTotal.Add(1)
		return api.SubmitAgentResults400JSONResponse{
			BadRequestJSONResponse: api.BadRequestJSONResponse{
				Error:   "request_body_required",
				Message: "Request body is required",
			},
		}, nil
	}

	if len(req.Body.Results) == 0 {
		h.submissionFailureTotal.Add(1)
		return api.SubmitAgentResults400JSONResponse{
			BadRequestJSONResponse: api.BadRequestJSONResponse{
				Error:   "empty_batch",
				Message: "Batch must contain at least one result",
			},
		}, nil
	}

	urlAgentID := req.AgentId.String()

	// Pre-validate all results before touching the database.
	// Both agent_id mismatch and missing endpoint_id are structural (400) errors.
	for i, result := range req.Body.Results {
		if result.AgentId.String() != urlAgentID {
			h.submissionFailureTotal.Add(1)
			h.logger.WarnContext(ctx, "Batch rejected: agent_id mismatch",
				slog.String("url_agent_id", urlAgentID),
				slog.String("result_agent_id", result.AgentId.String()),
				slog.Int("result_index", i),
			)
			return api.SubmitAgentResults400JSONResponse{
				BadRequestJSONResponse: api.BadRequestJSONResponse{
					Error:   "agent_id_mismatch",
					Message: fmt.Sprintf("result[%d]: agent_id %q does not match URL agent ID %q", i, result.AgentId.String(), urlAgentID),
				},
			}, nil
		}
		if result.EndpointId == (uuid.UUID{}) {
			h.submissionFailureTotal.Add(1)
			h.logger.WarnContext(ctx, "Batch rejected: missing endpoint_id",
				slog.Int("result_index", i),
			)
			return api.SubmitAgentResults400JSONResponse{
				BadRequestJSONResponse: api.BadRequestJSONResponse{
					Error:   "endpoint_id_required",
					Message: fmt.Sprintf("result[%d]: endpoint_id is required", i),
				},
			}, nil
		}
	}

	submissionID := uuid.Must(uuid.NewV7())
	receivedAt := time.Now().UTC()
	accepted := 0
	duplicates := 0

	q := queries.New(h.db.DB())

	for i, result := range req.Body.Results {
		checkID := result.Id.String()

		// Dedup check: SELECT before INSERT — clean sqlc, no raw RowsAffected
		_, err := q.CheckResultExists(ctx, checkID)
		if err == nil {
			// Row found — duplicate
			duplicates++
			h.logger.DebugContext(ctx, "Duplicate result skipped",
				slog.String("check_id", checkID),
				slog.Int("result_index", i),
			)
			continue
		}
		if err != sql.ErrNoRows {
			h.submissionFailureTotal.Add(1)
			h.logger.ErrorContext(ctx, "Failed to check result existence",
				slog.String("check_id", checkID),
				slog.String("error", err.Error()),
			)
			return api.SubmitAgentResults503JSONResponse{
				InternalServerErrorJSONResponse: api.InternalServerErrorJSONResponse{
					Error:   "database_error",
					Message: "Failed to process batch",
				},
			}, nil
		}

		// Validate endpoint_id: must exist and belong to this agent
		endpointID := result.EndpointId.String()
		_, epErr := q.GetEndpointByIDAndAgentID(ctx, queries.GetEndpointByIDAndAgentIDParams{
			ID:      endpointID,
			AgentID: urlAgentID,
		})
		if epErr == sql.ErrNoRows {
			h.submissionFailureTotal.Add(1)
			h.logger.WarnContext(ctx, "Batch rejected: unknown or unauthorized endpoint_id",
				slog.String("check_id", checkID),
				slog.String("endpoint_id", endpointID),
				slog.Int("result_index", i),
			)
			return api.SubmitAgentResults422JSONResponse{
				UnprocessableEntityJSONResponse: api.UnprocessableEntityJSONResponse{
					Error:   "invalid_endpoint_id",
					Message: fmt.Sprintf("result[%d]: endpoint_id %q does not exist or does not belong to this agent", i, endpointID),
				},
			}, nil
		}
		if epErr != nil {
			h.submissionFailureTotal.Add(1)
			h.logger.ErrorContext(ctx, "Failed to validate endpoint_id",
				slog.String("check_id", checkID),
				slog.String("error", epErr.Error()),
			)
			return api.SubmitAgentResults503JSONResponse{
				InternalServerErrorJSONResponse: api.InternalServerErrorJSONResponse{
					Error:   "database_error",
					Message: "Failed to process batch",
				},
			}, nil
		}

		// Inspect union type without DB calls; needed before inserting base row
		checkType, success, extractErr := extractCheckInfo(result)
		if extractErr != nil {
			h.submissionFailureTotal.Add(1)
			h.logger.ErrorContext(ctx, "Unrecognised check type",
				slog.String("check_id", checkID),
				slog.String("error", extractErr.Error()),
			)
			return api.SubmitAgentResults503JSONResponse{
				InternalServerErrorJSONResponse: api.InternalServerErrorJSONResponse{
					Error:   "check type error",
					Message: "Unrecognised check type",
				},
			}, nil
		}

		var successInt int64
		if success {
			successInt = 1
		}

		// Insert the base check_results row FIRST (child rows reference this via FK)
		if err := q.InsertCheckResult(ctx, queries.InsertCheckResultParams{
			ID:         checkID,
			AgentID:    urlAgentID,
			EndpointID: endpointID,
			CheckType:  checkType,
			Success:    successInt,
			CheckedAt:  result.Timestamp,
		}); err != nil {
			h.submissionFailureTotal.Add(1)
			h.logger.ErrorContext(ctx, "Failed to insert check result",
				slog.String("check_id", checkID),
				slog.String("error", err.Error()),
			)
			return api.SubmitAgentResults503JSONResponse{
				InternalServerErrorJSONResponse: api.InternalServerErrorJSONResponse{
					Error:   "database_error",
					Message: "Failed to store result",
				},
			}, nil
		}

		// Insert per-type child row (parent now exists)
		if _, _, insertErr := h.insertTypeSpecificResult(ctx, q, checkID, result); insertErr != nil {
			h.submissionFailureTotal.Add(1)
			h.logger.ErrorContext(ctx, "Failed to insert type-specific result",
				slog.String("check_id", checkID),
				slog.String("check_type", checkType),
				slog.String("error", insertErr.Error()),
			)
			return api.SubmitAgentResults503JSONResponse{
				InternalServerErrorJSONResponse: api.InternalServerErrorJSONResponse{
					Error:   "database_error",
					Message: "Failed to store result",
				},
			}, nil
		}

		accepted++
	}

	// Update agent's last_seen_at regardless of whether all results were duplicates
	if err := q.UpdateAgentLastSeen(ctx, queries.UpdateAgentLastSeenParams{
		LastSeenAt: sql.NullTime{Time: receivedAt, Valid: true},
		ID:         urlAgentID,
	}); err != nil {
		// Non-fatal: results are stored; log and continue
		h.logger.WarnContext(ctx, "Failed to update agent last_seen_at",
			slog.String("agent_id", urlAgentID),
			slog.String("error", err.Error()),
		)
	}

	// Update agent's last_result_submitted_at on every result batch
	if err := q.UpdateAgentLastResultSubmittedAt(ctx, queries.UpdateAgentLastResultSubmittedAtParams{
		LastResultSubmittedAt: sql.NullTime{Time: receivedAt, Valid: true},
		ID:                    urlAgentID,
	}); err != nil {
		// Non-fatal: results are stored; log and continue
		h.logger.WarnContext(ctx, "Failed to update agent last_result_submitted_at",
			slog.String("agent_id", urlAgentID),
			slog.String("error", err.Error()),
		)
	}

	// TODO: alert triggering — evaluate thresholds for each accepted result
	// and send notifications (Discord, email) when thresholds are breached.
	h.logger.DebugContext(ctx, "Alert evaluation stub — not yet implemented",
		slog.String("agent_id", urlAgentID),
		slog.Int("accepted", accepted),
	)

	h.resultsAccepted.Add(uint64(accepted))
	h.resultsDuplicates.Add(uint64(duplicates))
	h.submissionSuccessTotal.Add(1)

	h.logger.DebugContext(ctx, "Batch submission processed",
		slog.String("agent_id", urlAgentID),
		slog.String("submission_id", submissionID.String()),
		slog.Int("total", len(req.Body.Results)),
		slog.Int("accepted", accepted),
		slog.Int("duplicates", duplicates),
	)

	return api.SubmitAgentResults202JSONResponse(api.ResultsBatchAcknowledgment{
		SubmissionId:      submissionID,
		Accepted:          accepted,
		DuplicatesSkipped: &duplicates,
		ReceivedAt:        receivedAt,
	}), nil
}

// extractCheckInfo inspects the CheckType union and returns the check type string
// and overall success flag without performing any database operations.
func extractCheckInfo(result api.MonitoringResult) (checkType string, success bool, err error) {
	disc, discErr := result.CheckType.Discriminator()
	if discErr != nil {
		return "", false, fmt.Errorf("could not determine check type: %w", discErr)
	}
	switch disc {
	case "ping":
		ping, e := result.CheckType.AsPingCheck()
		if e != nil {
			return "", false, e
		}
		return "ping", ping.Result.Successes > 0, nil
	case "httpget":
		httpGet, e := result.CheckType.AsHttpGetCheck()
		if e != nil {
			return "", false, e
		}
		return "httpget", httpGet.Result.Success, nil
	case "tcpconnect":
		tcp, e := result.CheckType.AsTcpConnectCheck()
		if e != nil {
			return "", false, e
		}
		return "tcpconnect", tcp.Result.Connected, nil
	case "udpconnect":
		udp, e := result.CheckType.AsUdpConnectCheck()
		if e != nil {
			return "", false, e
		}
		return "udpconnect", udp.Result.ProbeSuccessful, nil
	case "traceroute":
		tr, e := result.CheckType.AsTracerouteCheck()
		if e != nil {
			return "", false, e
		}
		return "traceroute", tr.Result.TargetReached, nil
	case "plugin":
		plugin, e := result.CheckType.AsPluginCheck()
		if e != nil {
			return "", false, e
		}
		return "plugin", plugin.Result.Success, nil
	default:
		return "", false, fmt.Errorf("unrecognised check type %q in result %s", disc, result.Id.String())
	}
}

// insertTypeSpecificResult inserts the per-type child row into the appropriate table.
// Returns (success bool, checkType string, error). On an unrecognised union variant
// it returns an error and the caller returns 503 to the agent.
func (h *Handler) insertTypeSpecificResult(ctx context.Context, q *queries.Queries, resultID string, result api.MonitoringResult) (success bool, checkType string, err error) {
	disc, discErr := result.CheckType.Discriminator()
	if discErr != nil {
		return false, "unknown", fmt.Errorf("could not determine check type: %w", discErr)
	}
	switch disc {
	case "ping":
		ping, e := result.CheckType.AsPingCheck()
		if e != nil {
			return false, "ping", e
		}
		checkType = "ping"
		success = ping.Result.Successes > 0
		latenciesJSON, _ := json.Marshal(ping.Result.SuccessLatencies)
		err = q.InsertPingCheckResult(ctx, queries.InsertPingCheckResultParams{
			CheckID:              resultID,
			ResolvedIp:           ping.Result.ResolvedIp,
			Successes:            int64(ping.Result.Successes),
			Failures:             int64(ping.Result.Failures),
			SuccessLatenciesJson: string(latenciesJSON),
			ErrorsJson:           marshalErrorDetails(ping.Result.ErrorDetails),
		})
		return

	case "httpget":
		httpGet, e := result.CheckType.AsHttpGetCheck()
		if e != nil {
			return false, "httpget", e
		}
		checkType = "httpget"
		success = httpGet.Result.Success
		err = q.InsertHttpGetCheckResult(ctx, queries.InsertHttpGetCheckResultParams{
			CheckID:    resultID,
			StatusCode: int64(httpGet.Result.StatusCode),
			ResponseTimeMs: sql.NullFloat64{
				Float64: ptrFloat64Val(httpGet.Result.ResponseTimeMs),
				Valid:   httpGet.Result.ResponseTimeMs != nil,
			},
			ResponseSizeBytes: sql.NullInt64{
				Int64: ptrInt64Val(httpGet.Result.ResponseSizeBytes),
				Valid: httpGet.Result.ResponseSizeBytes != nil,
			},
			ErrorsJson: marshalErrorDetails(httpGet.Result.ErrorDetails),
		})
		return

	case "tcpconnect":
		tcp, e := result.CheckType.AsTcpConnectCheck()
		if e != nil {
			return false, "tcpconnect", e
		}
		checkType = "tcpconnect"
		success = tcp.Result.Connected
		err = q.InsertTcpConnectCheckResult(ctx, queries.InsertTcpConnectCheckResultParams{
			CheckID:    resultID,
			ResolvedIp: tcp.Result.ResolvedIp,
			Connected:  boolToInt64(tcp.Result.Connected),
			ConnectTimeMs: sql.NullFloat64{
				Float64: ptrFloat64Val(tcp.Result.ConnectTimeMs),
				Valid:   tcp.Result.ConnectTimeMs != nil,
			},
			ErrorsJson: marshalErrorDetails(tcp.Result.ErrorDetails),
		})
		return

	case "udpconnect":
		udp, e := result.CheckType.AsUdpConnectCheck()
		if e != nil {
			return false, "udpconnect", e
		}
		checkType = "udpconnect"
		success = udp.Result.ProbeSuccessful
		err = q.InsertUdpConnectCheckResult(ctx, queries.InsertUdpConnectCheckResultParams{
			CheckID:         resultID,
			ResolvedIp:      udp.Result.ResolvedIp,
			ProbeSuccessful: boolToInt64(udp.Result.ProbeSuccessful),
			ResponseTimeMs: sql.NullFloat64{
				Float64: ptrFloat64Val(udp.Result.ResponseTimeMs),
				Valid:   udp.Result.ResponseTimeMs != nil,
			},
			ErrorsJson: marshalErrorDetails(udp.Result.ErrorDetails),
		})
		return

	case "traceroute":
		tr, e := result.CheckType.AsTracerouteCheck()
		if e != nil {
			return false, "traceroute", e
		}
		checkType = "traceroute"
		success = tr.Result.TargetReached
		err = q.InsertTracerouteCheckResult(ctx, queries.InsertTracerouteCheckResultParams{
			CheckID:       resultID,
			TargetReached: boolToInt64(tr.Result.TargetReached),
			ErrorsJson:    marshalErrorDetails(tr.Result.ErrorDetails),
		})
		if err != nil {
			return
		}
		for _, hop := range tr.Result.Hops {
			hopID := uuid.Must(uuid.NewV7())
			latenciesJSON, _ := json.Marshal(hop.SuccessLatencies)
			if err = q.InsertTracerouteHop(ctx, queries.InsertTracerouteHopParams{
				ID:      hopID.String(),
				CheckID: resultID,
				Hop:     int64(hop.Hop),
				ResolvedIp: sql.NullString{
					String: ptrStringVal(hop.ResolvedIp),
					Valid:  hop.ResolvedIp != nil,
				},
				Hostname: sql.NullString{
					String: ptrStringVal(hop.Hostname),
					Valid:  hop.Hostname != nil,
				},
				SuccessLatenciesJson: string(latenciesJSON),
			}); err != nil {
				return
			}
		}
		return

	case "plugin":
		plugin, e := result.CheckType.AsPluginCheck()
		if e != nil {
			return false, "plugin", e
		}
		checkType = "plugin"
		success = plugin.Result.Success
		dataJSON, _ := json.Marshal(plugin.Result.Data)
		err = q.InsertPluginCheckResult(ctx, queries.InsertPluginCheckResultParams{
			CheckID:       resultID,
			PluginName:    plugin.Result.PluginName,
			PluginVersion: plugin.Result.PluginVersion,
			Success:       boolToInt64(plugin.Result.Success),
			ResponseTimeMs: sql.NullFloat64{
				Float64: ptrFloat64Val(plugin.Result.ResponseTimeMs),
				Valid:   plugin.Result.ResponseTimeMs != nil,
			},
			ErrorsJson: marshalErrorDetails(plugin.Result.ErrorDetails),
			DataJson:   string(dataJSON),
		})
		return

	default:
		return false, "unknown", fmt.Errorf("unrecognised check type %q in result %s", disc, resultID)
	}
}

// GetMetrics returns current handler metrics in Prometheus exposition format
func (h *Handler) GetMetrics() string {
	out := ""
	out += "# HELP smotra_submit_results_attempts_total Total batch submission attempts\n"
	out += "# TYPE smotra_submit_results_attempts_total counter\n"
	out += fmt.Sprintf("smotra_submit_results_attempts_total %d\n", h.submissionAttemptsTotal.Load())
	out += "\n"

	out += "# HELP smotra_submit_results_success_total Successful batch submissions\n"
	out += "# TYPE smotra_submit_results_success_total counter\n"
	out += fmt.Sprintf("smotra_submit_results_success_total %d\n", h.submissionSuccessTotal.Load())
	out += "\n"

	out += "# HELP smotra_submit_results_failure_total Failed batch submissions\n"
	out += "# TYPE smotra_submit_results_failure_total counter\n"
	out += fmt.Sprintf("smotra_submit_results_failure_total %d\n", h.submissionFailureTotal.Load())
	out += "\n"

	out += "# HELP smotra_submit_results_accepted_total Individual results accepted\n"
	out += "# TYPE smotra_submit_results_accepted_total counter\n"
	out += fmt.Sprintf("smotra_submit_results_accepted_total %d\n", h.resultsAccepted.Load())
	out += "\n"

	out += "# HELP smotra_submit_results_duplicates_total Duplicate results skipped\n"
	out += "# TYPE smotra_submit_results_duplicates_total counter\n"
	out += fmt.Sprintf("smotra_submit_results_duplicates_total %d\n", h.resultsDuplicates.Load())
	out += "\n"

	return out
}

// --- pointer-dereference helpers ---

func ptrFloat64Val(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

func ptrInt64Val(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

func ptrStringVal(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// marshalErrorDetails encodes an ErrorDetails value as {"errors":[...]} JSON
// for storage in the errors_json column. Returns sql.NullString{} when nil or empty.
func marshalErrorDetails(ed *api.ErrorDetails) sql.NullString {
	if ed == nil || ed.Errors == nil || len(*ed.Errors) == 0 {
		return sql.NullString{}
	}
	type obj struct {
		Errors []string `json:"errors"`
	}
	b, _ := json.Marshal(obj{Errors: *ed.Errors})
	return sql.NullString{String: string(b), Valid: true}
}

func boolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
