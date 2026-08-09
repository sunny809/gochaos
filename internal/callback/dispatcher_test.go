package callback

import (
	"net"
	"testing"
)

func TestIsBlockedIP(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		blocked bool
	}{
		{"loopback IPv4", "127.0.0.1", true},
		{"loopback IPv4 other", "127.0.0.100", true},
		{"RFC1918 10.x", "10.0.0.1", true},
		{"RFC1918 172.16.x", "172.16.0.1", true},
		{"RFC1918 172.31.x", "172.31.255.1", true},
		{"RFC1918 192.168.x", "192.168.1.1", true},
		{"CGNAT 100.64.x", "100.64.0.1", true},
		{"link-local 169.254.x", "169.254.1.1", true},
		{"IPv6 loopback", "::1", true},
		{"IPv6 link-local", "fe80::1", true},
		{"IPv6 ULA", "fc00::1", true},
		{"IPv6 ULA fd00", "fd00::1", true},
		{"public IPv4", "8.8.8.8", false},
		{"public IPv4 2", "1.1.1.1", false},
		{"public IPv4 3", "203.0.113.1", false},
		{"public IPv6", "2001:db8::1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("failed to parse IP %q", tt.ip)
			}
			got := IsBlockedIP(ip)
			if got != tt.blocked {
				t.Errorf("IsBlockedIP(%s) = %v, want %v", tt.ip, got, tt.blocked)
			}
		})
	}
}

func TestIsBlockedHost(t *testing.T) {
	t.Run("DNS resolution failure blocks request", func(t *testing.T) {
		// A host that won't resolve
		blocked := IsBlockedHost("this-host-definitely-does-not-exist.invalid")
		if !blocked {
			t.Error("expected unresolvable host to be blocked")
		}
	})
}
