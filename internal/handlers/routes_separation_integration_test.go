package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	healthAPI "github.com/smotra-monitoring/server/internal/api/health"
	api "github.com/smotra-monitoring/server/internal/api/v1"
	"github.com/smotra-monitoring/server/internal/logger"
	"github.com/smotra-monitoring/server/internal/middleware"
	"github.com/smotra-monitoring/server/internal/testutil"
)

// TestRoutesSeparation verifies that endpoints are registered at the correct paths:
// - Health/metrics at root level (/, /healthz, /metrics)
// - API endpoints under /v1 prefix
// - No duplicate registrations
func TestRoutesSeparation(t *testing.T) {
	log := logger.New(logger.Config{Level: "error", Format: "json"})
	db := testutil.NewMockDatabase()
	cfg := testutil.DefaultTestConfig()

	// Create router with same structure as main.go
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.RequestID(log))
	r.Use(middleware.Logger(log))
	r.Use(middleware.Recovery(log))
	r.Use(middleware.CORS)
	r.Use(middleware.AgentAPIKeyAuth(log, db))
	r.Use(middleware.OAuth2Auth(log))

	// Create shared metrics handler
	metricsHandler := NewMetricsHandler(log, "test")

	// Register Health handler at root level
	healthHandler := NewHealthHandler(log, db, cfg, "test", metricsHandler)
	healthStrictHandler := healthAPI.NewStrictHandler(healthHandler, nil)
	healthAPI.HandlerFromMux(healthStrictHandler, r)

	// Register API handler under /v1
	apiHandler := NewAuthenticatedHandler(log, db, cfg, "test", metricsHandler)
	apiStrictHandler := api.NewStrictHandler(apiHandler, nil)
	r.Route("/v1", func(r chi.Router) {
		api.HandlerFromMux(apiStrictHandler, r)
	})

	testServer := httptest.NewServer(r)
	defer testServer.Close()

	tests := []struct {
		name           string
		path           string
		expectedStatus int
		description    string
	}{
		// Root-level health endpoints - should work
		{
			name:           "HealthCheck_AtRoot",
			path:           "/healthz",
			expectedStatus: http.StatusOK,
			description:    "Health check should be accessible at root level",
		},
		{
			name:           "ReadinessCheck_AtRoot",
			path:           "/healthz/ready",
			expectedStatus: http.StatusServiceUnavailable, // 503 when not ready initially
			description:    "Readiness check should be accessible at root level",
		},
		{
			name:           "LivenessCheck_AtRoot",
			path:           "/healthz/live",
			expectedStatus: http.StatusOK,
			description:    "Liveness check should be accessible at root level",
		},
		{
			name:           "Metrics_AtRoot",
			path:           "/metrics",
			expectedStatus: http.StatusOK,
			description:    "Prometheus metrics should be accessible at root level",
		},

		// Health endpoints should NOT exist under /v1
		{
			name:           "HealthCheck_NotUnderAPIv1",
			path:           "/v1/healthz",
			expectedStatus: http.StatusNotFound,
			description:    "Health check should NOT be duplicated under /v1",
		},
		{
			name:           "Metrics_NotUnderAPIv1",
			path:           "/v1/metrics",
			expectedStatus: http.StatusNotFound,
			description:    "Metrics should NOT be duplicated under /v1",
		},

		// API endpoints should NOT exist at root level
		{
			name:           "AgentRegister_NotAtRoot",
			path:           "/agent/register",
			expectedStatus: http.StatusNotFound,
			description:    "Agent register should NOT be accessible at root level",
		},
		{
			name:           "AgentClaim_NotAtRoot",
			path:           "/agent/claim",
			expectedStatus: http.StatusNotFound,
			description:    "Agent claim should NOT be accessible at root level",
		},

		// API endpoints should work under /v1 (with appropriate errors for bad requests)
		{
			name:           "AgentRegister_UnderAPIv1",
			path:           "/v1/agent/register",
			expectedStatus: http.StatusBadRequest, // Bad request due to missing body, but route exists
			description:    "Agent register should be accessible under /v1",
		},
		{
			name:           "AgentClaim_UnderAPIv1",
			path:           "/v1/agent/claim",
			expectedStatus: http.StatusBadRequest, // Bad request due to missing body, but route exists
			description:    "Agent claim should be accessible under /v1",
		},

		// API v1 root endpoint (not implemented as handler, returns 404)
		{
			name:           "APIv1_Root",
			path:           "/v1/",
			expectedStatus: http.StatusNotFound, // Not implemented in strict handler, only in custom route
			description:    "API v1 root returns 404 from strict handler",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, testServer.URL+tt.path, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			// For GET-only endpoints, use GET method
			if tt.path == "/healthz" || tt.path == "/healthz/ready" || tt.path == "/healthz/live" ||
				tt.path == "/metrics" || tt.path == "/v1/" ||
				tt.path == "/v1/healthz" || tt.path == "/v1/metrics" {
				req.Method = http.MethodGet
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("%s: Expected status %d, got %d",
					tt.description, tt.expectedStatus, resp.StatusCode)
			}
		})
	}
}

// TestRouteSeparation_NoConflicts verifies that there are no routing conflicts
// by checking that similar paths don't interfere with each other
func TestRouteSeparation_NoConflicts(t *testing.T) {
	log := logger.New(logger.Config{Level: "error", Format: "json"})
	db := testutil.NewMockDatabase()
	cfg := testutil.DefaultTestConfig()

	// Create router
	r := chi.NewRouter()
	r.Use(middleware.RequestID(log))
	r.Use(middleware.AgentAPIKeyAuth(log, db))
	r.Use(middleware.OAuth2Auth(log))

	metricsHandler := NewMetricsHandler(log, "test")

	// Register both handler groups
	healthHandler := NewHealthHandler(log, db, cfg, "test", metricsHandler)
	healthStrictHandler := healthAPI.NewStrictHandler(healthHandler, nil)
	healthAPI.HandlerFromMux(healthStrictHandler, r)

	apiHandler := NewAuthenticatedHandler(log, db, cfg, "test", metricsHandler)
	apiStrictHandler := api.NewStrictHandler(apiHandler, nil)
	r.Route("/v1", func(r chi.Router) {
		api.HandlerFromMux(apiStrictHandler, r)
	})

	testServer := httptest.NewServer(r)
	defer testServer.Close()

	// Test that both health and API endpoints work simultaneously
	t.Run("BothEndpointsAccessible", func(t *testing.T) {
		// Health endpoint
		healthReq, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, testServer.URL+"/healthz", nil)
		healthResp, err := http.DefaultClient.Do(healthReq)
		if err != nil {
			t.Fatalf("Health request failed: %v", err)
		}
		defer healthResp.Body.Close()

		if healthResp.StatusCode == http.StatusNotFound {
			t.Errorf("Health endpoint failed: got status %d", healthResp.StatusCode)
		}

		// API endpoint (will fail due to missing body, but route exists)
		apiReq, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, testServer.URL+"/v1/agent/register", nil)
		apiResp, err := http.DefaultClient.Do(apiReq)
		if err != nil {
			t.Fatalf("API request failed: %v", err)
		}
		defer apiResp.Body.Close()

		// Should get 400 (bad request) not 404 (not found) - proves route exists
		if apiResp.StatusCode == http.StatusNotFound {
			t.Error("API endpoint not found - routing conflict detected")
		}
	})
}

// TestHealthEndpoints_OnlyAtRoot verifies health endpoints are ONLY at root, not duplicated
func TestHealthEndpoints_OnlyAtRoot(t *testing.T) {
	log := logger.New(logger.Config{Level: "error", Format: "json"})
	db := testutil.NewMockDatabase()
	cfg := testutil.DefaultTestConfig()

	r := chi.NewRouter()
	metricsHandler := NewMetricsHandler(log, "test")

	// Only register health handler
	healthHandler := NewHealthHandler(log, db, cfg, "test", metricsHandler)
	healthHandler.SetReady(true) // Set ready so tests pass
	healthStrictHandler := healthAPI.NewStrictHandler(healthHandler, nil)
	healthAPI.HandlerFromMux(healthStrictHandler, r)

	testServer := httptest.NewServer(r)
	defer testServer.Close()

	paths := []string{"/healthz", "/healthz/ready", "/healthz/live", "/metrics"}

	for _, path := range paths {
		t.Run("RootLevel_"+path, func(t *testing.T) {
			req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, testServer.URL+path, nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("%s at root: expected 200, got %d", path, resp.StatusCode)
			}
		})

		t.Run("APIv1_"+path, func(t *testing.T) {
			req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, testServer.URL+"/v1"+path, nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusNotFound {
				t.Errorf("%s under /v1: expected 404, got %d (should not be duplicated)", path, resp.StatusCode)
			}
		})
	}
}
