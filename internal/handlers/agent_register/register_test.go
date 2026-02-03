package agent_register

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	api "github.com/smotra-monitoring/server/internal/api/v1"
	"github.com/smotra-monitoring/server/internal/logger"
	"github.com/smotra-monitoring/server/internal/testutil"
)

func TestNewHandler(t *testing.T) {
	log := logger.Default()
	db := testutil.NewMockDatabase()
	cfg := testutil.DefaultTestConfig()

	handler := NewHandler(log, db, cfg)

	if handler == nil {
		t.Fatal("NewHandler returned nil")
	}

	if handler.logger == nil {
		t.Error("Handler logger is nil")
	}

	if handler.db == nil {
		t.Error("Handler db is nil")
	}

	if handler.config == nil {
		t.Error("Handler config is nil")
	}
}

func TestHandler_GetMetrics(t *testing.T) {
	log := logger.Default()
	db := testutil.NewMockDatabase()
	cfg := testutil.DefaultTestConfig()

	handler := NewHandler(log, db, cfg)
	metrics := handler.GetMetrics()

	if metrics == "" {
		t.Fatal("GetMetrics returned empty string")
	}

	expectedKeys := []string{
		"agent_registration_attempts_total",
		"agent_registration_success_total",
		"agent_registration_failure_total",
		"agent_registration_idempotent_total",
	}

	for _, key := range expectedKeys {
		if !strings.Contains(metrics, key) {
			t.Errorf("Expected metric %s to be present", key)
		}
	}
}

func TestHandler_ValidateRequest_ValidData(t *testing.T) {
	log := logger.Default()
	db := testutil.NewMockDatabase()
	cfg := testutil.DefaultTestConfig()
	handler := NewHandler(log, db, cfg)

	agentID := uuid.Must(uuid.NewV7())
	req := &api.AgentSelfRegistration{
		AgentId:        agentID,
		ClaimTokenHash: "a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3",
		Hostname:       "test-host",
		AgentVersion:   "1.0.0",
	}

	err := handler.validateRequest(req)
	if err != nil {
		t.Errorf("Expected valid request to pass validation, got error: %v", err)
	}
}

func TestHandler_ValidateRequest_EmptyAgentID(t *testing.T) {
	log := logger.Default()
	db := testutil.NewMockDatabase()
	cfg := testutil.DefaultTestConfig()
	handler := NewHandler(log, db, cfg)

	req := &api.AgentSelfRegistration{
		AgentId:        uuid.Nil,
		ClaimTokenHash: "a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3",
		Hostname:       "test-host",
		AgentVersion:   "1.0.0",
	}

	err := handler.validateRequest(req)
	if err == nil {
		t.Error("Expected validation to fail for nil UUID")
	}
}

func TestHandler_ValidateRequest_InvalidClaimTokenHash(t *testing.T) {
	log := logger.Default()
	db := testutil.NewMockDatabase()
	cfg := testutil.DefaultTestConfig()
	handler := NewHandler(log, db, cfg)

	agentID := uuid.Must(uuid.NewV7())

	tests := []struct {
		name string
		hash string
	}{
		{"empty", ""},
		{"too short", "a665a459"},
		{"too long", "a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3ff"},
		{"non-hex characters", "g665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &api.AgentSelfRegistration{
				AgentId:        agentID,
				ClaimTokenHash: tt.hash,
				Hostname:       "test-host",
				AgentVersion:   "1.0.0",
			}
			err := handler.validateRequest(req)
			if err == nil {
				t.Errorf("Expected validation to fail for %s", tt.name)
			}
		})
	}
}

func TestHandler_ValidateRequest_MissingHostname(t *testing.T) {
	log := logger.Default()
	db := testutil.NewMockDatabase()
	cfg := testutil.DefaultTestConfig()
	handler := NewHandler(log, db, cfg)

	agentID := uuid.Must(uuid.NewV7())
	req := &api.AgentSelfRegistration{
		AgentId:        agentID,
		ClaimTokenHash: "a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3",
		Hostname:       "",
		AgentVersion:   "1.0.0",
	}

	err := handler.validateRequest(req)
	if err == nil {
		t.Error("Expected validation to fail for missing hostname")
	}
}

func TestHandler_ValidateRequest_MissingVersion(t *testing.T) {
	log := logger.Default()
	db := testutil.NewMockDatabase()
	cfg := testutil.DefaultTestConfig()
	handler := NewHandler(log, db, cfg)

	agentID := uuid.Must(uuid.NewV7())
	req := &api.AgentSelfRegistration{
		AgentId:        agentID,
		ClaimTokenHash: "a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3",
		Hostname:       "test-host",
		AgentVersion:   "",
	}

	err := handler.validateRequest(req)
	if err == nil {
		t.Error("Expected validation to fail for missing agent version")
	}
}

func TestGenerateClaimToken_Format(t *testing.T) {
	token, err := GenerateClaimToken()
	if err != nil {
		t.Fatalf("GenerateClaimToken failed: %v", err)
	}

	// Check length (8 bytes = 16 hex characters)
	if len(token) != 16 {
		t.Errorf("Expected token length to be 16, got: %d", len(token))
	}

	// Check that it's valid hex
	for _, char := range token {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			t.Errorf("Expected token to contain only hex characters, found: %c", char)
		}
	}
}

func TestGenerateClaimToken_Uniqueness(t *testing.T) {
	token1, err := GenerateClaimToken()
	if err != nil {
		t.Fatalf("GenerateClaimToken failed: %v", err)
	}

	token2, err := GenerateClaimToken()
	if err != nil {
		t.Fatalf("GenerateClaimToken failed: %v", err)
	}

	if token1 == token2 {
		t.Error("Expected GenerateClaimToken to produce unique tokens")
	}
}

func TestHandler_PostAgentsRegister_DatabaseError(t *testing.T) {
	t.Skip("Skipping unit test - requires real database. See integration tests.")
}

func TestHandler_PostAgentsRegister_Success(t *testing.T) {
	t.Skip("Skipping unit test - requires real database. See integration tests.")
}

func TestHandler_PostAgentsRegister_Idempotent(t *testing.T) {
	t.Skip("Skipping unit test - requires real database. See integration tests.")
}
