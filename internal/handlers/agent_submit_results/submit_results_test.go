package agent_submit_results

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	api "github.com/smotra-monitoring/server/internal/api/v1"
	"github.com/smotra-monitoring/server/internal/logger"
	"github.com/smotra-monitoring/server/internal/testutil"
)

// makePingCheck builds a CheckType union wrapping a PingCheck
func makePingCheck(t *testing.T, resolved string, successes, failures int32) api.CheckType {
	t.Helper()
	raw, _ := json.Marshal(api.PingCheck{
		Type: "ping",
		Result: api.PingResult{
			ResolvedIp:       resolved,
			Successes:        successes,
			Failures:         failures,
			SuccessLatencies: []float64{},
		},
	})
	var ct api.CheckType
	if err := json.Unmarshal(raw, &ct); err != nil {
		t.Fatalf("failed to build CheckType: %v", err)
	}
	return ct
}

func makeResult(agentID uuid.UUID, checkType api.CheckType, endpointID uuid.UUID) api.MonitoringResult {
	return api.MonitoringResult{
		Id:         uuid.Must(uuid.NewV7()),
		AgentId:    agentID,
		CheckType:  checkType,
		EndpointId: endpointID,
		Timestamp:  time.Now().UTC(),
	}
}

func TestNewHandler(t *testing.T) {
	h := NewHandler(logger.Default(), testutil.NewMockDatabase())
	if h == nil {
		t.Fatal("NewHandler returned nil")
	}
	if h.logger == nil {
		t.Error("logger is nil")
	}
	if h.db == nil {
		t.Error("db is nil")
	}
}

func TestGetMetrics_ContainsAllCounters(t *testing.T) {
	h := NewHandler(logger.Default(), testutil.NewMockDatabase())
	out := h.GetMetrics()
	for _, key := range []string{
		"smotra_submit_results_attempts_total",
		"smotra_submit_results_success_total",
		"smotra_submit_results_failure_total",
		"smotra_submit_results_accepted_total",
		"smotra_submit_results_duplicates_total",
	} {
		if !strings.Contains(out, key) {
			t.Errorf("missing metric %q in GetMetrics output", key)
		}
	}
}

// The nil-body and empty-batch paths return before any DB access, so they work
// with a mock that returns nil for DB().
func TestHandle_NilBody_Returns400(t *testing.T) {
	h := NewHandler(logger.Default(), testutil.NewMockDatabase())

	resp, err := h.Handle(context.Background(), api.SubmitAgentResultsRequestObject{
		AgentId: uuid.Must(uuid.NewV7()),
		Body:    nil,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := resp.(api.SubmitAgentResults400JSONResponse); !ok {
		t.Errorf("expected 400 response, got %T", resp)
	}
	if h.submissionFailureTotal.Load() != 1 {
		t.Error("failure counter not incremented")
	}
}

func TestHandle_EmptyBatch_Returns400(t *testing.T) {
	h := NewHandler(logger.Default(), testutil.NewMockDatabase())

	resp, err := h.Handle(context.Background(), api.SubmitAgentResultsRequestObject{
		AgentId: uuid.Must(uuid.NewV7()),
		Body:    &api.BatchMonitoringResults{Results: []api.MonitoringResult{}},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := resp.(api.SubmitAgentResults400JSONResponse); !ok {
		t.Errorf("expected 400 response, got %T", resp)
	}
}

func TestHandle_AgentIDMismatch_Returns400(t *testing.T) {
	h := NewHandler(logger.Default(), testutil.NewMockDatabase())

	urlAgentID := uuid.Must(uuid.NewV7())
	wrongAgentID := uuid.Must(uuid.NewV7())

	result := makeResult(wrongAgentID, makePingCheck(t, "1.2.3.4", 3, 0), uuid.Must(uuid.NewV7()))

	resp, err := h.Handle(context.Background(), api.SubmitAgentResultsRequestObject{
		AgentId: urlAgentID,
		Body:    &api.BatchMonitoringResults{Results: []api.MonitoringResult{result}},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := resp.(api.SubmitAgentResults400JSONResponse); !ok {
		t.Errorf("expected 400 response, got %T", resp)
	}
	if h.submissionFailureTotal.Load() != 1 {
		t.Error("failure counter not incremented on mismatch")
	}
}

func TestHandle_ZeroEndpointID_Returns400(t *testing.T) {
	h := NewHandler(logger.Default(), testutil.NewMockDatabase())

	agentID := uuid.Must(uuid.NewV7())
	result := makeResult(agentID, makePingCheck(t, "1.2.3.4", 1, 0), uuid.UUID{}) // zero UUID = missing field

	resp, err := h.Handle(context.Background(), api.SubmitAgentResultsRequestObject{
		AgentId: agentID,
		Body:    &api.BatchMonitoringResults{Results: []api.MonitoringResult{result}},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r400, ok := resp.(api.SubmitAgentResults400JSONResponse)
	if !ok {
		t.Fatalf("expected 400 response, got %T", resp)
	}
	if r400.Error != "endpoint_id_required" {
		t.Errorf("expected error code %q, got %q", "endpoint_id_required", r400.Error)
	}
	if h.submissionFailureTotal.Load() != 1 {
		t.Error("failure counter not incremented")
	}
}

// MetricsProvider interface satisfaction (compile-time check)
var _ interface{ GetMetrics() string } = (*Handler)(nil)

func TestPtrHelpers(t *testing.T) {
	f := 3.14
	if ptrFloat64Val(&f) != 3.14 {
		t.Error("ptrFloat64Val mismatch")
	}
	if ptrFloat64Val(nil) != 0 {
		t.Error("ptrFloat64Val(nil) should be 0")
	}

	i64 := int64(42)
	if ptrInt64Val(&i64) != 42 {
		t.Error("ptrInt64Val mismatch")
	}
	if ptrInt64Val(nil) != 0 {
		t.Error("ptrInt64Val(nil) should be 0")
	}

	s := "hello"
	if ptrStringVal(&s) != "hello" {
		t.Error("ptrStringVal mismatch")
	}
	if ptrStringVal(nil) != "" {
		t.Error("ptrStringVal(nil) should be empty")
	}

	if boolToInt64(true) != 1 {
		t.Error("boolToInt64(true) should be 1")
	}
	if boolToInt64(false) != 0 {
		t.Error("boolToInt64(false) should be 0")
	}
}
