package scanner

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
)

// ErrUnreachable is returned when the target cannot be reached over TCP or
// does not complete a browser-like TLS handshake at all.
var ErrUnreachable = errors.New("scanner: target unreachable")

// HybridGroups are the hybrid ML-KEM groups crypto/tls supports, probed one by one.
var HybridGroups = []tls.CurveID{tls.X25519MLKEM768, tls.SecP256r1MLKEM768, tls.SecP384r1MLKEM1024}

// ClassicGroups are the classical (EC)DHE groups crypto/tls supports.
var ClassicGroups = []tls.CurveID{tls.X25519, tls.CurveP256, tls.CurveP384, tls.CurveP521}

// BrowserGroups mimics the group preference of current browsers (Chrome,
// Firefox): hybrid first, with key shares for X25519MLKEM768 and X25519.
// Go cannot reproduce a browser ClientHello byte for byte (GREASE, extension
// order), only its key exchange preference.
var BrowserGroups = []tls.CurveID{tls.X25519MLKEM768, tls.X25519, tls.CurveP256, tls.CurveP384}

var probedVersions = []uint16{tls.VersionTLS10, tls.VersionTLS11, tls.VersionTLS12, tls.VersionTLS13}

// Options configures a Scanner. Zero values select the defaults.
type Options struct {
	HandshakeTimeout     time.Duration // per connection; default 5s
	ScanTimeout          time.Duration // whole scan; default 60s
	PerTargetConcurrency int           // parallel handshakes per scan; default 4
	GlobalConcurrency    int           // parallel handshakes across scans; default 16

	Resolver   Resolver               // default net.DefaultResolver
	AddrPolicy func(netip.Addr) error // default CheckAddr (anti-SSRF)
	RootCAs    *x509.CertPool         // nil: system roots
	Now        func() time.Time       // default time.Now
}

// Scanner runs TLS scans. It is safe for concurrent use; GlobalConcurrency
// bounds the handshakes of all scans together.
type Scanner struct {
	opts Options
	sem  chan struct{}

	active, maxActive atomic.Int32 // concurrent handshakes (observed by tests)
}

// New returns a Scanner with opts, filling in defaults.
func New(opts Options) *Scanner {
	if opts.HandshakeTimeout <= 0 {
		opts.HandshakeTimeout = 5 * time.Second
	}
	if opts.ScanTimeout <= 0 {
		opts.ScanTimeout = 60 * time.Second
	}
	if opts.PerTargetConcurrency <= 0 {
		opts.PerTargetConcurrency = 4
	}
	if opts.GlobalConcurrency <= 0 {
		opts.GlobalConcurrency = 16
	}
	if opts.Resolver == nil {
		opts.Resolver = net.DefaultResolver
	}
	if opts.AddrPolicy == nil {
		opts.AddrPolicy = CheckAddr
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Scanner{opts: opts, sem: make(chan struct{}, opts.GlobalConcurrency)}
}

type target struct {
	host string // normalized name or IP literal
	sni  string // "" for IP literals
	addr netip.AddrPort
}

// Scan probes host:port (port 0 means 443). It returns an error only when no
// result is possible: invalid or blocked target, DNS failure, or a target that
// is unreachable or does not speak TLS. Individual probe failures are recorded
// in the Result with status "error".
func (s *Scanner) Scan(ctx context.Context, host string, port int) (*Result, error) {
	if port == 0 {
		port = 443
	}
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("%w: port %d", ErrInvalidTarget, port)
	}
	name, literal, err := normalizeHost(host)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, s.opts.ScanTimeout)
	defer cancel()
	start := s.opts.Now()

	all, ip, err := s.resolve(ctx, name, literal)
	if err != nil {
		return nil, err
	}
	t := target{host: name, sni: name, addr: netip.AddrPortFrom(ip, uint16(port))}
	if literal.IsValid() {
		t.sni = ""
	}

	res := &Result{
		SchemaVersion:       SchemaVersion,
		Host:                name,
		Port:                port,
		IP:                  ip.String(),
		StartedAt:           start.UTC(),
		TLS12SuitesComplete: true,
		NotEvaluated:        notEvaluated,
	}
	for _, a := range all {
		res.ResolvedIPs = append(res.ResolvedIPs, a.String())
	}

	// Browser-like handshake first: it tells us whether the target speaks TLS at all.
	base := s.handshake(ctx, t, &tls.Config{MinVersion: tls.VersionTLS12, CurvePreferences: BrowserGroups})
	if base.status == ProbeFailed {
		return nil, fmt.Errorf("%w: %w", ErrUnreachable, base.err)
	}
	res.Default = base.summary()
	res.KeyExchange.Offered = groupNames(BrowserGroups)
	if base.state != nil && base.state.CurveID != 0 {
		res.KeyExchange.Selected = groupName(base.state.CurveID)
		res.KeyExchange.SelectedHybrid = isHybrid(base.state.CurveID)
	}

	var (
		mu    sync.Mutex
		certs = base.certs
		state = base.state
	)
	keep := func(p probe) { // first certificate chain seen, in case the default handshake failed
		mu.Lock()
		defer mu.Unlock()
		if len(certs) == 0 && len(p.certs) > 0 {
			certs, state = p.certs, p.state
		}
	}

	groups := append(append([]tls.CurveID{}, HybridGroups...), ClassicGroups...)
	res.KeyExchange.Groups = make([]GroupProbe, len(groups))
	res.Versions = make([]VersionProbe, len(probedVersions))

	var g errgroup.Group
	g.SetLimit(s.opts.PerTargetConcurrency)
	for i, id := range groups {
		g.Go(func() error {
			p := s.probeGroup(ctx, t, id)
			keep(p)
			res.KeyExchange.Groups[i] = GroupProbe{Name: groupName(id), ID: uint16(id), Hybrid: isHybrid(id), Status: p.status, Error: p.errString()}
			return nil
		})
	}
	for i, v := range probedVersions {
		g.Go(func() error {
			p := s.handshake(ctx, t, versionConfig(v))
			keep(p)
			res.Versions[i] = VersionProbe{Version: tls.VersionName(v), ID: v, Status: p.status, Error: p.errString()}
			if v == tls.VersionTLS13 && p.state != nil {
				res.TLS13Suite = tls.CipherSuiteName(p.state.CipherSuite)
			}
			return nil
		})
	}
	g.Go(func() error {
		res.TLS12Suites, res.TLS12SuitesComplete = s.enumerateTLS12Suites(ctx, t)
		return nil
	})
	_ = g.Wait()

	if len(certs) > 0 {
		res.Certificate = analyzeCertificate(certs, state, t.host, s.opts.Now(), s.opts.RootCAs)
	}
	res.DurationMS = s.opts.Now().Sub(start).Milliseconds()
	return res, nil
}

// probeGroup offers a single group. Hybrids need TLS 1.3; classical groups are
// tried with TLS 1.2 and 1.3 and ECDHE suites only, so a success proves the
// group was actually used (an RSA key exchange would not use any group).
func (s *Scanner) probeGroup(ctx context.Context, t target, id tls.CurveID) probe {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, MaxVersion: tls.VersionTLS13, CurvePreferences: []tls.CurveID{id}, CipherSuites: ecdheSuites()}
	if isHybrid(id) {
		cfg.MinVersion = tls.VersionTLS13
	}
	p := s.handshake(ctx, t, cfg)
	if p.status == Supported && p.state.CurveID != id {
		p.status = NotSupported
		p.err = fmt.Errorf("server negotiated %s instead", groupName(p.state.CurveID))
	}
	return p
}

// versionConfig offers exactly one version with every suite and group Go has.
func versionConfig(v uint16) *tls.Config {
	return &tls.Config{
		MinVersion:       v,
		MaxVersion:       v,
		CipherSuites:     allTLS12Suites(),
		CurvePreferences: append(append([]tls.CurveID{}, BrowserGroups...), tls.CurveP521, tls.SecP256r1MLKEM768, tls.SecP384r1MLKEM1024),
	}
}

// enumerateTLS12Suites offers every remaining suite and removes the one the
// server picks, until the server refuses. complete is false if a probe errored.
func (s *Scanner) enumerateTLS12Suites(ctx context.Context, t target) (found []CipherSuite, complete bool) {
	remaining := allTLS12Suites()
	for len(remaining) > 0 {
		p := s.handshake(ctx, t, &tls.Config{ //nolint:gosec // G402: TLS 1.2 on purpose, to enumerate its suites
			MinVersion:       tls.VersionTLS12,
			MaxVersion:       tls.VersionTLS12,
			CipherSuites:     remaining,
			CurvePreferences: ClassicGroups,
		})
		switch p.status {
		case NotSupported:
			return found, true
		case ProbeFailed:
			return found, false
		}
		chosen := p.state.CipherSuite
		idx := -1
		for i, id := range remaining {
			if id == chosen {
				idx = i
			}
		}
		if idx < 0 { // server picked a suite we did not offer: stop rather than loop
			return found, false
		}
		found = append(found, suiteInfo(chosen))
		remaining = append(remaining[:idx], remaining[idx+1:]...)
	}
	return found, true
}

func (s *Scanner) acquire(ctx context.Context) error {
	select {
	case s.sem <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	n := s.active.Add(1)
	for {
		m := s.maxActive.Load()
		if n <= m || s.maxActive.CompareAndSwap(m, n) {
			return nil
		}
	}
}

func (s *Scanner) release() {
	s.active.Add(-1)
	<-s.sem
}
