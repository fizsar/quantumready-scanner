//go:build go1.27

// crypto/mldsa needs Go 1.27; go.mod's minimum is 1.26 (the toolchain, in
// CI and in the images, is 1.27).

package scanner

import (
	"crypto/mldsa"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"slices"
	"testing"
	"time"
)

// mldsaLeaf is an ML-DSA-65 certificate for testHost, signed by the test CA.
func (p *testPKI) mldsaLeaf(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := mldsa.GenerateKey(mldsa.MLDSA65())
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: testHost},
		DNSNames: []string{testHost}, NotBefore: testNow.AddDate(0, 0, -10), NotAfter: testNow.AddDate(0, 0, 80),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, p.ca, key.Public(), p.caKey)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der, p.ca.Raw}, PrivateKey: key}
}

func TestCertificateSelection(t *testing.T) {
	pki := newTestPKI(t)
	ml, ec := pki.mldsaLeaf(t), pki.leaf(t, leafOpts{})
	servers := map[string]*tls.Config{
		"dual":      {Certificates: []tls.Certificate{ml, ec}},
		"only":      {Certificates: []tls.Certificate{ml}, MinVersion: tls.VersionTLS13},
		"classical": {Certificates: []tls.Certificate{ec}},
		"tls12":     {Certificates: []tls.Certificate{ec}, MaxVersion: tls.VersionTLS12},
	}
	scanOf := func(name string, intensity Intensity) *Result {
		return scan(t, newTestScanner(pki, Options{Intensity: intensity}), startTLS(t, servers[name]))
	}
	noPQCodes := func(t *testing.T, r *Result) {
		t.Helper()
		if slices.Contains(r.NotEvaluated, NotEvalPQCertificate) || slices.Contains(r.NotEvaluated, NotEvalPQCertificateFallback) {
			t.Errorf("not evaluated %v", r.NotEvaluated)
		}
	}

	t.Run("dual", func(t *testing.T) {
		r := scanOf("dual", IntensityFull)
		s := r.CertificateSelection
		if !s.WithMLDSA.PostQuantum || s.WithMLDSA.KeyAlgorithm != "ML-DSA-65" || s.WithoutStatus != Supported ||
			s.WithoutMLDSA.PostQuantum || s.WithoutMLDSA.KeyAlgorithm != "ECDSA" || s.WithMLDSA.SHA256 == s.WithoutMLDSA.SHA256 {
			t.Errorf("with %+v\nwithout %+v (%s)", s.WithMLDSA, s.WithoutMLDSA, s.WithoutStatus)
		}
		if r.Certificate.KeyAlgorithm != "ML-DSA-65" || r.Certificate.Chain[0].PublicKeyOID != "2.16.840.1.101.3.4.3.18" {
			t.Errorf("certificate %s %+v", r.Certificate.KeyAlgorithm, r.Certificate.Chain[0])
		}
		noPQCodes(t, r)
		// The log shows each certificate where it came.
		def := logged(r, PurposeDefault, "")[0].Response.Certificate
		probe := logged(r, PurposeCertificate, "without ML-DSA")
		if def == nil || def.KeyAlgorithm != "ML-DSA-65" || len(probe) != 1 || probe[0].Client != clientRaw ||
			probe[0].Response.Certificate.KeyAlgorithm != "ECDSA" || probe[0].Response.HandshakeSignature != 0x0403 {
			t.Errorf("default %+v, probe %+v", def, probe)
		}
		if o := offerOfRecord(t, r, probe[0]); slices.ContainsFunc(o.SignatureSchemes, func(s uint16) bool { return s >= 0x0904 && s <= 0x0906 }) ||
			!slices.Equal(o.SignatureSchemes, FirefoxSignatureSchemes) || !slices.Equal(o.SupportedVersions, []uint16{tls.VersionTLS13}) {
			t.Errorf("certificate probe offered %+v", o)
		}
	})
	t.Run("only", func(t *testing.T) {
		r := scanOf("only", IntensityFull)
		s := r.CertificateSelection
		if !s.WithMLDSA.PostQuantum || s.WithoutMLDSA != nil || s.WithoutStatus != NotSupported || s.WithoutErrorCode != "alert:40" {
			t.Errorf("%+v", s)
		}
		noPQCodes(t, r)
	})
	t.Run("only, basic", func(t *testing.T) {
		r := scanOf("only", IntensityBasic)
		if s := r.CertificateSelection; !s.WithMLDSA.PostQuantum || s.WithoutStatus != "" || !slices.Contains(r.NotEvaluated, NotEvalPQCertificateFallback) {
			t.Errorf("%+v, %v", s, r.NotEvaluated)
		}
	})
	t.Run("classical", func(t *testing.T) {
		r := scanOf("classical", IntensityFull)
		s := r.CertificateSelection
		if s.WithMLDSA.PostQuantum || s.WithoutStatus != Supported || s.WithoutMLDSA.SHA256 != s.WithMLDSA.SHA256 {
			t.Errorf("with %+v\nwithout %+v", s.WithMLDSA, s.WithoutMLDSA)
		}
		noPQCodes(t, r)
	})
	t.Run("TLS 1.2 only: ML-DSA does not exist there", func(t *testing.T) {
		r := scanOf("tls12", IntensityFull)
		if s := r.CertificateSelection; s.WithMLDSA.PostQuantum || s.WithoutStatus != "" || len(logged(r, PurposeCertificate, "without ML-DSA")) != 0 {
			t.Errorf("%+v", s)
		}
		noPQCodes(t, r)
	})
}
