package scanner

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"syscall"
	"time"
)

// ErrBlockedAddress is returned when a target is, or resolves to, an address
// that is not public unicast (anti-SSRF).
var ErrBlockedAddress = errors.New("scanner: address not allowed")

// ErrInvalidTarget is returned for malformed host names or ports.
var ErrInvalidTarget = errors.New("scanner: invalid target")

// ErrResolve is returned when the host name does not resolve.
var ErrResolve = errors.New("scanner: name does not resolve")

// globalUnicast6 is the only IPv6 space we connect to (RFC 4291 2000::/3).
var globalUnicast6 = netip.MustParsePrefix("2000::/3")

// blockedPrefixes are special-purpose ranges (IANA registries) that must never
// be scanned: they reach the host, the Docker networks, the cloud metadata
// service or other internal services instead of the public Internet.
var blockedPrefixes = mustPrefixes(
	// IPv4
	"0.0.0.0/8",          // "this network"
	"10.0.0.0/8",         // private
	"100.64.0.0/10",      // carrier-grade NAT
	"127.0.0.0/8",        // loopback
	"169.254.0.0/16",     // link-local, incl. cloud metadata (169.254.169.254)
	"172.16.0.0/12",      // private, incl. Docker bridge networks (172.17.0.0/16, 172.18.0.0/16…)
	"192.0.0.0/24",       // IETF protocol assignments
	"192.0.2.0/24",       // documentation (TEST-NET-1)
	"192.88.99.0/24",     // 6to4 relay anycast
	"192.168.0.0/16",     // private
	"198.18.0.0/15",      // benchmarking
	"198.51.100.0/24",    // documentation (TEST-NET-2)
	"203.0.113.0/24",     // documentation (TEST-NET-3)
	"224.0.0.0/4",        // multicast
	"240.0.0.0/4",        // reserved, incl. broadcast
	"255.255.255.255/32", // broadcast
	// IPv6 (on top of requiring 2000::/3)
	"64:ff9b::/96",   // NAT64: would reach an embedded IPv4 address
	"64:ff9b:1::/48", // local-use NAT64
	"2001::/23",      // IETF protocol assignments (Teredo, ORCHID…)
	"2001:db8::/32",  // documentation
	"2002::/16",      // 6to4: embeds an IPv4 address
	"3fff::/20",      // documentation (RFC 9637)
)

func mustPrefixes(ss ...string) []netip.Prefix {
	ps := make([]netip.Prefix, len(ss))
	for i, s := range ss {
		ps[i] = netip.MustParsePrefix(s)
	}
	return ps
}

// CheckAddr returns nil only for public unicast addresses. It is the default
// AddrPolicy, applied after resolving and again right before connecting.
func CheckAddr(a netip.Addr) error {
	if !a.IsValid() {
		return fmt.Errorf("%w: invalid address", ErrBlockedAddress)
	}
	a = a.Unmap() // ::ffff:10.0.0.1 is 10.0.0.1
	if a.Zone() != "" {
		return fmt.Errorf("%w: %s has a zone", ErrBlockedAddress, a)
	}
	if a.Is6() && !globalUnicast6.Contains(a) {
		return fmt.Errorf("%w: %s is not global unicast", ErrBlockedAddress, a)
	}
	for _, p := range blockedPrefixes {
		if p.Contains(a) {
			return fmt.Errorf("%w: %s is in %s", ErrBlockedAddress, a, p)
		}
	}
	return nil
}

// Resolver looks up host addresses; *net.Resolver satisfies it.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// normalizeHost lower-cases the host, strips a trailing dot and validates it as
// an IP literal or an ASCII (punycode) DNS name.
func normalizeHost(host string) (name string, ip netip.Addr, err error) {
	h := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if a, err := netip.ParseAddr(strings.Trim(h, "[]")); err == nil {
		return a.String(), a, nil
	}
	if len(h) == 0 || len(h) > 253 {
		return "", netip.Addr{}, fmt.Errorf("%w: bad host length", ErrInvalidTarget)
	}
	for _, label := range strings.Split(h, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", netip.Addr{}, fmt.Errorf("%w: bad label %q", ErrInvalidTarget, label)
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				return "", netip.Addr{}, fmt.Errorf("%w: bad character %q", ErrInvalidTarget, r)
			}
		}
	}
	return h, netip.Addr{}, nil
}

// resolve returns every address of host and the one to scan: the first IPv4,
// else the first IPv6. It refuses the whole host if any address is blocked:
// a public site has no reason to resolve to an internal address.
func (s *Scanner) resolve(ctx context.Context, host string, literal netip.Addr) (all []netip.Addr, pick netip.Addr, err error) {
	if literal.IsValid() {
		all = []netip.Addr{literal}
	} else {
		all, err = s.opts.Resolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, netip.Addr{}, fmt.Errorf("%w: %s: %w", ErrResolve, host, err)
		}
		if len(all) == 0 {
			return nil, netip.Addr{}, fmt.Errorf("%w: %s: no addresses", ErrResolve, host)
		}
	}
	for _, a := range all {
		if err := s.opts.AddrPolicy(a); err != nil {
			return all, netip.Addr{}, err
		}
	}
	for _, a := range all {
		if a.Unmap().Is4() {
			return all, a.Unmap(), nil
		}
	}
	return all, all[0], nil
}

// dial connects to an already validated address (never to a name, so a second
// DNS answer cannot redirect us: DNS rebinding) and re-checks the policy on the
// socket itself before connecting.
func (s *Scanner) dial(ctx context.Context, addr netip.AddrPort) (net.Conn, error) {
	if s.simulatedRTT > 0 {
		t := time.NewTimer(2 * s.simulatedRTT)
		select {
		case <-t.C:
		case <-ctx.Done():
			t.Stop()
			return nil, ctx.Err()
		}
	}
	d := net.Dialer{
		Timeout: s.opts.HandshakeTimeout,
		Control: func(_, address string, _ syscall.RawConn) error {
			ap, err := netip.ParseAddrPort(address)
			if err != nil {
				return fmt.Errorf("%w: %w", ErrBlockedAddress, err)
			}
			return s.opts.AddrPolicy(ap.Addr())
		},
	}
	return d.DialContext(ctx, "tcp", addr.String())
}
