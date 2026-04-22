package auth

import (
	"fmt"
	"net"
	"net/url"
)

// validateProviderURL checks an outbound URL for SSRF risk.
// It rejects loopback, link-local, and RFC1918 private addresses as well as
// the well-known cloud metadata service IP (169.254.169.254).
//
// This is applied to every URL the server fetches on behalf of a request
// (OIDC discovery, token, userinfo, revocation endpoints).
func validateProviderURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("URL scheme must be http or https, got %q", u.Scheme)
	}

	hostname := u.Hostname()
	if hostname == "" {
		return fmt.Errorf("URL has no hostname")
	}

	// Reject raw IPs that are private / link-local / loopback.
	ips, err := net.LookupHost(hostname)
	if err != nil {
		// If resolution fails we cannot verify safety — reject.
		return fmt.Errorf("cannot resolve hostname %q: %w", hostname, err)
	}
	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		if err := checkIPSafe(ip); err != nil {
			return fmt.Errorf("hostname %q resolves to unsafe address %s: %w", hostname, ipStr, err)
		}
	}
	return nil
}

// checkIPSafe rejects IP addresses that must never be contacted via a
// server-side request (SSRF protection).
func checkIPSafe(ip net.IP) error {
	// Cloud metadata endpoints (AWS, GCP, Azure, DigitalOcean, etc.)
	blockedCIDRs := []string{
		"169.254.0.0/16", // link-local (includes 169.254.169.254 metadata)
		"127.0.0.0/8",    // loopback
		"10.0.0.0/8",     // RFC1918
		"172.16.0.0/12",  // RFC1918
		"192.168.0.0/16", // RFC1918
		"::1/128",        // IPv6 loopback
		"fc00::/7",       // IPv6 unique-local (fd00::/8 is a sub-range)
		"fe80::/10",      // IPv6 link-local
	}

	for _, cidr := range blockedCIDRs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return fmt.Errorf("address is in blocked range %s", cidr)
		}
	}
	return nil
}
