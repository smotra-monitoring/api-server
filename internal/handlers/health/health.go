package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/smotra-monitoring/server/internal/database"
	"github.com/smotra-monitoring/server/internal/logger"
	"github.com/smotra-monitoring/server/pkg/api"
)

// Handler handles health check endpoints
type Handler struct {
	logger    *logger.Logger
	db        database.Database
	startTime time.Time
	version   string
	mu        sync.RWMutex
	ready     bool
}

// NewHandler creates a new health check handler
func NewHandler(logger *logger.Logger, db database.Database, version string) *Handler {
	return &Handler{
		logger:    logger.WithComponent("health"),
		db:        db,
		startTime: time.Now(),
		version:   version,
		ready:     false,
	}
}

// SetReady sets the readiness status
func (h *Handler) SetReady(ready bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ready = ready
}

// IsReady returns the readiness status
func (h *Handler) IsReady() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.ready
}

// HealthCheck implements the /healthz endpoint
func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	status := api.HealthStatus{
		Timestamp: time.Now(),
		Version:   &h.version,
	}

	uptime := int(time.Since(h.startTime).Seconds())
	status.UptimeSeconds = &uptime

	// Check components
	components := make(map[string]struct {
		Message        *string                          `json:"message,omitempty"`
		ResponseTimeMs *float32                         `json:"response_time_ms,omitempty"`
		Status         api.HealthStatusComponentsStatus `json:"status"`
	})
	overallHealthy := true

	// Check database
	if h.db != nil {
		dbHealth, err := h.db.Health(ctx)
		componentStatus := struct {
			Message        *string                          `json:"message,omitempty"`
			ResponseTimeMs *float32                         `json:"response_time_ms,omitempty"`
			Status         api.HealthStatusComponentsStatus `json:"status"`
		}{
			Status: api.HealthStatusComponentsStatusHealthy,
		}

		if err != nil {
			overallHealthy = false
			componentStatus.Status = api.HealthStatusComponentsStatusUnhealthy
			msg := err.Error()
			componentStatus.Message = &msg
		} else {
			msg := dbHealth.Message
			componentStatus.Message = &msg
			responseTime := float32(dbHealth.ResponseTime.Milliseconds())
			componentStatus.ResponseTimeMs = &responseTime
		}

		components["database"] = componentStatus
	}

	status.Components = &components

	// Set overall status
	if overallHealthy {
		status.Status = api.HealthStatusStatusHealthy
	} else {
		status.Status = api.HealthStatusStatusUnhealthy
	}

	// Write response
	statusCode := http.StatusOK
	if status.Status == api.HealthStatusStatusUnhealthy {
		statusCode = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(status); err != nil {
		h.logger.Error("failed to encode health status", "error", err)
	}
}

// ReadinessCheck implements the /healthz/ready endpoint
func (h *Handler) ReadinessCheck(w http.ResponseWriter, r *http.Request) {
	if !h.IsReady() {
		h.logger.Debug("readiness check failed: not ready")
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("not ready"))
		return
	}

	// Check if database is accessible
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		h.logger.Warn("readiness check failed: database ping failed", "error", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("database not ready"))
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ready"))
}

// LivenessCheck implements the /healthz/live endpoint
func (h *Handler) LivenessCheck(w http.ResponseWriter, r *http.Request) {
	// Simple liveness check - if we can respond, we're alive
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("alive"))
}
