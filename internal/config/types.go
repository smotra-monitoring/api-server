package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/smotra-monitoring/server/internal/database"
)

// Config holds the application configuration
type Config struct {
	Server         ServerConfig             `json:"server" yaml:"server"`
	DatabaseType   string                   `json:"database_type" yaml:"database_type"` // postgres or sqlite
	PostgresConfig *database.PostgresConfig `json:"postgres_config,omitempty" yaml:"postgres_config,omitempty"`
	SQLiteConfig   *database.SQLiteConfig   `json:"sqlite_config,omitempty" yaml:"sqlite_config,omitempty"`
	Logging        LoggingConfig            `json:"logging" yaml:"logging"`
	Auth           AuthConfig               `json:"auth" yaml:"auth"`
	Agent          AgentConfig              `json:"agent" yaml:"agent"`
	CORS           CORSConfig               `json:"cors,omitempty" yaml:"cors,omitempty"`
	RateLimiter    RateLimiterConfig        `json:"rate_limiter" yaml:"rate_limiter"`
}

// ServerConfig holds server-specific configuration
type ServerConfig struct {
	Host            string        `json:"host" yaml:"host"`
	Port            int           `json:"port" yaml:"port"`
	ReadTimeout     time.Duration `json:"read_timeout" yaml:"read_timeout"`
	WriteTimeout    time.Duration `json:"write_timeout" yaml:"write_timeout"`
	IdleTimeout     time.Duration `json:"idle_timeout" yaml:"idle_timeout"`
	ShutdownTimeout time.Duration `json:"shutdown_timeout" yaml:"shutdown_timeout"`
	Environment     string        `json:"environment" yaml:"environment"` // development, staging, production

	// TrustedProxies lists CIDRs (or bare IPs) of reverse proxies whose X-Forwarded-For
	// header is trusted when determining the client IP. Empty means use RemoteAddr only.
	TrustedProxies []string `json:"trusted_proxies,omitempty" yaml:"trusted_proxies,omitempty"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string `json:"level" yaml:"level"`   // debug, info, warn, error
	Format string `json:"format" yaml:"format"` // json, text
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	// FrontendCallbackURL is the frontend URL the server redirects to after
	// receiving the OAuth2 callback from the IDP (e.g. https://app.example.com/auth/callback).
	FrontendCallbackURL string `json:"frontend_callback_url" yaml:"frontend_callback_url"`

	// ServerCallbackURL is the URL of this server's /auth/oauth2/callback endpoint,
	// registered with IDPs as the redirect_uri.
	ServerCallbackURL string `json:"server_callback_url" yaml:"server_callback_url"`

	// Providers is a map of provider name to its configuration.
	// Built-in provider names: okta, auth0, azure, google, github.
	// Custom providers can be added here with any unique name.
	Providers map[string]OAuthProviderConfig `json:"providers,omitempty" yaml:"providers,omitempty"`
}

// OAuthProviderType defines the endpoint resolution strategy for a provider.
type OAuthProviderType string

const (
	// OAuthProviderTypeOIDC resolves endpoints via OIDC discovery at
	// {IssuerURL}/.well-known/openid-configuration.
	OAuthProviderTypeOIDC OAuthProviderType = "oidc"

	// OAuthProviderTypeStatic uses endpoint URLs directly from config (no discovery).
	// Required for GitHub and other non-OIDC OAuth2 providers.
	OAuthProviderTypeStatic OAuthProviderType = "static"
)

// OAuthProviderConfig holds configuration for a single OAuth2/OIDC provider.
type OAuthProviderConfig struct {
	// Type determines endpoint resolution strategy: "oidc" or "static".
	Type OAuthProviderType `json:"type" yaml:"type"`

	// IssuerURL is the OIDC issuer base URL. Required for type=oidc.
	// Discovery document is fetched from {IssuerURL}/.well-known/openid-configuration.
	IssuerURL string `json:"issuer_url,omitempty" yaml:"issuer_url,omitempty"`

	// ClientID is the OAuth2 application client ID registered with the provider.
	ClientID string `json:"client_id" yaml:"client_id"`

	Scopes []string `json:"scopes,omitempty" yaml:"scopes,omitempty"`

	// Endpoint overrides. For type=static these are required.
	// For type=oidc they override the values from the discovery document.
	AuthorizationEndpoint string `json:"authorization_endpoint,omitempty" yaml:"authorization_endpoint,omitempty"`
	TokenEndpoint         string `json:"token_endpoint,omitempty" yaml:"token_endpoint,omitempty"`
	UserInfoEndpoint      string `json:"userinfo_endpoint,omitempty" yaml:"userinfo_endpoint,omitempty"`
	RevocationEndpoint    string `json:"revocation_endpoint,omitempty" yaml:"revocation_endpoint,omitempty"`
	EndSessionEndpoint    string `json:"end_session_endpoint,omitempty" yaml:"end_session_endpoint,omitempty"`
}

// CORSConfig holds CORS middleware configuration
type CORSConfig struct {
	// Origin is the value set in the Access-Control-Allow-Origin response header.
	// Leave empty to disable CORS headers entirely.
	Origin string `json:"origin" yaml:"origin"`
}

// RateLimiterConfig configures the external rate-limiter service consulted for every request.
type RateLimiterConfig struct {
	// Enabled turns the rate limiter on. When false, all other fields are ignored.
	Enabled bool `json:"enabled" yaml:"enabled"`

	// URL of the limiter service; it receives GET with body {"ip":"<client-ip>"}
	// and answers 200 (allow) or 429 (deny).
	URL string `json:"url" yaml:"url"`

	// Timeout bounds a single limiter call (1ms-20ms).
	Timeout time.Duration `json:"timeout" yaml:"timeout"`

	// FailOpen allows requests when the limiter errors or times out; otherwise they get 503.
	FailOpen bool `json:"fail_open" yaml:"fail_open"`

	// Cache429Sec is how long an IP denied by the limiter is rejected locally without a new call (min 5).
	Cache429Sec int `json:"cache_429_sec" yaml:"cache_429_sec"`

	// CircuitBreakerSec is how long the limiter is skipped after a failed call (min 5).
	CircuitBreakerSec int `json:"circuit_breaker_sec" yaml:"circuit_breaker_sec"`

	// ExemptPaths are URL path prefixes that bypass the limiter.
	ExemptPaths []string `json:"exempt_paths,omitempty" yaml:"exempt_paths,omitempty"`

	// WhitelistSubnets are CIDRs or bare IPs whose clients bypass the limiter.
	WhitelistSubnets []string `json:"whitelist_subnets,omitempty" yaml:"whitelist_subnets,omitempty"`
}

// AgentConfig holds agent-related configuration
type AgentConfig struct {
	ClaimTokenExpirationHours int    `json:"claim_token_expiration_hours" yaml:"claim_token_expiration_hours"`
	ServerURL                 string `json:"server_url" yaml:"server_url"`

	// Claim status polling configuration (linear backoff)
	ClaimPollInitialIntervalSecs int `json:"claim_poll_initial_interval_secs" yaml:"claim_poll_initial_interval_secs"`
	ClaimPollIncrementSecs       int `json:"claim_poll_increment_secs" yaml:"claim_poll_increment_secs"`
	ClaimPollMaxIntervalSecs     int `json:"claim_poll_max_interval_secs" yaml:"claim_poll_max_interval_secs"`
}

// applyEnvOverrides applies environment variable overrides to the configuration.
func (c *Config) applyEnvOverrides() {
	if pw := os.Getenv("DATABASE_PASSWORD"); pw != "" && c.PostgresConfig != nil {
		c.PostgresConfig.Password = pw
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Server validation
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}
	for _, p := range c.Server.TrustedProxies {
		if _, _, err := net.ParseCIDR(p); err != nil && net.ParseIP(p) == nil {
			return fmt.Errorf("invalid server.trusted_proxies entry %q: must be a CIDR or IP address", p)
		}
	}

	// Database validation
	if c.DatabaseType != "postgres" && c.DatabaseType != "sqlite" {
		return fmt.Errorf("invalid database type: %s (must be 'postgres' or 'sqlite')", c.DatabaseType)
	}

	if c.DatabaseType == "postgres" {
		if c.PostgresConfig == nil {
			return fmt.Errorf("postgres config is required when database type is postgres")
		}
		if c.PostgresConfig.Host == "" {
			return fmt.Errorf("database host is required for postgres")
		}
		if c.PostgresConfig.Username == "" {
			return fmt.Errorf("database username is required for postgres")
		}
		if c.PostgresConfig.Database == "" {
			return fmt.Errorf("database name is required for postgres")
		}
	}

	if c.DatabaseType == "sqlite" {
		if c.SQLiteConfig == nil {
			return fmt.Errorf("sqlite config is required when database type is sqlite")
		}
		if c.SQLiteConfig.FilePath == "" {
			return fmt.Errorf("database file path is required for sqlite")
		}
	}

	// Logging validation
	validLogLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLogLevels[c.Logging.Level] {
		return fmt.Errorf("invalid log level: %s", c.Logging.Level)
	}

	validLogFormats := map[string]bool{"json": true, "text": true}
	if !validLogFormats[c.Logging.Format] {
		return fmt.Errorf("invalid log format: %s", c.Logging.Format)
	}

	// Auth validation
	if c.Auth.FrontendCallbackURL == "" {
		return fmt.Errorf("auth.frontend_callback_url is required")
	}
	if c.Auth.ServerCallbackURL == "" {
		return fmt.Errorf("auth.server_callback_url is required")
	}
	for name, p := range c.Auth.Providers {
		if p.ClientID == "" {
			return fmt.Errorf("auth.providers[%s]: client_id is required", name)
		}
		switch p.Type {
		case OAuthProviderTypeOIDC:
			if p.IssuerURL == "" {
				return fmt.Errorf("auth.providers[%s]: issuer_url is required for type=oidc", name)
			}
		case OAuthProviderTypeStatic:
			if p.AuthorizationEndpoint == "" {
				return fmt.Errorf("auth.providers[%s]: authorization_endpoint is required for type=static", name)
			}
			if p.TokenEndpoint == "" {
				return fmt.Errorf("auth.providers[%s]: token_endpoint is required for type=static", name)
			}
			if p.UserInfoEndpoint == "" {
				return fmt.Errorf("auth.providers[%s]: userinfo_endpoint is required for type=static", name)
			}
		default:
			return fmt.Errorf("auth.providers[%s]: unknown type %q (must be oidc or static)", name, p.Type)
		}
	}

	// Agent config validation
	if c.Agent.ClaimTokenExpirationHours < 1 {
		return fmt.Errorf("claim token expiration hours must be at least 1, got %d", c.Agent.ClaimTokenExpirationHours)
	}
	if c.Agent.ServerURL == "" {
		return fmt.Errorf("agent server URL is required")
	}

	// Polling configuration validation
	if c.Agent.ClaimPollInitialIntervalSecs < 1 {
		return fmt.Errorf("claim poll initial interval must be at least 1 second, got %d", c.Agent.ClaimPollInitialIntervalSecs)
	}
	if c.Agent.ClaimPollIncrementSecs < 1 {
		return fmt.Errorf("claim poll increment must be at least 1 second, got %d", c.Agent.ClaimPollIncrementSecs)
	}
	if c.Agent.ClaimPollMaxIntervalSecs < c.Agent.ClaimPollInitialIntervalSecs {
		return fmt.Errorf("claim poll max interval (%d) must be >= initial interval (%d)", c.Agent.ClaimPollMaxIntervalSecs, c.Agent.ClaimPollInitialIntervalSecs)
	}

	// Rate limiter validation (only enforced when enabled)
	if err := c.RateLimiter.validate(); err != nil {
		return err
	}

	return nil
}

const (
	minRateLimiterTimeout = 1 * time.Millisecond
	maxRateLimiterTimeout = 20 * time.Millisecond
	minCache429Secs       = 5
	minCircuitBreakerSecs = 1
)

func validateIPList(field string, entries []string) error {
	for _, p := range entries {
		if _, _, err := net.ParseCIDR(p); err != nil && net.ParseIP(p) == nil {
			return fmt.Errorf("invalid %s entry %q: must be a CIDR or IP address", field, p)
		}
	}
	return nil
}

func (r RateLimiterConfig) validate() error {
	if !r.Enabled {
		return nil
	}
	u, err := url.Parse(r.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("rate_limiter.url must be an http(s) URL, got %q", r.URL)
	}
	if r.Timeout < minRateLimiterTimeout || r.Timeout > maxRateLimiterTimeout {
		return fmt.Errorf("rate_limiter.timeout must be between %s and %s, got %s", minRateLimiterTimeout, maxRateLimiterTimeout, r.Timeout)
	}
	if r.Cache429Sec < minCache429Secs {
		return fmt.Errorf("rate_limiter.cache_429_sec must be at least %d, got %d", minCache429Secs, r.Cache429Sec)
	}
	if r.CircuitBreakerSec < minCircuitBreakerSecs {
		return fmt.Errorf("rate_limiter.circuit_breaker_sec must be at least %d, got %d", minCircuitBreakerSecs, r.CircuitBreakerSec)
	}
	for _, p := range r.ExemptPaths {
		if !strings.HasPrefix(p, "/") {
			return fmt.Errorf("invalid rate_limiter.exempt_paths entry %q: must start with /", p)
		}
	}
	return validateIPList("rate_limiter.whitelist_subnets", r.WhitelistSubnets)
}

// Default returns a Config with sensible default values
func Default() *Config {
	sqlLiteCfg := database.DefaultSQLiteConfig()

	cfg := Config{
		Server:       DefaultServerConfig(),
		DatabaseType: "sqlite",
		SQLiteConfig: &sqlLiteCfg,
		Logging:      DefaultLoggingConfig(),
		Auth:         DefaultAuthConfig(),
		Agent:        DefaultAgentConfig(),
		CORS:         DefaultCORSConfig(),
		RateLimiter:  DefaultRateLimiterConfig(),
	}

	return &cfg
}

func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		Host:            "0.0.0.0",
		Port:            8080,
		ReadTimeout:     15 * time.Second,
		WriteTimeout:    15 * time.Second,
		IdleTimeout:     120 * time.Second,
		ShutdownTimeout: 30 * time.Second,
		Environment:     "development",
	}
}

func DefaultLoggingConfig() LoggingConfig {
	return LoggingConfig{
		Level:  "info",
		Format: "json",
	}
}

func DefaultAuthConfig() AuthConfig {
	return AuthConfig{
		FrontendCallbackURL: "http://localhost:3000/auth/callback",
		ServerCallbackURL:   "http://localhost:8080/v1/auth/oauth2/callback",
		Providers:           map[string]OAuthProviderConfig{},
	}
}

func DefaultAgentConfig() AgentConfig {
	return AgentConfig{
		ClaimTokenExpirationHours:    24,
		ServerURL:                    "https://www.smotra.net",
		ClaimPollInitialIntervalSecs: 5,  // Start at 5 seconds
		ClaimPollIncrementSecs:       5,  // Increase by 5 seconds each attempt
		ClaimPollMaxIntervalSecs:     30, // Cap at 30 seconds
	}
}

func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		Origin: "", // empty = CORS headers disabled
	}
}

func DefaultRateLimiterConfig() RateLimiterConfig {
	return RateLimiterConfig{
		Enabled:           false,
		Timeout:           20 * time.Millisecond,
		FailOpen:          true,
		Cache429Sec:       15,
		CircuitBreakerSec: 5,
		ExemptPaths:       []string{"/healthz", "/metrics"},
	}
}
