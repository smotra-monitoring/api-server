package database

import (
	"context"
	"fmt"
	"time"
)

// DBMetrics exposes database health and connection pool statistics as a MetricsProvider.
type DBMetrics struct {
	db Database
}

// NewDBMetrics creates a new DBMetrics wrapping the given Database.
func NewDBMetrics(db Database) *DBMetrics {
	return &DBMetrics{db: db}
}

// GetMetrics returns Prometheus-formatted database health and connection pool metrics.
func (m *DBMetrics) GetMetrics() string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var out string

	dbHealth, err := m.db.Health(ctx)
	dbHealthy := 0.0
	if err == nil {
		dbHealthy = 1.0
	}

	out += "# HELP smotra_db_healthy Database health status (1 = healthy, 0 = unhealthy)\n"
	out += "# TYPE smotra_db_healthy gauge\n"
	out += fmt.Sprintf("smotra_db_healthy %.0f\n", dbHealthy)
	out += "\n"

	if err != nil {
		return out
	}

	out += "# HELP smotra_db_response_time_ms Database response time in milliseconds\n"
	out += "# TYPE smotra_db_response_time_ms gauge\n"
	out += fmt.Sprintf("smotra_db_response_time_ms %.2f\n", float64(dbHealth.ResponseTime.Milliseconds()))
	out += "\n"

	out += "# HELP smotra_db_connections_open Current number of open connections to the database\n"
	out += "# TYPE smotra_db_connections_open gauge\n"
	out += fmt.Sprintf("smotra_db_connections_open %d\n", dbHealth.DBOpenConns)
	out += "\n"

	out += "# HELP smotra_db_connections_in_use Number of connections currently in use\n"
	out += "# TYPE smotra_db_connections_in_use gauge\n"
	out += fmt.Sprintf("smotra_db_connections_in_use %d\n", dbHealth.DBInUseConns)
	out += "\n"

	out += "# HELP smotra_db_connections_idle Number of idle connections\n"
	out += "# TYPE smotra_db_connections_idle gauge\n"
	out += fmt.Sprintf("smotra_db_connections_idle %d\n", dbHealth.DBIdleConns)
	out += "\n"

	out += "# HELP smotra_db_wait_connections_count_total Total number of times a goroutine waited for a connection\n"
	out += "# TYPE smotra_db_wait_connections_count_total counter\n"
	out += fmt.Sprintf("smotra_db_wait_connections_count_total %d\n", dbHealth.DBWaitConnsCount)
	out += "\n"

	out += "# HELP smotra_db_wait_connections_duration_ms Total time blocked waiting for a new connection (ms)\n"
	out += "# TYPE smotra_db_wait_connections_duration_ms counter\n"
	out += fmt.Sprintf("smotra_db_wait_connections_duration_ms %d\n", dbHealth.DBWaitConnsDuration.Milliseconds())
	out += "\n"

	return out
}
