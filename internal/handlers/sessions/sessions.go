package sessions

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	api "github.com/smotra-monitoring/server/internal/api/v1"
	"github.com/smotra-monitoring/server/internal/database"
	"github.com/smotra-monitoring/server/internal/database/queries"
	"github.com/smotra-monitoring/server/internal/logger"
	"github.com/smotra-monitoring/server/internal/middleware"
)

// Handler implements listing and purging of the authenticated user's sessions.
type Handler struct {
	logger *logger.Logger
	db     database.Database

	// Metrics
	listTotal         atomic.Uint64
	listSuccessTotal  atomic.Uint64
	listFailureTotal  atomic.Uint64
	purgeTotal        atomic.Uint64
	purgeSuccessTotal atomic.Uint64
	purgeFailureTotal atomic.Uint64
	purgeBadRequest   atomic.Uint64
	purgeForbidden    atomic.Uint64
	purgeNotFound     atomic.Uint64
	purgeAllTotal     atomic.Uint64
	purgeAllSuccess   atomic.Uint64
	purgeAllFailure   atomic.Uint64
	revokedTotal      atomic.Uint64
}

// NewHandler creates a new sessions handler.
func NewHandler(log *logger.Logger, db database.Database) *Handler {
	return &Handler{
		logger: log.WithComponent("sessions"),
		db:     db,
	}
}

func internalError(msg string) api.InternalServerErrorJSONResponse {
	return api.InternalServerErrorJSONResponse{Error: "internal_server_error", Message: msg}
}

// ListSessions returns the caller's active sessions.
func (h *Handler) ListSessions(ctx context.Context, _ api.ListSessionsRequestObject) (api.ListSessionsResponseObject, error) {
	h.listTotal.Add(1)

	// Auth is guaranteed by the AuthenticatedHandler wrapper.
	authInfo := ctx.Value(middleware.AuthContextKey).(*middleware.AuthInfo)
	client := middleware.ClientInfoFromContext(ctx)

	rows, err := queries.New(h.db.DB()).ListActiveSessionsByUserID(ctx, authInfo.UserID)
	if err != nil {
		h.listFailureTotal.Add(1)
		h.logger.ErrorContext(ctx, "failed to list sessions",
			slog.String("user_id", authInfo.UserID), slog.String("error", err.Error()))
		return api.ListSessions500JSONResponse{InternalServerErrorJSONResponse: internalError("Failed to list sessions")}, nil
	}

	items := make([]api.Session, 0, len(rows))
	for _, row := range rows {
		id, err := uuid.Parse(row.ID)
		if err != nil {
			h.logger.WarnContext(ctx, "skipping session with malformed id", slog.String("session_id", row.ID))
			continue
		}
		item := api.Session{
			Id:         id,
			CreatedAt:  row.CreatedAt,
			LastUsedAt: row.LastUsedAt,
			ExpiresAt:  row.ExpiresAt,
			Provider:   row.Oauth2Provider,
			IsCurrent:  row.ID == authInfo.SessionID,
		}
		if row.IpAddress.Valid {
			item.IpAddress = &row.IpAddress.String
		}
		if row.UserAgent.Valid {
			item.UserAgent = &row.UserAgent.String
		}
		items = append(items, item)
	}

	h.listSuccessTotal.Add(1)
	h.logger.InfoContext(ctx, "sessions listed",
		slog.String("user_id", authInfo.UserID),
		slog.String("session_id", authInfo.SessionID),
		slog.String("ip_address", client.IPAddress),
		slog.Int("count", len(items)),
	)
	return api.ListSessions200JSONResponse{Sessions: items}, nil
}

// PurgeSession revokes a single session owned by the caller.
func (h *Handler) PurgeSession(ctx context.Context, req api.PurgeSessionRequestObject) (api.PurgeSessionResponseObject, error) {
	h.purgeTotal.Add(1)

	authInfo := ctx.Value(middleware.AuthContextKey).(*middleware.AuthInfo)
	client := middleware.ClientInfoFromContext(ctx)

	if req.SessionId == uuid.Nil {
		h.purgeBadRequest.Add(1)
		return api.PurgeSession400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
			Error: "bad_request", Message: "session_id must be a valid UUID",
		}}, nil
	}
	sessionID := req.SessionId.String()

	q := queries.New(h.db.DB())
	session, err := q.GetSessionByID(ctx, sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return h.purgeNotFoundResponse(ctx, authInfo, client, sessionID), nil
	}
	if err != nil {
		h.purgeFailureTotal.Add(1)
		h.logger.ErrorContext(ctx, "failed to load session for purge",
			slog.String("user_id", authInfo.UserID), slog.String("session_id", sessionID), slog.String("error", err.Error()))
		return api.PurgeSession500JSONResponse{InternalServerErrorJSONResponse: internalError("Failed to purge session")}, nil
	}

	if session.UserID != authInfo.UserID {
		h.purgeForbidden.Add(1)
		h.logger.WarnContext(ctx, "session purge denied: not the owner",
			slog.String("user_id", authInfo.UserID),
			slog.String("session_id", sessionID),
			slog.String("ip_address", client.IPAddress),
		)
		return api.PurgeSession403JSONResponse{ForbiddenJSONResponse: api.ForbiddenJSONResponse{
			Error: "forbidden", Message: "Session does not belong to the authenticated user",
		}}, nil
	}

	now := time.Now().UTC()
	if session.Revoked != 0 || !session.SlidingExpiresAt.After(now) || !session.ExpiresAt.After(now) {
		return h.purgeNotFoundResponse(ctx, authInfo, client, sessionID), nil
	}

	rows, err := q.RevokeSessionByIDForUser(ctx, queries.RevokeSessionByIDForUserParams{ID: sessionID, UserID: authInfo.UserID})
	if err != nil {
		h.purgeFailureTotal.Add(1)
		h.logger.ErrorContext(ctx, "failed to revoke session",
			slog.String("user_id", authInfo.UserID), slog.String("session_id", sessionID), slog.String("error", err.Error()))
		return api.PurgeSession500JSONResponse{InternalServerErrorJSONResponse: internalError("Failed to purge session")}, nil
	}
	if rows == 0 {
		return h.purgeNotFoundResponse(ctx, authInfo, client, sessionID), nil
	}

	h.purgeSuccessTotal.Add(1)
	h.revokedTotal.Add(1)
	h.logger.InfoContext(ctx, "session purged",
		slog.String("user_id", authInfo.UserID),
		slog.String("session_id", sessionID),
		slog.Bool("is_current", sessionID == authInfo.SessionID),
		slog.String("ip_address", client.IPAddress),
	)
	return api.PurgeSession204Response{}, nil
}

func (h *Handler) purgeNotFoundResponse(ctx context.Context, authInfo *middleware.AuthInfo, client middleware.ClientInfo, sessionID string) api.PurgeSessionResponseObject {
	h.purgeNotFound.Add(1)
	h.logger.InfoContext(ctx, "session purge: session not found or inactive",
		slog.String("user_id", authInfo.UserID),
		slog.String("session_id", sessionID),
		slog.String("ip_address", client.IPAddress),
	)
	return api.PurgeSession404JSONResponse{NotFoundJSONResponse: api.NotFoundJSONResponse{
		Error: "not_found", Message: "Session not found",
	}}
}

// PurgeSessions revokes every active session of the caller except the current one.
func (h *Handler) PurgeSessions(ctx context.Context, _ api.PurgeSessionsRequestObject) (api.PurgeSessionsResponseObject, error) {
	h.purgeAllTotal.Add(1)

	authInfo := ctx.Value(middleware.AuthContextKey).(*middleware.AuthInfo)
	client := middleware.ClientInfoFromContext(ctx)

	count, err := queries.New(h.db.DB()).RevokeOtherSessionsForUser(ctx, queries.RevokeOtherSessionsForUserParams{
		UserID: authInfo.UserID,
		ID:     authInfo.SessionID,
	})
	if err != nil {
		h.purgeAllFailure.Add(1)
		h.logger.ErrorContext(ctx, "failed to purge sessions",
			slog.String("user_id", authInfo.UserID), slog.String("error", err.Error()))
		return api.PurgeSessions500JSONResponse{InternalServerErrorJSONResponse: internalError("Failed to purge sessions")}, nil
	}

	h.purgeAllSuccess.Add(1)
	h.revokedTotal.Add(uint64(count))
	h.logger.InfoContext(ctx, "sessions purged",
		slog.String("user_id", authInfo.UserID),
		slog.String("kept_session_id", authInfo.SessionID),
		slog.Int64("revoked_count", count),
		slog.String("ip_address", client.IPAddress),
	)
	return api.PurgeSessions200JSONResponse{RevokedCount: int(count)}, nil
}

// GetMetrics returns handler metrics in Prometheus format.
func (h *Handler) GetMetrics() string {
	out := ""
	counter := func(name, help string, v uint64) {
		out += fmt.Sprintf("# HELP %s %s\n# TYPE %s counter\n%s %d\n\n", name, help, name, name, v)
	}

	counter("smotra_sessions_list_total", "Total requests to list sessions", h.listTotal.Load())
	counter("smotra_sessions_list_success_total", "Successful session list requests", h.listSuccessTotal.Load())
	counter("smotra_sessions_list_failure_total", "Failed session list requests", h.listFailureTotal.Load())
	counter("smotra_sessions_purge_total", "Total requests to purge a single session", h.purgeTotal.Load())
	counter("smotra_sessions_purge_success_total", "Successful single session purges", h.purgeSuccessTotal.Load())
	counter("smotra_sessions_purge_failure_total", "Single session purges failed due to server errors", h.purgeFailureTotal.Load())
	counter("smotra_sessions_purge_bad_request_total", "Single session purges rejected as invalid", h.purgeBadRequest.Load())
	counter("smotra_sessions_purge_forbidden_total", "Single session purges denied because the session belongs to another user", h.purgeForbidden.Load())
	counter("smotra_sessions_purge_not_found_total", "Single session purges for unknown or inactive sessions", h.purgeNotFound.Load())
	counter("smotra_sessions_purge_all_total", "Total requests to purge all other sessions", h.purgeAllTotal.Load())
	counter("smotra_sessions_purge_all_success_total", "Successful purge-all requests", h.purgeAllSuccess.Load())
	counter("smotra_sessions_purge_all_failure_total", "Failed purge-all requests", h.purgeAllFailure.Load())
	counter("smotra_sessions_revoked_total", "Total sessions revoked via the sessions API", h.revokedTotal.Load())
	return out
}
