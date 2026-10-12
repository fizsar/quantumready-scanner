package scanner

import (
	"context"
	"errors"
	"net/netip"
	"testing"
)

func TestCheckAddr(t *testing.T) {
	allowed := []string{
		"8.8.8.8",
		"149.56.13.98", // the VPS itself: public, see docs/scanner.md
		"172.15.255.255",
		"172.32.0.0",
		"2606:4700:4700::1111",
	}
	blocked := []string{
		"127.0.0.1", "127.255.255.254", // loopback
		"10.0.0.1", "192.168.1.1", // private
		"172.16.0.1", "172.17.0.1", "172.18.0.2", "172.31.255.255", // private, Docker bridge networks
		"169.254.169.254",    // cloud metadata
		"100.64.0.1",         // CGNAT
		"0.0.0.0", "0.1.2.3", // this network
		"224.0.0.1", "239.255.255.250", // multicast
		"255.255.255.255", "240.0.0.1", // broadcast, reserved
		"198.18.0.1", "192.0.2.1", "203.0.113.5", // benchmarking, documentation
		"::", "::1", // unspecified, loopback
		"fe80::1", "fc00::1", "fd12:3456::1", "ff02::1", // link-local, ULA, multicast
		"::ffff:127.0.0.1", "::ffff:10.0.0.1", "::ffff:172.17.0.1", // IPv4-mapped
		"::127.0.0.1",                       // IPv4-compatible
		"64:ff9b::a00:1",                    // NAT64 → 10.0.0.1
		"2002:a00:1::1",                     // 6to4 → 10.0.0.1
		"2001:db8::1", "2001::1", "3fff::1", // documentation, Teredo
	}
	for _, s := range allowed {
		if err := CheckAddr(netip.MustParseAddr(s)); err != nil {
			t.Errorf("CheckAddr(%s) = %v, want allowed", s, err)
		}
	}
	for _, s := range blocked {
		if err := CheckAddr(netip.MustParseAddr(s)); !errors.Is(err, ErrBlockedAddress) {
			t.Errorf("CheckAddr(%s) = %v, want ErrBlockedAddress", s, err)
		}
	}
	if err := CheckAddr(netip.Addr{}); !errors.Is(err, ErrBlockedAddress) {
		t.Errorf("CheckAddr(invalid) = %v", err)
	}
}

func TestScanRefusesNonPublicTargets(t *testing.T) {
	tests := []struct {
		name string
		host string
		res  *fakeResolver
	}{
		{"loopback literal", "127.0.0.1", nil},
		{"docker gateway literal", "172.17.0.1", nil},
		{"metadata literal", "169.254.169.254", nil},
		{"name to private", "internal.test", resolverFor("internal.test", "10.0.0.5")},
		{"name to docker network", "caddy.test", resolverFor("caddy.test", "172.18.0.2")},
		{"name to loopback v6", "v6.test", resolverFor("v6.test", "::1")},
		{"mixed public and private", "mixed.test", resolverFor("mixed.test", "8.8.8.8", "10.0.0.5")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := Options{AddrPolicy: CheckAddr} // the real policy
			if tt.res != nil {
				opts.Resolver = tt.res
			}
			s := New(opts)
			_, err := s.Scan(context.Background(), tt.host, 443)
			if !errors.Is(err, ErrBlockedAddress) {
				t.Fatalf("Scan(%s) error = %v, want ErrBlockedAddress", tt.host, err)
			}
			if s.maxActive.Load() != 0 {
				t.Errorf("a connection was attempted")
			}
		})
	}
}

// The dialer re-checks the policy on the socket, even if resolution was bypassed.
func TestDialRechecksPolicy(t *testing.T) {
	s := New(Options{})
	_, err := s.dial(context.Background(), netip.MustParseAddrPort("127.0.0.1:443"))
	if !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("dial(127.0.0.1) error = %v, want ErrBlockedAddress", err)
	}
}

func TestNormalizeHost(t *testing.T) {
	good := map[string]string{
		"Example.COM.":              "example.com",
		" quantumready.fizsar.com ": "quantumready.fizsar.com",
		"xn--bcher-kva.de":          "xn--bcher-kva.de",
		"[2606:4700::1111]":         "2606:4700::1111",
	}
	for in, want := range good {
		got, _, err := normalizeHost(in)
		if err != nil || got != want {
			t.Errorf("normalizeHost(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "a..b", "-a.com", "a-.com", "ex ample.com", "exa_mple.com", "bücher.de", "http://x.com"} {
		if _, _, err := normalizeHost(in); !errors.Is(err, ErrInvalidTarget) {
			t.Errorf("normalizeHost(%q) error = %v, want ErrInvalidTarget", in, err)
		}
	}
}
