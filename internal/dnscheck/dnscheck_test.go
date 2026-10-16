package dnscheck

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"

	"github.com/fizsar/quantumready-saas/internal/scanner"
)

// fakeDNS plays both roles on 127.0.0.1: a recursive resolver (questions
// with RD) and the authoritative server of the test zones (without RD).
type fakeDNS struct {
	port    uint16
	mu      sync.Mutex
	asked   []string    // "rd|type|name" of every question
	spoofed atomic.Bool // send a wrong-ID answer before the real one
}

func (f *fakeDNS) questions() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.asked)
}

func name(s string) dnsmessage.Name { return dnsmessage.MustNewName(s + ".") }

func rr(n string, body dnsmessage.ResourceBody) dnsmessage.Resource {
	return dnsmessage.Resource{Header: dnsmessage.ResourceHeader{Name: name(n), Class: dnsmessage.ClassINET, TTL: 300}, Body: body}
}

func (f *fakeDNS) answer(q dnsmessage.Message, tcp bool) dnsmessage.Message {
	qq := q.Questions[0]
	n := strings.TrimSuffix(qq.Name.String(), ".")
	rd := q.RecursionDesired
	f.mu.Lock()
	f.asked = append(f.asked, map[bool]string{true: "rd", false: "auth"}[rd]+"|"+qq.Type.String()+"|"+n)
	f.mu.Unlock()
	m := dnsmessage.Message{Header: dnsmessage.Header{ID: q.ID, Response: true, RecursionDesired: rd, RecursionAvailable: rd}, Questions: q.Questions}
	switch {
	case qq.Type == dnsmessage.TypeNS && rd:
		switch n {
		case "example.com":
			m.Answers = []dnsmessage.Resource{rr(n, &dnsmessage.NSResource{NS: name("ns1.example.com")})}
		case "evil.com":
			m.Answers = []dnsmessage.Resource{rr(n, &dnsmessage.NSResource{NS: name("ns.evil.com")})}
		case "lame.com":
			m.Answers = []dnsmessage.Resource{rr(n, &dnsmessage.NSResource{NS: name("ns1.example.com")})}
		}
	case qq.Type == dnsmessage.TypeA && rd:
		switch n {
		case "ns1.example.com":
			m.Answers = []dnsmessage.Resource{rr(n, &dnsmessage.AResource{A: [4]byte{127, 0, 0, 1}})}
		case "ns.evil.com":
			m.Answers = []dnsmessage.Resource{rr(n, &dnsmessage.AResource{A: [4]byte{10, 0, 0, 53}})}
		}
	case qq.Type == dnsmessage.TypeTXT && rd:
		// A resolver's cached, stale answer: must never be used.
		m.Answers = []dnsmessage.Resource{rr(n, &dnsmessage.TXTResource{TXT: []string{"qr-verify=stale"}})}
	case qq.Type == dnsmessage.TypeTXT:
		m.Authoritative = !strings.HasSuffix(n, "lame.com")
		switch n {
		case "_quantumready.example.com":
			m.Answers = []dnsmessage.Resource{rr(n, &dnsmessage.TXTResource{TXT: []string{"qr-verify=", "abc"}}),
				rr(n, &dnsmessage.TXTResource{TXT: []string{"v=other"}})}
		case "_quantumready.alias.example.com":
			m.Answers = []dnsmessage.Resource{rr(n, &dnsmessage.CNAMEResource{CNAME: name("_quantumready.example.com")})}
		case "_quantumready.big.example.com":
			if !tcp {
				m.Truncated = true
			} else {
				m.Answers = []dnsmessage.Resource{rr(n, &dnsmessage.TXTResource{TXT: []string{"qr-verify=bigone"}})}
			}
		case "_quantumready.empty.example.com", "_quantumready.lame.com":
		default:
			m.RCode = dnsmessage.RCodeNameError
		}
	}
	return m
}

func startFake(t *testing.T) *fakeDNS {
	t.Helper()
	var udp net.PacketConn
	var tcp net.Listener
	for range 20 {
		u, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		l, err := net.Listen("tcp", u.LocalAddr().String())
		if err == nil {
			udp, tcp = u, l
			break
		}
		u.Close()
	}
	if udp == nil {
		t.Fatal("no free port for UDP and TCP")
	}
	t.Cleanup(func() { udp.Close(); tcp.Close() })
	f := &fakeDNS{port: uint16(udp.LocalAddr().(*net.UDPAddr).Port)}
	go func() {
		buf := make([]byte, 512)
		for {
			n, from, err := udp.ReadFrom(buf)
			if err != nil {
				return
			}
			var q dnsmessage.Message
			if q.Unpack(buf[:n]) != nil {
				continue
			}
			m := f.answer(q, false)
			if f.spoofed.Load() {
				bad := m
				bad.ID++
				bad.Answers = []dnsmessage.Resource{rr("_quantumready.example.com", &dnsmessage.TXTResource{TXT: []string{"qr-verify=spoofed"}})}
				p, _ := bad.Pack()
				_, _ = udp.WriteTo(p, from)
			}
			p, _ := m.Pack()
			_, _ = udp.WriteTo(p, from)
		}
	}()
	go func() {
		for {
			c, err := tcp.Accept()
			if err != nil {
				return
			}
			var l [2]byte
			if _, err := io.ReadFull(c, l[:]); err == nil {
				b := make([]byte, binary.BigEndian.Uint16(l[:]))
				if _, err := io.ReadFull(c, b); err == nil {
					var q dnsmessage.Message
					if q.Unpack(b) == nil {
						m := f.answer(q, true)
						p, _ := m.Pack()
						binary.BigEndian.PutUint16(l[:], uint16(len(p)))
						_, _ = c.Write(append(l[:], p...))
					}
				}
			}
			c.Close()
		}
	}()
	return f
}

func (f *fakeDNS) checker() *Checker {
	return &Checker{
		Resolvers: []netip.AddrPort{netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), f.port)},
		Port:      f.port,
		Timeout:   time.Second,
		// Loopback stands in for a public server here; everything else as in production.
		CheckAddr: func(a netip.Addr) error {
			if a.IsLoopback() {
				return nil
			}
			return scanner.CheckAddr(a)
		},
	}
}

func TestTXTFromTheAuthoritativeServer(t *testing.T) {
	f := startFake(t)
	c := f.checker()
	ctx := context.Background()

	got, err := c.TXT(ctx, "_quantumready.Example.com.")
	if err != nil || !slices.Equal(got, []string{"qr-verify=abc", "v=other"}) {
		t.Fatalf("TXT = %q, %v", got, err)
	}
	// The TXT question went only to the authoritative server, without recursion.
	for _, q := range f.questions() {
		if strings.HasPrefix(q, "rd|TypeTXT") {
			t.Errorf("TXT asked to a resolver: %v", f.questions())
		}
	}
	if !slices.Contains(f.questions(), "auth|TypeTXT|_quantumready.example.com") || !slices.Contains(f.questions(), "rd|TypeNS|example.com") {
		t.Errorf("questions: %v", f.questions())
	}

	for n, want := range map[string][]string{
		"_quantumready.alias.example.com": {"qr-verify=abc", "v=other"}, // CNAME followed
		"_quantumready.big.example.com":   {"qr-verify=bigone"},         // truncated: TCP
		"_quantumready.none.example.com":  nil,                          // NXDOMAIN: no record
		"_quantumready.empty.example.com": nil,                          // no data
	} {
		if got, err := c.TXT(ctx, n); err != nil || !slices.Equal(got, want) {
			t.Errorf("TXT(%s) = %q, %v; want %q", n, got, err, want)
		}
	}
}

func TestUnavailableIsNotAbsent(t *testing.T) {
	f := startFake(t)
	c := f.checker()
	ctx := context.Background()
	for n, why := range map[string]string{
		"_quantumready.evil.com": "nameserver on a private address",
		"_quantumready.lame.com": "non-authoritative answer",
		"_quantumready.nope":     "no zone",
	} {
		if _, err := c.TXT(ctx, n); !errors.Is(err, ErrUnavailable) {
			t.Errorf("%s (%s): %v", n, why, err)
		}
	}
	// Nothing answers at all.
	dead := &Checker{Resolvers: []netip.AddrPort{netip.MustParseAddrPort("127.0.0.1:9")}, Timeout: 200 * time.Millisecond}
	if _, err := dead.TXT(ctx, "_quantumready.example.com"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("dead resolver: %v", err)
	}
}

func TestSpoofedAnswersAreIgnored(t *testing.T) {
	f := startFake(t)
	f.spoofed.Store(true)
	got, err := f.checker().TXT(context.Background(), "_quantumready.example.com")
	if err != nil || slices.Contains(got, "qr-verify=spoofed") || !slices.Contains(got, "qr-verify=abc") {
		t.Errorf("TXT = %q, %v", got, err)
	}
}
