package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/smotra-monitoring/server/internal/config"
)

// discoveryDoc holds the subset of OIDC discovery document fields we use.
type discoveryDoc struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserInfoEndpoint      string `json:"userinfo_endpoint"`
	RevocationEndpoint    string `json:"revocation_endpoint"`
	EndSessionEndpoint    string `json:"end_session_endpoint"`
}

// cachedDoc wraps a discoveryDoc with a TTL timestamp.
type cachedDoc struct {
	doc       discoveryDoc
	expiresAt time.Time
}

// endpointResolver resolves the effective OAuth2/OIDC endpoints for a provider.
// For type=oidc it fetches the OIDC discovery document (cached for 5 minutes).
// For type=static it reads endpoints directly from config.
// Explicit endpoint overrides in config always win over discovery values.
type endpointResolver struct {
	client            *http.Client
	allowPrivateHosts bool // set in tests to skip SSRF validation

	mu    sync.Mutex
	cache map[string]cachedDoc // keyed by issuer URL
}

func newEndpointResolver(client *http.Client) *endpointResolver {
	return &endpointResolver{
		client: client,
		cache:  make(map[string]cachedDoc),
	}
}

// endpoints holds the resolved OAuth2/OIDC endpoint URLs for a provider.
type endpoints struct {
	AuthorizationEndpoint string
	TokenEndpoint         string
	UserInfoEndpoint      string
	RevocationEndpoint    string // empty if unsupported
	EndSessionEndpoint    string // empty if unsupported
}

// resolve returns the effective Endpoints for the given provider
// It performs OIDC discovery if needed and applies config overrides.
func (r *endpointResolver) resolve(ctx context.Context, cfg config.OAuthProviderConfig) (endpoints, error) {
	var base endpoints

	switch cfg.Type {
	case config.OAuthProviderTypeOIDC:
		if cfg.IssuerURL == "" {
			return endpoints{}, fmt.Errorf("issuer_url is required for type=oidc")
		}
		doc, err := r.fetchDiscovery(ctx, cfg.IssuerURL)
		if err != nil {
			return endpoints{}, fmt.Errorf("OIDC discovery failed: %w", err)
		}
		base = endpoints{
			AuthorizationEndpoint: doc.AuthorizationEndpoint,
			TokenEndpoint:         doc.TokenEndpoint,
			UserInfoEndpoint:      doc.UserInfoEndpoint,
			RevocationEndpoint:    doc.RevocationEndpoint,
			EndSessionEndpoint:    doc.EndSessionEndpoint,
		}

	case config.OAuthProviderTypeStatic:
		base = endpoints{
			AuthorizationEndpoint: cfg.AuthorizationEndpoint,
			TokenEndpoint:         cfg.TokenEndpoint,
			UserInfoEndpoint:      cfg.UserInfoEndpoint,
			RevocationEndpoint:    cfg.RevocationEndpoint,
			EndSessionEndpoint:    cfg.EndSessionEndpoint,
		}

	default:
		return endpoints{}, fmt.Errorf("unknown provider type %q", cfg.Type)
	}

	// Explicit overrides in config win over discovery values.
	if cfg.AuthorizationEndpoint != "" {
		base.AuthorizationEndpoint = cfg.AuthorizationEndpoint
	}
	if cfg.TokenEndpoint != "" {
		base.TokenEndpoint = cfg.TokenEndpoint
	}
	if cfg.UserInfoEndpoint != "" {
		base.UserInfoEndpoint = cfg.UserInfoEndpoint
	}
	if cfg.RevocationEndpoint != "" {
		base.RevocationEndpoint = cfg.RevocationEndpoint
	}
	if cfg.EndSessionEndpoint != "" {
		base.EndSessionEndpoint = cfg.EndSessionEndpoint
	}

	return base, nil
}

// fetchDiscovery fetches and caches the OIDC discovery document for the given issuer URL.
func (r *endpointResolver) fetchDiscovery(ctx context.Context, issuerURL string) (discoveryDoc, error) {
	if issuerURL == "" {
		return discoveryDoc{}, fmt.Errorf("issuer URL cannot be empty")
	}

	r.mu.Lock()
	if cached, ok := r.cache[issuerURL]; ok && time.Now().Before(cached.expiresAt) {
		r.mu.Unlock()
		return cached.doc, nil
	}
	r.mu.Unlock()

	discoveryURL := issuerURL + "/.well-known/openid-configuration"

	if !r.allowPrivateHosts {
		if err := validateProviderURL(discoveryURL); err != nil {
			return discoveryDoc{}, fmt.Errorf("SSRF check failed for discovery URL: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		return discoveryDoc{}, err
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return discoveryDoc{}, fmt.Errorf("fetching discovery document: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return discoveryDoc{}, fmt.Errorf("discovery document returned HTTP %d", resp.StatusCode)
	}

	var doc discoveryDoc
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return discoveryDoc{}, fmt.Errorf("decoding discovery document: %w", err)
	}

	r.mu.Lock()
	r.cache[issuerURL] = cachedDoc{doc: doc, expiresAt: time.Now().Add(5 * time.Minute)}
	r.mu.Unlock()

	return doc, nil
}
