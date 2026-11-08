// Package netutil holds small IP/CIDR helpers shared across packages.
package netutil

import "net"

// ParseNets converts CIDRs and bare IPs into networks; invalid entries are skipped.
func ParseNets(entries []string) []*net.IPNet {
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

// Contains reports whether ip is inside any of nets.
func Contains(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
