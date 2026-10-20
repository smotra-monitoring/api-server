package agent_claim

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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

// testServerImpl wraps Handler and implements the full StrictServerInterface
type testServerImpl struct {
	*Handler
}

// PostClaimAgent delegates to this handler
func (t *testServerImpl) PostClaimAgent(ctx context.Context, request api.PostClaimAgentRequestObject) (api.PostClaimAgentResponseObject, error) {
	return t.Handle(ctx, request)
}

// Stub implementations for other endpoints (required by StrictServerInterface)
func (t *testServerImpl) HealthCheck(ctx context.Context, request healthAPI.HealthCheckRequestObject) (healthAPI.HealthCheckResponseObject, error) {
	return nil, nil
}

func (t *testServerImpl) LivenessCheck(ctx context.Context, request healthAPI.LivenessCheckRequestObject) (healthAPI.LivenessCheckResponseObject, error) {
	return nil, nil
}

func (t *testServerImpl) ReadinessCheck(ctx context.Context, request healthAPI.ReadinessCheckRequestObject) (healthAPI.ReadinessCheckResponseObject, error) {
	return nil, nil
}

func (t *testServerImpl) PrometheusMetrics(ctx context.Context, request healthAPI.PrometheusMetricsRequestObject) (healthAPI.PrometheusMetricsResponseObject, error) {
	return healthAPI.PrometheusMetrics200TextResponse(""), nil
}

func (t *testServerImpl) GetAgentConfiguration(ctx context.Context, request api.GetAgentConfigurationRequestObject) (api.GetAgentConfigurationResponseObject, error) {
	return nil, nil
}

func (t *testServerImpl) RegisterAgentSelf(ctx context.Context, request api.RegisterAgentSelfRequestObject) (api.RegisterAgentSelfResponseObject, error) {
	return nil, nil
}

func (t *testServerImpl) GetAgentClaimStatus(ctx context.Context, request api.GetAgentClaimStatusRequestObject) (api.GetAgentClaimStatusResponseObject, error) {
	return nil, nil
}

func (t *testServerImpl) Logout(ctx context.Context, request api.LogoutRequestObject) (api.LogoutResponseObject, error) {
	return nil, nil
}

func (t *testServerImpl) AuthRefresh(ctx context.Context, req api.AuthRefreshRequestObject) (api.AuthRefreshResponseObject, error) {
	return nil, nil
}

func (t *testServerImpl) Oauth2Authorize(ctx context.Context, request api.Oauth2AuthorizeRequestObject) (api.Oauth2AuthorizeResponseObject, error) {
	return nil, nil
}

func (t *testServerImpl) Oauth2Callback(ctx context.Context, request api.Oauth2CallbackRequestObject) (api.Oauth2CallbackResponseObject, error) {
	return nil, nil
}

func (t *testServerImpl) Oauth2Revoke(ctx context.Context, request api.Oauth2RevokeRequestObject) (api.Oauth2RevokeResponseObject, error) {
	return nil, nil
}

func (t *testServerImpl) Oauth2Token(ctx context.Context, request api.Oauth2TokenRequestObject) (api.Oauth2TokenResponseObject, error) {
	return nil, nil
}

func (t *testServerImpl) GetUserInfo(ctx context.Context, request api.GetUserInfoRequestObject) (api.GetUserInfoResponseObject, error) {
	return nil, nil
}

func (t *testServerImpl) SubmitAgentResults(ctx context.Context, request api.SubmitAgentResultsRequestObject) (api.SubmitAgentResultsResponseObject, error) {
	return nil, nil
}

func (t *testServerImpl) SendAgentHeartbeat(ctx context.Context, request api.SendAgentHeartbeatRequestObject) (api.SendAgentHeartbeatResponseObject, error) {
	return nil, nil
}
func (t *testServerImpl) ListAgents(ctx context.Context, request api.ListAgentsRequestObject) (api.ListAgentsResponseObject, error) {
	return nil, nil
}

func setupTestRouter(handler *Handler) *chi.Mux {
	testImpl := &testServerImpl{Handler: handler}
	r := chi.NewRouter()
	strictHandler := api.NewStrictHandler(testImpl, nil)
	api.HandlerFromMux(strictHandler, r)
	return r
}

// setupTestRouterWithAuth builds a router that runs the OAuth2 middleware so tests
// can inject a real session token via the Authorization header.
func setupTestRouterWithAuth(db database.Database, handler *Handler) *chi.Mux {
	testImpl := &testServerImpl{Handler: handler}
	r := chi.NewRouter()
	r.Use(middleware.OAuth2Auth(logger.Default(), db))
	strictHandler := api.NewStrictHandler(testImpl, nil)
	api.HandlerFromMux(strictHandler, r)
	return r
}

// setupUserAndSession creates a user in the given workspace and a valid OAuth2 session.
// Returns the userID and a plain Bearer token for use in test requests.
func setupUserAndSession(t *testing.T, db database.Database, workspaceID string) (userID, plainToken string) {
	t.Helper()
	ctx := context.Background()
	q := queries.New(db.DB())

	userID = uuid.Must(uuid.NewV7()).String()
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO users (id, workspace_id, oauth_provider, oauth_subject, display_name) VALUES (?, ?, 'github', ?, 'Test User')`,
		userID, workspaceID, "sub-"+userID[:8]); err != nil {
		t.Fatalf("create user: %v", err)
	}

	plainToken = "test-session-token-" + userID
	now := time.Now().UTC()
	if _, err := q.CreateSession(ctx, queries.CreateSessionParams{
		ID:                uuid.Must(uuid.NewV7()).String(),
		UserID:            userID,
		TokenHash:         middleware.HashAPIKeyForTests(plainToken),
		SlidingExpiresAt:  now.Add(24 * time.Hour),
		ExpiresAt:         now.Add(24 * time.Hour),
		Oauth2Provider:    "github",
		Oauth2AccessToken: "tok",
		Oauth2TokenType:   "Bearer",
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	return
}

func TestClaimAgent_Integration_Success(t *testing.T) {
	log := logger.Default()
	db := testutil.SetupTestSQLiteDB(t)
	ctx := context.Background()

	q := queries.New(db.DB())
	testutil.ApplyMigrations(t, ctx, db.DB(), "../../../data/db/dev/migrations")

	handler := NewHandler(log, db)

	// Create tenant
	tenantID := uuid.Must(uuid.NewV7()).String()
	_, err := db.DB().ExecContext(ctx, `INSERT INTO tenants (id, name) VALUES (?, ?)`,
		tenantID, "Test Tenant")
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	// Create workspace
	workspaceID := uuid.Must(uuid.NewV7()).String()
	_, err = db.DB().ExecContext(ctx, `INSERT INTO workspaces (id, tenant_id, name) VALUES (?, ?, ?)`,
		workspaceID, tenantID, "Default Section")
	if err != nil {
		t.Fatalf("Failed to create section: %v", err)
	}

	// Create authenticated user in the workspace
	_, plainToken := setupUserAndSession(t, db, workspaceID)
	router := setupTestRouterWithAuth(db, handler)

	// Create unclaimed agent
	agentID := uuid.Must(uuid.NewV7())
	claimToken := "test-claim-token-12345678"
	claimTokenHash := sha256.Sum256([]byte(claimToken))
	claimTokenHashStr := hex.EncodeToString(claimTokenHash[:])
	expiresAt := time.Now().Add(24 * time.Hour)

	_, err = q.UpsertAgentClaim(ctx, queries.UpsertAgentClaimParams{
		ID:                  agentID.String(),
		ClaimTokenHash:      claimTokenHashStr,
		Hostname:            "test-host",
		AgentVersion:        "1.0.0",
		ClaimTokenExpiresAt: expiresAt,
	})
	if err != nil {
		t.Fatalf("Failed to create agent claim: %v", err)
	}

	// Claim the agent
	reqBody := api.ClaimAgentRequest{
		AgentId:    agentID,
		ClaimToken: claimToken,
		Name:       ptrString("My Test Agent"),
	}

	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/agent/claim", bytes.NewReader(reqJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+plainToken)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	//
	// API response verification
	//

	var response api.ClaimAgentResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.Status != "claimed" {
		t.Errorf("Expected status 'claimed', got '%s'", response.Status)
	}

	if response.AgentId.String() != agentID.String() {
		t.Errorf("Expected agentId '%s', got '%s'", agentID.String(), response.AgentId.String())
	}

	//
	// Database verification
	//

	// Verify agent was created in production table
	// Verify claimFromDB was marked as claimed and agent was created
	claimFromDB, err := q.GetAgentClaim(ctx, agentID.String())
	if err != nil {
		t.Fatalf("Failed to get agent claim: %v", err)
	}

	// Verify claim was marked as claimed
	if !claimFromDB.ClaimedAt.Valid {
		t.Error("Expected claimed_at to be set")
	}

	if !claimFromDB.ClaimedByUserID.Valid {
		t.Error("Expected claimed_by_user_id to be set")
	}

	if !claimFromDB.ApiKeyPlaintext.Valid || claimFromDB.ApiKeyPlaintext.String == "" {
		t.Error("Expected api_key_plaintext to be set for delivery")
	}
}

func TestClaimAgent_Integration_InvalidToken(t *testing.T) {
	log := logger.Default()
	db := testutil.SetupTestSQLiteDB(t)
	ctx := context.Background()

	q := queries.New(db.DB())
	testutil.ApplyMigrations(t, ctx, db.DB(), "../../../data/db/dev/migrations")

	handler := NewHandler(log, db)

	// Create tenant and section
	tenantID := uuid.Must(uuid.NewV7()).String()
	_, err := db.DB().ExecContext(ctx, `INSERT INTO tenants (id, name) VALUES (?, ?)`,
		tenantID, "Test Tenant")
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	workspaceID := uuid.Must(uuid.NewV7()).String()
	_, err = db.DB().ExecContext(ctx, `INSERT INTO workspaces (id, tenant_id, name) VALUES (?, ?, ?)`,
		workspaceID, tenantID, "Default Section")
	if err != nil {
		t.Fatalf("Failed to create section: %v", err)
	}

	_, plainToken := setupUserAndSession(t, db, workspaceID)
	router := setupTestRouterWithAuth(db, handler)

	// Create unclaimed agent
	agentID := uuid.Must(uuid.NewV7())
	claimToken := "test-claim-token-12345678"
	claimTokenHash := sha256.Sum256([]byte(claimToken))
	claimTokenHashStr := hex.EncodeToString(claimTokenHash[:])
	expiresAt := time.Now().Add(24 * time.Hour)

	_, err = q.UpsertAgentClaim(ctx, queries.UpsertAgentClaimParams{
		ID:                  agentID.String(),
		ClaimTokenHash:      claimTokenHashStr,
		Hostname:            "test-host",
		AgentVersion:        "1.0.0",
		ClaimTokenExpiresAt: expiresAt,
	})
	if err != nil {
		t.Fatalf("Failed to create agent claim: %v", err)
	}

	// Try to claim with wrong token
	reqBody := api.ClaimAgentRequest{
		AgentId:    agentID,
		ClaimToken: "wrong-token-12345678",
		Name:       ptrString("My Test Agent"),
	}

	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/agent/claim", bytes.NewReader(reqJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+plainToken)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("Expected status 409, got %d. Body: %s", w.Code, w.Body.String())
	}

	var errResp api.Error
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("Failed to unmarshal error response: %v", err)
	}

	if errResp.Error != "already_claimed_or_invalid" {
		t.Errorf("Expected error 'already_claimed_or_invalid', got '%s'", errResp.Error)
	}
}

func TestClaimAgent_Integration_AlreadyClaimed(t *testing.T) {
	log := logger.Default()
	db := testutil.SetupTestSQLiteDB(t)
	ctx := context.Background()

	q := queries.New(db.DB())
	testutil.ApplyMigrations(t, ctx, db.DB(), "../../../data/db/dev/migrations")

	handler := NewHandler(log, db)

	// Create tenant, user, and workspace
	tenantID := uuid.Must(uuid.NewV7()).String()
	_, err := db.DB().ExecContext(ctx, `INSERT INTO tenants (id, name) VALUES (?, ?)`,
		tenantID, "Test Tenant")
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	workspaceID := uuid.Must(uuid.NewV7()).String()
	_, err = db.DB().ExecContext(ctx, `INSERT INTO workspaces (id, tenant_id, name) VALUES (?, ?, ?)`,
		workspaceID, tenantID, "Default Section")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}

	_, plainToken := setupUserAndSession(t, db, workspaceID)
	router := setupTestRouterWithAuth(db, handler)

	// Create already claimed agent
	agentID := uuid.Must(uuid.NewV7())
	claimToken := "test-claim-token-12345678"
	claimTokenHash := sha256.Sum256([]byte(claimToken))
	claimTokenHashStr := hex.EncodeToString(claimTokenHash[:])
	expiresAt := time.Now().Add(24 * time.Hour)

	_, err = q.UpsertAgentClaim(ctx, queries.UpsertAgentClaimParams{
		ID:                  agentID.String(),
		ClaimTokenHash:      claimTokenHashStr,
		Hostname:            "test-host",
		AgentVersion:        "1.0.0",
		ClaimTokenExpiresAt: expiresAt,
	})
	if err != nil {
		t.Fatalf("Failed to create agent claim: %v", err)
	}

	// Claim the agent
	reqBody1 := api.ClaimAgentRequest{
		AgentId:    agentID,
		ClaimToken: claimToken,
		Name:       ptrString("My Test Agent"),
	}

	reqJSON1, _ := json.Marshal(reqBody1)
	req1 := httptest.NewRequest(http.MethodPost, "/agent/claim", bytes.NewReader(reqJSON1))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Authorization", "Bearer "+plainToken)

	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w1.Code, w1.Body.String())
	}

	// Try to claim again
	reqBody2 := api.ClaimAgentRequest{
		AgentId:    agentID,
		ClaimToken: claimToken,
		Name:       ptrString("My Test Agent"),
	}

	reqJSON2, _ := json.Marshal(reqBody2)
	req2 := httptest.NewRequest(http.MethodPost, "/agent/claim", bytes.NewReader(reqJSON2))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+plainToken)

	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusConflict {
		t.Errorf("Expected status 409, got %d. Body: %s", w2.Code, w2.Body.String())
	}

	var errResp api.Error
	if err := json.Unmarshal(w2.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("Failed to unmarshal error response: %v", err)
	}

	if errResp.Error != "already_claimed_or_invalid" {
		t.Errorf("Expected error 'already_claimed_or_invalid', got '%s'", errResp.Error)
	}
}

func TestClaimAgent_Integration_NotFound(t *testing.T) {
	log := logger.Default()
	db := testutil.SetupTestSQLiteDB(t)
	ctx := context.Background()

	testutil.ApplyMigrations(t, ctx, db.DB(), "../../../data/db/dev/migrations")

	handler := NewHandler(log, db)

	// Create tenant and section
	tenantID := uuid.Must(uuid.NewV7()).String()
	_, err := db.DB().ExecContext(ctx, `INSERT INTO tenants (id, name) VALUES (?, ?)`,
		tenantID, "Test Tenant")
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	workspaceID := uuid.Must(uuid.NewV7()).String()
	_, err = db.DB().ExecContext(ctx, `INSERT INTO workspaces (id, tenant_id, name) VALUES (?, ?, ?)`,
		workspaceID, tenantID, "Default Section")
	if err != nil {
		t.Fatalf("Failed to create section: %v", err)
	}

	_, plainToken := setupUserAndSession(t, db, workspaceID)
	router := setupTestRouterWithAuth(db, handler)

	// Try to claim non-existent agent
	agentID := uuid.Must(uuid.NewV7())
	reqBody := api.ClaimAgentRequest{
		AgentId:    agentID,
		ClaimToken: "some-token",
		Name:       ptrString("My Test Agent"),
	}

	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/agent/claim", bytes.NewReader(reqJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+plainToken)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 (not found), got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestClaimAgent_Integration_UsesHostnameWhenNameNotProvided(t *testing.T) {
	log := logger.Default()
	db := testutil.SetupTestSQLiteDB(t)
	ctx := context.Background()

	q := queries.New(db.DB())
	testutil.ApplyMigrations(t, ctx, db.DB(), "../../../data/db/dev/migrations")

	handler := NewHandler(log, db)

	// Create tenant and section
	tenantID := uuid.Must(uuid.NewV7()).String()
	_, err := db.DB().ExecContext(ctx, `INSERT INTO tenants (id, name) VALUES (?, ?)`,
		tenantID, "Test Tenant")
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	workspaceID := uuid.Must(uuid.NewV7()).String()
	_, err = db.DB().ExecContext(ctx, `INSERT INTO workspaces (id, tenant_id, name) VALUES (?, ?, ?)`,
		workspaceID, tenantID, "Default Section")
	if err != nil {
		t.Fatalf("Failed to create section: %v", err)
	}

	_, plainToken := setupUserAndSession(t, db, workspaceID)
	router := setupTestRouterWithAuth(db, handler)

	// Create unclaimed agent
	agentID := uuid.Must(uuid.NewV7())
	claimToken := "test-claim-token-12345678"
	claimTokenHash := sha256.Sum256([]byte(claimToken))
	claimTokenHashStr := hex.EncodeToString(claimTokenHash[:])
	expiresAt := time.Now().Add(24 * time.Hour)

	_, err = q.UpsertAgentClaim(ctx, queries.UpsertAgentClaimParams{
		ID:                  agentID.String(),
		ClaimTokenHash:      claimTokenHashStr,
		Hostname:            "production-server-01",
		AgentVersion:        "1.0.0",
		ClaimTokenExpiresAt: expiresAt,
	})
	if err != nil {
		t.Fatalf("Failed to create agent claim: %v", err)
	}

	// Claim without providing name
	reqBody := api.ClaimAgentRequest{
		AgentId:    agentID,
		ClaimToken: claimToken,
		Name:       nil, // No name provided
	}

	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/agent/claim", bytes.NewReader(reqJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+plainToken)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Verify agent was created with hostname as name
	// Verify claim was marked as claimed
	claim, err := q.GetAgentClaim(ctx, agentID.String())
	if err != nil {
		t.Fatalf("Failed to get agent claim: %v", err)
	}

	if !claim.ClaimedAt.Valid {
		t.Error("Expected claim to be marked as claimed")
	}

	if claim.Hostname != "production-server-01" {
		t.Errorf("Expected hostname 'production-server-01', got '%s'", claim.Hostname)
	}
}

func ptrString(s string) *string {
	return &s
}

// setupClaimFixture creates a tenant, workspace, and unclaimed agent claim for reuse across tests.
func setupClaimFixture(t *testing.T, db database.Database) (tenantID, workspaceID string, agentID uuid.UUID, claimToken string) {
	t.Helper()
	ctx := context.Background()
	q := queries.New(db.DB())

	tenantID = uuid.Must(uuid.NewV7()).String()
	if _, err := db.DB().ExecContext(ctx, `INSERT INTO tenants (id, name) VALUES (?, ?)`, tenantID, "Fixture Tenant"); err != nil {
		t.Fatalf("fixture tenant: %v", err)
	}
	workspaceID = uuid.Must(uuid.NewV7()).String()
	if _, err := db.DB().ExecContext(ctx, `INSERT INTO workspaces (id, tenant_id, name) VALUES (?, ?, 'default')`, workspaceID, tenantID); err != nil {
		t.Fatalf("fixture workspace: %v", err)
	}
	agentID = uuid.Must(uuid.NewV7())
	claimToken = "test-claim-token-12345678"
	tokenHash := sha256.Sum256([]byte(claimToken))
	if _, err := q.UpsertAgentClaim(ctx, queries.UpsertAgentClaimParams{
		ID:                  agentID.String(),
		ClaimTokenHash:      hex.EncodeToString(tokenHash[:]),
		Hostname:            "fixture-host",
		AgentVersion:        "1.0.0",
		ClaimTokenExpiresAt: time.Now().Add(24 * time.Hour),
	}); err != nil {
		t.Fatalf("fixture claim: %v", err)
	}
	return
}

func TestClaimAgent_Integration_AgentClaimedIntoUserWorkspace(t *testing.T) {
	log := logger.Default()
	db := testutil.SetupTestSQLiteDB(t)
	ctx := context.Background()
	testutil.ApplyMigrations(t, ctx, db.DB(), "../../../data/db/dev/migrations")

	_, _, agentID, claimToken := setupClaimFixture(t, db)

	// Tenant B with its own workspace — the user below belongs here.
	tenantB := uuid.Must(uuid.NewV7()).String()
	if _, err := db.DB().ExecContext(ctx, `INSERT INTO tenants (id, name) VALUES (?, ?)`, tenantB, "Other Tenant"); err != nil {
		t.Fatalf("tenant B: %v", err)
	}
	workspaceB := uuid.Must(uuid.NewV7()).String()
	if _, err := db.DB().ExecContext(ctx, `INSERT INTO workspaces (id, tenant_id, name) VALUES (?, ?, 'default')`, workspaceB, tenantB); err != nil {
		t.Fatalf("workspace B: %v", err)
	}
	_, plainToken := setupUserAndSession(t, db, workspaceB)

	handler := NewHandler(log, db)
	router := setupTestRouterWithAuth(db, handler)

	// User B (tenant B) claims the agent; the server uses user B's workspace.
	reqBody := api.ClaimAgentRequest{
		AgentId:    agentID,
		ClaimToken: claimToken,
	}
	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/agent/claim", bytes.NewReader(reqJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+plainToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Verify the agent was created in user B's workspace.
	var agentWorkspaceID string
	err := db.DB().QueryRowContext(ctx, "SELECT workspace_id FROM agents WHERE id = ?", agentID.String()).Scan(&agentWorkspaceID)
	if err != nil {
		t.Fatalf("query agent workspace: %v", err)
	}
	if agentWorkspaceID != workspaceB {
		t.Errorf("expected agent workspace %q (user's), got %q", workspaceB, agentWorkspaceID)
	}
}

// TestClaimAgent_Integration_RequestBodyWorkspaceIdIgnored verifies that the agent
// is always claimed into the authenticated user's workspace.
func TestClaimAgent_Integration_RequestBodyWorkspaceIdIgnored(t *testing.T) {
	log := logger.Default()
	db := testutil.SetupTestSQLiteDB(t)
	ctx := context.Background()
	testutil.ApplyMigrations(t, ctx, db.DB(), "../../../data/db/dev/migrations")

	_, _, agentID, claimToken := setupClaimFixture(t, db)

	// Set up an authenticated user in their own workspace.
	tenantID := uuid.Must(uuid.NewV7()).String()
	if _, err := db.DB().ExecContext(ctx, `INSERT INTO tenants (id, name) VALUES (?, ?)`, tenantID, "Test Tenant 2"); err != nil {
		t.Fatalf("tenant: %v", err)
	}
	wsID := uuid.Must(uuid.NewV7()).String()
	if _, err := db.DB().ExecContext(ctx, `INSERT INTO workspaces (id, tenant_id, name) VALUES (?, ?, 'default')`, wsID, tenantID); err != nil {
		t.Fatalf("workspace: %v", err)
	}
	_, plainToken := setupUserAndSession(t, db, wsID)

	handler := NewHandler(log, db)
	router := setupTestRouterWithAuth(db, handler)

	reqBody := api.ClaimAgentRequest{
		AgentId:    agentID,
		ClaimToken: claimToken,
	}
	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/agent/claim", bytes.NewReader(reqJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+plainToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Verify the agent is in the user's workspace.
	var agentWorkspaceID string
	err := db.DB().QueryRowContext(ctx, "SELECT workspace_id FROM agents WHERE id = ?", agentID.String()).Scan(&agentWorkspaceID)
	if err != nil {
		t.Fatalf("query agent workspace: %v", err)
	}
	if agentWorkspaceID != wsID {
		t.Errorf("expected agent workspace %q (user's), got %q", wsID, agentWorkspaceID)
	}
}

func TestClaimAgent_Integration_OAuthUserSetsClaimedBy(t *testing.T) {
	log := logger.Default()
	db := testutil.SetupTestSQLiteDB(t)
	ctx := context.Background()
	testutil.ApplyMigrations(t, ctx, db.DB(), "../../../data/db/dev/migrations")

	tenantID, workspaceID, agentID, claimToken := setupClaimFixture(t, db)

	// Create a user in the same tenant/workspace.
	userID := uuid.Must(uuid.NewV7()).String()
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO users (id, workspace_id, oauth_provider, oauth_subject, display_name) VALUES (?, ?, 'github', 'sub-owner', 'Owner')`,
		userID, workspaceID); err != nil {
		t.Fatalf("user: %v", err)
	}
	_ = tenantID

	plainToken := "oauth-claim-test-token-abc"
	now := time.Now().UTC()
	q := queries.New(db.DB())
	if _, err := q.CreateSession(ctx, queries.CreateSessionParams{
		ID:                uuid.Must(uuid.NewV7()).String(),
		UserID:            userID,
		TokenHash:         middleware.HashAPIKeyForTests(plainToken),
		SlidingExpiresAt:  now.Add(24 * time.Hour),
		ExpiresAt:         now.Add(24 * time.Hour),
		Oauth2Provider:    "github",
		Oauth2AccessToken: "tok",
		Oauth2TokenType:   "Bearer",
	}); err != nil {
		t.Fatalf("session: %v", err)
	}

	handler := NewHandler(log, db)
	router := setupTestRouterWithAuth(db, handler)

	reqBody := api.ClaimAgentRequest{
		AgentId:    agentID,
		ClaimToken: claimToken,
		Name:       ptrString("OAuth Test Agent"),
	}
	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/agent/claim", bytes.NewReader(reqJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+plainToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	claimFromDB, err := q.GetAgentClaim(ctx, agentID.String())
	if err != nil {
		t.Fatalf("get claim: %v", err)
	}
	if !claimFromDB.ClaimedByUserID.Valid || claimFromDB.ClaimedByUserID.String != userID {
		t.Errorf("expected claimed_by_user_id = %q, got %v", userID, claimFromDB.ClaimedByUserID)
	}
	if !claimFromDB.ApiKeyPlaintext.Valid || claimFromDB.ApiKeyPlaintext.String == "" {
		t.Error("expected api_key_plaintext to be set")
	}
}
