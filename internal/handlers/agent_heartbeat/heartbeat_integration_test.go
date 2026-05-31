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
	sectionID := uuid.Must(uuid.NewV7()).String()
	if _, err := db.DB().ExecContext(ctx, `INSERT INTO sections (id, tenant_id, name) VALUES (?, ?, ?)`, sectionID, tenantID, "Default"); err != nil {
		t.Fatalf("insert section: %v", err)
	}
	agentID := uuid.Must(uuid.NewV7())
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO agents (id, section_id, name, api_key_hash, base_config) VALUES (?, ?, ?, ?, ?)`,
		agentID.String(), sectionID, "test-agent", "fakehash", "{}"); err != nil {
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

	body := &api.AgentHeartbeat{
		Timestamp:        time.Now().UTC(),
		Status:           api.Healthy,
		CpuUsagePercent:  55.0,
		MemoryUsageMb:    2048.0,
		MemoryTotalMb:    8192.0,
		SystemUptimeSecs: 172800,
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

	body := &api.AgentHeartbeat{
		Timestamp:        time.Now().UTC(),
		Status:           api.Healthy,
		CpuUsagePercent:  10.0,
		MemoryUsageMb:    512.0,
		MemoryTotalMb:    4096.0,
		SystemUptimeSecs: 3600,
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
