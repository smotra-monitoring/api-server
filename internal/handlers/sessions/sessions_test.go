package sessions

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/smotra-monitoring/server/internal/database"
	"github.com/smotra-monitoring/server/internal/database/queries"
	"github.com/smotra-monitoring/server/internal/logger"
	"github.com/smotra-monitoring/server/internal/middleware"
	"github.com/smotra-monitoring/server/internal/testutil"
)

type seedSession struct {
	id      string
	token   string
	userID  string
	ip      string
	ua      string
	revoked bool
	expired bool
}

// newTestDB returns a migrated SQLite DB with two users.
func newTestDB(t *testing.T) (db database.Database, userA, userB string) {
	t.Helper()
	db = testutil.SetupTestSQLiteDB(t)
	ctx := context.Background()
	testutil.ApplyMigrations(t, ctx, db.DB(), "../../../data/db/dev/migrations")

	tenantID := uuid.Must(uuid.NewV7()).String()
	workspaceID := uuid.Must(uuid.NewV7()).String()
	if _, err := db.DB().ExecContext(ctx, `INSERT INTO tenants (id, name) VALUES (?, ?)`, tenantID, "T"); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if _, err := db.DB().ExecContext(ctx, `INSERT INTO workspaces (id, tenant_id, name) VALUES (?, ?, ?)`, workspaceID, tenantID, "W"); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	mk := func(sub string) string {
		id := uuid.Must(uuid.NewV7()).String()
		if _, err := db.DB().ExecContext(ctx,
			`INSERT INTO users (id, workspace_id, oauth_provider, oauth_subject, display_name) VALUES (?, ?, ?, ?, ?)`,
			id, workspaceID, "test_provider", sub, sub); err != nil {
			t.Fatalf("create user: %v", err)
		}
		return id
	}
	return db, mk("user-a"), mk("user-b")
}

// seed inserts a session; the returned struct has id and token filled in.
func seed(t *testing.T, db database.Database, s seedSession) seedSession {
	t.Helper()
	s.id = uuid.Must(uuid.NewV7()).String()
	s.token = "tok-" + s.id

	now := time.Now().UTC()
	exp := now.Add(24 * time.Hour)
	if s.expired {
		exp = now.Add(-24 * time.Hour)
	}
	q := queries.New(db.DB())
	_, err := q.CreateSession(context.Background(), queries.CreateSessionParams{
		ID:                s.id,
		UserID:            s.userID,
		TokenHash:         middleware.HashAPIKeyForTests(s.token),
		SlidingExpiresAt:  exp,
		ExpiresAt:         exp,
		Oauth2Provider:    "test_provider",
		Oauth2AccessToken: "fake",
		Oauth2TokenType:   "Bearer",
		IpAddress:         sql.NullString{String: s.ip, Valid: s.ip != ""},
		UserAgent:         sql.NullString{String: s.ua, Valid: s.ua != ""},
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if s.revoked {
		if err := q.RevokeSession(context.Background(), s.id); err != nil {
			t.Fatalf("revoke session: %v", err)
		}
	}
	return s
}

func ctxFor(userID, sessionID string) context.Context {
	return context.WithValue(context.Background(), middleware.AuthContextKey, &middleware.AuthInfo{
		AuthType: "oauth2", Authenticated: true, UserID: userID, SessionID: sessionID,
	})
}

func isActive(t *testing.T, db database.Database, id string) bool {
	t.Helper()
	row, err := queries.New(db.DB()).GetSessionByID(context.Background(), id)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	return row.Revoked == 0
}

func TestGetMetrics_Format(t *testing.T) {
	h := NewHandler(logger.Default(), nil)
	out := h.GetMetrics()

	for _, name := range []string{
		"smotra_sessions_list_total",
		"smotra_sessions_purge_total",
		"smotra_sessions_purge_forbidden_total",
		"smotra_sessions_purge_not_found_total",
		"smotra_sessions_purge_all_total",
		"smotra_sessions_revoked_total",
	} {
		if !strings.Contains(out, "# HELP "+name+" ") || !strings.Contains(out, "# TYPE "+name+" counter") || !strings.Contains(out, name+" 0\n") {
			t.Errorf("metrics output missing HELP/TYPE/value for %s", name)
		}
	}
}

func TestMetrics_CountersIncrementPerPath(t *testing.T) {
	db, userA, userB := newTestDB(t)
	h := NewHandler(logger.Default(), db)

	current := seed(t, db, seedSession{userID: userA})
	other := seed(t, db, seedSession{userID: userA})
	foreign := seed(t, db, seedSession{userID: userB})
	ctx := ctxFor(userA, current.id)

	if _, err := h.ListSessions(ctx, listReq()); err != nil {
		t.Fatal(err)
	}
	_, _ = h.PurgeSession(ctx, purgeReq(foreign.id))
	_, _ = h.PurgeSession(ctx, purgeReq(uuid.Must(uuid.NewV7()).String()))
	_, _ = h.PurgeSession(ctx, purgeReq(other.id))
	_, _ = h.PurgeSession(ctx, purgeReq(uuid.Nil.String()))
	_, _ = h.PurgeSessions(ctx, purgeAllReq())

	checks := map[string]uint64{
		"list":         h.listSuccessTotal.Load(),
		"purge":        h.purgeTotal.Load(),
		"purgeSuccess": h.purgeSuccessTotal.Load(),
		"forbidden":    h.purgeForbidden.Load(),
		"notFound":     h.purgeNotFound.Load(),
		"badRequest":   h.purgeBadRequest.Load(),
		"purgeAll":     h.purgeAllSuccess.Load(),
		"revoked":      h.revokedTotal.Load(),
	}
	want := map[string]uint64{"list": 1, "purge": 4, "purgeSuccess": 1, "forbidden": 1, "notFound": 1, "badRequest": 1, "purgeAll": 1, "revoked": 1}
	for k, w := range want {
		if checks[k] != w {
			t.Errorf("%s counter = %d, want %d", k, checks[k], w)
		}
	}
}

func TestMetrics_ConcurrentIncrements(t *testing.T) {
	db, userA, _ := newTestDB(t)
	h := NewHandler(logger.Default(), db)
	ctx := ctxFor(userA, "current")

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = h.ListSessions(ctx, listReq())
		}()
	}
	wg.Wait()

	if got := h.listTotal.Load(); got != 50 {
		t.Errorf("listTotal = %d, want 50", got)
	}
}
