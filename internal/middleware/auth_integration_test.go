package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/smotra-monitoring/server/internal/database/queries"
	"github.com/smotra-monitoring/server/internal/logger"
	"github.com/smotra-monitoring/server/internal/testutil"
)

// Integration tests require a database
func TestAgentAPIKeyAuth_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create test database
	db := testutil.SetupTestSQLiteDB(t)
	ctx := context.Background()

	// Apply schema
	schemaSQL, err := os.ReadFile("../../data/db/dev/migrations/0001_schema.up.sql")
	if err != nil {
		t.Fatalf("Failed to read schema file: %v", err)
	}

	_, err = db.DB().ExecContext(ctx, string(schemaSQL))
	if err != nil {
		t.Fatalf("Failed to apply schema: %v", err)
	}

	// Create test data
	q := queries.New(db.DB())

	// Create a tenant
	tenantID := uuid.New().String()
	_, err = db.DB().ExecContext(ctx, "INSERT INTO tenants (id, name) VALUES (?, ?)", tenantID, "Test Tenant")
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	// Create a section
	sectionID := uuid.New().String()
	_, err = db.DB().ExecContext(ctx, "INSERT INTO sections (id, tenant_id, name) VALUES (?, ?, ?)", sectionID, tenantID, "Test Section")
	if err != nil {
		t.Fatalf("Failed to create section: %v", err)
	}

	// Create an agent with a known API key
	agentID := "019bdeb2-50dc-794e-808b-cf47526b867f"
	apiKey := "test-secret-key-123"
	apiKeyHash := hashAPIKey(apiKey)
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

	// Test 1: Valid API key authentication
	t.Run("ValidAPIKey", func(t *testing.T) {
		log := logger.New(logger.Config{Level: "error", Format: "json"})
		middleware := AgentAPIKeyAuth(log, db)

		authenticated := false
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authInfo := r.Context().Value(AuthContextKey)
			if authInfo == nil {
				t.Error("Expected authentication info in context")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			info, ok := authInfo.(*AuthInfo)
			if !ok {
				t.Error("Expected AuthInfo type")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			if !info.Authenticated {
				t.Error("Expected Authenticated to be true")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			if info.AgentID != agentID {
				t.Errorf("Expected AgentID %q, got %q", agentID, info.AgentID)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			if info.AuthType != "agent_api_key" {
				t.Errorf("Expected AuthType 'agent_api_key', got %q", info.AuthType)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			authenticated = true
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/agent/"+agentID+"/configuration", nil)
		req.Header.Set("X-Agent-API-Key", apiKey)
		w := httptest.NewRecorder()

		middleware(handler).ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		if !authenticated {
			t.Error("Handler was not called or authentication failed")
		}
	})

	// Test 2: Invalid API key
	t.Run("InvalidAPIKey", func(t *testing.T) {
		log := logger.New(logger.Config{Level: "error", Format: "json"})
		// This should NOT reject the request, just mark as unauthenticated
		middleware := AgentAPIKeyAuth(log, db)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/agent/"+agentID+"/configuration", nil)
		req.Header.Set("X-Agent-API-Key", "wrong-key")
		w := httptest.NewRecorder()

		middleware(handler).ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}
	})

	// Test 3: Non-existent agent
	t.Run("NonExistentAgent", func(t *testing.T) {
		log := logger.New(logger.Config{Level: "error", Format: "json"})
		middleware := AgentAPIKeyAuth(log, db)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("Handler should not be called for non-existent agent")
		})

		nonExistentID := "019bdeb2-0000-0000-0000-000000000000"
		req := httptest.NewRequest("GET", "/agent/"+nonExistentID+"/configuration", nil)
		req.Header.Set("X-Agent-API-Key", apiKey)
		w := httptest.NewRecorder()

		middleware(handler).ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("Expected status 500, got %d", w.Code)
		}
	})
}

func TestAuthenticationChain_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create test database
	db := testutil.SetupTestSQLiteDB(t)
	ctx := context.Background()

	// Apply schema
	schemaSQL, err := os.ReadFile("../../data/db/dev/migrations/0001_schema.up.sql")
	if err != nil {
		t.Fatalf("Failed to read schema file: %v", err)
	}

	_, err = db.DB().ExecContext(ctx, string(schemaSQL))
	if err != nil {
		t.Fatalf("Failed to apply schema: %v", err)
	}

	// Create test agent
	q := queries.New(db.DB())
	tenantID := uuid.New().String()
	sectionID := uuid.New().String()
	agentID := "019bdeb2-50dc-794e-808b-cf47526b867f"
	apiKey := "test-key"
	apiKeyHash := hashAPIKey(apiKey)

	_, err = db.DB().ExecContext(ctx, "INSERT INTO tenants (id, name) VALUES (?, ?)", tenantID, "Test Tenant")
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

	log := logger.New(logger.Config{Level: "error", Format: "json"})

	//  Middleware execution order: first wrapped = first executed
	// We want: AgentAPIKey -> OAuth2 -> RequireAuth -> handler
	// So we wrap in reverse: handler -> RequireAuth -> OAuth2 -> AgentAPIKey
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Authenticated!"))
	})

	requireAuth := RequireAuthForTests(log)(finalHandler)
	oauth := OAuth2Auth(log)(requireAuth)
	chain := AgentAPIKeyAuth(log, db)(oauth)

	// Test: Valid API key passes through entire chain
	t.Run("ValidAPIKeyPassesChain", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/agent/"+agentID+"/configuration", nil)
		req.Header.Set("X-Agent-API-Key", apiKey)
		w := httptest.NewRecorder()

		chain.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		if w.Body.String() != "Authenticated!" {
			t.Errorf("Expected body 'Authenticated!', got %q", w.Body.String())
		}
	})

	// Test: No authentication fails at RequireAuth
	t.Run("NoAuthFailsChain", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/agent/"+agentID+"/configuration", nil)
		w := httptest.NewRecorder()

		chain.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", w.Code)
		}
	})

	// Test: OAuth2 Bearer token is rejected
	t.Run("OAuth2IsNotImplemented", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/agent/"+agentID+"/configuration", nil)
		req.Header.Set("Authorization", "Bearer some-token")
		w := httptest.NewRecorder()

		chain.ServeHTTP(w, req)

		if w.Code != http.StatusNotImplemented {
			t.Errorf("Expected status 501, got %d", w.Code)
		}
	})
}
