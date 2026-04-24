package config

import (
	"fmt"
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

	// Endpoint overrides. For type=static these are required.
	// For type=oidc they override the values from the discovery document.
	AuthorizationEndpoint string `json:"authorization_endpoint,omitempty" yaml:"authorization_endpoint,omitempty"`
	TokenEndpoint         string `json:"token_endpoint,omitempty" yaml:"token_endpoint,omitempty"`
	UserInfoEndpoint      string `json:"userinfo_endpoint,omitempty" yaml:"userinfo_endpoint,omitempty"`
	RevocationEndpoint    string `json:"revocation_endpoint,omitempty" yaml:"revocation_endpoint,omitempty"`
	EndSessionEndpoint    string `json:"end_session_endpoint,omitempty" yaml:"end_session_endpoint,omitempty"`
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

// Validate validates the configuration
func (c *Config) Validate() error {
	// Server validation
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
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

	return nil
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
