package ratelimit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestClient_Allow_StatusMapping(t *testing.T) {
	for _, tc := range []struct {
		name        string
		status      int
		wantAllowed bool
		wantErr     bool
	}{
		{"200 allows", http.StatusOK, true, false},
		{"429 denies", http.StatusTooManyRequests, false, false},
		{"500 is error", http.StatusInternalServerError, false, true},
		{"404 is error", http.StatusNotFound, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()

			allowed, err := NewClient(srv.URL, time.Second).Allow(context.Background(), "1.2.3.4")
			if allowed != tc.wantAllowed || (err != nil) != tc.wantErr {
				t.Errorf("Allow() = (%v, %v), want allowed=%v err=%v", allowed, err, tc.wantAllowed, tc.wantErr)
			}
		})
	}
}

func TestClient_Allow_SendsGetWithJSONBody(t *testing.T) {
	var method, contentType string
	var got checkRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		contentType = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL, time.Second).Allow(context.Background(), "203.0.113.7"); err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if method != http.MethodGet {
		t.Errorf("method = %s, want GET", method)
	}
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q", contentType)
	}
	if got.IP != "203.0.113.7" {
		t.Errorf("body ip = %q", got.IP)
	}
}

func TestClient_Allow_Timeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
	}))
	defer srv.Close()
	defer close(release)

	start := time.Now()
	_, err := NewClient(srv.URL, 5*time.Millisecond).Allow(context.Background(), "1.2.3.4")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("call took %s, timeout not enforced", elapsed)
	}
}

func TestClient_Allow_ConnectionRefused(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	if _, err := NewClient(url, 5*time.Millisecond).Allow(context.Background(), "1.2.3.4"); err == nil {
		t.Error("expected error when limiter is down")
	}
}

// fakeChecker is a scriptable Checker.
type fakeChecker struct {
	calls   atomic.Int32
	allowed bool
	err     error
}

func (f *fakeChecker) Allow(context.Context, string) (bool, error) {
	f.calls.Add(1)
	return f.allowed, f.err
}

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newTestLimiter(c Checker, failOpen bool) (*Limiter, *fakeClock) {
	clock := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	l := NewLimiter(c, Options{FailOpen: failOpen, Cache429TTL: 15 * time.Second, BreakerTimeout: 60 * time.Second})
	l.now = clock.Now
	return l, clock
}

func TestLimiter_Allowed(t *testing.T) {
	fc := &fakeChecker{allowed: true}
	l, _ := newTestLimiter(fc, true)

	for i := 0; i < 3; i++ {
		if d := l.Check(context.Background(), "1.1.1.1"); d.Verdict != Allow {
			t.Fatalf("verdict = %v, want Allow", d.Verdict)
		}
	}
	if fc.calls.Load() != 3 {
		t.Errorf("allowed decisions must not be cached; calls = %d, want 3", fc.calls.Load())
	}
}

func TestLimiter_DenyIsCachedUntilTTL(t *testing.T) {
	fc := &fakeChecker{allowed: false}
	l, clock := newTestLimiter(fc, true)

	if d := l.Check(context.Background(), "1.1.1.1"); d.Verdict != Deny || d.RetryAfter != 15*time.Second {
		t.Fatalf("first decision = %+v", d)
	}
	clock.Advance(5 * time.Second)
	d := l.Check(context.Background(), "1.1.1.1")
	if d.Verdict != Deny || d.RetryAfter != 10*time.Second {
		t.Errorf("cached decision = %+v, want Deny with 10s left", d)
	}
	if fc.calls.Load() != 1 {
		t.Errorf("calls = %d, want 1 (second served from cache)", fc.calls.Load())
	}

	// Different IP is not affected by the cache.
	l.Check(context.Background(), "2.2.2.2")
	if fc.calls.Load() != 2 {
		t.Errorf("calls = %d, want 2 for a different IP", fc.calls.Load())
	}

	// After TTL the limiter is consulted again.
	clock.Advance(11 * time.Second)
	fc.allowed = true
	if d := l.Check(context.Background(), "1.1.1.1"); d.Verdict != Allow {
		t.Errorf("after TTL verdict = %v, want Allow", d.Verdict)
	}
	if fc.calls.Load() != 3 {
		t.Errorf("calls = %d, want 3", fc.calls.Load())
	}
}

func TestLimiter_FailOpen(t *testing.T) {
	fc := &fakeChecker{err: errors.New("boom")}
	l, _ := newTestLimiter(fc, true)

	if d := l.Check(context.Background(), "1.1.1.1"); d.Verdict != Allow {
		t.Errorf("verdict = %v, want Allow when failing open", d.Verdict)
	}
	if l.failOpen.Load() != 1 || l.errors.Load() != 1 {
		t.Errorf("failOpen=%d errors=%d", l.failOpen.Load(), l.errors.Load())
	}
}

func TestLimiter_FailClosed(t *testing.T) {
	fc := &fakeChecker{err: errors.New("boom")}
	l, _ := newTestLimiter(fc, false)

	if d := l.Check(context.Background(), "1.1.1.1"); d.Verdict != Unavailable {
		t.Errorf("verdict = %v, want Unavailable when failing closed", d.Verdict)
	}
}

func TestLimiter_CircuitBreaker(t *testing.T) {
	fc := &fakeChecker{err: errors.New("boom")}
	l, clock := newTestLimiter(fc, true)

	l.Check(context.Background(), "1.1.1.1") // trips the breaker
	l.Check(context.Background(), "2.2.2.2")
	l.Check(context.Background(), "3.3.3.3")
	if fc.calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1 while breaker is open", fc.calls.Load())
	}
	if l.breakerSkips.Load() != 2 {
		t.Errorf("breakerSkips = %d, want 2", l.breakerSkips.Load())
	}

	// Breaker closes after the timeout and the limiter is used again.
	clock.Advance(61 * time.Second)
	fc.err = nil
	fc.allowed = true
	if d := l.Check(context.Background(), "1.1.1.1"); d.Verdict != Allow {
		t.Errorf("verdict = %v, want Allow", d.Verdict)
	}
	if fc.calls.Load() != 2 {
		t.Errorf("calls = %d, want 2 after breaker closed", fc.calls.Load())
	}
}

func TestLimiter_CachedDenyWinsOverOpenBreaker(t *testing.T) {
	fc := &fakeChecker{allowed: false}
	l, _ := newTestLimiter(fc, true)

	l.Check(context.Background(), "1.1.1.1") // cached 429
	fc.err = errors.New("boom")
	l.Check(context.Background(), "9.9.9.9") // trips breaker

	if d := l.Check(context.Background(), "1.1.1.1"); d.Verdict != Deny {
		t.Errorf("verdict = %v, want Deny from cache while breaker is open", d.Verdict)
	}
}

func TestLimiter_FailClosedWhileBreakerOpen(t *testing.T) {
	fc := &fakeChecker{err: errors.New("boom")}
	l, _ := newTestLimiter(fc, false)

	l.Check(context.Background(), "1.1.1.1") // trips the breaker
	if d := l.Check(context.Background(), "2.2.2.2"); d.Verdict != Unavailable {
		t.Errorf("verdict = %v, want Unavailable", d.Verdict)
	}
	if fc.calls.Load() != 1 {
		t.Errorf("calls = %d, want 1", fc.calls.Load())
	}
}

func TestLimiter_CacheIsBounded(t *testing.T) {
	fc := &fakeChecker{allowed: false}
	l, _ := newTestLimiter(fc, true)

	for i := 0; i < maxCachedIPs; i++ {
		l.blocked[fmt.Sprintf("ip-%d", i)] = l.now().Add(time.Hour)
	}
	start := time.Now()
	l.addTo429Cache("new-ip")
	duration := time.Since(start)

	// during the DDoS this might be an extra time added to DDoS processing.
	// though DDoS with 100_000 unique IPs, IMO very unlikely in practice.
	t.Logf("addTo429Cache (when cache is full) duration: %v", duration)

	if len(l.blocked) > maxCachedIPs {
		t.Errorf("cache grew to %d, limit %d", len(l.blocked), maxCachedIPs)
	}
}

func TestLimiter_CacheEvictsExpiredWhenFull(t *testing.T) {
	fc := &fakeChecker{allowed: false}
	l, _ := newTestLimiter(fc, true)

	expired := l.now().Add(-time.Second)
	for i := 0; i < maxCachedIPs; i++ {
		l.blocked[fmt.Sprintf("ip-%d", i)] = expired
	}

	start := time.Now()
	l.addTo429Cache("new-ip")
	duration := time.Since(start)
	// this is the DDoS with more than 100K unique IPs
	t.Logf("addTo429Cache (when cache is full, all entries are expired and evicted) duration: %v", duration)

	if _, ok := l.blocked["new-ip"]; !ok {
		t.Error("new entry must be stored after expired entries are swept")
	}
	if len(l.blocked) != 1 {
		t.Errorf("len = %d, want 1 after sweep", len(l.blocked))
	}
}

func TestLimiter_ConcurrentChecks(t *testing.T) {
	fc := &fakeChecker{allowed: false}
	l, _ := newTestLimiter(fc, true)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				l.Check(context.Background(), "1.1.1.1")
				l.RecordWhitelisted()
			}
		}()
	}
	wg.Wait()

	if l.checks.Load() != 5000 || l.whitelisted.Load() != 5000 {
		t.Errorf("checks=%d whitelisted=%d, want 5000 each", l.checks.Load(), l.whitelisted.Load())
	}
	if l.denied.Load() != 5000 {
		t.Errorf("denied=%d, want 5000", l.denied.Load())
	}
}

func TestLimiter_GetMetrics(t *testing.T) {
	fc := &fakeChecker{allowed: false}
	l, _ := newTestLimiter(fc, true)
	l.Check(context.Background(), "1.1.1.1")
	l.Check(context.Background(), "1.1.1.1")
	l.RecordWhitelisted()

	out := l.GetMetrics()
	for _, want := range []string{
		"# HELP smotra_ratelimiter_checks_total",
		"# TYPE smotra_ratelimiter_checks_total counter",
		"smotra_ratelimiter_checks_total 2\n",
		"smotra_ratelimiter_denied_total 2\n",
		"smotra_ratelimiter_cache_hits_total 1\n",
		"smotra_ratelimiter_whitelisted_total 1\n",
		"smotra_ratelimiter_errors_total 0\n",
		"smotra_ratelimiter_breaker_skips_total 0\n",
		"smotra_ratelimiter_fail_open_total 0\n",
		"smotra_ratelimiter_allowed_total 0\n",
		"# TYPE smotra_ratelimiter_blocked_entries gauge",
		"smotra_ratelimiter_blocked_entries 1\n",
		"# TYPE smotra_ratelimiter_blocked_utilization_ratio gauge",
		"smotra_ratelimiter_blocked_utilization_ratio 1e-05\n",
		"# TYPE smotra_ratelimiter_call_duration_seconds summary",
		"smotra_ratelimiter_call_duration_seconds_count 1\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("metrics output missing %q\n%s", want, out)
		}
	}
}
