package scanner

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"
)

// A self-signed leaf is self-signed whether or not it claims to be a CA (the
// lab's kyber server makes one without the CA flag, as many servers do); a
// leaf signed by another key under the same name is not.
func TestSelfSigned(t *testing.T) {
	mk := func(isCA bool, signer *ecdsa.PrivateKey) *x509.Certificate {
		key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if signer == nil {
			signer = key
		}
		tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "self.test"},
			NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
			BasicConstraintsValid: isCA, IsCA: isCA}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), signer)
		if err != nil {
			t.Fatal(err)
		}
		c, err := x509.ParseCertificate(der)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	other, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	for name, tc := range map[string]struct {
		c    *x509.Certificate
		want bool
	}{
		"self-signed leaf, no CA flag": {mk(false, nil), true},
		"self-signed CA":               {mk(true, nil), true},
		"same name, another key":       {mk(false, other), false},
	} {
		if got := selfSigned(tc.c); got != tc.want {
			t.Errorf("%s: selfSigned = %v, want %v", name, got, tc.want)
		}
	}
}
