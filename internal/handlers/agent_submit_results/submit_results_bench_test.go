package agent_submit_results

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	api "github.com/smotra-monitoring/server/internal/api/v1"
	"github.com/smotra-monitoring/server/internal/database"
	"github.com/smotra-monitoring/server/internal/logger"
)

// ─── infrastructure helpers ───────────────────────────────────────────────────

// benchDB creates a temporary SQLite database with all migrations applied and a
// pre-seeded tenant, section, agent, and endpoint. The database is closed via
// b.Cleanup automatically.
func benchDB(b *testing.B) (database.Database, uuid.UUID, string) {
	b.Helper()

	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")
	cfg := database.SQLiteConfig{
		Config: database.Config{
			Type:         "sqlite",
			MaxOpenConns: 1,
			MaxIdleConns: 1,
		},
		FilePath: dbPath,
	}

	db := database.NewSQLiteDB(cfg)
	ctx := context.Background()
	if err := db.Open(ctx); err != nil {
		b.Fatalf("open bench db: %v", err)
	}
	b.Cleanup(func() { db.Close() })

	benchApplyMigrations(b, ctx, db.DB(), "../../../data/db/dev/migrations")

	tenantID := uuid.Must(uuid.NewV7()).String()
	benchExec(b, db.DB(), `INSERT INTO tenants (id, name) VALUES (?, ?)`, tenantID, "BenchTenant")

	sectionID := uuid.Must(uuid.NewV7()).String()
	benchExec(b, db.DB(), `INSERT INTO sections (id, tenant_id, name) VALUES (?, ?, ?)`, sectionID, tenantID, "BenchSection")

	agentID := uuid.Must(uuid.NewV7())
	benchExec(b, db.DB(),
		`INSERT INTO agents (id, section_id, name, api_key_hash, base_config) VALUES (?, ?, ?, ?, ?)`,
		agentID.String(), sectionID, "bench-agent", "fakehash", "{}")

	endpointID := uuid.Must(uuid.NewV7()).String()
	benchExec(b, db.DB(),
		`INSERT INTO endpoints (id, agent_id, hostname, resolved_ip, enabled) VALUES (?, ?, ?, ?, 1)`,
		endpointID, agentID.String(), "bench.example.com", "10.0.0.1")

	return db, agentID, endpointID
}

func benchApplyMigrations(b *testing.B, ctx context.Context, db *sql.DB, migrationsDir string) {
	b.Helper()

	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		b.Fatalf("read migrations dir %q: %v", migrationsDir, err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, fname := range files {
		content, err := os.ReadFile(filepath.Join(migrationsDir, fname))
		if err != nil {
			b.Fatalf("read migration %s: %v", fname, err)
		}
		if _, err := db.ExecContext(ctx, string(content)); err != nil {
			b.Fatalf("apply migration %s: %v", fname, err)
		}
	}
}

func benchExec(b *testing.B, db *sql.DB, query string, args ...any) {
	b.Helper()
	if _, err := db.ExecContext(context.Background(), query, args...); err != nil {
		b.Fatalf("exec %q: %v", query, err)
	}
}

// ─── HTTP helper ──────────────────────────────────────────────────────────────

// benchPost serialises results into a BatchMonitoringResults JSON body and
// issues a POST to the handler via the provided chi router.
func benchPost(b *testing.B, router *chi.Mux, agentID uuid.UUID, results []api.MonitoringResult) *httptest.ResponseRecorder {
	b.Helper()
	body, _ := json.Marshal(api.BatchMonitoringResults{Results: results})
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/agent/%s/results", agentID),
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// ─── check-type constructors ──────────────────────────────────────────────────

// benchMarshal marshals v and wraps it into a CheckType union.
func benchMarshal(b *testing.B, v any) api.CheckType {
	b.Helper()
	raw, _ := json.Marshal(v)
	var ct api.CheckType
	if err := json.Unmarshal(raw, &ct); err != nil {
		b.Fatalf("build CheckType: %v", err)
	}
	return ct
}

func benchPingCT(b *testing.B) api.CheckType {
	b.Helper()
	return benchMarshal(b, api.PingCheck{
		Type: "ping",
		Result: api.PingResult{
			ResolvedIp:       "8.8.8.8",
			Successes:        5,
			Failures:         0,
			SuccessLatencies: []float64{10.0, 12.0, 15.0},
		},
	})
}

func benchHttpGetCT(b *testing.B) api.CheckType {
	b.Helper()
	respTime := 145.3
	respSize := int64(2048)
	return benchMarshal(b, api.HttpGetCheck{
		Type: "httpget",
		Result: api.HttpGetResult{
			StatusCode:        200,
			Success:           true,
			ResponseTimeMs:    &respTime,
			ResponseSizeBytes: &respSize,
		},
	})
}

func benchTcpConnectCT(b *testing.B) api.CheckType {
	b.Helper()
	connectTime := 8.2
	return benchMarshal(b, api.TcpConnectCheck{
		Type: "tcpconnect",
		Result: api.TcpConnectResult{
			ResolvedIp:    "8.8.8.8",
			Connected:     true,
			ConnectTimeMs: &connectTime,
		},
	})
}

func benchUdpConnectCT(b *testing.B) api.CheckType {
	b.Helper()
	respTime := 6.1
	return benchMarshal(b, api.UdpConnectCheck{
		Type: "udpconnect",
		Result: api.UdpConnectResult{
			ResolvedIp:      "8.8.8.8",
			ProbeSuccessful: true,
			ResponseTimeMs:  &respTime,
		},
	})
}

func benchPluginCT(b *testing.B) api.CheckType {
	b.Helper()
	respTime := 55.0
	return benchMarshal(b, api.PluginCheck{
		Type: "plugin",
		Result: api.PluginResult{
			PluginName:     "test-plugin",
			PluginVersion:  "1.0.0",
			Success:        true,
			ResponseTimeMs: &respTime,
		},
	})
}

// benchTracerouteCTN builds a traceroute CheckType with numHops hops.
func benchTracerouteCTN(b *testing.B, numHops int) api.CheckType {
	b.Helper()
	hops := make([]api.TracerouteHop, numHops)
	for i := range hops {
		lat := []float64{float64(i+1) * 2.5}
		ip := fmt.Sprintf("10.0.%d.1", i+1)
		host := fmt.Sprintf("hop%d.example.com", i+1)
		hops[i] = api.TracerouteHop{
			Hop:              int32(i + 1),
			ResolvedIp:       &ip,
			Hostname:         &host,
			SuccessLatencies: &lat,
		}
	}
	return benchMarshal(b, api.TracerouteCheck{
		Type: "traceroute",
		Result: api.TracerouteResult{
			TargetReached: true,
			Hops:          hops,
		},
	})
}

// ─── Benchmarks ───────────────────────────────────────────────────────────────

// BenchmarkSubmitResults_SingleTypes measures per-check-type insertion cost for
// a single-result batch. UUID generation is excluded from the timed window via
// b.StopTimer/b.StartTimer so results reflect only handler + database work.
func BenchmarkSubmitResults_SingleTypes(b *testing.B) {
	db, agentID, endpointIDStr := benchDB(b)
	h := NewHandler(logger.Default(), db)
	router := setupTestRouter(h)
	endpointID := uuid.MustParse(endpointIDStr)

	cases := []struct {
		name string
		ct   api.CheckType
	}{
		{"Ping", benchPingCT(b)},
		{"HttpGet", benchHttpGetCT(b)},
		{"TcpConnect", benchTcpConnectCT(b)},
		{"UdpConnect", benchUdpConnectCT(b)},
		{"Plugin", benchPluginCT(b)},
		{"Traceroute_5hops", benchTracerouteCTN(b, 5)},
	}

	for _, tc := range cases {
		tc := tc
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				result := makeResult(agentID, tc.ct, endpointID)
				b.StartTimer()

				w := benchPost(b, router, agentID, []api.MonitoringResult{result})
				if w.Code != http.StatusAccepted {
					b.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
				}
			}
		})
	}
}

// BenchmarkSubmitResults_BatchSizes measures how total handler time (and ns/op)
// scales as batch size grows. UUID generation is excluded from the timed window.
// A sub-linear ns/op growth indicates that fixed per-request overhead (JSON
// parsing, HTTP routing, last_seen_at update) is being amortised effectively.
func BenchmarkSubmitResults_BatchSizes(b *testing.B) {
	db, agentID, endpointIDStr := benchDB(b)
	h := NewHandler(logger.Default(), db)
	router := setupTestRouter(h)
	endpointID := uuid.MustParse(endpointIDStr)
	ct := benchPingCT(b)

	for _, size := range []int{1, 10, 50, 100, 500} {
		size := size
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				results := make([]api.MonitoringResult, size)
				for j := range results {
					results[j] = makeResult(agentID, ct, endpointID)
				}
				b.StartTimer()

				w := benchPost(b, router, agentID, results)
				if w.Code != http.StatusAccepted {
					b.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
				}
			}
		})
	}
}

// BenchmarkSubmitResults_Deduplication measures the cost of the SELECT-only
// fast path when every result in a batch is already known to the server.
// A baseline batch of 100 unique ping results is pre-inserted before the timed
// loop so that all subsequent iterations exercise CheckResultExists exclusively.
// Compare ns/op against BenchmarkSubmitResults_BatchSizes/size=100 to quantify
// the INSERT overhead vs. the SELECT-only path.
func BenchmarkSubmitResults_Deduplication(b *testing.B) {
	db, agentID, endpointIDStr := benchDB(b)
	h := NewHandler(logger.Default(), db)
	router := setupTestRouter(h)
	endpointID := uuid.MustParse(endpointIDStr)
	ct := benchPingCT(b)

	const batchSize = 100
	fixed := make([]api.MonitoringResult, batchSize)
	for i := range fixed {
		fixed[i] = makeResult(agentID, ct, endpointID)
	}

	// Seed the database so all subsequent submissions are duplicates.
	w := benchPost(b, router, agentID, fixed)
	if w.Code != http.StatusAccepted {
		b.Fatalf("pre-insert failed: %d: %s", w.Code, w.Body.String())
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		w := benchPost(b, router, agentID, fixed)
		if w.Code != http.StatusAccepted {
			b.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
		}
	}
}

// BenchmarkSubmitResults_Traceroute_Hops measures how traceroute insertion cost
// scales with hop count. Each hop requires an extra INSERT into
// check_results_traceroute_hops, so ns/op should grow linearly with numHops.
func BenchmarkSubmitResults_Traceroute_Hops(b *testing.B) {
	db, agentID, endpointIDStr := benchDB(b)
	h := NewHandler(logger.Default(), db)
	router := setupTestRouter(h)
	endpointID := uuid.MustParse(endpointIDStr)

	for _, hops := range []int{1, 5, 15, 30} {
		hops := hops
		ct := benchTracerouteCTN(b, hops)
		b.Run(fmt.Sprintf("hops=%d", hops), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				result := makeResult(agentID, ct, endpointID)
				b.StartTimer()

				w := benchPost(b, router, agentID, []api.MonitoringResult{result})
				if w.Code != http.StatusAccepted {
					b.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
				}
			}
		})
	}
}

// BenchmarkSubmitResults_Parallel exercises the handler under concurrent load.
// Each goroutine submits single-result batches with unique UUIDs. With SQLite
// (MaxOpenConns=1) writes are serialised at the connection pool, so this
// benchmark exposes lock-contention overhead and is most meaningful when the
// production PostgreSQL backend is substituted.
func BenchmarkSubmitResults_Parallel(b *testing.B) {
	db, agentID, endpointIDStr := benchDB(b)
	h := NewHandler(logger.Default(), db)
	router := setupTestRouter(h)
	endpointID := uuid.MustParse(endpointIDStr)
	ct := benchPingCT(b)

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			result := makeResult(agentID, ct, endpointID)
			w := benchPost(b, router, agentID, []api.MonitoringResult{result})
			if w.Code != http.StatusAccepted {
				b.Errorf("expected 202, got %d", w.Code)
			}
		}
	})
}
