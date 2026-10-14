package main

import (
	"errors"
	"net/netip"
	"testing"

	"github.com/fizsar/quantumready-saas/internal/scanner"
)

func TestLabPolicy(t *testing.T) {
	for _, a := range []string{"127.0.0.1", "::1", "10.1.2.3", "172.16.0.1", "172.31.213.5", "192.168.1.1", "::ffff:10.0.0.1"} {
		if err := labPolicy(netip.MustParseAddr(a)); err != nil {
			t.Errorf("labPolicy(%s) = %v, want allowed", a, err)
		}
	}
	// Public addresses too: a lab run must never reach a third party.
	for _, a := range []string{"149.56.13.98", "1.1.1.1", "2606:4700::1111", "169.254.169.254", "100.64.0.1", "172.32.0.1", "fd00::1", "0.0.0.0"} {
		if err := labPolicy(netip.MustParseAddr(a)); !errors.Is(err, scanner.ErrBlockedAddress) {
			t.Errorf("labPolicy(%s) = %v, want blocked", a, err)
		}
	}
}
