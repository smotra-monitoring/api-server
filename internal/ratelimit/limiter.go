package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// maxCachedIPs bounds memory used by the local 429 cache during a flood from many addresses.
const maxCachedIPs = 100_000

// Verdict is the outcome of a rate-limit check.
type Verdict int

const (
	// Allow means the request may proceed.
	Allow Verdict = iota
	// Deny means the client is rate limited (respond 429).
	Deny
	// Unavailable means the limiter failed and fail-open is off (respond 503).
	Unavailable
)

// Decision carries the verdict and, for Deny, how long the client should wait.
type Decision struct {
	Verdict    Verdict
	RetryAfter time.Duration
}

// Options configures a Limiter.
type Options struct {
	FailOpen       bool
	Cache429TTL    time.Duration
	BreakerTimeout time.Duration
}

// Limiter combines the limiter client with a local 429 cache and a circuit breaker.
type Limiter struct {
	checker Checker
	opts    Options
	now     func() time.Time

	mu      sync.Mutex
	blocked map[string]time.Time // ip -> expiry of the cached 429

	breakerUntil atomic.Value // time.Time until which the limiter is skipped

	checks       atomic.Uint64
	allowed      atomic.Uint64
	denied       atomic.Uint64
	cacheHits    atomic.Uint64
	errors       atomic.Uint64
	breakerSkips atomic.Uint64
	failOpen     atomic.Uint64
	whitelisted  atomic.Uint64
	callCount    atomic.Uint64
	callNanos    atomic.Uint64
}

// NewLimiter creates a Limiter around checker.
func NewLimiter(checker Checker, opts Options) *Limiter {
	return &Limiter{
		checker: checker,
		opts:    opts,
		now:     time.Now,
		blocked: make(map[string]time.Time),
	}
}

// RecordWhitelisted counts a request that bypassed the limiter via the whitelist.
func (l *Limiter) RecordWhitelisted() {
	l.whitelisted.Add(1)
}

// Check returns the decision for ip.
func (l *Limiter) Check(ctx context.Context, ip string) Decision {
	l.checks.Add(1)

	if retry, ok := l.isCachedDenial(ip); ok {
		l.cacheHits.Add(1)
		l.denied.Add(1)
		return Decision{Verdict: Deny, RetryAfter: retry}
	}

	if until, ok := l.breakerUntil.Load().(time.Time); ok && l.now().Before(until) {
		l.breakerSkips.Add(1)
		return l.unavailable()
	}

	start := time.Now()
	allowed, err := l.checker.Allow(ctx, ip)
	l.callCount.Add(1)
	l.callNanos.Add(uint64(time.Since(start)))

	if err != nil {
		l.errors.Add(1)
		l.breakerUntil.Store(l.now().Add(l.opts.BreakerTimeout))
		return l.unavailable()
	}

	if allowed {
		l.allowed.Add(1)
		return Decision{Verdict: Allow}
	}

	l.denied.Add(1)
	l.addTo429Cache(ip)
	return Decision{Verdict: Deny, RetryAfter: l.opts.Cache429TTL}
}

func (l *Limiter) unavailable() Decision {
	if l.opts.FailOpen {
		l.failOpen.Add(1)
		return Decision{Verdict: Allow}
	}
	return Decision{Verdict: Unavailable}
}

func (l *Limiter) isCachedDenial(ip string) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	expiry, ok := l.blocked[ip]
	if !ok {
		return 0, false
	}
	remaining := expiry.Sub(l.now())
	if remaining <= 0 {
		delete(l.blocked, ip)
		return 0, false
	}
	return remaining, true
}

func (l *Limiter) addTo429Cache(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.blocked) >= maxCachedIPs {
		now := l.now()
		for k, exp := range l.blocked {
			if !exp.After(now) {
				delete(l.blocked, k)
			}
		}
		if len(l.blocked) >= maxCachedIPs {
			return // still full of live entries; the limiter call decides for this IP next time
		}
	}
	l.blocked[ip] = l.now().Add(l.opts.Cache429TTL)
}

// GetMetrics returns Prometheus-formatted rate limiter metrics.
func (l *Limiter) GetMetrics() string {
	var out string
	counter := func(name, help string, v uint64) {
		out += fmt.Sprintf("# HELP %s %s\n# TYPE %s counter\n%s %d\n\n", name, help, name, name, v)
	}

	counter("smotra_ratelimiter_checks_total", "Requests evaluated by the rate limiter", l.checks.Load())
	counter("smotra_ratelimiter_allowed_total", "Requests allowed by the limiter service", l.allowed.Load())
	counter("smotra_ratelimiter_denied_total", "Requests denied with 429 (limiter service or local cache)", l.denied.Load())
	counter("smotra_ratelimiter_cache_hits_total", "Denials served from the local 429 cache", l.cacheHits.Load())
	counter("smotra_ratelimiter_errors_total", "Failed or timed out limiter calls", l.errors.Load())
	counter("smotra_ratelimiter_breaker_skips_total", "Checks that skipped the limiter while the circuit breaker was open", l.breakerSkips.Load())
	counter("smotra_ratelimiter_fail_open_total", "Requests allowed because the limiter was unavailable", l.failOpen.Load())
	counter("smotra_ratelimiter_whitelisted_total", "Requests that bypassed the limiter via whitelist_subnets", l.whitelisted.Load())

	l.mu.Lock()
	blockedLen := len(l.blocked)
	l.mu.Unlock()

	out += "# HELP smotra_ratelimiter_blocked_entries Current number of IPs in the local 429 cache\n"
	out += "# TYPE smotra_ratelimiter_blocked_entries gauge\n"
	out += fmt.Sprintf("smotra_ratelimiter_blocked_entries %d\n\n", blockedLen)
	out += "# HELP smotra_ratelimiter_blocked_utilization_ratio Fill ratio of the local 429 cache (entries / capacity)\n"
	out += "# TYPE smotra_ratelimiter_blocked_utilization_ratio gauge\n"
	out += fmt.Sprintf("smotra_ratelimiter_blocked_utilization_ratio %g\n\n", float64(blockedLen)/float64(maxCachedIPs))

	out += "# HELP smotra_ratelimiter_call_duration_seconds Duration of limiter service calls\n"
	out += "# TYPE smotra_ratelimiter_call_duration_seconds summary\n"
	out += fmt.Sprintf("smotra_ratelimiter_call_duration_seconds_sum %g\n", float64(l.callNanos.Load())/1e9)
	out += fmt.Sprintf("smotra_ratelimiter_call_duration_seconds_count %d\n\n", l.callCount.Load())

	return out
}
