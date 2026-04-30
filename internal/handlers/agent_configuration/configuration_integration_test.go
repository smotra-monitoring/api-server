package agent_configuration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

// RegisterAgentSelf delegates to agent register handler
func (t *testServerImpl) RegisterAgentSelf(ctx context.Context, request api.RegisterAgentSelfRequestObject) (api.RegisterAgentSelfResponseObject, error) {
	return nil, nil
}

// GetAgentClaimStatus delegates to agent claim status handler
func (t *testServerImpl) GetAgentClaimStatus(ctx context.Context, request api.GetAgentClaimStatusRequestObject) (api.GetAgentClaimStatusResponseObject, error) {
	return nil, nil
}

// ClaimAgent delegates to agent claim handler
func (t *testServerImpl) PostClaimAgent(ctx context.Context, request api.PostClaimAgentRequestObject) (api.PostClaimAgentResponseObject, error) {
	return nil, nil
}

func (t *testServerImpl) Logout(ctx context.Context, request api.LogoutRequestObject) (api.LogoutResponseObject, error) {
	return nil, nil
}

func (t *testServerImpl) Oauth2Authorize(ctx context.Context, request api.Oauth2AuthorizeRequestObject) (api.Oauth2AuthorizeResponseObject, error) {
	return nil, nil
}

func (t *testServerImpl) Oauth2Callback(ctx context.Context, request api.Oauth2CallbackRequestObject) (api.Oauth2CallbackResponseObject, error) {
	return nil, nil
}

func (t *testServerImpl) Oauth2Revoke(ctx context.Context, request api.Oauth2RevokeRequestObject) (api.Oauth2RevokeResponseObject, error) {
	return nil, nil
}

func (t *testServerImpl) Oauth2Token(ctx context.Context, request api.Oauth2TokenRequestObject) (api.Oauth2TokenResponseObject, error) {
	return nil, nil
}

func (t *testServerImpl) GetUserInfo(ctx context.Context, request api.GetUserInfoRequestObject) (api.GetUserInfoResponseObject, error) {
	return nil, nil
}

func (t *testServerImpl) SubmitAgentResults(ctx context.Context, request api.SubmitAgentResultsRequestObject) (api.SubmitAgentResultsResponseObject, error) {
	return nil, nil
}

func setupTestRouter(handler *Handler) *chi.Mux {
	testImpl := &testServerImpl{Handler: handler}
	r := chi.NewRouter()
	strictHandler := api.NewStrictHandler(testImpl, nil)
	api.HandlerFromMux(strictHandler, r)
	return r
}

func TestGetAgentConfiguration_Integration(t *testing.T) {
	log := logger.Default()

	// Create a test SQLite database
	db := testutil.SetupTestSQLiteDB(t)

	ctx := context.Background()

	// Apply schema manually (read from migration file)
	testutil.ApplyMigrations(t, ctx, db.DB(), "../../../data/db/dev/migrations")

	handler := NewHandler(log, db, "1.0.0")

	// Insert test data
	q := queries.New(db.DB())

	// Create a tenant
	tenantID := uuid.NewString()
	if _, err := db.DB().ExecContext(ctx, "INSERT INTO tenants (id, name) VALUES (?, ?)", tenantID, "Test Tenant"); err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	// Create a section
	sectionID := uuid.NewString()
	if _, err := db.DB().ExecContext(ctx, "INSERT INTO sections (id, tenant_id, name) VALUES (?, ?, ?)", sectionID, tenantID, "Test Section"); err != nil {
		t.Fatalf("Failed to create section: %v", err)
	}

	// Create an agent with configuration
	agentID := uuid.NewString()
	baseConfig := `{
		"monitoring": {
			"interval_secs": 60,
			"timeout_secs": 5,
			"ping_count": 3,
			"max_concurrent": 10,
			"traceroute_on_failure": false,
			"traceroute_max_hops": 30
		},
		"server": {
			"url": "https://api.smotra.net",
			"api_key": "test-key",
			"report_interval_secs": 300,
			"heartbeat_interval_secs": 300,
			"verify_tls": true,
			"timeout_secs": 5,
			"retry_attempts": 3
		},
		"storage": {
			"cache_dir": "./cache",
			"max_cached_results": 10000,
			"max_cache_age_secs": 86400
		}
	}`

	_, err := q.CreateAgent(ctx, queries.CreateAgentParams{
		ID:         agentID,
		SectionID:  sectionID,
		Name:       "Test Agent",
		ApiKeyHash: "test-hash",
		BaseConfig: baseConfig,
	})
	if err != nil {
		t.Fatalf("Failed to create agent: %v", err)
	}

	// Create tags for the agent
	agentTag1ID := uuid.NewString()
	if _, err := db.DB().ExecContext(ctx, "INSERT INTO tags (id, section_id, name, scope) VALUES (?, ?, ?, ?)", agentTag1ID, sectionID, "production", "agent"); err != nil {
		t.Fatalf("Failed to create tag: %v", err)
	}

	if _, err := db.DB().ExecContext(ctx, "INSERT INTO agent_tags (agent_id, tag_id) VALUES (?, ?)", agentID, agentTag1ID); err != nil {
		t.Fatalf("Failed to link agent tag: %v", err)
	}

	// Create endpoints for the section (not per-agent)
	endpoint1ID := uuid.NewString()
	if _, err = db.DB().ExecContext(ctx, "INSERT INTO endpoints (id, section_id, address, enabled) VALUES (?, ?, ?, ?)", endpoint1ID, sectionID, "192.168.1.1", 1); err != nil {
		t.Fatalf("Failed to create endpoint: %v", err)
	}

	endpoint2ID := uuid.NewString()
	if _, err = db.DB().ExecContext(ctx, "INSERT INTO endpoints (id, section_id, address, enabled) VALUES (?, ?, ?, ?)", endpoint2ID, sectionID, "8.8.8.8", 1); err != nil {
		t.Fatalf("Failed to create endpoint: %v", err)
	}

	// Create endpoint tag
	endpointTag1ID := uuid.NewString()
	if _, err = db.DB().ExecContext(ctx, "INSERT INTO tags (id, section_id, name, scope) VALUES (?, ?, ?, ?)", endpointTag1ID, sectionID, "critical", "endpoint"); err != nil {
		t.Fatalf("Failed to create endpoint tag: %v", err)
	}

	if _, err = db.DB().ExecContext(ctx, "INSERT INTO endpoint_tags (endpoint_id, tag_id) VALUES (?, ?)", endpoint1ID, endpointTag1ID); err != nil {
		t.Fatalf("Failed to link endpoint tag: %v", err)
	}

	// Link the same endpoint tag to endpoint2 so both are resolved via topology
	if _, err = db.DB().ExecContext(ctx, "INSERT INTO endpoint_tags (endpoint_id, tag_id) VALUES (?, ?)", endpoint2ID, endpointTag1ID); err != nil {
		t.Fatalf("Failed to link endpoint2 tag: %v", err)
	}

	// Create a topology: agents tagged 'production' monitor endpoints tagged 'critical'
	topologyID := uuid.NewString()
	if _, err = db.DB().ExecContext(ctx, "INSERT INTO topologies (id, section_id, name, type, enabled) VALUES (?, ?, ?, ?, ?)", topologyID, sectionID, "Test Topology", "full-mesh", 1); err != nil {
		t.Fatalf("Failed to create topology: %v", err)
	}
	if _, err = db.DB().ExecContext(ctx, "INSERT INTO topology_members (topology_id, tag_id, role) VALUES (?, ?, ?)", topologyID, agentTag1ID, "monitor"); err != nil {
		t.Fatalf("Failed to add agent topology member: %v", err)
	}
	if _, err = db.DB().ExecContext(ctx, "INSERT INTO topology_members (topology_id, tag_id, role) VALUES (?, ?, ?)", topologyID, endpointTag1ID, "target"); err != nil {
		t.Fatalf("Failed to add endpoint topology member: %v", err)
	}

	// Setup router and server
	router := setupTestRouter(handler)
	server := httptest.NewServer(router)
	defer server.Close()

	t.Run("GetConfiguration_Success", func(t *testing.T) {
		url := fmt.Sprintf("%s/agent/%s/configuration", server.URL, agentID)
		resp, err := http.Get(url)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		var config api.AgentConfig
		if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// Verify configuration
		if config.AgentId.String() != agentID {
			t.Errorf("Expected agent ID %s, got %s", agentID, config.AgentId.String())
		}

		if config.AgentName != "Test Agent" {
			t.Errorf("Expected agent name 'Test Agent', got %s", config.AgentName)
		}

		if config.Version != 9 {
			t.Errorf("Expected version 9, got %d", config.Version)
		}

		// Verify tags
		if len(*config.Tags) != 1 {
			t.Errorf("Expected 1 tag, got %d", len(*config.Tags))
		} else if (*config.Tags)[0] != "production" {
			t.Errorf("Expected tag 'production', got %s", (*config.Tags)[0])
		}

		// Verify endpoints: count, IDs, and the required id field is populated
		if len(config.Endpoints) != 2 {
			t.Errorf("Expected 2 endpoints, got %d", len(config.Endpoints))
		} else {
			// Build a set of expected endpoint IDs for order-independent comparison
			expectedIDs := map[string]bool{
				endpoint1ID: true,
				endpoint2ID: true,
			}
			for _, ep := range config.Endpoints {
				epID := ep.Id.String()
				if epID == (uuid.UUID{}).String() {
					t.Errorf("Endpoint has zero UUID id — required field is empty")
				}
				if !expectedIDs[epID] {
					t.Errorf("Unexpected endpoint ID %q in response", epID)
				}
				delete(expectedIDs, epID)
			}
			for missing := range expectedIDs {
				t.Errorf("Endpoint ID %q not found in response", missing)
			}
		}

		// Verify monitoring config
		if config.Monitoring.IntervalSecs != 60 {
			t.Errorf("Expected interval_secs 60, got %d", config.Monitoring.IntervalSecs)
		}

		// Verify server config
		if config.Server.Url == nil || *config.Server.Url != "https://api.smotra.net" {
			t.Errorf("Expected server URL 'https://api.smotra.net', got %v", config.Server.Url)
		}

		// Verify storage config
		if config.Storage.CacheDir != "./cache" {
			t.Errorf("Expected cache_dir './cache', got %s", config.Storage.CacheDir)
		}
	})

	t.Run("GetConfiguration_NotFound", func(t *testing.T) {
		nonExistentID := uuid.NewString()
		url := fmt.Sprintf("%s/agent/%s/configuration", server.URL, nonExistentID)
		resp, err := http.Get(url)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected status 404, got %d", resp.StatusCode)
		}

		var errorResp api.Error
		if err := json.NewDecoder(resp.Body).Decode(&errorResp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if errorResp.Error != "not_found" {
			t.Errorf("Expected error 'not_found', got %s", errorResp.Error)
		}
	})

	t.Run("GetConfiguration_Metrics", func(t *testing.T) {
		metrics := handler.GetMetrics()

		if !strings.Contains(metrics, "get_configuration_total") {
			t.Errorf("Expected get_configuration_total metric, got %s", metrics)
		}

		if !strings.Contains(metrics, "get_configuration_success") {
			t.Errorf("Expected get_configuration_success metric, got %s", metrics)
		}

		if !strings.Contains(metrics, "get_configuration_failure") {
			t.Errorf("Expected get_configuration_failure metric, got %s", metrics)
		}

		if !strings.Contains(metrics, "topology_resolutions_total") {
			t.Errorf("Expected topology_resolutions_total metric, got %s", metrics)
		}

		if !strings.Contains(metrics, "topology_endpoints_resolved_total") {
			t.Errorf("Expected topology_endpoints_resolved_total metric, got %s", metrics)
		}
	})
}
