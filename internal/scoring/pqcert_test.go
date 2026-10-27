package scoring

import (
	"slices"
	"strings"
	"testing"

	"github.com/fizsar/quantumready-saas/internal/catalog"
	"github.com/fizsar/quantumready-saas/internal/scanner"
)

var (
	mldsa65 = scanner.PresentedCert{KeyAlgorithm: "ML-DSA-65", KeyOID: "2.16.840.1.101.3.4.3.18", SignatureOID: "2.16.840.1.101.3.4.3.18", SHA256: "aa", PostQuantum: true, PostQuantumChain: true}
	ecdsa   = scanner.PresentedCert{KeyAlgorithm: "ECDSA", KeyOID: "1.2.840.10045.2.1", SignatureOID: "1.2.840.10045.4.3.2", SHA256: "bb"}
)

// pqCertificate is ideal() with an ML-DSA-65 leaf: what the scanner saw
// offering ML-DSA, and without (status, and the certificate if any).
func pqCertificate(status scanner.Status, without *scanner.PresentedCert) *scanner.Result {
	return apply(func(r *scanner.Result) {
		c := r.Certificate
		c.KeyAlgorithm, c.KeySize, c.Curve, c.SignatureAlgorithm = "ML-DSA-65", 0, "", "ECDSA-SHA256"
		c.Chain = []scanner.ChainCert{{KeyAlgorithm: "ML-DSA-65", PublicKeyOID: mldsa65.KeyOID, SignatureOID: "1.2.840.10045.4.3.2"}}
		r.CertificateSelection = &scanner.CertificateSelection{WithMLDSA: &mldsa65, WithoutMLDSA: without, WithoutStatus: status}
	})
}

func findingOf(res Result, code string) *Finding {
	for i := range res.Findings {
		if res.Findings[i].Code == code {
			return &res.Findings[i]
		}
	}
	return nil
}

func TestPostQuantumCertificate(t *testing.T) {
	classical := Evaluate(ideal())
	other := ecdsa
	for name, c := range map[string]struct {
		in       *scanner.Result
		fallback bool
	}{
		"dual: a classical one without ML-DSA":       {pqCertificate(scanner.Supported, &other), false},
		"only: refused without ML-DSA":               {pqCertificate(scanner.NotSupported, nil), true},
		"only: the same ML-DSA one without it":       {pqCertificate(scanner.Supported, &mldsa65), true},
		"could not tell (an error, or basic)":        {pqCertificate(scanner.ProbeFailed, nil), false},
		"not probed (intensity basic, empty status)": {pqCertificate("", nil), false},
	} {
		res := Evaluate(c.in)
		pq, fb := findingOf(res, "CERT_PQ_SIGNATURE"), findingOf(res, "CERT_PQ_NO_CLASSICAL_FALLBACK")
		if pq == nil || pq.Axis != AxisQuantum || pq.Severity != Info || pq.Params["algorithm"] != "ML-DSA-65" {
			t.Errorf("%s: CERT_PQ_SIGNATURE %+v", name, pq)
		}
		if (fb != nil) != c.fallback || fb != nil && (fb.Axis != AxisCurrent || fb.Severity != Medium) {
			t.Errorf("%s: CERT_PQ_NO_CLASSICAL_FALLBACK %+v", name, fb)
		}
		if findingOf(res, "CERT_CLASSICAL_SIGNATURE") != nil {
			t.Errorf("%s: an ML-DSA leaf is not a classical signature", name)
		}
		// 2026.3: security today as with the ECDSA one; readiness +15, and
		// the missing fallback takes nothing away on either axis.
		if res.Readiness.Score != classical.Readiness.Score+ReadyPQCert {
			t.Errorf("%s: readiness %d, the ECDSA one %d", name, res.Readiness.Score, classical.Readiness.Score)
		}
		if res.Score != classical.Score || res.Grade != classical.Grade || !slices.Equal(blockPoints(res), blockPoints(classical)) {
			t.Errorf("%s: %d %s %v, the ECDSA one %d %s %v", name, res.Score, res.Grade, blockPoints(res), classical.Score, classical.Grade, blockPoints(classical))
		}
	}
}

func blockPoints(r Result) []int {
	var out []int
	for _, b := range r.Blocks {
		out = append(out, b.Points)
		for _, it := range b.Items {
			out = append(out, it.Points)
		}
	}
	return out
}

// CERT_CLASSICAL_SIGNATURE says what the scan saw: that the server chose its
// classical certificate though offered ML-DSA; for scans from before 2.8.5,
// nothing about it, and no claim about the state of the web either.
func TestClassicalSignatureText(t *testing.T) {
	offered := apply(func(r *scanner.Result) {
		r.CertificateSelection = &scanner.CertificateSelection{WithMLDSA: &ecdsa, WithoutMLDSA: &ecdsa, WithoutStatus: scanner.Supported}
	})
	for name, c := range map[string]struct {
		in   *scanner.Result
		says bool
	}{"offered ML-DSA": {offered, true}, "before 2.8.5": {ideal(), false}} {
		f := findingOf(Evaluate(c.in), "CERT_CLASSICAL_SIGNATURE")
		if f == nil || (f.Params["pq_offered"] == "1") != c.says {
			t.Fatalf("%s: %+v", name, f)
		}
		for _, lang := range catalog.Languages {
			d := catalog.Describe(f.Code, f.Params, lang).Detail
			if strings.Contains(d, "ML-DSA") != c.says || strings.Contains(d, "web pública") || strings.Contains(d, "public web") || strings.Contains(d, "  ") {
				t.Errorf("%s/%s: %q", name, lang, d)
			}
		}
	}
}
