package agent_claim_status

import (
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
	api "github.com/smotra-monitoring/server/internal/api/v1"
	"github.com/smotra-monitoring/server/internal/database/queries"
	"github.com/smotra-monitoring/server/internal/logger"
	"github.com/smotra-monitoring/server/internal/testutil"
)

// testServerImpl wraps Handler and implements the full StrictServerInterface
type testServerImpl struct {
	*Handler
}

// GetAgentClaimStatus delegates to this handler
func (t *testServerImpl) GetAgentClaimStatus(ctx context.Context, request api.GetAgentClaimStatusRequestObject) (api.GetAgentClaimStatusResponseObject, error) {
	return t.Handle(ctx, request)
}

func (t *testServerImpl) GetAgentConfiguration(ctx context.Context, request api.GetAgentConfigurationRequestObject) (api.GetAgentConfigurationResponseObject, error) {
	return nil, nil
}

func (t *testServerImpl) RegisterAgentSelf(ctx context.Context, request api.RegisterAgentSelfRequestObject) (api.RegisterAgentSelfResponseObject, error) {
	return nil, nil
}

func (t *testServerImpl) PostClaimAgent(ctx context.Context, request api.PostClaimAgentRequestObject) (api.PostClaimAgentResponseObject, error) {
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

func TestGetAgentClaimStatus_Integration_NotFound(t *testing.T) {
	log := logger.Default()
	db := testutil.SetupTestSQLiteDB(t)
	ctx := context.Background()

	applySchema(t, ctx, db.DB())

	handler := NewHandler(log, db)
	router := setupTestRouter(handler)

	agentID := uuid.Must(uuid.NewV7())
	req := httptest.NewRequest(http.MethodGet, "/agent/"+agentID.String()+"/claim-status", nil)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestGetAgentClaimStatus_Integration_PendingClaim(t *testing.T) {
	log := logger.Default()
	db := testutil.SetupTestSQLiteDB(t)
	ctx := context.Background()

	q := queries.New(db.DB())
	applySchema(t, ctx, db.DB())

	handler := NewHandler(log, db)
	router := setupTestRouter(handler)

	// Create unclaimed agent
	agentID := uuid.Must(uuid.NewV7())
	claimToken := "test-claim-token-12345678"
	claimTokenHash := sha256.Sum256([]byte(claimToken))
	claimTokenHashStr := hex.EncodeToString(claimTokenHash[:])
	expiresAt := time.Now().Add(24 * time.Hour)

	_, err := q.UpsertAgentClaim(ctx, queries.UpsertAgentClaimParams{
		ID:                  agentID.String(),
		ClaimTokenHash:      claimTokenHashStr,
		Hostname:            "test-host",
		AgentVersion:        "1.0.0",
		ClaimTokenExpiresAt: expiresAt,
	})
	if err != nil {
		t.Fatalf("Failed to create agent claim: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/agent/"+agentID.String()+"/claim-status", nil)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if status, ok := response["status"].(string); !ok || status != "pending_claim" {
		t.Errorf("Expected status 'pending_claim', got '%v'", response["status"])
	}

	// Should not have api_key or config_url
	if _, ok := response["api_key"]; ok {
		t.Error("Expected no api_key in pending response")
	}
}

func TestGetAgentClaimStatus_Integration_AlreadyDelivered1(t *testing.T) {
	log := logger.Default()
	db := testutil.SetupTestSQLiteDB(t)
	ctx := context.Background()

	q := queries.New(db.DB())
	applySchema(t, ctx, db.DB())

	handler := NewHandler(log, db)
	router := setupTestRouter(handler)

	// Create tenant and user
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

	// Create claimed agent with API key ready for delivery
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

	// Mark as claimed with API key ready
	testAPIKey := "sk_live_test123456789abcdef"
	err = q.MarkAgentClaimClaimed(ctx, queries.MarkAgentClaimClaimedParams{
		ClaimedByUserID: sql.NullString{String: userID, Valid: true},
		ApiKeyPlaintext: sql.NullString{String: testAPIKey, Valid: true},
		ID:              agentID.String(),
	})
	if err != nil {
		t.Fatalf("Failed to mark agent as claimed: %v", err)
	}

	// First poll - should deliver API key
	req1 := httptest.NewRequest(http.MethodGet, "/agent/"+agentID.String()+"/claim-status", nil)
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w1.Code, w1.Body.String())
	}

	var response1 api.ClaimStatusClaimed
	if err := json.Unmarshal(w1.Body.Bytes(), &response1); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response1.Status != "claimed" {
		t.Errorf("Expected status 'claimed', got '%v'", response1.Status)
	}

	if response1.ApiKey == "" || response1.ApiKey != testAPIKey {
		t.Errorf("Expected api_key '%s', got '%v'", testAPIKey, response1.ApiKey)
	}

	// Second poll - API key should be cleared (already delivered)
	req2 := httptest.NewRequest(http.MethodGet, "/agent/"+agentID.String()+"/claim-status", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w2.Code, w2.Body.String())
	}

	var response2 api.ClaimStatusPending
	if err := json.Unmarshal(w2.Body.Bytes(), &response2); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	// Should return pending_claim after delivery
	if response2.Status != "pending_claim" {
		t.Errorf("Expected status 'pending_claim' after delivery, got '%v'", response2.Status)
	}

	// Verify api_key_delivered flag is set
	claim, err := q.GetAgentClaim(ctx, agentID.String())
	if err != nil {
		t.Fatalf("Failed to get agent claim: %v", err)
	}

	if claim.ApiKeyDelivered == 0 {
		t.Error("Expected api_key_delivered to be set to 1")
	}

	if claim.ApiKeyPlaintext.Valid && claim.ApiKeyPlaintext.String != "" {
		t.Errorf("Expected api_key_plaintext to be cleared, got '%s'", claim.ApiKeyPlaintext.String)
	}
}

func TestGetAgentClaimStatus_Integration_AlreadyDelivered2(t *testing.T) {
	log := logger.Default()
	db := testutil.SetupTestSQLiteDB(t)
	ctx := context.Background()

	q := queries.New(db.DB())
	applySchema(t, ctx, db.DB())

	handler := NewHandler(log, db)
	router := setupTestRouter(handler)

	// Create tenant and user
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

	// Create agent claim that's already been delivered
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

	// Mark as claimed, but not yet delivered
	err = q.MarkAgentClaimClaimed(ctx, queries.MarkAgentClaimClaimedParams{
		ClaimedByUserID: sql.NullString{String: userID, Valid: true},
		ApiKeyPlaintext: sql.NullString{String: "sk_live_test123456789abcdef", Valid: true},
		ID:              agentID.String(),
	})
	if err != nil {
		t.Fatalf("Failed to mark agent as claimed: %v", err)
	}

	// 1-st query: Trigger API key delivery
	req1 := httptest.NewRequest(http.MethodGet, "/agent/"+agentID.String()+"/claim-status", nil)
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w1.Code, w1.Body.String())
	}

	var response1 api.ClaimStatusClaimed
	if err := json.Unmarshal(w1.Body.Bytes(), &response1); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response1.Status != "claimed" {
		t.Errorf("Expected status 'claimed' for claimed agent claim, got '%v'", response1.Status)
	}

	// 2-nd query: Repeat API key delivery, should return pending_claim, due to securty reasons
	//             to avoid attacks that try to repeatedly fetch API keys
	req2 := httptest.NewRequest(http.MethodGet, "/agent/"+agentID.String()+"/claim-status", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w2.Code, w2.Body.String())
	}

	var response2 api.ClaimStatusPending
	if err := json.Unmarshal(w2.Body.Bytes(), &response2); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response2.Status != "pending_claim" {
		t.Errorf("Expected status 'pending_claim' for already delivered, got '%v'", response2.Status)
	}

}
