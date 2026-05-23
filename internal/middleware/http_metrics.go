package middleware

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

// HTTPMetrics tracks HTTP request counters and exposes them as a MetricsProvider.
type HTTPMetrics struct {
	total   atomic.Uint64
	success atomic.Uint64 // 1xx, 2xx, 3xx
	failure atomic.Uint64 // 4xx, 5xx
}

// NewHTTPMetrics creates a new HTTPMetrics instance.
func NewHTTPMetrics() *HTTPMetrics {
	return &HTTPMetrics{}
}

// Middleware returns a chi-compatible middleware that counts HTTP requests.
func (m *HTTPMetrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(wrapped, r)

		m.total.Add(1)
		if wrapped.statusCode >= 400 {
			m.failure.Add(1)
		} else {
			m.success.Add(1)
		}
	})
}

// GetMetrics returns Prometheus-formatted metrics for HTTP requests.
func (m *HTTPMetrics) GetMetrics() string {
	var out string

	out += "# HELP smotra_http_requests_total Total number of HTTP requests\n"
	out += "# TYPE smotra_http_requests_total counter\n"
	out += fmt.Sprintf("smotra_http_requests_total %d\n", m.total.Load())
	out += "\n"

	out += "# HELP smotra_http_requests_success_total Total number of successful HTTP requests (1xx-3xx)\n"
	out += "# TYPE smotra_http_requests_success_total counter\n"
	out += fmt.Sprintf("smotra_http_requests_success_total %d\n", m.success.Load())
	out += "\n"

	out += "# HELP smotra_http_requests_failure_total Total number of failed HTTP requests (4xx-5xx)\n"
	out += "# TYPE smotra_http_requests_failure_total counter\n"
	out += fmt.Sprintf("smotra_http_requests_failure_total %d\n", m.failure.Load())
	out += "\n"

	return out
}
