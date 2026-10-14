package scoring

import (
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/fizsar/quantumready-saas/internal/scanner"
	"github.com/fizsar/quantumready-saas/internal/scanner/rawtls"
)

// Version identifies the scoring model. Any change to weights, thresholds,
// caps or the rules that emit findings must bump it (see docs/scoring.md):
// stored results keep the version they were scored with.
const Version = "2026.2"

// Severity of a finding. Pass marks something done right.
type Severity string

// Severities, from best to worst.
const (
	Pass     Severity = "pass"
	Info     Severity = "info"
	Low      Severity = "low"
	Medium   Severity = "medium"
	High     Severity = "high"
	Critical Severity = "critical"
)

var severityRank = map[Severity]int{Pass: 0, Info: 1, Low: 2, Medium: 3, High: 4, Critical: 5}

// Axis separates today's risk from the quantum one.
type Axis string

const (
	// AxisQuantum covers harvest now, decrypt later (key exchange) and the
	// future signature risk (certificates).
	AxisQuantum Axis = "quantum"
	// AxisCurrent covers weaknesses exploitable today, regardless of quantum computers.
	AxisCurrent Axis = "current"
)

// Category groups findings by the part of TLS they concern.
type Category string

// Categories.
const (
	CatKeyExchange  Category = "key_exchange"
	CatProtocol     Category = "protocol"
	CatCipherSuites Category = "cipher_suites"
	CatCertificate  Category = "certificate"
	CatScan         Category = "scan"
)

// Finding is one judgement about the scan. Its texts (ES/EN) come from the
// catalog by Code; Params fill their placeholders. Only code and params are
// stored, so texts can be fixed without migrating data.
type Finding struct {
	Code     string            `json:"code"`
	Axis     Axis              `json:"axis"`
	Severity Severity          `json:"severity"`
	Category Category          `json:"category"`
	Params   map[string]string `json:"params,omitempty"`
}

// Item is one scored criterion inside a block.
type Item struct {
	ID     string `json:"id"`
	Points int    `json:"points"`
	Max    int    `json:"max"`
}

// Block is one part of the score breakdown.
type Block struct {
	ID     string `json:"id"`
	Points int    `json:"points"`
	Max    int    `json:"max"`
	Items  []Item `json:"items,omitempty"`
}

// Cap is the worst grade allowed by today's risks (worst-case rule).
type Cap struct {
	Grade string   `json:"grade"`
	Codes []string `json:"codes"` // findings that impose it
}

// Result is the judgement of one scan.
type Result struct {
	ScoringVersion string `json:"scoring_version"`
	Score          int    `json:"score"`       // 0–100: sum of the blocks plus FloorPoints
	ScoreGrade     string `json:"score_grade"` // letter from Score alone
	// FloorPoints lifts the score to FloorScore when nothing is broken or weak
	// (no F or C cap): classical TLS done right is never an F.
	FloorPoints int    `json:"floor_points,omitempty"`
	Grade       string `json:"grade"` // final letter: the worse of ScoreGrade and Cap
	Cap         *Cap   `json:"cap,omitempty"`
	// PQC: the server negotiates a hybrid ML-KEM group with a browser-like offer.
	PQC bool `json:"pqc"`
	// Worst non-pass severity per axis ("none" if nothing to report).
	QuantumRisk string    `json:"quantum_risk"`
	CurrentRisk string    `json:"current_risk"`
	Blocks      []Block   `json:"blocks"`
	Findings    []Finding `json:"findings"`
	// NotEvaluated is copied from the scan for display; it never affects points.
	NotEvaluated []string `json:"not_evaluated"`
}

// Points per block and criterion. See docs/scoring.md before changing them.
const (
	PointsKexPreferred = 50 // hybrid ML-KEM picked with a browser-like offer
	PointsKexSupported = 30 // hybrid accepted only when offered alone
	PointsTLS13        = 15
	PointsFS           = 10
	PointsVariety      = 5 // more than one hybrid group
	PointsCertValid    = 5
	PointsCertKey      = 5
	PointsCertSig      = 5
	PointsCertAuto     = 5
)

// FloorScore is the minimum score of a server with no F or C cap: the bottom
// of D. F is reserved for broken or weak setups.
const FloorScore = 40

// Grade thresholds: minimum score per letter.
var gradeThresholds = []struct {
	min   int
	grade string
}{{90, "A"}, {75, "B"}, {60, "C"}, {40, "D"}, {0, "F"}}

// GradeFor returns the letter for a score.
func GradeFor(score int) string {
	for _, t := range gradeThresholds {
		if score >= t.min {
			return t.grade
		}
	}
	return "F"
}

// AutomationMaxValidityDays is the validity at or under which a certificate
// counts as renewed by automation. 47 days is the CA/Browser Forum maximum
// from March 2029.
const AutomationMaxValidityDays = 47

// acmeFirstCAs issue certificates mainly through ACME (automated renewal).
// Matched against the issuer organization, case-insensitively.
var acmeFirstCAs = []string{"Let's Encrypt", "Google Trust Services", "ZeroSSL"}

// Evaluate scores a scan result. It is deterministic and uses only r.
func Evaluate(r *scanner.Result) Result {
	e := &evaluator{r: r}
	e.keyExchange()
	e.tls13()
	e.forwardSecrecy()
	e.variety()
	e.certificate()
	e.protocols()
	e.suites()
	e.dh()
	e.groups()
	e.experimentalPQC()
	e.symmetric()
	e.coverage()
	return e.result()
}

type evaluator struct {
	r        *scanner.Result
	blocks   []Block
	findings []Finding
	caps     map[string][]string // grade -> codes
}

func (e *evaluator) add(code string, axis Axis, sev Severity, cat Category, params map[string]string) {
	e.findings = append(e.findings, Finding{Code: code, Axis: axis, Severity: sev, Category: cat, Params: params})
}

func (e *evaluator) capAt(grade, code string) {
	if e.caps == nil {
		e.caps = map[string][]string{}
	}
	e.caps[grade] = append(e.caps[grade], code)
}

func (e *evaluator) block(id string, points, maxPoints int, items ...Item) {
	e.blocks = append(e.blocks, Block{ID: id, Points: points, Max: maxPoints, Items: items})
}

func (e *evaluator) hybridSupported() []string {
	var out []string
	for _, g := range e.r.KeyExchange.Groups {
		if g.Hybrid && g.Status == scanner.Supported {
			out = append(out, g.Name)
		}
	}
	return out
}

func (e *evaluator) hybridProbeFailed() bool {
	for _, g := range e.r.KeyExchange.Groups {
		if g.Hybrid && g.Status == scanner.ProbeFailed {
			return true
		}
	}
	return false
}

func (e *evaluator) version(name string) scanner.Status {
	for _, v := range e.r.Versions {
		if v.Version == name {
			return v.Status
		}
	}
	return ""
}

// supports reports a version seen by crypto/tls or, with intensity full, by
// the raw probes (which also see versions whose suites Go cannot offer).
func (e *evaluator) supports(version string) bool {
	if e.version(version) == scanner.Supported || e.r.Default.Version == version {
		return true
	}
	if raw := e.r.Raw; raw != nil {
		for _, v := range raw.Versions {
			if v.Version == version && v.Status == scanner.Supported {
				return true
			}
		}
		return version == "TLS 1.3" && len(raw.TLS13Suites) > 0
	}
	return false
}

// keyExchange: harvest now, decrypt later. What matters is what a real client
// gets (the browser-like offer), not only what the server can do.
func (e *evaluator) keyExchange() {
	hybrids := e.hybridSupported()
	switch {
	case e.r.KeyExchange.SelectedHybrid:
		e.block("key_exchange", PointsKexPreferred, PointsKexPreferred)
		e.add("KEX_HYBRID_PREFERRED", AxisQuantum, Pass, CatKeyExchange, map[string]string{"group": e.r.KeyExchange.Selected})
	case len(hybrids) > 0:
		e.block("key_exchange", PointsKexSupported, PointsKexPreferred)
		e.add("KEX_HYBRID_NOT_PREFERRED", AxisQuantum, High, CatKeyExchange, map[string]string{"groups": strings.Join(hybrids, ", ")})
	case e.hybridProbeFailed():
		// Not preferred, and support could not be checked: no points without
		// evidence, but no claim that it is unsupported either.
		e.block("key_exchange", 0, PointsKexPreferred)
		e.add("KEX_HYBRID_UNKNOWN", AxisQuantum, High, CatKeyExchange, nil)
	default:
		e.block("key_exchange", 0, PointsKexPreferred)
		e.add("KEX_NO_HYBRID", AxisQuantum, Critical, CatKeyExchange, nil)
	}
}

// tls13: hybrid ML-KEM groups only exist in TLS 1.3.
func (e *evaluator) tls13() {
	if e.supports("TLS 1.3") {
		e.block("tls13", PointsTLS13, PointsTLS13)
		e.add("TLS13_SUPPORTED", AxisQuantum, Pass, CatProtocol, nil)
		return
	}
	e.block("tls13", 0, PointsTLS13)
	if e.version("TLS 1.3") == scanner.NotSupported {
		e.add("TLS13_MISSING", AxisQuantum, High, CatProtocol, nil)
	} // a probe error is reported by coverage(), not judged
}

// forwardSecrecy: every TLS 1.2 suite must use an ephemeral key exchange.
// A TLS 1.3-only server has it by design.
func (e *evaluator) forwardSecrecy() {
	if !e.supports("TLS 1.2") {
		if e.supports("TLS 1.3") {
			e.block("forward_secrecy", PointsFS, PointsFS)
			e.add("FS_ALL", AxisCurrent, Pass, CatCipherSuites, nil)
		} else {
			e.block("forward_secrecy", 0, PointsFS)
		}
		return
	}
	tls12 := e.tls12Suites()
	var noFS []string
	for _, s := range tls12 {
		if !s.ForwardSecrecy {
			noFS = append(noFS, s.Name)
		}
	}
	switch {
	case len(tls12) == 0:
		e.block("forward_secrecy", 0, PointsFS) // nothing observed: coverage() reports it
	case len(noFS) == 0:
		e.block("forward_secrecy", PointsFS, PointsFS)
		e.add("FS_ALL", AxisCurrent, Pass, CatCipherSuites, nil)
	case len(noFS) == len(tls12) && !e.supports("TLS 1.3"):
		e.block("forward_secrecy", 0, PointsFS)
		e.add("FS_NONE", AxisCurrent, High, CatCipherSuites, map[string]string{"suites": strings.Join(noFS, ", ")})
		e.capAt("C", "FS_NONE")
	default:
		e.block("forward_secrecy", 0, PointsFS)
		e.add("FS_PARTIAL", AxisCurrent, Medium, CatCipherSuites, map[string]string{"suites": strings.Join(noFS, ", ")})
	}
}

// variety: more than one standard PQC group (e.g. SecP256r1MLKEM768 for FIPS
// clients). Since 2026.2, pure ML-KEM (FIPS 203, seen by the raw probes)
// counts here, but never in the key exchange block: browsers do not offer it.
func (e *evaluator) variety() {
	hybrids := e.hybridSupported()
	pure := e.rawGroups(rawtls.GroupPureMLKEM)
	standard := append(append([]string{}, hybrids...), pure...)
	switch {
	case len(standard) > 1 && len(pure) > 0:
		e.block("hybrid_variety", PointsVariety, PointsVariety)
		e.add("KEX_MULTIPLE_PQC", AxisQuantum, Pass, CatKeyExchange, map[string]string{"groups": strings.Join(standard, ", ")})
	case len(standard) > 1:
		e.block("hybrid_variety", PointsVariety, PointsVariety)
		e.add("KEX_MULTIPLE_HYBRIDS", AxisQuantum, Pass, CatKeyExchange, map[string]string{"groups": strings.Join(hybrids, ", ")})
	case len(hybrids) == 1:
		e.block("hybrid_variety", 0, PointsVariety)
		e.add("KEX_SINGLE_HYBRID", AxisQuantum, Low, CatKeyExchange, map[string]string{"group": hybrids[0]})
	default:
		e.block("hybrid_variety", 0, PointsVariety)
	}
	if len(pure) > 0 {
		e.add("KEX_PURE_MLKEM", AxisQuantum, Info, CatKeyExchange, map[string]string{"groups": strings.Join(pure, ", ")})
	}
}

// rawGroups are the TLS 1.3 groups of the given kinds found by the raw probes.
func (e *evaluator) rawGroups(kinds ...rawtls.GroupKind) []string {
	if e.r.Raw == nil {
		return nil
	}
	var out []string
	for _, g := range e.r.Raw.TLS13Groups {
		if slices.Contains(kinds, rawtls.GroupKind(g.Kind)) {
			out = append(out, g.Name)
		}
	}
	return out
}

// experimentalPQC: pre-standard Kyber or draft hybrids. Browsers no longer
// offer them (Chrome dropped Kyber in 2024), so they protect nobody and earn
// no points; alone they mean a PQC migration started and got stuck.
func (e *evaluator) experimentalPQC() {
	exp := e.rawGroups(rawtls.GroupKyberDraft, rawtls.GroupHybridDraft)
	if len(exp) == 0 {
		return
	}
	params := map[string]string{"groups": strings.Join(exp, ", ")}
	if len(e.hybridSupported()) > 0 {
		e.add("KEX_EXPERIMENTAL_PQC_EXTRA", AxisQuantum, Info, CatKeyExchange, params)
		return
	}
	e.add("KEX_EXPERIMENTAL_PQC_ONLY", AxisQuantum, Medium, CatKeyExchange, params)
}

// dh: finite-field DH group size from ServerKeyExchange (Logjam).
func (e *evaluator) dh() {
	raw := e.r.Raw
	if raw == nil || len(raw.DH) == 0 {
		return
	}
	var bits []string
	for _, d := range raw.DH {
		if b := strconv.Itoa(d.PrimeBits); !slices.Contains(bits, b) {
			bits = append(bits, b)
		}
	}
	params := map[string]string{"bits": strings.Join(bits, ", ")}
	switch smallest := raw.DH[0].PrimeBits; { // sorted, smallest first
	case smallest < 1024:
		e.add("DH_WEAK", AxisCurrent, Critical, CatKeyExchange, params)
		e.capAt("F", "DH_WEAK")
	case smallest < 2048:
		e.add("DH_SMALL", AxisCurrent, High, CatKeyExchange, params)
		e.capAt("C", "DH_SMALL")
	default:
		e.add("DH_OK", AxisCurrent, Pass, CatKeyExchange, params)
	}
}

// groups: classical groups beyond the usual ones. Weak curves are reported
// without a cap: browsers never offer them.
func (e *evaluator) groups() {
	raw := e.r.Raw
	if raw == nil {
		return
	}
	var weak, extra []string
	for _, g := range raw.TLS12Curves {
		switch rawtls.GroupKind(g.Kind) {
		case rawtls.GroupECDHEWeak, rawtls.GroupBinary:
			weak = append(weak, g.Name)
		default:
			extra = append(extra, g.Name+" (TLS 1.2)")
		}
	}
	if raw.ExplicitCurve {
		weak = append(weak, "explicit")
	}
	for _, g := range raw.TLS13Groups {
		switch rawtls.GroupKind(g.Kind) {
		case rawtls.GroupFFDHE, rawtls.GroupBrainpool, rawtls.GroupECDHE, rawtls.GroupOther:
			extra = append(extra, g.Name)
		}
	}
	if len(weak) > 0 {
		e.add("GROUP_WEAK_CURVES", AxisCurrent, Low, CatKeyExchange, map[string]string{"groups": strings.Join(weak, ", ")})
	}
	if len(extra) > 0 {
		e.add("GROUPS_EXTRA", AxisCurrent, Info, CatKeyExchange, map[string]string{"groups": strings.Join(extra, ", ")})
	}
}

func (e *evaluator) certificate() {
	c := e.r.Certificate
	maxPoints := PointsCertValid + PointsCertKey + PointsCertSig + PointsCertAuto
	if c == nil {
		e.block("certificate", 0, maxPoints)
		e.add("CERT_MISSING", AxisCurrent, Info, CatScan, nil)
		return
	}

	// Validity: chain, name and dates. Each failure caps the grade at F.
	valid := c.Trusted && c.HostnameMatch && !c.Expired && !c.NotYetValid
	switch {
	case c.Expired:
		e.add("CERT_EXPIRED", AxisCurrent, Critical, CatCertificate, map[string]string{"date": c.NotAfter.Format("2006-01-02")})
		e.capAt("F", "CERT_EXPIRED")
	case c.NotYetValid:
		e.add("CERT_NOT_YET_VALID", AxisCurrent, Critical, CatCertificate, map[string]string{"date": c.NotBefore.Format("2006-01-02")})
		e.capAt("F", "CERT_NOT_YET_VALID")
	case !c.Trusted:
		code := "CERT_UNTRUSTED"
		if c.SelfSigned {
			code = "CERT_SELF_SIGNED"
		}
		e.add(code, AxisCurrent, Critical, CatCertificate, map[string]string{"error": c.TrustError})
		e.capAt("F", code)
	}
	if !c.HostnameMatch {
		e.add("CERT_HOSTNAME_MISMATCH", AxisCurrent, Critical, CatCertificate,
			map[string]string{"host": e.r.Host, "names": strings.Join(c.SANs, ", ")})
		e.capAt("F", "CERT_HOSTNAME_MISMATCH")
	}
	if valid {
		e.add("CERT_VALID", AxisCurrent, Pass, CatCertificate, map[string]string{"issuer": issuerName(c), "days": strconv.Itoa(c.DaysToExpiry)})
		if c.DaysToExpiry <= 14 {
			e.add("CERT_EXPIRES_SOON", AxisCurrent, Medium, CatCertificate, map[string]string{"days": strconv.Itoa(c.DaysToExpiry)})
		}
	}

	keyOK := (c.KeyAlgorithm == "RSA" && c.KeySize >= 2048) ||
		(c.KeyAlgorithm == "ECDSA" && c.KeySize >= 256) || c.KeyAlgorithm == "Ed25519"
	if !keyOK && (c.KeyAlgorithm == "RSA" || c.KeyAlgorithm == "ECDSA") {
		e.add("CERT_WEAK_KEY", AxisCurrent, High, CatCertificate, map[string]string{"key": keyLabel(c)})
		e.capAt("C", "CERT_WEAK_KEY")
	}

	sigOK := !strings.Contains(c.SignatureAlgorithm, "SHA1") && !strings.Contains(c.SignatureAlgorithm, "MD5")
	if !sigOK {
		e.add("CERT_WEAK_SIGNATURE", AxisCurrent, High, CatCertificate, map[string]string{"algorithm": c.SignatureAlgorithm})
		e.capAt("C", "CERT_WEAK_SIGNATURE")
	}

	// Future quantum risk: every WebPKI signature today is classical. Not
	// harvest-now-decrypt-later (a signature only has to hold during the
	// handshake), so it never costs points.
	if c.KeyAlgorithm == "RSA" || c.KeyAlgorithm == "ECDSA" || c.KeyAlgorithm == "Ed25519" {
		e.add("CERT_CLASSICAL_SIGNATURE", AxisQuantum, Low, CatCertificate, map[string]string{"key": keyLabel(c)})
	}

	automated, reason := automation(c)
	switch {
	case automated && reason == "validity":
		e.add("CERT_AUTOMATED_SHORT", AxisQuantum, Pass, CatCertificate, map[string]string{"days": strconv.Itoa(c.ValidityDays)})
	case automated:
		e.add("CERT_AUTOMATED_ACME", AxisQuantum, Pass, CatCertificate, map[string]string{"issuer": issuerName(c), "days": strconv.Itoa(c.ValidityDays)})
	default:
		e.add("CERT_NOT_AUTOMATED", AxisQuantum, Low, CatCertificate, map[string]string{"days": strconv.Itoa(c.ValidityDays), "issuer": issuerName(c)})
	}

	// Certificate Transparency: informational only.
	if c.SCTsEmbedded+c.SCTsHandshake > 0 {
		e.add("CERT_CT", AxisCurrent, Info, CatCertificate,
			map[string]string{"embedded": strconv.Itoa(c.SCTsEmbedded), "handshake": strconv.Itoa(c.SCTsHandshake)})
	} else {
		e.add("CERT_CT_MISSING", AxisCurrent, Info, CatCertificate, nil)
	}

	items := []Item{
		{ID: "valid", Points: pts(valid, PointsCertValid), Max: PointsCertValid},
		{ID: "key", Points: pts(keyOK, PointsCertKey), Max: PointsCertKey},
		{ID: "signature", Points: pts(sigOK, PointsCertSig), Max: PointsCertSig},
		{ID: "automation", Points: pts(automated, PointsCertAuto), Max: PointsCertAuto},
	}
	total := 0
	for _, it := range items {
		total += it.Points
	}
	e.block("certificate", total, maxPoints, items...)
}

// automation reports whether the certificate shows signs of automated
// renewal, which is what will make the switch to post-quantum certificates
// painless: a very short validity, or an ACME-first CA.
func automation(c *scanner.Certificate) (bool, string) {
	if c.ValidityDays <= AutomationMaxValidityDays {
		return true, "validity"
	}
	for _, org := range c.IssuerOrg {
		for _, ca := range acmeFirstCAs {
			if strings.EqualFold(org, ca) || strings.HasPrefix(strings.ToLower(org), strings.ToLower(ca)) {
				return true, "acme"
			}
		}
	}
	return false, ""
}

func (e *evaluator) protocols() {
	var legacy []string
	for _, v := range []string{"TLS 1.0", "TLS 1.1"} {
		if e.supports(v) {
			legacy = append(legacy, v)
		}
	}
	if len(legacy) > 0 {
		e.add("PROTO_LEGACY_TLS", AxisCurrent, Medium, CatProtocol, map[string]string{"versions": strings.Join(legacy, ", ")})
		e.capAt("B", "PROTO_LEGACY_TLS")
	}
	raw := e.r.Raw
	if raw == nil {
		return
	}
	if raw.SSLv2.Status == scanner.Supported { // DROWN; prohibited by RFC 6176
		e.add("PROTO_SSL2", AxisCurrent, Critical, CatProtocol, map[string]string{"ciphers": strings.Join(raw.SSLv2.Ciphers, ", ")})
		e.capAt("F", "PROTO_SSL2")
	}
	if raw.SSLv3.Status == scanner.Supported { // POODLE; prohibited by RFC 7568
		e.add("PROTO_SSL3", AxisCurrent, High, CatProtocol, nil)
		e.capAt("C", "PROTO_SSL3")
	}
}

// rawSuites are the suites the raw probes found in version (intensity full).
func (e *evaluator) rawSuites(version string) []scanner.CipherSuite {
	if e.r.Raw == nil {
		return nil
	}
	var out []scanner.CipherSuite
	for _, s := range e.r.Raw.Suites {
		if s.Version == version {
			out = append(out, scanner.CipherSuite{Name: s.Name, ID: s.ID, ForwardSecrecy: s.ForwardSecrecy, AEAD: s.AEAD})
		}
	}
	return out
}

// tls12Suites are the TLS 1.2 suites found by crypto/tls and the raw probes.
func (e *evaluator) tls12Suites() []scanner.CipherSuite {
	return append(append([]scanner.CipherSuite{}, e.r.TLS12Suites...), e.rawSuites("TLS 1.2")...)
}

// allSuites returns the suites accepted in any version up to TLS 1.2 (and
// SSLv3), once each. Suites missing from TLS 1.2 are labeled with the
// versions that accept them, e.g. "TLS_RSA_WITH_3DES_EDE_CBC_SHA (TLS 1.0)".
func (e *evaluator) allSuites() []scanner.CipherSuite {
	tls12 := e.tls12Suites()
	in12 := map[string]bool{}
	for _, s := range tls12 {
		in12[s.Name] = true
	}
	out := append([]scanner.CipherSuite{}, tls12...)
	var order []string
	legacy := map[string][]string{}
	byName := map[string]scanner.CipherSuite{}
	for _, v := range []struct {
		name   string
		suites []scanner.CipherSuite
	}{
		{"TLS 1.0", append(append([]scanner.CipherSuite{}, e.r.TLS10Suites...), e.rawSuites("TLS 1.0")...)},
		{"TLS 1.1", append(append([]scanner.CipherSuite{}, e.r.TLS11Suites...), e.rawSuites("TLS 1.1")...)},
		{"SSLv3", e.rawSuites("SSLv3")},
	} {
		for _, s := range v.suites {
			if in12[s.Name] {
				continue
			}
			if _, seen := legacy[s.Name]; !seen {
				order = append(order, s.Name)
				byName[s.Name] = s
			}
			legacy[s.Name] = append(legacy[s.Name], v.name)
		}
	}
	for _, n := range order {
		s := byName[n]
		s.Name = n + " (" + strings.Join(legacy[n], ", ") + ")"
		out = append(out, s)
	}
	return out
}

// suitesByName indexes the IANA registry by name.
var suitesByName = sync.OnceValue(func() map[string]rawtls.Suite {
	m := map[string]rawtls.Suite{}
	for _, s := range rawtls.Suites() {
		m[s.Name] = s
	}
	return m
})

// suiteInfo returns the registry attributes of an accepted suite. Scans carry
// IDs; the name (without the "(TLS 1.0)" label) is the fallback.
func suiteInfo(s scanner.CipherSuite) rawtls.Suite {
	name, _, _ := strings.Cut(s.Name, " (")
	if info, ok := rawtls.SuiteByID(s.ID); ok && info.Name == name {
		return info
	}
	return suitesByName()[name]
}

// suites judges every accepted suite once, by its worst property (attributes
// from the IANA registry in rawtls): no encryption, EXPORT, anonymous, single
// DES, RC4, 3DES, CBC, then CCM_8. TLS 1.3 suites come from the raw probes.
func (e *evaluator) suites() {
	var null, export, anon, des, rc4, des3, cbc, ccm8, integrity, modern, tls13 []string
	for _, s := range e.allSuites() {
		info := suiteInfo(s)
		switch {
		case info.Mode == rawtls.ModeNone:
			null = append(null, s.Name)
		case info.Export:
			export = append(export, s.Name)
		case info.Anonymous():
			anon = append(anon, s.Name)
		case info.Mode == rawtls.ModeStream:
			rc4 = append(rc4, s.Name)
		case info.Cipher == "3DES_EDE_CBC":
			des3 = append(des3, s.Name)
		case info.Bits > 0 && info.Bits < 112:
			des = append(des, s.Name)
		case info.Mode == rawtls.ModeCBC:
			cbc = append(cbc, s.Name)
		case info.Mode == rawtls.ModeCCM8:
			ccm8 = append(ccm8, s.Name)
		}
	}
	if raw := e.r.Raw; raw != nil {
		for _, s := range raw.TLS13Suites {
			info := suiteInfo(scanner.CipherSuite{Name: s.Name, ID: s.ID})
			tls13 = append(tls13, s.Name)
			switch {
			case info.Mode == rawtls.ModeNone:
				integrity = append(integrity, s.Name)
			case info.Mode == rawtls.ModeCCM8:
				ccm8 = append(ccm8, s.Name+" (TLS 1.3)")
			case strings.HasPrefix(info.Cipher, "AEGIS") || strings.HasPrefix(info.Cipher, "ASCON"):
				modern = append(modern, s.Name)
			}
		}
	}
	type rule struct {
		code   string
		sev    Severity
		cap    string
		suites []string
	}
	for _, r := range []rule{
		{"SUITE_NULL", Critical, "F", null},
		{"SUITE_INTEGRITY_ONLY", Critical, "F", integrity},
		{"SUITE_EXPORT", Critical, "F", export},
		{"SUITE_ANON", Critical, "F", anon},
		{"SUITE_DES", Critical, "F", des},
		{"SUITE_RC4", High, "C", rc4},
		{"SUITE_3DES", High, "C", des3},
		{"SUITE_CBC", Low, "B", cbc},
		{"SUITE_CCM8", Low, "", ccm8},
		{"SUITE_MODERN_AEAD", Info, "", modern},
	} {
		if len(r.suites) == 0 {
			continue
		}
		e.add(r.code, AxisCurrent, r.sev, CatCipherSuites, map[string]string{"suites": strings.Join(r.suites, ", ")})
		if r.cap != "" {
			e.capAt(r.cap, r.code)
		}
	}
	if len(tls13) > 0 && len(integrity) == 0 && len(ccm8) == 0 {
		e.add("TLS13_SUITES_AEAD", AxisCurrent, Pass, CatCipherSuites, map[string]string{"suites": strings.Join(tls13, ", ")})
	}
}

// symmetric: AES-128 and Grover. Informational: NIST still deems AES-128
// adequate, so it never costs points.
func (e *evaluator) symmetric() {
	suite := e.r.TLS13Suite
	if suite == "" {
		suite = e.r.Default.CipherSuite
	}
	if strings.Contains(suite, "AES_128") {
		e.add("SYM_AES128", AxisQuantum, Info, CatCipherSuites, map[string]string{"suite": suite})
	}
}

// coverage reports probes that could not decide. They are never judged as
// "not supported" and never cost or earn points by themselves.
func (e *evaluator) coverage() {
	failed := 0
	for _, g := range e.r.KeyExchange.Groups {
		if g.Status == scanner.ProbeFailed {
			failed++
		}
	}
	for _, v := range e.r.Versions {
		if v.Status == scanner.ProbeFailed {
			failed++
		}
	}
	if !e.r.TLS12SuitesComplete {
		failed++
	}
	if !e.r.LegacySuitesComplete {
		failed++
	}
	if raw := e.r.Raw; raw != nil {
		for _, incomplete := range []bool{
			raw.SSLv2.Status == scanner.ProbeFailed, raw.SSLv3.Status == scanner.ProbeFailed,
			!raw.SuitesComplete, !raw.TLS13SuitesComplete, !raw.TLS13GroupsComplete, !raw.TLS12CurvesComplete,
		} {
			if incomplete {
				failed++
			}
		}
	}
	if failed > 0 {
		e.add("SCAN_PARTIAL", AxisCurrent, Info, CatScan, map[string]string{"count": strconv.Itoa(failed)})
	}
}

func (e *evaluator) result() Result {
	res := Result{
		ScoringVersion: Version,
		PQC:            e.r.KeyExchange.SelectedHybrid,
		Blocks:         e.blocks,
		NotEvaluated:   append([]string{}, e.r.NotEvaluated...),
	}
	for _, b := range e.blocks {
		res.Score += b.Points
	}
	_, broken := e.caps["F"]
	_, weak := e.caps["C"]
	if !broken && !weak && res.Score < FloorScore {
		res.FloorPoints = FloorScore - res.Score
		res.Score = FloorScore
	}
	res.ScoreGrade = GradeFor(res.Score)
	res.Grade = res.ScoreGrade
	for _, g := range []string{"F", "D", "C", "B"} { // worst cap wins
		if codes, ok := e.caps[g]; ok {
			res.Cap = &Cap{Grade: g, Codes: codes}
			if gradeRank(g) > gradeRank(res.Grade) {
				res.Grade = g
			}
			break
		}
	}

	slices.SortStableFunc(e.findings, func(a, b Finding) int {
		if d := severityRank[b.Severity] - severityRank[a.Severity]; d != 0 {
			return d
		}
		if a.Axis != b.Axis {
			if a.Axis == AxisQuantum {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Code, b.Code)
	})
	res.Findings = e.findings
	res.QuantumRisk = worst(e.findings, AxisQuantum)
	res.CurrentRisk = worst(e.findings, AxisCurrent)
	return res
}

func gradeRank(g string) int { return strings.Index("ABCDF", g) }

func worst(fs []Finding, axis Axis) string {
	best := 1 // info and pass do not count as risk
	out := "none"
	for _, f := range fs {
		if f.Axis == axis && severityRank[f.Severity] > best {
			best = severityRank[f.Severity]
			out = string(f.Severity)
		}
	}
	return out
}

func pts(ok bool, p int) int {
	if ok {
		return p
	}
	return 0
}

func issuerName(c *scanner.Certificate) string {
	if len(c.IssuerOrg) > 0 {
		return c.IssuerOrg[0]
	}
	return c.Issuer
}

func keyLabel(c *scanner.Certificate) string {
	if c.Curve != "" {
		return c.KeyAlgorithm + " " + c.Curve
	}
	return c.KeyAlgorithm + "-" + strconv.Itoa(c.KeySize)
}
