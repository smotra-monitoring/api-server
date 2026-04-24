package testutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/smotra-monitoring/server/internal/config"
)

// CreateTestConfigYAML creates a temporary YAML config file for testing
func CreateTestConfigYAML(t *testing.T, content string) string {
	t.Helper()

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	return configFile
}

// CreateTestConfigJSON creates a temporary JSON config file for testing
func CreateTestConfigJSON(t *testing.T, content string) string {
	t.Helper()

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.json")

	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	return configFile
}

// DefaultTestConfig returns a default config suitable for testing
func DefaultTestConfig() *config.Config {
	cfg := config.Default()
	cfg.Auth.FrontendCallbackURL = "http://localhost:3000/auth/callback"
	cfg.Auth.ServerCallbackURL = "http://localhost:8080/v1/auth/oauth2/callback"
	return cfg
}
