package scanner

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"
)

const testHost = "example.test"

// testNow is the fixed scan time used by the tests.
var testNow = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

type testPKI struct {
	ca    *x509.Certificate
	caKey crypto.Signer
	pool  *x509.CertPool
}

func newTestPKI(t *testing.T) *testPKI {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "QuantumReady Test CA", Organization: []string{"QR Test"}},
		NotBefore:             testNow.AddDate(-1, 0, 0),
		NotAfter:              testNow.AddDate(5, 0, 0),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	ca, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	return &testPKI{ca: ca, caKey: key, pool: pool}
}

type leafOpts struct {
	dns       []string // default: testHost
	key       string   // "p256" (default), "rsa2048", "rsa1024"
	notBefore time.Time
	notAfter  time.Time
	scts      int // embedded SCTs (fake entries)
}

// sctListExtension builds an SCT list extension with n dummy entries.
func sctListExtension(t *testing.T, n int) pkix.Extension {
	t.Helper()
	var entries []byte
	for i := range n {
		sct := bytes.Repeat([]byte{byte(i + 1)}, 40)
		entries = binary.BigEndian.AppendUint16(entries, uint16(len(sct)))
		entries = append(entries, sct...)
	}
	list := binary.BigEndian.AppendUint16(nil, uint16(len(entries)))
	list = append(list, entries...)
	val, err := asn1.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	return pkix.Extension{Id: oidSCTList, Value: val}
}

func (p *testPKI) leaf(t *testing.T, o leafOpts) tls.Certificate {
	t.Helper()
	if o.dns == nil {
		o.dns = []string{testHost}
	}
	if o.notBefore.IsZero() {
		o.notBefore = testNow.AddDate(0, 0, -10)
	}
	if o.notAfter.IsZero() {
		o.notAfter = testNow.AddDate(0, 0, 80)
	}
	var key crypto.Signer
	var err error
	switch o.key {
	case "", "p256":
		key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case "rsa2048":
		key, err = rsa.GenerateKey(rand.Reader, 2048)
	case "rsa1024":
		key, err = rsa.GenerateKey(rand.Reader, 1024)
	default:
		t.Fatalf("unknown key %q", o.key)
	}
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: o.dns[0]},
		DNSNames:     o.dns,
		NotBefore:    o.notBefore,
		NotAfter:     o.notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if o.scts > 0 {
		tmpl.ExtraExtensions = []pkix.Extension{sctListExtension(t, o.scts)}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, p.ca, key.Public(), p.caKey)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der, p.ca.Raw}, PrivateKey: key}
}

// startTLS starts a TLS server on 127.0.0.1 with cfg (Certificates required)
// and returns its port.
func startTLS(t *testing.T, cfg *tls.Config) int {
	t.Helper()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Config.ErrorLog = log.New(io.Discard, "", 0) // failed probes are expected
	srv.TLS = cfg
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv.Listener.Addr().(*net.TCPAddr).Port
}

// fakeResolver answers from a fixed table, one answer per call (the last one
// repeats), and counts lookups.
type fakeResolver struct {
	answers map[string][][]netip.Addr
	calls   atomic.Int32
}

func (r *fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	n := int(r.calls.Add(1)) - 1
	a, ok := r.answers[host]
	if !ok {
		return nil, fmt.Errorf("no such host %s", host)
	}
	if n >= len(a) {
		n = len(a) - 1
	}
	return a[n], nil
}

func resolverFor(host string, addrs ...string) *fakeResolver {
	var as []netip.Addr
	for _, s := range addrs {
		as = append(as, netip.MustParseAddr(s))
	}
	return &fakeResolver{answers: map[string][][]netip.Addr{host: {as}}}
}

// loopbackOnly is the test AddrPolicy: test servers live on 127.0.0.1, which
// the default policy (rightly) refuses.
func loopbackOnly(a netip.Addr) error {
	if a == netip.MustParseAddr("127.0.0.1") {
		return nil
	}
	return CheckAddr(a)
}

func newTestScanner(pki *testPKI, opts Options) *Scanner {
	if opts.Resolver == nil {
		opts.Resolver = resolverFor(testHost, "127.0.0.1")
	}
	if opts.AddrPolicy == nil {
		opts.AddrPolicy = loopbackOnly
	}
	if opts.RootCAs == nil && pki != nil {
		opts.RootCAs = pki.pool
	}
	if opts.Now == nil {
		opts.Now = func() time.Time { return testNow }
	}
	if opts.HandshakeTimeout == 0 {
		opts.HandshakeTimeout = 2 * time.Second
	}
	return New(opts)
}

func groupStatus(r *Result, name string) Status {
	for _, g := range r.KeyExchange.Groups {
		if g.Name == name {
			return g.Status
		}
	}
	return ""
}

func versionStatus(r *Result, v string) Status {
	for _, p := range r.Versions {
		if p.Version == v {
			return p.Status
		}
	}
	return ""
}

func suiteNames(r *Result) map[string]CipherSuite {
	m := map[string]CipherSuite{}
	for _, s := range r.TLS12Suites {
		m[s.Name] = s
	}
	return m
}
