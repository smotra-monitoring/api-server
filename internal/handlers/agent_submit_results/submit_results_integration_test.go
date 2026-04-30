package agent_submit_results

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	api "github.com/smotra-monitoring/server/internal/api/v1"
	"github.com/smotra-monitoring/server/internal/database"
	"github.com/smotra-monitoring/server/internal/logger"
	"github.com/smotra-monitoring/server/internal/testutil"
)

// testServerImpl satisfies the full api.StrictServerInterface by delegating
// SubmitAgentResults to the handler under test and stubbing everything else.
type testServerImpl struct {
	*Handler
}

func (s *testServerImpl) SubmitAgentResults(ctx context.Context, req api.SubmitAgentResultsRequestObject) (api.SubmitAgentResultsResponseObject, error) {
	return s.Handle(ctx, req)
}
func (s *testServerImpl) GetAgentConfiguration(ctx context.Context, req api.GetAgentConfigurationRequestObject) (api.GetAgentConfigurationResponseObject, error) {
	return nil, nil
}
func (s *testServerImpl) RegisterAgentSelf(ctx context.Context, req api.RegisterAgentSelfRequestObject) (api.RegisterAgentSelfResponseObject, error) {
	return nil, nil
}
func (s *testServerImpl) GetAgentClaimStatus(ctx context.Context, req api.GetAgentClaimStatusRequestObject) (api.GetAgentClaimStatusResponseObject, error) {
	return nil, nil
}
func (s *testServerImpl) PostClaimAgent(ctx context.Context, req api.PostClaimAgentRequestObject) (api.PostClaimAgentResponseObject, error) {
	return nil, nil
}

func (t *testServerImpl) Oauth2Authorize(ctx context.Context, request api.Oauth2AuthorizeRequestObject) (api.Oauth2AuthorizeResponseObject, error) {
	return nil, nil
}

func (t *testServerImpl) Oauth2Callback(ctx context.Context, request api.Oauth2CallbackRequestObject) (api.Oauth2CallbackResponseObject, error) {
	return nil, nil
}

func (t *testServerImpl) Oauth2Revoke(ctx context.Context, request api.Oauth2RevokeRequestObject) (api.Oauth2RevokeResponseObject, error) {
	return nil, nil
}

func (t *testServerImpl) Oauth2Token(ctx context.Context, request api.Oauth2TokenRequestObject) (api.Oauth2TokenResponseObject, error) {
	return nil, nil
}

func (t *testServerImpl) GetUserInfo(ctx context.Context, request api.GetUserInfoRequestObject) (api.GetUserInfoResponseObject, error) {
	return nil, nil
}

func (s *testServerImpl) Logout(ctx context.Context, req api.LogoutRequestObject) (api.LogoutResponseObject, error) {
	return nil, nil
}

func setupTestRouter(h *Handler) *chi.Mux {
	impl := &testServerImpl{Handler: h}
	r := chi.NewRouter()
	api.HandlerFromMux(api.NewStrictHandler(impl, nil), r)
	return r
}

// setupRealDB creates a SQLite DB with all migrations applied plus a test tenant/section/agent.
// Returns (db, agentID, sectionID).
func setupRealDB(t *testing.T) (database.Database, uuid.UUID, string) {
	t.Helper()
	db := testutil.SetupTestSQLiteDB(t)
	ctx := context.Background()
	testutil.ApplyMigrations(t, ctx, db.DB(), "../../../data/db/dev/migrations")

	tenantID := uuid.Must(uuid.NewV7()).String()
	if _, err := db.DB().ExecContext(ctx, `INSERT INTO tenants (id, name) VALUES (?, ?)`, tenantID, "Test Tenant"); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	sectionID := uuid.Must(uuid.NewV7()).String()
	if _, err := db.DB().ExecContext(ctx, `INSERT INTO sections (id, tenant_id, name) VALUES (?, ?, ?)`, sectionID, tenantID, "Default"); err != nil {
		t.Fatalf("insert section: %v", err)
	}
	agentID := uuid.Must(uuid.NewV7())
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO agents (id, section_id, name, api_key_hash, base_config) VALUES (?, ?, ?, ?, ?)`,
		agentID.String(), sectionID, "test-agent", "fakehash", "{}"); err != nil {
		t.Fatalf("insert agent: %v", err)
	}

	return db, agentID, sectionID
}

// setupTopologyPermission creates a topology + tag membership so that agentID is
// permitted (via GetEndpointsForAgent / GetEndpointByIDAndAgentID) to monitor endpointID.
func setupTopologyPermission(t *testing.T, db database.Database, sectionID, agentID, endpointID string) {
	t.Helper()
	ctx := context.Background()

	tagID := uuid.Must(uuid.NewV7()).String()
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO tags (id, section_id, name, scope) VALUES (?, ?, ?, 'global')`,
		tagID, sectionID, "topo-"+tagID[:8]); err != nil {
		t.Fatalf("insert topology tag: %v", err)
	}
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO agent_tags (agent_id, tag_id) VALUES (?, ?)`, agentID, tagID); err != nil {
		t.Fatalf("assign tag to agent: %v", err)
	}
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO endpoint_tags (endpoint_id, tag_id) VALUES (?, ?)`, endpointID, tagID); err != nil {
		t.Fatalf("assign tag to endpoint: %v", err)
	}

	topologyID := uuid.Must(uuid.NewV7()).String()
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO topologies (id, section_id, name, type, enabled) VALUES (?, ?, ?, 'full-mesh', 1)`,
		topologyID, sectionID, "topo-"+topologyID[:8]); err != nil {
		t.Fatalf("insert topology: %v", err)
	}
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO topology_members (topology_id, tag_id, role) VALUES (?, ?, 'monitor')`,
		topologyID, tagID); err != nil {
		t.Fatalf("insert agent topology member: %v", err)
	}
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO topology_members (topology_id, tag_id, role) VALUES (?, ?, 'target')`,
		topologyID, tagID); err != nil {
		t.Fatalf("insert endpoint topology member: %v", err)
	}
}

func pingCheckType(t *testing.T, resolved string, successes, failures int32) api.CheckType {
	t.Helper()
	raw, _ := json.Marshal(api.PingCheck{
		Type: "ping",
		Result: api.PingResult{
			ResolvedIp:       resolved,
			Successes:        successes,
			Failures:         failures,
			SuccessLatencies: []float64{10.0, 15.0},
		},
	})
	var ct api.CheckType
	if err := json.Unmarshal(raw, &ct); err != nil {
		t.Fatalf("unmarshal CheckType: %v", err)
	}
	return ct
}

func tracerouteCheckType(t *testing.T, hops []api.TracerouteHop) api.CheckType {
	t.Helper()
	raw, _ := json.Marshal(api.TracerouteCheck{
		Type: "traceroute",
		Result: api.TracerouteResult{
			TargetReached: true,
			Hops:          hops,
		},
	})
	var ct api.CheckType
	if err := json.Unmarshal(raw, &ct); err != nil {
		t.Fatalf("unmarshal CheckType: %v", err)
	}
	return ct
}

func postBatch(t *testing.T, router *chi.Mux, agentID uuid.UUID, results []api.MonitoringResult) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(api.BatchMonitoringResults{Results: results})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/agent/%s/results", agentID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestIntegration_PingBatch_Accepted(t *testing.T) {
	db, agentID, sectionID := setupRealDB(t)
	h := NewHandler(logger.Default(), db)
	router := setupTestRouter(h)
	ctx := context.Background()

	endpointID := uuid.Must(uuid.NewV7()).String()
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO endpoints (id, section_id, address, enabled) VALUES (?, ?, ?, 1)`,
		endpointID, sectionID, "8.8.8.8"); err != nil {
		t.Fatalf("insert endpoint: %v", err)
	}
	setupTopologyPermission(t, db, sectionID, agentID.String(), endpointID)

	result := api.MonitoringResult{
		Id:         uuid.Must(uuid.NewV7()),
		AgentId:    agentID,
		CheckType:  pingCheckType(t, "8.8.8.8", 5, 0),
		EndpointId: uuid.MustParse(endpointID),
		Timestamp:  time.Now().UTC(),
	}

	w := postBatch(t, router, agentID, []api.MonitoringResult{result})
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}

	var ack api.ResultsBatchAcknowledgment
	if err := json.Unmarshal(w.Body.Bytes(), &ack); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ack.Accepted != 1 {
		t.Errorf("expected accepted=1, got %d", ack.Accepted)
	}
	if ack.DuplicatesSkipped != nil && *ack.DuplicatesSkipped != 0 {
		t.Errorf("expected duplicates=0, got %d", *ack.DuplicatesSkipped)
	}
	if ack.SubmissionId == (uuid.UUID{}) {
		t.Error("expected non-zero submission_id")
	}

	var cnt int
	db.DB().QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM check_results WHERE agent_id = ? AND check_type = 'ping'`, agentID.String()).Scan(&cnt)
	if cnt != 1 {
		t.Errorf("expected 1 row in check_results, got %d", cnt)
	}
	var pingCnt int
	db.DB().QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM check_results_ping WHERE check_id = ?`, result.Id.String()).Scan(&pingCnt)
	if pingCnt != 1 {
		t.Errorf("expected 1 row in check_results_ping, got %d", pingCnt)
	}
}

func TestIntegration_Deduplication(t *testing.T) {
	db, agentID, sectionID := setupRealDB(t)
	h := NewHandler(logger.Default(), db)
	router := setupTestRouter(h)
	ctx := context.Background()

	endpointID := uuid.Must(uuid.NewV7()).String()
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO endpoints (id, section_id, address, enabled) VALUES (?, ?, ?, 1)`,
		endpointID, sectionID, "1.1.1.1"); err != nil {
		t.Fatalf("insert endpoint: %v", err)
	}
	setupTopologyPermission(t, db, sectionID, agentID.String(), endpointID)

	result := api.MonitoringResult{
		Id:         uuid.Must(uuid.NewV7()),
		AgentId:    agentID,
		CheckType:  pingCheckType(t, "1.1.1.1", 3, 1),
		EndpointId: uuid.MustParse(endpointID),
		Timestamp:  time.Now().UTC(),
	}

	w1 := postBatch(t, router, agentID, []api.MonitoringResult{result})
	if w1.Code != http.StatusAccepted {
		t.Fatalf("first: expected 202, got %d", w1.Code)
	}

	w2 := postBatch(t, router, agentID, []api.MonitoringResult{result})
	if w2.Code != http.StatusAccepted {
		t.Fatalf("second: expected 202, got %d: %s", w2.Code, w2.Body.String())
	}

	var ack api.ResultsBatchAcknowledgment
	if err := json.Unmarshal(w2.Body.Bytes(), &ack); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ack.Accepted != 0 {
		t.Errorf("expected 0 accepted on second POST, got %d", ack.Accepted)
	}
	if ack.DuplicatesSkipped == nil || *ack.DuplicatesSkipped != 1 {
		t.Errorf("expected 1 duplicate, got %v", ack.DuplicatesSkipped)
	}

	var cnt int
	db.DB().QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM check_results WHERE id = ?`, result.Id.String()).Scan(&cnt)
	if cnt != 1 {
		t.Errorf("expected exactly 1 row, got %d", cnt)
	}
}

func TestIntegration_UpdatesLastSeenAt(t *testing.T) {
	db, agentID, sectionID := setupRealDB(t)
	h := NewHandler(logger.Default(), db)
	router := setupTestRouter(h)
	ctx := context.Background()

	endpointID := uuid.Must(uuid.NewV7()).String()
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO endpoints (id, section_id, address, enabled) VALUES (?, ?, ?, 1)`,
		endpointID, sectionID, "9.9.9.9"); err != nil {
		t.Fatalf("insert endpoint: %v", err)
	}
	setupTopologyPermission(t, db, sectionID, agentID.String(), endpointID)

	start := time.Now().UTC().Add(-time.Second)
	finish := start.Add(2 * time.Second)
	result := api.MonitoringResult{
		Id:         uuid.Must(uuid.NewV7()),
		AgentId:    agentID,
		CheckType:  pingCheckType(t, "9.9.9.9", 1, 0),
		EndpointId: uuid.MustParse(endpointID),
		Timestamp:  time.Now().UTC(),
	}

	w := postBatch(t, router, agentID, []api.MonitoringResult{result})
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}

	var lastSeen sql.NullTime
	db.DB().QueryRowContext(context.Background(),
		`SELECT last_seen_at FROM agents WHERE id = ?`, agentID.String()).Scan(&lastSeen)
	if !lastSeen.Valid {
		t.Fatal("last_seen_at should be set")
	}
	if lastSeen.Time.Before(start) {
		t.Errorf("last_seen_at %v before submission time %v", lastSeen.Time, start)
	}
	if lastSeen.Time.After(finish) {
		t.Errorf("last_seen_at %v after expected finish time %v", lastSeen.Time, finish)
	}
}

func TestIntegration_EndpointIDStored(t *testing.T) {
	db, agentID, sectionID := setupRealDB(t)
	ctx := context.Background()

	endpointID := uuid.Must(uuid.NewV7()).String()
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO endpoints (id, section_id, address, enabled) VALUES (?, ?, ?, 1)`,
		endpointID, sectionID, "10.0.0.1"); err != nil {
		t.Fatalf("insert endpoint: %v", err)
	}
	setupTopologyPermission(t, db, sectionID, agentID.String(), endpointID)

	h := NewHandler(logger.Default(), db)
	router := setupTestRouter(h)
	result := api.MonitoringResult{
		Id:         uuid.Must(uuid.NewV7()),
		AgentId:    agentID,
		CheckType:  pingCheckType(t, "10.0.0.1", 2, 0),
		EndpointId: uuid.MustParse(endpointID),
		Timestamp:  time.Now().UTC(),
	}

	w := postBatch(t, router, agentID, []api.MonitoringResult{result})
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}

	var storedID string
	db.DB().QueryRowContext(ctx,
		`SELECT endpoint_id FROM check_results WHERE id = ?`, result.Id.String()).Scan(&storedID)
	if storedID != endpointID {
		t.Errorf("expected endpoint_id %q in check_results, got %q", endpointID, storedID)
	}
}

func TestIntegration_TracerouteHopsStored(t *testing.T) {
	db, agentID, sectionID := setupRealDB(t)
	h := NewHandler(logger.Default(), db)
	router := setupTestRouter(h)
	ctx := context.Background()

	endpointID := uuid.Must(uuid.NewV7()).String()
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO endpoints (id, section_id, address, enabled) VALUES (?, ?, ?, 1)`,
		endpointID, sectionID, "10.0.0.1"); err != nil {
		t.Fatalf("insert endpoint: %v", err)
	}
	setupTopologyPermission(t, db, sectionID, agentID.String(), endpointID)

	hop1addr := "192.168.1.1"
	hop2addr := "10.0.0.1"
	hops := []api.TracerouteHop{
		{Hop: 1, ResolvedIp: &hop1addr, SuccessLatencies: &[]float64{1.0, 1.1, 1.2}},
		{Hop: 2, ResolvedIp: &hop2addr, SuccessLatencies: &[]float64{5.6}},
	}
	result := api.MonitoringResult{
		Id:         uuid.Must(uuid.NewV7()),
		AgentId:    agentID,
		CheckType:  tracerouteCheckType(t, hops),
		EndpointId: uuid.MustParse(endpointID),
		Timestamp:  time.Now().UTC(),
	}

	w := postBatch(t, router, agentID, []api.MonitoringResult{result})
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}

	rows, err := db.DB().QueryContext(context.Background(),
		`SELECT hop, resolved_ip FROM check_results_traceroute_hops WHERE check_id = ? ORDER BY hop`, result.Id.String())
	if err != nil {
		t.Fatalf("query check_results_traceroute_hops: %v", err)
	}
	defer rows.Close()

	type hopRow struct {
		hop  int
		addr sql.NullString
	}
	var got []hopRow
	for rows.Next() {
		var hr hopRow
		rows.Scan(&hr.hop, &hr.addr)
		got = append(got, hr)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 hops, got %d", len(got))
	}
	if got[0].hop != 1 || !got[0].addr.Valid || got[0].addr.String != hop1addr {
		t.Errorf("hop1 wrong: %+v", got[0])
	}
	if got[1].hop != 2 || !got[1].addr.Valid || got[1].addr.String != hop2addr {
		t.Errorf("hop2 wrong: %+v", got[1])
	}
}

func TestIntegration_MixedTypeBatch(t *testing.T) {
	db, agentID, sectionID := setupRealDB(t)
	h := NewHandler(logger.Default(), db)
	router := setupTestRouter(h)
	ctx := context.Background()

	endpointID := uuid.Must(uuid.NewV7()).String()
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO endpoints (id, section_id, address, enabled) VALUES (?, ?, ?, 1)`,
		endpointID, sectionID, "8.8.8.8"); err != nil {
		t.Fatalf("insert endpoint: %v", err)
	}
	setupTopologyPermission(t, db, sectionID, agentID.String(), endpointID)

	results := []api.MonitoringResult{
		{
			Id: uuid.Must(uuid.NewV7()), AgentId: agentID,
			CheckType:  pingCheckType(t, "8.8.8.8", 3, 0),
			EndpointId: uuid.MustParse(endpointID),
			Timestamp:  time.Now().UTC(),
		},
		{
			Id: uuid.Must(uuid.NewV7()), AgentId: agentID,
			CheckType:  tracerouteCheckType(t, []api.TracerouteHop{{Hop: 1}}),
			EndpointId: uuid.MustParse(endpointID),
			Timestamp:  time.Now().UTC(),
		},
	}

	w := postBatch(t, router, agentID, results)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}

	var ack api.ResultsBatchAcknowledgment
	if err := json.Unmarshal(w.Body.Bytes(), &ack); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ack.Accepted != 2 {
		t.Errorf("expected 2 accepted, got %d", ack.Accepted)
	}

	var cnt int
	db.DB().QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM check_results WHERE agent_id = ?`, agentID.String()).Scan(&cnt)
	if cnt != 2 {
		t.Errorf("expected 2 rows in check_results, got %d", cnt)
	}
}

func TestIntegration_UnknownEndpointID_Returns422(t *testing.T) {
	db, agentID, _ := setupRealDB(t)
	h := NewHandler(logger.Default(), db)
	router := setupTestRouter(h)

	result := api.MonitoringResult{
		Id:         uuid.Must(uuid.NewV7()),
		AgentId:    agentID,
		CheckType:  pingCheckType(t, "1.2.3.4", 1, 0),
		EndpointId: uuid.Must(uuid.NewV7()), // random UUID — not in DB
		Timestamp:  time.Now().UTC(),
	}

	w := postBatch(t, router, agentID, []api.MonitoringResult{result})
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIntegration_WrongAgentEndpointID_Returns422(t *testing.T) {
	db, agentID, _ := setupRealDB(t)
	h := NewHandler(logger.Default(), db)
	router := setupTestRouter(h)
	ctx := context.Background()

	// Create a second agent, insert an endpoint under it — no topology connects the
	// first agent to this endpoint, so submission must be rejected (422).
	otherTenantID := uuid.Must(uuid.NewV7()).String()
	if _, err := db.DB().ExecContext(ctx, `INSERT INTO tenants (id, name) VALUES (?, ?)`, otherTenantID, "Other Tenant"); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	otherSectionID := uuid.Must(uuid.NewV7()).String()
	if _, err := db.DB().ExecContext(ctx, `INSERT INTO sections (id, tenant_id, name) VALUES (?, ?, ?)`, otherSectionID, otherTenantID, "Other Section"); err != nil {
		t.Fatalf("insert section: %v", err)
	}
	otherAgentID := uuid.Must(uuid.NewV7()).String()
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO agents (id, section_id, name, api_key_hash, base_config) VALUES (?, ?, ?, ?, ?)`,
		otherAgentID, otherSectionID, "other-agent", "fakehash2", "{}"); err != nil {
		t.Fatalf("insert other agent: %v", err)
	}
	otherEndpointID := uuid.Must(uuid.NewV7()).String()
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO endpoints (id, section_id, address, enabled) VALUES (?, ?, ?, 1)`,
		otherEndpointID, otherSectionID, "9.9.9.9"); err != nil {
		t.Fatalf("insert other endpoint: %v", err)
	}

	// Submit with the other agent's endpoint_id — should be rejected
	result := api.MonitoringResult{
		Id:         uuid.Must(uuid.NewV7()),
		AgentId:    agentID,
		CheckType:  pingCheckType(t, "9.9.9.9", 1, 0),
		EndpointId: uuid.MustParse(otherEndpointID),
		Timestamp:  time.Now().UTC(),
	}

	w := postBatch(t, router, agentID, []api.MonitoringResult{result})
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
}
