package agent_list

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sync/atomic"

	"github.com/google/uuid"
	api "github.com/smotra-monitoring/server/internal/api/v1"
	"github.com/smotra-monitoring/server/internal/database"
	"github.com/smotra-monitoring/server/internal/database/queries"
	"github.com/smotra-monitoring/server/internal/logger"
	"github.com/smotra-monitoring/server/internal/middleware"
)

const (
	defaultPage     = 1
	defaultPageSize = 20
	maxPageSize     = 100
	minPageSize     = 1
)

// Handler handles requests to list agents for an authenticated user's tenant.
type Handler struct {
	logger *logger.Logger
	db     database.Database

	// Metrics
	listAttemptsTotal atomic.Uint64
	listSuccessTotal  atomic.Uint64
	listFailureTotal  atomic.Uint64
}

// NewHandler creates a new agent list handler.
func NewHandler(log *logger.Logger, db database.Database) *Handler {
	return &Handler{
		logger: log.WithComponent("agent_list"),
		db:     db,
	}
}

// Handle returns a paginated list of agents belonging to the authenticated user's tenant.
func (h *Handler) Handle(ctx context.Context, req api.ListAgentsRequestObject) (api.ListAgentsResponseObject, error) {
	h.listAttemptsTotal.Add(1)

	// Auth is guaranteed by AuthenticatedHandler; cast directly.
	authInfo := ctx.Value(middleware.AuthContextKey).(*middleware.AuthInfo)
	userID := authInfo.UserID

	q := queries.New(h.db.DB())

	// Resolve tenant ID from user.
	user, err := q.GetUserByID(ctx, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			h.listFailureTotal.Add(1)
			h.logger.WarnContext(ctx, "User not found for agent list", slog.String("user_id", userID))
			return api.ListAgents401JSONResponse{
				UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
					Error:   "unauthorized",
					Message: "User not found",
				},
			}, nil
		}
		h.listFailureTotal.Add(1)
		h.logger.ErrorContext(ctx, "Failed to retrieve user", slog.String("user_id", userID), slog.String("error", err.Error()))
		return api.ListAgents500JSONResponse{
			InternalServerErrorJSONResponse: api.InternalServerErrorJSONResponse{
				Error:   "database_error",
				Message: "Failed to retrieve user information",
			},
		}, nil
	}

	tenantID := user.TenantID

	// Parse and clamp pagination parameters.
	page := defaultPage
	if req.Params.Page != nil && *req.Params.Page >= 1 {
		page = *req.Params.Page
	}

	pageSize := defaultPageSize
	if req.Params.PageSize != nil {
		switch {
		case *req.Params.PageSize < minPageSize:
			pageSize = minPageSize
		case *req.Params.PageSize > maxPageSize:
			pageSize = maxPageSize
		default:
			pageSize = *req.Params.PageSize
		}
	}

	offset := int64((page - 1) * pageSize)
	limit := int64(pageSize)

	// Count total agents for pagination metadata.
	totalItems, err := q.CountAgentsByTenant(ctx, tenantID)
	if err != nil {
		h.listFailureTotal.Add(1)
		h.logger.ErrorContext(ctx, "Failed to count agents", slog.String("tenant_id", tenantID), slog.String("error", err.Error()))
		return api.ListAgents500JSONResponse{
			InternalServerErrorJSONResponse: api.InternalServerErrorJSONResponse{
				Error:   "database_error",
				Message: "Failed to count agents",
			},
		}, nil
	}

	// Fetch agents for the current page.
	rows, err := q.ListAgentsByTenant(ctx, queries.ListAgentsByTenantParams{
		TenantID: tenantID,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		h.listFailureTotal.Add(1)
		h.logger.ErrorContext(ctx, "Failed to list agents", slog.String("tenant_id", tenantID), slog.String("error", err.Error()))
		return api.ListAgents500JSONResponse{
			InternalServerErrorJSONResponse: api.InternalServerErrorJSONResponse{
				Error:   "database_error",
				Message: "Failed to retrieve agents",
			},
		}, nil
	}

	// Build response items.
	items := make([]api.AgentListItem, 0, len(rows))
	for _, row := range rows {
		item := h.rowToItem(row)
		items = append(items, item)
	}

	totalPages := int(math.Ceil(float64(totalItems) / float64(pageSize)))
	if totalPages < 1 {
		totalPages = 1
	}
	hasNext := page < totalPages
	hasPrev := page > 1

	h.listSuccessTotal.Add(1)
	return api.ListAgents200JSONResponse{
		Agents: items,
		Pagination: api.Pagination{
			Page:        page,
			PageSize:    pageSize,
			TotalItems:  int(totalItems),
			TotalPages:  totalPages,
			HasNext:     &hasNext,
			HasPrevious: &hasPrev,
		},
	}, nil
}

// rowToItem converts a database row to an API response item.
func (h *Handler) rowToItem(row queries.ListAgentsByTenantRow) api.AgentListItem {
	item := api.AgentListItem{
		Id:            api.UUIDv7(uuid.MustParse(row.ID)),
		SectionId:     row.SectionID,
		Name:          row.Name,
		ConfigVersion: int(row.ConfigVersion),
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}

	if row.AgentVersion.Valid {
		item.AgentVersion = &row.AgentVersion.String
	}

	if row.LastSeenAt.Valid {
		item.LastSeenAt = &row.LastSeenAt.Time
	}

	if row.LastResultSubmittedAt.Valid {
		item.LastResultSubmittedAt = &row.LastResultSubmittedAt.Time
	}

	// Parse ip_addresses_json into structured slice; silently skip on error.
	if row.IpAddressesJson != "" && row.IpAddressesJson != "[]" {
		var ifaces []api.AgentNetworkInterface
		if err := json.Unmarshal([]byte(row.IpAddressesJson), &ifaces); err == nil && len(ifaces) > 0 {
			item.IpAddresses = &ifaces
		}
	}

	return item
}

// GetMetrics returns current handler metrics in Prometheus format.
func (h *Handler) GetMetrics() string {
	out := ""

	out += "# HELP smotra_agent_list_attempts_total Total requests to list agents\n"
	out += "# TYPE smotra_agent_list_attempts_total counter\n"
	out += fmt.Sprintf("smotra_agent_list_attempts_total %d\n", h.listAttemptsTotal.Load())
	out += "\n"

	out += "# HELP smotra_agent_list_success_total Successful agent list responses\n"
	out += "# TYPE smotra_agent_list_success_total counter\n"
	out += fmt.Sprintf("smotra_agent_list_success_total %d\n", h.listSuccessTotal.Load())
	out += "\n"

	out += "# HELP smotra_agent_list_failure_total Failed agent list responses\n"
	out += "# TYPE smotra_agent_list_failure_total counter\n"
	out += fmt.Sprintf("smotra_agent_list_failure_total %d\n", h.listFailureTotal.Load())

	return out
}
