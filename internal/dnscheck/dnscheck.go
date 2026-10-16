// Package dnscheck reads a TXT record from the zone's authoritative name
// servers, never from a cache: neither the host's (systemd-resolved,
// Docker's embedded DNS) nor a public resolver's. Public resolvers are used
// only to find the zone's NS records and their addresses, which change
// rarely; the TXT question goes to the authoritative servers without
// recursion and only an authoritative answer counts. Only DNS is followed
// (CNAMEs, a few hops); nameserver addresses must be public (the scanner's
// anti-SSRF rules), so a domain cannot point its NS at internal hosts.
package dnscheck

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strings"
	"time"

	"golang.org/x/net/dns/dnsmessage"

	"github.com/fizsar/quantumready-saas/internal/scanner"
)

// ErrUnavailable means no authoritative server gave a usable answer: a
// temporary failure, not proof that the record is missing.
var ErrUnavailable = errors.New("dnscheck: no authoritative answer")

// DefaultResolvers are public recursive resolvers (Cloudflare, Quad9,
// Google), used only to find the authoritative servers.
var DefaultResolvers = []netip.AddrPort{
	netip.MustParseAddrPort("1.1.1.1:53"),
	netip.MustParseAddrPort("9.9.9.9:53"),
	netip.MustParseAddrPort("8.8.8.8:53"),
	netip.MustParseAddrPort("[2606:4700:4700::1111]:53"),
}

// Checker asks authoritative servers. The zero value uses the defaults.
type Checker struct {
	Resolvers []netip.AddrPort       // default DefaultResolvers
	Timeout   time.Duration          // per query; default 3 s
	CheckAddr func(netip.Addr) error // nameserver addresses; default scanner.CheckAddr
	Port      uint16                 // of the authoritative servers; default 53 (tests change it)
}

const (
	maxCNAMEs  = 4
	maxServers = 6
)

func (c *Checker) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return 3 * time.Second
}

// TXT returns the TXT strings at name (each record's strings joined), as
// the zone's authoritative servers answer now. No record is (nil, nil);
// ErrUnavailable if no server answered.
func (c *Checker) TXT(ctx context.Context, name string) ([]string, error) {
	name = strings.TrimSuffix(strings.ToLower(name), ".")
	for range maxCNAMEs + 1 {
		servers, err := c.authoritative(ctx, name)
		if err != nil {
			return nil, err
		}
		txt, cname, err := c.askAuthoritative(ctx, servers, name)
		if err != nil || cname == "" {
			return txt, err
		}
		name = cname
	}
	return nil, fmt.Errorf("%w: too many CNAMEs", ErrUnavailable)
}

// authoritative finds the zone of name (the closest ancestor with NS
// records) and the public addresses of its nameservers.
func (c *Checker) authoritative(ctx context.Context, name string) ([]netip.AddrPort, error) {
	labels := strings.Split(name, ".")
	for i := 0; i < len(labels)-1; i++ { // the TLD's own servers never hold the record
		zone := strings.Join(labels[i:], ".")
		msg, err := c.recursive(ctx, zone, dnsmessage.TypeNS)
		if err != nil {
			return nil, err
		}
		var hosts []string
		for _, rr := range msg.Answers {
			if ns, ok := rr.Body.(*dnsmessage.NSResource); ok && sameName(rr.Header.Name, zone) {
				hosts = append(hosts, strings.TrimSuffix(strings.ToLower(ns.NS.String()), "."))
			}
		}
		if len(hosts) == 0 {
			continue
		}
		var addrs []netip.AddrPort
		for _, h := range hosts {
			for _, t := range []dnsmessage.Type{dnsmessage.TypeA, dnsmessage.TypeAAAA} {
				m, err := c.recursive(ctx, h, t)
				if err != nil {
					continue
				}
				for _, rr := range m.Answers {
					var a netip.Addr
					switch b := rr.Body.(type) {
					case *dnsmessage.AResource:
						a = netip.AddrFrom4(b.A)
					case *dnsmessage.AAAAResource:
						a = netip.AddrFrom16(b.AAAA).Unmap()
					default:
						continue
					}
					if c.checkAddr(a) == nil && len(addrs) < maxServers {
						addrs = append(addrs, netip.AddrPortFrom(a, c.port()))
					}
				}
			}
		}
		if len(addrs) == 0 {
			return nil, fmt.Errorf("%w: no public address for the nameservers of %s", ErrUnavailable, zone)
		}
		return addrs, nil
	}
	return nil, fmt.Errorf("%w: no nameservers for %s", ErrUnavailable, name)
}

func (c *Checker) checkAddr(a netip.Addr) error {
	if c.CheckAddr != nil {
		return c.CheckAddr(a)
	}
	return scanner.CheckAddr(a)
}

func (c *Checker) port() uint16 {
	if c.Port != 0 {
		return c.Port
	}
	return 53
}

// recursive asks the public resolvers in turn (recursion desired).
func (c *Checker) recursive(ctx context.Context, name string, t dnsmessage.Type) (*dnsmessage.Message, error) {
	resolvers := c.Resolvers
	if len(resolvers) == 0 {
		resolvers = DefaultResolvers
	}
	var last error
	for _, r := range resolvers {
		m, err := c.exchange(ctx, r, name, t, true)
		if err == nil && (m.RCode == dnsmessage.RCodeSuccess || m.RCode == dnsmessage.RCodeNameError) {
			return m, nil
		}
		if err == nil {
			err = fmt.Errorf("rcode %v", m.RCode)
		}
		last = err
		if ctx.Err() != nil {
			break
		}
	}
	return nil, fmt.Errorf("%w: resolvers: %w", ErrUnavailable, last)
}

// askAuthoritative asks the servers for TXT at name without recursion. A
// CNAME at name (and no TXT) is returned for the caller to follow.
func (c *Checker) askAuthoritative(ctx context.Context, servers []netip.AddrPort, name string) (txt []string, cname string, err error) {
	var last error
	for _, s := range servers {
		m, err := c.exchange(ctx, s, name, dnsmessage.TypeTXT, false)
		switch {
		case err != nil:
			last = err
			continue
		case !m.Authoritative:
			last = errors.New("answer not authoritative")
			continue
		case m.RCode == dnsmessage.RCodeNameError:
			return nil, "", nil
		case m.RCode != dnsmessage.RCodeSuccess:
			last = fmt.Errorf("rcode %v", m.RCode)
			continue
		}
		for _, rr := range m.Answers {
			if !sameName(rr.Header.Name, name) {
				continue
			}
			switch b := rr.Body.(type) {
			case *dnsmessage.TXTResource:
				txt = append(txt, strings.Join(b.TXT, ""))
			case *dnsmessage.CNAMEResource:
				cname = strings.TrimSuffix(strings.ToLower(b.CNAME.String()), ".")
			}
		}
		if len(txt) > 0 {
			cname = ""
		}
		return txt, cname, nil
	}
	return nil, "", fmt.Errorf("%w: %w", ErrUnavailable, last)
}

func sameName(n dnsmessage.Name, name string) bool {
	return strings.EqualFold(strings.TrimSuffix(n.String(), "."), name)
}

// exchange sends one question over UDP (TCP if the answer is truncated) and
// returns the matching answer.
func (c *Checker) exchange(ctx context.Context, server netip.AddrPort, name string, t dnsmessage.Type, recursion bool) (*dnsmessage.Message, error) {
	qname, err := dnsmessage.NewName(name + ".")
	if err != nil {
		return nil, err
	}
	var idb [2]byte
	if _, err := rand.Read(idb[:]); err != nil {
		return nil, err
	}
	q := dnsmessage.Question{Name: qname, Type: t, Class: dnsmessage.ClassINET}
	query := dnsmessage.Message{Header: dnsmessage.Header{ID: binary.BigEndian.Uint16(idb[:]), RecursionDesired: recursion}, Questions: []dnsmessage.Question{q}}
	packed, err := query.Pack()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()
	m, err := c.roundTrip(ctx, "udp", server, packed, query.ID, q)
	if err == nil && m.Truncated {
		m, err = c.roundTrip(ctx, "tcp", server, packed, query.ID, q)
	}
	return m, err
}

func (c *Checker) roundTrip(ctx context.Context, network string, server netip.AddrPort, packed []byte, id uint16, q dnsmessage.Question) (*dnsmessage.Message, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, network, server.String())
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	if network == "tcp" {
		var l [2]byte
		binary.BigEndian.PutUint16(l[:], uint16(len(packed))) //nolint:gosec // a single question, far below 64 KiB
		packed = append(l[:], packed...)
	}
	if _, err := conn.Write(packed); err != nil {
		return nil, err
	}
	for {
		var buf []byte
		if network == "tcp" {
			var l [2]byte
			if _, err := io.ReadFull(conn, l[:]); err != nil {
				return nil, err
			}
			buf = make([]byte, binary.BigEndian.Uint16(l[:]))
			if _, err := io.ReadFull(conn, buf); err != nil {
				return nil, err
			}
		} else {
			buf = make([]byte, 65535)
			n, err := conn.Read(buf)
			if err != nil {
				return nil, err
			}
			buf = buf[:n]
		}
		var m dnsmessage.Message
		if err := m.Unpack(buf); err != nil {
			if network == "tcp" {
				return nil, err
			}
			continue // garbage on UDP: keep waiting for the real answer
		}
		if m.ID != id || !m.Response || len(m.Questions) != 1 || m.Questions[0].Type != q.Type ||
			!strings.EqualFold(m.Questions[0].Name.String(), q.Name.String()) {
			if network == "tcp" {
				return nil, errors.New("mismatched answer")
			}
			continue // not ours (spoofed or late): ignore
		}
		return &m, nil
	}
}
