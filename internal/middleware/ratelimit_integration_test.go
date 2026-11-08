package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	healthAPI "github.com/smotra-monitoring/server/internal/api/health"
	"github.com/smotra-monitoring/server/internal/config"
	"github.com/smotra-monitoring/server/internal/handlers/metrics"
	"github.com/smotra-monitoring/server/internal/testutil"
)

// fakeExternalChecker answers 429 for blocked IPs and 200 otherwise, like the real limiter.
func fakeExternalChecker(t *testing.T, blocked map[string]bool, calls *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body struct {
			IP string `json:"ip"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if blocked[body.IP] {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newRateLimitedRouterWithMetrics(limiterURL string, failOpen bool, reached *atomic.Int32) (http.Handler, *metrics.Handler) {
	cfg := config.RateLimiterConfig{
		Enabled:           true,
		URL:               limiterURL,
		Timeout:           20 * time.Millisecond,
		FailOpen:          failOpen,
		Cache429Sec:       15,
		CircuitBreakerSec: 60,
		ExemptPaths:       []string{"/healthz"},
		WhitelistSubnets:  []string{"10.0.0.0/8"},
	}
	return newRouterWithRateLimit(cfg, reached)
}

func newRouterWithRateLimit(cfg config.RateLimiterConfig, reached *atomic.Int32) (http.Handler, *metrics.Handler) {
	log := testutil.NewSilentLogger()
	registry := metrics.NewHandler(log, "test")

	r := chi.NewRouter()
	r.Use(RequestID(log))
	r.Use(ClientInfoMiddleware(nil))
	r.Use(RateLimit(log, cfg, registry))

	handler := func(w http.ResponseWriter, _ *http.Request) {
		reached.Add(1)
		w.WriteHeader(http.StatusOK)
	}
	r.Get("/healthz", handler)
	r.Get("/v1/agents", handler)
	r.Post("/v1/agent/register", handler)
	r.Delete("/v1/sessions", handler)
	return r, registry
}

func scrapeMetrics(t *testing.T, registry *metrics.Handler) string {
	t.Helper()
	resp, err := registry.PrometheusMetrics(context.Background(), struct{}{})
	if err != nil {
		t.Fatalf("PrometheusMetrics: %v", err)
	}
	return string(resp.(healthAPI.PrometheusMetrics200TextResponse))
}

func serve(h http.Handler, method, path, remote string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = remote
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRateLimitIntegration_BlockedIPNeverReachesHandlers(t *testing.T) {
	var limiterCalls, reached atomic.Int32
	svc := fakeExternalChecker(t, map[string]bool{"203.0.113.5": true}, &limiterCalls)
	router, _ := newRateLimitedRouterWithMetrics(svc.URL, true, &reached)

	for _, tc := range []struct{ method, path string }{
		{"GET", "/v1/agents"},
		{"POST", "/v1/agent/register"},
		{"DELETE", "/v1/sessions"},
	} {
		rec := serve(router, tc.method, tc.path, "203.0.113.5:5000")
		if rec.Code != http.StatusTooManyRequests {
			t.Errorf("%s %s: code = %d, want 429", tc.method, tc.path, rec.Code)
		}
		if rec.Header().Get("Retry-After") == "" {
			t.Errorf("%s %s: missing Retry-After", tc.method, tc.path)
		}
		if rec.Header().Get("X-Request-ID") == "" {
			t.Errorf("%s %s: request ID middleware did not run before limiter", tc.method, tc.path)
		}
	}
	if reached.Load() != 0 {
		t.Errorf("handlers reached %d times for a blocked IP", reached.Load())
	}
	if limiterCalls.Load() != 1 {
		t.Errorf("limiter calls = %d, want 1 (429 cached afterwards)", limiterCalls.Load())
	}
}

func TestRateLimitIntegration_AllowedWhitelistedAndExempt(t *testing.T) {
	var limiterCalls, reached atomic.Int32
	svc := fakeExternalChecker(t, map[string]bool{"203.0.113.5": true, "10.0.0.7": true}, &limiterCalls)
	router, _ := newRateLimitedRouterWithMetrics(svc.URL, true, &reached)

	// Allowed IP is served normally.
	if rec := serve(router, "GET", "/v1/agents", "198.51.100.1:1"); rec.Code != http.StatusOK {
		t.Errorf("allowed IP code = %d, want 200", rec.Code)
	}
	if limiterCalls.Load() != 1 {
		t.Errorf("limiter calls = %d, want 1", limiterCalls.Load())
	}

	// Whitelisted client is served even though the service would block it, without any call.
	if rec := serve(router, "GET", "/v1/agents", "10.0.0.7:1"); rec.Code != http.StatusOK {
		t.Errorf("whitelisted code = %d, want 200", rec.Code)
	}
	// Exempt path is served for a blocked IP, without any call.
	if rec := serve(router, "GET", "/healthz", "203.0.113.5:1"); rec.Code != http.StatusOK {
		t.Errorf("exempt path code = %d, want 200", rec.Code)
	}

	if limiterCalls.Load() != 1 {
		t.Errorf("limiter calls = %d, want still 1", limiterCalls.Load())
	}

	// Normal path is blocked for a blocked IP, without any call.
	if rec := serve(router, "GET", "/v1/agents", "203.0.113.5:1"); rec.Code != http.StatusTooManyRequests {
		t.Errorf("normal path code = %d, want 429", rec.Code)
	}

	if limiterCalls.Load() != 2 {
		t.Errorf("limiter calls = %d, want 2", limiterCalls.Load())
	}
}

func TestRateLimitIntegration_LimiterDown(t *testing.T) {
	var limiterCalls, reached atomic.Int32
	svc := fakeExternalChecker(t, nil, &limiterCalls)
	url := svc.URL
	svc.Close()

	t.Run("fail open", func(t *testing.T) {
		reached.Store(0)
		router, _ := newRateLimitedRouterWithMetrics(url, true, &reached)
		for i := 0; i < 3; i++ {
			if rec := serve(router, "GET", "/v1/agents", "198.51.100.1:1"); rec.Code != http.StatusOK {
				t.Fatalf("code = %d, want 200", rec.Code)
			}
		}
		if reached.Load() != 3 {
			t.Errorf("reached = %d, want 3", reached.Load())
		}
	})

	t.Run("fail closed", func(t *testing.T) {
		reached.Store(0)
		router, _ := newRateLimitedRouterWithMetrics(url, false, &reached)
		if rec := serve(router, "GET", "/v1/agents", "198.51.100.1:1"); rec.Code != http.StatusServiceUnavailable {
			t.Errorf("code = %d, want 503", rec.Code)
		}
		if reached.Load() != 0 {
			t.Errorf("reached = %d, want 0", reached.Load())
		}
	})
}

func TestRateLimitIntegration_LimiterTimeoutTripsBreaker(t *testing.T) {
	var calls, reached atomic.Int32
	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		<-release
	}))
	defer slow.Close()
	defer close(release)

	router, _ := newRateLimitedRouterWithMetrics(slow.URL, true, &reached)
	for i := 0; i < 3; i++ {
		if rec := serve(router, "GET", "/v1/agents", "198.51.100.1:1"); rec.Code != http.StatusOK {
			t.Fatalf("code = %d, want 200 (fail open)", rec.Code)
		}
	}
	if calls.Load() != 1 {
		t.Errorf("slow limiter called %d times, want 1 (breaker open afterwards)", calls.Load())
	}
}
