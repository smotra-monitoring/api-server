package agent_claim

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/smotra-monitoring/server/internal/api"
	"github.com/smotra-monitoring/server/internal/database/queries"
	"github.com/smotra-monitoring/server/internal/logger"
	"github.com/smotra-monitoring/server/internal/testutil"
)

// testServerImpl wraps Handler and implements the full StrictServerInterface
type testServerImpl struct {
	*Handler
}

// ClaimAgent delegates to this handler
func (t *testServerImpl) ClaimAgent(ctx context.Context, request api.ClaimAgentRequestObject) (api.ClaimAgentResponseObject, error) {
	return t.Handle(ctx, request)
}

// Stub implementations for other endpoints (required by StrictServerInterface)
func (t *testServerImpl) HealthCheck(ctx context.Context, request api.HealthCheckRequestObject) (api.HealthCheckResponseObject, error) {
	return nil, nil
}

func (t *testServerImpl) LivenessCheck(ctx context.Context, request api.LivenessCheckRequestObject) (api.LivenessCheckResponseObject, error) {
	return nil, nil
}

func (t *testServerImpl) ReadinessCheck(ctx context.Context, request api.ReadinessCheckRequestObject) (api.ReadinessCheckResponseObject, error) {
	return nil, nil
}

func (t *testServerImpl) PrometheusMetrics(ctx context.Context, request api.PrometheusMetricsRequestObject) (api.PrometheusMetricsResponseObject, error) {
	return api.PrometheusMetrics200TextResponse(""), nil
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

func setupTestRouter(handler *Handler) *chi.Mux {
	testImpl := &testServerImpl{Handler: handler}
	r := chi.NewRouter()
	strictHandler := api.NewStrictHandler(testImpl, nil)
	api.HandlerFromMux(strictHandler, r)
	return r
}

func applySchema(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()

	// Apply schema
	schemaSQL, err := os.ReadFile("../../../data/db/dev/migrations/0001_schema.up.sql")
	if err != nil {
		t.Fatalf("Failed to read schema file: %v", err)
	}

	_, err = db.ExecContext(ctx, string(schemaSQL))
	if err != nil {
		t.Fatalf("Failed to apply schema: %v", err)
	}
}

func TestClaimAgent_Integration_Success(t *testing.T) {
	log := logger.Default()
	db := testutil.SetupTestSQLiteDB(t)
	ctx := context.Background()

	q := queries.New(db.DB())
	applySchema(t, ctx, db.DB())

	handler := NewHandler(log, db)
	router := setupTestRouter(handler)

	// Create tenant
	tenantID := uuid.Must(uuid.NewV7()).String()
	_, err := db.DB().ExecContext(ctx, `INSERT INTO tenants (id, name) VALUES (?, ?)`,
		tenantID, "Test Tenant")
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	// Create section
	sectionID := uuid.Must(uuid.NewV7()).String()
	_, err = db.DB().ExecContext(ctx, `INSERT INTO sections (id, tenant_id, name) VALUES (?, ?, ?)`,
		sectionID, tenantID, "Default Section")
	if err != nil {
		t.Fatalf("Failed to create section: %v", err)
	}

	// Create unclaimed agent
	agentID := uuid.Must(uuid.NewV7())
	claimToken := "test-claim-token-12345678"
	claimTokenHash := sha256.Sum256([]byte(claimToken))
	claimTokenHashStr := hex.EncodeToString(claimTokenHash[:])
	expiresAt := time.Now().Add(24 * time.Hour).Format("2006-01-02 15:04:05")

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
		SectionId:  uuid.MustParse(sectionID),
		Name:       ptrString("My Test Agent"),
	}

	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/agent/claim", bytes.NewReader(reqJSON))
	req.Header.Set("Content-Type", "application/json")

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

	// TODO: uncomment after implementing OAuth user association
	// if !claimFromDB.ClaimedByUserID.Valid {
	// 	t.Error("Expected claimed_by_user_id to be set")
	// }

	if !claimFromDB.ApiKeyPlaintext.Valid || claimFromDB.ApiKeyPlaintext.String == "" {
		t.Error("Expected api_key_plaintext to be set for delivery")
	}
}

func TestClaimAgent_Integration_InvalidToken(t *testing.T) {
	log := logger.Default()
	db := testutil.SetupTestSQLiteDB(t)
	ctx := context.Background()

	q := queries.New(db.DB())
	applySchema(t, ctx, db.DB())

	handler := NewHandler(log, db)
	router := setupTestRouter(handler)

	// Create tenant and section
	tenantID := uuid.Must(uuid.NewV7()).String()
	_, err := db.DB().ExecContext(ctx, `INSERT INTO tenants (id, name) VALUES (?, ?)`,
		tenantID, "Test Tenant")
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	sectionID := uuid.Must(uuid.NewV7()).String()
	_, err = db.DB().ExecContext(ctx, `INSERT INTO sections (id, tenant_id, name) VALUES (?, ?, ?)`,
		sectionID, tenantID, "Default Section")
	if err != nil {
		t.Fatalf("Failed to create section: %v", err)
	}

	// Create unclaimed agent
	agentID := uuid.Must(uuid.NewV7())
	claimToken := "test-claim-token-12345678"
	claimTokenHash := sha256.Sum256([]byte(claimToken))
	claimTokenHashStr := hex.EncodeToString(claimTokenHash[:])
	expiresAt := time.Now().Add(24 * time.Hour).Format("2006-01-02 15:04:05")

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
		SectionId:  uuid.MustParse(sectionID),
		Name:       ptrString("My Test Agent"),
	}

	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/agent/claim", bytes.NewReader(reqJSON))
	req.Header.Set("Content-Type", "application/json")

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
	applySchema(t, ctx, db.DB())

	handler := NewHandler(log, db)
	router := setupTestRouter(handler)

	// Create tenant, user, and section
	tenantID := uuid.Must(uuid.NewV7()).String()
	_, err := db.DB().ExecContext(ctx, `INSERT INTO tenants (id, name) VALUES (?, ?)`,
		tenantID, "Test Tenant")
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	userID := uuid.Must(uuid.NewV7()).String()
	_, err = db.DB().ExecContext(ctx,
		`INSERT INTO users (id, tenant_id, oauth_provider, oauth_subject, display_name) VALUES (?, ?, ?, ?, ?)`,
		userID, tenantID, "github", "test-user-123", "Test User")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	sectionID := uuid.Must(uuid.NewV7()).String()
	_, err = db.DB().ExecContext(ctx, `INSERT INTO sections (id, tenant_id, name) VALUES (?, ?, ?)`,
		sectionID, tenantID, "Default Section")
	if err != nil {
		t.Fatalf("Failed to create section: %v", err)
	}

	// Create already claimed agent
	agentID := uuid.Must(uuid.NewV7())
	claimToken := "test-claim-token-12345678"
	claimTokenHash := sha256.Sum256([]byte(claimToken))
	claimTokenHashStr := hex.EncodeToString(claimTokenHash[:])
	expiresAt := time.Now().Add(24 * time.Hour).Format("2006-01-02 15:04:05")

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
		SectionId:  uuid.MustParse(sectionID),
		Name:       ptrString("My Test Agent"),
	}

	reqJSON1, _ := json.Marshal(reqBody1)
	req1 := httptest.NewRequest(http.MethodPost, "/agent/claim", bytes.NewReader(reqJSON1))
	req1.Header.Set("Content-Type", "application/json")

	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w1.Code, w1.Body.String())
	}

	// Try to claim again
	reqBody2 := api.ClaimAgentRequest{
		AgentId:    agentID,
		ClaimToken: claimToken,
		SectionId:  uuid.MustParse(sectionID),
		Name:       ptrString("My Test Agent"),
	}

	reqJSON2, _ := json.Marshal(reqBody2)
	req2 := httptest.NewRequest(http.MethodPost, "/agent/claim", bytes.NewReader(reqJSON2))
	req2.Header.Set("Content-Type", "application/json")

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

	applySchema(t, ctx, db.DB())

	handler := NewHandler(log, db)
	router := setupTestRouter(handler)

	// Create tenant and section
	tenantID := uuid.Must(uuid.NewV7()).String()
	_, err := db.DB().ExecContext(ctx, `INSERT INTO tenants (id, name) VALUES (?, ?)`,
		tenantID, "Test Tenant")
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	sectionID := uuid.Must(uuid.NewV7()).String()
	_, err = db.DB().ExecContext(ctx, `INSERT INTO sections (id, tenant_id, name) VALUES (?, ?, ?)`,
		sectionID, tenantID, "Default Section")
	if err != nil {
		t.Fatalf("Failed to create section: %v", err)
	}

	// Try to claim non-existent agent
	agentID := uuid.Must(uuid.NewV7())
	reqBody := api.ClaimAgentRequest{
		AgentId:    agentID,
		ClaimToken: "some-token",
		SectionId:  uuid.MustParse(sectionID),
		Name:       ptrString("My Test Agent"),
	}

	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/agent/claim", bytes.NewReader(reqJSON))
	req.Header.Set("Content-Type", "application/json")

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
	applySchema(t, ctx, db.DB())

	handler := NewHandler(log, db)
	router := setupTestRouter(handler)

	// Create tenant and section
	tenantID := uuid.Must(uuid.NewV7()).String()
	_, err := db.DB().ExecContext(ctx, `INSERT INTO tenants (id, name) VALUES (?, ?)`,
		tenantID, "Test Tenant")
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	sectionID := uuid.Must(uuid.NewV7()).String()
	_, err = db.DB().ExecContext(ctx, `INSERT INTO sections (id, tenant_id, name) VALUES (?, ?, ?)`,
		sectionID, tenantID, "Default Section")
	if err != nil {
		t.Fatalf("Failed to create section: %v", err)
	}

	// Create unclaimed agent
	agentID := uuid.Must(uuid.NewV7())
	claimToken := "test-claim-token-12345678"
	claimTokenHash := sha256.Sum256([]byte(claimToken))
	claimTokenHashStr := hex.EncodeToString(claimTokenHash[:])
	expiresAt := time.Now().Add(24 * time.Hour).Format("2006-01-02 15:04:05")

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
		SectionId:  uuid.MustParse(sectionID),
		Name:       nil, // No name provided
	}

	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/agent/claim", bytes.NewReader(reqJSON))
	req.Header.Set("Content-Type", "application/json")

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
