package netutil

import (
	"net"
	"testing"
)

func TestParseNets(t *testing.T) {
	nets := ParseNets([]string{"10.0.0.0/8", "192.0.2.9", "fd00::/8", "2001:db8::1", "not-an-ip", "10.0.0.0/33"})
	if len(nets) != 4 {
		t.Fatalf("len = %d, want 4 (invalid entries skipped)", len(nets))
	}
}

func TestContains(t *testing.T) {
	nets := ParseNets([]string{"10.0.0.0/8", "192.0.2.9", "fd00::/8", "2001:db8::1"})

	for _, tc := range []struct {
		ip   string
		want bool
	}{
		{"10.1.2.3", true},
		{"11.0.0.1", false},
		{"192.0.2.9", true},
		{"192.0.2.10", false},
		{"fd00::5", true},
		{"fe80::1", false},
		{"2001:db8::1", true},
		{"2001:db8::2", false},
	} {
		if got := Contains(net.ParseIP(tc.ip), nets); got != tc.want {
			t.Errorf("Contains(%s) = %v, want %v", tc.ip, got, tc.want)
		}
	}
}

func TestContains_EmptyNets(t *testing.T) {
	if Contains(net.ParseIP("10.0.0.1"), nil) {
		t.Error("Contains with no networks must be false")
	}
}
