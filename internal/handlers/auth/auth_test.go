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
	return auth.NewHandlerForTesting(log, newTestAuthConfig(stubURL))
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

func TestOauth2Token_AuthorizationCode_ProxiesToIDP(t *testing.T) {
	tokenResponse := api.TokenResponse{
		AccessToken: "access-token-123",
		TokenType:   "Bearer",
		ExpiresIn:   3600,
	}
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			if err := r.ParseForm(); err != nil {
				http.Error(w, "bad form", 400)
				return
			}
			if r.FormValue("grant_type") != "authorization_code" {
				http.Error(w, "wrong grant_type", 400)
				return
			}
			if r.FormValue("client_id") != "test-client-id" {
				http.Error(w, "wrong client_id", 400)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(tokenResponse)
			return
		}
		http.NotFound(w, r)
	}))
	defer stub.Close()

	h := newTestHandler(t, stub.URL)

	code := "auth-code"
	redirectURI := "http://client.test/callback"
	codeVerifier := "verifier123"

	resp, err := h.Oauth2Token(context.Background(), api.Oauth2TokenRequestObject{
		Body: &api.Oauth2TokenFormdataRequestBody{
			Provider:     "teststatic",
			GrantType:    api.Oauth2TokenFormdataBodyGrantTypeAuthorizationCode,
			Code:         &code,
			RedirectUri:  &redirectURI,
			CodeVerifier: &codeVerifier,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tokenResp, ok := resp.(api.Oauth2Token200JSONResponse)
	if !ok {
		t.Fatalf("expected 200 response, got %T", resp)
	}
	if tokenResp.AccessToken != tokenResponse.AccessToken {
		t.Errorf("expected access_token=%q, got %q", tokenResponse.AccessToken, tokenResp.AccessToken)
	}
}

func TestOauth2Token_MissingProvider_Returns400(t *testing.T) {
	h := newTestHandler(t, "http://idp.test")

	code := "c"
	redirectURI := "u"
	codeVerifier := "v"

	resp, err := h.Oauth2Token(context.Background(), api.Oauth2TokenRequestObject{
		Body: &api.Oauth2TokenFormdataRequestBody{
			Provider:     "", // missing provider
			GrantType:    api.Oauth2TokenFormdataBodyGrantTypeAuthorizationCode,
			Code:         &code,
			RedirectUri:  &redirectURI,
			CodeVerifier: &codeVerifier,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := resp.(api.Oauth2Token400JSONResponse); !ok {
		t.Errorf("expected 400, got %T", resp)
	}
}

func TestOauth2Token_IDPError_SanitizesMessage(t *testing.T) {
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			http.Error(w, `{"error":"invalid_grant","error_description":"Internal DB error: connection refused"}`, 400)
			return
		}
		http.NotFound(w, r)
	}))
	defer stub.Close()

	h := newTestHandler(t, stub.URL)

	code := "bad-code"
	redirectURI := "http://client.test/callback"
	codeVerifier := "v"

	resp, err := h.Oauth2Token(context.Background(), api.Oauth2TokenRequestObject{
		Body: &api.Oauth2TokenFormdataRequestBody{
			Provider:     "teststatic",
			GrantType:    api.Oauth2TokenFormdataBodyGrantTypeAuthorizationCode,
			Code:         &code,
			RedirectUri:  &redirectURI,
			CodeVerifier: &codeVerifier,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	errResp, ok := resp.(api.Oauth2Token401JSONResponse)
	if !ok {
		t.Fatalf("expected 401, got %T", resp)
	}
	// Must not expose raw IDP error text.
	if strings.Contains(errResp.Message, "connection refused") {
		t.Errorf("raw IDP error leaked into response: %q", errResp.Message)
	}
	if strings.Contains(errResp.Message, "Internal DB error") {
		t.Errorf("raw IDP error leaked into response: %q", errResp.Message)
	}
}

// ─── Oauth2Revoke ─────────────────────────────────────────────────────────────

func TestOauth2Revoke_NoRevocationEndpoint_Returns200WithWarning(t *testing.T) {
	h := newTestHandler(t, "http://idp.test")

	hint := api.Oauth2RevokeFormdataBodyTokenTypeHintAccessToken
	resp, err := h.Oauth2Revoke(context.Background(), api.Oauth2RevokeRequestObject{
		Body: &api.Oauth2RevokeFormdataRequestBody{
			Provider:      "noendsession",
			Token:         "some-token",
			TokenTypeHint: &hint,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	revokeResp, ok := resp.(api.Oauth2Revoke200JSONResponse)
	if !ok {
		t.Fatalf("expected 200, got %T", resp)
	}
	if revokeResp.Warning == nil || *revokeResp.Warning == "" {
		t.Error("expected non-empty warning for provider without revocation endpoint")
	}
}

func TestOauth2Revoke_WithRevocationEndpoint_Proxies(t *testing.T) {
	revokeCalled := false
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/revoke" {
			revokeCalled = true
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer stub.Close()

	h := newTestHandler(t, stub.URL)

	hint := api.Oauth2RevokeFormdataBodyTokenTypeHintAccessToken
	resp, err := h.Oauth2Revoke(context.Background(), api.Oauth2RevokeRequestObject{
		Body: &api.Oauth2RevokeFormdataRequestBody{
			Provider:      "teststatic",
			Token:         "access-token-xyz",
			TokenTypeHint: &hint,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := resp.(api.Oauth2Revoke200JSONResponse); !ok {
		t.Errorf("expected 200, got %T", resp)
	}
	if !revokeCalled {
		t.Error("expected revocation endpoint to be called")
	}
}

// ─── GetUserInfo ──────────────────────────────────────────────────────────────

func TestGetUserInfo_ProxiesToIDP(t *testing.T) {
	userInfo := api.UserInfo{Sub: "user-123"}
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/userinfo" {
			if r.Header.Get("Authorization") == "" {
				http.Error(w, "missing auth", 401)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(userInfo)
			return
		}
		http.NotFound(w, r)
	}))
	defer stub.Close()

	h := newTestHandler(t, stub.URL)

	ctx := context.WithValue(context.Background(), middleware.AuthContextKey, &middleware.AuthInfo{
		AuthType:    "oauth2",
		BearerToken: "Bearer test-access-token",
	})

	resp, err := h.GetUserInfo(ctx, api.GetUserInfoRequestObject{
		Params: api.GetUserInfoParams{Provider: "teststatic"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	uiResp, ok := resp.(api.GetUserInfo200JSONResponse)
	if !ok {
		t.Fatalf("expected 200, got %T", resp)
	}
	if uiResp.Sub != "user-123" {
		t.Errorf("expected sub=user-123, got %q", uiResp.Sub)
	}
}

func TestGetUserInfo_MissingAuthHeader_Returns401(t *testing.T) {
	h := newTestHandler(t, "http://idp.test")

	ctx := context.Background() // no AuthInfo in context → should return 401

	resp, err := h.GetUserInfo(ctx, api.GetUserInfoRequestObject{
		Params: api.GetUserInfoParams{Provider: "teststatic"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := resp.(api.GetUserInfo401JSONResponse); !ok {
		t.Errorf("expected 401, got %T", resp)
	}
}

// ─── Logout ───────────────────────────────────────────────────────────────────

func TestLogout_WithEndSessionEndpoint_Returns302(t *testing.T) {
	h := newTestHandler(t, "http://idp.test")

	postLogout := "http://frontend.test/"
	resp, err := h.Logout(context.Background(), api.LogoutRequestObject{
		Body: &api.LogoutJSONRequestBody{
			Provider:              "teststatic",
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
	if !strings.Contains(redirect.Headers.Location, "frontend.test") {
		t.Errorf("expected post_logout_redirect_uri in Location (%q)", redirect.Headers.Location)
	}
}

func TestLogout_NoEndSessionEndpoint_Returns200(t *testing.T) {
	h := newTestHandler(t, "http://idp.test")

	resp, err := h.Logout(context.Background(), api.LogoutRequestObject{
		Body: &api.LogoutJSONRequestBody{Provider: "noendsession"},
	})
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
	cfg := &config.AuthConfig{
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
	h := auth.NewHandler(log, cfg)

	code := "c"
	redirectURI := "http://client.test/callback"
	codeVerifier := "v"

	resp, err := h.Oauth2Token(context.Background(), api.Oauth2TokenRequestObject{
		Body: &api.Oauth2TokenFormdataRequestBody{
			Provider:     "ssrf",
			GrantType:    api.Oauth2TokenFormdataBodyGrantTypeAuthorizationCode,
			Code:         &code,
			RedirectUri:  &redirectURI,
			CodeVerifier: &codeVerifier,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := resp.(api.Oauth2Token400JSONResponse); !ok {
		t.Errorf("expected 400 for SSRF attempt, got %T", resp)
	}
}

// ─── GitHub built-in default ──────────────────────────────────────────────────

func TestOauth2Authorize_GitHubBuiltin_UsesStaticEndpoints(t *testing.T) {
	log, _ := testutil.NewTestLogger()
	cfg := &config.AuthConfig{
		FrontendCallbackURL: "http://frontend.test/callback",
		ServerCallbackURL:   "http://server.test/callback",
		Providers: map[string]config.OAuthProviderConfig{
			// GitHub supplied in server config (just client_id; defaults supply endpoints).
			"github": {
				ClientID: "github-client-id",
			},
		},
	}
	h := auth.NewHandler(log, cfg)

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

// ─── GetMetrics ───────────────────────────────────────────────────────────────

func TestAuthHandler_GetMetrics_ContainsAllCounters(t *testing.T) {
	h := newTestHandler(t, "http://idp.test")

	out := h.GetMetrics()

	expectedCounters := []string{
		"auth_authorize_total",
		"auth_callback_total",
		"auth_token_total",
		"auth_revoke_total",
		"auth_userinfo_total",
		"auth_logout_total",
		"auth_unknown_provider_total",
		"auth_idp_error_total",
	}
	for _, counter := range expectedCounters {
		if !strings.Contains(out, counter) {
			t.Errorf("expected counter %q in GetMetrics output", counter)
		}
	}
}

func TestAuthHandler_GetMetrics_CountersIncrementCorrectly(t *testing.T) {
	h := newTestHandler(t, "http://idp.test")

	// Trigger one authorize request to increment the authorize counter.
	_, _ = h.Oauth2Authorize(context.Background(), api.Oauth2AuthorizeRequestObject{
		Params: api.Oauth2AuthorizeParams{
			Provider:            "teststatic",
			Scope:               "openid",
			State:               "s",
			CodeChallenge:       "c",
			CodeChallengeMethod: api.S256,
		},
	})

	out := h.GetMetrics()
	if !strings.Contains(out, "auth_authorize_total 1") {
		t.Errorf("expected auth_authorize_total 1 in GetMetrics output:\n%s", out)
	}
}
