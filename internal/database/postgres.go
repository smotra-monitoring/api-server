package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
)

// PostgresDB implements the Database interface for PostgreSQL
type PostgresDB struct {
	db     *sql.DB
	config PostgresConfig
}

// NewPostgresDB creates a new PostgreSQL database instance
func NewPostgresDB(config PostgresConfig) *PostgresDB {
	return &PostgresDB{
		config: config,
	}
}

// Open establishes a connection to the PostgreSQL database
func (p *PostgresDB) Open(ctx context.Context) error {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		p.config.Host,
		p.config.Port,
		p.config.Username,
		p.config.Password,
		p.config.Database,
		p.config.SSLMode,
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(p.config.MaxOpenConns)
	db.SetMaxIdleConns(p.config.MaxIdleConns)
	db.SetConnMaxLifetime(p.config.ConnMaxLifetime)
	db.SetConnMaxIdleTime(p.config.ConnMaxIdleTime)

	// Verify connection
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("failed to ping database: %w", err)
	}

	p.db = db
	return nil
}

// Close closes the database connection
func (p *PostgresDB) Close() error {
	if p.db != nil {
		return p.db.Close()
	}
	return nil
}

// Ping checks the database connection
func (p *PostgresDB) Ping(ctx context.Context) error {
	if p.db == nil {
		return fmt.Errorf("database not initialized")
	}
	return p.db.PingContext(ctx)
}

// Health returns health information about the database
func (p *PostgresDB) Health(ctx context.Context) (HealthInfo, error) {
	start := time.Now()

	info := HealthInfo{
		Status: "unhealthy",
	}

	if p.db == nil {
		info.Message = "database not initialized"
		return info, fmt.Errorf("database not initialized")
	}

	// Ping the database
	if err := p.db.PingContext(ctx); err != nil {
		info.Message = fmt.Sprintf("ping failed: %v", err)
		info.ResponseTime = time.Since(start)
		return info, err
	}

	info.ResponseTime = time.Since(start)

	// Get connection stats
	stats := p.db.Stats()
	info.OpenConns = stats.OpenConnections
	info.IdleConns = stats.Idle
	info.Status = "healthy"
	info.Message = "connected to PostgreSQL"

	return info, nil
}

// BeginTx starts a new transaction
func (p *PostgresDB) BeginTx(ctx context.Context) (*sql.Tx, error) {
	if p.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return p.db.BeginTx(ctx, nil)
}

// DB returns the underlying sql.DB instance
func (p *PostgresDB) DB() *sql.DB {
	return p.db
}
