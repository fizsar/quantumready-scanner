//go:build go1.27

// crypto/mldsa needs Go 1.27; go.mod's minimum is 1.26 (the toolchain, in
// CI and in the images, is 1.27).

package rawtls

import (
	"bytes"
	"crypto/mldsa"
	"crypto/tls"
	"testing"
)

func mldsaCert(t testing.TB) tls.Certificate {
	t.Helper()
	k, err := mldsa.GenerateKey(mldsa.MLDSA65())
	if err != nil {
		t.Fatal(err)
	}
	return certFor(t, k)
}

// What a server with an ML-DSA and an ECDSA certificate presents depends on
// the client's signature schemes; one with ML-DSA only refuses a client
// without them.
func TestExchangeCertificateMLDSA(t *testing.T) {
	ml, ec := mldsaCert(t), testCert(t)
	probe := func(certs []tls.Certificate, schemes []uint16) (*CertificateFlight, error) {
		conn := certServer(t, certs, nil)
		return ExchangeCertificate(conn, CertificateProbe{ServerName: "rawtls.test", Suites: CertificateSuites,
			Groups: []uint16{0x11ec, x25519}, KeyShareGroup: 0x11ec, SignatureSchemes: schemes})
	}
	for name, c := range map[string]struct {
		certs   []tls.Certificate
		schemes []uint16
		leaf    []byte
		scheme  uint16
	}{
		"dual, like Chrome":  {[]tls.Certificate{ml, ec}, chromeSchemes, ml.Certificate[0], 0x0905},
		"dual, like Firefox": {[]tls.Certificate{ml, ec}, firefoxSchemes, ec.Certificate[0], 0x0403},
		"only, like Chrome":  {[]tls.Certificate{ml}, chromeSchemes, ml.Certificate[0], 0x0905},
	} {
		f, err := probe(c.certs, c.schemes)
		if err != nil || len(f.Chain) == 0 || !bytes.Equal(f.Chain[0], c.leaf) || f.SignatureScheme != c.scheme {
			t.Errorf("%s: err %v, scheme 0x%04x", name, err, f.SignatureScheme)
		}
	}
	f, err := probe([]tls.Certificate{ml}, firefoxSchemes)
	if err != nil || f.Alert == nil || f.Alert.Description != 40 || f.Chain != nil || f.ServerHello != nil {
		t.Errorf("only, like Firefox: %+v, %v", f, err)
	}
}
