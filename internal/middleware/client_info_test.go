package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func clientInfoFor(t *testing.T, trusted []string, remoteAddr string, headers map[string]string) ClientInfo {
	t.Helper()
	var got ClientInfo
	h := ClientInfoMiddleware(trusted)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = ClientInfoFromContext(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remoteAddr
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	h.ServeHTTP(httptest.NewRecorder(), req)
	return got
}

func TestClientInfo_IP(t *testing.T) {
	tests := []struct {
		name    string
		trusted []string
		remote  string
		xff     string
		want    string
	}{
		{"no trusted proxies uses RemoteAddr", nil, "203.0.113.5:1234", "", "203.0.113.5"},
		{"spoofed XFF ignored without trusted proxies", nil, "203.0.113.5:1234", "1.2.3.4", "203.0.113.5"},
		{"spoofed XFF ignored from untrusted peer", []string{"10.0.0.0/8"}, "203.0.113.5:1234", "1.2.3.4", "203.0.113.5"},
		{"XFF honoured from trusted peer", []string{"10.0.0.0/8"}, "10.0.0.1:80", "198.51.100.9", "198.51.100.9"},
		{"rightmost untrusted hop wins", []string{"10.0.0.0/8"}, "10.0.0.1:80", "6.6.6.6, 198.51.100.9, 10.0.0.2", "198.51.100.9"},
		{"bare IP entry", []string{"10.0.0.1"}, "10.0.0.1:80", "198.51.100.9", "198.51.100.9"},
		{"trusted peer without XFF falls back to peer", []string{"10.0.0.0/8"}, "10.0.0.1:80", "", "10.0.0.1"},
		{"garbage XFF falls back to peer", []string{"10.0.0.0/8"}, "10.0.0.1:80", "not-an-ip", "10.0.0.1"},
		{"IPv6 peer", nil, "[2001:db8::1]:443", "", "2001:db8::1"},
		{"invalid entries ignored", []string{"bogus"}, "203.0.113.5:1", "1.2.3.4", "203.0.113.5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := map[string]string{}
			if tt.xff != "" {
				headers["X-Forwarded-For"] = tt.xff
			}
			if got := clientInfoFor(t, tt.trusted, tt.remote, headers).IPAddress; got != tt.want {
				t.Errorf("IP = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClientInfo_UserAgent(t *testing.T) {
	got := clientInfoFor(t, nil, "203.0.113.5:1", map[string]string{"User-Agent": "Mozilla/5.0"})
	if got.UserAgent != "Mozilla/5.0" {
		t.Errorf("UserAgent = %q", got.UserAgent)
	}

	long := strings.Repeat("a", maxUserAgentLen+100)
	got = clientInfoFor(t, nil, "203.0.113.5:1", map[string]string{"User-Agent": long})
	if len(got.UserAgent) != maxUserAgentLen {
		t.Errorf("UserAgent length = %d, want %d", len(got.UserAgent), maxUserAgentLen)
	}
}

func TestClientInfoFromContext_Missing(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := ClientInfoFromContext(req.Context()); got != (ClientInfo{}) {
		t.Errorf("expected zero value, got %+v", got)
	}
}
