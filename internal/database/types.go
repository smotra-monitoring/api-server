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
	Status              string        `json:"status"`
	ResponseTime        time.Duration `json:"response_time_ms"`
	DBOpenConns         int           `json:"db_open_connections"`
	DBInUseConns        int           `json:"db_in_use_connections"`
	DBIdleConns         int           `json:"db_idle_connections"`
	DBWaitConnsCount    int64         `json:"db_wait_connections_count"`
	DBWaitConnsDuration time.Duration `json:"db_wait_connections_duration_ms"`
	Message             string        `json:"message,omitempty"`
}

// Config holds common database configuration
type Config struct {
	Type            string        `json:"type" yaml:"type"`
	MaxOpenConns    int           `json:"max_open_conns" yaml:"maxopenconns"`
	MaxIdleConns    int           `json:"max_idle_conns" yaml:"maxidleconns"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime" yaml:"connmaxlifetime"`
	ConnMaxIdleTime time.Duration `json:"conn_max_idle_time" yaml:"connmaxidletime"`
}

// PostgresConfig holds PostgreSQL-specific configuration
type PostgresConfig struct {
	Config   `yaml:",inline"`
	Host     string `json:"host" yaml:"host"`
	Port     int    `json:"port" yaml:"port"`
	Username string `json:"username" yaml:"username"`
	Password string `json:"password" yaml:"password"`
	Database string `json:"database" yaml:"database"`
	SSLMode  string `json:"sslmode" yaml:"sslmode"`
}

// SQLiteConfig holds SQLite-specific configuration
type SQLiteConfig struct {
	Config   `yaml:"config,inline"`
	FilePath string `json:"file_path" yaml:"filepath"`
}

// DefaultConfig returns a default database configuration
func DefaultConfig() Config {
	return Config{
		Type:            "",
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: 15 * time.Minute,
		ConnMaxIdleTime: 5 * time.Minute,
	}
}

func DefaultSQLiteConfig() SQLiteConfig {
	config := DefaultConfig()
	config.Type = "sqlite"
	// SQLite with WAL mode benefits from limited connections to avoid write contention
	config.MaxOpenConns = 1
	config.MaxIdleConns = 1
	config.ConnMaxLifetime = 0 // Connections never expire
	config.ConnMaxIdleTime = 0
	return SQLiteConfig{
		Config:   config,
		FilePath: "./data/smotra.db",
	}
}

func DefaultPostgresConfig() PostgresConfig {
	config := DefaultConfig()
	config.Type = "postgres"
	return PostgresConfig{
		Config:   config,
		Host:     "localhost",
		Port:     5432,
		Username: "smotra",
		Password: "changeme",
		Database: "smotra",
		SSLMode:  "disable",
	}
}
