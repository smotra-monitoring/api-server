package agent_get

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	api "github.com/smotra-monitoring/server/internal/api/v1"
	"github.com/smotra-monitoring/server/internal/logger"
	"github.com/smotra-monitoring/server/internal/middleware"
	"github.com/smotra-monitoring/server/internal/testutil"
)

// emptyDBMock returns a MockDatabase whose DB() is an open in-memory SQLite with no schema.
// DB operations will fail with "no such table", which exercises error paths.
func emptyDBMock(t *testing.T) *testutil.MockDatabase {
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

// oauth2Ctx returns a context with a valid OAuth2 AuthInfo value.
func oauth2Ctx(userID string) context.Context {
	return context.WithValue(context.Background(), middleware.AuthContextKey, &middleware.AuthInfo{
		AuthType:      "oauth2",
		Authenticated: true,
		UserID:        userID,
		SessionID:     "test-session",
	})
}

func buildGetRequest(agentID uuid.UUID) api.GetAgentRequestObject {
	return api.GetAgentRequestObject{
		AgentId: api.UUIDv7(agentID),
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
		"smotra_agent_get_attempts_total",
		"smotra_agent_get_success_total",
		"smotra_agent_get_failure_total",
		"smotra_agent_get_not_found_total",
		"smotra_agent_get_workspace_denied_total",
	} {
		if !strings.Contains(out, key) {
			t.Errorf("missing metric %q in GetMetrics output", key)
		}
	}
}

func TestGetMetrics_InitialValuesAreZero(t *testing.T) {
	h := NewHandler(logger.Default(), testutil.NewMockDatabase())
	out := h.GetMetrics()
	for _, expected := range []string{
		"smotra_agent_get_attempts_total 0",
		"smotra_agent_get_success_total 0",
		"smotra_agent_get_failure_total 0",
		"smotra_agent_get_not_found_total 0",
		"smotra_agent_get_workspace_denied_total 0",
	} {
		if !strings.Contains(out, expected) {
			t.Errorf("expected %q in GetMetrics output, got:\n%s", expected, out)
		}
	}
}

func TestHandle_DBError_Returns500(t *testing.T) {
	// emptyDBMock has no schema -> GetAgentByID fails with "no such table: agents" -> 500
	h := NewHandler(logger.Default(), emptyDBMock(t))

	resp, err := h.Handle(oauth2Ctx("user-123"), buildGetRequest(uuid.Must(uuid.NewV7())))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := resp.(api.GetAgent500JSONResponse); !ok {
		t.Errorf("expected GetAgent500JSONResponse, got %T", resp)
	}
	if h.getAttemptsTotal.Load() != 1 {
		t.Error("attempts counter not incremented")
	}
	if h.getFailureTotal.Load() != 1 {
		t.Error("failure counter not incremented on DB error")
	}
	if h.getSuccessTotal.Load() != 0 {
		t.Error("success counter should not be incremented on error")
	}
}

func TestHandle_CountersIncrementOnAttempt(t *testing.T) {
	h := NewHandler(logger.Default(), emptyDBMock(t))

	// Two calls; both fail at DB level -> attempts=2, failures=2
	_, _ = h.Handle(oauth2Ctx("u1"), buildGetRequest(uuid.Must(uuid.NewV7())))
	_, _ = h.Handle(oauth2Ctx("u2"), buildGetRequest(uuid.Must(uuid.NewV7())))

	if h.getAttemptsTotal.Load() != 2 {
		t.Errorf("expected attempts=2, got %d", h.getAttemptsTotal.Load())
	}
	if h.getFailureTotal.Load() != 2 {
		t.Errorf("expected failures=2, got %d", h.getFailureTotal.Load())
	}
}
