package testutil

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/smotra-monitoring/server/internal/database"
)

// SetupTestSQLiteDB creates a temporary SQLite database for testing
func SetupTestSQLiteDB(t *testing.T) database.Database {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := database.SQLiteConfig{
		Config: database.Config{
			Type:            "sqlite",
			MaxOpenConns:    1,
			MaxIdleConns:    1,
			ConnMaxLifetime: 0,
			ConnMaxIdleTime: 0,
		},
		FilePath: dbPath,
	}

	db := database.NewSQLiteDB(cfg)
	ctx := context.Background()

	if err := db.Open(ctx); err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
	})

	return db
}
