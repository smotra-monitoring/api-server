package sessions

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	api "github.com/smotra-monitoring/server/internal/api/v1"
	"github.com/smotra-monitoring/server/internal/database"
	"github.com/smotra-monitoring/server/internal/logger"
	"github.com/smotra-monitoring/server/internal/middleware"
)

func listReq() api.ListSessionsRequestObject      { return api.ListSessionsRequestObject{} }
func purgeAllReq() api.PurgeSessionsRequestObject { return api.PurgeSessionsRequestObject{} }
func purgeReq(id string) api.PurgeSessionRequestObject {
	return api.PurgeSessionRequestObject{SessionId: uuid.MustParse(id)}
}

// testServerImpl implements the strict interface; only the session endpoints are real.
// It rejects unauthenticated calls the way AuthenticatedHandler does in production.
type testServerImpl struct {
	api.StrictServerInterface
	h *Handler
}

func authed(ctx context.Context) bool {
	info, ok := ctx.Value(middleware.AuthContextKey).(*middleware.AuthInfo)
	return ok && info.Authenticated && info.AuthType == "oauth2"
}

var unauthorized = api.UnauthorizedJSONResponse{Error: "unauthorized", Message: "Valid session required"}

func (s *testServerImpl) ListSessions(ctx context.Context, r api.ListSessionsRequestObject) (api.ListSessionsResponseObject, error) {
	if !authed(ctx) {
		return api.ListSessions401JSONResponse{UnauthorizedJSONResponse: unauthorized}, nil
	}
	return s.h.ListSessions(ctx, r)
}

func (s *testServerImpl) PurgeSessions(ctx context.Context, r api.PurgeSessionsRequestObject) (api.PurgeSessionsResponseObject, error) {
	if !authed(ctx) {
		return api.PurgeSessions401JSONResponse{UnauthorizedJSONResponse: unauthorized}, nil
	}
	return s.h.PurgeSessions(ctx, r)
}

func (s *testServerImpl) PurgeSession(ctx context.Context, r api.PurgeSessionRequestObject) (api.PurgeSessionResponseObject, error) {
	if !authed(ctx) {
		return api.PurgeSession401JSONResponse{UnauthorizedJSONResponse: unauthorized}, nil
	}
	return s.h.PurgeSession(ctx, r)
}

func newRouter(db database.Database) *chi.Mux {
	log := logger.Default()
	r := chi.NewRouter()
	r.Use(middleware.ClientInfoMiddleware(nil))
	r.Use(middleware.OAuth2Auth(log, db))
	api.HandlerFromMux(api.NewStrictHandler(&testServerImpl{h: NewHandler(log, db)}, nil), r)
	return r
}

func do(r http.Handler, method, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func skipShort(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
}

func TestListSessions_Integration_OnlyOwnActiveSessions(t *testing.T) {
	skipShort(t)
	db, userA, userB := newTestDB(t)
	router := newRouter(db)

	current := seed(t, db, seedSession{userID: userA, ip: "203.0.113.7", ua: "Firefox"})
	other := seed(t, db, seedSession{userID: userA})
	seed(t, db, seedSession{userID: userA, revoked: true})
	seed(t, db, seedSession{userID: userA, expired: true})
	seed(t, db, seedSession{userID: userB})

	w := do(router, http.MethodGet, "/sessions", current.token)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp api.SessionListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(resp.Sessions))
	}

	byID := map[string]api.Session{}
	for _, s := range resp.Sessions {
		byID[s.Id.String()] = s
	}
	cur, ok := byID[current.id]
	if !ok || !cur.IsCurrent {
		t.Errorf("current session missing or not flagged is_current: %+v", cur)
	}
	if cur.IpAddress == nil || *cur.IpAddress != "203.0.113.7" || cur.UserAgent == nil || *cur.UserAgent != "Firefox" {
		t.Errorf("ip/user agent not returned: %+v", cur)
	}
	if o, ok := byID[other.id]; !ok || o.IsCurrent {
		t.Errorf("other session missing or wrongly flagged current: %+v", o)
	}
}

func TestListSessions_Integration_Unauthenticated(t *testing.T) {
	skipShort(t)
	db, _, _ := newTestDB(t)
	if w := do(newRouter(db), http.MethodGet, "/sessions", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestPurgeSession_Integration_OwnSession(t *testing.T) {
	skipShort(t)
	db, userA, _ := newTestDB(t)
	router := newRouter(db)

	current := seed(t, db, seedSession{userID: userA})
	other := seed(t, db, seedSession{userID: userA})

	if w := do(router, http.MethodDelete, "/sessions/"+other.id, current.token); w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if isActive(t, db, other.id) {
		t.Error("session should be revoked")
	}
	if !isActive(t, db, current.id) {
		t.Error("current session should be untouched")
	}
	// The purged token no longer authenticates.
	if w := do(router, http.MethodGet, "/sessions", other.token); w.Code != http.StatusUnauthorized {
		t.Errorf("purged token should be rejected, got %d", w.Code)
	}
	// Purging again reports not found.
	if w := do(router, http.MethodDelete, "/sessions/"+other.id, current.token); w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for already purged session, got %d", w.Code)
	}
}

func TestPurgeSession_Integration_CurrentSessionAllowed(t *testing.T) {
	skipShort(t)
	db, userA, _ := newTestDB(t)
	router := newRouter(db)
	current := seed(t, db, seedSession{userID: userA})

	if w := do(router, http.MethodDelete, "/sessions/"+current.id, current.token); w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
	if isActive(t, db, current.id) {
		t.Error("current session should be revoked")
	}
}

func TestPurgeSession_Integration_OtherUsersSessionForbidden(t *testing.T) {
	skipShort(t)
	db, userA, userB := newTestDB(t)
	router := newRouter(db)
	current := seed(t, db, seedSession{userID: userA})
	foreign := seed(t, db, seedSession{userID: userB})

	if w := do(router, http.MethodDelete, "/sessions/"+foreign.id, current.token); w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
	if !isActive(t, db, foreign.id) {
		t.Error("another user's session must not be revoked")
	}
}

func TestPurgeSession_Integration_InvalidAndUnknownID(t *testing.T) {
	skipShort(t)
	db, userA, _ := newTestDB(t)
	router := newRouter(db)
	current := seed(t, db, seedSession{userID: userA})

	for _, tc := range []struct {
		name, id string
		want     int
	}{
		{"not a uuid", "not-a-uuid", http.StatusBadRequest},
		{"nil uuid", uuid.Nil.String(), http.StatusBadRequest},
		{"unknown uuid", uuid.Must(uuid.NewV7()).String(), http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if w := do(router, http.MethodDelete, "/sessions/"+tc.id, current.token); w.Code != tc.want {
				t.Errorf("expected %d, got %d: %s", tc.want, w.Code, w.Body.String())
			}
		})
	}
}

func TestPurgeSession_Integration_Unauthenticated(t *testing.T) {
	skipShort(t)
	db, userA, _ := newTestDB(t)
	s := seed(t, db, seedSession{userID: userA})

	if w := do(newRouter(db), http.MethodDelete, "/sessions/"+s.id, ""); w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
	if !isActive(t, db, s.id) {
		t.Error("session must not be revoked by an unauthenticated request")
	}
}

func TestPurgeSessions_Integration_KeepsCurrentAndOtherUsers(t *testing.T) {
	skipShort(t)
	db, userA, userB := newTestDB(t)
	router := newRouter(db)

	current := seed(t, db, seedSession{userID: userA})
	a2 := seed(t, db, seedSession{userID: userA})
	a3 := seed(t, db, seedSession{userID: userA})
	foreign := seed(t, db, seedSession{userID: userB})

	w := do(router, http.MethodDelete, "/sessions", current.token)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp api.PurgeSessionsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.RevokedCount != 2 {
		t.Errorf("expected revoked_count=2, got %d", resp.RevokedCount)
	}
	if !isActive(t, db, current.id) || !isActive(t, db, foreign.id) {
		t.Error("current session and other users' sessions must stay active")
	}
	if isActive(t, db, a2.id) || isActive(t, db, a3.id) {
		t.Error("other sessions of the user should be revoked")
	}
}

func TestPurgeSessions_Integration_Unauthenticated(t *testing.T) {
	skipShort(t)
	db, _, _ := newTestDB(t)
	if w := do(newRouter(db), http.MethodDelete, "/sessions", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}
