package config

import (
	"testing"

	"github.com/smotra-monitoring/server/internal/database"
)

func TestTypes_PostgresConfig(t *testing.T) {
	cfg := Default()
	cfg.DatabaseType = "postgres"
	pgsql := database.DefaultPostgresConfig()
	cfg.PostgresConfig = &pgsql

	if err := cfg.Validate(); err != nil {
		t.Errorf("Valid postgres config failed validation: %v", err)
	}
}

func TestTypes_SQLiteConfig(t *testing.T) {
	cfg := Default()
	if cfg.SQLiteConfig == nil {
		t.Fatal("Default SQLiteConfig should not be nil")
	}

	if cfg.SQLiteConfig.FilePath == "" {
		t.Error("Default SQLiteConfig FilePath should not be empty")
	}
}
