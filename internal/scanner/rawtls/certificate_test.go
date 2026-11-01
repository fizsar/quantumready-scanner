package rawtls

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"slices"
	"testing"

	"golang.org/x/crypto/cryptobyte"
)

// certificateBody builds a TLS 1.0–1.2 Certificate message body.
func certificateBody(chain ...[]byte) []byte {
	var b cryptobyte.Builder
	b.AddUint24LengthPrefixed(func(b *cryptobyte.Builder) {
		for _, c := range chain {
			b.AddUint24LengthPrefixed(func(b *cryptobyte.Builder) { b.AddBytes(c) })
		}
	})
	return b.BytesOrPanic()
}

func TestParseCertificate(t *testing.T) {
	leaf, ca := []byte("leaf-der"), []byte("ca-der")
	chain, err := ParseCertificate(certificateBody(leaf, ca))
	if err != nil || len(chain) != 2 || !bytes.Equal(chain[0], leaf) || !bytes.Equal(chain[1], ca) {
		t.Fatalf("chain %q, err %v", chain, err)
	}
	if chain, err := ParseCertificate(certificateBody()); err != nil || len(chain) != 0 {
		t.Errorf("empty list: %q, %v (a server may send none)", chain, err)
	}
	for name, body := range map[string][]byte{
		"empty":                 {},
		"list longer than body": {0, 0, 9, 0, 0, 1, 1},
		"trailing bytes":        append(certificateBody(leaf), 0),
		"empty certificate":     {0, 0, 3, 0, 0, 0},
		"truncated entry":       {0, 0, 4, 0, 0, 5, 1},
		"TLS 1.3 form":          {0, 0, 0, 7, 0, 0, 2, 1, 2, 0, 0}, // context byte first
	} {
		if _, err := ParseCertificate(body); !errors.Is(err, ErrMalformed) {
			t.Errorf("%s: err = %v, want ErrMalformed", name, err)
		}
	}
}

// The chain of a real TLS 1.2 flight (a crypto/tls server) is read in clear
// when ReadKeyExchange is set, and only then.
func TestReadResponseCertificates(t *testing.T) {
	// tlsServer presents testCert (ECDSA): offer an ECDHE_ECDSA suite.
	h := &Hello{Version: VersionTLS12, Suites: []uint16{0xC02B}, Groups: []uint16{x25519, secp256r1}, ServerName: "rawtls.test"}
	flight := captureFlight(t, &tls.Config{MaxVersion: tls.VersionTLS12}, h)

	resp, err := readResponse(bytes.NewReader(flight), h, Options{ReadKeyExchange: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Certificates) != 1 || resp.KeyExchange == nil || resp.KeyExchange.Kind != "EC" {
		t.Fatalf("certificates %d, key exchange %+v, alert %v", len(resp.Certificates), resp.KeyExchange, resp.Alert)
	}
	if c, err := x509.ParseCertificate(resp.Certificates[0]); err != nil || c.Subject.CommonName != "rawtls.test" {
		t.Errorf("leaf does not parse as the server's: %v", err)
	}
	resp, err = readResponse(bytes.NewReader(flight), h, Options{})
	if err != nil || resp.Certificates != nil {
		t.Errorf("without ReadKeyExchange: certificates %d, err %v", len(resp.Certificates), err)
	}

	// A malformed Certificate is skipped: the probe still gets its key exchange.
	stream := record(recordHandshake, bytes.Join([][]byte{sh12(dheRSAAES128GCM), hsMsg(typeCertificate, []byte{0, 0, 9}),
		hsMsg(typeServerKeyExchange, dhSKE(2048)), hsMsg(typeServerHelloDone, nil)}, nil))
	resp, err = readResponse(bytes.NewReader(stream), tls12Hello, Options{ReadKeyExchange: true})
	if err != nil || resp.Certificates != nil || resp.KeyExchange == nil || resp.KeyExchange.DHPrimeBits != 2048 {
		t.Errorf("malformed certificate: %+v, err %v", resp, err)
	}
}

func TestSSLv2Certificate(t *testing.T) {
	stream := ssl2ServerHello([]uint32{0x010080}, 16, false)
	resp, err := readSSLv2Response(bytes.NewReader(stream), allSSLv2())
	if err != nil || !bytes.Equal(resp.ServerHello.Certificate, []byte{1, 2, 3, 4, 5}) {
		t.Fatalf("certificate %v, err %v", resp.ServerHello, err)
	}
	other := slices.Clone(stream)
	other[4] = 2 // certificate type other than X.509
	resp, err = readSSLv2Response(bytes.NewReader(other), allSSLv2())
	if err != nil || resp.ServerHello.Certificate != nil || resp.ServerHello.CertLen != 5 {
		t.Errorf("non-X.509 certificate type: %+v, err %v", resp.ServerHello, err)
	}
}

func FuzzParseCertificate(f *testing.F) {
	f.Add(certificateBody([]byte("leaf"), []byte("ca")))
	f.Add(certificateBody())
	f.Add([]byte{0, 0, 4, 0, 0, 5, 1})
	f.Fuzz(func(t *testing.T, body []byte) {
		chain, err := ParseCertificate(body)
		if err != nil {
			if !errors.Is(err, ErrMalformed) {
				t.Fatalf("untyped error %T: %v", err, err)
			}
			return
		}
		for _, c := range chain {
			if len(c) == 0 {
				t.Fatal("empty certificate accepted")
			}
		}
		if !bytes.Equal(certificateBody(chain...), body) { // nothing skipped, nothing invented
			t.Fatalf("chain %q does not rebuild the message", chain)
		}
	})
}
