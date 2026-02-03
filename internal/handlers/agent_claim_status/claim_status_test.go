package agent_claim_status

import (
	"strings"
	"testing"

	api "github.com/smotra-monitoring/server/internal/api/v1"
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
		"agent_claim_status_poll_attempts_total",
		"agent_claim_status_pending_total",
		"agent_claim_status_poll_failed_total",
		"agent_claim_status_already_delivered_total",
		"agent_claim_status_not_found_total",
		"agent_api_key_delivery_total",
	}

	for _, key := range expectedKeys {
		if !strings.Contains(metrics, key) {
			t.Errorf("Expected metric %s to be present", key)
		}
	}
}

func TestNewClaimStatus200Response_Pending(t *testing.T) {
	pending := api.ClaimStatusPending{
		Status: "pending_claim",
	}

	response, err := newClaimStatus200Response(pending)
	if err != nil {
		t.Fatalf("newClaimStatus200Response failed: %v", err)
	}

	// Verify the response implements the correct interface
	if response == nil {
		t.Error("newClaimStatus200Response returned nil")
	}
}

func TestNewClaimStatus200Response_Claimed(t *testing.T) {
	claimed := api.ClaimStatusClaimed{
		Status:    "claimed",
		ApiKey:    "sk_live_test123",
		ConfigUrl: "/agents/123/configuration",
	}

	response, err := newClaimStatus200Response(claimed)
	if err != nil {
		t.Fatalf("newClaimStatus200Response failed: %v", err)
	}

	// Verify the response implements the correct interface
	if response == nil {
		t.Error("newClaimStatus200Response returned nil")
	}
}

func TestClaimStatusResponse_VisitGetAgentClaimStatusResponse(t *testing.T) {
	// Test that newClaimStatus200Response produces valid responses for both types
	pending := api.ClaimStatusPending{
		Status: "pending_claim",
	}

	response, err := newClaimStatus200Response(pending)
	if err != nil {
		t.Fatalf("newClaimStatus200Response failed for pending: %v", err)
	}

	if response == nil {
		t.Error("newClaimStatus200Response returned nil response")
	}

	// Test with claimed status
	claimed := api.ClaimStatusClaimed{
		Status:    "claimed",
		ApiKey:    "sk_live_test",
		ConfigUrl: "/agents/test/configuration",
	}

	response2, err := newClaimStatus200Response(claimed)
	if err != nil {
		t.Fatalf("newClaimStatus200Response failed for claimed: %v", err)
	}

	if response2 == nil {
		t.Error("newClaimStatus200Response returned nil response for claimed")
	}
}

func TestHandler_GetAgentClaimStatus_NotFound(t *testing.T) {
	t.Skip("Skipping unit test - requires real database. See integration tests.")
}

func TestHandler_GetAgentClaimStatus_Pending(t *testing.T) {
	t.Skip("Skipping unit test - requires real database. See integration tests.")
}

func TestHandler_GetAgentClaimStatus_Claimed(t *testing.T) {
	t.Skip("Skipping unit test - requires real database. See integration tests.")
}

func TestHandler_GetAgentClaimStatus_AlreadyDelivered(t *testing.T) {
	t.Skip("Skipping unit test - requires real database. See integration tests.")
}
