package rawtls

import (
	"bytes"
	"crypto/ecdh"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"testing"
)

// The decrypting certificate probe (tls13cert.go) against RFC 8448, section 3
// (rfc8448_vectors_test.go): the published values of a real TLS 1.3
// handshake, so the key schedule and the decryption are checked against an
// independent source, not only against crypto/tls.

func vec(t testing.TB, name string) []byte {
	t.Helper()
	b, err := hex.DecodeString(rfc8448[name])
	if err != nil || len(b) == 0 {
		t.Fatalf("vector %s: %v", name, err)
	}
	return b
}

// rfcClient is the RFC's client: its x25519 key, its ClientHello as sent and
// the Hello the answer is checked against.
func rfcClient(t testing.TB) (*Hello, []byte, *keyExchange) {
	t.Helper()
	priv, err := ecdh.X25519().NewPrivateKey(vec(t, "clientPrivate"))
	if err != nil {
		t.Fatal(err)
	}
	kx := ecdhExchangeWith(x25519, priv)
	records := vec(t, "clientHelloRecord")
	ch, err := ParseClientHello(records)
	if err != nil {
		t.Fatal(err)
	}
	h := &Hello{Version: ch.LegacyVersion, SupportedVersions: ch.SupportedVersions, Suites: ch.Suites, Groups: ch.Groups, KeyShares: []KeyShare{kx.share}}
	return h, records, kx
}

func TestRFC8448KeySchedule(t *testing.T) {
	_, records, kx := rfcClient(t)
	shared, err := kx.finish(vec(t, "serverPublic"))
	if err != nil || !bytes.Equal(shared, vec(t, "sharedSecret")) {
		t.Fatalf("shared secret %x, %v", shared, err)
	}
	transcript := append(handshakeMessages(records), handshakeMessages(vec(t, "serverHelloRecord"))...)
	hs, secret, key, iv, err := handshakeKeys(0x1301, shared, transcript)
	if err != nil {
		t.Fatal(err)
	}
	for name, got := range map[string][]byte{"handshakeSecret": hs, "serverHandshakeTraffic": secret, "key": key, "iv": iv} {
		if !bytes.Equal(got, vec(t, name)) {
			t.Errorf("%s = %x, want %x", name, got, vec(t, name))
		}
	}
}

func TestRFC8448Decrypt(t *testing.T) {
	dec, err := newDecrypter(0x1301, vec(t, "key"), vec(t, "iv"))
	if err != nil {
		t.Fatal(err)
	}
	rec := vec(t, "flightRecord")
	plain, err := dec.open(rec[:5], rec[5:])
	want := append(vec(t, "flightPlaintext"), recordHandshake) // TLSInnerPlaintext: content, then its type
	if err != nil || !bytes.Equal(plain, want) {
		t.Fatalf("decrypted %d bytes (%v), want %d", len(plain), err, len(want))
	}
	// The next record uses the next sequence number: the same one again fails.
	if _, err := dec.open(rec[:5], rec[5:]); err == nil {
		t.Error("a record opened twice: the sequence number did not advance")
	}
}

// The whole reading path with the RFC's bytes: ServerHello in clear, then the
// encrypted flight, read up to CertificateVerify.
func TestRFC8448Flight(t *testing.T) {
	h, records, kx := rfcClient(t)
	server := append(vec(t, "serverHelloRecord"), vec(t, "flightRecord")...)
	f, err := readCertificateFlight(bytes.NewReader(server), h, records, kx)
	if err != nil {
		t.Fatal(err)
	}
	want, err := ParseTLS13Certificate(vec(t, "certificate")[4:])
	if err != nil {
		t.Fatal(err)
	}
	cv := vec(t, "certificateVerify")
	if f.ServerHello.CipherSuite != 0x1301 || len(f.Chain) != 1 || !bytes.Equal(f.Chain[0], want[0]) ||
		f.SignatureScheme != uint16(cv[4])<<8|uint16(cv[5]) || f.SignatureScheme != 0x0804 || f.Alert != nil {
		t.Fatalf("%+v", f)
	}
	if c, err := x509.ParseCertificate(f.Chain[0]); err != nil || c.PublicKeyAlgorithm != x509.RSA {
		t.Errorf("leaf: %v", err)
	}
	// One flipped bit anywhere in the encrypted record and nothing is read.
	for _, i := range []int{len(vec(t, "serverHelloRecord")) + 5, len(server) - 1} {
		bad := bytes.Clone(server)
		bad[i] ^= 1
		if f, err := readCertificateFlight(bytes.NewReader(bad), h, records, kx); !errors.Is(err, ErrMalformed) || f.Chain != nil {
			t.Errorf("bit %d flipped: %+v, %v", i, f, err)
		}
	}
}

// Everything the probe reads from a server, from the RFC exchange on.
func FuzzReadCertificateFlight(f *testing.F) {
	f.Add(append(vec(f, "serverHelloRecord"), vec(f, "flightRecord")...))
	f.Add(vec(f, "serverHelloRecord"))
	f.Add(record(recordAlert, []byte{2, 40}))
	h, records, kx := rfcClient(f)
	f.Fuzz(func(t *testing.T, server []byte) {
		fl, err := readCertificateFlight(bytes.NewReader(server), h, records, kx)
		if err != nil && !typedError(err) && !errors.Is(err, ErrRetry) {
			t.Fatalf("untyped error %T: %v", err, err)
		}
		if fl == nil {
			return
		}
		total := 0
		for _, c := range fl.Chain {
			if len(c) == 0 {
				t.Fatal("empty certificate")
			}
			total += len(c)
		}
		if total > len(server) {
			t.Fatalf("%d bytes of certificates from %d", total, len(server))
		}
	})
}

// What the probe does with decrypted handshake data: the fuzzer's bytes are
// encrypted with the RFC's keys, so it gets past the AEAD.
func FuzzDecryptedFlight(f *testing.F) {
	f.Add(vec(f, "flightPlaintext"), uint8(recordHandshake))
	f.Add(vec(f, "certificate"), uint8(recordHandshake))
	f.Add([]byte{2, 40}, uint8(recordAlert))
	key, iv := vec(f, "key"), vec(f, "iv")
	f.Fuzz(func(t *testing.T, content []byte, inner uint8) {
		if len(content) > maxPlaintext {
			return
		}
		seal, _ := newDecrypter(0x1301, key, iv)
		plain := append(bytes.Clone(content), inner)
		n := len(plain) + seal.aead.Overhead()
		hdr := []byte{recordApplicationData, 3, 3, byte(n >> 8), byte(n)}
		rec := append(hdr, seal.aead.Seal(nil, iv, plain, hdr)...)
		open, _ := newDecrypter(0x1301, key, iv)
		fl := &CertificateFlight{}
		err := readEncryptedFlight(&limitedReader{r: bytes.NewReader(rec), n: len(rec)}, open, fl)
		if err != nil && !typedError(err) {
			t.Fatalf("untyped error %T: %v", err, err)
		}
		if fl.Alert != nil && !fl.Encrypted {
			t.Fatal("a decrypted alert not marked encrypted")
		}
		for _, c := range fl.Chain {
			if len(c) == 0 {
				t.Fatal("empty certificate")
			}
		}
	})
}

// sealed is one record with the RFC's handshake keys (sequence 0) around
// plain (TLSInnerPlaintext: content, type, padding).
func sealed(t testing.TB, plain []byte) []byte {
	t.Helper()
	d, err := newDecrypter(0x1301, vec(t, "key"), vec(t, "iv"))
	if err != nil {
		t.Fatal(err)
	}
	n := len(plain) + d.aead.Overhead()
	hdr := []byte{recordApplicationData, 3, 3, byte(n >> 8), byte(n)}
	return append(hdr, d.aead.Seal(nil, vec(t, "iv"), plain, hdr)...)
}

// TLS 1.3 lets a server pad its records with zeros after the content type
// (RFC 8446, 5.4); none of the servers in the tests does.
func TestDecryptedPadding(t *testing.T) {
	read := func(rec []byte) (*CertificateFlight, error) {
		dec, _ := newDecrypter(0x1301, vec(t, "key"), vec(t, "iv"))
		f := &CertificateFlight{}
		return f, readEncryptedFlight(&limitedReader{r: bytes.NewReader(rec), n: len(rec)}, dec, f)
	}
	padded := append(append(vec(t, "flightPlaintext"), recordHandshake), make([]byte, 100)...)
	f, err := read(sealed(t, padded))
	if err != nil || len(f.Chain) != 1 || f.SignatureScheme != 0x0804 {
		t.Errorf("padded flight: %+v, %v", f, err)
	}
	if _, err := read(sealed(t, make([]byte, 20))); !errors.Is(err, ErrMalformed) {
		t.Errorf("padding only: %v", err)
	}
}
