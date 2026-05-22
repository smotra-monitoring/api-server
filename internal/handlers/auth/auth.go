package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	api "github.com/smotra-monitoring/server/internal/api/v1"
	"github.com/smotra-monitoring/server/internal/config"
	"github.com/smotra-monitoring/server/internal/database"
	"github.com/smotra-monitoring/server/internal/database/queries"
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
	serverConfig      *config.ServerConfig
	db                database.Database
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
	refreshTotal    atomic.Uint64
	unknownProvider atomic.Uint64
	idpErrorTotal   atomic.Uint64
}

// NewHandler creates a new auth handler.
func NewHandler(log *logger.Logger, auth *config.AuthConfig, server *config.ServerConfig, db database.Database) *Handler {
	client := newHTTPClient()
	return &Handler{
		log:          log.WithComponent("auth"),
		authConfig:   auth,
		serverConfig: server,
		db:           db,
		resolver:     newEndpointResolver(client),
		client:       client,
	}
}

// NewHandlerForTesting creates an auth handler with SSRF validation disabled.
// Use only in tests.
func NewHandlerForTesting(log *logger.Logger, auth *config.AuthConfig, server *config.ServerConfig, db database.Database) *Handler {
	h := NewHandler(log, auth, server, db)
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

// ─── Oauth2Authorize ────────────────────────────────────────────────────────────

// Oauth2Authorize handles GET /auth/oauth2/authorize.
// It resolves the provider's authorization endpoint, persists a pending state record,
// and redirects the browser to the IDP.
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

	// Persist the pending state so /token can retrieve the provider name.
	q := queries.New(h.db.DB())
	stateID := uuid.Must(uuid.NewV7()).String()
	expiresAt_utc := time.Now().Add(10 * time.Minute).UTC() // pending states are short-lived and single-use, so 10m is generous
	if err := q.CreatePendingState(ctx, queries.CreatePendingStateParams{
		ID:        stateID,
		State:     req.Params.State,
		Provider:  providerName,
		ExpiresAt: expiresAt_utc,
	}); err != nil {
		// Non-fatal if duplicate state (unlikely); log and continue
		h.log.WarnContext(ctx, "failed to persist pending state", slog.String("error", err.Error()))
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
// Called by the IDP after the user authenticates.
// Stores the authorization code in the pending state and redirects to the frontend.
func (h *Handler) Oauth2Callback(ctx context.Context, req api.Oauth2CallbackRequestObject) (api.Oauth2CallbackResponseObject, error) {
	h.callbackTotal.Add(1)

	frontendURL := h.authConfig.FrontendCallbackURL
	params := url.Values{}

	if req.Params.Error != nil {
		params.Set("error", *req.Params.Error)
		if req.Params.ErrorDescription != nil {
			params.Set("error_description", *req.Params.ErrorDescription)
		}
	} else {
		code := ""
		state := ""
		if req.Params.Code != nil {
			code = *req.Params.Code
			params.Set("code", code)
		}
		if req.Params.State != nil {
			state = *req.Params.State
			params.Set("state", state)
		}
		// Record auth code in pending state so /token can look up the provider.
		if code != "" && state != "" {
			q := queries.New(h.db.DB())
			if err := q.SetPendingStateAuthCode(ctx, queries.SetPendingStateAuthCodeParams{
				AuthCode: sql.NullString{String: code, Valid: true},
				State:    state,
			}); err != nil {
				// Non-fatal: log and continue redirect
				h.log.WarnContext(ctx, "failed to record auth code in pending state",
					slog.String("state", state), slog.String("error", err.Error()))
			}
		}
	}

	location := frontendURL + "?" + params.Encode()

	return api.Oauth2Callback302Response{
		Headers: api.Oauth2Callback302ResponseHeaders{Location: location},
	}, nil
}

// ─── Oauth2Token ───────────────────────────────────────────────────────────────

// idpTokenResponse is the raw response from the IDP token endpoint.
type idpTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

// Oauth2Token handles POST /auth/oauth2/token.
// Exchanges an authorization code for a server-managed opaque session token.
// IDP tokens are stored server-side and never returned to the client.
func (h *Handler) Oauth2Token(ctx context.Context, req api.Oauth2TokenRequestObject) (api.Oauth2TokenResponseObject, error) {
	h.tokenTotal.Add(1)

	if req.Body == nil {
		h.log.WarnContext(ctx, "token request missing body")
		return api.Oauth2Token400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
			Error: "bad_request", Message: "Request body is required",
		}}, nil
	}

	if req.Body.Code == "" || req.Body.RedirectUri == "" || req.Body.CodeVerifier == "" {
		h.log.WarnContext(ctx, "token request missing required fields", slog.String("code", req.Body.Code), slog.String("redirect_uri", req.Body.RedirectUri), slog.String("code_verifier", req.Body.CodeVerifier))
		return api.Oauth2Token400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
			Error: "bad_request", Message: "code, redirect_uri, and code_verifier are required",
		}}, nil
	}

	// Resolve provider from the pending state record keyed by auth code.
	q := queries.New(h.db.DB())
	pending, err := q.GetPendingStateByAuthCode(ctx, sql.NullString{String: req.Body.Code, Valid: true})
	if err != nil {
		h.log.WarnContext(ctx, "pending state not found for auth code", slog.String("code", req.Body.Code))
		return api.Oauth2Token401JSONResponse{UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
			Error: "invalid_grant", Message: "Authorization code not found or expired",
		}}, nil
	}
	providerName := pending.Provider

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

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", cfg.ClientID)
	form.Set("code", req.Body.Code)
	form.Set("redirect_uri", h.authConfig.ServerCallbackURL)
	form.Set("code_verifier", req.Body.CodeVerifier)

	idpResp, err := h.postForm(ctx, endpoints.TokenEndpoint, form)
	if err != nil {
		h.idpErrorTotal.Add(1)
		body, _ := io.ReadAll(idpResp.Body)
		h.log.ErrorContext(ctx, "token request to IdP failed", slog.String("provider", providerName), slog.String("error", err.Error()), slog.String("body", string(body)))
		return api.Oauth2Token400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
			Error: "idp_error", Message: "Identity provider token request failed",
		}}, nil
	}
	defer idpResp.Body.Close()

	if idpResp.StatusCode != http.StatusOK {
		h.idpErrorTotal.Add(1)
		body, _ := io.ReadAll(idpResp.Body)
		h.log.WarnContext(ctx, "IdP returned error for token request",
			slog.String("provider", providerName),
			slog.Int("status", idpResp.StatusCode),
			slog.String("body", string(body)),
		)
		return api.Oauth2Token401JSONResponse{UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
			Error: "idp_error", Message: "Identity provider rejected the token request",
		}}, nil
	}

	var idpTok idpTokenResponse
	if err := json.NewDecoder(idpResp.Body).Decode(&idpTok); err != nil {
		h.idpErrorTotal.Add(1)
		h.log.ErrorContext(ctx, "failed to decode IdP token response", slog.String("provider", providerName), slog.String("error", err.Error()))
		return api.Oauth2Token400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
			Error: "idp_error", Message: "Could not parse identity provider response",
		}}, nil
	}

	// Delete the consumed pending state (one-time use).
	_ = q.DeletePendingState(ctx, pending.ID)

	// Extract user identity from id_token (OIDC) or access token claims.
	sub, displayName, email, avatarURL := extractUserClaims(idpTok.IDToken, idpTok.AccessToken, providerName)
	if sub == "" {
		h.log.WarnContext(ctx, "could not extract subject from IdP token", slog.String("provider", providerName))
		return api.Oauth2Token401JSONResponse{UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
			Error: "invalid_token", Message: "Could not determine user identity from IdP token",
		}}, nil
	}

	// Upsert tenant + user in a transaction.
	tx, err := h.db.DB().BeginTx(ctx, nil)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to begin database transaction", slog.String("error", err.Error()))
		return api.Oauth2Token400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
			Error: "internal_error", Message: "Database error",
		}}, nil
	}
	defer tx.Rollback() //nolint:errcheck

	qTx := queries.New(tx)

	tenantID := uuid.Must(uuid.NewV7()).String()
	// UpsertTenant is idempotent: keyed on provider+sub so each OAuth identity
	// gets exactly one personal tenant. Returns the existing ID on conflict.
	tenantName := fmt.Sprintf("%s:%s", providerName, sub)
	resultingTenantID, err := qTx.UpsertTenant(ctx, queries.UpsertTenantParams{
		ID:   tenantID,
		Name: tenantName,
	})
	if err != nil {
		h.log.ErrorContext(ctx, "UpsertTenant failed", slog.String("error", err.Error()))
		return api.Oauth2Token400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
			Error: "internal_error", Message: "Database error",
		}}, nil
	}

	userID, err := qTx.UpsertUserByOAuth(ctx, queries.UpsertUserByOAuthParams{
		ID:            uuid.Must(uuid.NewV7()).String(),
		TenantID:      resultingTenantID,
		OauthProvider: providerName,
		OauthSubject:  sub,
		DisplayName:   displayName,
		Email:         sql.NullString{String: email, Valid: email != ""},
		AvatarUrl:     sql.NullString{String: avatarURL, Valid: avatarURL != ""},
	})
	if err != nil {
		h.log.ErrorContext(ctx, "UpsertUserByOAuth failed", slog.String("error", err.Error()))
		return api.Oauth2Token400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
			Error: "internal_error", Message: "Database error",
		}}, nil
	}

	if err := tx.Commit(); err != nil {
		h.log.ErrorContext(ctx, "transaction commit failed", slog.String("error", err.Error()))
		return api.Oauth2Token400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
			Error: "internal_error", Message: "Database error",
		}}, nil
	}

	// Create session.
	isProduction := h.serverConfig.Environment == "production"
	plaintext, tokenHash, err := generateOpaqueToken(isProduction)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to generate opaque token", slog.String("error", err.Error()))
		return api.Oauth2Token400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
			Error: "internal_error", Message: "Token generation failed",
		}}, nil
	}

	now := time.Now().UTC()
	slidingExpiresAt := now.Add(7 * 24 * time.Hour)
	expiresAt := now.Add(90 * 24 * time.Hour)
	if slidingExpiresAt.After(expiresAt) {
		slidingExpiresAt = expiresAt
	}

	var idpTokenExpiry sql.NullTime
	if idpTok.ExpiresIn > 0 {
		idpTokenExpiry = sql.NullTime{Time: now.Add(time.Duration(idpTok.ExpiresIn) * time.Second), Valid: true}
	}

	_, err = q.CreateSession(ctx, queries.CreateSessionParams{
		ID:                 uuid.Must(uuid.NewV7()).String(),
		UserID:             userID,
		TokenHash:          tokenHash,
		SlidingExpiresAt:   slidingExpiresAt,
		ExpiresAt:          expiresAt,
		Oauth2Provider:     providerName,
		Oauth2AccessToken:  idpTok.AccessToken,
		Oauth2RefreshToken: sql.NullString{String: idpTok.RefreshToken, Valid: idpTok.RefreshToken != ""},
		Oauth2TokenExpiry:  idpTokenExpiry,
		Oauth2IDToken:      sql.NullString{String: idpTok.IDToken, Valid: idpTok.IDToken != ""},
		Oauth2Scope:        sql.NullString{String: idpTok.Scope, Valid: idpTok.Scope != ""},
		Oauth2TokenType:    idpTok.TokenType,
	})
	if err != nil {
		h.log.ErrorContext(ctx, "CreateSession failed", slog.String("error", err.Error()))
		return api.Oauth2Token400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
			Error: "internal_error", Message: "Session creation failed",
		}}, nil
	}

	h.log.InfoContext(ctx, "session created", slog.String("user_id", userID), slog.String("provider", providerName))

	return api.Oauth2Token200JSONResponse(api.TokenResponse{
		OpaqueToken: plaintext,
		ExpiresAt:   expiresAt,
	}), nil
}

// extractUserClaims parses an id_token (OIDC) or access token to extract
// the subject, display name, email, and avatar URL.
// jwt.ParseInsecure is safe here because we only need the claims payload;
// we already trust the IdP because we exchanged a code over TLS.
func extractUserClaims(idToken, accessToken, provider string) (sub, displayName, email, avatarURL string) {
	tokenStr := idToken
	if tokenStr == "" {
		tokenStr = accessToken
	}
	if tokenStr == "" {
		return
	}

	var claims jwt.MapClaims
	// ParseInsecure skips signature validation intentionally — we trust the IDP response.
	_, _, err := jwt.NewParser(jwt.WithoutClaimsValidation()).ParseUnverified(tokenStr, &claims)
	if err != nil {
		return
	}

	if v, ok := claims["sub"].(string); ok {
		sub = v
	}
	// GitHub uses "login" not standard OIDC claims.
	if provider == "github" {
		if v, ok := claims["login"].(string); ok && displayName == "" {
			displayName = v
		}
	}
	for _, key := range []string{"name", "preferred_username", "nickname", "email"} {
		if v, ok := claims[key].(string); ok && displayName == "" {
			displayName = v
		}
	}
	if v, ok := claims["email"].(string); ok {
		email = v
	}
	for _, key := range []string{"picture", "avatar_url"} {
		if v, ok := claims[key].(string); ok && avatarURL == "" {
			avatarURL = v
		}
	}
	return
}

// ─── Oauth2Revoke ──────────────────────────────────────────────────────────────

// Oauth2Revoke handles POST /auth/oauth2/revoke.
// Revokes the server-managed session identified by the opaque token in the request body.
// Also attempts IDP token revocation (best-effort).
func (h *Handler) Oauth2Revoke(ctx context.Context, req api.Oauth2RevokeRequestObject) (api.Oauth2RevokeResponseObject, error) {
	h.revokeTotal.Add(1)

	if req.Body == nil || req.Body.OpaqueToken == "" {
		h.log.WarnContext(ctx, "revoke: missing opaque_token in request body")
		return api.Oauth2Revoke400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
			Error: "bad_request", Message: "opaque_token is required",
		}}, nil
	}

	tokenHash := hashToken(req.Body.OpaqueToken)
	q := queries.New(h.db.DB())
	session, err := q.GetSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		// Token not found or already expired/revoked — return success to avoid enumeration.
		h.log.InfoContext(ctx, "revoke: session not found or already revoked")
		return api.Oauth2Revoke200JSONResponse{}, nil
	}

	if err := q.RevokeSession(ctx, session.ID); err != nil {
		h.log.ErrorContext(ctx, "failed to revoke session", slog.String("session_id", session.ID), slog.String("error", err.Error()))
	}

	// Best-effort IdP revocation.
	providerName := session.Oauth2Provider
	cfg, cfgErr := h.resolveProvider(providerName)
	if cfgErr != nil {
		h.log.WarnContext(ctx, "revoke: unknown provider", slog.String("provider", providerName))
		warning := fmt.Sprintf("Session revoked. IdP revocation skipped: provider %q not configured", providerName)
		return api.Oauth2Revoke200JSONResponse{Warning: &warning}, nil
	}

	endpoints, endErr := h.resolver.resolve(ctx, cfg)
	if endErr != nil || endpoints.RevocationEndpoint == "" {
		h.log.WarnContext(ctx, "revoke: provider does not support token revocation", slog.String("provider", providerName))
		warning := fmt.Sprintf("Session revoked. Provider %q does not support token revocation", providerName)
		return api.Oauth2Revoke200JSONResponse{Warning: &warning}, nil
	}

	form := url.Values{}
	form.Set("token", session.Oauth2AccessToken)
	form.Set("client_id", cfg.ClientID)
	idpResp, err := h.postForm(ctx, endpoints.RevocationEndpoint, form)
	idpResp.Body.Close()
	if err != nil {
		h.log.ErrorContext(ctx, "IdP token revocation request failed", slog.String("provider", providerName), slog.String("error", err.Error()))
		warning := fmt.Sprintf("Session revoked. IdP revocation failed: %v", err)
		return api.Oauth2Revoke200JSONResponse{Warning: &warning}, nil
	}

	if idpResp.StatusCode != http.StatusOK {
		h.log.WarnContext(ctx, "IdP returned error for revocation request",
			slog.String("provider", providerName),
			slog.Int("status", idpResp.StatusCode),
		)
	}

	return api.Oauth2Revoke200JSONResponse{}, nil
}

// ─── AuthRefresh ───────────────────────────────────────────────────────────────

// AuthRefresh handles POST /auth/refresh.
// Issues a new session token, revoking the old one atomically.
func (h *Handler) AuthRefresh(ctx context.Context, _ api.AuthRefreshRequestObject) (api.AuthRefreshResponseObject, error) {
	h.refreshTotal.Add(1)

	// Authentication is guaranteed by the AuthenticatedHandler wrapper.
	authInfo := ctx.Value(middleware.AuthContextKey).(*middleware.AuthInfo)

	q := queries.New(h.db.DB())
	oldSession, err := q.GetSessionByID(ctx, authInfo.SessionID)
	if err != nil {
		h.log.WarnContext(ctx, "refresh: session not found for authenticated request", slog.String("session_id", authInfo.SessionID))
		return api.AuthRefresh401JSONResponse{UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
			Error: "unauthorized", Message: "Session not found",
		}}, nil
	}

	isProduction := h.serverConfig.Environment == "production"
	plaintext, tokenHash, err := generateOpaqueToken(isProduction)
	if err != nil {
		h.log.ErrorContext(ctx, "refresh: token generation failed", slog.String("error", err.Error()))
		return api.AuthRefresh401JSONResponse{UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
			Error: "internal_error", Message: "Token generation failed",
		}}, nil
	}

	now := time.Now().UTC()
	newSlidingExpiresAt := now.Add(7 * 24 * time.Hour)
	expiresAt := now.Add(90 * 24 * time.Hour)
	if newSlidingExpiresAt.After(expiresAt) {
		newSlidingExpiresAt = expiresAt
	}

	// Create new session inheriting IDP tokens from old session.
	_, err = q.CreateSession(ctx, queries.CreateSessionParams{
		ID:                 uuid.Must(uuid.NewV7()).String(),
		UserID:             oldSession.UserID,
		TokenHash:          tokenHash,
		SlidingExpiresAt:   newSlidingExpiresAt,
		ExpiresAt:          expiresAt,
		Oauth2Provider:     oldSession.Oauth2Provider,
		Oauth2AccessToken:  oldSession.Oauth2AccessToken,
		Oauth2RefreshToken: oldSession.Oauth2RefreshToken,
		Oauth2TokenExpiry:  oldSession.Oauth2TokenExpiry,
		Oauth2IDToken:      oldSession.Oauth2IDToken,
		Oauth2Scope:        oldSession.Oauth2Scope,
		Oauth2TokenType:    oldSession.Oauth2TokenType,
	})
	if err != nil {
		h.log.ErrorContext(ctx, "refresh: CreateSession failed", slog.String("error", err.Error()))
		return api.AuthRefresh401JSONResponse{UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
			Error: "internal_error", Message: "Session creation failed",
		}}, nil
	}

	// Revoke the old session.
	_ = q.RevokeSession(ctx, oldSession.ID)

	h.log.InfoContext(ctx, "session refreshed", slog.String("user_id", oldSession.UserID))

	return api.AuthRefresh200JSONResponse(api.TokenResponse{
		OpaqueToken: plaintext,
		ExpiresAt:   expiresAt,
	}), nil
}

// ─── GetUserInfo ───────────────────────────────────────────────────────────────

// GetUserInfo handles GET /auth/userinfo.
// Serves user information from the local database using the authenticated session.
func (h *Handler) GetUserInfo(ctx context.Context, _ api.GetUserInfoRequestObject) (api.GetUserInfoResponseObject, error) {
	h.userInfoTotal.Add(1)

	// Authentication is guaranteed by the AuthenticatedHandler wrapper.
	authInfo := ctx.Value(middleware.AuthContextKey).(*middleware.AuthInfo)

	q := queries.New(h.db.DB())
	user, err := q.GetUserByID(ctx, authInfo.UserID)
	if err != nil {
		h.log.WarnContext(ctx, "GetUserInfo: user not found", slog.String("user_id", authInfo.UserID))
		return api.GetUserInfo401JSONResponse{UnauthorizedJSONResponse: api.UnauthorizedJSONResponse{
			Error: "unauthorized", Message: "User not found",
		}}, nil
	}

	info := api.UserInfo{
		Sub: user.ID,
	}
	if user.DisplayName != "" {
		name := user.DisplayName
		info.Name = &name
	}
	if user.Email.Valid {
		email := openapi_types.Email(user.Email.String)
		info.Email = &email
	}
	if user.AvatarUrl.Valid {
		picture := user.AvatarUrl.String
		info.Picture = &picture
	}

	return api.GetUserInfo200JSONResponse(info), nil
}

// ─── Logout ────────────────────────────────────────────────────────────────────

// Logout handles POST /auth/logout.
// Revokes the current session and redirects to the IDP end-session endpoint if supported.
func (h *Handler) Logout(ctx context.Context, req api.LogoutRequestObject) (api.LogoutResponseObject, error) {
	h.logoutTotal.Add(1)

	// Authentication is guaranteed by the AuthenticatedHandler wrapper.
	// Fetch the full session record before revoking so we can pass id_token_hint
	// to the IDP end-session endpoint (required by OIDC RP-Initiated Logout spec).
	var idTokenHint string
	authInfo := ctx.Value(middleware.AuthContextKey).(*middleware.AuthInfo)
	if authInfo.SessionID != "" {
		q := queries.New(h.db.DB())

		if session, err := q.GetSessionByID(ctx, authInfo.SessionID); err == nil {
			if session.Oauth2IDToken.Valid {
				idTokenHint = session.Oauth2IDToken.String
			}
		}
		_ = q.RevokeSession(ctx, authInfo.SessionID)
	}

	// Resolve IDP end-session URL from session provider.
	providerName := authInfo.Provider
	if providerName == "" {
		h.log.InfoContext(ctx, "logout: unknown provider name", slog.String("provider", providerName))
		msg := "Logged out."
		return api.Logout200JSONResponse{Message: &msg}, nil
	}

	cfg, err := h.resolveProvider(providerName)
	if err != nil {
		h.log.WarnContext(ctx, "logout: unknown provider", slog.String("provider", providerName))
		msg := "Logged out."
		return api.Logout200JSONResponse{Message: &msg}, nil
	}

	endpoints, err := h.resolver.resolve(ctx, cfg)
	if err != nil || endpoints.EndSessionEndpoint == "" {
		h.log.InfoContext(ctx, "logout: no end_session_endpoint", slog.String("provider", providerName))
		msg := fmt.Sprintf("Logged out. Provider %q does not support remote session termination.", providerName)
		return api.Logout200JSONResponse{Message: &msg}, nil
	}

	params := url.Values{}
	if idTokenHint != "" {
		params.Set("id_token_hint", idTokenHint)
	}
	if req.Body != nil && req.Body.PostLogoutRedirectUri != nil {
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

	metrics += "# HELP auth_refresh_total Total number of /auth/refresh requests\n"
	metrics += "# TYPE auth_refresh_total counter\n"
	metrics += fmt.Sprintf("auth_refresh_total %d\n", h.refreshTotal.Load())
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
