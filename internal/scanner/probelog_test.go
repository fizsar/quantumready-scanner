package scanner

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"slices"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/fizsar/quantumready-saas/internal/scanner/rawtls"
)

func logged(r *Result, purpose, subject string) []ProbeRecord {
	var out []ProbeRecord
	for _, p := range r.ProbeLog.Probes {
		if p.Purpose == purpose && p.Subject == subject {
			out = append(out, p)
		}
	}
	return out
}

func offerOfRecord(t *testing.T, r *Result, p ProbeRecord) Offer {
	t.Helper()
	if p.Offer == nil {
		t.Fatalf("%s %s: no offer", p.Purpose, p.Subject)
	}
	o, ok := r.ProbeLog.Offer(*p.Offer)
	if !ok {
		t.Fatalf("%s %s: offer %d does not expand", p.Purpose, p.Subject, *p.Offer)
	}
	return o
}

func u16s(ids ...tls.CurveID) []uint16 {
	out := make([]uint16, len(ids))
	for i, id := range ids {
		out[i] = uint16(id)
	}
	return out
}

// The log holds one record per connection, with what went on the wire, and
// agrees with the result's own fields.
func TestProbeLogMatchesTheScan(t *testing.T) {
	pki := newTestPKI(t)
	port := startTLS(t, &tls.Config{Certificates: []tls.Certificate{pki.leaf(t, leafOpts{})},
		CurvePreferences: []tls.CurveID{tls.X25519MLKEM768, tls.X25519, tls.CurveP256}})
	for _, intensity := range []Intensity{IntensityBasic, IntensityFull} {
		t.Run(string(intensity), func(t *testing.T) {
			r := scan(t, newTestScanner(pki, Options{Intensity: intensity}), port)
			l := r.ProbeLog
			if r.SchemaVersion != 2 {
				t.Errorf("schema %d", r.SchemaVersion)
			}
			if l == nil || len(l.Probes) != r.Connections {
				t.Fatalf("%d records for %d connections", len(l.Probes), r.Connections)
			}
			if len(l.Offers) >= len(l.Probes) {
				t.Errorf("%d offers for %d probes: identical ClientHellos are not shared", len(l.Offers), len(l.Probes))
			}
			if !slices.ContainsFunc(l.Offers, func(o Offer) bool { return o.Like != nil }) {
				t.Error("no enumeration offer stored as a difference")
			}
			for i, p := range l.Probes {
				offerOfRecord(t, r, p) // every connection sent a ClientHello
				if i > 0 && p.StartMS < l.Probes[i-1].StartMS {
					t.Errorf("record %d starts before the previous one", i)
				}
				goOnly := slices.Contains([]string{PurposeDefault, PurposeGroup, PurposeVersion}, p.Purpose)
				rawOnly := slices.Contains([]string{PurposeSSLv2, PurposeSSLv3, PurposeTLS13Suites, PurposeTLS13Groups, PurposeTLS12Curves}, p.Purpose)
				if p.Client == clientRaw && (goOnly || intensity == IntensityBasic) || p.Client == clientCryptoTLS && rawOnly ||
					p.Client != clientRaw && p.Client != clientCryptoTLS {
					t.Errorf("%s probe by %q", p.Purpose, p.Client)
				}
			}

			// The browser-like offer, read from the bytes sent.
			def := logged(r, PurposeDefault, "")
			if len(def) != 1 {
				t.Fatalf("%d default probes", len(def))
			}
			o := offerOfRecord(t, r, def[0])
			if !slices.Equal(o.Groups, u16s(BrowserGroups...)) || !slices.Equal(o.KeyShares, u16s(tls.X25519MLKEM768, tls.X25519)) ||
				!slices.Equal(o.SupportedVersions, []uint16{tls.VersionTLS13, tls.VersionTLS12}) || !slices.Contains(o.Extensions, 0) {
				t.Errorf("default offer %+v", o)
			}
			if !slices.Contains(o.SignatureSchemes, 0x0904) {
				t.Errorf("Go 1.27 offers ML-DSA by default; offer %v", o.SignatureSchemes)
			}
			resp := def[0].Response
			if resp.Type != RespServerHello || resp.Version != tls.VersionTLS13 || resp.GroupName != r.Default.Group || resp.CipherSuiteName != r.Default.CipherSuite {
				t.Errorf("default response %+v, result %+v", resp, r.Default)
			}

			// Each group and version probe offered exactly what it tests, and
			// its record says what the result says.
			for _, g := range r.KeyExchange.Groups {
				recs := logged(r, PurposeGroup, g.Name)
				if len(recs) != 1 {
					t.Fatalf("%s: %d records", g.Name, len(recs))
				}
				p := recs[0]
				if o := offerOfRecord(t, r, p); !slices.Equal(o.Groups, []uint16{g.ID}) {
					t.Errorf("%s: offered groups %v", g.Name, o.Groups)
				}
				if p.Status != g.Status || p.ErrorCode != g.ErrorCode || p.Error != g.Error {
					t.Errorf("%s: record %s %q, result %s %q", g.Name, p.Status, p.ErrorCode, g.Status, g.ErrorCode)
				}
				if g.Status == Supported && p.Response.Group != g.ID {
					t.Errorf("%s: server chose 0x%04x", g.Name, p.Response.Group)
				}
			}
			for _, v := range r.Versions {
				p := logged(r, PurposeVersion, v.Version)[0]
				o := offerOfRecord(t, r, p)
				offered := o.SupportedVersions
				if offered == nil {
					offered = []uint16{o.LegacyVersion}
				}
				if !slices.Equal(offered, []uint16{v.ID}) {
					t.Errorf("%s: offered %v", v.Version, offered)
				}
				if p.Status != v.Status || p.ErrorCode != v.ErrorCode {
					t.Errorf("%s: record %s %q, result %s %q", v.Version, p.Status, p.ErrorCode, v.Status, v.ErrorCode)
				}
			}
			if intensity == IntensityFull {
				ssl2 := logged(r, PurposeSSLv2, "")
				if len(ssl2) != 1 || !offerOfRecord(t, r, ssl2[0]).SSLv2 || ssl2[0].Client != clientRaw {
					t.Errorf("SSLv2 probe %+v", ssl2)
				}
				if r.ProbeLog.Probes[len(r.ProbeLog.Probes)-1].Client != clientRaw {
					t.Error("raw probes run last")
				}
			}
			if _, err := json.Marshal(r); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// A HelloRetryRequest and the ServerHello after it, as the server sent them.
func TestProbeLogHelloRetry(t *testing.T) {
	pki := newTestPKI(t)
	port := startTLS(t, &tls.Config{Certificates: []tls.Certificate{pki.leaf(t, leafOpts{})}, CurvePreferences: []tls.CurveID{tls.CurveP256}})
	r := scan(t, newTestScanner(pki, Options{}), port)
	p := logged(r, PurposeDefault, "")[0]
	resp := p.Response
	// The second ClientHello, as sent: the same offer with the key share asked for.
	if p.RetryOffer == nil {
		t.Fatal("no second ClientHello logged")
	}
	first, second := offerOfRecord(t, r, p), r.ProbeLog.Offers[*p.RetryOffer]
	if !slices.Equal(second.KeyShares, []uint16{uint16(tls.CurveP256)}) || !slices.Equal(second.Groups, first.Groups) || slices.Equal(first.KeyShares, second.KeyShares) {
		t.Errorf("first %+v\nsecond %+v", first, second)
	}
	if resp.Type != RespHelloRetryRequest || resp.RetryGroup != uint16(tls.CurveP256) || resp.RetryGroupName != "secp256r1" ||
		resp.Version != tls.VersionTLS13 || resp.Group != uint16(tls.CurveP256) || resp.CipherSuiteName == "" {
		t.Errorf("%+v", resp)
	}
}

// An alert sent in clear: its level and code from the bytes, the stable code beside the error.
func TestProbeLogAlertInClear(t *testing.T) {
	pki := newTestPKI(t)
	port := startTLS(t, &tls.Config{Certificates: []tls.Certificate{pki.leaf(t, leafOpts{})}, MaxVersion: tls.VersionTLS12})
	r := scan(t, newTestScanner(pki, Options{}), port)
	p := logged(r, PurposeVersion, "TLS 1.3")[0]
	want := ProbeAlert{Level: 2, Code: 70, Name: "protocol_version"}
	if p.Response.Type != RespAlert || p.Response.Alert == nil || *p.Response.Alert != want || p.ErrorCode != "alert:70" || p.Status != NotSupported {
		t.Errorf("%+v", p)
	}
	for _, v := range r.Versions {
		if v.Version == "TLS 1.3" && v.ErrorCode != "alert:70" {
			t.Errorf("version probe error code %q", v.ErrorCode)
		}
	}
}

// An alert that came after encryption started: not in the bytes, but in the
// error crypto/tls returns, whose value is the alert itself.
func TestEncryptedAlertFromTheError(t *testing.T) {
	pki := newTestPKI(t)
	port := startTLS(t, &tls.Config{Certificates: []tls.Certificate{pki.leaf(t, leafOpts{})}, MaxVersion: tls.VersionTLS12})
	// A real crypto/tls error carrying a server alert (protocol_version).
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	herr := tls.Client(conn, &tls.Config{MinVersion: tls.VersionTLS13, InsecureSkipVerify: true}).Handshake()
	if code, ok := remoteAlert(herr); !ok || code != 70 {
		t.Fatalf("remoteAlert(%v) = %d, %v", herr, code, ok)
	}
	// The same error after a TLS 1.3 ServerHello and encrypted records.
	body := append(append([]byte{3, 3}, make([]byte, 32)...), 0, 0x13, 0x01, 0, 0, 16, 0, 43, 0, 2, 3, 4, 0, 51, 0, 6, 0, 0x1d, 0, 2, 1, 2) // 16 bytes of extensions
	sh := append([]byte{2, byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body))}, body...)
	in := append([]byte{22, 3, 3, byte(len(sh) >> 8), byte(len(sh))}, sh...)
	in = append(in, 23, 3, 3, 0, 19)
	in = append(in, make([]byte, 19)...)
	resp := responseOf([]byte{22}, in, herr)
	if resp.Type != RespServerHello || resp.Alert == nil || *resp.Alert != (ProbeAlert{Code: 70, Name: "protocol_version", Encrypted: true}) {
		t.Errorf("%+v alert %+v", resp, resp.Alert)
	}
}

// probeOnce runs one crypto/tls probe against 127.0.0.1:port and returns its record.
func probeOnce(t *testing.T, port int, timeout time.Duration) ProbeRecord {
	t.Helper()
	s := newTestScanner(nil, Options{HandshakeTimeout: timeout})
	tg := target{host: testHost, sni: testHost, addr: netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), uint16(port)),
		conns: new(atomic.Int32), log: newProbeLog()}
	s.handshake(context.Background(), tg, &tls.Config{MinVersion: tls.VersionTLS12}, probeTag{purpose: PurposeDefault}, nil)
	l := tg.log.result()
	if len(l.Probes) != 1 {
		t.Fatalf("%d records", len(l.Probes))
	}
	return l.Probes[0]
}

// serve accepts connections and handles each with fn.
func serve(t *testing.T, fn func(net.Conn)) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(3 * time.Second))
				fn(c)
			}()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestProbeLogWithoutAnswer(t *testing.T) {
	readHello := func(c net.Conn) { _, _ = c.Read(make([]byte, 4096)) }
	closed, _ := net.Listen("tcp", "127.0.0.1:0")
	refused := closed.Addr().(*net.TCPAddr).Port
	closed.Close()
	for name, c := range map[string]struct {
		port     int
		typ      string
		code     string
		hasOffer bool
	}{
		"silent":  {silentServer(t), RespTimeout, "timeout", true},
		"closes":  {serve(t, readHello), RespClosed, "closed", true},
		"http":    {serve(t, func(c net.Conn) { readHello(c); _, _ = io.WriteString(c, "HTTP/1.1 400 Bad Request\r\n\r\n") }), RespNotTLS, "not_tls", true},
		"refused": {refused, RespNoConnection, "connection_refused", false},
	} {
		t.Run(name, func(t *testing.T) {
			p := probeOnce(t, c.port, 300*time.Millisecond)
			if p.Response.Type != c.typ || p.ErrorCode != c.code || (p.Offer != nil) != c.hasOffer || p.Status != classifyStatusOf(c.code) {
				t.Errorf("%+v", p)
			}
		})
	}
}

// classifyStatusOf is the status classify gives these cases.
func classifyStatusOf(code string) Status {
	if code == "closed" {
		return NotSupported
	}
	return ProbeFailed
}

// What the budget refuses is logged as not sent.
func TestProbeLogBudget(t *testing.T) {
	pki := newTestPKI(t)
	port := startTLS(t, &tls.Config{Certificates: []tls.Certificate{pki.leaf(t, leafOpts{})}})
	r := scan(t, newTestScanner(pki, Options{Intensity: IntensityFull, RawProbeBudget: 2}), port)
	notSent := 0
	for _, p := range r.ProbeLog.Probes {
		if p.Response.Type == RespNotSent {
			notSent++
			if p.ErrorCode != "budget" || p.Offer != nil || p.Client != clientRaw {
				t.Errorf("%+v", p)
			}
		}
	}
	if notSent == 0 || len(r.ProbeLog.Probes) != r.Connections+notSent {
		t.Errorf("%d not sent, %d records, %d connections", notSent, len(r.ProbeLog.Probes), r.Connections)
	}
}

func TestErrorCode(t *testing.T) {
	timeoutErr := &net.OpError{Op: "read", Err: &timeoutError{}}
	for _, c := range []struct {
		err  error
		want string
	}{
		{nil, ""},
		{errBudget, "budget"},
		{&connectError{fmt.Errorf("%w: 10.0.0.1 is private", ErrBlockedAddress)}, "blocked_address"},
		{&connectError{&net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}}, "connection_refused"},
		{&connectError{&net.OpError{Op: "dial", Err: syscall.EHOSTUNREACH}}, "unreachable"},
		{&connectError{errors.New("dial: other")}, "connect_error"},
		{timeoutErr, "timeout"},
		{context.DeadlineExceeded, "timeout"},
		{context.Canceled, "canceled"},
		{io.EOF, "closed"},
		{fmt.Errorf("read: %w", syscall.ECONNRESET), "reset"},
		{&alertError{rawtls.Alert{Level: 2, Description: 40}}, "alert:40"},
		{&rawtls.UnofferedError{What: "cipher suite", Value: 5}, "unoffered_cipher_suite"},
		{&rawtls.UnofferedError{What: "SSLv2 cipher", Value: 5}, "unoffered_cipher"},
		{fmt.Errorf("%w: short ServerHello", rawtls.ErrMalformed), "malformed"},
		{rawtls.ErrLimit, "read_limit"},
		{&outcomeError{"other_group", "server negotiated x25519 instead"}, "other_group"},
		{errors.New("tls: server selected unsupported protocol version 300"), "unoffered_version"},
		{errors.New("tls: server chose an unconfigured cipher suite"), "unoffered_cipher_suite"},
		{errors.New("tls: first record does not look like a TLS handshake"), "not_tls"},
		{errors.New("tls: something new in Go 1.28"), "tls_error"},
		{errors.New("boom"), "error"},
	} {
		if got := errorCode(c.err); got != c.want {
			t.Errorf("errorCode(%v) = %q, want %q", c.err, got, c.want)
		}
	}
	// Texts are what they were before schema 2.
	if (&alertError{rawtls.Alert{Level: 2, Description: 40}}).Error() != "alert handshake_failure" ||
		(&connectError{errors.New("x")}).Error() != "connect: x" {
		t.Error("error texts changed")
	}
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func TestTrustErrorCode(t *testing.T) {
	pki := newTestPKI(t)
	verify := func(o leafOpts, roots *x509.CertPool) error {
		c := pki.leaf(t, o)
		leaf, _ := x509.ParseCertificate(c.Certificate[0])
		_, err := leaf.Verify(x509.VerifyOptions{Roots: roots, CurrentTime: testNow})
		return err
	}
	for name, c := range map[string]struct {
		err  error
		want string
	}{
		"ok":            {verify(leafOpts{}, pki.pool), ""},
		"unknown CA":    {verify(leafOpts{}, x509.NewCertPool()), "unknown_authority"},
		"expired":       {verify(leafOpts{notAfter: testNow.AddDate(0, 0, -1)}, pki.pool), "expired"},
		"not yet valid": {verify(leafOpts{notBefore: testNow.AddDate(0, 0, 1)}, pki.pool), "expired"},
		"hostname":      {x509.HostnameError{Host: "x"}, "hostname_mismatch"},
		"other":         {errors.New("x"), "invalid"},
	} {
		if got := trustErrorCode(c.err); got != c.want {
			t.Errorf("%s: %q, want %q (%v)", name, got, c.want, c.err)
		}
	}
}

func TestAlgorithmOIDs(t *testing.T) {
	pki := newTestPKI(t)
	for key, want := range map[string][2]string{
		"p256":    {"1.2.840.10045.4.3.2", "1.2.840.10045.2.1"}, // ecdsa-with-SHA256 (signed by the P-256 CA), id-ecPublicKey
		"rsa2048": {"1.2.840.10045.4.3.2", "1.2.840.113549.1.1.1"},
	} {
		c := pki.leaf(t, leafOpts{key: key})
		leaf, _ := x509.ParseCertificate(c.Certificate[0])
		if sig, k := algorithmOIDs(leaf); sig != want[0] || k != want[1] {
			t.Errorf("%s: %s %s, want %v", key, sig, k, want)
		}
	}
	if sig, k := algorithmOIDs(&x509.Certificate{}); sig != "" || k != "" {
		t.Errorf("empty certificate: %q %q", sig, k)
	}
}

// Offers stored as differences expand to exactly what was sent.
func TestOfferDifferences(t *testing.T) {
	grease := uint16(0x0a0a)
	full := Offer{LegacyVersion: 0x0303, Suites: []uint16{1, 2, 3, 4, 5}, Groups: []uint16{29, 23, 24}, SignatureSchemes: []uint16{0x0403}, Extensions: []uint16{0, 10, 13}}
	without := func(o Offer, suites, groups []uint16) Offer {
		o.Suites = slices.DeleteFunc(slices.Clone(o.Suites), func(id uint16) bool { return slices.Contains(suites, id) })
		o.Groups = slices.DeleteFunc(slices.Clone(o.Groups), func(id uint16) bool { return slices.Contains(groups, id) })
		return o
	}
	other := without(full, []uint16{1, 3, 5}, nil) // its suites follow offer 2's, but…
	other.LegacyVersion = 0x0302                   // …another field differs
	greased := full
	greased.Suites = []uint16{grease, 1, 2, grease}
	l := &probeLog{start: time.Now(), offers: []Offer{
		full,                               // 0
		without(full, []uint16{3}, nil),    // 1: one suite found
		without(full, []uint16{3, 5}, nil), // 2: then another
		without(full, nil, []uint16{23}),   // 3: another enumeration (groups)
		other,                              // 4: in full
		greased,                            // 5
		without(greased, []uint16{2}, nil), // 6: repeated GREASE: in full
		{LegacyVersion: 0x0303, Suites: []uint16{1, 2, 3, 4, 5, 6}, Groups: full.Groups, SignatureSchemes: full.SignatureSchemes, Extensions: full.Extensions}, // 7: a suite added: in full
	}}
	tags := []string{"a", "a", "a", "b", "a", "c", "c", "a"}
	for i := range l.offers {
		l.probes = append(l.probes, ProbeRecord{Client: clientRaw, Purpose: tags[i], Offer: &i, at: l.start.Add(time.Duration(i) * time.Millisecond)})
	}
	r := l.result()
	abbreviated := 0
	for i, p := range r.Probes {
		got, ok := r.Offer(*p.Offer)
		want := l.offers[i]
		gj, _ := json.Marshal(got)
		wj, _ := json.Marshal(want)
		if !ok || string(gj) != string(wj) {
			t.Errorf("probe %d: %s, want %s", i, gj, wj)
		}
		if r.Offers[*p.Offer].Like != nil {
			abbreviated++
		}
	}
	if abbreviated != 2 { // offers 1 and 2; not 3 (other enumeration), 4, 6 (GREASE) or 7
		t.Errorf("%d offers abbreviated, want 2: %+v", abbreviated, r.Offers)
	}
	if _, ok := (&ProbeLog{Offers: []Offer{{Like: new(int)}}}).Offer(0); ok {
		t.Error("an offer like itself expanded")
	}
}

// sh13 is a TLS 1.3 ServerHello record (a HelloRetryRequest when hrr).
func sh13(hrr bool, suite, group uint16) []byte {
	random := make([]byte, 32)
	if hrr { // SHA-256("HelloRetryRequest"), RFC 8446 4.1.3
		random = []byte{0xCF, 0x21, 0xAD, 0x74, 0xE5, 0x9A, 0x61, 0x11, 0xBE, 0x1D, 0x8C, 0x02, 0x1E, 0x65, 0xB8, 0x91,
			0xC2, 0xA2, 0x11, 0x16, 0x7A, 0xBB, 0x8C, 0x5E, 0x07, 0x9E, 0x09, 0xE2, 0xC8, 0xA8, 0x33, 0x9C}
	}
	ks := []byte{0, 51, 0, 2, byte(group >> 8), byte(group)}
	if !hrr {
		ks = []byte{0, 51, 0, 6, byte(group >> 8), byte(group), 0, 2, 1, 2}
	}
	exts := append([]byte{0, 43, 0, 2, 3, 4}, ks...)
	body := append(append([]byte{3, 3}, random...), 0, byte(suite>>8), byte(suite), 0, 0, byte(len(exts)))
	body = append(body, exts...)
	msg := append([]byte{2, 0, byte(len(body) >> 8), byte(len(body))}, body...)
	return append([]byte{22, 3, 3, byte(len(msg) >> 8), byte(len(msg))}, msg...)
}

// After a HelloRetryRequest, the parameters are those of the ServerHello
// that followed, not the request's (a server that changed its mind shows).
func TestResponseAfterHelloRetry(t *testing.T) {
	in := append(sh13(true, 0x1301, 0x0017), sh13(false, 0x1302, 0x001d)...)
	r := responseOf([]byte{22}, in, nil)
	if r.Type != RespHelloRetryRequest || r.RetryGroup != 0x0017 || r.Group != 0x001d || r.CipherSuite != 0x1302 || r.GroupName != "x25519" {
		t.Errorf("%+v", r)
	}
	// Without the ServerHello after it: the request's suite and version only.
	r = responseOf([]byte{22}, sh13(true, 0x1301, 0x0017), nil)
	if r.Type != RespHelloRetryRequest || r.RetryGroup != 0x0017 || r.Group != 0 || r.CipherSuite != 0x1301 {
		t.Errorf("%+v", r)
	}
}

// The certificate's error code and the chain's OIDs reach the result.
func TestCertificateCodesInTheResult(t *testing.T) {
	pki := newTestPKI(t)
	port := startTLS(t, &tls.Config{Certificates: []tls.Certificate{pki.leaf(t, leafOpts{})}})
	r := scan(t, newTestScanner(pki, Options{RootCAs: x509.NewCertPool()}), port)
	c := r.Certificate
	if c.Trusted || c.TrustErrorCode != "unknown_authority" || c.TrustError == "" {
		t.Errorf("trusted %v, code %q, error %q", c.Trusted, c.TrustErrorCode, c.TrustError)
	}
	if c.Chain[0].SignatureOID != "1.2.840.10045.4.3.2" || c.Chain[0].PublicKeyOID != "1.2.840.10045.2.1" {
		t.Errorf("chain OIDs %+v", c.Chain[0])
	}
}
