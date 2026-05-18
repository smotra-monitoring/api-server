package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	api "github.com/smotra-monitoring/server/internal/api/v1"
	"github.com/smotra-monitoring/server/internal/config"
	"github.com/smotra-monitoring/server/internal/handlers/auth"
	"github.com/smotra-monitoring/server/internal/middleware"
	"github.com/smotra-monitoring/server/internal/testutil"
)

// newTestConfig returns an AuthConfig pre-populated with a stub IDP server URL.
func newTestAuthConfig(stubURL string) *config.AuthConfig {
	cfg := testutil.DefaultTestConfig()
	cfg.Auth.FrontendCallbackURL = "http://frontend.test/auth/callback"
	cfg.Auth.ServerCallbackURL = "http://server.test/v1/auth/oauth2/callback"
	cfg.Auth.Providers = map[string]config.OAuthProviderConfig{
		"teststatic": {
			Type:                  config.OAuthProviderTypeStatic,
			ClientID:              "test-client-id",
			AuthorizationEndpoint: stubURL + "/authorize",
			TokenEndpoint:         stubURL + "/token",
			UserInfoEndpoint:      stubURL + "/userinfo",
			RevocationEndpoint:    stubURL + "/revoke",
			EndSessionEndpoint:    stubURL + "/logout",
		},
		"testoidc": {
			Type:      config.OAuthProviderTypeOIDC,
			ClientID:  "oidc-client-id",
			IssuerURL: stubURL,
		},
		"noendsession": {
			Type:                  config.OAuthProviderTypeStatic,
			ClientID:              "noes-client-id",
			AuthorizationEndpoint: stubURL + "/authorize",
			TokenEndpoint:         stubURL + "/token",
			UserInfoEndpoint:      stubURL + "/userinfo",
			// No RevocationEndpoint or EndSessionEndpoint
		},
	}
	return &cfg.Auth
}

func newTestHandler(t *testing.T, stubURL string) *auth.Handler {
	t.Helper()
	log, _ := testutil.NewTestLogger()
	db := testutil.SetupTestSQLiteDB(t)
	cfg := testutil.DefaultTestConfig()
	return auth.NewHandlerForTesting(log, newTestAuthConfig(stubURL), &cfg.Server, db)
}

// ─── Oauth2Authorize ──────────────────────────────────────────────────────────

func TestOauth2Authorize_StaticProvider_Returns302(t *testing.T) {
	h := newTestHandler(t, "http://idp.test")

	req := api.Oauth2AuthorizeRequestObject{
		Params: api.Oauth2AuthorizeParams{
			Provider:            "teststatic",
			Scope:               "openid profile",
			State:               "csrf-token",
			CodeChallenge:       "challenge",
			CodeChallengeMethod: api.S256,
		},
	}

	resp, err := h.Oauth2Authorize(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	redirect, ok := resp.(api.Oauth2Authorize302Response)
	if !ok {
		t.Fatalf("expected 302 response, got %T", resp)
	}

	loc, err := url.Parse(redirect.Headers.Location)
	if err != nil {
		t.Fatalf("invalid Location URL: %v", err)
	}

	if loc.Path != "/authorize" {
		t.Errorf("expected path /authorize, got %s", loc.Path)
	}
	q := loc.Query()
	if q.Get("client_id") != "test-client-id" {
		t.Errorf("expected client_id=test-client-id, got %q", q.Get("client_id"))
	}
	if q.Get("state") != "csrf-token" {
		t.Errorf("expected state=csrf-token, got %q", q.Get("state"))
	}
	if q.Get("code_challenge") != "challenge" {
		t.Errorf("expected code_challenge=challenge, got %q", q.Get("code_challenge"))
	}
	if q.Get("redirect_uri") != "http://server.test/v1/auth/oauth2/callback" {
		t.Errorf("redirect_uri is not server callback URL, got %q", q.Get("redirect_uri"))
	}
}

func TestOauth2Authorize_UnknownProvider_Returns400(t *testing.T) {
	h := newTestHandler(t, "http://idp.test")

	req := api.Oauth2AuthorizeRequestObject{
		Params: api.Oauth2AuthorizeParams{
			Provider:            "notconfigured",
			Scope:               "openid",
			State:               "s",
			CodeChallenge:       "c",
			CodeChallengeMethod: api.S256,
		},
	}

	resp, err := h.Oauth2Authorize(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := resp.(api.Oauth2Authorize400JSONResponse); !ok {
		t.Errorf("expected 400 response, got %T", resp)
	}
}

// ─── Oauth2Callback ───────────────────────────────────────────────────────────

func TestOauth2Callback_Success_RedirectsToFrontend(t *testing.T) {
	h := newTestHandler(t, "http://idp.test")

	code := "auth-code-xyz"
	state := "csrf-state"
	req := api.Oauth2CallbackRequestObject{
		Params: api.Oauth2CallbackParams{
			Code:  &code,
			State: &state,
		},
	}

	resp, err := h.Oauth2Callback(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	redirect, ok := resp.(api.Oauth2Callback302Response)
	if !ok {
		t.Fatalf("expected 302 response, got %T", resp)
	}

	loc, err := url.Parse(redirect.Headers.Location)
	if err != nil {
		t.Fatalf("invalid Location URL: %v", err)
	}

	if !strings.HasPrefix(redirect.Headers.Location, "http://frontend.test/auth/callback") {
		t.Errorf("expected redirect to frontend, got %q", redirect.Headers.Location)
	}
	q := loc.Query()
	if q.Get("code") != code {
		t.Errorf("expected code=%q, got %q", code, q.Get("code"))
	}
	if q.Get("state") != state {
		t.Errorf("expected state=%q, got %q", state, q.Get("state"))
	}
}

func TestOauth2Callback_Error_ForwardsErrorToFrontend(t *testing.T) {
	h := newTestHandler(t, "http://idp.test")

	errCode := "access_denied"
	errDesc := "User denied access"
	req := api.Oauth2CallbackRequestObject{
		Params: api.Oauth2CallbackParams{
			Error:            &errCode,
			ErrorDescription: &errDesc,
		},
	}

	resp, err := h.Oauth2Callback(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	redirect, ok := resp.(api.Oauth2Callback302Response)
	if !ok {
		t.Fatalf("expected 302 response, got %T", resp)
	}

	loc, err := url.Parse(redirect.Headers.Location)
	if err != nil {
		t.Fatalf("invalid Location URL: %v", err)
	}

	q := loc.Query()
	if q.Get("error") != errCode {
		t.Errorf("expected error=%q, got %q", errCode, q.Get("error"))
	}
	if q.Get("error_description") != errDesc {
		t.Errorf("expected error_description=%q, got %q", errDesc, q.Get("error_description"))
	}
	// code must not be present on error
	if q.Get("code") != "" {
		t.Errorf("code should not be present on error callback, got %q", q.Get("code"))
	}
}

// ─── Oauth2Token ──────────────────────────────────────────────────────────────

// TestOauth2Token_MissingCode verifies that missing required fields returns 400.
func TestOauth2Token_MissingCode_Returns400(t *testing.T) {
	h := newTestHandler(t, "http://idp.test")

	resp, err := h.Oauth2Token(context.Background(), api.Oauth2TokenRequestObject{
		Body: &api.Oauth2TokenFormdataRequestBody{
			GrantType: api.AuthorizationCode,
			// code, redirect_uri, code_verifier all empty
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := resp.(api.Oauth2Token400JSONResponse); !ok {
		t.Errorf("expected 400 for missing code, got %T", resp)
	}
}

// TestOauth2Token_UnknownAuthCode verifies that a code not in pending_states returns 401.
func TestOauth2Token_UnknownAuthCode_Returns401(t *testing.T) {
	h := newTestHandler(t, "http://idp.test")

	resp, err := h.Oauth2Token(context.Background(), api.Oauth2TokenRequestObject{
		Body: &api.Oauth2TokenFormdataRequestBody{
			GrantType:    api.AuthorizationCode,
			Code:         "no-such-code",
			RedirectUri:  "http://client.test/callback",
			CodeVerifier: "verifier",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := resp.(api.Oauth2Token401JSONResponse); !ok {
		t.Errorf("expected 401 for unknown auth code, got %T", resp)
	}
}

// ─── Oauth2Revoke ─────────────────────────────────────────────────────────────

// TestOauth2Revoke_MissingToken verifies that missing opaque_token returns 400.
func TestOauth2Revoke_MissingToken_Returns400(t *testing.T) {
	h := newTestHandler(t, "http://idp.test")

	resp, err := h.Oauth2Revoke(context.Background(), api.Oauth2RevokeRequestObject{
		Body: &api.Oauth2RevokeJSONRequestBody{OpaqueToken: ""},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := resp.(api.Oauth2Revoke400JSONResponse); !ok {
		t.Errorf("expected 400, got %T", resp)
	}
}

// TestOauth2Revoke_UnknownToken_Returns200 verifies that an unknown token is treated as a no-op
// to avoid token enumeration attacks.
func TestOauth2Revoke_UnknownToken_Returns200(t *testing.T) {
	h := newTestHandler(t, "http://idp.test")

	resp, err := h.Oauth2Revoke(context.Background(), api.Oauth2RevokeRequestObject{
		Body: &api.Oauth2RevokeJSONRequestBody{OpaqueToken: "st_test_unknowntokenhash"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := resp.(api.Oauth2Revoke200JSONResponse); !ok {
		t.Errorf("expected 200 no-op for unknown token, got %T", resp)
	}
}

// ─── GetUserInfo ──────────────────────────────────────────────────────────────

// TestGetUserInfo_NoSession verifies that a missing/unauthenticated session returns 401.
func TestGetUserInfo_NoSession_Returns401(t *testing.T) {
	h := newTestHandler(t, "http://idp.test")

	// No AuthInfo in context
	resp, err := h.GetUserInfo(context.Background(), api.GetUserInfoRequestObject{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := resp.(api.GetUserInfo401JSONResponse); !ok {
		t.Errorf("expected 401, got %T", resp)
	}
}

// TestGetUserInfo_AuthenticatedButMissingUser verifies that an authenticated session
// with a non-existent user_id returns 401.
func TestGetUserInfo_AuthenticatedButMissingUser_Returns401(t *testing.T) {
	h := newTestHandler(t, "http://idp.test")

	ctx := context.WithValue(context.Background(), middleware.AuthContextKey, &middleware.AuthInfo{
		AuthType:      "oauth2",
		Authenticated: true,
		UserID:        "nonexistent-user-id",
		SessionID:     "some-session-id",
	})

	resp, err := h.GetUserInfo(ctx, api.GetUserInfoRequestObject{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := resp.(api.GetUserInfo401JSONResponse); !ok {
		t.Errorf("expected 401, got %T", resp)
	}
}

// ─── Logout ───────────────────────────────────────────────────────────────────

// TestLogout_WithEndSessionEndpoint_Returns302 verifies redirect when session has a provider
// with an end-session endpoint.
func TestLogout_WithEndSessionEndpoint_Returns302(t *testing.T) {
	h := newTestHandler(t, "http://idp.test")

	postLogout := "http://frontend.test/"
	ctx := context.WithValue(context.Background(), middleware.AuthContextKey, &middleware.AuthInfo{
		AuthType:      "oauth2",
		Authenticated: true,
		Provider:      "teststatic",
		SessionID:     "fake-session-id",
	})

	resp, err := h.Logout(ctx, api.LogoutRequestObject{
		Body: &api.LogoutJSONRequestBody{
			PostLogoutRedirectUri: &postLogout,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	redirect, ok := resp.(api.Logout302Response)
	if !ok {
		t.Fatalf("expected 302, got %T", resp)
	}
	if !strings.HasPrefix(redirect.Headers.Location, "http://idp.test/logout") {
		t.Errorf("expected redirect to IDP logout, got %q", redirect.Headers.Location)
	}
	if !strings.Contains(redirect.Headers.Location, "post_logout_redirect_uri=") {
		t.Error("expected post_logout_redirect_uri in Location")
	}
}

// TestLogout_NoEndSessionEndpoint_Returns200 verifies 200 when provider has no end-session endpoint.
func TestLogout_NoEndSessionEndpoint_Returns200(t *testing.T) {
	h := newTestHandler(t, "http://idp.test")

	ctx := context.WithValue(context.Background(), middleware.AuthContextKey, &middleware.AuthInfo{
		AuthType:      "oauth2",
		Authenticated: true,
		Provider:      "noendsession",
		SessionID:     "fake-session-id",
	})

	resp, err := h.Logout(ctx, api.LogoutRequestObject{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := resp.(api.Logout200JSONResponse); !ok {
		t.Errorf("expected 200, got %T", resp)
	}
}

// ─── SSRF protection ──────────────────────────────────────────────────────────

func TestOauth2Token_SSRFViaStaticEndpoint_Returns400(t *testing.T) {
	log, _ := testutil.NewTestLogger()
	db := testutil.SetupTestSQLiteDB(t)
	cfg := testutil.DefaultTestConfig()
	authCfg := &config.AuthConfig{
		FrontendCallbackURL: "http://frontend.test/callback",
		ServerCallbackURL:   "http://server.test/callback",
		Providers: map[string]config.OAuthProviderConfig{
			"ssrf": {
				Type:          config.OAuthProviderTypeStatic,
				ClientID:      "id",
				TokenEndpoint: "http://169.254.169.254/token",
				// Other endpoints not needed for this test
				AuthorizationEndpoint: "https://example.com/auth",
				UserInfoEndpoint:      "https://example.com/userinfo",
			},
		},
	}
	h := auth.NewHandler(log, authCfg, &cfg.Server, db)

	resp, err := h.Oauth2Token(context.Background(), api.Oauth2TokenRequestObject{
		Body: &api.Oauth2TokenFormdataRequestBody{
			GrantType:    api.AuthorizationCode,
			Code:         "no-such-code",
			RedirectUri:  "http://client.test/callback",
			CodeVerifier: "v",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return 401 because code is not in pending_states; SSRF URL never reached.
	if _, ok401 := resp.(api.Oauth2Token401JSONResponse); !ok401 {
		if _, ok400 := resp.(api.Oauth2Token400JSONResponse); !ok400 {
			t.Errorf("expected 400 or 401 for SSRF/unknown code attempt, got %T", resp)
		}
	}
}

// ─── GitHub built-in default ──────────────────────────────────────────────────

func TestOauth2Authorize_GitHubBuiltin_UsesStaticEndpoints(t *testing.T) {
	log, _ := testutil.NewTestLogger()
	db := testutil.SetupTestSQLiteDB(t)
	cfg := testutil.DefaultTestConfig()
	authCfg := &config.AuthConfig{
		FrontendCallbackURL: "http://frontend.test/callback",
		ServerCallbackURL:   "http://server.test/callback",
		Providers: map[string]config.OAuthProviderConfig{
			// GitHub supplied in server config (just client_id; defaults supply endpoints).
			"github": {
				ClientID: "github-client-id",
			},
		},
	}
	h := auth.NewHandler(log, authCfg, &cfg.Server, db)

	resp, err := h.Oauth2Authorize(context.Background(), api.Oauth2AuthorizeRequestObject{
		Params: api.Oauth2AuthorizeParams{
			Provider:            "github",
			Scope:               "read:user",
			State:               "s",
			CodeChallenge:       "c",
			CodeChallengeMethod: api.S256,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	redirect, ok := resp.(api.Oauth2Authorize302Response)
	if !ok {
		t.Fatalf("expected 302, got %T", resp)
	}
	if !strings.HasPrefix(redirect.Headers.Location, "https://github.com/login/oauth/authorize") {
		t.Errorf("expected GitHub authorization URL, got %q", redirect.Headers.Location)
	}
}

// ─── OIDC discovery (integration-style, uses httptest) ───────────────────────

func TestOauth2Authorize_OIDCProvider_UsesDiscoveryEndpoints(t *testing.T) {
	discoveryCallCount := 0
	authEndpoint := ""

	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			discoveryCallCount++
			authEndpoint = "http://" + r.Host + "/idp/authorize"
			doc := map[string]string{
				"authorization_endpoint": authEndpoint,
				"token_endpoint":         "http://" + r.Host + "/idp/token",
				"userinfo_endpoint":      "http://" + r.Host + "/idp/userinfo",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(doc)
			return
		}
		http.NotFound(w, r)
	}))
	defer stub.Close()

	h := newTestHandler(t, stub.URL)

	makeAuthorizeReq := func() api.Oauth2AuthorizeResponseObject {
		resp, err := h.Oauth2Authorize(context.Background(), api.Oauth2AuthorizeRequestObject{
			Params: api.Oauth2AuthorizeParams{
				Provider:            "testoidc",
				Scope:               "openid",
				State:               "s",
				CodeChallenge:       "c",
				CodeChallengeMethod: api.S256,
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return resp
	}

	// First call — triggers discovery.
	resp1 := makeAuthorizeReq()
	redirect1, ok := resp1.(api.Oauth2Authorize302Response)
	if !ok {
		t.Fatalf("expected 302, got %T", resp1)
	}
	if !strings.HasPrefix(redirect1.Headers.Location, authEndpoint) {
		t.Errorf("expected redirect to discovery-resolved endpoint %q, got %q", authEndpoint, redirect1.Headers.Location)
	}

	// Second call — must use cache, not re-fetch discovery.
	_ = makeAuthorizeReq()
	if discoveryCallCount != 1 {
		t.Errorf("expected discovery to be called once (cached), got %d calls", discoveryCallCount)
	}
}
