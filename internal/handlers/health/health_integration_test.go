package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	apiHealth "github.com/smotra-monitoring/server/internal/api/health"
	api "github.com/smotra-monitoring/server/internal/api/v1"
	"github.com/smotra-monitoring/server/internal/logger"
	"github.com/smotra-monitoring/server/internal/testutil"
)

// testServerImpl wraps Handler and implements the full StrictServerInterface
type testServerImpl struct {
	*Handler
}

// PrometheusMetrics provides a stub implementation for testing
func (t *testServerImpl) PrometheusMetrics(ctx context.Context, request apiHealth.PrometheusMetricsRequestObject) (apiHealth.PrometheusMetricsResponseObject, error) {
	// Stub implementation for testing - not used in health tests
	return apiHealth.PrometheusMetrics200TextResponse(""), nil
}

func (t *testServerImpl) GetAgentConfiguration(ctx context.Context, request api.GetAgentConfigurationRequestObject) (api.GetAgentConfigurationResponseObject, error) {
	// Stub implementation for testing - not used in health tests
	return api.GetAgentConfiguration404JSONResponse{
		NotFoundJSONResponse: api.NotFoundJSONResponse{
			Error:   "not_found",
			Message: "Stub - not implemented",
		},
	}, nil
}

func setupTestRouter(handler *Handler) *chi.Mux {
	testImpl := &testServerImpl{Handler: handler}
	r := chi.NewRouter()
	strictHandler := apiHealth.NewStrictHandler(testImpl, nil)
	apiHealth.HandlerFromMux(strictHandler, r)
	return r
}

func TestHealthEndpoints_Integration(t *testing.T) {
	log := logger.Default()
	db := testutil.NewMockDatabase()
	version := "1.0.0"

	handler := NewHandler(log, db, version)
	handler.SetReady(true)

	router := setupTestRouter(handler)
	server := httptest.NewServer(router)
	defer server.Close()

	t.Run("HealthCheck", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/healthz")
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		var status apiHealth.SystemStatus
		if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if status.Status != apiHealth.SystemHealthStatusHealthy {
			t.Errorf("Expected healthy status, got %s", status.Status)
		}
	})

	t.Run("ReadinessCheck", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/healthz/ready")
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("LivenessCheck", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/healthz/live")
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})
}

func TestHealthEndpoints_Integration_DatabaseFailure(t *testing.T) {
	log := logger.Default()
	db := testutil.NewMockDatabase()
	db.ShouldFail = true
	version := "1.0.0"

	handler := NewHandler(log, db, version)
	handler.SetReady(true)

	router := setupTestRouter(handler)
	server := httptest.NewServer(router)
	defer server.Close()

	t.Run("HealthCheck_Unhealthy", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/healthz")
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("Expected status 503, got %d", resp.StatusCode)
		}

		var status apiHealth.SystemStatus
		if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if status.Status != apiHealth.SystemHealthStatusUnhealthy {
			t.Errorf("Expected unhealthy status, got %s", status.Status)
		}
	})

	t.Run("ReadinessCheck_NotReady", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/healthz/ready")
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("Expected status 503, got %d", resp.StatusCode)
		}
	})

	t.Run("LivenessCheck_StillAlive", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/healthz/live")
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200 for liveness even with db failure, got %d", resp.StatusCode)
		}
	})
}

func TestHealthEndpoints_Integration_Headers(t *testing.T) {
	log := logger.Default()
	db := testutil.NewMockDatabase()

	handler := NewHandler(log, db, "1.0.0")
	handler.SetReady(true)

	router := setupTestRouter(handler)
	server := httptest.NewServer(router)
	defer server.Close()

	resp, err := http.Get(server.URL + "/healthz")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}
}
