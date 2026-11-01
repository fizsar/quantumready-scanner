package rawtls

import (
	"bytes"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// clientHelloWant is a testdata/clienthello/*.want.json file: the lists of a
// captured ClientHello, written by a separate parser (in Python) from the
// same bytes, so the two parsers check each other.
type clientHelloWant struct {
	LegacyVersion        uint16   `json:"legacy_version"`
	Suites               []uint16 `json:"suites"`
	Extensions           []uint16 `json:"extensions"`
	ServerName           string   `json:"server_name"`
	SupportedVersions    []uint16 `json:"supported_versions"`
	Groups               []uint16 `json:"groups"`
	KeyShares            []uint16 `json:"key_shares"`
	SignatureSchemes     []uint16 `json:"signature_schemes"`
	SignatureSchemesCert []uint16 `json:"signature_schemes_cert"`
	ALPN                 []string `json:"alpn"`
}

func (w clientHelloWant) of(ch *ClientHelloInfo) clientHelloWant {
	return clientHelloWant{ch.LegacyVersion, ch.Suites, ch.Extensions, ch.ServerName, ch.SupportedVersions, ch.Groups,
		ch.KeyShares, ch.SignatureSchemes, ch.SignatureSchemesCert, ch.ALPN}
}

// Real ClientHellos captured on 26/10/2026 from Chrome for Testing 156, Firefox
// 157 and the scanner's own browser-like probe (Go 1.27.3, cmd/qrscan), byte
// for byte. Chrome's has GREASE values, ECH and two unassigned extensions.
func TestParseClientHelloCaptured(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("testdata", "clienthello", "*.hex"))
	if len(files) != 3 {
		t.Fatalf("%d captures, want 3", len(files))
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			b, err := hex.DecodeString(strings.TrimSpace(string(raw)))
			if err != nil {
				t.Fatal(err)
			}
			var want clientHelloWant
			j, _ := os.ReadFile(strings.TrimSuffix(f, ".hex") + ".want.json")
			if err := json.Unmarshal(j, &want); err != nil {
				t.Fatal(err)
			}
			ch, err := ParseClientHello(b)
			if err != nil {
				t.Fatal(err)
			}
			got := want.of(ch)
			gj, _ := json.Marshal(got)
			wj, _ := json.Marshal(want)
			if !bytes.Equal(gj, wj) {
				t.Errorf("parsed\n%s\nwant (independent parser)\n%s", gj, wj)
			}
		})
	}
	// What the 2.8.5 and 2.8.7 notes rely on, as measured.
	chrome := mustParseCapture(t, "chrome156")
	firefox := mustParseCapture(t, "firefox157")
	mldsa := []uint16{0x0904, 0x0905, 0x0906}
	if !hasAll(chrome.SignatureSchemes, mldsa) || slices.ContainsFunc(firefox.SignatureSchemes, func(s uint16) bool { return slices.Contains(mldsa, s) }) {
		t.Error("Chrome 156 offers ML-DSA and Firefox 157 does not: not what the captures say")
	}
}

func mustParseCapture(t *testing.T, name string) *ClientHelloInfo {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "clienthello", name+".hex"))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := hex.DecodeString(strings.TrimSpace(string(raw)))
	ch, err := ParseClientHello(b)
	if err != nil {
		t.Fatal(err)
	}
	return ch
}

func hasAll(list, want []uint16) bool {
	for _, w := range want {
		if !slices.Contains(list, w) {
			return false
		}
	}
	return true
}

// Our own ClientHellos parse back to what was asked for.
func TestParseClientHelloOfMarshal(t *testing.T) {
	ks, _ := X25519KeyShare()
	many := make([]uint16, 9000) // > 16 KiB: the ClientHello spans two records
	for i := range many {
		many[i] = uint16(0x1000 + i)
	}
	for name, h := range map[string]*Hello{
		"tls12":      {Version: VersionTLS12, Suites: tls12Hello.Suites, ServerName: "rawtls.test", Groups: tls12Hello.Groups},
		"tls13":      {Version: VersionTLS12, SupportedVersions: []uint16{VersionTLS13, VersionTLS12}, Suites: tls13Hello.Suites, ServerName: "rawtls.test", Groups: []uint16{x25519, secp256r1}, KeyShares: []KeyShare{ks}},
		"emptyShare": {Version: VersionTLS12, SupportedVersions: []uint16{VersionTLS13}, Suites: tls13Hello.Suites, Groups: []uint16{0x0100}},
		"sslv3":      {Version: VersionSSL30, Suites: []uint16{0x000A, 0x0005}, NoExtensions: true},
		"tls10":      {Version: VersionTLS10, Suites: []uint16{0x002F}, Groups: []uint16{secp256r1}},
		"tworecords": {Version: VersionTLS12, Suites: many},
	} {
		t.Run(name, func(t *testing.T) {
			b, err := h.Marshal()
			if err != nil {
				t.Fatal(err)
			}
			ch, err := ParseClientHello(b)
			if err != nil {
				t.Fatal(err)
			}
			suites := slices.Clone(h.Suites)
			if !h.NoExtensions {
				suites = append(suites, SCSVRenegotiation)
			}
			var shares []uint16
			for _, k := range h.KeyShares {
				shares = append(shares, k.Group)
			}
			schemes := h.SignatureSchemes
			if schemes == nil && h.Version >= VersionTLS12 && !h.NoExtensions {
				schemes = DefaultSignatureSchemes
			}
			switch {
			case ch.LegacyVersion != h.Version || !slices.Equal(ch.Suites, suites):
				t.Errorf("version 0x%04x, %d suites; want 0x%04x, %d", ch.LegacyVersion, len(ch.Suites), h.Version, len(suites))
			case ch.ServerName != h.ServerName || !slices.Equal(ch.Groups, h.Groups) || !slices.Equal(ch.SupportedVersions, h.SupportedVersions):
				t.Errorf("name %q groups %v versions %v", ch.ServerName, ch.Groups, ch.SupportedVersions)
			case !slices.Equal(ch.KeyShares, shares) || h.offersTLS13() != (ch.KeyShares != nil):
				t.Errorf("key shares %v (nil %v), want %v", ch.KeyShares, ch.KeyShares == nil, shares)
			case !slices.Equal(ch.SignatureSchemes, schemes):
				t.Errorf("signature schemes %v", ch.SignatureSchemes)
			case h.NoExtensions && ch.Extensions != nil:
				t.Errorf("extensions %v on a bare hello", ch.Extensions)
			}
		})
	}
}

// What crypto/tls sends, parsed from the bytes, is what a crypto/tls server
// reads from them.
func TestParseClientHelloOfCryptoTLS(t *testing.T) {
	for name, cfg := range map[string]*tls.Config{
		"browserlike": {MinVersion: tls.VersionTLS12, CurvePreferences: []tls.CurveID{tls.X25519MLKEM768, tls.X25519, tls.CurveP256}},
		"tls12only":   {MinVersion: tls.VersionTLS12, MaxVersion: tls.VersionTLS12, CipherSuites: []uint16{0xC02B, 0xC02F}},
		"alpn":        {NextProtos: []string{"h2", "http/1.1"}},
	} {
		t.Run(name, func(t *testing.T) {
			conn, seen := tlsServer(t, &tls.Config{})
			rec := &recorder{Conn: conn}
			c := cfg.Clone()
			c.ServerName, c.InsecureSkipVerify = "rawtls.test", true
			_ = tls.Client(rec, c).Handshake()
			ch, err := ParseClientHello(rec.out.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			s := <-seen
			var groups []uint16
			for _, g := range s.groups {
				groups = append(groups, uint16(g))
			}
			var schemes []uint16
			for _, x := range s.schemes {
				schemes = append(schemes, uint16(x))
			}
			switch {
			case !slices.Equal(ch.Suites, s.suites) || !slices.Equal(ch.Groups, groups) || !slices.Equal(ch.SignatureSchemes, schemes):
				t.Errorf("suites %v/%v groups %v/%v schemes %v/%v", ch.Suites, s.suites, ch.Groups, groups, ch.SignatureSchemes, schemes)
			case !slices.Equal(ch.Extensions, s.extensions) || ch.ServerName != s.serverName:
				t.Errorf("extensions %v/%v name %q", ch.Extensions, s.extensions, ch.ServerName)
			case len(s.versions) > 1 && !slices.Equal(ch.SupportedVersions, s.versions):
				t.Errorf("versions %v/%v", ch.SupportedVersions, s.versions)
			case !slices.Equal(ch.ALPN, cfg.NextProtos):
				t.Errorf("ALPN %v", ch.ALPN)
			}
		})
	}
}

// recorder keeps a copy of what is written.
type recorder struct {
	net.Conn
	out bytes.Buffer
}

func (r *recorder) Write(p []byte) (int, error) {
	r.out.Write(p)
	return r.Conn.Write(p)
}

func TestParseClientHelloErrors(t *testing.T) {
	good, _ := tls12Hello.Marshal()
	for name, b := range map[string][]byte{
		"empty":       nil,
		"truncated":   good[:len(good)-3],
		"alert":       record(recordAlert, []byte{2, 40}),
		"serverhello": record(recordHandshake, sh12(0x0033)),
		"http":        []byte("GET / HTTP/1.1\r\n\r\n"),
		"badext":      record(recordHandshake, hsMsg(typeClientHello, append(append(u16(VersionTLS12), make([]byte, 32)...), 0, 0, 2, 0, 0x2f, 1, 0, 0, 5, 0, 10, 0, 3, 0))),
		"sslv2short":  {0x80, 0x10, 1},
	} {
		if ch, err := ParseClientHello(b); err == nil {
			t.Errorf("%s: parsed %+v", name, ch)
		} else if !typedError(err) {
			t.Errorf("%s: untyped error %v", name, err)
		}
	}
}

func TestParseSSLv2ClientHello(t *testing.T) {
	b, err := MarshalSSLv2Hello(allSSLv2())
	if err != nil {
		t.Fatal(err)
	}
	ch, err := ParseClientHello(b)
	if err != nil {
		t.Fatal(err)
	}
	if !ch.SSLv2 || ch.LegacyVersion != VersionSSL20 || !slices.Equal(ch.SSLv2Ciphers, allSSLv2()) || ch.Suites != nil {
		t.Errorf("%+v", ch)
	}
	if _, err := ParseSSLv2ClientHello(b[:len(b)-1]); err == nil {
		t.Error("truncated SSLv2 hello parsed")
	}
}

func TestParseServerFlight(t *testing.T) {
	ks, _ := X25519KeyShare()
	h13 := &Hello{Version: VersionTLS12, SupportedVersions: []uint16{VersionTLS13}, Suites: tls13Hello.Suites, Groups: []uint16{x25519}, KeyShares: []KeyShare{ks}}
	cert := hsMsg(typeCertificate, make([]byte, 64))
	sh13 := func(random []byte, group uint16) []byte {
		ext := keyShareExt(group)
		if random != nil {
			ext = [2]any{extKeyShare, u16(group)}
		}
		return record(recordHandshake, hsMsg(typeServerHello, serverHelloBody(shOpts{suite: aes128GCMSHA256, random: random,
			exts: [][2]any{{extSupportedVersions, u16(VersionTLS13)}, ext}})))
	}
	ccs := record(recordChangeCipherSpec, []byte{1})
	appData := record(recordApplicationData, make([]byte, 40))
	dh12 := record(recordHandshake, bytes.Join([][]byte{sh12(dheRSAAES128GCM), cert, hsMsg(typeServerKeyExchange, dhSKE(1024)), hsMsg(typeServerHelloDone, nil)}, nil))
	for name, c := range map[string]struct {
		b     []byte
		check func(*Flight) bool
		err   bool
	}{
		"empty": {nil, func(f *Flight) bool { return *f == Flight{} }, false},
		"tls12 from crypto/tls": {captureFlight(t, &tls.Config{MaxVersion: tls.VersionTLS12}, &Hello{Version: VersionTLS12, Suites: []uint16{ecdheECDSAAES128GCM}, Groups: []uint16{x25519}}), func(f *Flight) bool {
			return f.ServerHello != nil && f.ServerHello.Version == VersionTLS12 && f.KeyExchange != nil && f.KeyExchange.Kind == "EC" && !f.Encrypted
		}, false},
		"tls13 from crypto/tls": {captureFlight(t, &tls.Config{MinVersion: tls.VersionTLS13}, h13), func(f *Flight) bool {
			return f.ServerHello != nil && f.ServerHello.Version == VersionTLS13 && f.ServerHello.KeyShareGroup == x25519 && f.Encrypted && f.Retry == nil
		}, false},
		"alert first": {record(recordAlert, []byte{2, 40}), func(f *Flight) bool {
			return f.ServerHello == nil && f.Alert != nil && f.Alert.Level == 2 && f.Alert.Description == 40
		}, false},
		"hello retry then server hello": {bytes.Join([][]byte{sh13(helloRetryRandom[:], secp256r1), ccs, sh13(nil, secp256r1), appData}, nil), func(f *Flight) bool {
			return f.ServerHello.HelloRetry && f.ServerHello.KeyShareGroup == secp256r1 && f.Retry != nil && !f.Retry.HelloRetry && f.Retry.KeyShareGroup == secp256r1 && f.Encrypted
		}, false},
		"hello retry then alert": {bytes.Join([][]byte{sh13(helloRetryRandom[:], secp256r1), record(recordAlert, []byte{2, 47})}, nil), func(f *Flight) bool {
			return f.ServerHello.HelloRetry && f.Retry == nil && f.Alert.Description == 47
		}, false},
		"tls12 dhe, then an alert in clear": {append(slices.Clone(dh12), record(recordAlert, []byte{2, 20})...), func(f *Flight) bool {
			return f.KeyExchange.Kind == "DH" && f.KeyExchange.DHPrimeBits == 1024 && f.Alert.Description == 20
		}, false},
		"tls12, then ChangeCipherSpec and encrypted Finished": {bytes.Join([][]byte{dh12, ccs, record(recordHandshake, make([]byte, 40))}, nil), func(f *Flight) bool {
			return f.KeyExchange != nil && f.Encrypted && f.Alert == nil
		}, false},
		"cut mid-record": {dh12[:len(dh12)-7], func(f *Flight) bool { return f.ServerHello == nil }, false},
		"not TLS":        {[]byte("HTTP/1.1 400 Bad Request\r\n\r\n"), func(f *Flight) bool { return f.ServerHello == nil }, true},
	} {
		f, err := ParseServerFlight(c.b)
		if (err != nil) != c.err || f == nil || !c.check(f) {
			t.Errorf("%s: %+v, err %v", name, f, err)
		}
		if err != nil && !typedError(err) {
			t.Errorf("%s: untyped error %v", name, err)
		}
	}
}

func FuzzParseClientHello(f *testing.F) {
	for _, h := range []*Hello{tls12Hello, tls13Hello, {Version: VersionSSL30, Suites: []uint16{5}, NoExtensions: true}} {
		b, _ := h.Marshal()
		f.Add(b)
	}
	ssl2, _ := MarshalSSLv2Hello(allSSLv2())
	f.Add(ssl2)
	f.Fuzz(func(t *testing.T, b []byte) {
		ch, err := ParseClientHello(b)
		if err != nil {
			if !typedError(err) {
				t.Fatalf("untyped error %T: %v", err, err)
			}
			return
		}
		if 2*len(ch.Suites)+3*len(ch.SSLv2Ciphers)+2*len(ch.Extensions) > len(b) {
			t.Fatalf("more parsed than read: %+v from %d bytes", ch, len(b))
		}
	})
}

func FuzzParseServerFlight(f *testing.F) {
	for _, s := range responseSeeds(f) {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		fl, err := ParseServerFlight(b)
		if fl == nil {
			t.Fatal("nil flight")
		}
		if err != nil && !typedError(err) && !errors.Is(err, errEncrypted) {
			t.Fatalf("untyped error %T: %v", err, err)
		}
		if fl.Retry != nil && (fl.ServerHello == nil || !fl.ServerHello.HelloRetry) {
			t.Fatal("a retry ServerHello without a HelloRetryRequest")
		}
		if fl.KeyExchange != nil && (fl.ServerHello == nil || fl.ServerHello.Version > VersionTLS12) {
			t.Fatal("key exchange without a TLS 1.0–1.2 ServerHello")
		}
	})
}

// After a HelloRetryRequest the client sends a second ClientHello in clear:
// both are read, with the key share the server asked for in the second.
func TestParseClientHellosAfterRetry(t *testing.T) {
	// Over TCP: on a synchronous net.Pipe both ends would write their
	// ChangeCipherSpec at once after the HelloRetryRequest and block.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		_ = tls.Server(c, &tls.Config{Certificates: []tls.Certificate{testCert(t)}, CurvePreferences: []tls.CurveID{tls.CurveP256}}).Handshake()
	}()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	rec := &recorder{Conn: conn}
	if err := tls.Client(rec, &tls.Config{ServerName: "rawtls.test", InsecureSkipVerify: true,
		CurvePreferences: []tls.CurveID{tls.X25519, tls.CurveP256}}).Handshake(); err != nil {
		t.Fatal(err)
	}
	chs, err := ParseClientHellos(rec.out.Bytes())
	if err != nil || len(chs) != 2 {
		t.Fatalf("%d hellos, %v", len(chs), err)
	}
	if !slices.Equal(chs[0].KeyShares, []uint16{x25519}) || !slices.Equal(chs[1].KeyShares, []uint16{secp256r1}) ||
		!slices.Equal(chs[0].Suites, chs[1].Suites) {
		t.Errorf("first %+v\nsecond %+v", chs[0], chs[1])
	}
	// Without a retry, one; a TLS 1.2 handshake goes on with a ClientKeyExchange, not a hello.
	conn12, _ := tlsServer(t, &tls.Config{MaxVersion: tls.VersionTLS12})
	rec12 := &recorder{Conn: conn12}
	_ = tls.Client(rec12, &tls.Config{ServerName: "rawtls.test", InsecureSkipVerify: true}).Handshake()
	if chs, err := ParseClientHellos(rec12.out.Bytes()); err != nil || len(chs) != 1 {
		t.Errorf("TLS 1.2: %d hellos, %v", len(chs), err)
	}
	if _, err := ParseClientHellos([]byte("GET /")); err == nil {
		t.Error("not TLS, parsed")
	}
}
