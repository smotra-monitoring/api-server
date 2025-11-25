package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// SQLiteDB implements the Database interface for SQLite
type SQLiteDB struct {
	db     *sql.DB
	config SQLiteConfig
}

// NewSQLiteDB creates a new SQLite database instance
func NewSQLiteDB(config SQLiteConfig) *SQLiteDB {
	return &SQLiteDB{
		config: config,
	}
}

// Open establishes a connection to the SQLite database
func (s *SQLiteDB) Open(ctx context.Context) error {
	// Ensure the directory exists
	dir := filepath.Dir(s.config.FilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open database with additional pragmas for better performance
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&_cache_size=1000000",
		s.config.FilePath)

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// SQLite benefits from limited connections in WAL mode
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0) // Connections never expire
	db.SetConnMaxIdleTime(0)

	// Verify connection
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("failed to ping database: %w", err)
	}

	// Enable foreign keys
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	s.db = db
	return nil
}

// Close closes the database connection
func (s *SQLiteDB) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// Ping checks the database connection
func (s *SQLiteDB) Ping(ctx context.Context) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}
	return s.db.PingContext(ctx)
}

// Health returns health information about the database
func (s *SQLiteDB) Health(ctx context.Context) (HealthInfo, error) {
	start := time.Now()

	info := HealthInfo{
		Status: "unhealthy",
	}

	if s.db == nil {
		info.Message = "database not initialized"
		return info, fmt.Errorf("database not initialized")
	}

	// Ping the database
	if err := s.db.PingContext(ctx); err != nil {
		info.Message = fmt.Sprintf("ping failed: %v", err)
		info.ResponseTime = time.Since(start)
		return info, err
	}

	info.ResponseTime = time.Since(start)

	// Get connection stats
	stats := s.db.Stats()
	info.OpenConns = stats.OpenConnections
	info.IdleConns = stats.Idle
	info.Status = "healthy"
	info.Message = fmt.Sprintf("connected to SQLite (%s)", s.config.FilePath)

	return info, nil
}

// BeginTx starts a new transaction
func (s *SQLiteDB) BeginTx(ctx context.Context) (*sql.Tx, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return s.db.BeginTx(ctx, nil)
}

// DB returns the underlying sql.DB instance
func (s *SQLiteDB) DB() *sql.DB {
	return s.db
}
