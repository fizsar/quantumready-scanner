package rawtls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"testing"
	"time"

	"golang.org/x/crypto/cryptobyte"
)

// testCert is a self-signed ECDSA P-256 certificate for crypto/tls servers.
func testCert(t testing.TB) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "rawtls.test"},
		DNSNames:     []string{"rawtls.test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// clientHelloInfo is what a crypto/tls server saw of our ClientHello.
type clientHelloInfo struct {
	serverName string
	versions   []uint16
	suites     []uint16
	groups     []tls.CurveID
	schemes    []tls.SignatureScheme
	extensions []uint16
}

// tlsServer runs a crypto/tls server on one end of a pipe and returns the
// other end plus a channel with what the server saw of the ClientHello.
func tlsServer(t testing.TB, cfg *tls.Config) (net.Conn, <-chan clientHelloInfo) {
	t.Helper()
	client, server := net.Pipe()
	seen := make(chan clientHelloInfo, 1)
	cfg = cfg.Clone()
	cfg.Certificates = []tls.Certificate{testCert(t)}
	cfg.GetConfigForClient = func(h *tls.ClientHelloInfo) (*tls.Config, error) {
		seen <- clientHelloInfo{h.ServerName, h.SupportedVersions, h.CipherSuites, h.SupportedCurves, h.SignatureSchemes, h.Extensions}
		return nil, nil
	}
	go func() {
		defer server.Close()
		_ = server.SetDeadline(time.Now().Add(5 * time.Second))
		_ = tls.Server(server, cfg).Handshake() // fails once we stop reading: expected
	}()
	t.Cleanup(func() { client.Close() })
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	return client, seen
}

// Server-side builders for handcrafted responses.

func record(typ uint8, payload []byte) []byte {
	return append([]byte{typ, 3, 3, byte(len(payload) >> 8), byte(len(payload))}, payload...)
}

func hsMsg(typ uint8, body []byte) []byte {
	n := len(body)
	return append([]byte{typ, byte(n >> 16), byte(n >> 8), byte(n)}, body...)
}

type shOpts struct {
	version     uint16 // legacy_version, default TLS 1.2
	suite       uint16
	compression uint8
	random      []byte // default zeros
	sessionID   []byte
	exts        [][2]any // {type uint16, data []byte}; nil: no extensions block
	noExtBlock  bool
}

func serverHelloBody(o shOpts) []byte {
	if o.version == 0 {
		o.version = VersionTLS12
	}
	var b cryptobyte.Builder
	b.AddUint16(o.version)
	r := make([]byte, 32)
	copy(r, o.random)
	b.AddBytes(r)
	b.AddUint8LengthPrefixed(func(b *cryptobyte.Builder) { b.AddBytes(o.sessionID) })
	b.AddUint16(o.suite)
	b.AddUint8(o.compression)
	if !o.noExtBlock {
		b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
			for _, e := range o.exts {
				b.AddUint16(e[0].(uint16))
				b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) { b.AddBytes(e[1].([]byte)) })
			}
		})
	}
	return b.BytesOrPanic()
}

func u16(v uint16) []byte { return []byte{byte(v >> 8), byte(v)} }

// keyShareExt is a ServerHello key_share with a dummy key.
func keyShareExt(group uint16) [2]any {
	return [2]any{extKeyShare, append(u16(group), 0, 4, 1, 2, 3, 4)}
}

// dhSKE is a ServerKeyExchange body with a DH prime of bits bits.
func dhSKE(bits int) []byte {
	p := make([]byte, (bits+7)/8+1) // leading zero byte, as some servers send
	p[1] = 0x80
	var b cryptobyte.Builder
	b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) { b.AddBytes(p) })
	b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) { b.AddUint8(2) })
	b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) { b.AddBytes([]byte{1, 2, 3}) })
	b.AddBytes([]byte{4, 1, 0, 2, 0xAA, 0xBB}) // signature, ignored
	return b.BytesOrPanic()
}
