package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/smotra-monitoring/server/internal/config"
	"github.com/smotra-monitoring/server/internal/handlers/metrics"
	"github.com/smotra-monitoring/server/internal/testutil"
)

func TestRateLimitIntegration_LimiterDisabled_NoMetricsRegistered(t *testing.T) {
	var reached atomic.Int32
	router, registry := newRouterWithRateLimit(config.RateLimiterConfig{Enabled: false, URL: "http://127.0.0.1:1/unused"}, &reached)

	if rec := serve(router, "GET", "/v1/agents", "203.0.113.5:1"); rec.Code != http.StatusOK {
		t.Errorf("code = %d, want 200", rec.Code)
	}
	if reached.Load() != 1 {
		t.Errorf("reached = %d, want 1", reached.Load())
	}
	if strings.Contains(scrapeMetrics(t, registry), "smotra_ratelimiter_") {
		t.Error("limiter metrics must not be registered when the limiter is disabled")
	}
}

func TestRateLimitIntegration_RegistersLimiterMetrics(t *testing.T) {
	var limiterCalls, reached atomic.Int32
	svc := fakeExternalChecker(t, map[string]bool{"203.0.113.5": true}, &limiterCalls)
	router, registry := newRateLimitedRouterWithMetrics(svc.URL, true, &reached)

	serve(router, "GET", "/v1/agents", "203.0.113.5:1")

	out := scrapeMetrics(t, registry)
	for _, want := range []string{"smotra_ratelimiter_checks_total 1\n", "smotra_ratelimiter_denied_total 1\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("metrics missing %q\n%s", want, out)
		}
	}
}

func TestCountRequests_RegistersAndCounts(t *testing.T) {
	registry := metrics.NewHandler(testutil.NewSilentLogger(), "test")
	handler := CountRequests(registry)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))

	out := scrapeMetrics(t, registry)
	for _, want := range []string{"smotra_http_requests_total 1\n", "smotra_http_requests_failure_total 1\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("metrics missing %q\n%s", want, out)
		}
	}
}
