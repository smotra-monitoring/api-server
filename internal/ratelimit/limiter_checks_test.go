package ratelimit

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestLimiter_isExemptPath(t *testing.T) {
	l, _ := newTestLimiterWithOptions(&fakeChecker{}, Options{ExemptPaths: []string{"/healthz", "/metrics"}})

	for path, want := range map[string]bool{
		"/healthz":       true,
		"/healthz/ready": true,
		"/metrics":       true,
		"/v1/agents":     false,
		"/":              false,
		"/x/healthz":     false,
	} {
		if got := l.isExemptPath(path); got != want {
			t.Errorf("isExemptPath(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestLimiter_isExemptPath_NoPrefixes(t *testing.T) {
	l, _ := newTestLimiter(&fakeChecker{}, true)
	if l.isExemptPath("/healthz") {
		t.Error("no exempt prefixes configured, nothing must be exempt")
	}
}

func TestLimiter_isWhitelisted(t *testing.T) {
	l, _ := newTestLimiterWithOptions(&fakeChecker{}, Options{WhitelistSubnets: []string{"10.0.0.0/8", "192.0.2.9", "fd00::/8"}})

	for ip, want := range map[string]bool{
		"10.1.2.3":   true,
		"192.0.2.9":  true,
		"fd00::1":    true,
		"11.0.0.1":   false,
		"192.0.2.10": false,
	} {
		if got := l.isWhitelisted(net.ParseIP(ip)); got != want {
			t.Errorf("isWhitelisted(%s) = %v, want %v", ip, got, want)
		}
	}
}

func TestLimiter_isCachedDenial(t *testing.T) {
	l, clock := newTestLimiter(&fakeChecker{}, true)

	if _, ok := l.isCachedDenial("1.1.1.1"); ok {
		t.Fatal("empty cache must not report a denial")
	}

	l.addTo429Cache("1.1.1.1")
	if retry, ok := l.isCachedDenial("1.1.1.1"); !ok || retry != 15*time.Second {
		t.Errorf("isCachedDenial = (%v, %v), want (15s, true)", retry, ok)
	}

	clock.Advance(16 * time.Second)
	if _, ok := l.isCachedDenial("1.1.1.1"); ok {
		t.Error("expired entry must not report a denial")
	}
	if len(l.blocked) != 0 {
		t.Errorf("expired entry must be removed, len = %d", len(l.blocked))
	}
}

func TestLimiter_isBreakerOpen(t *testing.T) {
	l, clock := newTestLimiter(&fakeChecker{err: errors.New("boom")}, true)

	if l.isBreakerOpen() {
		t.Fatal("breaker must start closed")
	}
	l.Check(context.Background(), "1.1.1.1", testPath)
	if !l.isBreakerOpen() {
		t.Error("breaker must open after a failed call")
	}
	clock.Advance(61 * time.Second)
	if l.isBreakerOpen() {
		t.Error("breaker must close after BreakerTimeout")
	}
}

func TestLimiter_Check_ExemptPathSkipsEverything(t *testing.T) {
	fc := &fakeChecker{allowed: false}
	l, _ := newTestLimiterWithOptions(fc, Options{ExemptPaths: []string{"/healthz"}})

	if d := l.Check(context.Background(), "203.0.113.5", "/healthz"); d.Verdict != Allow {
		t.Errorf("verdict = %v, want Allow", d.Verdict)
	}
	if fc.calls.Load() != 0 || l.checks.Load() != 0 {
		t.Errorf("calls=%d checks=%d, want 0 (exempt requests are not counted)", fc.calls.Load(), l.checks.Load())
	}
}

func TestLimiter_Check_WhitelistBypassesLimiter(t *testing.T) {
	fc := &fakeChecker{allowed: false}
	l, _ := newTestLimiterWithOptions(fc, Options{WhitelistSubnets: []string{"10.0.0.0/8"}})

	if d := l.Check(context.Background(), "10.1.2.3", testPath); d.Verdict != Allow {
		t.Errorf("verdict = %v, want Allow", d.Verdict)
	}
	if fc.calls.Load() != 0 || l.checks.Load() != 0 || l.whitelisted.Load() != 1 {
		t.Errorf("calls=%d checks=%d whitelisted=%d, want 0/0/1", fc.calls.Load(), l.checks.Load(), l.whitelisted.Load())
	}
	if d := l.Check(context.Background(), "11.0.0.1", testPath); d.Verdict != Deny {
		t.Errorf("non-whitelisted verdict = %v, want Deny", d.Verdict)
	}
}

func TestLimiter_Check_WhitelistWorksWhileBreakerOpenAndFailClosed(t *testing.T) {
	fc := &fakeChecker{err: errors.New("boom")}
	l, _ := newTestLimiterWithOptions(fc, Options{WhitelistSubnets: []string{"10.0.0.0/8"}})

	l.Check(context.Background(), "203.0.113.5", testPath) // trips the breaker
	if d := l.Check(context.Background(), "10.0.0.1", testPath); d.Verdict != Allow {
		t.Errorf("whitelisted verdict = %v, want Allow", d.Verdict)
	}
}

func TestLimiter_Check_UnknownClientIPIsAllowedWithoutCall(t *testing.T) {
	for _, ip := range []string{"", "not-an-ip"} {
		fc := &fakeChecker{allowed: false}
		l, _ := newTestLimiter(fc, true)

		if d := l.Check(context.Background(), ip, testPath); d.Verdict != Allow {
			t.Errorf("ip %q: verdict = %v, want Allow (unknown client IPs are allowed)", ip, d.Verdict)
		}
		if fc.calls.Load() != 0 || l.checks.Load() != 0 {
			t.Errorf("ip %q: calls=%d checks=%d, want 0", ip, fc.calls.Load(), l.checks.Load())
		}
	}
}

func TestLimiter_Check_CanonicalizesIPForCache(t *testing.T) {
	fc := &fakeChecker{allowed: false}
	l, _ := newTestLimiter(fc, true)

	l.Check(context.Background(), "2001:db8:0:0:0:0:0:1", testPath)
	l.Check(context.Background(), "2001:db8::1", testPath)

	if fc.calls.Load() != 1 {
		t.Errorf("calls = %d, want 1 (same address in different notation hits the cache)", fc.calls.Load())
	}
}
