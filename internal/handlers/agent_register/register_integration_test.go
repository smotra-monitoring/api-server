package agent_register

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

// RegisterAgentSelf delegates to this handler
func (t *testServerImpl) RegisterAgentSelf(ctx context.Context, request api.RegisterAgentSelfRequestObject) (api.RegisterAgentSelfResponseObject, error) {
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

func (t *testServerImpl) GetAgentClaimStatus(ctx context.Context, request api.GetAgentClaimStatusRequestObject) (api.GetAgentClaimStatusResponseObject, error) {
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

func TestRegisterAgentSelf_Integration_Success(t *testing.T) {
	log := logger.Default()
	db := testutil.SetupTestSQLiteDB(t)
	ctx := context.Background()

	q := queries.New(db.DB())
	applySchema(t, ctx, db.DB())

	handler := NewHandler(log, db)
	router := setupTestRouter(handler)

	// Generate test data
	agentID := uuid.Must(uuid.NewV7())
	claimToken := "test-claim-token-12345678"
	claimTokenHash := sha256.Sum256([]byte(claimToken))
	claimTokenHashStr := hex.EncodeToString(claimTokenHash[:])

	reqBody := api.AgentSelfRegistration{
		AgentId:        agentID,
		ClaimTokenHash: claimTokenHashStr,
		Hostname:       "test-agent-host",
		AgentVersion:   "1.0.0",
	}

	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/agent/register", bytes.NewReader(reqJSON))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Verify response
	var response api.AgentRegistrationResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.Status != "pending_claim" {
		t.Errorf("Expected status 'pending_claim', got '%s'", response.Status)
	}

	if response.PollUrl == "" {
		t.Error("Expected non-empty poll URL")
	}

	// Verify database entry
	claim, err := q.GetAgentClaim(ctx, agentID.String())
	if err != nil {
		t.Fatalf("Failed to get agent claim from database: %v", err)
	}

	if claim.Hostname != "test-agent-host" {
		t.Errorf("Expected hostname 'test-agent-host', got '%s'", claim.Hostname)
	}

	if claim.AgentVersion != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", claim.AgentVersion)
	}
}

func TestRegisterAgentSelf_Integration_Idempotent(t *testing.T) {
	log := logger.Default()
	db := testutil.SetupTestSQLiteDB(t)
	ctx := context.Background()

	q := queries.New(db.DB())
	applySchema(t, ctx, db.DB())

	handler := NewHandler(log, db)
	router := setupTestRouter(handler)

	agentID := uuid.Must(uuid.NewV7())
	claimToken := "test-claim-token-12345678"
	claimTokenHash := sha256.Sum256([]byte(claimToken))
	claimTokenHashStr := hex.EncodeToString(claimTokenHash[:])

	reqBody := api.AgentSelfRegistration{
		AgentId:        agentID,
		ClaimTokenHash: claimTokenHashStr,
		Hostname:       "test-agent-host",
		AgentVersion:   "1.0.0",
	}

	// First registration
	reqJSON, _ := json.Marshal(reqBody)
	req1 := httptest.NewRequest(http.MethodPost, "/agent/register", bytes.NewReader(reqJSON))
	req1.Header.Set("Content-Type", "application/json")

	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)

	if w1.Code != http.StatusCreated {
		t.Fatalf("First registration failed with status %d", w1.Code)
	}

	// Second registration (idempotent)
	reqJSON2, _ := json.Marshal(reqBody)
	req2 := httptest.NewRequest(http.MethodPost, "/agent/register", bytes.NewReader(reqJSON2))
	req2.Header.Set("Content-Type", "application/json")

	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("Expected idempotent status 200, got %d. Body: %s", w2.Code, w2.Body.String())
	}

	// Verify only one entry exists
	claim, err := q.GetAgentClaim(ctx, agentID.String())
	if err != nil {
		t.Fatalf("Failed to get agent claim: %v", err)
	}

	if claim.Hostname != "test-agent-host" {
		t.Error("Agent claim data changed on idempotent request")
	}
}

func TestRegisterAgentSelf_Integration_AlreadyClaimed(t *testing.T) {
	log := logger.Default()
	db := testutil.SetupTestSQLiteDB(t)
	ctx := context.Background()

	q := queries.New(db.DB())
	applySchema(t, ctx, db.DB())

	handler := NewHandler(log, db)
	router := setupTestRouter(handler)

	// Create a tenant and user for claiming using raw SQL
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

	// Create an already claimed agent
	agentID := uuid.Must(uuid.NewV7())
	claimToken := "test-claim-token-12345678"
	claimTokenHash := sha256.Sum256([]byte(claimToken))
	claimTokenHashStr := hex.EncodeToString(claimTokenHash[:])
	// expiresAt := time.Now().Add(24 * time.Hour).Format("2006-01-02 15:04:05")

	// Register the agent
	reqBody := api.AgentSelfRegistration{
		AgentId:        agentID,
		ClaimTokenHash: claimTokenHashStr,
		Hostname:       "test-agent-host",
		AgentVersion:   "1.0.0",
	}
	{
		reqJSON, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/agent/register", bytes.NewReader(reqJSON))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("Expected status 201 for successful registration, got %d. Body: %s", w.Code, w.Body.String())
		}
	}

	// Mark it as claimed using raw SQL
	err = q.MarkAgentClaimClaimed(ctx, queries.MarkAgentClaimClaimedParams{
		ClaimedByUserID: sql.NullString{String: userID, Valid: true},
		ApiKeyPlaintext: sql.NullString{String: "dummy-api-key", Valid: true},
		ID:              agentID.String(),
	})
	if err != nil {
		t.Fatalf("Failed to mark agent as claimed: %v", err)
	}

	// Try to register the claimed agent
	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/agent/register", bytes.NewReader(reqJSON))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for already claimed agent, got %d. Body: %s", w.Code, w.Body.String())
	}

	var errResp api.BadRequestJSONResponse
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("Failed to unmarshal error response: %v", err)
	}

	if errResp.Error != "already_claimed" {
		t.Errorf("Expected error 'already_claimed', got '%s'", errResp.Error)
	}
}

func TestRegisterAgentSelf_Integration_InvalidData(t *testing.T) {
	log := logger.Default()
	db := testutil.SetupTestSQLiteDB(t)
	ctx := context.Background()

	applySchema(t, ctx, db.DB())

	handler := NewHandler(log, db)
	router := setupTestRouter(handler)

	tests := []struct {
		name           string
		body           *api.AgentSelfRegistration
		expectedStatus int
		expectedError  string
	}{
		{
			name: "empty hostname",
			body: &api.AgentSelfRegistration{
				AgentId:        uuid.Must(uuid.NewV7()),
				ClaimTokenHash: "a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3",
				Hostname:       "",
				AgentVersion:   "1.0.0",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "validation_error",
		},
		{
			name: "empty version",
			body: &api.AgentSelfRegistration{
				AgentId:        uuid.Must(uuid.NewV7()),
				ClaimTokenHash: "a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3",
				Hostname:       "test-host",
				AgentVersion:   "",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "validation_error",
		},
		{
			name: "invalid token length",
			body: &api.AgentSelfRegistration{
				AgentId:        uuid.Must(uuid.NewV7()),
				ClaimTokenHash: "invalid-hash",
				Hostname:       "test-host",
				AgentVersion:   "1.0.0",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "validation_error",
		},
		{
			name: "invalid token decode to hex",
			body: &api.AgentSelfRegistration{
				AgentId:        uuid.Must(uuid.NewV7()),
				ClaimTokenHash: "11111111111111111111g1111111111111111111111111111111111111111111",
				Hostname:       "test-host",
				AgentVersion:   "1.0.0",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "validation_error",
		},
		// IMPORTANT: in this test body will be converted to default values, not nil,
		// because all field in AgentSelfRegistration are mandatory.
		{
			name:           "body nil",
			body:           nil,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "validation_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqJSON, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/agent/register", bytes.NewReader(reqJSON))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}

			var errResp api.BadRequestJSONResponse
			if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
				t.Fatalf("Failed to unmarshal error response: %v", err)
			}

			if errResp.Error != tt.expectedError {
				t.Errorf("Expected error '%s', got '%s'", tt.expectedError, errResp.Error)
			}
		})
	}
}
