package rawtls

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

// A broad TLS 1.2 offer, as used by the suite enumeration.
var tls12Hello = &Hello{Version: VersionTLS12, Suites: []uint16{dheRSAAES128GCM, ecdheRSAAES128GCM, 0x0033, 0x0004, 0x0003, 0xC013}, Groups: []uint16{x25519, secp256r1}}

var tls13Hello = &Hello{Version: VersionTLS12, SupportedVersions: []uint16{VersionTLS13}, Suites: []uint16{aes128GCMSHA256, aes128CCMSHA256},
	Groups: []uint16{x25519, 0x0100}, KeyShares: []KeyShare{{Group: x25519, Data: make([]byte, 32)}}}

func sh12(suite uint16) []byte {
	return hsMsg(typeServerHello, serverHelloBody(shOpts{suite: suite}))
}

func TestReadResponse(t *testing.T) {
	cert := hsMsg(typeCertificate, make([]byte, 300))
	done := hsMsg(typeServerHelloDone, nil)
	for _, tc := range []struct {
		name    string
		h       *Hello
		stream  []byte
		readKX  bool
		check   func(*Response) bool
		wantErr error
	}{
		{"alert", tls12Hello, record(recordAlert, []byte{2, 40}), false,
			func(r *Response) bool { return r.Alert.String() == "handshake_failure" && r.ServerHello == nil }, nil},
		{"ServerHello", tls12Hello, record(recordHandshake, sh12(ecdheRSAAES128GCM)), false,
			func(r *Response) bool { return r.ServerHello.CipherSuite == ecdheRSAAES128GCM && r.KeyExchange == nil }, nil},
		{"ServerHello across two records", tls12Hello,
			append(record(recordHandshake, sh12(0x0033)[:10]), record(recordHandshake, sh12(0x0033)[10:])...), false,
			func(r *Response) bool { return r.ServerHello.CipherSuite == 0x0033 }, nil},
		{"DHE: prime size from ServerKeyExchange", tls12Hello,
			record(recordHandshake, bytes.Join([][]byte{sh12(dheRSAAES128GCM), cert, hsMsg(typeServerKeyExchange, dhSKE(1024)), done}, nil)), true,
			func(r *Response) bool { return r.KeyExchange.Kind == "DH" && r.KeyExchange.DHPrimeBits == 1024 }, nil},
		{"DHE with OCSP staple, 2048 bits", tls12Hello,
			record(recordHandshake, bytes.Join([][]byte{sh12(0x0033), cert, hsMsg(typeCertificateStatus, []byte{1, 0, 0, 1, 9}), hsMsg(typeServerKeyExchange, dhSKE(2048))}, nil)), true,
			func(r *Response) bool { return r.KeyExchange.DHPrimeBits == 2048 }, nil},
		{"ECDHE named curve", tls12Hello,
			record(recordHandshake, bytes.Join([][]byte{sh12(0xC013), cert, hsMsg(typeServerKeyExchange, []byte{3, 0, 0x17, 2, 4, 1, 9, 9})}, nil)), true,
			func(r *Response) bool { return r.KeyExchange.Kind == "EC" && r.KeyExchange.Group == secp256r1 }, nil},
		{"ECDHE explicit curve", tls12Hello,
			record(recordHandshake, bytes.Join([][]byte{sh12(0xC013), cert, hsMsg(typeServerKeyExchange, []byte{1, 9, 9})}, nil)), true,
			func(r *Response) bool { return r.KeyExchange.ExplicitCurve }, nil},
		{"RSA export temporary key", tls12Hello,
			record(recordHandshake, bytes.Join([][]byte{sh12(0x0003), cert, hsMsg(typeServerKeyExchange, append(append(u16(64), append([]byte{0xFF}, make([]byte, 63)...)...), 0, 1, 3))}, nil)), true,
			func(r *Response) bool { return r.KeyExchange.Kind == "RSA" && r.KeyExchange.RSAModulusBits == 512 }, nil},
		{"static RSA: no ServerKeyExchange", tls12Hello,
			record(recordHandshake, bytes.Join([][]byte{sh12(0x0004), cert, done}, nil)), true,
			func(r *Response) bool { return r.KeyExchange == nil && r.ServerHello.CipherSuite == 0x0004 }, nil},
		{"alert after ServerHello", tls12Hello,
			append(record(recordHandshake, sh12(dheRSAAES128GCM)), record(recordAlert, []byte{2, 80})...), true,
			func(r *Response) bool { return r.Alert.Description == 80 && r.ServerHello != nil }, nil},
		{"TLS 1.3 ServerHello", tls13Hello,
			record(recordHandshake, hsMsg(typeServerHello, serverHelloBody(shOpts{suite: aes128CCMSHA256,
				exts: [][2]any{{extSupportedVersions, u16(VersionTLS13)}, keyShareExt(x25519)}}))), true,
			func(r *Response) bool {
				return r.ServerHello.Version == VersionTLS13 && r.ServerHello.KeyShareGroup == x25519 && r.KeyExchange == nil
			}, nil},
		{"HelloRetryRequest, then ChangeCipherSpec", tls13Hello,
			append(record(recordHandshake, hsMsg(typeServerHello, serverHelloBody(shOpts{suite: aes128GCMSHA256, random: helloRetryRandom[:],
				exts: [][2]any{{extSupportedVersions, u16(VersionTLS13)}, {extKeyShare, u16(0x0100)}}}))), record(recordChangeCipherSpec, []byte{1})...), false,
			func(r *Response) bool { return r.ServerHello.HelloRetry && r.ServerHello.KeyShareGroup == 0x0100 }, nil},
		{"ChangeCipherSpec before ServerHello is ignored", tls12Hello,
			append(record(recordChangeCipherSpec, []byte{1}), record(recordHandshake, sh12(0x0033))...), false,
			func(r *Response) bool { return r.ServerHello.CipherSuite == 0x0033 }, nil},
		{"downgrade sentinel", tls12Hello,
			record(recordHandshake, hsMsg(typeServerHello, serverHelloBody(shOpts{suite: 0x0033, random: append(make([]byte, 24), "DOWNGRD\x01"...)}))), false,
			func(r *Response) bool { return r.ServerHello.DowngradeSentinel }, nil},
		{"TLS 1.0 answer to a TLS 1.2 offer", tls12Hello,
			record(recordHandshake, hsMsg(typeServerHello, serverHelloBody(shOpts{version: VersionTLS10, suite: 0x0033, noExtBlock: true}))), false,
			func(r *Response) bool {
				return r.ServerHello.Version == VersionTLS10 && r.ServerHello.Extensions == nil
			}, nil},

		// Malformed: never "not supported".
		{"empty stream", tls12Hello, nil, false, nil, io.EOF},
		{"truncated record", tls12Hello, record(recordHandshake, sh12(0x0033))[:20], false, nil, io.ErrUnexpectedEOF},
		{"zero-length record", tls12Hello, []byte{22, 3, 3, 0, 0}, false, nil, ErrMalformed},
		{"oversized record", tls12Hello, []byte{22, 3, 3, 0x48, 0x01}, false, nil, ErrMalformed},
		{"record version 2.0", tls12Hello, []byte{22, 2, 0, 0, 1, 0}, false, nil, ErrMalformed},
		{"application data", tls12Hello, record(23, []byte{1}), false, nil, ErrMalformed},
		{"SSLv2-style answer", tls12Hello, []byte{0x80, 0x03, 0x00, 0x00, 0x01}, false, nil, ErrMalformed},
		{"alert of 3 bytes", tls12Hello, record(recordAlert, []byte{2, 40, 0}), false, nil, ErrMalformed},
		{"alert inside a message", tls12Hello, append(record(recordHandshake, sh12(0x0033)[:10]), record(recordAlert, []byte{2, 40})...), false, nil, ErrMalformed},
		{"Certificate before ServerHello", tls12Hello, record(recordHandshake, cert), false, nil, ErrMalformed},
		{"huge declared message", tls12Hello, record(recordHandshake, []byte{2, 0xFF, 0xFF, 0xFF}), false, nil, ErrMalformed},
		{"short ServerHello", tls12Hello, record(recordHandshake, hsMsg(typeServerHello, []byte{3, 3, 1})), false, nil, ErrMalformed},
		{"trailing bytes after extensions", tls12Hello,
			record(recordHandshake, hsMsg(typeServerHello, append(serverHelloBody(shOpts{suite: 0x0033}), 0))), false, nil, ErrMalformed},
		{"duplicate extension", tls12Hello,
			record(recordHandshake, hsMsg(typeServerHello, serverHelloBody(shOpts{suite: 0x0033, exts: [][2]any{{uint16(0xff01), []byte{0}}, {uint16(0xff01), []byte{0}}}}))), false, nil, ErrMalformed},
		{"session id of 33 bytes", tls12Hello,
			record(recordHandshake, hsMsg(typeServerHello, serverHelloBody(shOpts{suite: 0x0033, sessionID: make([]byte, 33)}))), false, nil, ErrMalformed},
		{"bad supported_versions", tls13Hello,
			record(recordHandshake, hsMsg(typeServerHello, serverHelloBody(shOpts{suite: aes128GCMSHA256, exts: [][2]any{{extSupportedVersions, []byte{3, 4, 0}}}}))), false, nil, ErrMalformed},
		{"TLS 1.3 without key_share", tls13Hello,
			record(recordHandshake, hsMsg(typeServerHello, serverHelloBody(shOpts{suite: aes128GCMSHA256, exts: [][2]any{{extSupportedVersions, u16(VersionTLS13)}}}))), false, nil, ErrMalformed},
		{"empty key_share key", tls13Hello,
			record(recordHandshake, hsMsg(typeServerHello, serverHelloBody(shOpts{suite: aes128GCMSHA256, exts: [][2]any{{extSupportedVersions, u16(VersionTLS13)}, {extKeyShare, []byte{0, 0x1d, 0, 0}}}}))), false, nil, ErrMalformed},
		{"HelloRetryRequest without supported_versions", tls13Hello,
			record(recordHandshake, hsMsg(typeServerHello, serverHelloBody(shOpts{suite: aes128GCMSHA256, random: helloRetryRandom[:], exts: [][2]any{{extKeyShare, u16(x25519)}}}))), false, nil, ErrMalformed},
		{"bad DH parameters", tls12Hello,
			record(recordHandshake, bytes.Join([][]byte{sh12(0x0033), hsMsg(typeServerKeyExchange, []byte{0, 0})}, nil)), true, nil, ErrMalformed},
		{"EC curve type 7", tls12Hello,
			record(recordHandshake, bytes.Join([][]byte{sh12(0xC013), hsMsg(typeServerKeyExchange, []byte{7})}, nil)), true, nil, ErrMalformed},
		{"ServerKeyExchange for static RSA", tls12Hello,
			record(recordHandshake, bytes.Join([][]byte{sh12(0x0004), hsMsg(typeServerKeyExchange, dhSKE(1024))}, nil)), true, nil, ErrMalformed},
		{"Finished in the first flight", tls12Hello,
			record(recordHandshake, bytes.Join([][]byte{sh12(0x0033), hsMsg(20, make([]byte, 12))}, nil)), true, nil, ErrMalformed},
		{"too many messages", tls12Hello,
			record(recordHandshake, bytes.Join([][]byte{sh12(0x0033), cert, cert, cert, cert, cert, cert}, nil)), true, nil, ErrMalformed},

		// The server picked something we did not offer.
		{"suite not offered", tls12Hello, record(recordHandshake, sh12(0x002F)), false, nil, ErrUnoffered},
		{"SCSV selected", tls12Hello, record(recordHandshake, sh12(SCSVRenegotiation)), false, nil, ErrUnoffered},
		{"higher version than offered", &Hello{Version: VersionTLS10, Suites: []uint16{0x0033}},
			record(recordHandshake, sh12(0x0033)), false, nil, ErrUnoffered},
		{"TLS 1.2 suite in TLS 1.0", tls12Hello,
			record(recordHandshake, hsMsg(typeServerHello, serverHelloBody(shOpts{version: VersionTLS10, suite: dheRSAAES128GCM}))), false, nil, ErrUnoffered},
		{"compression", tls12Hello,
			record(recordHandshake, hsMsg(typeServerHello, serverHelloBody(shOpts{suite: 0x0033, compression: 1}))), false, nil, ErrUnoffered},
		{"supported_versions not offered", tls12Hello,
			record(recordHandshake, hsMsg(typeServerHello, serverHelloBody(shOpts{suite: 0x0033, exts: [][2]any{{extSupportedVersions, u16(VersionTLS12)}}}))), false, nil, ErrUnoffered},
		{"TLS 1.2 answer to a TLS 1.3-only offer", tls13Hello,
			record(recordHandshake, hsMsg(typeServerHello, serverHelloBody(shOpts{suite: aes128GCMSHA256}))), false, nil, ErrUnoffered},
		{"TLS 1.2 suite in TLS 1.3", &Hello{Version: VersionTLS12, SupportedVersions: []uint16{VersionTLS13}, Suites: []uint16{aes128GCMSHA256, ecdheRSAAES128GCM}, KeyShares: tls13Hello.KeyShares},
			record(recordHandshake, hsMsg(typeServerHello, serverHelloBody(shOpts{suite: ecdheRSAAES128GCM, exts: [][2]any{{extSupportedVersions, u16(VersionTLS13)}, keyShareExt(x25519)}}))), false, nil, ErrUnoffered},
		{"key share for a group we sent no share for", tls13Hello,
			record(recordHandshake, hsMsg(typeServerHello, serverHelloBody(shOpts{suite: aes128GCMSHA256, exts: [][2]any{{extSupportedVersions, u16(VersionTLS13)}, keyShareExt(0x0100)}}))), false, nil, ErrUnoffered},
		{"HelloRetryRequest for a group not offered", tls13Hello,
			record(recordHandshake, hsMsg(typeServerHello, serverHelloBody(shOpts{suite: aes128GCMSHA256, random: helloRetryRandom[:], exts: [][2]any{{extSupportedVersions, u16(VersionTLS13)}, {extKeyShare, u16(secp256r1)}}}))), false, nil, ErrUnoffered},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := readResponse(bytes.NewReader(tc.stream), tc.h, Options{ReadKeyExchange: tc.readKX})
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !tc.check(resp) {
				t.Errorf("response = %+v (ServerHello %+v, KeyExchange %+v)", resp, resp.ServerHello, resp.KeyExchange)
			}
		})
	}
}

func TestReadResponseLimit(t *testing.T) {
	stream := record(recordHandshake, bytes.Join([][]byte{sh12(0x0033), hsMsg(typeCertificate, make([]byte, 3000)), hsMsg(typeServerKeyExchange, dhSKE(2048))}, nil))
	if _, err := readResponse(bytes.NewReader(stream), tls12Hello, Options{ReadKeyExchange: true, MaxBytes: 1000}); !errors.Is(err, ErrLimit) {
		t.Errorf("err = %v, want ErrLimit", err)
	}
	// An endless stream of small records stops at the limit.
	endless := io.MultiReader(bytes.NewReader(record(recordHandshake, []byte{11, 0xFF, 0xFF})), infiniteZeros{})
	if _, err := readResponse(endless, tls12Hello, Options{}); err == nil {
		t.Error("endless stream accepted")
	}
}

// An error after a valid ServerHello keeps the ServerHello.
func TestReadResponseKeepsServerHelloOnLaterError(t *testing.T) {
	stream := record(recordHandshake, sh12(dheRSAAES128GCM)) // then EOF before ServerKeyExchange
	resp, err := readResponse(bytes.NewReader(stream), tls12Hello, Options{ReadKeyExchange: true})
	if err == nil || resp == nil || resp.ServerHello == nil || resp.ServerHello.CipherSuite != dheRSAAES128GCM {
		t.Errorf("resp = %+v, err = %v", resp, err)
	}
}

type infiniteZeros struct{}

func (infiniteZeros) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

func TestBitLen(t *testing.T) {
	for _, tc := range []struct {
		in   []byte
		want int
	}{{nil, 0}, {[]byte{0, 0}, 0}, {[]byte{1}, 1}, {[]byte{0, 0x80, 0}, 16}, {[]byte{0x7F, 0xFF}, 15}} {
		if got := bitLen(tc.in); got != tc.want {
			t.Errorf("bitLen(%x) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestUnofferedError(t *testing.T) {
	err := error(&UnofferedError{"cipher suite", 0x002F})
	if !errors.Is(err, ErrUnoffered) || errors.Is(err, ErrMalformed) || err.Error() != "rawtls: server selected cipher suite 0x002f, which we did not offer" {
		t.Errorf("%v", err)
	}
}
