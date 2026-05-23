package auth

import (
	"net"
	"testing"
)

// ─── checkIPSafe ──────────────────────────────────────────────────────────────

func TestCheckIPSafe_PublicIPv4_ReturnsNil(t *testing.T) {
	// A regular public IP address must not be blocked.
	ip := net.ParseIP("8.8.8.8")
	if err := checkIPSafe(ip); err != nil {
		t.Errorf("expected public IP 8.8.8.8 to be safe, got: %v", err)
	}
}

func TestCheckIPSafe_Loopback_ReturnsError(t *testing.T) {
	for _, addr := range []string{"127.0.0.1", "127.1.2.3"} {
		ip := net.ParseIP(addr)
		if err := checkIPSafe(ip); err == nil {
			t.Errorf("expected loopback %s to be blocked, got nil", addr)
		}
	}
}

func TestCheckIPSafe_LinkLocal_ReturnsError(t *testing.T) {
	// Includes the AWS/GCP/Azure/DigitalOcean metadata endpoint.
	for _, addr := range []string{"169.254.169.254", "169.254.0.1"} {
		ip := net.ParseIP(addr)
		if err := checkIPSafe(ip); err == nil {
			t.Errorf("expected link-local %s to be blocked, got nil", addr)
		}
	}
}

func TestCheckIPSafe_RFC1918_ReturnsError(t *testing.T) {
	privateAddrs := []string{
		"10.0.0.1",
		"10.255.255.255",
		"172.16.0.1",
		"172.31.255.255",
		"192.168.0.1",
		"192.168.255.255",
	}
	for _, addr := range privateAddrs {
		ip := net.ParseIP(addr)
		if err := checkIPSafe(ip); err == nil {
			t.Errorf("expected private address %s to be blocked, got nil", addr)
		}
	}
}

func TestCheckIPSafe_IPv6Loopback_ReturnsError(t *testing.T) {
	ip := net.ParseIP("::1")
	if err := checkIPSafe(ip); err == nil {
		t.Error("expected IPv6 loopback ::1 to be blocked, got nil")
	}
}

func TestCheckIPSafe_IPv6UniqueLocal_ReturnsError(t *testing.T) {
	ip := net.ParseIP("fd00::1")
	if err := checkIPSafe(ip); err == nil {
		t.Error("expected IPv6 unique-local fd00::1 to be blocked, got nil")
	}
}

func TestCheckIPSafe_IPv6LinkLocal_ReturnsError(t *testing.T) {
	ip := net.ParseIP("fe80::1")
	if err := checkIPSafe(ip); err == nil {
		t.Error("expected IPv6 link-local fe80::1 to be blocked, got nil")
	}
}

func TestCheckIPSafe_PublicIPv6_ReturnsNil(t *testing.T) {
	// A publicly routable IPv6 address must not be blocked.
	ip := net.ParseIP("2001:4860:4860::8888") // Google DNS
	if err := checkIPSafe(ip); err != nil {
		t.Errorf("expected public IPv6 2001:4860:4860::8888 to be safe, got: %v", err)
	}
}
