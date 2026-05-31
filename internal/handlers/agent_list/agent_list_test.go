package agent_list

import (
	"context"
	"database/sql"
	"strings"
	"testing"

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

// buildListRequest builds a ListAgentsRequestObject with optional page/pageSize overrides.
func buildListRequest(page, pageSize *int) api.ListAgentsRequestObject {
	return api.ListAgentsRequestObject{
		Params: api.ListAgentsParams{
			Page:     page,
			PageSize: pageSize,
		},
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
		"smotra_agent_list_attempts_total",
		"smotra_agent_list_success_total",
		"smotra_agent_list_failure_total",
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
		"smotra_agent_list_attempts_total 0",
		"smotra_agent_list_success_total 0",
		"smotra_agent_list_failure_total 0",
	} {
		if !strings.Contains(out, expected) {
			t.Errorf("expected %q in GetMetrics output, got:\n%s", expected, out)
		}
	}
}

func TestHandle_DBError_Returns500(t *testing.T) {
	// emptyDBMock has no schema → GetUserByID fails with "no such table: users" → 500
	h := NewHandler(logger.Default(), emptyDBMock(t))

	resp, err := h.Handle(oauth2Ctx("user-123"), buildListRequest(nil, nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := resp.(api.ListAgents500JSONResponse); !ok {
		t.Errorf("expected ListAgents500JSONResponse, got %T", resp)
	}
	if h.listAttemptsTotal.Load() != 1 {
		t.Error("attempts counter not incremented")
	}
	if h.listFailureTotal.Load() != 1 {
		t.Error("failure counter not incremented on DB error")
	}
	if h.listSuccessTotal.Load() != 0 {
		t.Error("success counter should not be incremented on error")
	}
}

func TestHandle_CountersIncrementOnAttempt(t *testing.T) {
	h := NewHandler(logger.Default(), emptyDBMock(t))

	// Two calls; both fail at DB level → attempts=2, failures=2
	_, _ = h.Handle(oauth2Ctx("u1"), buildListRequest(nil, nil))
	_, _ = h.Handle(oauth2Ctx("u2"), buildListRequest(nil, nil))

	if h.listAttemptsTotal.Load() != 2 {
		t.Errorf("expected attempts=2, got %d", h.listAttemptsTotal.Load())
	}
	if h.listFailureTotal.Load() != 2 {
		t.Errorf("expected failures=2, got %d", h.listFailureTotal.Load())
	}
}
