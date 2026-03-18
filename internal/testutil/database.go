package testutil

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

// ApplyMigrations reads and applies all migration files from the migrations directory
// in order based on their numeric prefix (e.g., 0001_, 0002_, etc.)
func ApplyMigrations(t *testing.T, ctx context.Context, db *sql.DB, migrationsDir string) {
	t.Helper()

	// Read all migration files
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("Failed to read migrations directory: %v", err)
	}

	// Filter and sort .up.sql files by their numeric prefix
	var migrationFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".up.sql") {
			migrationFiles = append(migrationFiles, entry.Name())
		}
	}

	// Sort by name (which sorts by numeric prefix due to naming convention)
	sort.Strings(migrationFiles)

	// Apply each migration in order
	for _, filename := range migrationFiles {
		migrationPath := filepath.Join(migrationsDir, filename)
		migrationSQL, err := os.ReadFile(migrationPath)
		if err != nil {
			t.Fatalf("Failed to read migration file %s: %v", filename, err)
		}

		_, err = db.ExecContext(ctx, string(migrationSQL))
		if err != nil {
			t.Fatalf("Failed to apply migration %s: %v", filename, err)
		}

		t.Logf("Applied migration: %s", filename)
	}
}
