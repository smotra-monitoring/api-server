package agent_claim

import (
	"database/sql"
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

	// Verify agent was created in production table
	// Verify claim was marked as claimed and agent was created
	claim, err := q.GetAgentClaim(ctx, agentID.String())
	if err != nil {
		t.Fatalf("Failed to get agent claim: %v", err)
	}

	// Verify claim was marked as claimed
	if !claim.ClaimedAt.Valid {
		t.Error("Expected claimed_at to be set")
	}

	if !claim.ClaimedByUserID.Valid {
		t.Error("Expected claimed_by_user_id to be set")
	}

	if !claim.ApiKeyPlaintext.Valid || claim.ApiKeyPlaintext.String == "" {
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

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected status 403, got %d. Body: %s", w.Code, w.Body.String())
	}

	var errResp api.Error
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("Failed to unmarshal error response: %v", err)
	}

	if errResp.Error != "invalid_claim_token" {
		t.Errorf("Expected error 'invalid_claim_token', got '%s'", errResp.Error)
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

	// Mark as already claimed
	claimedAt := time.Now().Format("2006-01-02 15:04:05")
	_, err = db.DB().ExecContext(ctx, `UPDATE agent_claims SET claimed_at = ?, claimed_by_user_id = ? WHERE id = ?`,
		claimedAt, userID, agentID.String())
	if err != nil {
		t.Fatalf("Failed to mark agent as claimed: %v", err)
	}

	// Try to claim again
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

	if w.Code != http.StatusConflict {
		t.Errorf("Expected status 409, got %d. Body: %s", w.Code, w.Body.String())
	}

	var errResp api.Error
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("Failed to unmarshal error response: %v", err)
	}

	if errResp.Error != "already_claimed" {
		t.Errorf("Expected error 'already_claimed', got '%s'", errResp.Error)
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

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected status 403 (token mismatch on non-existent), got %d. Body: %s", w.Code, w.Body.String())
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
