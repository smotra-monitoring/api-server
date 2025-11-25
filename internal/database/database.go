package database

import (
	"context"
	"database/sql"
	"time"
)

// Database defines the interface for database operations
type Database interface {
	// Connection management
	Open(ctx context.Context) error
	Close() error
	Ping(ctx context.Context) error
	
	// Health check
	Health(ctx context.Context) (HealthInfo, error)
	
	// Transaction management
	BeginTx(ctx context.Context) (*sql.Tx, error)
	
	// Direct access to underlying connection (for migrations, etc.)
	DB() *sql.DB
}

// HealthInfo contains database health information
type HealthInfo struct {
	Status       string        `json:"status"`
	ResponseTime time.Duration `json:"response_time_ms"`
	OpenConns    int           `json:"open_connections"`
	IdleConns    int           `json:"idle_connections"`
	Message      string        `json:"message,omitempty"`
}

// Config holds database configuration
type Config struct {
	Type            string
	Host            string
	Port            int
	Username        string
	Password        string
	Database        string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	FilePath        string // For SQLite
}
