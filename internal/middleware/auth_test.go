package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/smotra-monitoring/server/internal/logger"
	"github.com/smotra-monitoring/server/internal/testutil"
)

func TestHashAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		apiKey   string
		expected string
	}{
		{
			name:     "simple key",
			apiKey:   "test-key-123",
			expected: "625faa3fbbc3d2bd9d6ee7678d04cc5339cb33dc68d9b58451853d60046e226a",
		},
		{
			name:     "empty key",
			apiKey:   "",
			expected: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hashAPIKey(tt.apiKey)
			if result != tt.expected {
				t.Errorf("hashAPIKey(%q) = %q, want %q", tt.apiKey, result, tt.expected)
			}
		})
	}
}

func TestExtractAgentIDFromPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "valid agent configuration path",
			path:     "/agent/019bdeb2-50dc-794e-808b-cf47526b867f/configuration",
			expected: "019bdeb2-50dc-794e-808b-cf47526b867f",
		},
		{
			name:     "valid agent path with trailing slash",
			path:     "/agent/019bdeb2-50dc-794e-808b-cf47526b867f/",
			expected: "019bdeb2-50dc-794e-808b-cf47526b867f",
		},
		{
			name:     "valid agent path without trailing",
			path:     "/agent/019bdeb2-50dc-794e-808b-cf47526b867f",
			expected: "019bdeb2-50dc-794e-808b-cf47526b867f",
		},
		{
			name:     "valid path with api prefix",
			path:     "/v1/agent/019bdeb2-50dc-794e-808b-cf47526b867f/configuration",
			expected: "019bdeb2-50dc-794e-808b-cf47526b867f",
		},
		{
			name:     "no agent in path",
			path:     "/v1/health",
			expected: "",
		},
		{
			name:     "agent at end of path",
			path:     "/v1/agent",
			expected: "",
		},
		{
			name:     "empty path",
			path:     "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractAgentIDFromPath(tt.path)
			if result != tt.expected {
				t.Errorf("extractAgentIDFromPath(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

func TestAgentAPIKeyAuth_NoAPIKey(t *testing.T) {
	log := logger.New(logger.Config{Level: "error", Format: "json"})
	mockDB := testutil.NewMockDatabase()

	middleware := AgentAPIKeyAuth(log, mockDB)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	req := httptest.NewRequest("GET", "/agent/019bdeb2-50dc-794e-808b-cf47526b867f/configuration", nil)
	w := httptest.NewRecorder()

	middleware(handler).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "OK" {
		t.Errorf("Expected body 'OK', got %q", w.Body.String())
	}
}

func TestAgentAPIKeyAuth_WithAPIKeyButNoAgentInPath(t *testing.T) {
	log := logger.New(logger.Config{Level: "error", Format: "json"})
	mockDB := testutil.NewMockDatabase()

	middleware := AgentAPIKeyAuth(log, mockDB)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		authInfo := ctx.Value(AuthContextKey)
		if authInfo != nil {
			info, ok := authInfo.(*AuthInfo)
			if ok && info.Authenticated {
				t.Error("Expected authentication to fail for non-existent agent")
			}
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	req := httptest.NewRequest("GET", "/v1/health", nil)
	req.Header.Set("X-Agent-API-Key", "test-key")
	w := httptest.NewRecorder()

	middleware(handler).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestOAuth2Auth_NoBearerToken(t *testing.T) {
	log := logger.New(logger.Config{Level: "error", Format: "json"})
	mockDB := testutil.NewMockDatabase()

	middleware := OAuth2Auth(log, mockDB)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	req := httptest.NewRequest("GET", "/agent/019bdeb2-50dc-794e-808b-cf47526b867f/configuration", nil)
	w := httptest.NewRecorder()

	middleware(handler).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// TestOAuth2Auth_UnknownToken verifies that an unrecognized Bearer token results
// in the request being passed through unauthenticated (no 401 — the handler decides).
func TestOAuth2Auth_UnknownToken(t *testing.T) {
	log := logger.New(logger.Config{Level: "error", Format: "json"})
	// Use a real SQLite DB so GetSessionByTokenHash returns "not found" cleanly.
	db := testutil.SetupTestSQLiteDB(t)

	mw := OAuth2Auth(log, db)

	var capturedCtx context.Context
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCtx = r.Context()
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/agent/019bdeb2-50dc-794e-808b-cf47526b867f/configuration", nil)
	req.Header.Set("Authorization", "Bearer st_test_unknown")
	w := httptest.NewRecorder()

	mw(handler).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// AuthInfo should not be set (or Authenticated=false) for an unknown token.
	if info, ok := capturedCtx.Value(AuthContextKey).(*AuthInfo); ok && info != nil && info.Authenticated {
		t.Error("Expected Authenticated=false for an unknown token")
	}
}

func TestRequireAuth_NoAuth(t *testing.T) {
	log := logger.New(logger.Config{Level: "error", Format: "json"})

	middleware := RequireAuthForTests(log)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called without authentication")
	})

	req := httptest.NewRequest("GET", "/agent/019bdeb2-50dc-794e-808b-cf47526b867f/configuration", nil)
	w := httptest.NewRecorder()

	middleware(handler).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func TestRequireAuth_WithAuth(t *testing.T) {
	log := logger.New(logger.Config{Level: "error", Format: "json"})

	middleware := RequireAuthForTests(log)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Create request with authentication info in context
	authInfo := &AuthInfo{
		AgentID:       "019bdeb2-50dc-794e-808b-cf47526b867f",
		AuthType:      "agent_api_key",
		Authenticated: true,
	}
	ctx := context.WithValue(context.Background(), AuthContextKey, authInfo)

	req := httptest.NewRequest("GET", "/agent/019bdeb2-50dc-794e-808b-cf47526b867f/configuration", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	middleware(handler).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}
