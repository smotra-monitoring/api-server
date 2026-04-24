package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

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
