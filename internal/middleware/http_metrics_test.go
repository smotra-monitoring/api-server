package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPMetrics_CountsRequests(t *testing.T) {
	m := NewHTTPMetrics()

	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 3; i++ {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	}

	if m.total.Load() != 3 {
		t.Errorf("expected total 3, got %d", m.total.Load())
	}
	if m.success.Load() != 3 {
		t.Errorf("expected success 3, got %d", m.success.Load())
	}
	if m.failure.Load() != 0 {
		t.Errorf("expected failure 0, got %d", m.failure.Load())
	}
}

func TestHTTPMetrics_CountsFailures(t *testing.T) {
	m := NewHTTPMetrics()

	makeHandler := func(code int) http.Handler {
		return m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
		}))
	}

	makeHandler(http.StatusBadRequest).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	makeHandler(http.StatusInternalServerError).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	makeHandler(http.StatusOK).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))

	if m.total.Load() != 3 {
		t.Errorf("expected total 3, got %d", m.total.Load())
	}
	if m.success.Load() != 1 {
		t.Errorf("expected success 1, got %d", m.success.Load())
	}
	if m.failure.Load() != 2 {
		t.Errorf("expected failure 2, got %d", m.failure.Load())
	}
}

func TestHTTPMetrics_DefaultStatusIsSuccess(t *testing.T) {
	m := NewHTTPMetrics()

	// Handler that does not call WriteHeader explicitly — defaults to 200
	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok")) //nolint:errcheck
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))

	if m.success.Load() != 1 {
		t.Errorf("expected success 1 for implicit 200, got %d", m.success.Load())
	}
}

func TestHTTPMetrics_GetMetrics_Format(t *testing.T) {
	m := NewHTTPMetrics()

	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))

	handler400 := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	handler400.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))

	out := m.GetMetrics()

	for _, expected := range []string{
		"# HELP smotra_http_requests_total",
		"# TYPE smotra_http_requests_total counter",
		"smotra_http_requests_total 2",
		"smotra_http_requests_success_total 1",
		"smotra_http_requests_failure_total 1",
	} {
		if !strings.Contains(out, expected) {
			t.Errorf("expected %q in GetMetrics output:\n%s", expected, out)
		}
	}
}
