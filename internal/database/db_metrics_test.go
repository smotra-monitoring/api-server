package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestDBMetrics_GetMetrics_Format(t *testing.T) {
	sqlDB := newTestSQLDB(t)

	m := NewDBMetrics(&staticDB{db: sqlDB})
	out := m.GetMetrics()

	for _, expected := range []string{
		"# HELP smotra_db_healthy",
		"# TYPE smotra_db_healthy gauge",
		"smotra_db_healthy 1",
		"smotra_db_response_time_ms",
		"# HELP smotra_db_connections_open",
		"# TYPE smotra_db_connections_open gauge",
		"smotra_db_connections_open",
		"smotra_db_connections_in_use",
		"smotra_db_connections_idle",
		"smotra_db_wait_count_total",
		"smotra_db_wait_duration_ms",
	} {
		if !strings.Contains(out, expected) {
			t.Errorf("expected %q in GetMetrics output:\n%s", expected, out)
		}
	}
}

func TestDBMetrics_GetMetrics_UnhealthyDB(t *testing.T) {
	sqlDB := newTestSQLDB(t)

	failingDB := &staticDB{db: sqlDB}
	failingDB.fail = true

	m := NewDBMetrics(failingDB)
	out := m.GetMetrics()

	if !strings.Contains(out, "smotra_db_healthy 0") {
		t.Errorf("expected smotra_db_healthy 0 for failing db, got:\n%s", out)
	}
	if strings.Contains(out, "smotra_db_response_time_ms") {
		t.Errorf("expected no smotra_db_response_time_ms when db is unhealthy, got:\n%s", out)
	}
}

func TestDBMetrics_GetMetrics_NilSQLDB(t *testing.T) {
	// When the underlying *sql.DB is nil, Health() may still succeed (e.g. in stubs/mocks).
	// Pool stats are emitted whenever Health() returns nil, regardless of the DB() accessor.
	m := NewDBMetrics(&staticDB{db: nil})
	out := m.GetMetrics()

	if !strings.Contains(out, "smotra_db_healthy") {
		t.Errorf("expected smotra_db_healthy even with nil sqlDB, got:\n%s", out)
	}
	// Pool stats are always present when Health() succeeds (values may be zero).
	if !strings.Contains(out, "smotra_db_connections_open") {
		t.Errorf("expected pool stats when Health() succeeds, got:\n%s", out)
	}
}

func newTestSQLDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// staticDB is a minimal Database stub that returns a fixed *sql.DB.
type staticDB struct {
	db   *sql.DB
	fail bool
}

func (s *staticDB) DB() *sql.DB                  { return s.db }
func (s *staticDB) Open(_ context.Context) error { return nil }
func (s *staticDB) Close() error                 { return nil }
func (s *staticDB) Ping(_ context.Context) error { return nil }
func (s *staticDB) Health(_ context.Context) (HealthInfo, error) {
	if s.fail {
		return HealthInfo{}, fmt.Errorf("mock health failure")
	}
	return HealthInfo{ResponseTime: 5 * time.Millisecond}, nil
}
func (s *staticDB) BeginTx(_ context.Context) (*sql.Tx, error) { return nil, nil }
