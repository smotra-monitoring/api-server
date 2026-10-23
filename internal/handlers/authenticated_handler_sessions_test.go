package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	api "github.com/smotra-monitoring/server/internal/api/v1"
	"github.com/smotra-monitoring/server/internal/logger"
	"github.com/smotra-monitoring/server/internal/middleware"
	"github.com/smotra-monitoring/server/internal/testutil"
)

func newSessionTestHandler() *AuthenticatedHandler {
	log := logger.New(logger.Config{Level: "error", Format: "json"})
	return NewAuthenticatedHandler(log, testutil.NewMockDatabase(), testutil.DefaultTestConfig(), "test", NewMetricsHandler(log, "test"))
}

func TestAuthenticatedHandler_SessionEndpoints_RejectUnauthenticated(t *testing.T) {
	agentCtx := context.WithValue(context.Background(), middleware.AuthContextKey,
		&middleware.AuthInfo{AuthType: "agent_api_key", Authenticated: true, AgentID: "a"})
	noUserCtx := context.WithValue(context.Background(), middleware.AuthContextKey,
		&middleware.AuthInfo{AuthType: "oauth2", Authenticated: true})
	unauthCtx := context.WithValue(context.Background(), middleware.AuthContextKey,
		&middleware.AuthInfo{AuthType: "oauth2", Authenticated: false})

	contexts := map[string]context.Context{
		"no auth":       context.Background(),
		"unauthed info": unauthCtx,
		"agent api key": agentCtx,
		"missing user":  noUserCtx,
	}

	for name, ctx := range contexts {
		t.Run(name, func(t *testing.T) {
			h := newSessionTestHandler()

			if resp, err := h.ListSessions(ctx, api.ListSessionsRequestObject{}); err != nil {
				t.Fatal(err)
			} else if _, ok := resp.(api.ListSessions401JSONResponse); !ok {
				t.Errorf("ListSessions: expected 401, got %T", resp)
			}

			if resp, err := h.PurgeSessions(ctx, api.PurgeSessionsRequestObject{}); err != nil {
				t.Fatal(err)
			} else if _, ok := resp.(api.PurgeSessions401JSONResponse); !ok {
				t.Errorf("PurgeSessions: expected 401, got %T", resp)
			}

			req := api.PurgeSessionRequestObject{SessionId: uuid.Must(uuid.NewV7())}
			if resp, err := h.PurgeSession(ctx, req); err != nil {
				t.Fatal(err)
			} else if _, ok := resp.(api.PurgeSession401JSONResponse); !ok {
				t.Errorf("PurgeSession: expected 401, got %T", resp)
			}
		})
	}
}

func TestSessionRoutes_UnderV1Only(t *testing.T) {
	log := logger.New(logger.Config{Level: "error", Format: "json"})
	db := testutil.NewMockDatabase()
	cfg := testutil.DefaultTestConfig()

	r := chi.NewRouter()
	r.Use(middleware.ClientInfoMiddleware(nil))
	r.Use(middleware.AgentAPIKeyAuth(log, db))
	r.Use(middleware.OAuth2Auth(log, db))
	h := NewAuthenticatedHandler(log, db, cfg, "test", NewMetricsHandler(log, "test"))
	r.Route("/v1", func(r chi.Router) { api.HandlerFromMux(api.NewStrictHandler(h, nil), r) })

	id := uuid.Must(uuid.NewV7()).String()
	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/v1/sessions", http.StatusUnauthorized},
		{http.MethodDelete, "/v1/sessions", http.StatusUnauthorized},
		{http.MethodDelete, "/v1/sessions/" + id, http.StatusUnauthorized},
		{http.MethodGet, "/sessions", http.StatusNotFound},
		{http.MethodDelete, "/sessions/" + id, http.StatusNotFound},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			if w.Code != tc.want {
				t.Errorf("expected %d, got %d: %s", tc.want, w.Code, w.Body.String())
			}
		})
	}
}
