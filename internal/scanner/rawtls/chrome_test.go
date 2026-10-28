package rawtls

import (
	"bytes"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/crypto/cryptobyte"
)

type rawExt struct {
	typ  uint16
	data []byte
}

// helloParts splits a single-record ClientHello into what precedes the
// extensions (legacy_version, random, session_id, suites, compression) and
// the extensions, as sent.
func helloParts(t *testing.T, rec []byte) (version uint16, random, sessionID, suites, compression []byte, exts []rawExt) {
	t.Helper()
	s := cryptobyte.String(rec[5+4:])
	var sid, cs, comp, list cryptobyte.String
	if !s.ReadUint16(&version) || !s.ReadBytes(&random, 32) || !s.ReadUint8LengthPrefixed(&sid) ||
		!s.ReadUint16LengthPrefixed(&cs) || !s.ReadUint8LengthPrefixed(&comp) || !s.ReadUint16LengthPrefixed(&list) || !s.Empty() {
		t.Fatal("ClientHello does not parse")
	}
	for !list.Empty() {
		var e rawExt
		var d cryptobyte.String
		if !list.ReadUint16(&e.typ) || !list.ReadUint16LengthPrefixed(&d) {
			t.Fatal("extension does not parse")
		}
		e.data = d
		exts = append(exts, e)
	}
	return version, random, sid, cs, comp, exts
}

func chromeCapture(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "clienthello", "chrome156.hex"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// ChromeHello against the capture of Chrome 156, byte for byte: everything
// but what Chrome draws at random on every connection (random, session_id,
// the key share data, the GREASE ECH config_id, enc and payload) must be
// the captured bytes, extension by extension and in the same order.
func TestChromeHelloMatchesCapture(t *testing.T) {
	capture := chromeCapture(t)
	ours, h, err := ChromeHello("lab.test") // the capture's SNI
	if err != nil {
		t.Fatal(err)
	}
	if len(ours) != len(capture) || !bytes.Equal(ours[:5], capture[:5]) || !bytes.Equal(ours[5:9], capture[5:9]) {
		t.Fatalf("record and message headers %x, want %x (lengths %d and %d)", ours[:9], capture[:9], len(ours), len(capture))
	}
	cv, _, cSID, cSuites, cComp, cExts := helloParts(t, capture)
	ov, _, oSID, oSuites, oComp, oExts := helloParts(t, ours)
	if ov != cv || len(oSID) != len(cSID) || !bytes.Equal(oSuites, cSuites) || !bytes.Equal(oComp, cComp) {
		t.Fatalf("version %04x, session_id %d bytes, suites %x, compression %x; want %04x, %d, %x, %x", ov, len(oSID), oSuites, oComp, cv, len(cSID), cSuites, cComp)
	}
	if len(oExts) != len(cExts) {
		t.Fatalf("%d extensions, want %d", len(oExts), len(cExts))
	}
	for i, c := range cExts {
		o := oExts[i]
		if o.typ != c.typ || len(o.data) != len(c.data) {
			t.Errorf("extension %d: 0x%04x of %d bytes, want 0x%04x of %d", i, o.typ, len(o.data), c.typ, len(c.data))
			continue
		}
		switch c.typ {
		case extKeyShare: // the groups and sizes; the keys are fresh
			og, cg := keyShareLayout(t, o.data), keyShareLayout(t, c.data)
			if !slices.Equal(og, cg) {
				t.Errorf("key_share layout %v, want %v", og, cg)
			}
		case extECH: // type, KDF, AEAD; then config_id, enc and payload are random, same sizes
			if !bytes.Equal(o.data[:5], c.data[:5]) || !bytes.Equal(o.data[6:8], c.data[6:8]) || !bytes.Equal(o.data[40:42], c.data[40:42]) {
				t.Errorf("ECH header %x, want %x", o.data[:8], c.data[:8])
			}
		default:
			if !bytes.Equal(o.data, c.data) {
				t.Errorf("extension 0x%04x: %x, want %x", c.typ, o.data, c.data)
			}
		}
	}

	// And through the parser, against the independent one (want.json).
	var want clientHelloWant
	j, _ := os.ReadFile(filepath.Join("testdata", "clienthello", "chrome156.want.json"))
	if err := json.Unmarshal(j, &want); err != nil {
		t.Fatal(err)
	}
	ch, err := ParseClientHello(ours)
	if err != nil {
		t.Fatal(err)
	}
	gj, _ := json.Marshal(want.of(ch))
	wj, _ := json.Marshal(want)
	if !bytes.Equal(gj, wj) {
		t.Errorf("parsed\n%s\nwant\n%s", gj, wj)
	}

	// What 2.8.7 asked of the probe, stated.
	for _, s := range h.SignatureSchemes {
		if s == 0x0807 || s == 0x0201 || s == 0x0203 {
			t.Errorf("offers 0x%04x: no Ed25519 or SHA-1", s)
		}
	}
	if slices.Index(h.SignatureSchemes, 0x0403) > slices.Index(h.SignatureSchemes, 0x0804) || !slices.Equal(ch.ALPN, []string{"h2", "http/1.1"}) {
		t.Error("ECDSA before RSA-PSS, ALPN h2 and http/1.1")
	}
	if len(h.KeyShares) != 2 || h.KeyShares[0].Group != 0x11ec || h.KeyShares[1].Group != 0x001d {
		t.Errorf("offer %+v: key shares for X25519MLKEM768 and x25519", h.KeyShares)
	}
}

// keyShareLayout is "group:size" for each entry of a key_share extension.
func keyShareLayout(t *testing.T, data []byte) []string {
	t.Helper()
	s := cryptobyte.String(data)
	var list cryptobyte.String
	if !s.ReadUint16LengthPrefixed(&list) {
		t.Fatal("bad key_share")
	}
	var out []string
	for !list.Empty() {
		var g uint16
		var k cryptobyte.String
		if !list.ReadUint16(&g) || !list.ReadUint16LengthPrefixed(&k) {
			t.Fatal("bad key_share entry")
		}
		out = append(out, fmt.Sprintf("%04x:%d", g, len(k)))
	}
	return out
}

func TestChromeHelloFresh(t *testing.T) {
	a, _, _ := ChromeHello("example.test")
	b, _, _ := ChromeHello("example.test")
	_, ra, sa, _, _, ea := helloParts(t, a)
	_, rb, sb, _, _, eb := helloParts(t, b)
	if bytes.Equal(ra, rb) || bytes.Equal(sa, sb) {
		t.Error("random or session_id repeated")
	}
	for i := range ea {
		// The ECH enc (bytes 8–40) and payload (from 42) are each drawn anew.
		if ea[i].typ == extKeyShare && bytes.Equal(ea[i].data, eb[i].data) ||
			ea[i].typ == extECH && (bytes.Equal(ea[i].data[8:40], eb[i].data[8:40]) || bytes.Equal(ea[i].data[42:], eb[i].data[42:])) {
			t.Errorf("extension 0x%04x repeated across connections", ea[i].typ)
		}
	}
	noSNI, _, _ := ChromeHello("")
	ch, err := ParseClientHello(noSNI)
	if err != nil || slices.Contains(ch.Extensions, extServerName) {
		t.Errorf("an IP literal gets no SNI: %v, %v", ch.Extensions, err)
	}
}

// A crypto/tls server answers Chrome's offer as it would Chrome: hybrid ML-KEM
// in TLS 1.3; in TLS 1.2, ECDHE with x25519 (read from ServerKeyExchange).
func TestExchangeChrome(t *testing.T) {
	conn, seen := tlsServer(t, &tls.Config{})
	resp, err := ExchangeChrome(conn, "rawtls.test", Options{ReadKeyExchange: true})
	if err != nil {
		t.Fatal(err)
	}
	sh := resp.ServerHello
	if sh.Version != VersionTLS13 || sh.KeyShareGroup != 0x11ec || sh.HelloRetry || !slices.Contains(CertificateSuites, sh.CipherSuite) {
		t.Errorf("TLS 1.3 answer %+v", sh)
	}
	got := <-seen
	if got.serverName != "rawtls.test" || !slices.Contains(got.groups, tls.X25519MLKEM768) || !slices.Contains(got.extensions, extALPN) {
		t.Errorf("server saw %+v", got)
	}

	conn, _ = tlsServer(t, &tls.Config{MaxVersion: tls.VersionTLS12})
	resp, err = ExchangeChrome(conn, "rawtls.test", Options{ReadKeyExchange: true})
	if err != nil {
		t.Fatal(err)
	}
	if sh := resp.ServerHello; sh.Version != VersionTLS12 || sh.CipherSuite != 0xc02b || resp.KeyExchange == nil || resp.KeyExchange.Group != 0x001d {
		t.Errorf("TLS 1.2 answer %+v, key exchange %+v", sh, resp.KeyExchange)
	}
}
