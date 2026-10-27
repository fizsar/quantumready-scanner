package scoring

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/fizsar/quantumready-saas/internal/scanner"
)

// The two post-quantum certificate servers of the TLS lab (milestone 2.8.5,
// OpenSSL 3.5.9): real scans of 27/10/2026 in testdata/lab_pq, outside
// testdata/lab, whose ten servers keep their results from before.
func labPQ(t *testing.T, name string) *scanner.Result {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "lab_pq", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var r scanner.Result
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	return &r
}

// valid makes the lab's self-signed certificate valid as ideal()'s is
// (chain, name, dates, issuer), keeping what it is: its key, its signature,
// its chain and which one the server chose with and without ML-DSA.
func valid(r *scanner.Result) *scanner.Result {
	c, ok := *r.Certificate, ideal().Certificate
	c.Trusted, c.TrustError, c.TrustErrorCode, c.HostnameMatch, c.SelfSigned = true, "", "", true, false
	c.Expired, c.NotYetValid, c.NotBefore, c.NotAfter, c.ValidityDays, c.DaysToExpiry = false, false, ok.NotBefore, ok.NotAfter, ok.ValidityDays, ok.DaysToExpiry
	c.Issuer, c.IssuerOrg, c.SCTsEmbedded, c.SCTsHandshake = ok.Issuer, ok.IssuerOrg, ok.SCTsEmbedded, ok.SCTsHandshake
	out := *r
	out.Certificate = &c
	return &out
}

// classical is r with an ECDSA P-256 leaf instead, chosen with and without ML-DSA.
func classical(r *scanner.Result) *scanner.Result {
	c := *r.Certificate
	c.KeyAlgorithm, c.KeySize, c.Curve, c.SignatureAlgorithm = "ECDSA", 256, "P-256", "ECDSA-SHA256"
	c.Chain = []scanner.ChainCert{{KeyAlgorithm: "ECDSA", KeySize: 256, PublicKeyOID: ecdsa.KeyOID, SignatureOID: ecdsa.SignatureOID}}
	out := *r
	out.Certificate = &c
	out.CertificateSelection = &scanner.CertificateSelection{WithMLDSA: &ecdsa, WithoutMLDSA: &ecdsa, WithoutStatus: scanner.Supported}
	return &out
}

func TestLabPostQuantumCertificates(t *testing.T) {
	for name, w := range map[string]struct {
		score, validScore int
		grade, validGrade string
		ready, validReady int      // quantum readiness, as scanned and made valid
		pq                []string // post-quantum certificate findings
	}{
		// As scanned, the self-signed certificate caps at F (as in every lab
		// server) and shows no automated renewal. Made valid: mldsa-dual has
		// OpenSSL's TLS 1.2 CBC suites (SUITE_CBC, cap B); mldsa-only is TLS
		// 1.3 only, and its missing fallback takes no points (2026.3).
		"mldsa-dual": {75, 100, "F", "B", 75, 90, []string{"CERT_PQ_SIGNATURE"}},
		"mldsa-only": {75, 100, "F", "A", 75, 90, []string{"CERT_PQ_SIGNATURE", "CERT_PQ_NO_CLASSICAL_FALLBACK"}},
	} {
		r := labPQ(t, name)
		raw, v, cl := Evaluate(r), Evaluate(valid(r)), Evaluate(valid(classical(r)))
		t.Logf("%s: as scanned %d %s; certificate made valid %d %s (with ECDSA instead: %d %s); %v", name, raw.Score, raw.Grade, v.Score, v.Grade, cl.Score, cl.Grade, codes(v))
		if raw.Score != w.score || raw.Grade != w.grade || v.Score != w.validScore || v.Grade != w.validGrade ||
			raw.Readiness.Score != w.ready || v.Readiness.Score != w.validReady {
			t.Errorf("%s: %d %s %d / %d %s %d; want %d %s %d / %d %s %d", name, raw.Score, raw.Grade, raw.Readiness.Score, v.Score, v.Grade, v.Readiness.Score,
				w.score, w.grade, w.ready, w.validScore, w.validGrade, w.validReady)
		}
		// 2026.3: the post-quantum certificate adds 15 to readiness and
		// nothing to security today, where it counts as an ECDSA key would.
		if v.Score != cl.Score || v.Grade != cl.Grade || !slices.Equal(blockPoints(v), blockPoints(cl)) || v.Readiness.Score != cl.Readiness.Score+ReadyPQCert {
			t.Errorf("%s: %d %s %d with ML-DSA, %d %s %d with ECDSA", name, v.Score, v.Grade, v.Readiness.Score, cl.Score, cl.Grade, cl.Readiness.Score)
		}
		for _, code := range []string{"CERT_PQ_SIGNATURE", "CERT_PQ_NO_CLASSICAL_FALLBACK"} {
			if slices.Contains(codes(v), code) != slices.Contains(w.pq, code) {
				t.Errorf("%s: %s present %v, want %v", name, code, slices.Contains(codes(v), code), slices.Contains(w.pq, code))
			}
		}
		if slices.Contains(codes(v), "CERT_CLASSICAL_SIGNATURE") || slices.Contains(codes(v), "CERT_WEAK_KEY") {
			t.Errorf("%s: an ML-DSA-65 key judged classical or weak: %v", name, codes(v))
		}
	}
}
