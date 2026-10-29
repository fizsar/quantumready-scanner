package scanner

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

// pemChain reads testdata/pqcerts/<name>: certificates written by OpenSSL
// 3.5.9 (the TLS lab's), so the OIDs are read from another implementation's
// certificates, not from Go's.
func pemChain(t *testing.T, name string) [][]byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "pqcerts", name))
	if err != nil {
		t.Fatal(err)
	}
	var chain [][]byte
	for {
		var block *pem.Block
		if block, b = pem.Decode(b); block == nil {
			return chain
		}
		chain = append(chain, block.Bytes)
	}
}

func TestPresentedPostQuantum(t *testing.T) {
	for name, want := range map[string]PresentedCert{
		"slhdsa-sha2-128s.pem": {KeyAlgorithm: "SLH-DSA-SHA2-128s", KeyOID: "2.16.840.1.101.3.4.3.20",
			SignatureAlgorithm: "SLH-DSA-SHA2-128s", SignatureOID: "2.16.840.1.101.3.4.3.20", PostQuantum: true, PostQuantumChain: true},
		"mldsa87-openssl.pem": {KeyAlgorithm: "ML-DSA-87", KeyOID: "2.16.840.1.101.3.4.3.19",
			SignatureAlgorithm: "ML-DSA-87", SignatureOID: "2.16.840.1.101.3.4.3.19", PostQuantum: true, PostQuantumChain: true},
		// A classical key signed by an SLH-DSA CA: the handshake is not
		// post-quantum, the chain is.
		"ecdsa-leaf-slhdsa-ca.pem": {KeyAlgorithm: "ECDSA", KeyOID: "1.2.840.10045.2.1",
			SignatureAlgorithm: "SLH-DSA-SHA2-128s", SignatureOID: "2.16.840.1.101.3.4.3.20", PostQuantum: false, PostQuantumChain: true},
	} {
		got := presented(pemChain(t, name))
		got.SHA256 = ""
		if *got != want {
			t.Errorf("%s:\n%+v\nwant\n%+v", name, *got, want)
		}
	}
	if presented(nil) != nil {
		t.Error("an empty chain presented a certificate")
	}
	if sig, key := oidsOfDER([]byte{0x30, 0x03, 1, 2, 3}); sig != "" || key != "" {
		t.Errorf("garbage: %q %q", sig, key)
	}
}

// crypto/x509 parses an SLH-DSA certificate without its key; the analysis
// names the algorithms by OID.
func TestAnalyzePostQuantumCertificates(t *testing.T) {
	for name, want := range map[string][2]string{
		"slhdsa-sha2-128s.pem":     {"SLH-DSA-SHA2-128s", "SLH-DSA-SHA2-128s"},
		"mldsa87-openssl.pem":      {"ML-DSA-87", "ML-DSA-87"},
		"ecdsa-leaf-slhdsa-ca.pem": {"ECDSA", "SLH-DSA-SHA2-128s"},
	} {
		var chain []*x509.Certificate
		for _, der := range pemChain(t, name) {
			c, err := x509.ParseCertificate(der)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			chain = append(chain, c)
		}
		c := analyzeCertificate(chain, nil, "x.test", testNow, x509.NewCertPool())
		if c.KeyAlgorithm != want[0] || c.SignatureAlgorithm != want[1] || c.Chain[0].KeyAlgorithm != want[0] || c.Chain[0].SignatureAlgorithm != want[1] {
			t.Errorf("%s: key %q, signature %q, chain %+v", name, c.KeyAlgorithm, c.SignatureAlgorithm, c.Chain[0])
		}
	}
}

func TestPostQuantumOIDTable(t *testing.T) {
	if len(pqAlgorithms) != 30 {
		t.Errorf("%d post-quantum OIDs, want 30 (ML-DSA, SLH-DSA, their pre-hash forms)", len(pqAlgorithms))
	}
	for oid, want := range map[string]string{"2.16.840.1.101.3.4.3.17": "ML-DSA-44", "2.16.840.1.101.3.4.3.18": "ML-DSA-65", "2.16.840.1.101.3.4.3.31": "SLH-DSA-SHAKE-256f"} {
		if got, ok := PostQuantumOID(oid); !ok || got != want {
			t.Errorf("%s: %q", oid, got)
		}
	}
	if _, ok := PostQuantumOID("1.2.840.10045.2.1"); ok {
		t.Error("ECDSA is post-quantum")
	}
}
