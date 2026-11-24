package agent_get

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	healthAPI "github.com/smotra-monitoring/server/internal/api/health"
	api "github.com/smotra-monitoring/server/internal/api/v1"
	"github.com/smotra-monitoring/server/internal/database"
	"github.com/smotra-monitoring/server/internal/database/queries"
	"github.com/smotra-monitoring/server/internal/logger"
	"github.com/smotra-monitoring/server/internal/middleware"
	"github.com/smotra-monitoring/server/internal/testutil"
)

// testServerImpl satisfies api.StrictServerInterface by delegating GetAgent
// to the handler under test (with OAuth2 auth enforcement) and stubbing everything else.
type testServerImpl struct {
	*Handler
}

func (s *testServerImpl) GetAgent(ctx context.Context, req api.GetAgentRequestObject) (api.GetAgentResponseObject, error) {
	authInfo, ok := ctx.Value(middleware.AuthContextKey).(*middleware.AuthInfo)
	if !ok || authInfo == nil || !authInfo.Authenticated || authInfo.AuthType != "oauth2" || authInfo.UserID == "" || authInfo.SessionID == "" {
		return api.GetAgent401JSONResponse{UnauthorizedJSONResponse: sessionUnauthorized}, nil
	}
	return s.Handle(ctx, req)
}

func (s *testServerImpl) ListAgents(ctx context.Context, req api.ListAgentsRequestObject) (api.ListAgentsResponseObject, error) {
	return nil, nil
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
func (s *testServerImpl) SendAgentHeartbeat(ctx context.Context, req api.SendAgentHeartbeatRequestObject) (api.SendAgentHeartbeatResponseObject, error) {
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

// Health stubs
func (s *testServerImpl) HealthCheck(ctx context.Context, req healthAPI.HealthCheckRequestObject) (healthAPI.HealthCheckResponseObject, error) {
	return nil, nil
}
func (s *testServerImpl) LivenessCheck(ctx context.Context, req healthAPI.LivenessCheckRequestObject) (healthAPI.LivenessCheckResponseObject, error) {
	return nil, nil
}
func (s *testServerImpl) ReadinessCheck(ctx context.Context, req healthAPI.ReadinessCheckRequestObject) (healthAPI.ReadinessCheckResponseObject, error) {
	return nil, nil
}
func (s *testServerImpl) PrometheusMetrics(ctx context.Context, req healthAPI.PrometheusMetricsRequestObject) (healthAPI.PrometheusMetricsResponseObject, error) {
	return healthAPI.PrometheusMetrics200TextResponse(""), nil
}

func setupRouter(db database.Database, handler *Handler) *chi.Mux {
	log := logger.Default()
	r := chi.NewRouter()
	r.Use(middleware.AgentAPIKeyAuth(log, db))
	r.Use(middleware.OAuth2Auth(log, db))

	impl := &testServerImpl{Handler: handler}
	api.HandlerFromMux(api.NewStrictHandler(impl, nil), r)
	return r
}

type testFixture struct {
	db              database.Database
	sessionToken    string
	agentAPIKey     string
	agentID         string
	workspaceID     string
	otherAgentID    string
	otherWorkspceID string
	tenantID        string
}

func setupDB(t *testing.T) testFixture {
	t.Helper()
	db := testutil.SetupTestSQLiteDB(t)
	ctx := context.Background()
	testutil.ApplyMigrations(t, ctx, db.DB(), "../../../data/db/dev/migrations")

	q := queries.New(db.DB())

	tenantID := uuid.Must(uuid.NewV7()).String()
	if _, err := db.DB().ExecContext(ctx, `INSERT INTO tenants (id, name) VALUES (?, ?)`, tenantID, "Test Tenant"); err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	workspaceID := uuid.Must(uuid.NewV7()).String()
	if _, err := db.DB().ExecContext(ctx, `INSERT INTO workspaces (id, tenant_id, name) VALUES (?, ?, ?)`,
		workspaceID, tenantID, "Primary Workspace"); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	otherWorkspaceID := uuid.Must(uuid.NewV7()).String()
	if _, err := db.DB().ExecContext(ctx, `INSERT INTO workspaces (id, tenant_id, name) VALUES (?, ?, ?)`,
		otherWorkspaceID, tenantID, "Other Workspace"); err != nil {
		t.Fatalf("create other workspace: %v", err)
	}

	userID := uuid.Must(uuid.NewV7()).String()
	if _, err := db.DB().ExecContext(ctx, `INSERT INTO users (id, workspace_id, oauth_provider, oauth_subject, display_name) VALUES (?, ?, ?, ?, ?)`,
		userID, workspaceID, "test_provider", "sub-123", "Test User"); err != nil {
		t.Fatalf("create user: %v", err)
	}

	plainToken := "test-session-token-abc123"
	now := time.Now().UTC()
	if _, err := q.CreateSession(ctx, queries.CreateSessionParams{
		ID:                 uuid.Must(uuid.NewV7()).String(),
		UserID:             userID,
		TokenHash:          middleware.HashAPIKeyForTests(plainToken),
		SlidingExpiresAt:   now.Add(24 * time.Hour),
		ExpiresAt:          now.Add(24 * time.Hour),
		Oauth2Provider:     "test_provider",
		Oauth2AccessToken:  "fake-access-token",
		Oauth2RefreshToken: sql.NullString{},
		Oauth2TokenExpiry:  sql.NullTime{},
		Oauth2IDToken:      sql.NullString{},
		Oauth2Scope:        sql.NullString{},
		Oauth2TokenType:    "Bearer",
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	agentID := uuid.Must(uuid.NewV7()).String()
	agentAPIKey := "sk_test_agent_key_123456"
	ipJSON := `[{"ip":"192.168.1.10","iface":"eth0","family":"ipv4","recommended":true}]`
	if _, err := q.CreateAgentFromClaim(ctx, queries.CreateAgentFromClaimParams{
		ID:              agentID,
		WorkspaceID:     workspaceID,
		Name:            "agent-alpha",
		ApiKeyHash:      middleware.HashAPIKeyForTests(agentAPIKey),
		BaseConfig:      `{"monitoring":{"interval_secs":60}}`,
		AgentVersion:    sql.NullString{String: "1.2.3", Valid: true},
		IpAddressesJson: ipJSON,
	}); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Update config (trigger bumps config_version_desired from 1 -> 2) and heartbeat state.
	if err := q.UpdateAgentConfiguration(ctx, queries.UpdateAgentConfigurationParams{
		ConfigVersionDesired: 2,
		BaseConfig:           `{"monitoring":{"interval_secs":30}}`,
		ID:                   agentID,
	}); err != nil {
		t.Fatalf("update agent config: %v", err)
	}
	if _, err := q.UpdateAgentHeartbeatState(ctx, queries.UpdateAgentHeartbeatStateParams{
		LastSeenAt:           sql.NullTime{Time: now, Valid: true},
		AgentVersion:         "1.2.3",
		ConfigVersionCurrent: sql.NullInt64{Int64: 1, Valid: true},
		ID:                   agentID,
	}); err != nil {
		t.Fatalf("update agent heartbeat state: %v", err)
	}
	if err := q.UpdateAgentLastResultSubmittedAt(ctx, queries.UpdateAgentLastResultSubmittedAtParams{
		LastResultSubmittedAt: sql.NullTime{Time: now.Add(-time.Minute), Valid: true},
		ID:                    agentID,
	}); err != nil {
		t.Fatalf("update last_result_submitted_at: %v", err)
	}

	otherAgentID := uuid.Must(uuid.NewV7()).String()
	if _, err := q.CreateAgent(ctx, queries.CreateAgentParams{
		ID:          otherAgentID,
		WorkspaceID: otherWorkspaceID,
		Name:        "agent-other-workspace",
		ApiKeyHash:  middleware.HashAPIKeyForTests("key-other"),
		BaseConfig:  `{}`,
	}); err != nil {
		t.Fatalf("create other agent: %v", err)
	}

	return testFixture{
		db:              db,
		sessionToken:    plainToken,
		agentAPIKey:     agentAPIKey,
		agentID:         agentID,
		workspaceID:     workspaceID,
		otherAgentID:    otherAgentID,
		otherWorkspceID: otherWorkspaceID,
		tenantID:        tenantID,
	}
}

func TestGetAgent_Integration_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	fx := setupDB(t)
	handler := NewHandler(logger.Default(), fx.db)
	router := setupRouter(fx.db, handler)

	req := httptest.NewRequest(http.MethodGet, "/agents/"+fx.agentID, nil)
	req.Header.Set("Authorization", "Bearer "+fx.sessionToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp api.Agent
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if got := uuid.UUID(resp.Id).String(); got != fx.agentID {
		t.Errorf("id = %s, want %s", got, fx.agentID)
	}
	if got := uuid.UUID(resp.WorkspaceId).String(); got != fx.workspaceID {
		t.Errorf("workspace_id = %s, want %s", got, fx.workspaceID)
	}
	if resp.Name != "agent-alpha" {
		t.Errorf("name = %q, want %q", resp.Name, "agent-alpha")
	}
	if resp.ConfigVersionDesired != 2 {
		t.Errorf("config_version_desired = %d, want 2", resp.ConfigVersionDesired)
	}
	if resp.ConfigVersionCurrent == nil || *resp.ConfigVersionCurrent != 1 {
		t.Errorf("config_version_current = %v, want 1", resp.ConfigVersionCurrent)
	}
	if resp.AgentVersion == nil || *resp.AgentVersion != "1.2.3" {
		t.Errorf("agent_version = %v, want 1.2.3", resp.AgentVersion)
	}
	if resp.IpAddresses == nil || len(*resp.IpAddresses) != 1 || (*resp.IpAddresses)[0].Ip != "192.168.1.10" {
		t.Errorf("ip_addresses = %+v, want 1 entry with 192.168.1.10", resp.IpAddresses)
	}
	if resp.LastSeenAt == nil {
		t.Error("expected last_seen_at to be populated")
	}
	if resp.LastResultSubmittedAt == nil {
		t.Error("expected last_result_submitted_at to be populated")
	}
	if handler.getSuccessTotal.Load() != 1 {
		t.Errorf("expected getSuccessTotal=1, got %d", handler.getSuccessTotal.Load())
	}
}

func TestGetAgent_Integration_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	fx := setupDB(t)
	handler := NewHandler(logger.Default(), fx.db)
	router := setupRouter(fx.db, handler)

	missingID := uuid.Must(uuid.NewV7()).String()
	req := httptest.NewRequest(http.MethodGet, "/agents/"+missingID, nil)
	req.Header.Set("Authorization", "Bearer "+fx.sessionToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d. Body: %s", w.Code, w.Body.String())
	}
	if handler.getNotFoundTotal.Load() != 1 {
		t.Errorf("expected getNotFoundTotal=1, got %d", handler.getNotFoundTotal.Load())
	}
	if handler.getFailureTotal.Load() != 1 {
		t.Errorf("expected getFailureTotal=1, got %d", handler.getFailureTotal.Load())
	}
}

func TestGetAgent_Integration_ForeignWorkspace_Returns401(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	fx := setupDB(t)
	handler := NewHandler(logger.Default(), fx.db)
	router := setupRouter(fx.db, handler)

	req := httptest.NewRequest(http.MethodGet, "/agents/"+fx.otherAgentID, nil)
	req.Header.Set("Authorization", "Bearer "+fx.sessionToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for foreign workspace, got %d. Body: %s", w.Code, w.Body.String())
	}
	if handler.getWorkspaceDeniedTotal.Load() != 1 {
		t.Errorf("expected getWorkspaceDeniedTotal=1, got %d", handler.getWorkspaceDeniedTotal.Load())
	}
	if handler.getFailureTotal.Load() != 1 {
		t.Errorf("expected getFailureTotal=1, got %d", handler.getFailureTotal.Load())
	}
}

func TestGetAgent_Integration_UnauthenticatedAndAgentAPIKey_Return401(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	fx := setupDB(t)
	handler := NewHandler(logger.Default(), fx.db)
	router := setupRouter(fx.db, handler)

	t.Run("no auth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/agents/"+fx.agentID, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", w.Code)
		}
	})

	t.Run("agent api key rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/agents/"+fx.agentID, nil)
		req.Header.Set("X-Agent-API-Key", fx.agentAPIKey)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 when using agent API key, got %d", w.Code)
		}
	})
}

func TestGetAgent_Integration_MissingUser_Returns401(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	fx := setupDB(t)
	handler := NewHandler(logger.Default(), fx.db)

	resp, err := handler.Handle(oauth2Ctx(uuid.Must(uuid.NewV7()).String()), buildGetRequest(uuid.MustParse(fx.agentID)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := resp.(api.GetAgent401JSONResponse); !ok {
		t.Errorf("expected GetAgent401JSONResponse for missing user, got %T", resp)
	}
}

func TestGetAgent_Integration_MalformedUUID_Returns400(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	fx := setupDB(t)
	handler := NewHandler(logger.Default(), fx.db)
	router := setupRouter(fx.db, handler)

	req := httptest.NewRequest(http.MethodGet, "/agents/not-a-uuid", nil)
	req.Header.Set("Authorization", "Bearer "+fx.sessionToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for malformed UUID, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestGetAgent_Integration_MetricsOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	fx := setupDB(t)
	handler := NewHandler(logger.Default(), fx.db)
	router := setupRouter(fx.db, handler)

	// 1 success + 1 not found + 1 workspace denied
	for _, id := range []string{fx.agentID, uuid.Must(uuid.NewV7()).String(), fx.otherAgentID} {
		req := httptest.NewRequest(http.MethodGet, "/agents/"+id, nil)
		req.Header.Set("Authorization", "Bearer "+fx.sessionToken)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}

	out := handler.GetMetrics()
	for _, expected := range []string{
		"smotra_agent_get_attempts_total 3",
		"smotra_agent_get_success_total 1",
		"smotra_agent_get_failure_total 2",
		"smotra_agent_get_not_found_total 1",
		"smotra_agent_get_workspace_denied_total 1",
	} {
		if !strings.Contains(out, expected) {
			t.Errorf("expected %q in metrics output, got:\n%s", expected, out)
		}
	}
}
