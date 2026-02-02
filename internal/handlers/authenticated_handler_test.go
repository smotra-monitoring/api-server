package handlers

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/smotra-monitoring/server/internal/api"
	"github.com/smotra-monitoring/server/internal/logger"
	"github.com/smotra-monitoring/server/internal/middleware"
	"github.com/smotra-monitoring/server/internal/testutil"
)

func TestAuthenticatedHandler_GetAgentConfiguration_NoAuth(t *testing.T) {
	log := logger.New(logger.Config{Level: "error", Format: "json"})
	mockDB := testutil.NewMockDatabase()
	cfg := testutil.DefaultTestConfig()
	handler := NewAuthenticatedHandler(log, mockDB, cfg, "test")

	agentID, _ := uuid.Parse("019bdeb2-50dc-794e-808b-cf47526b867f")
	request := api.GetAgentConfigurationRequestObject{
		AgentId: agentID,
	}

	ctx := context.Background()
	resp, err := handler.GetAgentConfiguration(ctx, request)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Should return 401 unauthorized
	respTyped, ok := resp.(api.GetAgentConfiguration401JSONResponse)
	if !ok {
		t.Errorf("Expected GetAgentConfiguration401JSONResponse, got %T", resp)
	} else if respTyped.Error != "unauthorized" {
		t.Errorf("Expected error 'unauthorized', got %q", respTyped.Error)
	}
}

func TestAuthenticatedHandler_GetAgentConfiguration_WithAuth(t *testing.T) {
	t.Skip("Skipping - requires integration test with real database")
	// This test would need a real database or more sophisticated mocking
	// See integration tests for full end-to-end authentication testing
}

func TestAuthenticatedHandler_GetAgentConfiguration_WrongAgent(t *testing.T) {
	log := logger.New(logger.Config{Level: "error", Format: "json"})
	mockDB := testutil.NewMockDatabase()
	cfg := testutil.DefaultTestConfig()
	handler := NewAuthenticatedHandler(log, mockDB, cfg, "test")

	authenticatedAgentID := "019bdeb2-50dc-794e-808b-cf47526b867f"
	requestedAgentID := "019bdeb2-0000-0000-0000-000000000000"
	requestedUUID, _ := uuid.Parse(requestedAgentID)

	// Create context with authentication for a different agent
	authInfo := &middleware.AuthInfo{
		AgentID:       authenticatedAgentID,
		AuthType:      "agent_api_key",
		Authenticated: true,
	}
	ctx := context.WithValue(context.Background(), middleware.AuthContextKey, authInfo)

	request := api.GetAgentConfigurationRequestObject{
		AgentId: requestedUUID,
	}

	resp, err := handler.GetAgentConfiguration(ctx, request)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Should return 503 forbidden due to agent ID mismatch
	respTyped, ok := resp.(api.GetAgentConfiguration503JSONResponse)
	if !ok {
		t.Errorf("Expected GetAgentConfiguration503JSONResponse, got %T", resp)
	} else if respTyped.Error != "forbidden" {
		t.Errorf("Expected error 'forbidden', got %q", respTyped.Error)
	}
}

func TestCombinedHandler_HealthCheck_NoAuthRequired(t *testing.T) {
	log := logger.New(logger.Config{Level: "error", Format: "json"})
	mockDB := testutil.NewMockDatabase()
	cfg := testutil.DefaultTestConfig()
	handler := NewCombinedHandler(log, mockDB, cfg, "test")

	ctx := context.Background() // No authentication in context
	request := api.HealthCheckRequestObject{}

	resp, err := handler.HealthCheck(ctx, request)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Health check should work without authentication
	_, ok := resp.(api.HealthCheck200JSONResponse)
	if !ok {
		t.Errorf("Expected HealthCheck200JSONResponse, got %T", resp)
	}
}

func TestCombinedHandler_PrometheusMetrics_NoAuthRequired(t *testing.T) {
	log := logger.New(logger.Config{Level: "error", Format: "json"})
	mockDB := testutil.NewMockDatabase()
	cfg := testutil.DefaultTestConfig()
	handler := NewCombinedHandler(log, mockDB, cfg, "test")

	ctx := context.Background() // No authentication in context
	request := api.PrometheusMetricsRequestObject{}

	resp, err := handler.PrometheusMetrics(ctx, request)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Metrics should work without authentication
	_, ok := resp.(api.PrometheusMetrics200TextResponse)
	if !ok {
		t.Errorf("Expected PrometheusMetrics200TextResponse, got %T", resp)
	}
}
