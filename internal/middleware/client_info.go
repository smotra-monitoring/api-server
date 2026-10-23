package middleware

import (
	"context"
	"net"
	"net/http"
	"strings"
)

const (
	// ClientInfoContextKey is the context key for ClientInfo.
	ClientInfoContextKey ContextKey = "client_info"
	maxUserAgentLen      int        = 512
)

// ClientInfo describes the remote client of a request.
type ClientInfo struct {
	IPAddress string
	UserAgent string
}

// ClientInfoFromContext returns the client info stored by the ClientInfo middleware.
func ClientInfoFromContext(ctx context.Context) ClientInfo {
	info, _ := ctx.Value(ClientInfoContextKey).(ClientInfo)
	return info
}

// ClientInfoMiddleware records the client IP and User-Agent in the request context.
// X-Forwarded-For is honoured only when the direct peer is within trustedProxies
// (CIDRs or bare IPs); invalid entries are ignored.
func ClientInfoMiddleware(trustedProxies []string) func(next http.Handler) http.Handler {
	nets := parseTrustedProxies(trustedProxies)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			info := ClientInfo{
				IPAddress: resolveClientIP(r, nets),
				UserAgent: truncate(r.UserAgent(), maxUserAgentLen),
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ClientInfoContextKey, info)))
		})
	}
}

func parseTrustedProxies(entries []string) []*net.IPNet {
	var nets []*net.IPNet
	for _, e := range entries {
		if _, n, err := net.ParseCIDR(e); err == nil {
			nets = append(nets, n)
			continue
		}
		if ip := net.ParseIP(e); ip != nil {
			bits := 8 * len(ip.To16())
			if v4 := ip.To4(); v4 != nil {
				ip, bits = v4, 32
			}
			nets = append(nets, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
		}
	}
	return nets
}

func isTrusted(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func resolveClientIP(r *http.Request, trusted []*net.IPNet) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	remoteAddress := net.ParseIP(host)
	if remoteAddress == nil {
		return ""
	}
	if len(trusted) == 0 || !isTrusted(remoteAddress, trusted) {
		return remoteAddress.String()
	}

	// Walk X-Forwarded-For right to left; the first hop that is not a trusted proxy is the client.
	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		ip := net.ParseIP(strings.TrimSpace(hops[i]))
		if ip == nil {
			continue
		}
		if !isTrusted(ip, trusted) {
			return ip.String()
		}
	}
	return remoteAddress.String()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return strings.ToValidUTF8(s[:max], "")
}
