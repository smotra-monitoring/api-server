package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/smotra-monitoring/server/internal/database"
	"gopkg.in/yaml.v3"
)

func TestLoadAndValidate_ValidYAML(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	defaultConfig := Default()
	defaultConfig.Server.Host = "127.0.0.1"
	defaultConfig.Server.Port = 9090
	defaultConfig.DatabaseType = "sqlite"
	defaultConfig.SQLiteConfig = &database.SQLiteConfig{
		FilePath: "/tmp/test.db",
		Config: database.Config{
			Type:            "sqlite",
			MaxOpenConns:    1,
			MaxIdleConns:    1,
			ConnMaxLifetime: 0,
			ConnMaxIdleTime: 0,
		},
	}
	defaultConfig.Logging.Level = "debug"

	yamlContent, err := yaml.Marshal(defaultConfig)
	if err != nil {
		t.Fatalf("Failed to marshal config to YAML: %v", err)
	}

	if err := os.WriteFile(configFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cfg, err := LoadAndValidate(configFile)
	if err != nil {
		t.Fatalf("LoadAndValidate failed: %v", err)
	}

	// Verify loaded values
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("Expected host 127.0.0.1, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("Expected port 9090, got %d", cfg.Server.Port)
	}
	if cfg.DatabaseType != "sqlite" {
		t.Errorf("Expected database type sqlite, got %s", cfg.DatabaseType)
	}
	if cfg.SQLiteConfig == nil {
		t.Fatal("SQLiteConfig is nil")
	}
	if cfg.SQLiteConfig.FilePath != "/tmp/test.db" {
		t.Errorf("Expected file path /tmp/test.db, got %s", cfg.SQLiteConfig.FilePath)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("Expected log level debug, got %s", cfg.Logging.Level)
	}
}

func TestLoadAndValidate_ValidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.json")

	defaultConfig := Default()
	defaultConfig.Server.Port = 8080
	defaultConfig.DatabaseType = "postgres"
	defaultConfig.PostgresConfig = &database.PostgresConfig{
		Host:     "db.example.com",
		Port:     5432,
		Username: "testuser",
		Password: "testpass",
		Database: "testdb",
		SSLMode:  "require",
	}

	jsonContent, err := json.Marshal(defaultConfig)
	if err != nil {
		t.Fatalf("Failed to marshal config to JSON: %v", err)
	}

	if err := os.WriteFile(configFile, []byte(jsonContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cfg, err := LoadAndValidate(configFile)
	if err != nil {
		t.Fatalf("LoadAndValidate failed: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("Expected port 8080, got %d", cfg.Server.Port)
	}
	if cfg.DatabaseType != "postgres" {
		t.Errorf("Expected database type postgres, got %s", cfg.DatabaseType)
	}
	if cfg.PostgresConfig == nil {
		t.Fatal("PostgresConfig is nil")
	}
	if cfg.PostgresConfig.Host != "db.example.com" {
		t.Errorf("Expected host db.example.com, got %s", cfg.PostgresConfig.Host)
	}
}

func TestLoadAndValidate_DatabasePasswordEnvOverride(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	defaultConfig := Default()
	defaultConfig.DatabaseType = "postgres"
	defaultConfig.PostgresConfig = &database.PostgresConfig{
		Host:     "localhost",
		Port:     5432,
		Username: "user",
		Password: "from-file",
		Database: "db",
		SSLMode:  "disable",
	}

	yamlContent, err := yaml.Marshal(defaultConfig)
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}
	if err := os.WriteFile(configFile, yamlContent, 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	t.Setenv("DATABASE_PASSWORD", "from-env")

	cfg, err := LoadAndValidate(configFile)
	if err != nil {
		t.Fatalf("LoadAndValidate failed: %v", err)
	}

	if cfg.PostgresConfig.Password != "from-env" {
		t.Errorf("Expected password from-env, got %s", cfg.PostgresConfig.Password)
	}
}

func TestLoadAndValidate_InvalidDatabaseType(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	defaultConfig := Default()
	defaultConfig.DatabaseType = "mysql"

	yamlContent, err := yaml.Marshal(defaultConfig)
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}
	if err := os.WriteFile(configFile, yamlContent, 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	_, err = LoadAndValidate(configFile)
	if err == nil {
		t.Error("Expected error for invalid database type, got nil")
	}
}

func TestLoadAndValidate_EmptyDatabaseType(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	defaultConfig := Default()
	defaultConfig.DatabaseType = ""

	yamlContent, err := yaml.Marshal(defaultConfig)
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}
	if err := os.WriteFile(configFile, yamlContent, 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	_, err = LoadAndValidate(configFile)
	if err == nil {
		t.Error("Expected error for empty database type, got nil")
	}
}

func TestLoadAndValidate_FileNotFound(t *testing.T) {
	_, err := LoadAndValidate("/nonexistent/config.yaml")
	if err == nil {
		t.Error("Expected error for nonexistent file, got nil")
	}
}

func TestLoadAndValidate_InvalidFile(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(configFile, []byte("invalid: yaml: content: ["), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	_, err := LoadAndValidate(configFile)
	if err == nil {
		t.Error("Expected error for invalid YAML, got nil")
	}
}

func TestConfig_Validate_InvalidPort(t *testing.T) {
	cfg := Default()
	cfg.Server.Port = 70000

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected validation error for invalid port, got nil")
	}
}

func TestConfig_Validate_InvalidDatabaseType(t *testing.T) {
	cfg := Default()
	cfg.DatabaseType = "mysql"

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected validation error for invalid database type, got nil")
	}
}

func TestConfig_Validate_PostgresMissingConfig(t *testing.T) {
	cfg := Default()
	cfg.DatabaseType = "postgres"
	cfg.PostgresConfig = nil

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected validation error for missing postgres config, got nil")
	}
}

func TestConfig_Validate_PostgresMissingHost(t *testing.T) {
	cfg := Default()
	cfg.DatabaseType = "postgres"
	cfg.PostgresConfig = &database.PostgresConfig{
		Username: "user",
		Password: "pass",
		Database: "db",
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected validation error for missing postgres host, got nil")
	}
}

func TestConfig_Validate_SQLiteMissingConfig(t *testing.T) {
	cfg := Default()
	cfg.DatabaseType = "sqlite"
	cfg.SQLiteConfig = nil

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected validation error for missing sqlite config, got nil")
	}
}

func TestConfig_Validate_SQLiteMissingFilePath(t *testing.T) {
	cfg := Default()
	cfg.DatabaseType = "sqlite"
	cfg.SQLiteConfig.FilePath = ""

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected validation error for missing sqlite file path, got nil")
	}
}

func TestConfig_Validate_InvalidLogLevel(t *testing.T) {
	cfg := Default()
	cfg.Logging.Level = "invalid"

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected validation error for invalid log level, got nil")
	}
}

func TestConfig_Validate_InvalidLogFormat(t *testing.T) {
	cfg := Default()
	cfg.Logging.Format = "xml"

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected validation error for invalid log format, got nil")
	}
}

func TestConfig_Validate_MissingFrontendCallbackURL(t *testing.T) {
	cfg := Default()
	cfg.Auth.FrontendCallbackURL = ""

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected validation error for missing frontend_callback_url, got nil")
	}
}

func TestConfig_Validate_TrustedProxies(t *testing.T) {
	for _, tc := range []struct {
		name    string
		proxies []string
		wantErr bool
	}{
		{"empty", nil, false},
		{"cidr and ip", []string{"10.0.0.0/8", "192.168.1.1", "fd00::/8"}, false},
		{"hostname rejected", []string{"proxy.example.com"}, true},
		{"bad mask rejected", []string{"10.0.0.0/33"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Default()
			cfg.Server.TrustedProxies = tc.proxies
			if err := cfg.Validate(); (err != nil) != tc.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestConfig_Validate_ValidConfig(t *testing.T) {
	cfg := Default()
	// Default() already sets FrontendCallbackURL and ServerCallbackURL.

	err := cfg.Validate()
	if err != nil {
		t.Errorf("Expected valid config, got error: %v", err)
	}
}

func TestDefault(t *testing.T) {
	cfg := Default()

	if cfg == nil {
		t.Fatal("Default() returned nil")
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("Expected default port 8080, got %d", cfg.Server.Port)
	}

	if cfg.DatabaseType != "sqlite" {
		t.Errorf("Expected default database type sqlite, got %s", cfg.DatabaseType)
	}

	if cfg.SQLiteConfig == nil {
		t.Error("Expected SQLiteConfig to be set")
	}

	if cfg.Logging.Level != "info" {
		t.Errorf("Expected default log level info, got %s", cfg.Logging.Level)
	}

	if cfg.Logging.Format != "json" {
		t.Errorf("Expected default log format json, got %s", cfg.Logging.Format)
	}

	if cfg.Auth.FrontendCallbackURL == "" {
		t.Error("Expected default FrontendCallbackURL to be non-empty")
	}

	if cfg.Auth.ServerCallbackURL == "" {
		t.Error("Expected default ServerCallbackURL to be non-empty")
	}
}

func TestConfig_Validate_AllLogLevels(t *testing.T) {
	validLevels := []string{"debug", "info", "warn", "error"}

	for _, level := range validLevels {
		t.Run(level, func(t *testing.T) {
			cfg := Default()
			cfg.Logging.Level = level

			if err := cfg.Validate(); err != nil {
				t.Errorf("Expected %s to be valid log level, got error: %v", level, err)
			}
		})
	}
}

func TestConfig_Validate_AllLogFormats(t *testing.T) {
	validFormats := []string{"json", "text"}

	for _, format := range validFormats {
		t.Run(format, func(t *testing.T) {
			cfg := Default()
			cfg.Logging.Format = format

			if err := cfg.Validate(); err != nil {
				t.Errorf("Expected %s to be valid log format, got error: %v", format, err)
			}
		})
	}
}

func enabledRateLimiterConfig() *Config {
	cfg := Default()
	cfg.RateLimiter.Enabled = true
	cfg.RateLimiter.URL = "http://127.0.0.1:9000/check"
	return cfg
}

func TestDefaultRateLimiterConfig(t *testing.T) {
	rl := Default().RateLimiter

	if rl.Enabled {
		t.Error("rate limiter must be disabled by default")
	}
	if rl.Timeout != 20*time.Millisecond {
		t.Errorf("Timeout = %s, want 20ms", rl.Timeout)
	}
	if !rl.FailOpen {
		t.Error("FailOpen must default to true")
	}
	if rl.Cache429Sec != 15 || rl.CircuitBreakerSec != 5 {
		t.Errorf("got cache_429_sec=%d circuit_breaker_sec=%d, want 15 and 5", rl.Cache429Sec, rl.CircuitBreakerSec)
	}
	if len(rl.ExemptPaths) != 2 || rl.ExemptPaths[0] != "/healthz" || rl.ExemptPaths[1] != "/metrics" {
		t.Errorf("ExemptPaths = %v, want [/healthz /metrics]", rl.ExemptPaths)
	}
	if err := Default().Validate(); err != nil {
		t.Errorf("default config must be valid: %v", err)
	}
}

func TestConfig_Validate_RateLimiter(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mutate  func(*RateLimiterConfig)
		wantErr bool
	}{
		{"enabled with defaults", func(*RateLimiterConfig) {}, false},
		{"empty url", func(r *RateLimiterConfig) { r.URL = "" }, true},
		{"non-http url", func(r *RateLimiterConfig) { r.URL = "ftp://host/x" }, true},
		{"url without host", func(r *RateLimiterConfig) { r.URL = "http://" }, true},
		{"timeout zero", func(r *RateLimiterConfig) { r.Timeout = 0 }, true},
		{"timeout below min", func(r *RateLimiterConfig) { r.Timeout = 500 * time.Microsecond }, true},
		{"timeout at min", func(r *RateLimiterConfig) { r.Timeout = time.Millisecond }, false},
		{"timeout at max", func(r *RateLimiterConfig) { r.Timeout = 20 * time.Millisecond }, false},
		{"timeout above max", func(r *RateLimiterConfig) { r.Timeout = 21 * time.Millisecond }, true},
		{"cache_429_sec below min", func(r *RateLimiterConfig) { r.Cache429Sec = 4 }, true},
		{"cache_429_sec at min", func(r *RateLimiterConfig) { r.Cache429Sec = 5 }, false},
		{"circuit_breaker_sec below min", func(r *RateLimiterConfig) { r.CircuitBreakerSec = 0 }, true},
		{"circuit_breaker_sec at min", func(r *RateLimiterConfig) { r.CircuitBreakerSec = 1 }, false},
		{"exempt path without slash", func(r *RateLimiterConfig) { r.ExemptPaths = []string{"healthz"} }, true},
		{"whitelist cidr and ip", func(r *RateLimiterConfig) { r.WhitelistSubnets = []string{"10.0.0.0/8", "192.168.1.1", "fd00::/8"} }, false},
		{"whitelist bad mask", func(r *RateLimiterConfig) { r.WhitelistSubnets = []string{"10.0.0.0/33"} }, true},
		{"whitelist hostname", func(r *RateLimiterConfig) { r.WhitelistSubnets = []string{"example.com"} }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := enabledRateLimiterConfig()
			tc.mutate(&cfg.RateLimiter)
			if err := cfg.Validate(); (err != nil) != tc.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestConfig_Validate_RateLimiterDisabledIgnoresValues(t *testing.T) {
	cfg := Default()
	cfg.RateLimiter = RateLimiterConfig{Enabled: false, URL: "bogus", Timeout: time.Hour}

	if err := cfg.Validate(); err != nil {
		t.Errorf("disabled rate limiter must not be validated: %v", err)
	}
}

func TestLoadAndValidate_RateLimiterDefaultsApplied(t *testing.T) {
	cfgFile := filepath.Join(t.TempDir(), "config.yaml")

	// Start from a full valid config, then replace the rate_limiter section with a minimal one.
	data, err := yaml.Marshal(Default())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	raw["rate_limiter"] = map[string]any{"enabled": true, "url": "http://127.0.0.1:9000/check"}
	data, err = yaml.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(cfgFile, data, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg, err := LoadAndValidate(cfgFile)
	if err != nil {
		t.Fatalf("LoadAndValidate: %v", err)
	}

	rl := cfg.RateLimiter
	if !rl.Enabled || rl.Timeout != 20*time.Millisecond || !rl.FailOpen || rl.Cache429Sec != 15 || rl.CircuitBreakerSec != 5 {
		t.Errorf("omitted keys must keep defaults, got %+v", rl)
	}
}

func TestLoadAndValidate_RateLimiterYAMLOverrides(t *testing.T) {
	cfgFile := filepath.Join(t.TempDir(), "config.yaml")

	cfg := enabledRateLimiterConfig()
	cfg.RateLimiter.Timeout = 10 * time.Millisecond
	cfg.RateLimiter.FailOpen = false
	cfg.RateLimiter.WhitelistSubnets = []string{"10.0.0.0/8"}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(cfgFile, data, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	loaded, err := LoadAndValidate(cfgFile)
	if err != nil {
		t.Fatalf("LoadAndValidate: %v", err)
	}
	if loaded.RateLimiter.Timeout != 10*time.Millisecond || loaded.RateLimiter.FailOpen {
		t.Errorf("overrides not applied: %+v", loaded.RateLimiter)
	}
	if len(loaded.RateLimiter.WhitelistSubnets) != 1 || loaded.RateLimiter.WhitelistSubnets[0] != "10.0.0.0/8" {
		t.Errorf("whitelist not loaded: %v", loaded.RateLimiter.WhitelistSubnets)
	}
}
