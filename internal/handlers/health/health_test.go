package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	apiHealth "github.com/smotra-monitoring/server/internal/api/health"
	"github.com/smotra-monitoring/server/internal/logger"
	"github.com/smotra-monitoring/server/internal/testutil"
)

func TestNewHandler(t *testing.T) {
	log := logger.Default()
	db := testutil.NewMockDatabase()
	appVersion := "1.0.0"

	handler := NewHandler(log, db, appVersion)

	if handler == nil {
		t.Fatal("NewHandler returned nil")
	}

	if handler.appVersion != appVersion {
		t.Errorf("Expected version %s, got %s", appVersion, handler.appVersion)
	}

	if handler.IsReady() {
		t.Error("Expected handler to not be ready initially")
	}
}

func TestHandler_SetReady(t *testing.T) {
	log := logger.Default()
	db := testutil.NewMockDatabase()

	handler := NewHandler(log, db, "1.0.0")

	// Initially not ready
	if handler.IsReady() {
		t.Error("Expected handler to not be ready initially")
	}

	// Set ready
	handler.SetReady(true)
	if !handler.IsReady() {
		t.Error("Expected handler to be ready after SetReady(true)")
	}

	// Set not ready
	handler.SetReady(false)
	if handler.IsReady() {
		t.Error("Expected handler to not be ready after SetReady(false)")
	}
}

func TestHandler_HealthCheck_Healthy(t *testing.T) {
	log := logger.Default()
	db := testutil.NewMockDatabase()
	version := "1.0.0"

	handler := NewHandler(log, db, version)
	handler.SetReady(true)

	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()

	request := apiHealth.HealthCheckRequestObject{}
	response, err := handler.HealthCheck(req.Context(), request)
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}

	if err := response.VisitHealthCheckResponse(rec); err != nil {
		t.Fatalf("Failed to write response: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	var status apiHealth.HealthStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if status.Status != apiHealth.HealthStatusStatusHealthy {
		t.Errorf("Expected status healthy, got %s", status.Status)
	}

	if status.Version == nil || *status.Version != version {
		t.Errorf("Expected version %s, got %v", version, status.Version)
	}

	if status.UptimeSeconds == nil {
		t.Error("Expected uptime to be set")
	}
}

func TestHandler_HealthCheck_Unhealthy(t *testing.T) {
	log := logger.Default()
	db := testutil.NewMockDatabase()
	db.ShouldFail = true

	handler := NewHandler(log, db, "1.0.0")

	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()

	request := apiHealth.HealthCheckRequestObject{}
	response, err := handler.HealthCheck(req.Context(), request)
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}

	if err := response.VisitHealthCheckResponse(rec); err != nil {
		t.Fatalf("Failed to write response: %v", err)
	}

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", rec.Code)
	}

	var status apiHealth.HealthStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if status.Status != apiHealth.HealthStatusStatusUnhealthy {
		t.Errorf("Expected status unhealthy, got %s", status.Status)
	}
}

func TestHandler_HealthCheck_Components(t *testing.T) {
	log := logger.Default()
	db := testutil.NewMockDatabase()

	handler := NewHandler(log, db, "1.0.0")

	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()

	request := apiHealth.HealthCheckRequestObject{}
	response, err := handler.HealthCheck(req.Context(), request)
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}

	if err := response.VisitHealthCheckResponse(rec); err != nil {
		t.Fatalf("Failed to write response: %v", err)
	}

	var status apiHealth.HealthStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if status.Components == nil {
		t.Fatal("Expected components to be set")
	}

	components := *status.Components
	if _, ok := components["database"]; !ok {
		t.Error("Expected database component")
	}
}

func TestHandler_ReadinessCheck_Ready(t *testing.T) {
	log := logger.Default()
	db := testutil.NewMockDatabase()

	handler := NewHandler(log, db, "1.0.0")
	handler.SetReady(true)

	req := httptest.NewRequest("GET", "/healthz/ready", nil)
	rec := httptest.NewRecorder()

	request := apiHealth.ReadinessCheckRequestObject{}
	response, err := handler.ReadinessCheck(req.Context(), request)
	if err != nil {
		t.Fatalf("ReadinessCheck failed: %v", err)
	}

	if err := response.VisitReadinessCheckResponse(rec); err != nil {
		t.Fatalf("Failed to write response: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func TestHandler_ReadinessCheck_NotReady(t *testing.T) {
	log := logger.Default()
	db := testutil.NewMockDatabase()

	handler := NewHandler(log, db, "1.0.0")
	// Don't set ready

	req := httptest.NewRequest("GET", "/healthz/ready", nil)
	rec := httptest.NewRecorder()

	request := apiHealth.ReadinessCheckRequestObject{}
	response, err := handler.ReadinessCheck(req.Context(), request)
	if err != nil {
		t.Fatalf("ReadinessCheck failed: %v", err)
	}

	if err := response.VisitReadinessCheckResponse(rec); err != nil {
		t.Fatalf("Failed to write response: %v", err)
	}

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", rec.Code)
	}
}

func TestHandler_ReadinessCheck_DatabaseDown(t *testing.T) {
	log := logger.Default()
	db := testutil.NewMockDatabase()
	db.ShouldFail = true

	handler := NewHandler(log, db, "1.0.0")
	handler.SetReady(true) // Set ready, but database will fail

	req := httptest.NewRequest("GET", "/healthz/ready", nil)
	rec := httptest.NewRecorder()

	request := apiHealth.ReadinessCheckRequestObject{}
	response, err := handler.ReadinessCheck(req.Context(), request)
	if err != nil {
		t.Fatalf("ReadinessCheck failed: %v", err)
	}

	if err := response.VisitReadinessCheckResponse(rec); err != nil {
		t.Fatalf("Failed to write response: %v", err)
	}

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", rec.Code)
	}
}

func TestHandler_LivenessCheck(t *testing.T) {
	log := logger.Default()
	db := testutil.NewMockDatabase()

	handler := NewHandler(log, db, "1.0.0")

	req := httptest.NewRequest("GET", "/healthz/live", nil)
	rec := httptest.NewRecorder()

	request := apiHealth.LivenessCheckRequestObject{}
	response, err := handler.LivenessCheck(req.Context(), request)
	if err != nil {
		t.Fatalf("LivenessCheck failed: %v", err)
	}

	if err := response.VisitLivenessCheckResponse(rec); err != nil {
		t.Fatalf("Failed to write response: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func TestHandler_LivenessCheck_AlwaysSucceeds(t *testing.T) {
	log := logger.Default()
	db := testutil.NewMockDatabase()
	db.ShouldFail = true // Even with failing database

	handler := NewHandler(log, db, "1.0.0")

	req := httptest.NewRequest("GET", "/healthz/live", nil)
	rec := httptest.NewRecorder()

	request := apiHealth.LivenessCheckRequestObject{}
	response, err := handler.LivenessCheck(req.Context(), request)
	if err != nil {
		t.Fatalf("LivenessCheck failed: %v", err)
	}

	if err := response.VisitLivenessCheckResponse(rec); err != nil {
		t.Fatalf("Failed to write response: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200 even with failing database, got %d", rec.Code)
	}
}

func TestHandler_HealthCheck_ContentType(t *testing.T) {
	log := logger.Default()
	db := testutil.NewMockDatabase()

	handler := NewHandler(log, db, "1.0.0")

	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()

	request := apiHealth.HealthCheckRequestObject{}
	response, err := handler.HealthCheck(req.Context(), request)
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}

	if err := response.VisitHealthCheckResponse(rec); err != nil {
		t.Fatalf("Failed to write response: %v", err)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}
}

func TestHandler_HealthCheck_Uptime(t *testing.T) {
	log := logger.Default()
	db := testutil.NewMockDatabase()

	handler := NewHandler(log, db, "1.0.0")

	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()

	request := apiHealth.HealthCheckRequestObject{}
	response, err := handler.HealthCheck(req.Context(), request)
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}

	if err := response.VisitHealthCheckResponse(rec); err != nil {
		t.Fatalf("Failed to write response: %v", err)
	}

	var status apiHealth.HealthStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if status.UptimeSeconds == nil {
		t.Fatal("Expected uptime to be set")
	}

	if *status.UptimeSeconds < 0 {
		t.Error("Uptime should not be negative")
	}
}
