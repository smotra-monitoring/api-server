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
	JWTSecret     string        `json:"jwt_secret" yaml:"jwt_secret"`
	JWTExpiration time.Duration `json:"jwt_expiration" yaml:"jwt_expiration"`
	// OAuth2 settings can be added here
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

	// Auth validation (warning only for development)
	if c.Auth.JWTSecret == "" && c.Server.Environment == "production" {
		return fmt.Errorf("JWT secret is required in production")
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
		JWTSecret:     "",
		JWTExpiration: 24 * time.Hour,
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
