package agent_claim

import (
	"strings"
	"testing"

	"github.com/smotra-monitoring/server/internal/logger"
	"github.com/smotra-monitoring/server/internal/testutil"
)

func TestNewHandler(t *testing.T) {
	log := logger.Default()
	db := testutil.NewMockDatabase()

	handler := NewHandler(log, db)

	if handler == nil {
		t.Fatal("NewHandler returned nil")
	}

	if handler.logger == nil {
		t.Error("Handler logger is nil")
	}

	if handler.db == nil {
		t.Error("Handler db is nil")
	}
}

func TestHandler_GetMetrics(t *testing.T) {
	log := logger.Default()
	db := testutil.NewMockDatabase()

	handler := NewHandler(log, db)
	metrics := handler.GetMetrics()

	if metrics == "" {
		t.Fatal("GetMetrics returned empty string")
	}

	expectedKeys := []string{
		"agent_claim_attempts_total",
		"agent_claim_success_total",
		"agent_claim_failure_total",
		"agent_claim_invalid_token_total",
		"agent_claim_not_found_total",
		"agent_claim_already_claimed_total",
	}

	for _, key := range expectedKeys {
		if !strings.Contains(metrics, key) {
			t.Errorf("Expected metric %s to be present", key)
		}
	}
}

func TestGenerateAPIKey_Format(t *testing.T) {
	key, err := generateAPIKey()
	if err != nil {
		t.Fatalf("generateAPIKey failed: %v", err)
	}

	// Check prefix
	if !strings.HasPrefix(key, "sk_live_") {
		t.Errorf("Expected API key to start with 'sk_live_', got: %s", key)
	}

	// Check length (sk_live_ = 8 chars + 64 hex chars = 72 total)
	if len(key) != 72 {
		t.Errorf("Expected API key length to be 72, got: %d", len(key))
	}
}

func TestGenerateAPIKey_OnlyHexCharacters(t *testing.T) {
	key, err := generateAPIKey()
	if err != nil {
		t.Fatalf("generateAPIKey failed: %v", err)
	}

	// Remove prefix and check remaining characters are hex
	suffix := strings.TrimPrefix(key, "sk_live_")
	for _, char := range suffix {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			t.Errorf("Expected API key to contain only hex characters, found: %c", char)
		}
	}
}

func TestGenerateAPIKey_Uniqueness(t *testing.T) {
	key1, err := generateAPIKey()
	if err != nil {
		t.Fatalf("generateAPIKey failed: %v", err)
	}

	key2, err := generateAPIKey()
	if err != nil {
		t.Fatalf("generateAPIKey failed: %v", err)
	}

	if key1 == key2 {
		t.Error("Expected generateAPIKey to produce unique keys")
	}
}

func TestHandler_PostAgentsClaim_NotFound(t *testing.T) {
	t.Skip("Skipping unit test - requires real database. See integration tests.")
}

func TestHandler_PostAgentsClaim_InvalidToken(t *testing.T) {
	t.Skip("Skipping unit test - requires real database. See integration tests.")
}

func TestHandler_PostAgentsClaim_AlreadyClaimed(t *testing.T) {
	t.Skip("Skipping unit test - requires real database. See integration tests.")
}

func TestHandler_PostAgentsClaim_Success(t *testing.T) {
	t.Skip("Skipping unit test - requires real database. See integration tests.")
}
