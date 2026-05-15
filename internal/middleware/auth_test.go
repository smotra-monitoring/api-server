package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
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

	middleware := OAuth2Auth(log)

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

func TestOAuth2Auth_WithBearerToken(t *testing.T) {
	log := logger.New(logger.Config{Level: "error", Format: "json"})

	middleware := OAuth2Auth(log)

	var capturedCtx context.Context
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCtx = r.Context()
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/agent/019bdeb2-50dc-794e-808b-cf47526b867f/configuration", nil)
	req.Header.Set("Authorization", "Bearer token123")
	w := httptest.NewRecorder()

	middleware(handler).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	authInfo, ok := capturedCtx.Value(AuthContextKey).(*AuthInfo)
	if !ok || authInfo == nil {
		t.Fatal("Expected AuthInfo in context, got none")
	}
	if authInfo.AuthType != "oauth2" {
		t.Errorf("Expected AuthType=oauth2, got %q", authInfo.AuthType)
	}
	if authInfo.BearerToken != "Bearer token123" {
		t.Errorf("Expected BearerToken='Bearer token123', got %q", authInfo.BearerToken)
	}
	if authInfo.Authenticated {
		t.Error("Expected Authenticated=false (token not validated)")
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

// ─── RequireAuthForTests: present but not authenticated ───────────────────────

func TestRequireAuth_PresentButNotAuthenticated_Returns401(t *testing.T) {
	log := logger.New(logger.Config{Level: "error", Format: "json"})

	mw := RequireAuthForTests(log)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called when auth is invalid")
	})

	// AuthInfo present but Authenticated=false
	authInfo := &AuthInfo{
		AgentID:       "019bdeb2-50dc-794e-808b-cf47526b867f",
		AuthType:      "agent_api_key",
		Authenticated: false,
	}
	ctx := context.WithValue(context.Background(), AuthContextKey, authInfo)

	req := httptest.NewRequest("GET", fmt.Sprintf("/agent/%s/configuration", authInfo.AgentID), nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	mw(handler).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// ─── getRequestIDFromHeader ───────────────────────────────────────────────────

func TestGetRequestIDFromHeader_Empty_ReturnsNil(t *testing.T) {
	log := logger.New(logger.Config{Level: "error", Format: "json"})
	req := httptest.NewRequest("GET", "/", nil)
	result := getRequestIDFromHeader(req, log)
	if result != nil {
		t.Errorf("expected nil for missing header, got %v", result)
	}
}

func TestGetRequestIDFromHeader_ValidUUID_ReturnsUUID(t *testing.T) {
	log := logger.New(logger.Config{Level: "error", Format: "json"})
	req := httptest.NewRequest("GET", "/", nil)
	validID := uuid.MustParse("019bdeb2-50dc-794e-808b-cf47526b867f")
	req.Header.Set("X-Request-ID", validID.String())

	result := getRequestIDFromHeader(req, log)
	if result == nil {
		t.Fatal("expected non-nil UUID, got nil")
	}
	if *result != validID {
		t.Errorf("expected %v, got %v", validID, *result)
	}
}

func TestGetRequestIDFromHeader_InvalidUUID_ReturnsNil(t *testing.T) {
	log := logger.New(logger.Config{Level: "error", Format: "json"})
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-ID", "not-a-valid-uuid")

	result := getRequestIDFromHeader(req, log)
	if result != nil {
		t.Errorf("expected nil for invalid UUID, got %v", result)
	}
}

// ─── HashAPIKeyForTests ───────────────────────────────────────────────────────

func TestHashAPIKeyForTests_MatchesInternalHash(t *testing.T) {
	key := "my-secret-api-key"
	result1 := HashAPIKeyForTests(key)
	result2 := HashAPIKeyForTests(key)
	if result1 != result2 {
		t.Error("HashAPIKeyForTests is not deterministic")
	}
	if result1 == "" {
		t.Error("HashAPIKeyForTests returned empty string")
	}
	if result1 != hashAPIKey(key) {
		t.Errorf("HashAPIKeyForTests(%q) = %q, hashAPIKey(%q) = %q", key, result1, key, hashAPIKey(key))
	}
}
