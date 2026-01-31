package agent_claim_status

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

// GetAgentClaimStatus delegates to this handler
func (t *testServerImpl) GetAgentClaimStatus(ctx context.Context, request api.GetAgentClaimStatusRequestObject) (api.GetAgentClaimStatusResponseObject, error) {
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

func (t *testServerImpl) ClaimAgent(ctx context.Context, request api.ClaimAgentRequestObject) (api.ClaimAgentResponseObject, error) {
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

	schema := `
	PRAGMA foreign_keys = ON;

	CREATE TABLE tenants (
		id           TEXT PRIMARY KEY,
		name         TEXT NOT NULL,
		created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%S', 'now'))
	) STRICT, WITHOUT ROWID;

	CREATE TABLE users (
		id              TEXT PRIMARY KEY,
		tenant_id       TEXT NOT NULL,
		oauth_provider  TEXT NOT NULL,
		oauth_subject   TEXT NOT NULL,
		display_name    TEXT NOT NULL,
		last_login_at   TEXT,
		created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%S', 'now')),
		updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%S', 'now')),
		UNIQUE(oauth_provider, oauth_subject),
		FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
	) STRICT, WITHOUT ROWID;

	CREATE TABLE sections (
		id           TEXT PRIMARY KEY,
		tenant_id    TEXT NOT NULL,
		name         TEXT NOT NULL,
		UNIQUE(tenant_id, name),
		FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
	) STRICT, WITHOUT ROWID;

	CREATE TABLE agent_claims (
		id                      TEXT PRIMARY KEY,
		claim_token_hash        TEXT NOT NULL,
		hostname                TEXT NOT NULL,
		agent_version           TEXT NOT NULL,
		claim_token_expires_at  TEXT NOT NULL,
		last_seen_at            TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%S', 'now')),
		created_at              TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%S', 'now')),
		claimed_at              TEXT,
		claimed_by_user_id      TEXT,
		api_key_plaintext       TEXT,
		api_key_delivered       INT NOT NULL DEFAULT 0,
		FOREIGN KEY (claimed_by_user_id) REFERENCES users(id) ON DELETE SET NULL
	) STRICT, WITHOUT ROWID;

	CREATE TABLE agents (
		id             TEXT PRIMARY KEY,
		section_id     TEXT NOT NULL,
		name           TEXT NOT NULL,
		api_key_hash   TEXT NOT NULL,
		base_config    TEXT NOT NULL DEFAULT '{}',
		version        INT NOT NULL DEFAULT 1,
		agent_version  TEXT,
		last_seen_at   TEXT,
		updated_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%S', 'now')),
		created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%S', 'now')),
		FOREIGN KEY (section_id) REFERENCES sections(id) ON DELETE CASCADE
	) STRICT, WITHOUT ROWID;
	`

	_, err := db.ExecContext(ctx, schema)
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
	expiresAt := time.Now().Add(24 * time.Hour).Format("2006-01-02 15:04:05")

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
	if _, hasKey := response["api_key"]; hasKey {
		t.Error("Expected no api_key in pending response")
	}
}

func TestGetAgentClaimStatus_Integration_ClaimedWithAPIKeyDelivery(t *testing.T) {
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

	var response1 map[string]interface{}
	if err := json.Unmarshal(w1.Body.Bytes(), &response1); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if status, ok := response1["status"].(string); !ok || status != "claimed" {
		t.Errorf("Expected status 'claimed', got '%v'", response1["status"])
	}

	if apiKey, ok := response1["api_key"].(string); !ok || apiKey != testAPIKey {
		t.Errorf("Expected api_key '%s', got '%v'", testAPIKey, response1["api_key"])
	}

	// Second poll - API key should be cleared (already delivered)
	req2 := httptest.NewRequest(http.MethodGet, "/agent/"+agentID.String()+"/claim-status", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w2.Code, w2.Body.String())
	}

	var response2 map[string]interface{}
	if err := json.Unmarshal(w2.Body.Bytes(), &response2); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	// Should return pending_claim after delivery
	if status, ok := response2["status"].(string); !ok || status != "pending_claim" {
		t.Errorf("Expected status 'pending_claim' after delivery, got '%v'", response2["status"])
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

func TestGetAgentClaimStatus_Integration_AlreadyDelivered(t *testing.T) {
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

	// Mark as claimed and delivered
	claimedAt := time.Now().Format("2006-01-02 15:04:05")
	_, err = db.DB().ExecContext(ctx, `UPDATE agent_claims SET claimed_at = ?, claimed_by_user_id = ?, api_key_delivered = 1 WHERE id = ?`,
		claimedAt, userID, agentID.String())
	if err != nil {
		t.Fatalf("Failed to mark agent as delivered: %v", err)
	}

	// Poll should return pending (agent should stop polling)
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
		t.Errorf("Expected status 'pending_claim' for already delivered, got '%v'", response["status"])
	}
}
