package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"

	api "github.com/smotra-monitoring/server/internal/api/v1"
	"github.com/smotra-monitoring/server/internal/config"
	"github.com/smotra-monitoring/server/internal/logger"
	"github.com/smotra-monitoring/server/internal/middleware"
)

// defaultProviders contains built-in provider configurations that are
// merged with (and overridden by) values supplied in the server config.
var defaultProviders = map[string]config.OAuthProviderConfig{
	"okta": {
		Type: config.OAuthProviderTypeOIDC,
		// IssuerURL must be set in config (tenant-specific)
	},
	"auth0": {
		Type: config.OAuthProviderTypeOIDC,
		// IssuerURL must be set in config (tenant-specific, e.g. https://your-tenant.auth0.com)
	},
	"azure": {
		Type: config.OAuthProviderTypeOIDC,
		// IssuerURL must be set in config (tenant-specific, e.g. https://login.microsoftonline.com/{tenant}/v2.0)
	},
	"google": {
		Type:      config.OAuthProviderTypeOIDC,
		IssuerURL: "https://accounts.google.com",
	},
	"github": {
		Type:                  config.OAuthProviderTypeStatic,
		AuthorizationEndpoint: "https://github.com/login/oauth/authorize",
		TokenEndpoint:         "https://github.com/login/oauth/access_token",
		UserInfoEndpoint:      "https://api.github.com/user",
		// GitHub has no standard revocation or end-session endpoint.
		RevocationEndpoint: "",
		EndSessionEndpoint: "",
	},
}

// Handler implements the authentication-related StrictServerInterface methods.
type Handler struct {
	log               *logger.Logger
	authConfig        *config.AuthConfig
	resolver          *endpointResolver
	client            *http.Client
	allowPrivateHosts bool // set in tests to skip SSRF validation

	// Metrics
	authorizeTotal  atomic.Uint64
	callbackTotal   atomic.Uint64
	tokenTotal      atomic.Uint64
	revokeTotal     atomic.Uint64
	userInfoTotal   atomic.Uint64
	logoutTotal     atomic.Uint64
	unknownProvider atomic.Uint64
	idpErrorTotal   atomic.Uint64
}

// NewHandler creates a new auth handler.
func NewHandler(log *logger.Logger, auth *config.AuthConfig) *Handler {
	client := newHTTPClient()
	return &Handler{
		log:        log.WithComponent("auth"),
		authConfig: auth,
		resolver:   newEndpointResolver(client),
		client:     client,
	}
}

// NewHandlerForTesting creates an auth handler with SSRF validation disabled.
// Use only in tests.
func NewHandlerForTesting(log *logger.Logger, auth *config.AuthConfig) *Handler {
	h := NewHandler(log, auth)
	h.allowPrivateHosts = true
	h.resolver.allowPrivateHosts = true
	return h
}

// resolveProvider looks up the effective provider config, merging defaults
// with any overrides from the server config.
func (h *Handler) resolveProvider(name string) (config.OAuthProviderConfig, error) {
	// Start with built-in defaults (if any).
	cfg, ok := defaultProviders[name]
	if !ok {
		// Not a built-in: must be fully configured in server config.
		cfg = config.OAuthProviderConfig{}
	}

	// Apply server-config overrides.
	if override, ok := h.authConfig.Providers[name]; ok {
		if override.Type != "" {
			cfg.Type = override.Type
		}
		if override.IssuerURL != "" {
			cfg.IssuerURL = override.IssuerURL
		}
		if override.ClientID != "" {
			cfg.ClientID = override.ClientID
		}
		if override.AuthorizationEndpoint != "" {
			cfg.AuthorizationEndpoint = override.AuthorizationEndpoint
		}
		if override.TokenEndpoint != "" {
			cfg.TokenEndpoint = override.TokenEndpoint
		}
		if override.UserInfoEndpoint != "" {
			cfg.UserInfoEndpoint = override.UserInfoEndpoint
		}
		if override.RevocationEndpoint != "" {
			cfg.RevocationEndpoint = override.RevocationEndpoint
		}
		if override.EndSessionEndpoint != "" {
			cfg.EndSessionEndpoint = override.EndSessionEndpoint
		}
	}

	if cfg.Type == "" {
		return config.OAuthProviderConfig{}, fmt.Errorf("provider %q is not configured on this server", name)
	}
	if cfg.ClientID == "" {
		return config.OAuthProviderConfig{}, fmt.Errorf("provider %q has no client_id configured", name)
	}
	return cfg, nil
}

// ─── Oauth2Authorize ───────────────────────────────────────────────────────────

// Oauth2Authorize handles GET /auth/oauth2/authorize.
// It resolves the provider's authorization endpoint and redirects the browser.
func (h *Handler) Oauth2Authorize(ctx context.Context, req api.Oauth2AuthorizeRequestObject) (api.Oauth2AuthorizeResponseObject, error) {
	h.authorizeTotal.Add(1)

	providerName := req.Params.Provider
	cfg, err := h.resolveProvider(providerName)
	if err != nil {
		h.unknownProvider.Add(1)
		h.log.WarnContext(ctx, "unknown provider in authorize", slog.String("provider", providerName))
		return api.Oauth2Authorize400JSONResponse{
			BadRequestJSONResponse: api.BadRequestJSONResponse{
				Error:   "unknown_provider",
				Message: err.Error(),
			},
		}, nil
	}

	endpoints, err := h.resolver.resolve(ctx, cfg)
	if err != nil {
		h.idpErrorTotal.Add(1)
		h.log.ErrorContext(ctx, "failed to resolve provider endpoints", slog.String("provider", providerName), slog.String("error", err.Error()))
		return api.Oauth2Authorize400JSONResponse{
			BadRequestJSONResponse: api.BadRequestJSONResponse{
				Error:   "provider_unavailable",
				Message: "Could not resolve identity provider configuration",
			},
		}, nil
	}

	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", cfg.ClientID)
	params.Set("redirect_uri", h.authConfig.ServerCallbackURL)
	params.Set("scope", req.Params.Scope)
	params.Set("state", req.Params.State)
	params.Set("code_challenge", req.Params.CodeChallenge)
	params.Set("code_challenge_method", string(req.Params.CodeChallengeMethod))

	location := endpoints.AuthorizationEndpoint + "?" + params.Encode()

	return api.Oauth2Authorize302Response{
		Headers: api.Oauth2Authorize302ResponseHeaders{Location: location},
	}, nil
}

// ─── Oauth2Callback ────────────────────────────────────────────────────────────

// Oauth2Callback handles GET /auth/oauth2/callback.
// Called by the IDP after the user authenticates. Redirects to the frontend.
func (h *Handler) Oauth2Callback(_ context.Context, req api.Oauth2CallbackRequestObject) (api.Oauth2CallbackResponseObject, error) {
	h.callbackTotal.Add(1)

	frontendURL := h.authConfig.FrontendCallbackURL
	params := url.Values{}

	if req.Params.Error != nil {
		params.Set("error", *req.Params.Error)
		if req.Params.ErrorDescription != nil {
			params.Set("error_description", *req.Params.ErrorDescription)
		}
	} else {
		if req.Params.Code != nil {
			params.Set("code", *req.Params.Code)
		}
		if req.Params.State != nil {
			params.Set("state", *req.Params.State)
		}
	}

	location := frontendURL + "?" + params.Encode()

	return api.Oauth2Callback302Response{
		Headers: api.Oauth2Callback302ResponseHeaders{Location: location},
	}, nil
}

// ─── Oauth2Token ───────────────────────────────────────────────────────────────

// Oauth2Token handles POST /auth/oauth2/token.
// Proxies the token request to the IDP, adding client_id from server config.
func (h *Handler) Oauth2Token(ctx context.Context, req api.Oauth2TokenRequestObject) (api.Oauth2TokenResponseObject, error) {
	h.tokenTotal.Add(1)

	if req.Body == nil {
		return api.Oauth2Token400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
			Error: "bad_request", Message: "Request body is required",
		}}, nil
	}

	providerName := req.Body.Provider
	grantType := string(req.Body.GrantType)

	cfg, err := h.resolveProvider(providerName)
	if err != nil {
		h.unknownProvider.Add(1)
		return api.Oauth2Token400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
			Error: "unknown_provider", Message: err.Error(),
		}}, nil
	}

	endpoints, err := h.resolver.resolve(ctx, cfg)
	if err != nil {
		h.idpErrorTotal.Add(1)
		h.log.ErrorContext(ctx, "failed to resolve token endpoint", slog.String("provider", providerName), slog.String("error", err.Error()))
		return api.Oauth2Token400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
			Error: "provider_unavailable", Message: "Could not resolve identity provider configuration",
		}}, nil
	}

	// Build the outgoing form body for the IDP.
	form := url.Values{}
	form.Set("grant_type", grantType)
	form.Set("client_id", cfg.ClientID)

	switch req.Body.GrantType {
	case api.Oauth2TokenFormdataBodyGrantTypeAuthorizationCode:
		if req.Body.Code == nil || req.Body.RedirectUri == nil || req.Body.CodeVerifier == nil {
			return api.Oauth2Token400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
				Error: "bad_request", Message: "code, redirect_uri, and code_verifier are required for authorization_code grant",
			}}, nil
		}
		if *req.Body.Code == "" || *req.Body.RedirectUri == "" || *req.Body.CodeVerifier == "" {
			return api.Oauth2Token400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
				Error: "bad_request", Message: "code, redirect_uri, and code_verifier are required for authorization_code grant",
			}}, nil
		}
		form.Set("code", *req.Body.Code)
		form.Set("redirect_uri", *req.Body.RedirectUri)
		form.Set("code_verifier", *req.Body.CodeVerifier)

	case api.Oauth2TokenFormdataBodyGrantTypeRefreshToken:
		if req.Body.RefreshToken == nil || *req.Body.RefreshToken == "" {
			return api.Oauth2Token400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
				Error: "bad_request", Message: "refresh_token is required for refresh_token grant",
			}}, nil
		}
		form.Set("refresh_token", *req.Body.RefreshToken)
		if req.Body.Scope != nil && *req.Body.Scope != "" {
			form.Set("scope", *req.Body.Scope)
		}

	default:
		return api.Oauth2Token400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
			Error: "unsupported_grant_type", Message: fmt.Sprintf("grant_type %q is not supported", grantType),
		}}, nil
	}

	idpResp, err := h.postForm(ctx, endpoints.TokenEndpoint, form)
	if err != nil {
		h.idpErrorTotal.Add(1)
		h.log.ErrorContext(ctx, "token request to IDP failed", slog.String("provider", providerName), slog.String("error", err.Error()))
		return api.Oauth2Token400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
			Error: "idp_error", Message: "Identity provider token request failed",
		}}, nil
	}
	defer idpResp.Body.Close()

	if idpResp.StatusCode != http.StatusOK {
		h.idpErrorTotal.Add(1)
		h.log.WarnContext(ctx, "IDP returned error for token request",
			slog.String("provider", providerName),
			slog.Int("status", idpResp.StatusCode),
		)
		return api.Oauth2Token401JSONResponse{UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
			Error: "idp_error", Message: "Identity provider rejected the token request",
		}}, nil
	}

	var tokenResp api.TokenResponse
	if err := json.NewDecoder(idpResp.Body).Decode(&tokenResp); err != nil {
		return api.Oauth2Token400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
			Error: "idp_error", Message: "Could not parse identity provider response",
		}}, nil
	}

	return api.Oauth2Token200JSONResponse(tokenResp), nil
}

// ─── Oauth2Revoke ──────────────────────────────────────────────────────────────

// Oauth2Revoke handles POST /auth/oauth2/revoke.
// Proxies revocation to the IDP. Providers without a revocation endpoint (e.g. GitHub)
// return 200 with a warning — revocation is treated as a no-op.
func (h *Handler) Oauth2Revoke(ctx context.Context, req api.Oauth2RevokeRequestObject) (api.Oauth2RevokeResponseObject, error) {
	h.revokeTotal.Add(1)

	if req.Body == nil {
		return api.Oauth2Revoke400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
			Error: "bad_request", Message: "Request body is required",
		}}, nil
	}

	providerName := req.Body.Provider
	cfg, err := h.resolveProvider(providerName)
	if err != nil {
		h.unknownProvider.Add(1)
		return api.Oauth2Revoke400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
			Error: "unknown_provider", Message: err.Error(),
		}}, nil
	}

	endpoints, err := h.resolver.resolve(ctx, cfg)
	if err != nil {
		h.idpErrorTotal.Add(1)
		h.log.ErrorContext(ctx, "failed to resolve revocation endpoint", slog.String("provider", providerName), slog.String("error", err.Error()))
		return api.Oauth2Revoke400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
			Error: "provider_unavailable", Message: "Could not resolve identity provider configuration",
		}}, nil
	}

	// Provider does not support revocation — return 200 no-op with a warning.
	if endpoints.RevocationEndpoint == "" {
		h.log.InfoContext(ctx, "revocation no-op: provider has no revocation endpoint", slog.String("provider", providerName))
		warning := fmt.Sprintf("Provider %q does not support token revocation", providerName)
		return api.Oauth2Revoke200JSONResponse{Warning: &warning}, nil
	}

	form := url.Values{}
	form.Set("token", req.Body.Token)
	form.Set("client_id", cfg.ClientID)
	if req.Body.TokenTypeHint != nil {
		form.Set("token_type_hint", string(*req.Body.TokenTypeHint))
	}

	idpResp, err := h.postForm(ctx, endpoints.RevocationEndpoint, form)
	if err != nil {
		h.idpErrorTotal.Add(1)
		h.log.ErrorContext(ctx, "revocation request to IDP failed", slog.String("provider", providerName), slog.String("error", err.Error()))
		return api.Oauth2Revoke400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
			Error: "idp_error", Message: "Identity provider revocation request failed",
		}}, nil
	}
	defer idpResp.Body.Close()

	// RFC 7009: any 2xx is a success.
	if idpResp.StatusCode < 200 || idpResp.StatusCode >= 300 {
		h.idpErrorTotal.Add(1)
		return api.Oauth2Revoke400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
			Error: "idp_error", Message: "Identity provider rejected the revocation request",
		}}, nil
	}

	return api.Oauth2Revoke200JSONResponse{}, nil
}

// ─── GetUserInfo ───────────────────────────────────────────────────────────────

// GetUserInfo handles GET /auth/userinfo.
// Forwards the request to the IDP userinfo endpoint using the Bearer token from the client.
func (h *Handler) GetUserInfo(ctx context.Context, req api.GetUserInfoRequestObject) (api.GetUserInfoResponseObject, error) {
	h.userInfoTotal.Add(1)

	providerName := req.Params.Provider
	cfg, err := h.resolveProvider(providerName)
	if err != nil {
		h.unknownProvider.Add(1)
		return api.GetUserInfo400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
			Error: "unknown_provider", Message: err.Error(),
		}}, nil
	}

	endpoints, err := h.resolver.resolve(ctx, cfg)
	if err != nil {
		h.idpErrorTotal.Add(1)
		h.log.ErrorContext(ctx, "failed to resolve userinfo endpoint", slog.String("provider", providerName), slog.String("error", err.Error()))
		return api.GetUserInfo400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
			Error: "provider_unavailable", Message: "Could not resolve identity provider configuration",
		}}, nil
	}

	// Extract the Bearer token stored in context by middleware.OAuth2Auth.
	authInfo, ok := ctx.Value(middleware.AuthContextKey).(*middleware.AuthInfo)
	if !ok || authInfo == nil || authInfo.BearerToken == "" {
		return api.GetUserInfo401JSONResponse{UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
			Error: "unauthorized", Message: "Authorization header is required",
		}}, nil
	}
	authHeader := authInfo.BearerToken

	idpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoints.UserInfoEndpoint, nil)
	if err != nil {
		return api.GetUserInfo400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
			Error: "internal_error", Message: "Failed to build userinfo request",
		}}, nil
	}
	idpReq.Header.Set("Authorization", authHeader)

	idpResp, err := h.client.Do(idpReq)
	if err != nil {
		h.idpErrorTotal.Add(1)
		h.log.ErrorContext(ctx, "userinfo request to IDP failed", slog.String("provider", providerName), slog.String("error", err.Error()))
		return api.GetUserInfo400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
			Error: "idp_error", Message: "Identity provider userinfo request failed",
		}}, nil
	}
	defer idpResp.Body.Close()

	if idpResp.StatusCode == http.StatusUnauthorized {
		return api.GetUserInfo401JSONResponse{UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
			Error: "unauthorized", Message: "Token is invalid or expired",
		}}, nil
	}
	if idpResp.StatusCode != http.StatusOK {
		h.idpErrorTotal.Add(1)
		return api.GetUserInfo400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
			Error: "idp_error", Message: "Identity provider returned an error",
		}}, nil
	}

	var userInfo api.UserInfo
	if err := json.NewDecoder(idpResp.Body).Decode(&userInfo); err != nil {
		return api.GetUserInfo400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
			Error: "idp_error", Message: "Could not parse identity provider userinfo response",
		}}, nil
	}

	return api.GetUserInfo200JSONResponse(userInfo), nil
}

// ─── Logout ────────────────────────────────────────────────────────────────────

// Logout handles POST /auth/logout.
// Revokes the token (if provider supports it) and redirects to the IDP end-session endpoint.
// Providers without an end-session endpoint (e.g. GitHub) return 200 with a message.
func (h *Handler) Logout(ctx context.Context, req api.LogoutRequestObject) (api.LogoutResponseObject, error) {
	h.logoutTotal.Add(1)

	if req.Body == nil {
		return api.Logout400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
			Error: "bad_request", Message: "Request body with provider is required",
		}}, nil
	}

	providerName := req.Body.Provider
	cfg, err := h.resolveProvider(providerName)
	if err != nil {
		h.unknownProvider.Add(1)
		return api.Logout400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
			Error: "unknown_provider", Message: err.Error(),
		}}, nil
	}

	endpoints, err := h.resolver.resolve(ctx, cfg)
	if err != nil {
		h.idpErrorTotal.Add(1)
		h.log.ErrorContext(ctx, "failed to resolve endpoints for logout", slog.String("provider", providerName), slog.String("error", err.Error()))
		return api.Logout400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
			Error: "provider_unavailable", Message: "Could not resolve identity provider configuration",
		}}, nil
	}

	if endpoints.EndSessionEndpoint == "" {
		h.log.InfoContext(ctx, "logout no-op end-session: provider has no end_session_endpoint", slog.String("provider", providerName))
		msg := fmt.Sprintf("Logged out. Provider %q does not support remote session termination.", providerName)
		return api.Logout200JSONResponse{Message: &msg}, nil
	}

	// Build IDP end-session URL.
	params := url.Values{}
	if req.Body.PostLogoutRedirectUri != nil {
		params.Set("post_logout_redirect_uri", *req.Body.PostLogoutRedirectUri)
	}

	location := endpoints.EndSessionEndpoint
	if len(params) > 0 {
		location += "?" + params.Encode()
	}

	return api.Logout302Response{
		Headers: api.Logout302ResponseHeaders{Location: location},
	}, nil
}

// ─── helpers ───────────────────────────────────────────────────────────────────

// postForm performs a URL-encoded form POST to the given endpoint.
func (h *Handler) postForm(ctx context.Context, endpoint string, form url.Values) (*http.Response, error) {
	if !h.allowPrivateHosts {
		if err := validateProviderURL(endpoint); err != nil {
			return nil, fmt.Errorf("SSRF check failed for endpoint %q: %w", endpoint, err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	// Caller is responsible for closing resp.Body.
	return resp, nil
}

// GetMetrics returns current handler metrics in Prometheus format
func (h *Handler) GetMetrics() string {
	metrics := ""

	metrics += "# HELP auth_authorize_total Total number of /auth/oauth2/authorize requests\n"
	metrics += "# TYPE auth_authorize_total counter\n"
	metrics += fmt.Sprintf("auth_authorize_total %d\n", h.authorizeTotal.Load())
	metrics += "\n"

	metrics += "# HELP auth_callback_total Total number of /auth/oauth2/callback requests\n"
	metrics += "# TYPE auth_callback_total counter\n"
	metrics += fmt.Sprintf("auth_callback_total %d\n", h.callbackTotal.Load())
	metrics += "\n"

	metrics += "# HELP auth_token_total Total number of /auth/oauth2/token requests\n"
	metrics += "# TYPE auth_token_total counter\n"
	metrics += fmt.Sprintf("auth_token_total %d\n", h.tokenTotal.Load())
	metrics += "\n"

	metrics += "# HELP auth_revoke_total Total number of /auth/oauth2/revoke requests\n"
	metrics += "# TYPE auth_revoke_total counter\n"
	metrics += fmt.Sprintf("auth_revoke_total %d\n", h.revokeTotal.Load())
	metrics += "\n"

	metrics += "# HELP auth_userinfo_total Total number of /auth/userinfo requests\n"
	metrics += "# TYPE auth_userinfo_total counter\n"
	metrics += fmt.Sprintf("auth_userinfo_total %d\n", h.userInfoTotal.Load())
	metrics += "\n"

	metrics += "# HELP auth_logout_total Total number of /auth/logout requests\n"
	metrics += "# TYPE auth_logout_total counter\n"
	metrics += fmt.Sprintf("auth_logout_total %d\n", h.logoutTotal.Load())
	metrics += "\n"

	metrics += "# HELP auth_unknown_provider_total Total number of unknown provider errors\n"
	metrics += "# TYPE auth_unknown_provider_total counter\n"
	metrics += fmt.Sprintf("auth_unknown_provider_total %d\n", h.unknownProvider.Load())
	metrics += "\n"

	metrics += "# HELP auth_idp_error_total Total number of IDP errors\n"
	metrics += "# TYPE auth_idp_error_total counter\n"
	metrics += fmt.Sprintf("auth_idp_error_total %d\n", h.idpErrorTotal.Load())
	metrics += "\n"

	metrics += "\n"

	return metrics
}
