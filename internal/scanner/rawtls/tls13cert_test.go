package rawtls

import (
	"bytes"
	"crypto"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"math/big"
	"net"
	"slices"
	"testing"
	"time"
)

// Signature schemes as Chrome 156 and Firefox 157 offer them (testdata/clienthello).
var (
	chromeSchemes  = []uint16{0x0904, 0x0905, 0x0906, 0x0403, 0x0804, 0x0401, 0x0503, 0x0805, 0x0501, 0x0806, 0x0601}
	firefoxSchemes = []uint16{0x0403, 0x0503, 0x0603, 0x0804, 0x0805, 0x0806, 0x0401, 0x0501, 0x0601, 0x0203, 0x0201}
)

func certFor(t testing.TB, key crypto.Signer) tls.Certificate {
	t.Helper()
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: "rawtls.test"},
		DNSNames: []string{"rawtls.test"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// certServer runs a crypto/tls server with certs on a pipe.
func certServer(t testing.TB, certs []tls.Certificate, groups []tls.CurveID) net.Conn {
	t.Helper()
	client, server := net.Pipe()
	go func() {
		defer server.Close()
		_ = server.SetDeadline(time.Now().Add(5 * time.Second))
		_ = tls.Server(server, &tls.Config{Certificates: certs, CurvePreferences: groups, MinVersion: tls.VersionTLS13}).Handshake()
	}()
	t.Cleanup(func() { client.Close() })
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	return client
}

var allShareGroups = []tls.CurveID{tls.X25519, tls.CurveP256, tls.CurveP384, tls.CurveP521, tls.X25519MLKEM768, tls.SecP256r1MLKEM768, tls.SecP384r1MLKEM1024}

// Every key share and every suite: the decrypted leaf is the server's certificate.
func TestExchangeCertificate(t *testing.T) {
	cert := testCert(t)
	for _, g := range allShareGroups {
		for _, suite := range CertificateSuites {
			conn := certServer(t, []tls.Certificate{cert}, allShareGroups)
			f, err := ExchangeCertificate(conn, CertificateProbe{ServerName: "rawtls.test", Suites: []uint16{suite},
				Groups: []uint16{uint16(g)}, KeyShareGroup: uint16(g), SignatureSchemes: firefoxSchemes})
			if err != nil {
				t.Errorf("%s/0x%04x: %v", g, suite, err)
				continue
			}
			if len(f.Chain) != 1 || !bytes.Equal(f.Chain[0], cert.Certificate[0]) || f.SignatureScheme != 0x0403 ||
				f.ServerHello.CipherSuite != suite || f.ServerHello.KeyShareGroup != uint16(g) || f.Alert != nil {
				t.Errorf("%s/0x%04x: chain %d, scheme 0x%04x, %+v", g, suite, len(f.Chain), f.SignatureScheme, f.ServerHello)
			}
		}
	}
}

func TestExchangeCertificateRetryAndErrors(t *testing.T) {
	conn := certServer(t, []tls.Certificate{testCert(t)}, []tls.CurveID{tls.CurveP256})
	f, err := ExchangeCertificate(conn, CertificateProbe{Suites: CertificateSuites, Groups: []uint16{x25519, secp256r1}, KeyShareGroup: x25519, SignatureSchemes: firefoxSchemes})
	if !errors.Is(err, ErrRetry) || !f.ServerHello.HelloRetry || f.ServerHello.KeyShareGroup != secp256r1 {
		t.Errorf("retry: %+v, %v", f, err)
	}
	for name, p := range map[string]CertificateProbe{
		"CCM suite":           {Suites: []uint16{0x1304}, Groups: []uint16{x25519}, KeyShareGroup: x25519},
		"share not in groups": {Suites: CertificateSuites, Groups: []uint16{secp256r1}, KeyShareGroup: x25519},
		"unknown share":       {Suites: CertificateSuites, Groups: []uint16{0x0100}, KeyShareGroup: 0x0100},
	} {
		nowhere := struct {
			io.Reader
			io.Writer
		}{bytes.NewReader(nil), io.Discard}
		if _, err := ExchangeCertificate(nowhere, p); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// fakeServer answers the first ClientHello with a TLS 1.3 ServerHello built
// from the client's own offer, then with after.
func fakeServer(t *testing.T, share func() []byte, after []byte) net.Conn {
	t.Helper()
	client, server := net.Pipe()
	go func() {
		defer server.Close()
		_ = server.SetDeadline(time.Now().Add(5 * time.Second))
		buf := make([]byte, 1<<16)
		n, _ := server.Read(buf)
		ch, err := ParseClientHello(buf[:n])
		if err != nil {
			return
		}
		sh := serverHelloBody(shOpts{suite: ch.Suites[0], exts: [][2]any{{extSupportedVersions, u16(VersionTLS13)},
			{extKeyShare, append(u16(ch.KeyShares[0]), append(u16(uint16(len(share()))), share()...)...)}}})
		_, _ = server.Write(append(record(recordHandshake, hsMsg(typeServerHello, sh)), after...))
	}()
	t.Cleanup(func() { client.Close() })
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	return client
}

func TestExchangeCertificateBadFlights(t *testing.T) {
	k, _ := ecdh.X25519().GenerateKey(rand.Reader)
	good := func() []byte { return k.PublicKey().Bytes() }
	p := CertificateProbe{Suites: []uint16{0x1301}, Groups: []uint16{x25519}, KeyShareGroup: x25519, SignatureSchemes: firefoxSchemes}
	for name, c := range map[string]struct {
		share func() []byte
		after []byte
	}{
		"short key share":      {func() []byte { return []byte{1, 2, 3} }, nil},
		"undecryptable record": {good, record(recordApplicationData, make([]byte, 40))},
		"handshake in clear":   {good, record(recordHandshake, hsMsg(typeEncryptedExtensions, nil))},
		"cut":                  {good, record(recordApplicationData, make([]byte, 40))[:20]},
	} {
		f, err := ExchangeCertificate(fakeServer(t, c.share, c.after), p)
		if err == nil || !typedError(err) || f == nil || f.Chain != nil {
			t.Errorf("%s: %+v, %v", name, f, err)
		}
	}
	// An alert in clear after the ServerHello ends the flight with it.
	f, err := ExchangeCertificate(fakeServer(t, good, record(recordAlert, []byte{2, 40})), p)
	if err != nil || f.Alert == nil || f.Alert.Description != 40 || f.Encrypted {
		t.Errorf("alert: %+v, %v", f, err)
	}
}

func TestParseTLS13Certificate(t *testing.T) {
	entry := func(der []byte) []byte {
		return append(append([]byte{byte(len(der) >> 16), byte(len(der) >> 8), byte(len(der))}, der...), 0, 0)
	}
	list := append(entry([]byte{1, 2, 3}), entry([]byte{4})...)
	body := append([]byte{0, byte(len(list) >> 16), byte(len(list) >> 8), byte(len(list))}, list...)
	chain, err := ParseTLS13Certificate(body)
	if err != nil || len(chain) != 2 || !slices.Equal(chain[0], []byte{1, 2, 3}) || !slices.Equal(chain[1], []byte{4}) {
		t.Errorf("%v %v", chain, err)
	}
	for _, bad := range [][]byte{nil, body[:len(body)-1], append(body, 0)} {
		if _, err := ParseTLS13Certificate(bad); err == nil {
			t.Errorf("%x parsed", bad)
		}
	}
}

func FuzzParseTLS13Certificate(f *testing.F) {
	f.Add([]byte{0, 0, 0, 8, 0, 0, 3, 1, 2, 3, 0, 0})
	f.Fuzz(func(t *testing.T, body []byte) {
		chain, err := ParseTLS13Certificate(body)
		if err != nil {
			if !errors.Is(err, ErrMalformed) {
				t.Fatalf("untyped error %v", err)
			}
			return
		}
		total := 0
		for _, c := range chain {
			if len(c) == 0 {
				t.Fatal("empty certificate")
			}
			total += len(c)
		}
		if total > len(body) {
			t.Fatalf("%d bytes of certificates from %d", total, len(body))
		}
	})
}
