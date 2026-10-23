package auth_test

import (
	"context"
	"database/sql"
	"net/http"
	"testing"

	api "github.com/smotra-monitoring/server/internal/api/v1"
	"github.com/smotra-monitoring/server/internal/database/queries"
	"github.com/smotra-monitoring/server/internal/handlers/auth"
	"github.com/smotra-monitoring/server/internal/middleware"
	"github.com/smotra-monitoring/server/internal/testutil"
)

func clientCtx(ip, ua string) context.Context {
	return context.WithValue(context.Background(), middleware.ClientInfoContextKey,
		middleware.ClientInfo{IPAddress: ip, UserAgent: ua})
}

// TestOauth2Token_StoresClientInfo verifies that the client IP and User-Agent are
// persisted on the session, and that AuthRefresh records the refreshing client.
func TestOauth2Token_StoresClientInfo(t *testing.T) {
	stub := newStubTokenAndUserinfoServer(t, map[string]any{"sub": "user-ci", "name": "Carol"}, http.StatusOK)
	defer stub.Close()

	log, _ := testutil.NewTestLogger()
	db := testutil.SetupTestSQLiteDB(t)
	testutil.ApplyMigrations(t, context.Background(), db.DB(), "../../../data/db/dev/migrations")
	cfg := testutil.DefaultTestConfig()
	h := auth.NewHandlerForTesting(log, newTestAuthConfig(stub.URL), &cfg.Server, db)

	setupPendingAuthCode(t, h, "teststatic", "ci-code", "ci-state")
	resp, err := h.Oauth2Token(clientCtx("203.0.113.7", "TestBrowser/1.0"), api.Oauth2TokenRequestObject{
		Body: &api.Oauth2TokenFormdataRequestBody{
			GrantType:    api.AuthorizationCode,
			Code:         "ci-code",
			RedirectUri:  "http://server.test/v1/auth/oauth2/callback",
			CodeVerifier: "verifier",
		},
	})
	if err != nil {
		t.Fatalf("Oauth2Token: %v", err)
	}
	if _, ok := resp.(api.Oauth2Token200JSONResponse); !ok {
		t.Fatalf("expected 200, got %T", resp)
	}

	q := queries.New(db.DB())
	var sessionID string
	if err := db.DB().QueryRow(`SELECT id FROM sessions`).Scan(&sessionID); err != nil {
		t.Fatalf("load session id: %v", err)
	}
	session, err := q.GetSessionByID(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSessionByID: %v", err)
	}
	if session.IpAddress != (sql.NullString{String: "203.0.113.7", Valid: true}) {
		t.Errorf("ip_address = %+v", session.IpAddress)
	}
	if session.UserAgent != (sql.NullString{String: "TestBrowser/1.0", Valid: true}) {
		t.Errorf("user_agent = %+v", session.UserAgent)
	}

	// Refresh from a different client: the new session records the new client.
	refreshCtx := context.WithValue(clientCtx("198.51.100.9", "Other/2.0"), middleware.AuthContextKey,
		&middleware.AuthInfo{AuthType: "oauth2", Authenticated: true, UserID: session.UserID, SessionID: sessionID})
	if _, err := h.AuthRefresh(refreshCtx, api.AuthRefreshRequestObject{}); err != nil {
		t.Fatalf("AuthRefresh: %v", err)
	}

	var newIP, newUA sql.NullString
	if err := db.DB().QueryRow(`SELECT ip_address, user_agent FROM sessions WHERE revoked = 0`).Scan(&newIP, &newUA); err != nil {
		t.Fatalf("load refreshed session: %v", err)
	}
	if newIP.String != "198.51.100.9" || newUA.String != "Other/2.0" {
		t.Errorf("refreshed session client info = %q / %q", newIP.String, newUA.String)
	}
}

func TestOauth2Token_WithoutClientInfo_Succeeds(t *testing.T) {
	stub := newStubTokenAndUserinfoServer(t, map[string]any{"sub": "user-nci", "name": "Dave"}, http.StatusOK)
	defer stub.Close()

	h := newTestHandlerWithMigrations(t, stub.URL)
	setupPendingAuthCode(t, h, "teststatic", "nci-code", "nci-state")
	resp, err := h.Oauth2Token(context.Background(), api.Oauth2TokenRequestObject{
		Body: &api.Oauth2TokenFormdataRequestBody{
			GrantType:    api.AuthorizationCode,
			Code:         "nci-code",
			RedirectUri:  "http://server.test/v1/auth/oauth2/callback",
			CodeVerifier: "verifier",
		},
	})
	if err != nil {
		t.Fatalf("Oauth2Token: %v", err)
	}
	if _, ok := resp.(api.Oauth2Token200JSONResponse); !ok {
		t.Fatalf("expected 200 without client info, got %T", resp)
	}
}
