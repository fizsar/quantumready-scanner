package rawtls

import (
	"bytes"
	"crypto/tls"
	"errors"
	"slices"
	"testing"

	"golang.org/x/crypto/cryptobyte"
)

const (
	ecdheECDSAAES128GCM = 0xC02B
	ecdheRSAAES128GCM   = 0xC02F
	dheRSAAES128GCM     = 0x009E
	aes128GCMSHA256     = 0x1301
	aes128CCMSHA256     = 0x1304
	x25519              = 0x001d
	secp256r1           = 0x0017
)

func TestExchangeTLS12AgainstCryptoTLS(t *testing.T) {
	conn, seen := tlsServer(t, &tls.Config{MaxVersion: tls.VersionTLS12})
	h := &Hello{Version: VersionTLS12, Suites: []uint16{dheRSAAES128GCM, ecdheECDSAAES128GCM}, ServerName: "rawtls.test", Groups: []uint16{x25519}}
	resp, err := Exchange(conn, h, Options{ReadKeyExchange: true})
	if err != nil {
		t.Fatal(err)
	}
	sh := resp.ServerHello
	if sh == nil || sh.Version != VersionTLS12 || sh.CipherSuite != ecdheECDSAAES128GCM || sh.HelloRetry {
		t.Fatalf("ServerHello = %+v", sh)
	}
	if ke := resp.KeyExchange; ke == nil || ke.Kind != "EC" || ke.Group != x25519 {
		t.Errorf("KeyExchange = %+v, want EC x25519", ke)
	}
	got := <-seen
	if got.serverName != "rawtls.test" || !slices.Contains(got.suites, ecdheECDSAAES128GCM) || !slices.Contains(got.suites, SCSVRenegotiation) {
		t.Errorf("server saw %+v", got)
	}
	if !slices.Contains(got.groups, tls.X25519) || len(got.schemes) != len(DefaultSignatureSchemes) {
		t.Errorf("server saw groups %v, %d signature schemes", got.groups, len(got.schemes))
	}
}

func TestExchangeTLS13AgainstCryptoTLS(t *testing.T) {
	conn, seen := tlsServer(t, &tls.Config{MinVersion: tls.VersionTLS13})
	ks, err := X25519KeyShare()
	if err != nil {
		t.Fatal(err)
	}
	h := &Hello{Version: VersionTLS12, SupportedVersions: []uint16{VersionTLS13}, Suites: []uint16{aes128GCMSHA256},
		ServerName: "rawtls.test", Groups: []uint16{x25519}, KeyShares: []KeyShare{ks}}
	resp, err := Exchange(conn, h, Options{ReadKeyExchange: true}) // ignored in TLS 1.3
	if err != nil {
		t.Fatal(err)
	}
	sh := resp.ServerHello
	if sh == nil || sh.Version != VersionTLS13 || sh.LegacyVersion != VersionTLS12 || sh.CipherSuite != aes128GCMSHA256 ||
		sh.HelloRetry || sh.KeyShareGroup != x25519 || len(sh.SessionID) != 32 {
		t.Fatalf("ServerHello = %+v", sh)
	}
	if got := <-seen; !slices.Equal(got.versions, []uint16{VersionTLS13}) {
		t.Errorf("server saw versions %x", got.versions)
	}
}

// An empty key_share makes the server name the group it wants.
func TestExchangeHelloRetryRequest(t *testing.T) {
	conn, _ := tlsServer(t, &tls.Config{MinVersion: tls.VersionTLS13})
	h := &Hello{Version: VersionTLS12, SupportedVersions: []uint16{VersionTLS13}, Suites: []uint16{aes128GCMSHA256},
		Groups: []uint16{0x0100, secp256r1}} // ffdhe2048 (unknown to Go), secp256r1
	resp, err := Exchange(conn, h, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if sh := resp.ServerHello; sh == nil || !sh.HelloRetry || sh.KeyShareGroup != secp256r1 || sh.Version != VersionTLS13 {
		t.Fatalf("ServerHello = %+v, want HelloRetryRequest for secp256r1", sh)
	}
}

func TestExchangeRefusals(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  *tls.Config
		h    *Hello
	}{
		{"CCM in TLS 1.3 (Go has none)", &tls.Config{MinVersion: tls.VersionTLS13},
			&Hello{Version: VersionTLS12, SupportedVersions: []uint16{VersionTLS13}, Suites: []uint16{aes128CCMSHA256}, Groups: []uint16{x25519}}},
		{"DHE only (Go has none)", &tls.Config{MaxVersion: tls.VersionTLS12},
			&Hello{Version: VersionTLS12, Suites: []uint16{dheRSAAES128GCM}, Groups: []uint16{x25519}}},
		{"TLS 1.0 against a TLS 1.2+ server", &tls.Config{MinVersion: tls.VersionTLS12},
			&Hello{Version: VersionTLS10, Suites: []uint16{0xC009, 0xC013}, Groups: []uint16{x25519}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn, _ := tlsServer(t, tc.cfg)
			resp, err := Exchange(conn, tc.h, Options{})
			if err != nil {
				t.Fatal(err)
			}
			if resp.Alert == nil || resp.ServerHello != nil || resp.Alert.Level != 2 {
				t.Fatalf("response = %+v, want a fatal alert", resp)
			}
		})
	}
}

// parsedHello decodes our own ClientHello, independently of the builder.
type parsedHello struct {
	recordVersion uint16
	version       uint16
	sessionID     []byte
	suites        []uint16
	extensions    map[uint16][]byte
	extOrder      []uint16
	length        int // handshake message, header included
}

func parseClientHello(t *testing.T, wire []byte) parsedHello {
	t.Helper()
	var hs []byte
	var p parsedHello
	for s := cryptobyte.String(wire); !s.Empty(); {
		var typ uint8
		var payload cryptobyte.String
		if !s.ReadUint8(&typ) || !s.ReadUint16(&p.recordVersion) || !s.ReadUint16LengthPrefixed(&payload) || typ != recordHandshake {
			t.Fatal("bad record")
		}
		if len(payload) > maxPlaintext {
			t.Fatalf("record of %d bytes", len(payload))
		}
		hs = append(hs, payload...)
	}
	p.length = len(hs)
	s := cryptobyte.String(hs)
	var typ uint8
	var body, random, sid, suites, comp, exts cryptobyte.String
	if !s.ReadUint8(&typ) || !s.ReadUint24LengthPrefixed(&body) || !s.Empty() || typ != typeClientHello {
		t.Fatal("bad handshake header")
	}
	if !body.ReadUint16(&p.version) || !body.ReadBytes((*[]byte)(&random), 32) || !body.ReadUint8LengthPrefixed(&sid) ||
		!body.ReadUint16LengthPrefixed(&suites) || !body.ReadUint8LengthPrefixed(&comp) {
		t.Fatal("bad ClientHello")
	}
	p.sessionID = sid
	for !suites.Empty() {
		var id uint16
		suites.ReadUint16(&id)
		p.suites = append(p.suites, id)
	}
	if !bytes.Equal(comp, []byte{0}) {
		t.Errorf("compression methods %x", []byte(comp))
	}
	p.extensions = map[uint16][]byte{}
	if body.Empty() {
		return p
	}
	if !body.ReadUint16LengthPrefixed(&exts) || !body.Empty() {
		t.Fatal("bad extensions")
	}
	for !exts.Empty() {
		var id uint16
		var data cryptobyte.String
		if !exts.ReadUint16(&id) || !exts.ReadUint16LengthPrefixed(&data) {
			t.Fatal("bad extension")
		}
		p.extensions[id] = data
		p.extOrder = append(p.extOrder, id)
	}
	return p
}

func TestMarshalVariants(t *testing.T) {
	t.Run("SSLv3 bare", func(t *testing.T) {
		wire, err := (&Hello{Version: VersionSSL30, Suites: []uint16{0x000A, 0x0005}, NoExtensions: true}).Marshal()
		if err != nil {
			t.Fatal(err)
		}
		p := parseClientHello(t, wire)
		if p.recordVersion != VersionSSL30 || p.version != VersionSSL30 || len(p.extensions) != 0 || !slices.Equal(p.suites, []uint16{0x000A, 0x0005}) {
			t.Errorf("%+v", p)
		}
	})
	t.Run("TLS 1.0 has no signature_algorithms", func(t *testing.T) {
		wire, _ := (&Hello{Version: VersionTLS10, Suites: []uint16{0xC013}, Groups: []uint16{secp256r1}, ServerName: "a.test"}).Marshal()
		p := parseClientHello(t, wire)
		_, sig := p.extensions[extSignatureAlgorithms]
		_, sv := p.extensions[extSupportedVersions]
		if p.recordVersion != VersionTLS10 || p.version != VersionTLS10 || sig || sv || p.extensions[extServerName] == nil ||
			!bytes.Equal(p.extensions[extECPointFormats], []byte{1, 0}) || len(p.sessionID) != 0 {
			t.Errorf("%+v", p)
		}
		if p.suites[len(p.suites)-1] != SCSVRenegotiation {
			t.Errorf("SCSV not last: %x", p.suites)
		}
	})
	t.Run("TLS 1.3 with an empty key_share", func(t *testing.T) {
		wire, _ := (&Hello{Version: VersionTLS12, SupportedVersions: []uint16{VersionTLS13}, Suites: []uint16{aes128GCMSHA256}, Groups: []uint16{0x0100}}).Marshal()
		p := parseClientHello(t, wire)
		if !bytes.Equal(p.extensions[extKeyShare], []byte{0, 0}) || !bytes.Equal(p.extensions[extSupportedVersions], []byte{2, 3, 4}) || len(p.sessionID) != 32 {
			t.Errorf("key_share %x, supported_versions %x", p.extensions[extKeyShare], p.extensions[extSupportedVersions])
		}
	})
	t.Run("padding to 512 bytes", func(t *testing.T) {
		for n := 1; n < 160; n++ { // walk through lengths around 256–511
			suites := make([]uint16, n)
			for i := range suites {
				suites[i] = uint16(0xC000 + i)
			}
			wire, err := (&Hello{Version: VersionTLS12, Suites: suites, ServerName: "a.test"}).Marshal()
			if err != nil {
				t.Fatal(err)
			}
			p := parseClientHello(t, wire)
			_, padded := p.extensions[extPadding]
			if p.length > 0xff && p.length < 0x200 {
				t.Fatalf("%d suites: ClientHello of %d bytes left unpadded", n, p.length)
			}
			if padded && (p.length < 0x200 || p.length > 0x200+3) {
				t.Fatalf("%d suites: padded to %d", n, p.length)
			}
		}
	})
	t.Run("split into records", func(t *testing.T) {
		big := KeyShare{Group: 0x0201, Data: make([]byte, 20000)}
		wire, err := (&Hello{Version: VersionTLS12, SupportedVersions: []uint16{VersionTLS13}, Suites: []uint16{aes128GCMSHA256}, KeyShares: []KeyShare{big}}).Marshal()
		if err != nil {
			t.Fatal(err)
		}
		if p := parseClientHello(t, wire); len(p.extensions[extKeyShare]) != 2+4+20000 {
			t.Errorf("key_share of %d bytes", len(p.extensions[extKeyShare]))
		}
	})
	t.Run("invalid", func(t *testing.T) {
		for _, h := range []*Hello{
			{Version: VersionTLS13, Suites: []uint16{1}},
			{Version: VersionSSL20, Suites: []uint16{1}},
			{Version: VersionTLS11, SupportedVersions: []uint16{VersionTLS13}, Suites: []uint16{1}},
			{Version: VersionTLS12},
			{Version: VersionTLS12, Suites: []uint16{1}, KeyShares: []KeyShare{{Group: 1, Data: make([]byte, 70000)}}, SupportedVersions: []uint16{VersionTLS13}},
		} {
			if _, err := h.Marshal(); err == nil {
				t.Errorf("Marshal(%+v) succeeded", h)
			}
		}
	})
}

func TestExchangeWriteError(t *testing.T) {
	conn, _ := tlsServer(t, &tls.Config{})
	conn.Close()
	if _, err := Exchange(conn, &Hello{Version: VersionTLS12, Suites: []uint16{1}}, Options{}); err == nil || errors.Is(err, ErrMalformed) {
		t.Errorf("err = %v, want a write error", err)
	}
}
