package handlers

import (
	"context"
	"testing"

	"github.com/google/uuid"
	healthAPI "github.com/smotra-monitoring/server/internal/api/health"
	api "github.com/smotra-monitoring/server/internal/api/v1"
	"github.com/smotra-monitoring/server/internal/database/queries"
	"github.com/smotra-monitoring/server/internal/logger"
	"github.com/smotra-monitoring/server/internal/middleware"
	"github.com/smotra-monitoring/server/internal/testutil"
)

func TestAuthenticatedHandler_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create test database
	db := testutil.SetupTestSQLiteDB(t)
	ctx := context.Background()

	// Apply schema
	testutil.ApplyMigrations(t, ctx, db.DB(), "../../data/db/dev/migrations")

	log := logger.New(logger.Config{Level: "error", Format: "json"})

	// Create test data
	q := queries.New(db.DB())

	tenantID := uuid.New().String()
	sectionID := uuid.New().String()
	agentID := "019bdeb2-50dc-794e-808b-cf47526b867f"
	apiKey := "test-secret-key-123"
	apiKeyHash := middleware.HashAPIKeyForTests(apiKey) // Helper function we need to export

	_, err := db.DB().ExecContext(ctx, "INSERT INTO tenants (id, name) VALUES (?, ?)", tenantID, "Test Tenant")
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	_, err = db.DB().ExecContext(ctx, "INSERT INTO sections (id, tenant_id, name) VALUES (?, ?, ?)", sectionID, tenantID, "Test Section")
	if err != nil {
		t.Fatalf("Failed to create section: %v", err)
	}

	baseConfig := `{"monitoring":{"interval_secs":60,"timeout_secs":5,"ping_count":3,"max_concurrent":10,"traceroute_on_failure":false,"traceroute_max_hops":30},"server":{"report_interval_secs":300,"heartbeat_interval_secs":300,"verify_tls":true,"timeout_secs":5,"retry_attempts":3},"storage":{"cache_dir":"./cache","max_cached_results":10000,"max_cache_age_secs":86400}}`

	_, err = q.CreateAgent(ctx, queries.CreateAgentParams{
		ID:         agentID,
		SectionID:  sectionID,
		Name:       "Test Agent",
		ApiKeyHash: apiKeyHash,
		BaseConfig: baseConfig,
	})
	if err != nil {
		t.Fatalf("Failed to create agent: %v", err)
	}

	cfg := testutil.DefaultTestConfig()
	metricsHandler := NewMetricsHandler(log, db, "test")
	healthHandler := NewHealthHandler(log, db, cfg, "test", metricsHandler)
	handler := NewAuthenticatedHandler(log, db, cfg, "test", metricsHandler)

	// Test 1: Health check works WITHOUT authentication
	t.Run("HealthCheckNoAuth", func(t *testing.T) {
		ctx := context.Background() // No auth
		resp, err := healthHandler.HealthCheck(ctx, healthAPI.HealthCheckRequestObject{})
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		_, ok := resp.(healthAPI.HealthCheck200JSONResponse)
		if !ok {
			t.Errorf("Expected HealthCheck200JSONResponse, got %T", resp)
		}
	})

	// Test 2: Metrics work WITHOUT authentication
	t.Run("MetricsNoAuth", func(t *testing.T) {
		ctx := context.Background() // No auth
		resp, err := healthHandler.PrometheusMetrics(ctx, healthAPI.PrometheusMetricsRequestObject{})
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		_, ok := resp.(healthAPI.PrometheusMetrics200TextResponse)
		if !ok {
			t.Errorf("Expected PrometheusMetrics200TextResponse, got %T", resp)
		}
	})

	// Test 3: Agent configuration REQUIRES authentication
	t.Run("ConfigurationRequiresAuth", func(t *testing.T) {
		agentUUID, _ := uuid.Parse(agentID)
		ctx := context.Background() // No auth

		resp, err := handler.GetAgentConfiguration(ctx, api.GetAgentConfigurationRequestObject{
			AgentId: agentUUID,
		})

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		respTyped, ok := resp.(api.GetAgentConfiguration401JSONResponse)
		if !ok {
			t.Errorf("Expected GetAgentConfiguration401JSONResponse, got %T", resp)
		}

		if respTyped.Error != "unauthorized" {
			t.Errorf("Expected error 'unauthorized', got %q", respTyped.Error)
		}
	})

	// Test 4: Agent configuration works WITH valid authentication
	t.Run("ConfigurationWithAuth", func(t *testing.T) {
		agentUUID, _ := uuid.Parse(agentID)

		authInfo := &middleware.AuthInfo{
			AgentID:       agentID,
			AuthType:      "agent_api_key",
			Authenticated: true,
		}
		ctx := context.WithValue(context.Background(), middleware.AuthContextKey, authInfo)

		resp, err := handler.GetAgentConfiguration(ctx, api.GetAgentConfigurationRequestObject{
			AgentId: agentUUID,
		})

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		_, ok := resp.(api.GetAgentConfiguration200JSONResponse)
		if !ok {
			t.Errorf("Expected GetAgentConfiguration200JSONResponse, got %T", resp)
		}
	})

	// Test 5: Agent configuration REJECTS wrong agent
	t.Run("ConfigurationWrongAgent", func(t *testing.T) {
		agentUUID, _ := uuid.Parse(agentID)

		// Authenticate as a different agent
		authInfo := &middleware.AuthInfo{
			AgentID:       "different-agent-id",
			AuthType:      "agent_api_key",
			Authenticated: true,
		}
		ctx := context.WithValue(context.Background(), middleware.AuthContextKey, authInfo)

		resp, err := handler.GetAgentConfiguration(ctx, api.GetAgentConfigurationRequestObject{
			AgentId: agentUUID,
		})

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		respTyped, ok := resp.(api.GetAgentConfiguration503JSONResponse)
		if !ok {
			t.Errorf("Expected GetAgentConfiguration503JSONResponse, got %T", resp)
		}

		if respTyped.Error != "forbidden" {
			t.Errorf("Expected error 'forbidden', got %q", respTyped.Error)
		}
	})
}
