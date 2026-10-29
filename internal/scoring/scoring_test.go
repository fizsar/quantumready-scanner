package scoring

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fizsar/quantumready-saas/internal/catalog"
	"github.com/fizsar/quantumready-saas/internal/scanner"
)

var allNotEvaluated = []string{
	scanner.NotEvalSSL, scanner.NotEvalDHESuites, scanner.NotEvalOtherSuites, scanner.NotEvalTLS13Suites,
	scanner.NotEvalServerPreference, scanner.NotEvalLegacyGroups, scanner.NotEvalNonStandardPQC, scanner.NotEvalImplementationBug, scanner.NotEvalCredentialSuites,
}

func groups(statuses map[string]scanner.Status) []scanner.GroupProbe {
	order := []struct {
		name   string
		hybrid bool
	}{
		{"X25519MLKEM768", true}, {"SecP256r1MLKEM768", true}, {"SecP384r1MLKEM1024", true},
		{"x25519", false}, {"secp256r1", false}, {"secp384r1", false}, {"secp521r1", false},
	}
	var out []scanner.GroupProbe
	for _, g := range order {
		st, ok := statuses[g.name]
		if !ok {
			st = scanner.NotSupported
		}
		out = append(out, scanner.GroupProbe{Name: g.name, Hybrid: g.hybrid, Status: st})
	}
	return out
}

func versions(supported ...string) []scanner.VersionProbe {
	var out []scanner.VersionProbe
	for _, v := range []string{"TLS 1.0", "TLS 1.1", "TLS 1.2", "TLS 1.3"} {
		st := scanner.NotSupported
		if slices.Contains(supported, v) {
			st = scanner.Supported
		}
		out = append(out, scanner.VersionProbe{Version: v, Status: st})
	}
	return out
}

func suite(name string) scanner.CipherSuite {
	return scanner.CipherSuite{
		Name:           name,
		ForwardSecrecy: strings.HasPrefix(name, "TLS_ECDHE_"),
		AEAD:           strings.Contains(name, "_GCM_") || strings.Contains(name, "CHACHA20"),
		Insecure:       strings.Contains(name, "RC4") || strings.Contains(name, "3DES") || strings.HasPrefix(name, "TLS_RSA_"),
	}
}

// ideal: PQC preferred with all three hybrids, TLS 1.2+1.3 with forward
// secrecy and AEAD only, a Let's Encrypt ECDSA certificate.
func ideal() *scanner.Result {
	return &scanner.Result{
		SchemaVersion: scanner.SchemaVersion,
		Host:          "example.com",
		Port:          443,
		Default:       scanner.Handshake{Status: scanner.Supported, Version: "TLS 1.3", CipherSuite: "TLS_AES_128_GCM_SHA256", Group: "X25519MLKEM768"},
		KeyExchange: scanner.KeyExchange{
			Selected:       "X25519MLKEM768",
			SelectedHybrid: true,
			Groups: groups(map[string]scanner.Status{
				"X25519MLKEM768": scanner.Supported, "SecP256r1MLKEM768": scanner.Supported, "SecP384r1MLKEM1024": scanner.Supported,
				"x25519": scanner.Supported, "secp256r1": scanner.Supported,
			}),
		},
		Versions: versions("TLS 1.2", "TLS 1.3"),
		TLS12Suites: []scanner.CipherSuite{
			suite("TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256"),
			suite("TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256"),
		},
		TLS12SuitesComplete: true, LegacySuitesComplete: true,
		TLS13Suite: "TLS_AES_128_GCM_SHA256",
		Certificate: &scanner.Certificate{
			Subject: "example.com", SANs: []string{"example.com"},
			Issuer: "YE1", IssuerOrg: []string{"Let's Encrypt"},
			NotBefore: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), NotAfter: time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC),
			ValidityDays: 90, DaysToExpiry: 60,
			KeyAlgorithm: "ECDSA", KeySize: 256, Curve: "P-256", SignatureAlgorithm: "ECDSA-SHA384",
			Trusted: true, HostnameMatch: true, SCTsEmbedded: 2,
		},
		NotEvaluated: allNotEvaluated,
	}
}

func singleHybrid(r *scanner.Result) {
	r.KeyExchange.Groups = groups(map[string]scanner.Status{
		"X25519MLKEM768": scanner.Supported, "x25519": scanner.Supported, "secp256r1": scanner.Supported,
	})
}

func noPQC(r *scanner.Result) {
	r.Default.Group = "x25519"
	r.KeyExchange.Selected, r.KeyExchange.SelectedHybrid = "x25519", false
	r.KeyExchange.Groups = groups(map[string]scanner.Status{"x25519": scanner.Supported, "secp256r1": scanner.Supported})
}

func commercialRSA(r *scanner.Result) {
	c := r.Certificate
	c.Issuer, c.IssuerOrg = "DigiCert Global G2 TLS RSA SHA256 2020 CA1", []string{"DigiCert Inc"}
	c.ValidityDays, c.DaysToExpiry = 199, 150
	c.KeyAlgorithm, c.KeySize, c.Curve, c.SignatureAlgorithm = "RSA", 2048, "", "SHA256-RSA"
}

func apply(fs ...func(*scanner.Result)) *scanner.Result {
	r := ideal()
	for _, f := range fs {
		f(r)
	}
	return r
}

type golden struct {
	name     string
	in       *scanner.Result
	score    int
	grade    string
	cap      string   // "" if none
	codes    []string // must be present
	notCodes []string // must be absent
	quantum  string
	current  string
	blockPts map[string]int // security today
	readyPts map[string]int // quantum readiness
	// readiness is "<score> <level>" of the quantum readiness axis ("": not checked).
	readiness string
}

func tls12Only(r *scanner.Result) {
	r.Default.Version, r.Versions, r.TLS13Suite = "TLS 1.2", versions("TLS 1.2"), ""
	r.Default.CipherSuite = "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256"
}

// kyber adds the obsolete Kyber draft codepoint, as the raw probes see it.
func kyber(r *scanner.Result) {
	r.Intensity = scanner.IntensityFull
	r.Raw = &scanner.RawResult{SuitesComplete: true, TLS13SuitesComplete: true, TLS13GroupsComplete: true, TLS12CurvesComplete: true,
		SSLv2: scanner.RawProtocol{Status: scanner.NotSupported}, SSLv3: scanner.RawProtocol{Status: scanner.NotSupported},
		TLS13Groups: []scanner.RawGroup{{Name: "X25519Kyber768Draft00", ID: 0x6399, Kind: "kyber_draft"}}}
}

// pqCert gives the leaf an ML-DSA-65 key, presented when ML-DSA is offered
// (with a classical certificate for clients without it).
func pqCert(r *scanner.Result) {
	c := r.Certificate
	c.KeyAlgorithm, c.KeySize, c.Curve = "ML-DSA-65", 0, ""
	c.Chain = []scanner.ChainCert{{KeyAlgorithm: "ML-DSA-65", PublicKeyOID: "2.16.840.1.101.3.4.3.18", SignatureOID: "1.2.840.10045.4.3.2"}}
	ec := scanner.PresentedCert{KeyAlgorithm: "ECDSA", KeyOID: "1.2.840.10045.2.1"}
	r.CertificateSelection = &scanner.CertificateSelection{WithMLDSA: &scanner.PresentedCert{KeyAlgorithm: "ML-DSA-65", KeyOID: "2.16.840.1.101.3.4.3.18", PostQuantum: true},
		WithoutMLDSA: &ec, WithoutStatus: scanner.Supported}
}

// Model 2026.3. Security today: TLS 1.3 30, forward secrecy 25, certificate
// 45 (valid 15, key 10, signature 10, automated renewal 10). Quantum
// readiness: hybrid ML-KEM preferred 50 (accepted alone 30), TLS 1.3 10, more
// than one standard group 10, automated renewal 15, post-quantum certificate
// 15, obsolete Kyber -15 (not below 0).
func goldenCases() []golden {
	return []golden{
		{
			name: "ideal: PQC preferred, three hybrids", in: ideal(),
			score: 100, grade: "A", readiness: "85 ready", quantum: "low", current: "none",
			codes:    []string{"KEX_HYBRID_PREFERRED", "KEX_MULTIPLE_HYBRIDS", "TLS13_SUPPORTED", "FS_ALL", "CERT_VALID", "CERT_AUTOMATED_ACME", "CERT_CLASSICAL_SIGNATURE", "CERT_CT", "SYM_AES128"},
			notCodes: []string{"SCAN_PARTIAL"},
			blockPts: map[string]int{"tls13": 30, "forward_secrecy": 25, "certificate": 45},
			readyPts: map[string]int{"key_exchange": 50, "tls13": 10, "hybrid_variety": 10, "cert_automation": 15, "pq_certificate": 0},
		},
		{
			name: "ideal with a post-quantum certificate: readiness 100", in: apply(pqCert),
			score: 100, grade: "A", readiness: "100 ready", codes: []string{"CERT_PQ_SIGNATURE"}, notCodes: []string{"CERT_CLASSICAL_SIGNATURE"},
			readyPts: map[string]int{"pq_certificate": 15},
		},
		{
			name: "like fizsar.com today: PQC preferred, one hybrid", in: apply(singleHybrid),
			score: 100, grade: "A", readiness: "75 ready", quantum: "low", current: "none",
			codes: []string{"KEX_HYBRID_PREFERRED", "KEX_SINGLE_HYBRID"},
		},
		{
			name: "impeccable classical TLS without PQC: A today, not ready", in: apply(noPQC),
			score: 100, grade: "A", readiness: "25 not_ready", quantum: "critical", current: "none",
			codes:    []string{"KEX_NO_HYBRID", "TLS13_SUPPORTED", "FS_ALL", "CERT_VALID"},
			readyPts: map[string]int{"key_exchange": 0, "tls13": 10, "hybrid_variety": 0, "cert_automation": 15},
		},
		{
			name: "classical TLS with a commercial RSA-2048 certificate", in: apply(noPQC, commercialRSA),
			score: 90, grade: "A", readiness: "10 not_ready", quantum: "critical", current: "none",
			codes: []string{"KEX_NO_HYBRID", "CERT_NOT_AUTOMATED"},
		},
		{
			name: "ML-KEM supported but not preferred: on track", in: apply(noPQC, singleHybrid),
			score: 100, grade: "A", readiness: "55 on_track", quantum: "high",
			codes: []string{"KEX_HYBRID_NOT_PREFERRED"}, notCodes: []string{"KEX_NO_HYBRID", "KEX_HYBRID_PREFERRED"},
		},
		{
			name: "obsolete Kyber alone: below no PQC at all", in: apply(noPQC, kyber),
			score: 100, grade: "A", readiness: "10 not_ready", codes: []string{"KEX_EXPERIMENTAL_PQC_ONLY"},
			readyPts: map[string]int{"kyber_draft": -15},
		},
		{
			name: "obsolete Kyber besides ML-KEM still costs 15", in: apply(kyber),
			score: 100, grade: "A", readiness: "70 ready", codes: []string{"KEX_EXPERIMENTAL_PQC_EXTRA"},
		},
		{
			name: "the Kyber penalty stops at 0", in: apply(noPQC, commercialRSA, kyber),
			score: 90, grade: "A", readiness: "0 not_ready",
		},
		{
			name:  "TLS 1.2 only, well configured -> C (no floor since 2026.3)",
			in:    apply(noPQC, tls12Only),
			score: 70, grade: "C", readiness: "15 not_ready", quantum: "critical", current: "none",
			codes:    []string{"TLS13_MISSING", "KEX_NO_HYBRID", "FS_ALL"},
			blockPts: map[string]int{"tls13": 0, "forward_secrecy": 25, "certificate": 45},
			readyPts: map[string]int{"key_exchange": 0, "tls13": 0, "hybrid_variety": 0, "cert_automation": 15},
		},
		{
			name:  "TLS 1.2 only with a commercial certificate",
			in:    apply(noPQC, tls12Only, commercialRSA),
			score: 60, grade: "C", readiness: "0 not_ready", codes: []string{"TLS13_MISSING", "CERT_NOT_AUTOMATED"},
		},
		{
			name: "TLS 1.2 only with CBC suites: the B cap does not lower a C",
			in: apply(noPQC, tls12Only, func(r *scanner.Result) {
				r.TLS12Suites = append(r.TLS12Suites, suite("TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA"))
			}),
			score: 70, grade: "C", cap: "B", codes: []string{"SUITE_CBC"},
		},
		{
			name: "TLS 1.2 only with RC4: C",
			in: apply(noPQC, tls12Only, func(r *scanner.Result) {
				r.TLS12Suites = append(r.TLS12Suites, suite("TLS_ECDHE_ECDSA_WITH_RC4_128_SHA"))
			}),
			score: 70, grade: "C", cap: "C", codes: []string{"SUITE_RC4"},
		},
		{
			name: "expired certificate caps at F", in: apply(func(r *scanner.Result) {
				r.Certificate.Expired, r.Certificate.Trusted, r.Certificate.DaysToExpiry = true, false, -3
			}),
			score: 85, grade: "F", cap: "F", current: "critical",
			codes: []string{"CERT_EXPIRED"}, notCodes: []string{"CERT_VALID", "CERT_UNTRUSTED"},
		},
		{
			name: "self-signed caps at F", in: apply(func(r *scanner.Result) {
				r.Certificate.Trusted, r.Certificate.SelfSigned, r.Certificate.TrustError = false, true, "x509: certificate signed by unknown authority"
			}),
			score: 85, grade: "F", cap: "F", codes: []string{"CERT_SELF_SIGNED"},
		},
		{
			name: "hostname mismatch caps at F", in: apply(func(r *scanner.Result) { r.Certificate.HostnameMatch = false }),
			score: 85, grade: "F", cap: "F", codes: []string{"CERT_HOSTNAME_MISMATCH"},
		},
		{
			name: "TLS 1.0/1.1 and CBC cap at B", in: apply(func(r *scanner.Result) {
				r.Versions = versions("TLS 1.0", "TLS 1.1", "TLS 1.2", "TLS 1.3")
				r.TLS12Suites = append(r.TLS12Suites, suite("TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA"))
			}),
			score: 100, grade: "B", cap: "B", current: "medium", codes: []string{"PROTO_LEGACY_TLS", "SUITE_CBC", "FS_ALL"},
		},
		{
			name: "RC4 and 3DES cap at C", in: apply(func(r *scanner.Result) {
				r.TLS12Suites = append(r.TLS12Suites, suite("TLS_ECDHE_RSA_WITH_RC4_128_SHA"), suite("TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA"))
			}),
			score: 100, grade: "C", cap: "C", current: "high", codes: []string{"SUITE_RC4", "SUITE_3DES"},
		},
		{
			// cloudflare.com, 2026-10-13: 3DES only in TLS 1.0 (found by testssl.sh, missed before).
			name: "3DES only in TLS 1.0 caps at C", in: apply(func(r *scanner.Result) {
				r.Versions = versions("TLS 1.0", "TLS 1.1", "TLS 1.2", "TLS 1.3")
				r.TLS10Suites = []scanner.CipherSuite{suite("TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256"), suite("TLS_RSA_WITH_3DES_EDE_CBC_SHA")}
			}),
			// The TLS 1.0 RSA suite also costs the forward secrecy block.
			score: 75, grade: "C", cap: "C", current: "high", codes: []string{"SUITE_3DES", "PROTO_LEGACY_TLS", "FS_PARTIAL"},
		},
		{
			name: "RSA-1024 caps at C and loses the key points", in: apply(func(r *scanner.Result) {
				r.Certificate.KeyAlgorithm, r.Certificate.KeySize, r.Certificate.Curve, r.Certificate.SignatureAlgorithm = "RSA", 1024, "", "SHA256-RSA"
			}),
			score: 90, grade: "C", cap: "C", codes: []string{"CERT_WEAK_KEY"},
		},
		{
			name: "SHA-1 signature caps at C", in: apply(func(r *scanner.Result) { r.Certificate.SignatureAlgorithm = "SHA1-RSA" }),
			score: 90, grade: "C", cap: "C", codes: []string{"CERT_WEAK_SIGNATURE"},
		},
		{
			name: "partial forward secrecy earns nothing of the block", in: apply(func(r *scanner.Result) {
				r.TLS12Suites = append(r.TLS12Suites, suite("TLS_RSA_WITH_AES_128_GCM_SHA256"))
			}),
			score: 75, grade: "B", current: "medium", codes: []string{"FS_PARTIAL"}, notCodes: []string{"FS_ALL", "FS_NONE"},
			blockPts: map[string]int{"forward_secrecy": 0},
		},
		{
			name: "RSA key exchange only, no TLS 1.3, commercial certificate -> F by its score (no floor)",
			in: apply(noPQC, commercialRSA, func(r *scanner.Result) {
				r.Default = scanner.Handshake{Status: scanner.NotSupported}
				r.KeyExchange.Selected = ""
				r.KeyExchange.Groups = groups(nil)
				r.Versions, r.TLS13Suite = versions("TLS 1.2"), ""
				r.TLS12Suites = []scanner.CipherSuite{suite("TLS_RSA_WITH_AES_128_GCM_SHA256")}
			}),
			score: 35, grade: "F", cap: "C", readiness: "0 not_ready", codes: []string{"FS_NONE", "TLS13_MISSING", "KEX_NO_HYBRID"},
		},
		{
			name: "TLS 1.0 only with RSA key exchange: no forward secrecy in any version",
			in: apply(noPQC, func(r *scanner.Result) {
				r.Default = scanner.Handshake{Status: scanner.NotSupported}
				r.KeyExchange.Selected = ""
				r.KeyExchange.Groups = groups(nil)
				r.Versions, r.TLS13Suite, r.TLS12Suites = versions("TLS 1.0"), "", nil
				r.TLS10Suites = []scanner.CipherSuite{suite("TLS_RSA_WITH_AES_128_CBC_SHA")}
			}),
			score: 45, grade: "D", cap: "C", codes: []string{"FS_NONE", "PROTO_LEGACY_TLS", "SUITE_CBC"}, notCodes: []string{"FS_ALL"},
		},
		{
			name: "TLS 1.3 only has forward secrecy by design", in: apply(func(r *scanner.Result) {
				r.Versions, r.TLS12Suites = versions("TLS 1.3"), nil
			}),
			score: 100, grade: "A", codes: []string{"FS_ALL"},
		},
		{
			name: "commercial certificate valid for 47 days counts as automated", in: apply(commercialRSA, func(r *scanner.Result) {
				r.Certificate.ValidityDays = 47
			}),
			score: 100, grade: "A", readiness: "85 ready", codes: []string{"CERT_AUTOMATED_SHORT"}, notCodes: []string{"CERT_NOT_AUTOMATED"},
		},
		{
			name: "expiring soon is a warning, not a cap", in: apply(func(r *scanner.Result) { r.Certificate.DaysToExpiry = 5 }),
			score: 100, grade: "A", current: "medium", codes: []string{"CERT_EXPIRES_SOON"},
		},
	}
}

func TestEvaluateGolden(t *testing.T) {
	for _, tc := range goldenCases() {
		t.Run(tc.name, func(t *testing.T) {
			got := Evaluate(tc.in)
			if got.ScoringVersion != Version {
				t.Errorf("scoring_version = %q, want %q", got.ScoringVersion, Version)
			}
			if got.Score != tc.score || got.Grade != tc.grade {
				t.Errorf("score/grade = %d %s, want %d %s (blocks %+v)", got.Score, got.Grade, tc.score, tc.grade, got.Blocks)
			}
			if got.ScoreGrade != GradeFor(tc.score) {
				t.Errorf("score_grade = %s, want %s", got.ScoreGrade, GradeFor(tc.score))
			}
			if got.FloorPoints != 0 {
				t.Errorf("floor_points = %d: no floor since 2026.3", got.FloorPoints)
			}
			sum := 0
			for _, b := range got.Blocks {
				sum += b.Points
			}
			if sum != got.Score {
				t.Errorf("blocks = %d, but score = %d", sum, got.Score)
			}
			r := got.Readiness
			ready := 0
			for _, b := range r.Blocks {
				ready += b.Points
			}
			if r.Score != max(ready, 0) || r.Level != LevelFor(r.Score) {
				t.Errorf("readiness %d %s, blocks add up to %d", r.Score, r.Level, ready)
			}
			if tc.readiness != "" && fmt.Sprintf("%d %s", r.Score, r.Level) != tc.readiness {
				t.Errorf("readiness = %d %s, want %s (%+v)", r.Score, r.Level, tc.readiness, r.Blocks)
			}
			gotCap := ""
			if got.Cap != nil {
				gotCap = got.Cap.Grade
			}
			if gotCap != tc.cap {
				t.Errorf("cap = %q, want %q (%+v)", gotCap, tc.cap, got.Cap)
			}
			codes := map[string]bool{}
			for _, f := range got.Findings {
				codes[f.Code] = true
			}
			for _, c := range tc.codes {
				if !codes[c] {
					t.Errorf("missing finding %s (got %v)", c, keys(codes))
				}
			}
			for _, c := range tc.notCodes {
				if codes[c] {
					t.Errorf("unexpected finding %s", c)
				}
			}
			if tc.quantum != "" && got.QuantumRisk != tc.quantum {
				t.Errorf("quantum risk = %s, want %s", got.QuantumRisk, tc.quantum)
			}
			if tc.current != "" && got.CurrentRisk != tc.current {
				t.Errorf("current risk = %s, want %s", got.CurrentRisk, tc.current)
			}
			for _, axis := range []struct {
				name   string
				want   map[string]int
				blocks []Block
			}{{"security", tc.blockPts, got.Blocks}, {"readiness", tc.readyPts, got.Readiness.Blocks}} {
				for id, want := range axis.want {
					i := slices.IndexFunc(axis.blocks, func(b Block) bool { return b.ID == id })
					if i < 0 || axis.blocks[i].Points != want {
						t.Errorf("%s block %s: %+v, want %d", axis.name, id, axis.blocks, want)
					}
				}
			}
		})
	}
}

func keys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// What crypto/tls cannot evaluate never adds or removes points or findings.
func TestNotEvaluatedNeverAffectsTheScore(t *testing.T) {
	for _, tc := range goldenCases() {
		with := *tc.in
		with.NotEvaluated = allNotEvaluated
		without := *tc.in
		without.NotEvaluated = nil
		a, b := Evaluate(&with), Evaluate(&without)
		a.NotEvaluated, b.NotEvaluated = nil, nil
		if !reflect.DeepEqual(a, b) {
			t.Errorf("%s: result changes with NotEvaluated:\nwith:    %+v\nwithout: %+v", tc.name, a, b)
		}
	}
	if got := Evaluate(ideal()).NotEvaluated; !slices.Equal(got, allNotEvaluated) {
		t.Errorf("NotEvaluated not passed through for display: %v", got)
	}
}

// A probe error is never read as "supported" nor as "not supported".
func TestProbeErrors(t *testing.T) {
	r := apply(noPQC, func(r *scanner.Result) {
		for i := range r.KeyExchange.Groups {
			if r.KeyExchange.Groups[i].Hybrid {
				r.KeyExchange.Groups[i].Status = scanner.ProbeFailed
			}
		}
		r.Versions[3].Status = scanner.ProbeFailed // TLS 1.3 probe errored…
		r.Default.Version = "TLS 1.2"              // …and the default handshake did not show it
		r.TLS12SuitesComplete = false
	})
	got := Evaluate(r)
	codes := map[string]bool{}
	for _, f := range got.Findings {
		codes[f.Code] = true
	}
	for _, c := range []string{"KEX_HYBRID_UNKNOWN", "SCAN_PARTIAL"} {
		if !codes[c] {
			t.Errorf("missing %s", c)
		}
	}
	for _, c := range []string{"KEX_NO_HYBRID", "KEX_HYBRID_NOT_PREFERRED", "TLS13_MISSING", "TLS13_SUPPORTED"} {
		if codes[c] {
			t.Errorf("%s must not be claimed from probe errors", c)
		}
	}
	for _, b := range got.Blocks {
		if (b.ID == "key_exchange" || b.ID == "tls13") && b.Points != 0 {
			t.Errorf("block %s = %d points from a probe error, want 0", b.ID, b.Points)
		}
	}
}

// Suites only accepted in TLS 1.0/1.1 are judged and labeled with their
// versions; suites also in TLS 1.2 are not repeated.
func TestLegacySuites(t *testing.T) {
	r := apply(func(r *scanner.Result) {
		r.Versions = versions("TLS 1.0", "TLS 1.1", "TLS 1.2", "TLS 1.3")
		r.TLS12Suites = append(r.TLS12Suites, suite("TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA"))
		r.TLS10Suites = []scanner.CipherSuite{suite("TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA"), suite("TLS_RSA_WITH_3DES_EDE_CBC_SHA")}
		r.TLS11Suites = []scanner.CipherSuite{suite("TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA"), suite("TLS_RSA_WITH_3DES_EDE_CBC_SHA")}
	})
	params := map[string]string{}
	for _, f := range Evaluate(r).Findings {
		params[f.Code] = f.Params["suites"]
	}
	if params["SUITE_3DES"] != "TLS_RSA_WITH_3DES_EDE_CBC_SHA (TLS 1.0, TLS 1.1)" {
		t.Errorf("SUITE_3DES suites = %q", params["SUITE_3DES"])
	}
	if params["SUITE_CBC"] != "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA" {
		t.Errorf("SUITE_CBC suites = %q, want the TLS 1.2 suite once", params["SUITE_CBC"])
	}

	r.LegacySuitesComplete = false
	found := false
	for _, f := range Evaluate(r).Findings {
		found = found || f.Code == "SCAN_PARTIAL"
	}
	if !found {
		t.Error("an incomplete TLS 1.0/1.1 enumeration is not reported")
	}
}

// Forward secrecy has one name per language, in the breakdown and in the findings.
func TestForwardSecrecyNaming(t *testing.T) {
	want := map[string]string{"es": "Secreto hacia adelante (forward secrecy)", "en": "Forward secrecy"}
	for lang, name := range want {
		if got := catalog.Label("block.forward_secrecy", lang); got != name {
			t.Errorf("%s block label = %q, want %q", lang, got, name)
		}
		if got := catalog.Describe("FS_ALL", nil, lang).Title; got != name {
			t.Errorf("%s FS_ALL title = %q, want %q", lang, got, name)
		}
	}
	for _, code := range []string{"FS_PARTIAL", "FS_NONE"} {
		if es := catalog.Describe(code, nil, "es").Title; !strings.Contains(es, "secreto hacia adelante (forward secrecy)") {
			t.Errorf("%s es title = %q", code, es)
		}
	}
	for _, lang := range []string{"es", "en"} {
		if r := catalog.Describe("KEX_SINGLE_HYBRID", map[string]string{"group": "X25519MLKEM768"}, lang).Remediation; !strings.Contains(r, "OpenSSL") || !strings.Contains(r, "Caddy") {
			t.Errorf("%s KEX_SINGLE_HYBRID remediation does not mention server software: %q", lang, r)
		}
	}
}

func TestGradeFor(t *testing.T) {
	for score, want := range map[int]string{100: "A", 90: "A", 89: "B", 75: "B", 74: "C", 60: "C", 59: "D", 40: "D", 39: "F", 0: "F"} {
		if got := GradeFor(score); got != want {
			t.Errorf("GradeFor(%d) = %s, want %s", score, got, want)
		}
	}
}

func TestModelAddsUpTo100(t *testing.T) {
	total := 0
	for _, b := range Evaluate(ideal()).Blocks {
		total += b.Max
		sum := 0
		for _, it := range b.Items {
			sum += it.Max
		}
		if len(b.Items) > 0 && sum != b.Max {
			t.Errorf("block %s items add up to %d, want %d", b.ID, sum, b.Max)
		}
	}
	if total != 100 {
		t.Errorf("blocks add up to %d, want 100", total)
	}
	// Quantum readiness: 100 at most, reached only with a post-quantum
	// certificate (85 without one, as the web says).
	ready := 0
	for _, b := range Evaluate(ideal()).Readiness.Blocks {
		ready += b.Max
	}
	if ready != 100 || ReadyPQCert != 15 || Evaluate(ideal()).Readiness.Score != 85 {
		t.Errorf("readiness adds up to %d (post-quantum certificate %d, ideal without one %d), want 100, 15, 85", ready, ReadyPQCert, Evaluate(ideal()).Readiness.Score)
	}
}

// The level thresholds: Ready from 70, On track from 35.
func TestLevelFor(t *testing.T) {
	for score, want := range map[int]string{100: LevelReady, 70: LevelReady, 69: LevelOnTrack, 35: LevelOnTrack, 34: LevelNotReady, 0: LevelNotReady} {
		if got := LevelFor(score); got != want {
			t.Errorf("LevelFor(%d) = %s, want %s", score, got, want)
		}
	}
}

func TestFindingsSortedBySeverity(t *testing.T) {
	fs := Evaluate(apply(noPQC, commercialRSA, func(r *scanner.Result) { r.Certificate.HostnameMatch = false })).Findings
	for i := 1; i < len(fs); i++ {
		if severityRank[fs[i-1].Severity] < severityRank[fs[i].Severity] {
			t.Fatalf("findings not sorted: %s (%s) before %s (%s)", fs[i-1].Code, fs[i-1].Severity, fs[i].Code, fs[i].Severity)
		}
	}
}

// Every emitted finding renders fully in every language, and every catalog
// entry is reachable from some case (no stale texts).
func TestCatalog(t *testing.T) {
	emitted := map[string]bool{}
	cases := goldenCases()
	probe := apply(noPQC, func(r *scanner.Result) {
		for i := range r.KeyExchange.Groups {
			r.KeyExchange.Groups[i].Status = scanner.ProbeFailed
		}
		r.TLS12SuitesComplete = false
	})
	extra := []*scanner.Result{
		probe,
		apply(func(r *scanner.Result) { r.Certificate.NotYetValid = true }),
		apply(func(r *scanner.Result) {
			r.Certificate.Trusted, r.Certificate.TrustError = false, "x509: unknown authority"
		}),
		apply(func(r *scanner.Result) { r.Certificate.SCTsEmbedded = 0 }),
		apply(func(r *scanner.Result) { r.Certificate = nil }),
		pqCertificate(scanner.NotSupported, nil), // CERT_PQ_SIGNATURE, CERT_PQ_NO_CLASSICAL_FALLBACK (pqcert_test.go)
	}
	inputs := extra
	for _, r := range labResults(t) { // model 2026.2: raw probe findings
		inputs = append(inputs, r)
	}
	for _, r := range rawExtras() {
		inputs = append(inputs, r)
	}
	for _, tc := range cases {
		inputs = append(inputs, tc.in)
	}
	for _, in := range inputs {
		for _, f := range Evaluate(in).Findings {
			emitted[f.Code] = true
			for _, lang := range catalog.Languages {
				txt := catalog.Describe(f.Code, f.Params, lang)
				if txt.Title == "" || txt.Detail == "" || txt.Title == f.Code {
					t.Errorf("%s/%s: missing text", f.Code, lang)
				}
				for _, s := range []string{txt.Title, txt.Detail, txt.Remediation} {
					if strings.Contains(s, "{") {
						t.Errorf("%s/%s: unfilled placeholder in %q", f.Code, lang, s)
					}
				}
				if f.Severity != Pass && f.Severity != Info && txt.Remediation == "" && f.Code != "CERT_CLASSICAL_SIGNATURE" {
					t.Errorf("%s/%s: a risk needs a remediation", f.Code, lang)
				}
			}
		}
	}
	for _, code := range catalog.Codes() {
		if !emitted[code] {
			t.Errorf("catalog entry %s is never emitted by the test cases", code)
		}
		for _, lang := range catalog.Languages {
			if !catalog.HasText(code, lang) {
				t.Errorf("%s: no %s text", code, lang)
			}
		}
	}

	var labelKeys []string
	for _, b := range Evaluate(ideal()).Blocks {
		labelKeys = append(labelKeys, "block."+b.ID)
		for _, it := range b.Items {
			labelKeys = append(labelKeys, "item."+it.ID)
		}
	}
	for _, c := range allNotEvaluated {
		labelKeys = append(labelKeys, "noteval."+c)
	}
	for _, s := range []Severity{Pass, Info, Low, Medium, High, Critical} {
		labelKeys = append(labelKeys, "severity."+string(s))
	}
	labelKeys = append(labelKeys, "severity.none", "axis.quantum", "axis.current", "adjust.floor")
	for _, k := range labelKeys {
		for _, lang := range catalog.Languages {
			if l := catalog.Label(k, lang); l == k || l == "" {
				t.Errorf("label %s/%s missing", k, lang)
			}
		}
	}
	if catalog.Describe("KEX_NO_HYBRID", nil, "fr").Title != catalog.Describe("KEX_NO_HYBRID", nil, "es").Title {
		t.Error("unknown language should fall back to Spanish")
	}
}

// Shor breaks key exchange and signatures; Grover only weakens symmetric
// ciphers (informational). Findings off the quantum axis have no threat.
func TestThreatOf(t *testing.T) {
	for _, c := range []struct {
		code string
		axis Axis
		want string
	}{
		{"SYM_AES128", AxisQuantum, ThreatGrover},
		{"KEX_NO_HYBRID", AxisQuantum, ThreatShor},
		{"CERT_CLASSICAL_SIGNATURE", AxisQuantum, ThreatShor},
		{"FS_ALL", AxisCurrent, ""},
	} {
		if got := ThreatOf(c.code, c.axis); got != c.want {
			t.Errorf("ThreatOf(%s, %s) = %q, want %q", c.code, c.axis, got, c.want)
		}
	}
}
