package database

import (
	"context"
	"fmt"
)

// New creates a new database instance based on the configuration
func New(config Config) (Database, error) {
	switch config.Type {
	case "postgres":
		return NewPostgresDB(config), nil
	case "sqlite":
		return NewSQLiteDB(config), nil
	default:
		return nil, fmt.Errorf("unsupported database type: %s", config.Type)
	}
}

// MustOpen opens a database connection or panics
func MustOpen(ctx context.Context, config Config) Database {
	db, err := New(config)
	if err != nil {
		panic(fmt.Sprintf("failed to create database: %v", err))
	}

	if err := db.Open(ctx); err != nil {
		panic(fmt.Sprintf("failed to open database: %v", err))
	}

	return db
}
