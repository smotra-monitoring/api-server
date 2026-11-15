package handlers

import (
	"context"
	"database/sql"
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
	workspaceID := uuid.New().String()
	agentID := "019bdeb2-50dc-794e-808b-cf47526b867f"
	apiKey := "test-secret-key-123"
	apiKeyHash := middleware.HashAPIKeyForTests(apiKey) // Helper function we need to export

	_, err := db.DB().ExecContext(ctx, "INSERT INTO tenants (id, name) VALUES (?, ?)", tenantID, "Test Tenant")
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	_, err = db.DB().ExecContext(ctx, "INSERT INTO workspaces (id, tenant_id, name) VALUES (?, ?, ?)", workspaceID, tenantID, "Test Section")
	if err != nil {
		t.Fatalf("Failed to create section: %v", err)
	}

	baseConfig := `{"monitoring":{"interval_secs":60,"timeout_secs":5,"ping_count":3,"max_concurrent":10,"traceroute_on_failure":false,"traceroute_max_hops":30},"server":{"heartbeat_interval_secs":300,"verify_tls":true,"timeout_secs":5,"retry_attempts":3},"storage":{"cache_dir":"./cache","max_cached_results":10000,"max_cache_age_secs":86400}}`

	_, err = q.CreateAgent(ctx, queries.CreateAgentParams{
		ID:          agentID,
		WorkspaceID: workspaceID,
		Name:        "Test Agent",
		ApiKeyHash:  apiKeyHash,
		BaseConfig:  baseConfig,
	})
	if err != nil {
		t.Fatalf("Failed to create agent: %v", err)
	}

	cfg := testutil.DefaultTestConfig()
	metricsHandler := NewMetricsHandler(log, "test")
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

	oauthCtx := func(userID string) context.Context {
		return context.WithValue(context.Background(), middleware.AuthContextKey, &middleware.AuthInfo{
			AuthType:      "oauth2",
			Authenticated: true,
			UserID:        userID,
			SessionID:     "session-" + userID,
		})
	}
	createUser := func(workspace string) string {
		id := uuid.Must(uuid.NewV7()).String()
		if _, err := db.DB().ExecContext(ctx, `INSERT INTO users (id, workspace_id, oauth_provider, oauth_subject, display_name) VALUES (?, ?, ?, ?, ?)`,
			id, workspace, "test_provider", "sub-"+id, "User"); err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}
		return id
	}

	otherWorkspaceID := uuid.New().String()
	if _, err = db.DB().ExecContext(ctx, "INSERT INTO workspaces (id, tenant_id, name) VALUES (?, ?, ?)", otherWorkspaceID, tenantID, "Other"); err != nil {
		t.Fatalf("Failed to create second workspace: %v", err)
	}
	sameWorkspaceUser := createUser(workspaceID)
	otherWorkspaceUser := createUser(otherWorkspaceID)

	// Test 6: OAuth2 user in the agent's workspace may read the configuration without touching last_seen_at
	t.Run("ConfigurationOAuth2SameWorkspace", func(t *testing.T) {
		agentUUID, _ := uuid.Parse(agentID)

		// Earlier subtests read as the agent, which sets last_seen_at.
		if _, err := db.DB().ExecContext(ctx, "UPDATE agents SET last_seen_at = NULL WHERE id = ?", agentID); err != nil {
			t.Fatalf("Failed to reset last_seen_at: %v", err)
		}

		resp, err := handler.GetAgentConfiguration(oauthCtx(sameWorkspaceUser), api.GetAgentConfigurationRequestObject{AgentId: agentUUID})
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if _, ok := resp.(api.GetAgentConfiguration200JSONResponse); !ok {
			t.Fatalf("Expected GetAgentConfiguration200JSONResponse, got %T", resp)
		}

		var lastSeen sql.NullTime
		if err := db.DB().QueryRowContext(ctx, "SELECT last_seen_at FROM agents WHERE id = ?", agentID).Scan(&lastSeen); err != nil {
			t.Fatalf("Failed to read last_seen_at: %v", err)
		}
		if lastSeen.Valid {
			t.Errorf("OAuth2 read must not set last_seen_at, got %v", lastSeen.Time)
		}
	})

	// Test 7: OAuth2 user from another workspace is rejected
	t.Run("ConfigurationOAuth2ForeignWorkspace", func(t *testing.T) {
		agentUUID, _ := uuid.Parse(agentID)

		resp, err := handler.GetAgentConfiguration(oauthCtx(otherWorkspaceUser), api.GetAgentConfigurationRequestObject{AgentId: agentUUID})
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if _, ok := resp.(api.GetAgentConfiguration401JSONResponse); !ok {
			t.Errorf("Expected GetAgentConfiguration401JSONResponse, got %T", resp)
		}
	})

	// Test 8: OAuth2 user that does not exist is rejected
	t.Run("ConfigurationOAuth2UnknownUser", func(t *testing.T) {
		agentUUID, _ := uuid.Parse(agentID)

		resp, err := handler.GetAgentConfiguration(oauthCtx(uuid.New().String()), api.GetAgentConfigurationRequestObject{AgentId: agentUUID})
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if _, ok := resp.(api.GetAgentConfiguration401JSONResponse); !ok {
			t.Errorf("Expected GetAgentConfiguration401JSONResponse, got %T", resp)
		}
	})

	// Test 9: OAuth2 user asking for an unknown agent gets 404
	t.Run("ConfigurationOAuth2UnknownAgent", func(t *testing.T) {
		resp, err := handler.GetAgentConfiguration(oauthCtx(sameWorkspaceUser), api.GetAgentConfigurationRequestObject{AgentId: uuid.New()})
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if _, ok := resp.(api.GetAgentConfiguration404JSONResponse); !ok {
			t.Errorf("Expected GetAgentConfiguration404JSONResponse, got %T", resp)
		}
	})
}
