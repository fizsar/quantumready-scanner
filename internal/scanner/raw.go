package scanner

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"slices"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/fizsar/quantumready-saas/internal/scanner/rawtls"
)

// errBudget stops raw probing once a scan has used RawProbeBudget connections.
var errBudget = errors.New("scanner: raw probe budget exhausted")

// rawScan runs the raw ClientHello probes of one scan (intensity full).
type rawScan struct {
	s          *Scanner
	t          target
	budget     int32
	used       atomic.Int32
	exhausted  atomic.Bool // a probe was skipped for lack of budget
	sslv3Suite RawSuite    // the suite picked by the SSLv3 probe, if any

	mu  sync.Mutex
	res RawResult
	sel *CertificateSelection // the scan's, filled by certificateWithoutMLDSA
	// chain is the certificate chain the raw probes saw in clear (TLS 1.0–1.2,
	// SSLv3, SSLv2), from the highest version that sent one (chainVersion).
	chain        [][]byte
	chainVersion uint16
}

// noteChain keeps the chain a probe of version v saw, unless one was already
// seen in a higher version: the same choice whatever order the probes end in.
func (rs *rawScan) noteChain(chain [][]byte, v uint16) {
	if len(chain) == 0 {
		return
	}
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.chain == nil || v > rs.chainVersion {
		rs.chain, rs.chainVersion = chain, v
	}
}

// rawOutcome is one raw probe, classified like the crypto/tls probes.
type rawOutcome struct {
	status Status
	resp   *rawtls.Response
	err    error
}

func (o rawOutcome) errString() string {
	if o.err == nil {
		return ""
	}
	return o.err.Error()
}

// exchange opens one connection (same semaphore, anti-SSRF dialer and
// timeout as the crypto/tls probes, within the budget) and runs fn on it. It
// returns the recorded connection (nil if none was made) and when it
// started, for the probe log.
func (rs *rawScan) exchange(ctx context.Context, fn func(net.Conn) error) (*recordingConn, time.Time, error) {
	start := time.Now()
	if rs.used.Add(1) > rs.budget {
		rs.used.Add(-1)
		rs.exhausted.Store(true)
		return nil, start, errBudget
	}
	if err := rs.s.acquire(ctx); err != nil {
		return nil, start, err
	}
	defer rs.s.release()
	ctx, cancel := context.WithTimeout(ctx, rs.s.opts.HandshakeTimeout)
	defer cancel()
	rs.t.conns.Add(1)
	start = time.Now()
	conn, err := rs.s.dial(ctx, rs.t.addr)
	if err != nil {
		return nil, start, &connectError{err}
	}
	defer conn.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	rc := &recordingConn{Conn: conn}
	return rc, start, fn(rc)
}

// hello sends h, classifies the answer and logs the probe under tag. check,
// if set, judges a ServerHello that does not answer the probe's question
// (another version, no HelloRetryRequest), before it is logged.
func (rs *rawScan) hello(ctx context.Context, tag probeTag, h *rawtls.Hello, readKeyExchange bool, check func(*rawtls.ServerHello) (Status, error)) rawOutcome {
	var resp *rawtls.Response
	rc, start, err := rs.exchange(ctx, func(c net.Conn) error {
		var err error
		resp, err = rawtls.Exchange(c, h, rawtls.Options{ReadKeyExchange: readKeyExchange})
		return err
	})
	o := classifyRaw(resp, err)
	if o.resp != nil && o.resp.ServerHello != nil {
		rs.noteChain(o.resp.Certificates, o.resp.ServerHello.Version)
	}
	if o.status == Supported && check != nil {
		if st, cerr := check(o.resp.ServerHello); cerr != nil {
			o.status, o.err = st, cerr
		}
	}
	rs.t.log.add(clientRaw, tag, start, rc, o.status, o.err, nil)
	return o
}

// otherVersion is the check of probes that offer one version.
func otherVersion(v uint16) func(*rawtls.ServerHello) (Status, error) {
	return func(sh *rawtls.ServerHello) (Status, error) {
		if sh.Version != v {
			return NotSupported, &outcomeError{"other_version", "server answered " + rawtls.VersionName(sh.Version)}
		}
		return Supported, nil
	}
}

// classifyRaw maps a raw exchange to supported / not supported / error.
// Like crypto/tls probes: an alert, a closed connection or a parameter we did
// not offer means "not supported"; anything unparsable, a timeout or the
// budget means "error", never "not supported".
func classifyRaw(resp *rawtls.Response, err error) rawOutcome {
	switch {
	case resp != nil && resp.ServerHello != nil:
		return rawOutcome{status: Supported, resp: resp, err: err}
	case resp != nil && resp.Alert != nil:
		return rawOutcome{status: NotSupported, resp: resp, err: &alertError{*resp.Alert}}
	case errors.Is(err, rawtls.ErrUnoffered), errors.Is(err, io.EOF), errors.Is(err, syscall.ECONNRESET):
		return rawOutcome{status: NotSupported, err: err}
	}
	return rawOutcome{status: ProbeFailed, err: err}
}

// runRaw runs every raw probe after the crypto/tls ones (it needs their
// results) and fills res.Raw. It returns the certificate chain the probes saw
// in clear, if any (leaf first, DER).
func (s *Scanner) runRaw(ctx context.Context, t target, res *Result) [][]byte {
	rs := &rawScan{s: s, t: t, budget: int32(min(s.opts.RawProbeBudget, 1<<20)), sel: res.CertificateSelection} //nolint:gosec // bounded
	found := goFound(res)

	var g errgroup.Group
	g.SetLimit(s.opts.PerTargetConcurrency)
	g.Go(func() error { rs.sslv2(ctx); return nil })
	g.Go(func() error { rs.sslv3(ctx); return nil })
	g.Go(func() error {
		// TLS 1.2 first; 1.1 and 1.0 then only look for suites not found above.
		complete := true
		for _, v := range []uint16{rawtls.VersionTLS12, rawtls.VersionTLS11, rawtls.VersionTLS10} {
			if !rs.suites(ctx, v, found, versionStatusOf(res, v) != Supported) {
				complete = false
			}
		}
		rs.mu.Lock()
		rs.res.SuitesComplete = complete
		rs.mu.Unlock()
		return nil
	})
	g.Go(func() error { rs.tls13Suites(ctx); return nil })
	g.Go(func() error { rs.certificateWithoutMLDSA(ctx, res); return nil })
	g.Go(func() error { rs.chromeOffer(ctx, res); return nil })
	_ = g.Wait()

	// Groups need to know whether TLS 1.3 / ECDHE is there at all.
	tls13 := versionStatusOf(res, tls.VersionTLS13) == Supported || len(rs.res.TLS13Suites) > 0
	ecdheVersion := rs.ecdheVersion(res)
	var g2 errgroup.Group
	g2.SetLimit(s.opts.PerTargetConcurrency)
	if rs.res.SSLv3.Status == Supported {
		// Only SSLv3 servers pay for this: the suites accepted in SSLv3 and
		// not already found in a TLS version.
		g2.Go(func() error {
			rs.mu.Lock()
			rs.res.Suites = append(rs.res.Suites, rs.sslv3Suite)
			rs.mu.Unlock()
			if !rs.suites(ctx, rawtls.VersionSSL30, found, false) {
				rs.mu.Lock()
				rs.res.SuitesComplete = false
				rs.mu.Unlock()
			}
			return nil
		})
	}
	g2.Go(func() error {
		if tls13 {
			rs.tls13Groups(ctx)
		} else {
			rs.setTLS13Groups(nil, true)
		}
		return nil
	})
	g2.Go(func() error {
		if ecdheVersion != 0 {
			rs.tls12Curves(ctx, ecdheVersion)
		} else {
			rs.setTLS12Curves(nil, false, true)
		}
		return nil
	})
	_ = g2.Wait()
	// ECDHE only in SSLv3: its curves are not probed (SSLv3 clients send no
	// supported_groups), so the curve list is not evaluated.
	if ecdheVersion == 0 && slices.ContainsFunc(rs.res.Suites, func(s RawSuite) bool {
		suite, _ := rawtls.SuiteByID(s.ID)
		return s.Version == "SSLv3" && suite.Kx == "ECDHE"
	}) {
		rs.res.TLS12CurvesComplete = false
	}

	rs.res.Probes = int(rs.used.Load())
	rs.res.BudgetExhausted = rs.exhausted.Load()
	slices.SortFunc(rs.res.DH, func(a, b DHParams) int { return a.PrimeBits - b.PrimeBits })
	rs.res.Normalize()
	res.Raw = &rs.res
	res.NotEvaluated = notEvaluatedFull(&rs.res)
	return rs.chain
}

// goFound is what crypto/tls already found, per version.
func goFound(res *Result) map[uint16]map[uint16]bool {
	found := map[uint16]map[uint16]bool{}
	for v, list := range map[uint16][]CipherSuite{rawtls.VersionTLS12: res.TLS12Suites, rawtls.VersionTLS11: res.TLS11Suites, rawtls.VersionTLS10: res.TLS10Suites} {
		found[v] = map[uint16]bool{}
		for _, s := range list {
			found[v][s.ID] = true
		}
	}
	return found
}

func versionStatusOf(res *Result, v uint16) Status {
	for _, p := range res.Versions {
		if p.ID == v {
			return p.Status
		}
	}
	return ""
}

func (rs *rawScan) sslv2(ctx context.Context) {
	var ids []uint32
	for _, c := range rawtls.SSLv2Ciphers {
		ids = append(ids, c.ID)
	}
	var resp *rawtls.SSLv2Response
	rc, start, err := rs.exchange(ctx, func(c net.Conn) error {
		var err error
		resp, err = rawtls.ExchangeSSLv2(c, ids)
		return err
	})
	p := RawProtocol{}
	switch {
	case err == nil && resp.ServerHello != nil:
		p.Status = Supported
		if c := resp.ServerHello.Certificate; c != nil {
			rs.noteChain([][]byte{c}, rawtls.VersionSSL20)
		}
		for _, id := range resp.ServerHello.Ciphers {
			for _, c := range rawtls.SSLv2Ciphers {
				if c.ID == id {
					p.Ciphers = append(p.Ciphers, c.Name)
				}
			}
		}
	case err == nil: // SSLv2 ERROR, TLS alert or a TLS ServerHello: no SSLv2
		p.Status = NotSupported
	default:
		o := classifyRaw(nil, err)
		p.Status, p.Error, p.ErrorCode = o.status, o.errString(), errorCode(o.err)
	}
	var r *ProbeResponse
	if rc != nil {
		r = sslv2Response(resp, rc.out, rc.in, err)
	}
	rs.t.log.add(clientRaw, probeTag{purpose: PurposeSSLv2}, start, rc, p.Status, err, r)
	rs.mu.Lock()
	rs.res.SSLv2 = p
	rs.mu.Unlock()
}

func (rs *rawScan) sslv3(ctx context.Context) {
	h := &rawtls.Hello{Version: rawtls.VersionSSL30, Suites: candidateSuites(rawtls.VersionSSL30, nil), NoExtensions: true}
	o := rs.hello(ctx, probeTag{purpose: PurposeSSLv3}, h, true, otherVersion(rawtls.VersionSSL30))
	p := RawProtocol{Status: o.status, Error: o.errString(), ErrorCode: errorCode(o.err)}
	if o.status == Supported {
		sh := o.resp.ServerHello
		p.Ciphers = []string{rawtls.SuiteName(sh.CipherSuite)}
		rs.noteDH(o, sh.Version)
		suite, _ := rawtls.SuiteByID(sh.CipherSuite)
		rs.mu.Lock()
		rs.sslv3Suite = rawSuite(suite, rawtls.VersionSSL30)
		rs.mu.Unlock()
	}
	rs.mu.Lock()
	rs.res.SSLv3 = p
	rs.mu.Unlock()
}

// candidateSuites are the offerable non-TLS 1.3 suites valid in version v,
// except those in skip.
func candidateSuites(v uint16, skip map[uint16]bool) []uint16 {
	var ids []uint16
	for _, s := range rawtls.Suites() {
		if s.Offerable && !s.TLS13 && s.MinVersion() <= v && !skip[s.ID] {
			ids = append(ids, s.ID)
		}
	}
	return ids
}

// tls12Groups is the supported_groups list of TLS 1.0–1.2 suite probes: the
// usual curves first, then every other one, so ECDHE suites can be chosen.
func tls12Groups() []uint16 {
	ids := []uint16{0x001d, 0x0017, 0x0018, 0x0019, 0x001e}
	for _, g := range rawtls.Groups() {
		if g.TLS12 && g.Kind != rawtls.GroupOther && !slices.Contains(ids, g.ID) {
			ids = append(ids, g.ID)
		}
	}
	return ids
}

// suites enumerates, in version v, the suites not yet found (by crypto/tls in
// v, or by this enumeration in a higher version). When detect is set (TLS
// 1.0–1.2 that crypto/tls did not see), the first probe offers every suite to
// learn whether the server speaks v at all, and records it in res.Versions.
// It returns false if it stopped on an error or the budget.
func (rs *rawScan) suites(ctx context.Context, v uint16, goFound map[uint16]map[uint16]bool, detect bool) bool {
	skip := map[uint16]bool{}
	for id := range goFound[v] {
		skip[id] = true
	}
	rs.mu.Lock()
	for _, s := range rs.res.Suites {
		skip[s.ID] = true
	}
	rs.mu.Unlock()
	remaining := candidateSuites(v, skip)
	offer := remaining
	if detect {
		offer = candidateSuites(v, nil)
	}
	version := func(st Status, err error) {
		if detect {
			rs.mu.Lock()
			rs.res.Versions = append(rs.res.Versions, VersionProbe{Version: rawtls.VersionName(v), ID: v, Status: st, Error: errText(err), ErrorCode: errorCode(err)})
			rs.mu.Unlock()
			detect = false
		}
	}
	groups := tls12Groups()
	for len(offer) > 0 {
		h := &rawtls.Hello{Version: v, Suites: offer, ServerName: rs.t.sni, Groups: groups}
		if v == rawtls.VersionSSL30 { // SSLv3 clients sent no extensions
			h = &rawtls.Hello{Version: v, Suites: offer, NoExtensions: true}
		}
		o := rs.hello(ctx, probeTag{PurposeSuites, rawtls.VersionName(v)}, h, true, otherVersion(v))
		switch o.status {
		case NotSupported: // refused, or the server does not speak v
			version(NotSupported, o.err)
			return true
		case ProbeFailed:
			version(ProbeFailed, o.err)
			return false
		}
		sh := o.resp.ServerHello
		version(Supported, nil)
		if slices.Contains(remaining, sh.CipherSuite) { // new, not a suite already found
			rs.noteDH(o, v)
			s, _ := rawtls.SuiteByID(sh.CipherSuite)
			rs.mu.Lock()
			rs.res.Suites = append(rs.res.Suites, rawSuite(s, v))
			rs.mu.Unlock()
		}
		remaining = slices.DeleteFunc(remaining, func(id uint16) bool { return id == sh.CipherSuite })
		offer = remaining
	}
	return true
}

func rawSuite(s rawtls.Suite, v uint16) RawSuite {
	return RawSuite{
		Version: rawtls.VersionName(v), Name: s.Name, ID: s.ID, ForwardSecrecy: s.ForwardSecrecy() && !s.Anonymous(),
		AEAD: s.AEAD(), Mode: string(s.Mode), Bits: s.Bits, Export: s.Export, Anonymous: s.Anonymous(),
	}
}

// noteDH records the DH group size of a DHE answer.
func (rs *rawScan) noteDH(o rawOutcome, v uint16) {
	ke := o.resp.KeyExchange
	if ke == nil || ke.Kind != "DH" {
		return
	}
	rs.mu.Lock()
	defer rs.mu.Unlock()
	for _, d := range rs.res.DH {
		if d.Version == rawtls.VersionName(v) && d.PrimeBits == ke.DHPrimeBits {
			return
		}
	}
	rs.res.DH = append(rs.res.DH, DHParams{Version: rawtls.VersionName(v), Suite: rawtls.SuiteName(o.resp.ServerHello.CipherSuite), PrimeBits: ke.DHPrimeBits})
}

func tls13SuiteIDs() []uint16 {
	var ids []uint16
	for _, s := range rawtls.Suites() {
		if s.Offerable && s.TLS13 {
			ids = append(ids, s.ID)
		}
	}
	return ids
}

func tls13GroupIDs() []uint16 {
	ids := []uint16{0x001d, 0x0017, 0x0018}
	for _, g := range rawtls.Groups() {
		if g.TLS13 && !slices.Contains(ids, g.ID) {
			ids = append(ids, g.ID)
		}
	}
	return ids
}

// tls13Suites enumerates every TLS 1.3 suite. A HelloRetryRequest also names
// the chosen suite, so the server does not need to support x25519.
func (rs *rawScan) tls13Suites(ctx context.Context) {
	ks, err := rawtls.X25519KeyShare()
	if err != nil {
		rs.setTLS13Suites(false)
		return
	}
	remaining := tls13SuiteIDs()
	groups := tls13GroupIDs()
	for len(remaining) > 0 {
		h := &rawtls.Hello{Version: rawtls.VersionTLS12, SupportedVersions: []uint16{rawtls.VersionTLS13}, Suites: remaining,
			ServerName: rs.t.sni, Groups: groups, KeyShares: []rawtls.KeyShare{ks}}
		o := rs.hello(ctx, probeTag{purpose: PurposeTLS13Suites}, h, false, nil)
		switch o.status {
		case NotSupported:
			rs.setTLS13Suites(true)
			return
		case ProbeFailed:
			rs.setTLS13Suites(false)
			return
		}
		id := o.resp.ServerHello.CipherSuite
		s, _ := rawtls.SuiteByID(id)
		rs.mu.Lock()
		rs.res.TLS13Suites = append(rs.res.TLS13Suites, rawSuite(s, rawtls.VersionTLS13))
		rs.mu.Unlock()
		remaining = slices.DeleteFunc(remaining, func(x uint16) bool { return x == id })
	}
	rs.setTLS13Suites(true)
}

func (rs *rawScan) setTLS13Suites(complete bool) {
	rs.mu.Lock()
	rs.res.TLS13SuitesComplete = complete
	rs.mu.Unlock()
}

// goProbedGroups are the groups crypto/tls already probes one by one.
func goProbedGroups() []uint16 {
	var ids []uint16
	for _, g := range append(append([]tls.CurveID{}, HybridGroups...), ClassicGroups...) {
		ids = append(ids, uint16(g))
	}
	return ids
}

// tls13Groups offers, with an empty key_share, the TLS 1.3 groups crypto/tls
// does not probe: a server that supports one names it in a HelloRetryRequest.
func (rs *rawScan) tls13Groups(ctx context.Context) {
	var remaining []uint16
	for _, g := range rawtls.Groups() {
		if g.TLS13 && !slices.Contains(goProbedGroups(), g.ID) {
			remaining = append(remaining, g.ID)
		}
	}
	var found []RawGroup
	for len(remaining) > 0 {
		h := &rawtls.Hello{Version: rawtls.VersionTLS12, SupportedVersions: []uint16{rawtls.VersionTLS13}, Suites: tls13SuiteIDs(),
			ServerName: rs.t.sni, Groups: remaining}
		o := rs.hello(ctx, probeTag{purpose: PurposeTLS13Groups}, h, false, func(sh *rawtls.ServerHello) (Status, error) {
			if !sh.HelloRetry { // a ServerHello without a key share we sent: cannot tell
				return ProbeFailed, &outcomeError{"no_hello_retry", "server answered without a HelloRetryRequest"}
			}
			return Supported, nil
		})
		switch o.status {
		case NotSupported:
			rs.setTLS13Groups(found, true)
			return
		case ProbeFailed:
			rs.setTLS13Groups(found, false)
			return
		}
		id := o.resp.ServerHello.KeyShareGroup
		found = append(found, rawGroup(id))
		remaining = slices.DeleteFunc(remaining, func(x uint16) bool { return x == id })
	}
	rs.setTLS13Groups(found, true)
}

func rawGroup(id uint16) RawGroup {
	g, _ := rawtls.GroupByID(id)
	return RawGroup{Name: rawtls.GroupName(id), ID: id, Kind: string(g.Kind)}
}

func (rs *rawScan) setTLS13Groups(found []RawGroup, complete bool) {
	rs.mu.Lock()
	rs.res.TLS13Groups, rs.res.TLS13GroupsComplete = found, complete
	rs.mu.Unlock()
}

// ecdheVersion is the highest TLS 1.0–1.2 version in which an ECDHE suite
// was accepted (by crypto/tls or a raw probe), or 0.
func (rs *rawScan) ecdheVersion(res *Result) uint16 {
	isECDHE := func(id uint16) bool {
		s, ok := rawtls.SuiteByID(id)
		return ok && s.Kx == "ECDHE"
	}
	best := uint16(0)
	for v, list := range map[uint16][]CipherSuite{rawtls.VersionTLS12: res.TLS12Suites, rawtls.VersionTLS11: res.TLS11Suites, rawtls.VersionTLS10: res.TLS10Suites} {
		for _, s := range list {
			if isECDHE(s.ID) {
				best = max(best, v)
			}
		}
	}
	for _, s := range rs.res.Suites {
		if isECDHE(s.ID) {
			best = max(best, versionByName[s.Version])
		}
	}
	return best
}

var versionByName = map[string]uint16{"TLS 1.0": rawtls.VersionTLS10, "TLS 1.1": rawtls.VersionTLS11, "TLS 1.2": rawtls.VersionTLS12}

// tls12Curves offers ECDHE suites with only the curves crypto/tls does not
// probe; the curve the server picks is in its ServerKeyExchange.
func (rs *rawScan) tls12Curves(ctx context.Context, v uint16) {
	var remaining []uint16
	for _, g := range rawtls.Groups() {
		if g.TLS12 && g.Kind != rawtls.GroupOther && !slices.Contains(goProbedGroups(), g.ID) {
			remaining = append(remaining, g.ID)
		}
	}
	var suites []uint16
	for _, s := range rawtls.Suites() {
		if s.Offerable && s.Kx == "ECDHE" && !s.TLS13 && s.MinVersion() <= v {
			suites = append(suites, s.ID)
		}
	}
	var found []RawGroup
	for len(remaining) > 0 {
		h := &rawtls.Hello{Version: v, Suites: suites, ServerName: rs.t.sni, Groups: remaining}
		o := rs.hello(ctx, probeTag{PurposeTLS12Curves, rawtls.VersionName(v)}, h, true, nil)
		switch o.status {
		case NotSupported:
			rs.setTLS12Curves(found, false, true)
			return
		case ProbeFailed:
			rs.setTLS12Curves(found, false, false)
			return
		}
		ke := o.resp.KeyExchange
		switch {
		case ke != nil && ke.ExplicitCurve:
			rs.setTLS12Curves(found, true, true)
			return
		case ke == nil || ke.Kind != "EC" || !slices.Contains(remaining, ke.Group):
			// No usable ServerKeyExchange, or a curve we did not offer: the
			// server ignores supported_groups, so we cannot tell.
			rs.setTLS12Curves(found, false, false)
			return
		}
		found = append(found, rawGroup(ke.Group))
		remaining = slices.DeleteFunc(remaining, func(x uint16) bool { return x == ke.Group })
	}
	rs.setTLS12Curves(found, false, true)
}

func (rs *rawScan) setTLS12Curves(found []RawGroup, explicit, complete bool) {
	rs.mu.Lock()
	rs.res.TLS12Curves, rs.res.ExplicitCurve, rs.res.TLS12CurvesComplete = found, explicit, complete
	rs.mu.Unlock()
}

// FirefoxSignatureSchemes is the signature_algorithms list of Firefox 157
// (captured on 26/10/2026, rawtls/testdata/clienthello): no ML-DSA.
var FirefoxSignatureSchemes = []uint16{0x0403, 0x0503, 0x0603, 0x0804, 0x0805, 0x0806, 0x0401, 0x0501, 0x0601, 0x0203, 0x0201}

// certificateProbeGroup is the group of the certificate probe's one key
// share: the one the browser-like offer got in TLS 1.3, or another group the
// server accepted, so that no HelloRetryRequest is needed.
func certificateProbeGroup(res *Result) (uint16, bool) {
	if id := res.Default.GroupID; id != 0 && res.Default.Version == "TLS 1.3" && rawtls.KeyShareSupported(id) {
		return id, true
	}
	for _, g := range res.KeyExchange.Groups {
		if g.Status == Supported && rawtls.KeyShareSupported(g.ID) {
			return g.ID, true
		}
	}
	return 0, false
}

// certificateWithoutMLDSA asks, in TLS 1.3 (the only version with ML-DSA),
// with the browser-like groups and Firefox's signature schemes, which
// certificate the server presents to a client without ML-DSA. The server
// refuses (alert, closed connection) only when it has no certificate it can
// use with those schemes.
func (rs *rawScan) certificateWithoutMLDSA(ctx context.Context, res *Result) {
	if versionStatusOf(res, tls.VersionTLS13) != Supported {
		return
	}
	tag := probeTag{PurposeCertificate, "without ML-DSA"}
	group, ok := certificateProbeGroup(res)
	if !ok {
		err := &outcomeError{"no_key_share", "no accepted group the probe can make a key share for"}
		rs.t.log.add(clientRaw, tag, time.Now(), nil, ProbeFailed, err, &ProbeResponse{Type: RespNotSent})
		rs.setCertificateWithout(nil, ProbeFailed, err)
		return
	}
	groups := uint16s(BrowserGroups)
	if !slices.Contains(groups, group) {
		groups = append(groups, group)
	}
	var f *rawtls.CertificateFlight
	rc, start, err := rs.exchange(ctx, func(c net.Conn) error {
		var err error
		f, err = rawtls.ExchangeCertificate(c, rawtls.CertificateProbe{ServerName: rs.t.sni, Suites: []uint16{0x1301, 0x1303, 0x1302},
			Groups: groups, KeyShareGroup: group, SignatureSchemes: FirefoxSignatureSchemes})
		return err
	})
	status := ProbeFailed
	var cert *PresentedCert
	switch {
	case err == nil && f != nil && len(f.Chain) > 0:
		status, cert = Supported, presented(f.Chain)
	case f != nil && f.Alert != nil:
		status, err = NotSupported, &alertError{*f.Alert}
	case errors.Is(err, io.EOF), errors.Is(err, syscall.ECONNRESET):
		status = NotSupported
	case err == nil:
		err = &outcomeError{"no_certificate", "the server sent no certificate"}
	}
	var resp *ProbeResponse
	if rc != nil {
		r := responseOf(rc.out, rc.in, err)
		r.Certificate = cert
		if f != nil {
			r.HandshakeSignature = f.SignatureScheme
			if f.Alert != nil && f.Encrypted { // decrypted: its level is known
				r.Alert = &ProbeAlert{Level: f.Alert.Level, Code: f.Alert.Description, Name: f.Alert.String(), Encrypted: true}
			}
		}
		resp = &r
	}
	rs.t.log.add(clientRaw, tag, start, rc, status, err, resp)
	rs.setCertificateWithout(cert, status, err)
}

// chromeOffer sends Chrome 156's ClientHello and records what the server
// picks, as Chrome would get it: version, suite and group (the key share in
// TLS 1.3, the ServerKeyExchange curve in TLS 1.2).
func (rs *rawScan) chromeOffer(ctx context.Context, res *Result) {
	var resp *rawtls.Response
	rc, start, err := rs.exchange(ctx, func(c net.Conn) error {
		var err error
		resp, err = rawtls.ExchangeChrome(c, rs.t.sni, rawtls.Options{ReadKeyExchange: true})
		return err
	})
	o := classifyRaw(resp, err)
	h := &Handshake{Status: o.status, Error: o.errString(), ErrorCode: errorCode(o.err)}
	retry := false
	if o.status == Supported {
		sh := o.resp.ServerHello
		h.Version, h.CipherSuite = tls.VersionName(sh.Version), tls.CipherSuiteName(sh.CipherSuite)
		group := sh.KeyShareGroup
		if ke := o.resp.KeyExchange; sh.Version <= rawtls.VersionTLS12 && ke != nil && ke.Kind == "EC" && !ke.ExplicitCurve {
			group = ke.Group
		}
		if group != 0 {
			h.Group, h.GroupID = groupName(tls.CurveID(group)), group
		}
		retry = sh.HelloRetry
		rs.noteChain(o.resp.Certificates, sh.Version)
	}
	rs.t.log.add(clientRaw, probeTag{purpose: PurposeChrome}, start, rc, o.status, o.err, nil)
	rs.mu.Lock()
	res.ChromeOffer, res.ChromeHelloRetry = h, retry
	rs.mu.Unlock()
}

func (rs *rawScan) setCertificateWithout(cert *PresentedCert, status Status, err error) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	sel := rs.sel
	if sel == nil {
		return
	}
	sel.WithoutMLDSA, sel.WithoutStatus, sel.WithoutError, sel.WithoutErrorCode = cert, status, errText(err), errorCode(err)
}

func uint16s(ids []tls.CurveID) []uint16 {
	out := make([]uint16, len(ids))
	for i, id := range ids {
		out[i] = uint16(id)
	}
	return out
}
