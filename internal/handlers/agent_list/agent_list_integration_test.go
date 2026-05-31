package agent_list

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

// testServerImpl satisfies the full api.StrictServerInterface by delegating
// ListAgents to the handler under test and stubbing everything else.
type testServerImpl struct {
	*Handler
}

func (s *testServerImpl) ListAgents(ctx context.Context, req api.ListAgentsRequestObject) (api.ListAgentsResponseObject, error) {
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

// setupRouter builds a chi router with the full middleware stack and the handler under test.
func setupRouter(db database.Database, handler *Handler) *chi.Mux {
	log := logger.Default()
	r := chi.NewRouter()
	r.Use(middleware.AgentAPIKeyAuth(log, db))
	r.Use(middleware.OAuth2Auth(log, db))

	impl := &testServerImpl{Handler: handler}
	api.HandlerFromMux(api.NewStrictHandler(impl, nil), r)
	return r
}

// setupDB creates a SQLite DB with all migrations and a tenant + user + session.
// Returns (db, plainSessionToken, agentID, sectionID).
func setupDB(t *testing.T) (database.Database, string, string, string) {
	t.Helper()
	db := testutil.SetupTestSQLiteDB(t)
	ctx := context.Background()
	testutil.ApplyMigrations(t, ctx, db.DB(), "../../../data/db/dev/migrations")

	q := queries.New(db.DB())

	// Tenant
	tenantID := uuid.Must(uuid.NewV7()).String()
	_, err := db.DB().ExecContext(ctx, `INSERT INTO tenants (id, name) VALUES (?, ?)`, tenantID, "Test Tenant")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	// Section
	sectionID := uuid.Must(uuid.NewV7()).String()
	_, err = db.DB().ExecContext(ctx, `INSERT INTO sections (id, tenant_id, name) VALUES (?, ?, ?)`,
		sectionID, tenantID, "Test Section")
	if err != nil {
		t.Fatalf("create section: %v", err)
	}

	// User
	userID := uuid.Must(uuid.NewV7()).String()
	_, err = db.DB().ExecContext(ctx, `INSERT INTO users (id, tenant_id, oauth_provider, oauth_subject, display_name) VALUES (?, ?, ?, ?, ?)`,
		userID, tenantID, "test_provider", "sub-123", "Test User")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Session with a known plaintext token
	plainToken := "test-session-token-abc123"
	tokenHash := middleware.HashAPIKeyForTests(plainToken)
	now := time.Now().UTC()
	_, err = q.CreateSession(ctx, queries.CreateSessionParams{
		ID:                 uuid.Must(uuid.NewV7()).String(),
		UserID:             userID,
		TokenHash:          tokenHash,
		SlidingExpiresAt:   now.Add(24 * time.Hour),
		ExpiresAt:          now.Add(24 * time.Hour),
		Oauth2Provider:     "test_provider",
		Oauth2AccessToken:  "fake-access-token",
		Oauth2RefreshToken: sql.NullString{},
		Oauth2TokenExpiry:  sql.NullTime{},
		Oauth2IDToken:      sql.NullString{},
		Oauth2Scope:        sql.NullString{},
		Oauth2TokenType:    "Bearer",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Two agents in the section
	baseConfig := `{"monitoring":{"interval_secs":60}}`
	for _, name := range []string{"agent-alpha", "agent-beta"} {
		agentID := uuid.Must(uuid.NewV7()).String()
		_, err = q.CreateAgent(ctx, queries.CreateAgentParams{
			ID:         agentID,
			SectionID:  sectionID,
			Name:       name,
			ApiKeyHash: middleware.HashAPIKeyForTests("key-" + name),
			BaseConfig: baseConfig,
		})
		if err != nil {
			t.Fatalf("create agent %s: %v", name, err)
		}
	}

	return db, plainToken, sectionID, tenantID
}

func TestListAgents_Integration_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db, sessionToken, _, _ := setupDB(t)
	handler := NewHandler(logger.Default(), db)
	router := setupRouter(db, handler)

	req := httptest.NewRequest(http.MethodGet, "/agents", nil)
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp api.AgentListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if len(resp.Agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(resp.Agents))
	}
	if resp.Pagination.TotalItems != 2 {
		t.Errorf("expected total_items=2, got %d", resp.Pagination.TotalItems)
	}
	if resp.Pagination.Page != 1 {
		t.Errorf("expected page=1, got %d", resp.Pagination.Page)
	}
	if resp.Pagination.PageSize != defaultPageSize {
		t.Errorf("expected page_size=%d, got %d", defaultPageSize, resp.Pagination.PageSize)
	}
}

func TestListAgents_Integration_PaginationPage2(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db, sessionToken, _, _ := setupDB(t)
	handler := NewHandler(logger.Default(), db)
	router := setupRouter(db, handler)

	// Request page 2 with page_size=1 (should return 1 of 2 agents)
	req := httptest.NewRequest(http.MethodGet, "/agents?page=2&page_size=1", nil)
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp api.AgentListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if len(resp.Agents) != 1 {
		t.Errorf("expected 1 agent on page 2, got %d", len(resp.Agents))
	}
	if resp.Pagination.TotalItems != 2 {
		t.Errorf("expected total_items=2, got %d", resp.Pagination.TotalItems)
	}
	if resp.Pagination.TotalPages != 2 {
		t.Errorf("expected total_pages=2, got %d", resp.Pagination.TotalPages)
	}
	if resp.Pagination.HasPrevious == nil || !*resp.Pagination.HasPrevious {
		t.Error("expected has_previous=true on page 2")
	}
	if resp.Pagination.HasNext == nil || *resp.Pagination.HasNext {
		t.Error("expected has_next=false on last page")
	}
}

func TestListAgents_Integration_AgentAPIKey_Returns401(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db, _, sectionID, _ := setupDB(t)
	ctx := context.Background()

	// Create an agent with a known API key
	agentID := uuid.Must(uuid.NewV7()).String()
	apiKey := "test-agent-api-key"
	_, err := queries.New(db.DB()).CreateAgent(ctx, queries.CreateAgentParams{
		ID:         agentID,
		SectionID:  sectionID,
		Name:       "auth-test-agent",
		ApiKeyHash: middleware.HashAPIKeyForTests(apiKey),
		BaseConfig: `{}`,
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// The raw Handler doesn't check auth type — that's AuthenticatedHandler's job.
	// Verify that a direct call with agent auth in context returns 401 from auth check.
	handler := NewHandler(logger.Default(), db)
	agentCtx := context.WithValue(context.Background(), middleware.AuthContextKey, &middleware.AuthInfo{
		AuthType:      "agent_api_key",
		Authenticated: true,
		AgentID:       agentID,
	})

	// Agent auth ctx → handler should return 500 (user lookup fails, not 401)
	// because the AuthenticatedHandler rejects agent API key before we get here.
	// The unit test for the auth check is in authenticated_handler_test.go.
	resp, err := handler.Handle(agentCtx, buildListRequest(nil, nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// When called with an agent auth context, UserID is empty → GetUserByID("") returns
	// sql.ErrNoRows → handler returns 401 (user not found).
	if _, ok := resp.(api.ListAgents401JSONResponse); !ok {
		t.Errorf("expected ListAgents401JSONResponse for agent auth, got %T", resp)
	}
}

func TestListAgents_Integration_EmptyTenant_Returns200WithEmptyList(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db := testutil.SetupTestSQLiteDB(t)
	ctx := context.Background()
	testutil.ApplyMigrations(t, ctx, db.DB(), "../../../data/db/dev/migrations")

	q := queries.New(db.DB())

	// Tenant + user + session but NO agents
	tenantID := uuid.Must(uuid.NewV7()).String()
	_, err := db.DB().ExecContext(ctx, `INSERT INTO tenants (id, name) VALUES (?, ?)`, tenantID, "Empty Tenant")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	userID := uuid.Must(uuid.NewV7()).String()
	_, err = db.DB().ExecContext(ctx, `INSERT INTO users (id, tenant_id, oauth_provider, oauth_subject, display_name) VALUES (?, ?, ?, ?, ?)`,
		userID, tenantID, "p", "s", "U")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	plainToken := "empty-tenant-token"
	now := time.Now().UTC()
	_, err = q.CreateSession(ctx, queries.CreateSessionParams{
		ID:                uuid.Must(uuid.NewV7()).String(),
		UserID:            userID,
		TokenHash:         middleware.HashAPIKeyForTests(plainToken),
		SlidingExpiresAt:  now.Add(time.Hour),
		ExpiresAt:         now.Add(time.Hour),
		Oauth2Provider:    "p",
		Oauth2AccessToken: "tok",
		Oauth2TokenType:   "Bearer",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	handler := NewHandler(logger.Default(), db)
	router := setupRouter(db, handler)

	req := httptest.NewRequest(http.MethodGet, "/agents", nil)
	req.Header.Set("Authorization", "Bearer "+plainToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp api.AgentListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.Agents) != 0 {
		t.Errorf("expected 0 agents for empty tenant, got %d", len(resp.Agents))
	}
	if resp.Pagination.TotalItems != 0 {
		t.Errorf("expected total_items=0, got %d", resp.Pagination.TotalItems)
	}
}
