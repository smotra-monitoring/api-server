package agent_heartbeat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	api "github.com/smotra-monitoring/server/internal/api/v1"
	"github.com/smotra-monitoring/server/internal/database"
	"github.com/smotra-monitoring/server/internal/logger"
	"github.com/smotra-monitoring/server/internal/testutil"
)

// testServerImpl satisfies the full api.StrictServerInterface by delegating
// SendAgentHeartbeat to the handler under test and stubbing everything else.
type testServerImpl struct {
	*Handler
}

func (s *testServerImpl) SendAgentHeartbeat(ctx context.Context, req api.SendAgentHeartbeatRequestObject) (api.SendAgentHeartbeatResponseObject, error) {
	return s.Handle(ctx, req)
}
func (s *testServerImpl) GetAgentConfiguration(ctx context.Context, req api.GetAgentConfigurationRequestObject) (api.GetAgentConfigurationResponseObject, error) {
	return nil, nil
}
func (s *testServerImpl) RegisterAgentSelf(ctx context.Context, req api.RegisterAgentSelfRequestObject) (api.RegisterAgentSelfResponseObject, error) {
	return nil, nil
}
func (s *testServerImpl) GetAgentClaimStatus(ctx context.Context, req api.GetAgentClaimStatusRequestObject) (api.GetAgentClaimStatusResponseObject, error) {
	return nil, nil
}
func (s *testServerImpl) PostClaimAgent(ctx context.Context, req api.PostClaimAgentRequestObject) (api.PostClaimAgentResponseObject, error) {
	return nil, nil
}
func (s *testServerImpl) SubmitAgentResults(ctx context.Context, req api.SubmitAgentResultsRequestObject) (api.SubmitAgentResultsResponseObject, error) {
	return nil, nil
}
func (s *testServerImpl) Oauth2Authorize(ctx context.Context, req api.Oauth2AuthorizeRequestObject) (api.Oauth2AuthorizeResponseObject, error) {
	return nil, nil
}
func (s *testServerImpl) Oauth2Callback(ctx context.Context, req api.Oauth2CallbackRequestObject) (api.Oauth2CallbackResponseObject, error) {
	return nil, nil
}
func (s *testServerImpl) Oauth2Revoke(ctx context.Context, req api.Oauth2RevokeRequestObject) (api.Oauth2RevokeResponseObject, error) {
	return nil, nil
}
func (s *testServerImpl) Oauth2Token(ctx context.Context, req api.Oauth2TokenRequestObject) (api.Oauth2TokenResponseObject, error) {
	return nil, nil
}
func (s *testServerImpl) GetUserInfo(ctx context.Context, req api.GetUserInfoRequestObject) (api.GetUserInfoResponseObject, error) {
	return nil, nil
}
func (s *testServerImpl) Logout(ctx context.Context, req api.LogoutRequestObject) (api.LogoutResponseObject, error) {
	return nil, nil
}

func (s *testServerImpl) ListSessions(ctx context.Context, req api.ListSessionsRequestObject) (api.ListSessionsResponseObject, error) {
	return nil, nil
}
func (s *testServerImpl) PurgeSessions(ctx context.Context, req api.PurgeSessionsRequestObject) (api.PurgeSessionsResponseObject, error) {
	return nil, nil
}
func (s *testServerImpl) PurgeSession(ctx context.Context, req api.PurgeSessionRequestObject) (api.PurgeSessionResponseObject, error) {
	return nil, nil
}
func (s *testServerImpl) AuthRefresh(ctx context.Context, req api.AuthRefreshRequestObject) (api.AuthRefreshResponseObject, error) {
	return nil, nil
}
func (s *testServerImpl) ListAgents(ctx context.Context, req api.ListAgentsRequestObject) (api.ListAgentsResponseObject, error) {
	return nil, nil
}

func setupTestRouter(h *Handler) *chi.Mux {
	impl := &testServerImpl{Handler: h}
	r := chi.NewRouter()
	api.HandlerFromMux(api.NewStrictHandler(impl, nil), r)
	return r
}

func setupRealDB(t *testing.T) (database.Database, uuid.UUID) {
	t.Helper()
	db := testutil.SetupTestSQLiteDB(t)
	ctx := context.Background()
	testutil.ApplyMigrations(t, ctx, db.DB(), "../../../data/db/dev/migrations")

	tenantID := uuid.Must(uuid.NewV7()).String()
	if _, err := db.DB().ExecContext(ctx, `INSERT INTO tenants (id, name) VALUES (?, ?)`, tenantID, "Test Tenant"); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	workspaceID := uuid.Must(uuid.NewV7()).String()
	if _, err := db.DB().ExecContext(ctx, `INSERT INTO workspaces (id, tenant_id, name) VALUES (?, ?, ?)`, workspaceID, tenantID, "Default"); err != nil {
		t.Fatalf("insert section: %v", err)
	}
	agentID := uuid.Must(uuid.NewV7())
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO agents (id, workspace_id, name, api_key_hash, base_config) VALUES (?, ?, ?, ?, ?)`,
		agentID.String(), workspaceID, "test-agent", "fakehash", "{}"); err != nil {
		t.Fatalf("insert agent: %v", err)
	}
	return db, agentID
}

func postHeartbeat(t *testing.T, router *chi.Mux, agentID uuid.UUID, body *api.AgentHeartbeat) *httptest.ResponseRecorder {
	t.Helper()
	payload, _ := json.Marshal(body)
	path := fmt.Sprintf("/agent/%s/heartbeat", agentID.String())
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

func TestHeartbeat_Integration_WithVitals_StoresSnapshot(t *testing.T) {
	db, agentID := setupRealDB(t)
	h := NewHandler(logger.Default(), db)
	router := setupTestRouter(h)

	now := time.Now().UTC()
	body := &api.AgentHeartbeat{
		Timestamp:    now,
		HealthStatus: api.Healthy,
		Metrics: api.AgentMetrics{
			CpuUsagePercent:  55.0,
			MemoryUsageMb:    2048.0,
			MemoryTotalMb:    8192.0,
			SystemUptimeSecs: 172800,
			AgentUptimeSecs:  7200,
		},
		AgentStatus: api.AgentStatus{
			AgentVersion:     "0.1.0",
			ConfigVersion:    1,
			IsRunning:        true,
			StartedAt:        now.Add(-time.Hour),
			ChecksPerformed:  50,
			ChecksSuccessful: 50,
			ChecksFailed:     0,
			ReportedAt:       now.Add(-30 * time.Second),
			ServerConnected:  true,
			CacheStats:       api.AgentCacheStats{Capacity: 1000, Len: 0},
		},
	}

	rr := postHeartbeat(t, router, agentID, body)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rr.Code, rr.Body.String())
	}

	var count int
	if err := db.DB().QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM agent_vitals WHERE agent_id = ?`, agentID.String()).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 vitals row, got %d", count)
	}

	if h.vitalsStoredTotal.Load() != 1 {
		t.Errorf("vitalsStoredTotal = %d, want 1", h.vitalsStoredTotal.Load())
	}
}

func TestHeartbeat_Integration_UpdatesAgentLastSeen(t *testing.T) {
	db, agentID := setupRealDB(t)
	h := NewHandler(logger.Default(), db)
	router := setupTestRouter(h)

	now := time.Now().UTC()
	body := &api.AgentHeartbeat{
		Timestamp:    now,
		HealthStatus: api.Healthy,
		Metrics: api.AgentMetrics{
			CpuUsagePercent:  10.0,
			MemoryUsageMb:    512.0,
			MemoryTotalMb:    4096.0,
			SystemUptimeSecs: 3600,
			AgentUptimeSecs:  1800,
		},
		AgentStatus: api.AgentStatus{
			AgentVersion:     "0.1.0",
			ConfigVersion:    1,
			IsRunning:        true,
			StartedAt:        now.Add(-30 * time.Minute),
			ChecksPerformed:  10,
			ChecksSuccessful: 10,
			ChecksFailed:     0,
			ReportedAt:       now.Add(-time.Minute),
			ServerConnected:  true,
			CacheStats:       api.AgentCacheStats{Capacity: 1000, Len: 0},
		},
	}
	postHeartbeat(t, router, agentID, body)

	var lastSeen *string
	if err := db.DB().QueryRowContext(context.Background(),
		`SELECT last_seen_at FROM agents WHERE id = ?`, agentID.String()).Scan(&lastSeen); err != nil {
		t.Fatalf("query last_seen_at: %v", err)
	}
	if lastSeen == nil {
		t.Error("expected last_seen_at to be set, got nil")
	}

	lastSeenTime, err := time.Parse(time.RFC3339Nano, *lastSeen)
	if err != nil {
		t.Fatalf("parse last_seen_at: %v", err)
	}
	if time.Since(lastSeenTime) > time.Second*5 {
		t.Errorf("last_seen_at = %s, expected recent timestamp", *lastSeen)
	}
}

func TestHeartbeat_Integration_StoresAgentStatus(t *testing.T) {
	db, agentID := setupRealDB(t)
	h := NewHandler(logger.Default(), db)
	router := setupTestRouter(h)

	now := time.Now().UTC().Truncate(time.Second)
	body := &api.AgentHeartbeat{
		Timestamp:    now,
		HealthStatus: api.Healthy,
		Metrics: api.AgentMetrics{
			CpuUsagePercent:  20.0,
			MemoryUsageMb:    256.0,
			MemoryTotalMb:    4096.0,
			SystemUptimeSecs: 100,
			AgentUptimeSecs:  50,
		},
		AgentStatus: api.AgentStatus{
			AgentVersion:      "1.2.3",
			ConfigVersion:     7,
			IsRunning:         true,
			StartedAt:         now.Add(-2 * time.Hour),
			StoppedAt:         nil,
			ChecksPerformed:   200,
			ChecksSuccessful:  195,
			ChecksFailed:      5,
			ReportedAt:        now.Add(-10 * time.Second),
			FailedReportCount: 1,
			ServerConnected:   true,
			CacheStats:        api.AgentCacheStats{Capacity: 500, Len: 12},
		},
	}

	rr := postHeartbeat(t, router, agentID, body)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rr.Code, rr.Body.String())
	}

	var (
		isRunning         int
		checksPerformed   int
		checksSuccessful  int
		checksFailed      int
		failedReportCount int
		serverConnected   int
		cacheCapacity     int
		cacheLen          int
	)
	err := db.DB().QueryRowContext(context.Background(), `
		SELECT is_running,
		       checks_performed, checks_successful, checks_failed,
		       failed_report_count, server_connected, cache_capacity, cache_len
		FROM agent_vitals WHERE agent_id = ?`, agentID.String()).Scan(
		&isRunning,
		&checksPerformed, &checksSuccessful, &checksFailed,
		&failedReportCount, &serverConnected, &cacheCapacity, &cacheLen,
	)
	if err != nil {
		t.Fatalf("query agent_status fields: %v", err)
	}
	if isRunning != 1 {
		t.Errorf("is_running = %d, want 1", isRunning)
	}
	if checksPerformed != 200 {
		t.Errorf("checks_performed = %d, want 200", checksPerformed)
	}
	if checksSuccessful != 195 {
		t.Errorf("checks_successful = %d, want 195", checksSuccessful)
	}
	if checksFailed != 5 {
		t.Errorf("checks_failed = %d, want 5", checksFailed)
	}
	if failedReportCount != 1 {
		t.Errorf("failed_report_count = %d, want 1", failedReportCount)
	}
	if serverConnected != 1 {
		t.Errorf("server_connected = %d, want 1", serverConnected)
	}
	if cacheCapacity != 500 {
		t.Errorf("cache_capacity = %d, want 500", cacheCapacity)
	}
	if cacheLen != 12 {
		t.Errorf("cache_len = %d, want 12", cacheLen)
	}

	// Versions live on the agents row, not in the vitals time series.
	var agentVersion string
	var configVersionCurrent int
	if err := db.DB().QueryRowContext(context.Background(),
		`SELECT agent_version, config_version_current FROM agents WHERE id = ?`, agentID.String(),
	).Scan(&agentVersion, &configVersionCurrent); err != nil {
		t.Fatalf("query agents versions: %v", err)
	}
	if agentVersion != "1.2.3" {
		t.Errorf("agents.agent_version = %q, want 1.2.3", agentVersion)
	}
	if configVersionCurrent != 7 {
		t.Errorf("agents.config_version_current = %d, want 7", configVersionCurrent)
	}

	rows, err := db.DB().QueryContext(context.Background(), `SELECT * FROM agent_vitals LIMIT 0`)
	if err != nil {
		t.Fatalf("query agent_vitals columns: %v", err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatalf("read agent_vitals columns: %v", err)
	}
	for _, c := range cols {
		if c == "agent_version" || c == "config_version" {
			t.Errorf("agent_vitals must not contain column %q", c)
		}
	}
}

func versionHeartbeat(agentVersion string, configVersion int) *api.AgentHeartbeat {
	now := time.Now().UTC()
	return &api.AgentHeartbeat{
		Timestamp:    now,
		HealthStatus: api.Healthy,
		Metrics: api.AgentMetrics{
			CpuUsagePercent:  10.0,
			MemoryUsageMb:    512.0,
			MemoryTotalMb:    4096.0,
			SystemUptimeSecs: 3600,
			AgentUptimeSecs:  1800,
		},
		AgentStatus: api.AgentStatus{
			AgentVersion:     agentVersion,
			ConfigVersion:    configVersion,
			IsRunning:        true,
			StartedAt:        now.Add(-30 * time.Minute),
			ChecksPerformed:  10,
			ChecksSuccessful: 10,
			ReportedAt:       now.Add(-time.Minute),
			ServerConnected:  true,
			CacheStats:       api.AgentCacheStats{Capacity: 1000, Len: 0},
		},
	}
}

func readAgentVersions(t *testing.T, db database.Database, agentID uuid.UUID) (agentVersion *string, desired int, current *int) {
	t.Helper()
	if err := db.DB().QueryRowContext(context.Background(),
		`SELECT agent_version, config_version_desired, config_version_current FROM agents WHERE id = ?`,
		agentID.String(),
	).Scan(&agentVersion, &desired, &current); err != nil {
		t.Fatalf("query agent versions: %v", err)
	}
	return agentVersion, desired, current
}

// Regression: an agent upgrade must not bump config_version_desired.
func TestHeartbeat_Integration_AgentVersionChange_DoesNotBumpDesiredConfigVersion(t *testing.T) {
	db, agentID := setupRealDB(t)
	router := setupTestRouter(NewHandler(logger.Default(), db))

	_, desiredBefore, currentBefore := readAgentVersions(t, db, agentID)
	if currentBefore != nil {
		t.Fatalf("config_version_current = %d before first heartbeat, want NULL", *currentBefore)
	}

	for _, v := range []string{"0.1.0", "0.2.0"} {
		if rr := postHeartbeat(t, router, agentID, versionHeartbeat(v, desiredBefore)); rr.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d: %s", rr.Code, rr.Body.String())
		}
	}

	agentVersion, desiredAfter, current := readAgentVersions(t, db, agentID)
	if agentVersion == nil || *agentVersion != "0.2.0" {
		t.Errorf("agent_version = %v, want 0.2.0", agentVersion)
	}
	if desiredAfter != desiredBefore {
		t.Errorf("config_version_desired = %d, want unchanged %d", desiredAfter, desiredBefore)
	}
	if current == nil || *current != desiredBefore {
		t.Errorf("config_version_current = %v, want %d", current, desiredBefore)
	}
}

func TestHeartbeat_Integration_EmptyAgentVersion_KeepsStoredValue(t *testing.T) {
	db, agentID := setupRealDB(t)
	router := setupTestRouter(NewHandler(logger.Default(), db))

	postHeartbeat(t, router, agentID, versionHeartbeat("1.0.0", 1))
	postHeartbeat(t, router, agentID, versionHeartbeat("", 1))

	agentVersion, _, _ := readAgentVersions(t, db, agentID)
	if agentVersion == nil || *agentVersion != "1.0.0" {
		t.Errorf("agent_version = %v, want 1.0.0", agentVersion)
	}
}

func TestHeartbeat_Integration_ConfigDrift_DetectedAndResolved(t *testing.T) {
	db, agentID := setupRealDB(t)
	router := setupTestRouter(NewHandler(logger.Default(), db))

	_, desired, _ := readAgentVersions(t, db, agentID)
	postHeartbeat(t, router, agentID, versionHeartbeat("1.0.0", desired))

	// A core agent change bumps the desired version via trg_agents_updated.
	if _, err := db.DB().ExecContext(context.Background(),
		`UPDATE agents SET base_config = ? WHERE id = ?`, `{"changed":true}`, agentID.String()); err != nil {
		t.Fatalf("update base_config: %v", err)
	}

	_, newDesired, current := readAgentVersions(t, db, agentID)
	if newDesired <= desired {
		t.Fatalf("config_version_desired = %d, want > %d", newDesired, desired)
	}
	if current == nil || *current != desired {
		t.Fatalf("config_version_current = %v, want %d (drift)", current, desired)
	}

	postHeartbeat(t, router, agentID, versionHeartbeat("1.0.0", newDesired))
	_, _, current = readAgentVersions(t, db, agentID)
	if current == nil || *current != newDesired {
		t.Errorf("config_version_current = %v, want %d after catch-up", current, newDesired)
	}
}
