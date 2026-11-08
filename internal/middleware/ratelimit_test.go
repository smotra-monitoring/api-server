package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smotra-monitoring/server/internal/ratelimit"
	"github.com/smotra-monitoring/server/internal/testutil"
)

type stubChecker struct {
	calls   atomic.Int32
	allowed bool
	err     error
	lastIP  atomic.Value
}

func (s *stubChecker) Allow(_ context.Context, ip string) (bool, error) {
	s.calls.Add(1)
	s.lastIP.Store(ip)
	return s.allowed, s.err
}

type rlHarness struct {
	checker *stubChecker
	limiter *ratelimit.Limiter
	handler http.Handler
	reached *atomic.Int32
}

func newRLHarness(t *testing.T, checker *stubChecker, failOpen bool, exempt, whitelist []string) *rlHarness {
	t.Helper()

	limiter := ratelimit.NewLimiter(checker, ratelimit.Options{
		FailOpen:         failOpen,
		Cache429TTL:      15 * time.Second,
		BreakerTimeout:   60 * time.Second,
		ExemptPaths:      exempt,
		WhitelistSubnets: whitelist,
	})
	var reached atomic.Int32
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached.Add(1)
		w.WriteHeader(http.StatusOK)
	})

	chain := ClientInfoMiddleware(nil)(newRateLimitHandler(testutil.NewSilentLogger(), limiter)(next))
	return &rlHarness{checker: checker, limiter: limiter, handler: chain, reached: &reached}
}

func (h *rlHarness) do(method, path, remoteAddr string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = remoteAddr
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)
	return rec
}

func TestRateLimit_Allowed(t *testing.T) {
	h := newRLHarness(t, &stubChecker{allowed: true}, true, nil, nil)

	rec := h.do("GET", "/v1/agents", "203.0.113.5:4000")

	if rec.Code != http.StatusOK || h.reached.Load() != 1 {
		t.Errorf("code=%d reached=%d, want 200 and 1", rec.Code, h.reached.Load())
	}
	if ip, _ := h.checker.lastIP.Load().(string); ip != "203.0.113.5" {
		t.Errorf("limiter received ip %q", ip)
	}
}

func TestRateLimit_DeniedReturns429WithoutReachingHandler(t *testing.T) {
	h := newRLHarness(t, &stubChecker{allowed: false}, true, nil, nil)

	for _, method := range []string{"GET", "POST", "DELETE"} {
		rec := h.do(method, "/v1/agent/register", "203.0.113.5:4000")

		if rec.Code != http.StatusTooManyRequests {
			t.Fatalf("%s: code = %d, want 429", method, rec.Code)
		}
		if rec.Header().Get("Retry-After") != "15" {
			t.Errorf("%s: Retry-After = %q, want 15", method, rec.Header().Get("Retry-After"))
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("%s: Content-Type = %q", method, ct)
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: body is not JSON: %v", method, err)
		}
		if body["error"] != "too_many_requests" || body["message"] == "" {
			t.Errorf("%s: unexpected body %v", method, body)
		}
	}
	if h.reached.Load() != 0 {
		t.Errorf("handler reached %d times, want 0", h.reached.Load())
	}
	if h.checker.calls.Load() != 1 {
		t.Errorf("limiter calls = %d, want 1 (rest served from 429 cache)", h.checker.calls.Load())
	}
}

func TestRateLimit_DenyIsPerIP(t *testing.T) {
	h := newRLHarness(t, &stubChecker{allowed: false}, true, nil, nil)

	h.do("GET", "/v1/x", "203.0.113.5:1")
	h.checker.allowed = true
	if rec := h.do("GET", "/v1/x", "203.0.113.6:1"); rec.Code != http.StatusOK {
		t.Errorf("other IP code = %d, want 200", rec.Code)
	}
	if rec := h.do("GET", "/v1/x", "203.0.113.5:2"); rec.Code != http.StatusTooManyRequests {
		t.Errorf("blocked IP code = %d, want 429", rec.Code)
	}
}

func TestRateLimit_ExemptPaths(t *testing.T) {
	h := newRLHarness(t, &stubChecker{allowed: false}, true, []string{"/healthz", "/metrics"}, nil)

	for _, path := range []string{"/healthz", "/healthz/ready", "/metrics"} {
		if rec := h.do("GET", path, "203.0.113.5:1"); rec.Code != http.StatusOK {
			t.Errorf("%s code = %d, want 200", path, rec.Code)
		}
	}
	if h.checker.calls.Load() != 0 {
		t.Errorf("limiter called %d times for exempt paths", h.checker.calls.Load())
	}
	if rec := h.do("GET", "/v1/agents", "203.0.113.5:1"); rec.Code != http.StatusTooManyRequests {
		t.Errorf("non-exempt path code = %d, want 429", rec.Code)
	}
}

func TestRateLimit_FailOpen(t *testing.T) {
	h := newRLHarness(t, &stubChecker{err: context.DeadlineExceeded}, true, nil, nil)

	if rec := h.do("GET", "/v1/x", "203.0.113.5:1"); rec.Code != http.StatusOK || h.reached.Load() != 1 {
		t.Errorf("code=%d reached=%d, want 200 and 1", rec.Code, h.reached.Load())
	}
}

func TestRateLimit_FailClosed(t *testing.T) {
	h := newRLHarness(t, &stubChecker{err: context.DeadlineExceeded}, false, nil, nil)

	rec := h.do("GET", "/v1/x", "203.0.113.5:1")
	if rec.Code != http.StatusServiceUnavailable || h.reached.Load() != 0 {
		t.Errorf("code=%d reached=%d, want 503 and 0", rec.Code, h.reached.Load())
	}
}

func TestRateLimit_BreakerSkipsLimiter(t *testing.T) {
	h := newRLHarness(t, &stubChecker{err: context.DeadlineExceeded}, true, nil, nil)

	for i := 0; i < 5; i++ {
		h.do("GET", "/v1/x", "203.0.113.5:1")
	}
	if h.checker.calls.Load() != 1 {
		t.Errorf("limiter calls = %d, want 1 while breaker is open", h.checker.calls.Load())
	}
	if h.reached.Load() != 5 {
		t.Errorf("reached = %d, want 5 (fail-open)", h.reached.Load())
	}
}

func TestRateLimit_WhitelistBypassesLimiter(t *testing.T) {
	h := newRLHarness(t, &stubChecker{allowed: false}, true, nil, []string{"10.0.0.0/8", "192.0.2.9"})

	for _, addr := range []string{"10.1.2.3:1", "192.0.2.9:1"} {
		if rec := h.do("GET", "/v1/x", addr); rec.Code != http.StatusOK {
			t.Errorf("%s code = %d, want 200", addr, rec.Code)
		}
	}
	if h.checker.calls.Load() != 0 {
		t.Errorf("limiter called %d times for whitelisted IPs", h.checker.calls.Load())
	}
	if rec := h.do("GET", "/v1/x", "11.0.0.1:1"); rec.Code != http.StatusTooManyRequests {
		t.Errorf("non-whitelisted code = %d, want 429", rec.Code)
	}
}

func TestRateLimit_WhitelistWorksWhileBreakerOpenAndFailClosed(t *testing.T) {
	h := newRLHarness(t, &stubChecker{err: context.DeadlineExceeded}, false, nil, []string{"10.0.0.0/8"})

	h.do("GET", "/v1/x", "203.0.113.5:1") // trips the breaker
	if rec := h.do("GET", "/v1/x", "10.0.0.1:1"); rec.Code != http.StatusOK {
		t.Errorf("whitelisted code = %d, want 200", rec.Code)
	}
}

func TestRateLimit_WhitelistUsesTrustedProxyResolvedIP(t *testing.T) {
	checker := &stubChecker{allowed: false}
	limiter := ratelimit.NewLimiter(checker, ratelimit.Options{FailOpen: true, Cache429TTL: 15 * time.Second, BreakerTimeout: time.Minute, WhitelistSubnets: []string{"10.0.0.0/8"}})
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := ClientInfoMiddleware([]string{"172.16.0.1"})(newRateLimitHandler(testutil.NewSilentLogger(), limiter)(next))

	do := func(remote, xff string) int {
		req := httptest.NewRequest("GET", "/v1/x", nil)
		req.RemoteAddr = remote
		if xff != "" {
			req.Header.Set("X-Forwarded-For", xff)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := do("172.16.0.1:1", "10.9.9.9"); code != http.StatusOK {
		t.Errorf("whitelisted client behind trusted proxy: code = %d, want 200", code)
	}
	// An untrusted peer cannot claim a whitelisted address via X-Forwarded-For.
	if code := do("203.0.113.5:1", "10.9.9.9"); code != http.StatusTooManyRequests {
		t.Errorf("spoofed XFF from untrusted peer: code = %d, want 429", code)
	}
}

func TestRateLimit_UnknownClientIPPassesThrough(t *testing.T) {
	checker := &stubChecker{allowed: false}
	limiter := ratelimit.NewLimiter(checker, ratelimit.Options{FailOpen: true, Cache429TTL: time.Minute, BreakerTimeout: time.Minute})
	var reached atomic.Int32
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { reached.Add(1) })

	// No ClientInfoMiddleware: context carries no IP.
	newRateLimitHandler(testutil.NewSilentLogger(), limiter)(next).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/v1/x", nil))

	if reached.Load() != 1 || checker.calls.Load() != 0 {
		t.Errorf("reached=%d calls=%d, want 1 and 0", reached.Load(), checker.calls.Load())
	}
}

func TestRateLimit_MetricsCounters(t *testing.T) {
	h := newRLHarness(t, &stubChecker{allowed: false}, true, []string{"/healthz"}, []string{"10.0.0.0/8"})

	h.do("GET", "/v1/x", "203.0.113.5:1")    // denied via limiter
	h.do("GET", "/v1/x", "203.0.113.5:1")    // denied via cache
	h.do("GET", "/v1/x", "10.0.0.1:1")       // whitelisted
	h.do("GET", "/healthz", "203.0.113.5:1") // exempt, not counted

	out := h.limiter.GetMetrics()
	for _, want := range []string{
		"smotra_ratelimiter_checks_total 2\n",
		"smotra_ratelimiter_denied_total 2\n",
		"smotra_ratelimiter_cache_hits_total 1\n",
		"smotra_ratelimiter_whitelisted_total 1\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("metrics missing %q\n%s", want, out)
		}
	}
}
