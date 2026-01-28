package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/smotra-monitoring/server/internal/database"
)

// MockDatabase is a mock implementation of the Database interface for testing
type MockDatabase struct {
	OpenFunc     func(ctx context.Context) error
	CloseFunc    func() error
	PingFunc     func(ctx context.Context) error
	HealthFunc   func(ctx context.Context) (database.HealthInfo, error)
	BeginTxFunc  func(ctx context.Context) (*sql.Tx, error)
	DBFunc       func() *sql.DB
	ShouldFail   bool
	HealthStatus string
	QueryResults map[string]func(ctx context.Context, args ...interface{}) (interface{}, error)
	db           *sql.DB
}

// NewMockDatabase creates a new mock database
func NewMockDatabase() *MockDatabase {
	return &MockDatabase{
		HealthStatus: "healthy",
		ShouldFail:   false,
		QueryResults: make(map[string]func(ctx context.Context, args ...interface{}) (interface{}, error)),
	}
}

// SetQueryResult sets a mock result for a specific query
func (m *MockDatabase) SetQueryResult(queryName string, fn func(ctx context.Context, args ...interface{}) (interface{}, error)) {
	m.QueryResults[queryName] = fn
}

// Open mocks the Open method
func (m *MockDatabase) Open(ctx context.Context) error {
	if m.OpenFunc != nil {
		return m.OpenFunc(ctx)
	}
	if m.ShouldFail {
		return fmt.Errorf("mock open error")
	}
	return nil
}

// Close mocks the Close method
func (m *MockDatabase) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	if m.ShouldFail {
		return fmt.Errorf("mock close error")
	}
	return nil
}

// Ping mocks the Ping method
func (m *MockDatabase) Ping(ctx context.Context) error {
	if m.PingFunc != nil {
		return m.PingFunc(ctx)
	}
	if m.ShouldFail {
		return fmt.Errorf("mock ping error")
	}
	return nil
}

// Health mocks the Health method
func (m *MockDatabase) Health(ctx context.Context) (database.HealthInfo, error) {
	if m.HealthFunc != nil {
		return m.HealthFunc(ctx)
	}

	info := database.HealthInfo{
		Status:       m.HealthStatus,
		ResponseTime: 10 * time.Millisecond,
		OpenConns:    5,
		IdleConns:    3,
		Message:      "mock database",
	}

	if m.ShouldFail {
		info.Status = "unhealthy"
		return info, fmt.Errorf("mock health error")
	}

	return info, nil
}

// BeginTx mocks the BeginTx method
func (m *MockDatabase) BeginTx(ctx context.Context) (*sql.Tx, error) {
	if m.BeginTxFunc != nil {
		return m.BeginTxFunc(ctx)
	}
	if m.ShouldFail {
		return nil, fmt.Errorf("mock begin tx error")
	}
	return nil, nil
}

// DB mocks the DB method
func (m *MockDatabase) DB() *sql.DB {
	if m.DBFunc != nil {
		return m.DBFunc()
	}
	// Return a mock DB if one is set
	if m.db != nil {
		return m.db
	}
	return nil
}

// SetDB sets the mock database connection
func (m *MockDatabase) SetDB(db *sql.DB) {
	m.db = db
}
