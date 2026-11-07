package middleware

import (
	"encoding/json"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"

	api "github.com/smotra-monitoring/server/internal/api/v1"
	"github.com/smotra-monitoring/server/internal/logger"
	"github.com/smotra-monitoring/server/internal/ratelimit"
)

// RateLimit returns a middleware that consults the external rate limiter before each request.
// Paths with an exempt prefix and clients inside whitelistSubnets bypass the check.
// It must run after ClientInfoMiddleware.
func RateLimit(log *logger.Logger, limiter *ratelimit.Limiter, exemptPaths, whitelistSubnets []string) func(next http.Handler) http.Handler {
	whitelist := parseTrustedProxies(whitelistSubnets)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if hasAnyPrefix(r.URL.Path, exemptPaths) {
				next.ServeHTTP(w, r)
				return
			}

			ipStr := ClientInfoFromContext(r.Context()).IPAddress
			ip := net.ParseIP(ipStr)
			if ip == nil {
				log.Warn("rate limiter skipped: client IP unknown", "client_ip", ipStr, "path", r.URL.Path)
				next.ServeHTTP(w, r)
				return
			}

			if isTrusted(ip, whitelist) {
				limiter.RecordWhitelisted()
				next.ServeHTTP(w, r)
				return
			}

			decision := limiter.Check(r.Context(), ip.String())
			switch decision.Verdict {
			case ratelimit.Deny:
				secs := int(math.Ceil(decision.RetryAfter.Seconds()))
				if secs < 1 {
					secs = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(secs))
				writeRateLimitError(w, r, log, http.StatusTooManyRequests, "too_many_requests", "Too many requests")
			case ratelimit.Unavailable:
				log.Warn("rate limiter unavailable, rejecting request", "path", r.URL.Path, "client_ip", ipStr)
				writeRateLimitError(w, r, log, http.StatusServiceUnavailable, "service_unavailable", "Service temporarily unavailable")
			default:
				next.ServeHTTP(w, r)
			}
		})
	}
}

func hasAnyPrefix(path string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

func writeRateLimitError(w http.ResponseWriter, r *http.Request, log *logger.Logger, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body := api.Error{
		Error:     code,
		Message:   message,
		RequestId: getRequestIDFromHeader(r, log),
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Error("failed to write rate limit response", "error", err)
	}
}
