package database

import (
	"fmt"
)

// New creates a new database instance based on the provided configuration
// For postgres, pass PostgresConfig; for sqlite, pass SQLiteConfig
func New(cfg interface{}) (Database, error) {
	switch c := cfg.(type) {
	case PostgresConfig:
		return NewPostgresDB(c), nil
	case *PostgresConfig:
		if c == nil {
			return nil, fmt.Errorf("postgres config is nil")
		}
		return NewPostgresDB(*c), nil
	case SQLiteConfig:
		return NewSQLiteDB(c), nil
	case *SQLiteConfig:
		if c == nil {
			return nil, fmt.Errorf("sqlite config is nil")
		}
		return NewSQLiteDB(*c), nil
	default:
		return nil, fmt.Errorf("unsupported database config type: %T", cfg)
	}
}
