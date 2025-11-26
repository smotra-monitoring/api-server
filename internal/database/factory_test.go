package database

import (
	"testing"
	"time"
)

func TestNew_PostgresConfig(t *testing.T) {
	cfg := PostgresConfig{
		Config: Config{
			Type:            "postgres",
			MaxOpenConns:    25,
			MaxIdleConns:    5,
			ConnMaxLifetime: 15 * time.Minute,
			ConnMaxIdleTime: 5 * time.Minute,
		},
		Host:     "localhost",
		Port:     5432,
		Username: "testuser",
		Password: "testpass",
		Database: "testdb",
		SSLMode:  "disable",
	}

	db, err := New(cfg)
	if err != nil {
		t.Fatalf("New() with PostgresConfig failed: %v", err)
	}

	if db == nil {
		t.Fatal("New() returned nil database")
	}

	_, ok := db.(*PostgresDB)
	if !ok {
		t.Error("Expected PostgresDB type")
	}
}

func TestNew_PostgresConfigPointer(t *testing.T) {
	cfg := &PostgresConfig{
		Config: Config{
			Type: "postgres",
		},
		Host:     "localhost",
		Port:     5432,
		Username: "testuser",
		Password: "testpass",
		Database: "testdb",
		SSLMode:  "disable",
	}

	db, err := New(cfg)
	if err != nil {
		t.Fatalf("New() with *PostgresConfig failed: %v", err)
	}

	if db == nil {
		t.Fatal("New() returned nil database")
	}
}

func TestNew_PostgresConfigNilPointer(t *testing.T) {
	var cfg *PostgresConfig = nil

	_, err := New(cfg)
	if err == nil {
		t.Error("Expected error for nil PostgresConfig pointer")
	}
}

func TestNew_SQLiteConfig(t *testing.T) {
	cfg := SQLiteConfig{
		Config: Config{
			Type:            "sqlite",
			MaxOpenConns:    1,
			MaxIdleConns:    1,
			ConnMaxLifetime: 0,
			ConnMaxIdleTime: 0,
		},
		FilePath: "/tmp/test.db",
	}

	db, err := New(cfg)
	if err != nil {
		t.Fatalf("New() with SQLiteConfig failed: %v", err)
	}

	if db == nil {
		t.Fatal("New() returned nil database")
	}

	_, ok := db.(*SQLiteDB)
	if !ok {
		t.Error("Expected SQLiteDB type")
	}
}

func TestNew_SQLiteConfigPointer(t *testing.T) {
	cfg := &SQLiteConfig{
		Config: Config{
			Type: "sqlite",
		},
		FilePath: "/tmp/test.db",
	}

	db, err := New(cfg)
	if err != nil {
		t.Fatalf("New() with *SQLiteConfig failed: %v", err)
	}

	if db == nil {
		t.Fatal("New() returned nil database")
	}
}

func TestNew_SQLiteConfigNilPointer(t *testing.T) {
	var cfg *SQLiteConfig = nil

	_, err := New(cfg)
	if err == nil {
		t.Error("Expected error for nil SQLiteConfig pointer")
	}
}

func TestNew_UnsupportedConfigType(t *testing.T) {
	type UnsupportedConfig struct {
		Value string
	}

	cfg := UnsupportedConfig{Value: "test"}

	_, err := New(cfg)
	if err == nil {
		t.Error("Expected error for unsupported config type")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.MaxOpenConns != 25 {
		t.Errorf("Expected MaxOpenConns 25, got %d", cfg.MaxOpenConns)
	}

	if cfg.MaxIdleConns != 5 {
		t.Errorf("Expected MaxIdleConns 5, got %d", cfg.MaxIdleConns)
	}

	if cfg.ConnMaxLifetime != 15*time.Minute {
		t.Errorf("Expected ConnMaxLifetime 15m, got %v", cfg.ConnMaxLifetime)
	}

	if cfg.ConnMaxIdleTime != 5*time.Minute {
		t.Errorf("Expected ConnMaxIdleTime 5m, got %v", cfg.ConnMaxIdleTime)
	}
}

func TestDefaultSQLiteConfig(t *testing.T) {
	cfg := DefaultSQLiteConfig()

	if cfg.Config.Type != "sqlite" {
		t.Errorf("Expected Type 'sqlite', got %s", cfg.Config.Type)
	}

	if cfg.FilePath == "" {
		t.Error("Expected FilePath to be set")
	}

	if cfg.FilePath != "./data/smotra.db" {
		t.Errorf("Expected FilePath './data/smotra.db', got %s", cfg.FilePath)
	}
}

func TestDefaultPostgresConfig(t *testing.T) {
	cfg := DefaultPostgresConfig()

	if cfg.Config.Type != "postgres" {
		t.Errorf("Expected Type 'postgres', got %s", cfg.Config.Type)
	}

	if cfg.Host != "localhost" {
		t.Errorf("Expected Host 'localhost', got %s", cfg.Host)
	}

	if cfg.Port != 5432 {
		t.Errorf("Expected Port 5432, got %d", cfg.Port)
	}

	if cfg.Username != "smotra" {
		t.Errorf("Expected Username 'smotra', got %s", cfg.Username)
	}

	if cfg.Database != "smotra" {
		t.Errorf("Expected Database 'smotra', got %s", cfg.Database)
	}

	if cfg.SSLMode != "disable" {
		t.Errorf("Expected SSLMode 'disable', got %s", cfg.SSLMode)
	}
}

func TestNewPostgresDB(t *testing.T) {
	cfg := DefaultPostgresConfig()
	db := NewPostgresDB(cfg)

	if db == nil {
		t.Fatal("NewPostgresDB returned nil")
	}

	if db.config.Host != cfg.Host {
		t.Errorf("Config not properly set")
	}
}

func TestNewSQLiteDB(t *testing.T) {
	cfg := DefaultSQLiteConfig()
	db := NewSQLiteDB(cfg)

	if db == nil {
		t.Fatal("NewSQLiteDB returned nil")
	}

	if db.config.FilePath != cfg.FilePath {
		t.Errorf("Config not properly set")
	}
}
