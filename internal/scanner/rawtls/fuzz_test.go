package rawtls

import (
	"bytes"
	"crypto/tls"
	"errors"
	"io"
	"slices"
	"testing"
	"time"
)

// countingReader counts what the parser consumed.
type countingReader struct {
	r io.Reader
	n int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += n
	return n, err
}

// typedError reports whether err is one of the errors callers can classify.
func typedError(err error) bool {
	for _, e := range []error{ErrMalformed, ErrUnoffered, ErrLimit, io.EOF, io.ErrUnexpectedEOF} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}

// captureFlight returns the first bytes a crypto/tls server sends back to h.
func captureFlight(t testing.TB, cfg *tls.Config, h *Hello) []byte {
	conn, _ := tlsServer(t, cfg)
	msg, err := h.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write(msg); err != nil {
		t.Fatal(err)
	}
	var out []byte
	buf := make([]byte, 4096)
	for len(out) < 8192 {
		_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		n, err := conn.Read(buf)
		out = append(out, buf[:n]...)
		if err != nil {
			break
		}
	}
	return out
}

func responseSeeds(t testing.TB) [][]byte {
	cert := hsMsg(typeCertificate, make([]byte, 64))
	ks, _ := X25519KeyShare()
	h13 := &Hello{Version: VersionTLS12, SupportedVersions: []uint16{VersionTLS13}, Suites: tls13Hello.Suites, Groups: []uint16{x25519}, KeyShares: []KeyShare{ks}}
	return [][]byte{
		record(recordAlert, []byte{2, 40}),
		record(recordHandshake, sh12(0x0033)),
		record(recordHandshake, bytes.Join([][]byte{sh12(dheRSAAES128GCM), cert, hsMsg(typeServerKeyExchange, dhSKE(1024)), hsMsg(typeServerHelloDone, nil)}, nil)),
		record(recordHandshake, bytes.Join([][]byte{sh12(0xC013), cert, hsMsg(typeServerKeyExchange, []byte{3, 0, 0x17, 2, 4, 1})}, nil)),
		record(recordHandshake, hsMsg(typeServerHello, serverHelloBody(shOpts{suite: aes128GCMSHA256, random: helloRetryRandom[:],
			exts: [][2]any{{extSupportedVersions, u16(VersionTLS13)}, {extKeyShare, u16(0x0100)}}}))),
		captureFlight(t, &tls.Config{MaxVersion: tls.VersionTLS12}, tls12Hello),
		captureFlight(t, &tls.Config{MinVersion: tls.VersionTLS13}, h13),
	}
}

func FuzzReadResponse(f *testing.F) {
	for i, s := range responseSeeds(f) {
		f.Add(s, uint8(i), uint16(0))
	}
	f.Fuzz(func(t *testing.T, stream []byte, mode uint8, limit uint16) {
		h := tls12Hello
		if mode&1 != 0 {
			h = tls13Hello
		}
		opts := Options{ReadKeyExchange: mode&2 != 0, MaxBytes: int(limit)}
		budget := opts.MaxBytes
		if budget <= 0 {
			budget = MaxResponseBytes
		}
		cr := &countingReader{r: bytes.NewReader(stream)}
		resp, err := readResponse(cr, h, opts)
		if cr.n > budget {
			t.Fatalf("read %d bytes, limit %d", cr.n, budget)
		}
		if err != nil && !typedError(err) {
			t.Fatalf("untyped error %T: %v", err, err)
		}
		if resp == nil {
			if err == nil {
				t.Fatal("nil response without error")
			}
			return
		}
		if resp.Alert == nil && resp.ServerHello == nil {
			t.Fatal("empty response")
		}
		if sh := resp.ServerHello; sh != nil {
			if !slices.Contains(h.Suites, sh.CipherSuite) || sh.Compression != 0 {
				t.Fatalf("accepted suite 0x%04x, compression %d", sh.CipherSuite, sh.Compression)
			}
			if sh.Version == VersionTLS13 && !sh.HelloRetry && !slices.ContainsFunc(h.KeyShares, func(k KeyShare) bool { return k.Group == sh.KeyShareGroup }) {
				t.Fatalf("accepted key share 0x%04x", sh.KeyShareGroup)
			}
			if sh.HelloRetry && !slices.Contains(h.Groups, sh.KeyShareGroup) {
				t.Fatalf("accepted HRR group 0x%04x", sh.KeyShareGroup)
			}
			if resp.KeyExchange != nil && (sh.Version > VersionTLS12 || !opts.ReadKeyExchange) {
				t.Fatal("key exchange read when not asked")
			}
		} else if resp.KeyExchange != nil {
			t.Fatal("key exchange without ServerHello")
		}
	})
}

func FuzzParseServerHello(f *testing.F) {
	f.Add(serverHelloBody(shOpts{suite: 0x0033}))
	f.Add(serverHelloBody(shOpts{suite: aes128GCMSHA256, exts: [][2]any{{extSupportedVersions, u16(VersionTLS13)}, keyShareExt(x25519)}}))
	f.Add(serverHelloBody(shOpts{suite: aes128GCMSHA256, random: helloRetryRandom[:], exts: [][2]any{{extSupportedVersions, u16(VersionTLS13)}, {extKeyShare, u16(secp256r1)}}}))
	f.Fuzz(func(t *testing.T, body []byte) {
		sh, err := ParseServerHello(body)
		if err != nil {
			if !errors.Is(err, ErrMalformed) {
				t.Fatalf("untyped error: %v", err)
			}
			return
		}
		if len(sh.SessionID) > 32 {
			t.Fatal("long session id")
		}
		if sh.HelloRetry && !slices.Contains(sh.Extensions, extSupportedVersions) {
			t.Fatal("HRR without supported_versions")
		}
		seen := map[uint16]bool{}
		for _, e := range sh.Extensions {
			if seen[e] {
				t.Fatalf("duplicate extension %d", e)
			}
			seen[e] = true
		}
	})
}

func FuzzParseServerKeyExchange(f *testing.F) {
	f.Add(dhSKE(1024), uint16(0x0033))
	f.Add([]byte{3, 0, 0x17, 2, 4, 1}, uint16(0xC013))
	f.Add(append(append(u16(2), 0xFF, 0xFF), 0, 1, 3), uint16(0x0003))
	f.Fuzz(func(t *testing.T, body []byte, id uint16) {
		s, ok := SuiteByID(id)
		if !ok {
			s = suites[int(id)%len(suites)]
		}
		ke, err := ParseServerKeyExchange(body, s)
		if err != nil {
			if !errors.Is(err, ErrMalformed) {
				t.Fatalf("untyped error: %v", err)
			}
			return
		}
		if ke.DHPrimeBits > 8*len(body) || ke.RSAModulusBits > 8*len(body) {
			t.Fatalf("%+v from %d bytes", ke, len(body))
		}
	})
}

func FuzzReadSSLv2Response(f *testing.F) {
	f.Add(ssl2ServerHello([]uint32{0x010080}, 16, false))
	f.Add(ssl2ServerHello([]uint32{0x060040, 0x0700C0}, 32, true))
	f.Add([]byte{0x80, 3, ssl2MsgError, 0, 1})
	f.Add(record(recordAlert, []byte{2, 70}))
	offer := allSSLv2()
	f.Fuzz(func(t *testing.T, stream []byte) {
		cr := &countingReader{r: bytes.NewReader(stream)}
		resp, err := readSSLv2Response(cr, offer)
		if cr.n > MaxResponseBytes {
			t.Fatalf("read %d bytes", cr.n)
		}
		if err != nil {
			if !typedError(err) {
				t.Fatalf("untyped error %T: %v", err, err)
			}
			return
		}
		if resp.ServerHello != nil {
			for _, c := range resp.ServerHello.Ciphers {
				if !slices.Contains(offer, c) {
					t.Fatalf("accepted cipher 0x%06x", c)
				}
			}
		}
	})
}
