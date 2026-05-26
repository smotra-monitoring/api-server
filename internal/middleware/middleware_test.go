package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/smotra-monitoring/server/internal/logger"
)

func TestLogger_Middleware(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(logger.Config{
		Level:  "info",
		Format: "json",
		Output: &buf,
	})

	handler := Logger(log)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	output := buf.String()
	if output == "" {
		t.Error("Expected log output, got empty string")
	}

	// Check for expected log fields
	if !bytes.Contains(buf.Bytes(), []byte("http request")) {
		t.Error("Expected log to contain 'http request'")
	}
	if !bytes.Contains(buf.Bytes(), []byte("GET")) {
		t.Error("Expected log to contain method 'GET'")
	}
	if !bytes.Contains(buf.Bytes(), []byte("/test")) {
		t.Error("Expected log to contain path '/test'")
	}
}

func TestLogger_LogsStatusCode(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(logger.Config{
		Level:  "info",
		Format: "json",
		Output: &buf,
	})

	handler := Logger(log)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}))

	req := httptest.NewRequest("GET", "/missing", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", rec.Code)
	}

	if !bytes.Contains(buf.Bytes(), []byte("404")) {
		t.Error("Expected log to contain status code 404")
	}
}

func TestLogger_LogsDuration(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(logger.Config{
		Level:  "info",
		Format: "json",
		Output: &buf,
	})

	handler := Logger(log)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !bytes.Contains(buf.Bytes(), []byte("duration_ms")) {
		t.Error("Expected log to contain 'duration_ms'")
	}
}

func TestRecovery_Middleware(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(logger.Config{
		Level:  "error",
		Format: "json",
		Output: &buf,
	})

	handler := Recovery(log)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	// Should not panic
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", rec.Code)
	}

	if !bytes.Contains(buf.Bytes(), []byte("panic recovered")) {
		t.Error("Expected log to contain 'panic recovered'")
	}
	if !bytes.Contains(buf.Bytes(), []byte("test panic")) {
		t.Error("Expected log to contain 'test panic'")
	}
}

func TestRecovery_NoPanic(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(logger.Config{
		Level:  "error",
		Format: "json",
		Output: &buf,
	})

	handler := Recovery(log)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	// Should not log anything
	if buf.String() != "" {
		t.Errorf("Expected no log output for successful request, got: %s", buf.String())
	}
}

func TestRequestID_GeneratesID(t *testing.T) {
	log := logger.New(logger.Config{
		Level:  "error",
		Format: "json",
		Output: nil,
	})
	handler := RequestID(log)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := w.Header().Get("X-Request-ID")
		if requestID == "" {
			t.Error("Expected X-Request-ID header to be set")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	requestID := rec.Header().Get("X-Request-ID")
	if requestID == "" {
		t.Error("Expected X-Request-ID header in response")
	}
}

func TestRequestID_PreservesExistingID(t *testing.T) {
	existingID := "existing-request-id"

	log := logger.New(logger.Config{
		Level:  "error",
		Format: "json",
		Output: nil,
	})
	handler := RequestID(log)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := w.Header().Get("X-Request-ID")
		if requestID != existingID {
			t.Errorf("Expected X-Request-ID %s, got %s", existingID, requestID)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", existingID)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	requestID := rec.Header().Get("X-Request-ID")
	if requestID != existingID {
		t.Errorf("Expected X-Request-ID %s, got %s", existingID, requestID)
	}
}

func TestCORS_SetsHeaders(t *testing.T) {
	handler := CORS("http://localhost:3000")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Check CORS headers
	allowOrigin := rec.Header().Get("Access-Control-Allow-Origin")
	if allowOrigin == "" {
		t.Error("Expected Access-Control-Allow-Origin header to be set")
	}

	allowMethods := rec.Header().Get("Access-Control-Allow-Methods")
	if allowMethods == "" {
		t.Error("Expected Access-Control-Allow-Methods header to be set")
	}

	allowHeaders := rec.Header().Get("Access-Control-Allow-Headers")
	if allowHeaders == "" {
		t.Error("Expected Access-Control-Allow-Headers header to be set")
	}

	maxAge := rec.Header().Get("Access-Control-Max-Age")
	if maxAge != "86400" {
		t.Errorf("Expected Access-Control-Max-Age 86400, got %s", maxAge)
	}
}

func TestCORS_OptionsRequest(t *testing.T) {
	handler := CORS("http://localhost:3000")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called for OPTIONS request")
	}))

	req := httptest.NewRequest("OPTIONS", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", rec.Code)
	}

	// CORS headers should still be set
	allowOrigin := rec.Header().Get("Access-Control-Allow-Origin")
	if allowOrigin == "" {
		t.Error("Expected Access-Control-Allow-Origin header for OPTIONS")
	}
}

func TestCORS_OptionsRequest_EmptyOrigin(t *testing.T) {
	handler := CORS("")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called for OPTIONS request")
	}))

	req := httptest.NewRequest("OPTIONS", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", rec.Code)
	}

	// CORS headers should not be set
	allowOrigin := rec.Header().Get("Access-Control-Allow-Origin")
	if allowOrigin != "" {
		t.Error("Expected no Access-Control-Allow-Origin header for OPTIONS with empty origin")
	}
}

func TestCORS_SetsConfiguredOrigin(t *testing.T) {
	const wantOrigin = "https://dashboard.smotra.net"
	handler := CORS(wantOrigin)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	got := rec.Header().Get("Access-Control-Allow-Origin")
	if got != wantOrigin {
		t.Errorf("Expected Access-Control-Allow-Origin %q, got %q", wantOrigin, got)
	}
}

func TestCORS_EmptyOrigin_NoHeaders(t *testing.T) {
	handler := CORS("")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Expected no Access-Control-Allow-Origin header, got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "" {
		t.Errorf("Expected no Access-Control-Allow-Methods header, got %q", got)
	}
}

func TestResponseWriter_CapturesStatusCode(t *testing.T) {
	rw := &responseWriter{
		ResponseWriter: httptest.NewRecorder(),
		statusCode:     http.StatusOK,
	}

	rw.WriteHeader(http.StatusCreated)

	if rw.statusCode != http.StatusCreated {
		t.Errorf("Expected status code 201, got %d", rw.statusCode)
	}
}

func TestResponseWriter_CapturesWrittenBytes(t *testing.T) {
	rw := &responseWriter{
		ResponseWriter: httptest.NewRecorder(),
		statusCode:     http.StatusOK,
	}

	data := []byte("test data")
	n, err := rw.Write(data)

	if err != nil {
		t.Errorf("Write failed: %v", err)
	}

	if n != len(data) {
		t.Errorf("Expected %d bytes written, got %d", len(data), n)
	}

	if rw.written != len(data) {
		t.Errorf("Expected %d bytes tracked, got %d", len(data), rw.written)
	}
}

func TestMiddleware_ChainedExecution(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(logger.Config{
		Level:  "info",
		Format: "json",
		Output: &buf,
	})

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	// Chain all middleware
	handler := RequestID(log)(Recovery(log)(Logger(log)(CORS("http://localhost:3000")(finalHandler))))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Check final response
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	// Check X-Request-ID was added
	requestID := rec.Header().Get("X-Request-ID")
	if requestID == "" {
		t.Error("Expected X-Request-ID header")
	}

	// Check CORS headers
	if rec.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Error("Expected CORS headers")
	}

	// Check logging occurred
	if !bytes.Contains(buf.Bytes(), []byte("http request")) {
		t.Error("Expected log output")
	}
}

func TestMiddleware_ChainedWithPanic(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(logger.Config{
		Level:  "error",
		Format: "json",
		Output: &buf,
	})

	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("intentional panic")
	})

	// Chain all middleware
	handler := RequestID(log)(Recovery(log)(Logger(log)(CORS("http://localhost:3000")(panicHandler))))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	// Should not panic
	handler.ServeHTTP(rec, req)

	// Check recovery worked
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500 after panic, got %d", rec.Code)
	}

	// Check panic was logged
	if !bytes.Contains(buf.Bytes(), []byte("panic recovered")) {
		t.Error("Expected panic to be logged")
	}
}
