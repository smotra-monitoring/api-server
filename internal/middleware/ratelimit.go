package middleware

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"time"

	api "github.com/smotra-monitoring/server/internal/api/v1"
	"github.com/smotra-monitoring/server/internal/config"
	"github.com/smotra-monitoring/server/internal/handlers/metrics"
	"github.com/smotra-monitoring/server/internal/logger"
	"github.com/smotra-monitoring/server/internal/ratelimit"
)

// RateLimit returns a middleware that consults the external rate limiter before each request.
// When disabled in cfg it is a pass-through. It must run after ClientInfoMiddleware.
func RateLimit(log *logger.Logger, cfg config.RateLimiterConfig, registry *metrics.Handler) func(next http.Handler) http.Handler {
	if !cfg.Enabled {
		return func(next http.Handler) http.Handler { return next }
	}

	limiter := ratelimit.NewLimiter(ratelimit.NewExternalChecker(cfg.URL, cfg.Timeout), ratelimit.Options{
		FailOpen:         cfg.FailOpen,
		Cache429TTL:      time.Duration(cfg.Cache429Sec) * time.Second,
		BreakerTimeout:   time.Duration(cfg.CircuitBreakerSec) * time.Second,
		ExemptPaths:      cfg.ExemptPaths,
		WhitelistSubnets: cfg.WhitelistSubnets,
		Logger:           log,
	})
	registry.RegisterMetricsProvider(limiter)
	log.Info("rate limiter enabled", "url", cfg.URL, "timeout", cfg.Timeout, "fail_open", cfg.FailOpen)

	return newRateLimitHandler(log, limiter)
}

func newRateLimitHandler(log *logger.Logger, limiter *ratelimit.Limiter) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			decision := limiter.Check(r.Context(), ClientInfoFromContext(r.Context()).IPAddress, r.URL.Path)

			switch decision.Verdict {
			case ratelimit.Deny:
				w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds(decision.RetryAfter)))
				writeRateLimitError(w, r, log, http.StatusTooManyRequests, "too_many_requests", "Too many requests")
			case ratelimit.Unavailable:
				writeRateLimitError(w, r, log, http.StatusServiceUnavailable, "service_unavailable", "Service temporarily unavailable")
			default:
				next.ServeHTTP(w, r)
			}
		})
	}
}

func retryAfterSeconds(d time.Duration) int {
	return max(1, int(math.Ceil(d.Seconds())))
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
