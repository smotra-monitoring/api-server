package agent_configuration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/smotra-monitoring/server/internal/database"
	"github.com/smotra-monitoring/server/internal/logger"
	"github.com/smotra-monitoring/server/internal/middleware"
	"github.com/smotra-monitoring/server/internal/testutil"
)

func newPatchTestDB(t *testing.T) (database.Database, string) {
	t.Helper()
	ctx := context.Background()
	db := testutil.SetupTestSQLiteDB(t)
	testutil.ApplyMigrations(t, ctx, db.DB(), "../../../data/db/dev/migrations")

	tenantID := uuid.NewString()
	workspaceID := uuid.NewString()
	mustExec(t, db, "INSERT INTO tenants (id, name) VALUES (?, ?)", tenantID, "Tenant")
	mustExec(t, db, "INSERT INTO workspaces (id, tenant_id, name) VALUES (?, ?, ?)", workspaceID, tenantID, "Workspace")
	return db, workspaceID
}

func mustExec(t *testing.T, db database.Database, query string, args ...any) {
	t.Helper()
	if _, err := db.DB().ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func insertAgent(t *testing.T, db database.Database, workspaceID, baseConfig string) string {
	t.Helper()
	id := uuid.NewString()
	mustExec(t, db, "INSERT INTO agents (id, workspace_id, name, api_key_hash, base_config) VALUES (?, ?, ?, ?, ?)",
		id, workspaceID, "agent", "hash", baseConfig)
	return id
}

func queryDesiredVersion(t *testing.T, db database.Database, agentID string) int64 {
	t.Helper()
	var v int64
	if err := db.DB().QueryRowContext(context.Background(),
		"SELECT config_version_desired FROM agents WHERE id = ?", agentID).Scan(&v); err != nil {
		t.Fatalf("read config_version_desired: %v", err)
	}
	return v
}

func assertTopLevelKeys(t *testing.T, body []byte) {
	t.Helper()
	var top map[string]json.RawMessage
	if err := json.Unmarshal(body, &top); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	got := make([]string, 0, len(top))
	for k := range top {
		got = append(got, k)
	}
	sort.Strings(got)
	want := []string{"base_config", "config_version_desired", "endpoints", "id", "name", "tags", "workspace_id"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("top-level keys = %v, want %v", got, want)
	}
}

func assertJSONEqual(t *testing.T, stored string, got map[string]interface{}) {
	t.Helper()
	var want map[string]interface{}
	if err := json.Unmarshal([]byte(stored), &want); err != nil {
		t.Fatalf("decode stored JSON: %v", err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Errorf("base_config = %#v, want %#v", got, want)
	}
}

func getConfigurationBody(t *testing.T, handler *Handler, agentID string) []byte {
	t.Helper()
	router := setupTestRouter(handler)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/agent/%s/configuration", agentID), nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	return rr.Body.Bytes()
}

func TestGetAgentConfiguration_ReturnsStoredPatchUnchanged(t *testing.T) {
	db, workspaceID := newPatchTestDB(t)
	handler := NewHandler(logger.Default(), db, "1.0.0")

	stored := `{"monitoring":{"interval_secs":30,"timeout_secs":null},"storage":{"max_cached_results":null},"server":{"nested":{"deep":[1,2,{"x":null}]}},"empty":{},"list":["a","b"]}`
	agentID := insertAgent(t, db, workspaceID, stored)

	body := getConfigurationBody(t, handler, agentID)
	assertTopLevelKeys(t, body)

	var resp struct {
		BaseConfig json.RawMessage `json:"base_config"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	var gotKeys, wantKeys map[string]json.RawMessage
	if err := json.Unmarshal(resp.BaseConfig, &gotKeys); err != nil {
		t.Fatalf("decode got keys: %v", err)
	}
	if err := json.Unmarshal([]byte(stored), &wantKeys); err != nil {
		t.Fatalf("decode stored keys: %v", err)
	}
	if len(gotKeys) != len(wantKeys) {
		t.Errorf("base_config keys = %d, want %d (no keys may be added)", len(gotKeys), len(wantKeys))
	}
	for _, absent := range []string{"tags", "endpoints", "version", "agent_id", "agent_name", "self_upgrade"} {
		if _, ok := gotKeys[absent]; ok {
			t.Errorf("base_config unexpectedly contains %q", absent)
		}
	}

	var gotAny, wantAny map[string]interface{}
	if err := json.Unmarshal(resp.BaseConfig, &gotAny); err != nil {
		t.Fatalf("decode got: %v", err)
	}
	if err := json.Unmarshal([]byte(stored), &wantAny); err != nil {
		t.Fatalf("decode want: %v", err)
	}
	if !reflect.DeepEqual(gotAny, wantAny) {
		t.Errorf("base_config = %s, want %s", resp.BaseConfig, stored)
	}

	// Explicit nulls survive instead of being dropped or zero-filled.
	monitoring := gotAny["monitoring"].(map[string]interface{})
	if v, ok := monitoring["timeout_secs"]; !ok || v != nil {
		t.Errorf("monitoring.timeout_secs = %v (present=%v), want explicit null", v, ok)
	}
}

func TestGetAgentConfiguration_EmptyBaseConfigReturnsEmptyObject(t *testing.T) {
	db, workspaceID := newPatchTestDB(t)
	handler := NewHandler(logger.Default(), db, "1.0.0")

	for name, stored := range map[string]string{"default": "{}", "json_null": "null"} {
		t.Run(name, func(t *testing.T) {
			agentID := insertAgent(t, db, workspaceID, stored)
			body := getConfigurationBody(t, handler, agentID)
			assertTopLevelKeys(t, body)

			var resp struct {
				BaseConfig json.RawMessage `json:"base_config"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if string(resp.BaseConfig) != "{}" {
				t.Errorf("base_config = %s, want {}", resp.BaseConfig)
			}
		})
	}
}

func TestGetAgentConfiguration_ConfigVersionBumpsOnValueChangesOnly(t *testing.T) {
	db, workspaceID := newPatchTestDB(t)
	handler := NewHandler(logger.Default(), db, "1.0.0")
	agentID := insertAgent(t, db, workspaceID, `{"a":1}`)

	dbVersion := func() int64 { return queryDesiredVersion(t, db, agentID) }
	apiServed := func() int64 {
		var resp struct {
			V int64 `json:"config_version_desired"`
		}
		if err := json.Unmarshal(getConfigurationBody(t, handler, agentID), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return resp.V
	}

	steps := []struct {
		name  string
		query string
		args  []any
		bump  int64
	}{
		{"base_config same value", "UPDATE agents SET base_config = ? WHERE id = ?", []any{`{"a":1}`, agentID}, 0},
		{"name same value", "UPDATE agents SET name = ? WHERE id = ?", []any{"agent", agentID}, 0},
		{"base_config changed", "UPDATE agents SET base_config = ? WHERE id = ?", []any{`{"a":2}`, agentID}, 1},
		{"name changed", "UPDATE agents SET name = ? WHERE id = ?", []any{"renamed", agentID}, 1},
	}
	for _, s := range steps {
		before := dbVersion()
		mustExec(t, db, s.query, s.args...)
		if got := dbVersion() - before; got != s.bump {
			t.Errorf("%s: config_version_desired changed by %d, want %d", s.name, got, s.bump)
		}
		if got := apiServed(); got != dbVersion() {
			t.Errorf("%s: served config_version_desired = %d, want %d", s.name, got, dbVersion())
		}
	}
}

func TestConfigVersionTriggers_IgnoreDummyUpdates(t *testing.T) {
	db, workspaceID := newPatchTestDB(t)
	agentID := insertAgent(t, db, workspaceID, `{}`)

	endpointID := uuid.NewString()
	mustExec(t, db, "INSERT INTO endpoints (id, workspace_id, address, enabled) VALUES (?, ?, ?, ?)", endpointID, workspaceID, "10.0.0.1", 1)

	tagID := uuid.NewString()
	mustExec(t, db, "INSERT INTO tags (id, workspace_id, name, scope) VALUES (?, ?, ?, ?)", tagID, workspaceID, "prod", "agent")
	mustExec(t, db, "INSERT INTO agent_tags (agent_id, tag_id) VALUES (?, ?)", agentID, tagID)

	topologyID := uuid.NewString()
	mustExec(t, db, "INSERT INTO topologies (id, workspace_id, name, type, enabled) VALUES (?, ?, ?, ?, ?)", topologyID, workspaceID, "topo", "full-mesh", 1)
	mustExec(t, db, "INSERT INTO topology_members (topology_id, tag_id, role) VALUES (?, ?, ?)", topologyID, tagID, "monitor")

	cases := []struct {
		name  string
		query string
		args  []any
		bump  int64
	}{
		{"endpoint address same", "UPDATE endpoints SET address = ? WHERE id = ?", []any{"10.0.0.1", endpointID}, 0},
		{"endpoint enabled same", "UPDATE endpoints SET enabled = ? WHERE id = ?", []any{1, endpointID}, 0},
		{"endpoint address changed", "UPDATE endpoints SET address = ? WHERE id = ?", []any{"10.0.0.2", endpointID}, 1},
		{"endpoint enabled changed", "UPDATE endpoints SET enabled = ? WHERE id = ?", []any{0, endpointID}, 1},
		{"tag name same", "UPDATE tags SET name = ? WHERE id = ?", []any{"prod", tagID}, 0},
		{"tag name changed", "UPDATE tags SET name = ? WHERE id = ?", []any{"production", tagID}, 1},
		{"topology unchanged", "UPDATE topologies SET name = ?, type = ?, enabled = ? WHERE id = ?", []any{"topo", "full-mesh", 1, topologyID}, 0},
		{"topology enabled changed", "UPDATE topologies SET enabled = ? WHERE id = ?", []any{0, topologyID}, 1},
		{"topology type changed", "UPDATE topologies SET type = ? WHERE id = ?", []any{"one-way", topologyID}, 1},
		{"member role same", "UPDATE topology_members SET role = ? WHERE topology_id = ? AND tag_id = ?", []any{"monitor", topologyID, tagID}, 0},
		{"member role changed", "UPDATE topology_members SET role = ? WHERE topology_id = ? AND tag_id = ?", []any{"target", topologyID, tagID}, 1},
	}
	for _, c := range cases {
		before := queryDesiredVersion(t, db, agentID)
		mustExec(t, db, c.query, c.args...)
		if got := queryDesiredVersion(t, db, agentID) - before; got != c.bump {
			t.Errorf("%s: config_version_desired changed by %d, want %d", c.name, got, c.bump)
		}
	}
}

func insertUser(t *testing.T, db database.Database, workspaceID string) string {
	t.Helper()
	id := uuid.NewString()
	mustExec(t, db, "INSERT INTO users (id, workspace_id, oauth_provider, oauth_subject, display_name) VALUES (?, ?, ?, ?, ?)",
		id, workspaceID, "test_provider", "sub-"+id, "User")
	return id
}

func oauth2Info(userID string) *middleware.AuthInfo {
	return &middleware.AuthInfo{AuthType: "oauth2", Authenticated: true, UserID: userID, SessionID: "session-" + userID}
}

func serveConfiguration(handler *Handler, info *middleware.AuthInfo, agentID string) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	setupTestRouterWithAuth(handler, info).ServeHTTP(rr,
		httptest.NewRequest(http.MethodGet, fmt.Sprintf("/agent/%s/configuration", agentID), nil))
	return rr
}

func TestGetAgentConfiguration_LastSeenByAuthType(t *testing.T) {
	for _, tc := range []struct {
		authType string
		updated  bool
	}{
		{"agent_api_key", true},
		{"oauth2", false},
	} {
		t.Run(tc.authType, func(t *testing.T) {
			db, workspaceID := newPatchTestDB(t)
			handler := NewHandler(logger.Default(), db, "1.0.0")
			agentID := insertAgent(t, db, workspaceID, `{}`)

			info := &middleware.AuthInfo{AuthType: "agent_api_key", Authenticated: true, AgentID: agentID}
			if tc.authType == "oauth2" {
				info = oauth2Info(insertUser(t, db, workspaceID))
			}

			if rr := serveConfiguration(handler, info, agentID); rr.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
			}

			var lastSeen sql.NullTime
			if err := db.DB().QueryRowContext(context.Background(),
				"SELECT last_seen_at FROM agents WHERE id = ?", agentID).Scan(&lastSeen); err != nil {
				t.Fatalf("read last_seen_at: %v", err)
			}
			if lastSeen.Valid != tc.updated {
				t.Errorf("last_seen_at set = %v, want %v", lastSeen.Valid, tc.updated)
			}
		})
	}
}

func TestGetAgentConfiguration_OAuth2WorkspaceScope(t *testing.T) {
	db, workspaceID := newPatchTestDB(t)
	handler := NewHandler(logger.Default(), db, "1.0.0")
	agentID := insertAgent(t, db, workspaceID, `{}`)

	var tenantID string
	if err := db.DB().QueryRowContext(context.Background(), "SELECT tenant_id FROM workspaces WHERE id = ?", workspaceID).Scan(&tenantID); err != nil {
		t.Fatalf("read tenant: %v", err)
	}
	otherWorkspaceID := uuid.NewString()
	mustExec(t, db, "INSERT INTO workspaces (id, tenant_id, name) VALUES (?, ?, ?)", otherWorkspaceID, tenantID, "Other")

	sameUser := insertUser(t, db, workspaceID)
	foreignUser := insertUser(t, db, otherWorkspaceID)

	cases := []struct {
		name    string
		userID  string
		agentID string
		want    int
	}{
		{"same workspace", sameUser, agentID, http.StatusOK},
		{"foreign workspace", foreignUser, agentID, http.StatusUnauthorized},
		{"unknown user", uuid.NewString(), agentID, http.StatusUnauthorized},
		{"unknown agent", sameUser, uuid.NewString(), http.StatusNotFound},
	}
	for _, tc := range cases {
		if rr := serveConfiguration(handler, oauth2Info(tc.userID), tc.agentID); rr.Code != tc.want {
			t.Errorf("%s: status = %d, want %d: %s", tc.name, rr.Code, tc.want, rr.Body.String())
		}
	}

	if got := handler.getConfigurationWorkspaceDenied.Load(); got != 1 {
		t.Errorf("workspace denied counter = %d, want 1 (foreign workspace only)", got)
	}
	if got := handler.getConfigurationOAuth2.Load(); got != 1 {
		t.Errorf("oauth2 counter = %d, want 1 (successful same-workspace read only)", got)
	}
	if m := handler.GetMetrics(); !strings.Contains(m, "smotra_agent_configuration_get_configuration_workspace_denied_total 1") {
		t.Errorf("metrics missing workspace denied counter:\n%s", m)
	}
}
