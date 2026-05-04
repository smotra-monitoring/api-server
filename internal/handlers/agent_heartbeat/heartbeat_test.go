package agent_heartbeat

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	api "github.com/smotra-monitoring/server/internal/api/v1"
	"github.com/smotra-monitoring/server/internal/logger"
	"github.com/smotra-monitoring/server/internal/testutil"
)

// mockDBWithSQLite returns a MockDatabase whose DB() returns an open in-memory SQLite.
// The schema is empty — DB ops fail gracefully with "no such table" errors.
func mockDBWithSQLite(t *testing.T) *testutil.MockDatabase {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	mock := testutil.NewMockDatabase()
	mock.DBFunc = func() *sql.DB { return db }
	return mock
}

func makeValidBody() *api.AgentHeartbeat {
	return &api.AgentHeartbeat{
		Timestamp:        time.Now().UTC(),
		Status:           api.Healthy,
		CpuUsagePercent:  42.5,
		MemoryUsageMb:    1024.0,
		MemoryTotalMb:    8192.0,
		SystemUptimeSecs: 86400,
	}
}

func TestNewHandler(t *testing.T) {
	h := NewHandler(logger.Default(), testutil.NewMockDatabase())
	if h == nil {
		t.Fatal("NewHandler returned nil")
	}
}

func TestGetMetrics_ContainsAllCounters(t *testing.T) {
	h := NewHandler(logger.Default(), testutil.NewMockDatabase())
	out := h.GetMetrics()
	for _, key := range []string{
		"smotra_agent_heartbeat_attempts_total",
		"smotra_agent_heartbeat_success_total",
		"smotra_agent_heartbeat_failure_total",
		"smotra_agent_vitals_stored_total",
	} {
		if !strings.Contains(out, key) {
			t.Errorf("missing metric %q in GetMetrics output", key)
		}
	}
}

func TestHandle_NilBody_Returns400(t *testing.T) {
	// nil body check happens before any DB access.
	h := NewHandler(logger.Default(), testutil.NewMockDatabase())

	resp, err := h.Handle(context.Background(), api.SendAgentHeartbeatRequestObject{
		AgentId: uuid.Must(uuid.NewV7()),
		Body:    nil,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := resp.(api.SendAgentHeartbeat400JSONResponse); !ok {
		t.Errorf("expected 400 response, got %T", resp)
	}
	if h.heartbeatFailureTotal.Load() != 1 {
		t.Error("failure counter not incremented")
	}
}

func TestHandle_WithVitals_IncrementsVitalsCounter_OnMockDB(t *testing.T) {
	// Empty SQLite (no schema): UpdateAgentLastSeen fails with "no such table" (logged),
	// then InsertAgentVitals also fails → handler returns 503.
	h := NewHandler(logger.Default(), mockDBWithSQLite(t))
	resp, err := h.Handle(context.Background(), api.SendAgentHeartbeatRequestObject{
		AgentId: uuid.Must(uuid.NewV7()),
		Body:    makeValidBody(),
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// InsertAgentVitals fails → 503
	if _, ok := resp.(api.SendAgentHeartbeat503JSONResponse); !ok {
		t.Errorf("expected 503 response with empty DB, got %T", resp)
	}
	if h.heartbeatFailureTotal.Load() != 1 {
		t.Error("failure counter not incremented")
	}
}
