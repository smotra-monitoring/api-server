package middleware

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	api "github.com/smotra-monitoring/server/internal/api/v1"
	"github.com/smotra-monitoring/server/internal/database"
	"github.com/smotra-monitoring/server/internal/database/queries"
	"github.com/smotra-monitoring/server/internal/logger"
)

// ContextKey is a type for context keys
type ContextKey string

const (
	// AuthContextKey is the context key for authentication info
	AuthContextKey ContextKey = "auth"
)

// AuthInfo contains authentication information
type AuthInfo struct {
	AgentID       string
	AuthType      string // "agent_api_key" or "oauth2"
	Authenticated bool
}

// AgentAPIKeyAuth returns a middleware that authenticates using agent API keys
func AgentAPIKeyAuth(log *logger.Logger, db database.Database) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract API key from X-Agent-API-Key header
			apiKey := r.Header.Get("X-Agent-API-Key")

			// If no API key, continue to next middleware/handler (might be OAuth2)
			if apiKey == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Extract agent ID from the URL path
			// Path format: /agent/{agentId}/configuration
			agentID := extractAgentIDFromPath(r.URL.Path)

			// If API key present but no agent ID in path, that's an error
			if agentID == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Check if authentication is already successful from another method
			if authInfo := r.Context().Value(AuthContextKey); authInfo != nil {
				if info, ok := authInfo.(*AuthInfo); ok && info.Authenticated {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Verify API key
			authenticated, err := verifyAgentAPIKey(r.Context(), db, agentID, apiKey)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			// Set authentication info in context
			authInfo := &AuthInfo{
				AgentID:       agentID,
				AuthType:      "agent_api_key",
				Authenticated: authenticated,
			}
			ctx := context.WithValue(r.Context(), AuthContextKey, authInfo)

			log.Info("Agent authentication via API key", "agent_id", agentID, "authenticated", authenticated)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OAuth2Auth returns a middleware that handles OAuth2 authentication (stub)
func OAuth2Auth(log *logger.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if authentication is already successful
			if authInfo := r.Context().Value(AuthContextKey); authInfo != nil {
				if info, ok := authInfo.(*AuthInfo); ok && info.Authenticated {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Extract Bearer token from Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Check if it's a Bearer token
			if strings.HasPrefix(authHeader, "Bearer ") {
				log.Warn("OAuth2 authentication attempted but not implemented")

				response := api.GetAgentConfiguration501JSONResponse{}
				response.Error = "not_implemented"
				response.Message = "OAuth2 authentication is not yet implemented"
				response.RequestId = getRequestIDFromHeader(r, log)

				response.VisitGetAgentConfigurationResponse(w)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAuthForTests returns a middleware that requires authentication
func RequireAuthForTests(log *logger.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authInfo := r.Context().Value(AuthContextKey)
			if authInfo == nil {
				log.Warn("No authentication provided", "path", r.URL.Path)

				response := api.GetAgentConfiguration401JSONResponse{}
				response.Error = "unauthorized"
				response.Message = "No authentication provided - neither Agent API Key nor OAuth2 token was provided"
				response.RequestId = getRequestIDFromHeader(r, log)

				response.VisitGetAgentConfigurationResponse(w)
				return
			}

			info, ok := authInfo.(*AuthInfo)
			if !ok || !info.Authenticated {
				log.Warn("Invalid authentication", "path", r.URL.Path)

				response := api.GetAgentConfiguration401JSONResponse{}
				response.Error = "unauthorized"
				response.Message = "Provided authentication is invalid"
				response.RequestId = getRequestIDFromHeader(r, log)

				response.VisitGetAgentConfigurationResponse(w)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func getRequestIDFromHeader(r *http.Request, log *logger.Logger) *uuid.UUID {
	requestIDStr := r.Header.Get("X-Request-ID")
	if requestIDStr == "" {
		return nil
	}
	requestID, err := uuid.Parse(requestIDStr)
	if err != nil {
		log.Error("Failed to parse X-Request-ID to UUID", "error", err)
		return nil
	}
	return &requestID
}

// verifyAgentAPIKey verifies the API key for a given agent
func verifyAgentAPIKey(ctx context.Context, db database.Database, agentID, apiKey string) (bool, error) {
	q := queries.New(db.DB())

	// Get the stored API key hash for the agentDB
	agentDB, err := q.VerifyAgentAPIKey(ctx, agentID)
	if err == sql.ErrNoRows {
		return false, nil // Agent not found
	} else if err != nil {
		return false, fmt.Errorf("failed to get agent: %w", err)
	}

	// Hash the provided API key
	hashedKey := hashAPIKey(apiKey)

	// Compare using constant-time comparison to prevent timing attacks
	return subtle.ConstantTimeCompare([]byte(hashedKey), []byte(agentDB.ApiKeyHash)) == 1, nil
}

// hashAPIKey hashes an API key using SHA-256
func hashAPIKey(apiKey string) string {
	hash := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(hash[:])
}

// HashAPIKeyForTests exports hashAPIKey for testing purposes
func HashAPIKeyForTests(apiKey string) string {
	return hashAPIKey(apiKey)
}

// extractAgentIDFromPath extracts the agent ID from URL paths like /agent/{agentId}/...
func extractAgentIDFromPath(path string) string {
	// Remove leading/trailing slashes and split
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")

	// Look for pattern: .../agent/{agentId}/...
	for i, part := range parts {
		if part == "agent" && i+1 < len(parts) {
			return parts[i+1]
		}
	}

	return ""
}
