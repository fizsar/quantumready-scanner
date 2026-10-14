package main

import (
	"fmt"
	"net/netip"

	"github.com/fizsar/quantumready-saas/internal/scanner"
)

// labMarker identifies this code: cmd/worker's tests check that it never
// ends up in a production binary. Keep it in sync with cmd/worker.
const labMarker = "qrscan-lab-addr-policy"

var labPrefixes = []netip.Prefix{
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("172.16.0.0/12"), // includes the TLS lab network (test/tlslab)
	netip.MustParsePrefix("192.168.0.0/16"),
}

// labPolicy is the address policy of -lab: ONLY loopback and RFC 1918
// addresses, so that a lab run can never reach a third party. It replaces the
// anti-SSRF policy (which refuses exactly these) for the TLS test lab.
func labPolicy(a netip.Addr) error {
	a = a.Unmap()
	if a == netip.IPv6Loopback() {
		return nil
	}
	for _, p := range labPrefixes {
		if p.Contains(a) {
			return nil
		}
	}
	return fmt.Errorf("%w: %s: %s is not loopback or RFC 1918", scanner.ErrBlockedAddress, labMarker, a)
}
