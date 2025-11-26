package health

import (
	"context"
	"sync"
	"time"

	"github.com/smotra-monitoring/server/internal/api"
	"github.com/smotra-monitoring/server/internal/database"
	"github.com/smotra-monitoring/server/internal/logger"
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
func (h *Handler) HealthCheck(ctx context.Context, request api.HealthCheckRequestObject) (api.HealthCheckResponseObject, error) {
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
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
		dbHealth, err := h.db.Health(checkCtx)
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
		return api.HealthCheck200JSONResponse(status), nil
	} else {
		status.Status = api.HealthStatusStatusUnhealthy
		return api.HealthCheck503JSONResponse(status), nil
	}
}

// ReadinessCheck implements the /healthz/ready endpoint
func (h *Handler) ReadinessCheck(ctx context.Context, request api.ReadinessCheckRequestObject) (api.ReadinessCheckResponseObject, error) {
	if !h.IsReady() {
		h.logger.Debug("readiness check failed: not ready")
		return api.ReadinessCheck503Response{}, nil
	}

	// Check if database is accessible
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if err := h.db.Ping(pingCtx); err != nil {
		h.logger.Warn("readiness check failed: database ping failed", "error", err)
		return api.ReadinessCheck503Response{}, nil
	}

	return api.ReadinessCheck200Response{}, nil
}

// LivenessCheck implements the /healthz/live endpoint
func (h *Handler) LivenessCheck(ctx context.Context, request api.LivenessCheckRequestObject) (api.LivenessCheckResponseObject, error) {
	// Simple liveness check - if we can respond, we're alive
	return api.LivenessCheck200Response{}, nil
}
