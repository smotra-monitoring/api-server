package health

import (
	"context"
	"sync"
	"time"

	apiHealth "github.com/smotra-monitoring/server/internal/api/health"
	"github.com/smotra-monitoring/server/internal/database"
	"github.com/smotra-monitoring/server/internal/logger"
)

// Handler handles health check endpoints
type Handler struct {
	logger     *logger.Logger
	db         database.Database
	startTime  time.Time
	appVersion string
	mu         sync.RWMutex
	ready      bool
}

// NewHandler creates a new health check handler
func NewHandler(logger *logger.Logger, db database.Database, appVersion string) *Handler {
	return &Handler{
		logger:     logger.WithComponent("health"),
		db:         db,
		startTime:  time.Now(),
		appVersion: appVersion,
		ready:      false,
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
func (h *Handler) HealthCheck(ctx context.Context, request apiHealth.HealthCheckRequestObject) (apiHealth.HealthCheckResponseObject, error) {
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	status := apiHealth.SystemStatus{
		Timestamp: time.Now(),
		Version:   &h.appVersion,
	}

	uptime := int(time.Since(h.startTime).Seconds())
	status.UptimeSeconds = &uptime

	// Check components
	components := apiHealth.ComponentsStatus(make(map[string]apiHealth.ComponentStatus))
	overallHealthy := true

	// Check database
	if h.db != nil {
		dbHealth, err := h.db.Health(checkCtx)
		var componentStatus apiHealth.ComponentStatus
		componentStatus.Status = apiHealth.ComponentHealthStatusHealthy

		if err != nil {
			overallHealthy = false
			componentStatus.Status = apiHealth.ComponentHealthStatusUnhealthy
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
		status.Status = apiHealth.SystemHealthStatusHealthy
		return apiHealth.HealthCheck200JSONResponse(status), nil
	} else {
		status.Status = apiHealth.SystemHealthStatusUnhealthy
		return apiHealth.HealthCheck503JSONResponse(status), nil
	}
}

// ReadinessCheck implements the /healthz/ready endpoint
func (h *Handler) ReadinessCheck(ctx context.Context, request apiHealth.ReadinessCheckRequestObject) (apiHealth.ReadinessCheckResponseObject, error) {
	if !h.IsReady() {
		h.logger.Debug("readiness check failed: not ready")
		return apiHealth.ReadinessCheck503Response{}, nil
	}

	// Check if database is accessible
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if err := h.db.Ping(pingCtx); err != nil {
		h.logger.Warn("readiness check failed: database ping failed", "error", err)
		return apiHealth.ReadinessCheck503Response{}, nil
	}

	return apiHealth.ReadinessCheck200Response{}, nil
}

// LivenessCheck implements the /healthz/live endpoint
func (h *Handler) LivenessCheck(ctx context.Context, request apiHealth.LivenessCheckRequestObject) (apiHealth.LivenessCheckResponseObject, error) {
	// Simple liveness check - if we can respond, we're alive
	return apiHealth.LivenessCheck200Response{}, nil
}
