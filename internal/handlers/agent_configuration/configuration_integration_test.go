package agent_configuration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	schema := `
	-- 1. Tenants: Top-level isolation
	CREATE TABLE tenants (
		id           TEXT PRIMARY KEY,
		name         TEXT NOT NULL,
		created_at   TEXT DEFAULT (strftime('%Y-%m-%d %H:%M:%S', 'now'))
	) STRICT, WITHOUT ROWID;

	-- 2. Sections: Divisions within a tenant
	CREATE TABLE sections (
		id           TEXT PRIMARY KEY,
		tenant_id    TEXT NOT NULL,
		name         TEXT NOT NULL,
		UNIQUE(tenant_id, name),
		FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
	) STRICT, WITHOUT ROWID;

	-- 3. Tags: Scoped definitions for agents or endpoints
	CREATE TABLE tags (
		id           TEXT PRIMARY KEY,
		section_id   TEXT NOT NULL,
		name         TEXT NOT NULL,
		scope        TEXT CHECK(scope IN ('agent', 'endpoint', 'global')) DEFAULT 'global',
		UNIQUE(section_id, name),
		FOREIGN KEY (section_id) REFERENCES sections(id) ON DELETE CASCADE
	) STRICT, WITHOUT ROWID;

	-- 4. Agents: The remote monitoring units
	CREATE TABLE agents (
		id             TEXT PRIMARY KEY,
		version        INTEGER NOT NULL DEFAULT 1,
		section_id     TEXT NOT NULL,
		name           TEXT NOT NULL,
		api_key_hash   TEXT NOT NULL,
		base_config    TEXT NOT NULL, -- JSON blob
		last_seen_at   TEXT,
		created_at     TEXT DEFAULT (strftime('%Y-%m-%d %H:%M:%S', 'now')),
		FOREIGN KEY (section_id) REFERENCES sections(id) ON DELETE CASCADE
	) STRICT, WITHOUT ROWID;

	-- 5. Agent Tags: Many-to-Many link
	CREATE TABLE agent_tags (
		agent_id    TEXT NOT NULL,
		tag_id      TEXT NOT NULL,
		PRIMARY KEY (agent_id, tag_id),
		FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE,
		FOREIGN KEY (tag_id) REFERENCES tags(id) ON DELETE CASCADE
	) STRICT, WITHOUT ROWID;

	-- 6. Endpoints: Specific targets per agent
	CREATE TABLE endpoints (
		id          TEXT PRIMARY KEY,
		agent_id    TEXT NOT NULL,
		address     TEXT NOT NULL,
		port		INTEGER,
		enabled     INT DEFAULT 1, -- 1 for true, 0 for false
		created_at  TEXT DEFAULT (strftime('%Y-%m-%d %H:%M:%S', 'now')),
		FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE
	) STRICT, WITHOUT ROWID;

	-- 7. Endpoint Tags: Many-to-Many link
	CREATE TABLE endpoint_tags (
		endpoint_id TEXT NOT NULL,
		tag_id      TEXT NOT NULL,
		PRIMARY KEY (endpoint_id, tag_id),
		FOREIGN KEY (endpoint_id) REFERENCES endpoints(id) ON DELETE CASCADE,
		FOREIGN KEY (tag_id) REFERENCES tags(id) ON DELETE CASCADE
	) STRICT, WITHOUT ROWID;
	`

	_, err := db.DB().ExecContext(ctx, schema)
	if err != nil {
		t.Fatalf("Failed to apply schema: %v", err)
	}

	handler := NewHandler(log, db, "1.0.0")

	// Insert test data
	q := queries.New(db.DB())

	// Create a tenant
	tenantID := uuid.NewString()
	if _, err = db.DB().ExecContext(ctx, "INSERT INTO tenants (id, name) VALUES (?, ?)", tenantID, "Test Tenant"); err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	// Create a section
	sectionID := uuid.NewString()
	if _, err = db.DB().ExecContext(ctx, "INSERT INTO sections (id, tenant_id, name) VALUES (?, ?, ?)", sectionID, tenantID, "Test Section"); err != nil {
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

	_, err = q.CreateAgent(ctx, queries.CreateAgentParams{
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
	tag1ID := uuid.NewString()
	if _, err = db.DB().ExecContext(ctx, "INSERT INTO tags (id, section_id, name, scope) VALUES (?, ?, ?, ?)", tag1ID, sectionID, "production", "agent"); err != nil {
		t.Fatalf("Failed to create tag: %v", err)
	}

	if _, err = db.DB().ExecContext(ctx, "INSERT INTO agent_tags (agent_id, tag_id) VALUES (?, ?)", agentID, tag1ID); err != nil {
		t.Fatalf("Failed to link agent tag: %v", err)
	}

	// Create endpoints for the agent
	endpoint1ID := uuid.NewString()
	if _, err = db.DB().ExecContext(ctx, "INSERT INTO endpoints (id, agent_id, address, enabled) VALUES (?, ?, ?, ?)", endpoint1ID, agentID, "192.168.1.1", 1); err != nil {
		t.Fatalf("Failed to create endpoint: %v", err)
	}

	endpoint2ID := uuid.NewString()
	if _, err = db.DB().ExecContext(ctx, "INSERT INTO endpoints (id, agent_id, address, enabled) VALUES (?, ?, ?, ?)", endpoint2ID, agentID, "8.8.8.8", 1); err != nil {
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

		if config.Version != 1 {
			t.Errorf("Expected version 1, got %d", config.Version)
		}

		// Verify tags
		if len(*config.Tags) != 1 {
			t.Errorf("Expected 1 tag, got %d", len(*config.Tags))
		} else if (*config.Tags)[0] != "production" {
			t.Errorf("Expected tag 'production', got %s", (*config.Tags)[0])
		}

		// Verify endpoints
		if len(config.Endpoints) != 2 {
			t.Errorf("Expected 2 endpoints, got %d", len(config.Endpoints))
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

		if metrics["get_configuration_total"] < 2 {
			t.Errorf("Expected at least 2 total requests, got %d", metrics["get_configuration_total"])
		}

		if metrics["get_configuration_success"] < 1 {
			t.Errorf("Expected at least 1 successful request, got %d", metrics["get_configuration_success"])
		}

		if metrics["get_configuration_failure"] < 1 {
			t.Errorf("Expected at least 1 failed request, got %d", metrics["get_configuration_failure"])
		}
	})
}
