package database

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteDB_Integration_OpenAndClose(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := SQLiteConfig{
		Config: Config{
			Type:            "sqlite",
			MaxOpenConns:    1,
			MaxIdleConns:    1,
			ConnMaxLifetime: 0,
			ConnMaxIdleTime: 0,
		},
		FilePath: dbPath,
	}

	db := NewSQLiteDB(cfg)
	ctx := context.Background()

	// Test Open
	err := db.Open(ctx)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}

	// Verify database file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Database file was not created")
	}

	// Test Close
	err = db.Close()
	if err != nil {
		t.Errorf("Close() failed: %v", err)
	}
}

func TestSQLiteDB_Integration_Ping(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := DefaultSQLiteConfig()
	cfg.FilePath = dbPath

	db := NewSQLiteDB(cfg)
	ctx := context.Background()

	err := db.Open(ctx)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer db.Close()

	// Test Ping
	err = db.Ping(ctx)
	if err != nil {
		t.Errorf("Ping() failed: %v", err)
	}
}

func TestSQLiteDB_Integration_PingBeforeOpen(t *testing.T) {
	cfg := DefaultSQLiteConfig()
	cfg.FilePath = "/tmp/test-not-opened.db"

	db := NewSQLiteDB(cfg)
	ctx := context.Background()

	// Test Ping before Open
	err := db.Ping(ctx)
	if err == nil {
		t.Error("Expected Ping() to fail before Open()")
	}
}

func TestSQLiteDB_Integration_Health(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := DefaultSQLiteConfig()
	cfg.FilePath = dbPath

	db := NewSQLiteDB(cfg)
	ctx := context.Background()

	err := db.Open(ctx)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer db.Close()

	// Test Health
	health, err := db.Health(ctx)
	if err != nil {
		t.Errorf("Health() failed: %v", err)
	}

	if health.Status != "healthy" {
		t.Errorf("Expected status 'healthy', got %s", health.Status)
	}

	// ResponseTime should be non-negative (can be 0 on very fast systems)
	if health.ResponseTime < 0 {
		t.Error("Expected ResponseTime to be non-negative")
	}

	if health.Message != "" {
		if len(health.Message) >= 20 && health.Message[:20] != "connected to SQLite " {
			t.Errorf("Unexpected message: %s", health.Message)
		}
	}
}

func TestSQLiteDB_Integration_HealthBeforeOpen(t *testing.T) {
	cfg := DefaultSQLiteConfig()
	cfg.FilePath = "/tmp/test-not-opened.db"

	db := NewSQLiteDB(cfg)
	ctx := context.Background()

	// Test Health before Open
	health, err := db.Health(ctx)
	if err == nil {
		t.Error("Expected Health() to fail before Open()")
	}

	if health.Status != "unhealthy" {
		t.Errorf("Expected status 'unhealthy', got %s", health.Status)
	}
}

func TestSQLiteDB_Integration_BeginTx(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := DefaultSQLiteConfig()
	cfg.FilePath = dbPath

	db := NewSQLiteDB(cfg)
	ctx := context.Background()

	err := db.Open(ctx)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer db.Close()

	// Test BeginTx
	tx, err := db.BeginTx(ctx)
	if err != nil {
		t.Fatalf("BeginTx() failed: %v", err)
	}

	if tx == nil {
		t.Fatal("BeginTx() returned nil transaction")
	}

	// Rollback the transaction
	err = tx.Rollback()
	if err != nil {
		t.Errorf("Rollback() failed: %v", err)
	}
}

func TestSQLiteDB_Integration_BeginTxBeforeOpen(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := DefaultSQLiteConfig()
	cfg.FilePath = dbPath

	db := NewSQLiteDB(cfg)
	ctx := context.Background()

	// Test BeginTx before Open
	_, err := db.BeginTx(ctx)
	if err == nil {
		t.Error("Expected BeginTx() to fail before Open()")
	}
}

func TestSQLiteDB_Integration_DB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := DefaultSQLiteConfig()
	cfg.FilePath = dbPath

	db := NewSQLiteDB(cfg)
	ctx := context.Background()

	err := db.Open(ctx)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer db.Close()

	// Test DB()
	sqlDB := db.DB()
	if sqlDB == nil {
		t.Fatal("DB() returned nil")
	}

	// Verify we can use the underlying DB
	err = sqlDB.PingContext(ctx)
	if err != nil {
		t.Errorf("Ping on underlying DB failed: %v", err)
	}
}

func TestSQLiteDB_Integration_CreateTable(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := DefaultSQLiteConfig()
	cfg.FilePath = dbPath

	db := NewSQLiteDB(cfg)
	ctx := context.Background()

	err := db.Open(ctx)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer db.Close()

	// Create a test table
	_, err = db.DB().ExecContext(ctx, `
		CREATE TABLE test_table (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Insert data
	result, err := db.DB().ExecContext(ctx, `
		INSERT INTO test_table (name) VALUES (?)
	`, "test_name")
	if err != nil {
		t.Fatalf("Failed to insert data: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("Failed to get last insert id: %v", err)
	}

	if id != 1 {
		t.Errorf("Expected last insert id 1, got %d", id)
	}

	// Query data
	var name string
	err = db.DB().QueryRowContext(ctx, `
		SELECT name FROM test_table WHERE id = ?
	`, id).Scan(&name)
	if err != nil {
		t.Fatalf("Failed to query data: %v", err)
	}

	if name != "test_name" {
		t.Errorf("Expected name 'test_name', got %s", name)
	}
}

func TestSQLiteDB_Integration_Transaction(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := DefaultSQLiteConfig()
	cfg.FilePath = dbPath

	db := NewSQLiteDB(cfg)
	ctx := context.Background()

	err := db.Open(ctx)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer db.Close()

	// Create a test table
	_, err = db.DB().ExecContext(ctx, `
		CREATE TABLE test_table (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			value TEXT NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Test transaction commit
	tx, err := db.BeginTx(ctx)
	if err != nil {
		t.Fatalf("BeginTx() failed: %v", err)
	}

	_, err = tx.ExecContext(ctx, `INSERT INTO test_table (value) VALUES (?)`, "value1")
	if err != nil {
		t.Fatalf("Failed to insert in transaction: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	// Verify data was committed
	var count int
	err = db.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM test_table`).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query count: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected count 1, got %d", count)
	}

	// Test transaction rollback
	tx, err = db.BeginTx(ctx)
	if err != nil {
		t.Fatalf("BeginTx() failed: %v", err)
	}

	_, err = tx.ExecContext(ctx, `INSERT INTO test_table (value) VALUES (?)`, "value2")
	if err != nil {
		t.Fatalf("Failed to insert in transaction: %v", err)
	}

	err = tx.Rollback()
	if err != nil {
		t.Fatalf("Failed to rollback transaction: %v", err)
	}

	// Verify data was not committed
	err = db.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM test_table`).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query count: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected count 1 after rollback, got %d", count)
	}
}

func TestSQLiteDB_Integration_ForeignKeys(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := DefaultSQLiteConfig()
	cfg.FilePath = dbPath

	db := NewSQLiteDB(cfg)
	ctx := context.Background()

	err := db.Open(ctx)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer db.Close()

	// Verify foreign keys are enabled
	var fkEnabled int
	err = db.DB().QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&fkEnabled)
	if err != nil {
		t.Fatalf("Failed to query foreign_keys pragma: %v", err)
	}

	if fkEnabled != 1 {
		t.Error("Foreign keys are not enabled")
	}
}

func TestSQLiteDB_Integration_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := DefaultSQLiteConfig()
	cfg.FilePath = dbPath

	db := NewSQLiteDB(cfg)
	ctx := context.Background()

	err := db.Open(ctx)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer db.Close()

	// Create a test table
	_, err = db.DB().ExecContext(ctx, `
		CREATE TABLE test_table (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			value INTEGER NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Concurrent writes (SQLite WAL mode should handle this)
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(val int) {
			_, err := db.DB().ExecContext(ctx, `INSERT INTO test_table (value) VALUES (?)`, val)
			if err != nil {
				t.Errorf("Concurrent insert failed: %v", err)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all inserts succeeded
	var count int
	err = db.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM test_table`).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query count: %v", err)
	}

	if count != 10 {
		t.Errorf("Expected count 10, got %d", count)
	}
}

func TestSQLiteDB_Integration_HealthResponseTime(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := DefaultSQLiteConfig()
	cfg.FilePath = dbPath

	db := NewSQLiteDB(cfg)
	ctx := context.Background()

	err := db.Open(ctx)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer db.Close()

	// Test Health response time
	health, err := db.Health(ctx)
	if err != nil {
		t.Fatalf("Health() failed: %v", err)
	}

	if health.ResponseTime < 0 {
		t.Error("ResponseTime should be non-negative")
	}

	if health.ResponseTime > 1*time.Second {
		t.Errorf("ResponseTime too high: %v", health.ResponseTime)
	}
}
