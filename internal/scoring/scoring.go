package scoring

import (
	"slices"
	"strconv"
	"strings"

	"github.com/fizsar/quantumready-saas/internal/scanner"
)

// Version identifies the scoring model. Any change to weights, thresholds,
// caps or the rules that emit findings must bump it (see docs/scoring.md):
// stored results keep the version they were scored with.
const Version = "2026.1"

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
	Score          int    `json:"score"`       // 0–100, post-quantum readiness points
	ScoreGrade     string `json:"score_grade"` // letter from Score alone
	Grade          string `json:"grade"`       // final letter: the worse of ScoreGrade and Cap
	Cap            *Cap   `json:"cap,omitempty"`
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

func (e *evaluator) supports(version string) bool {
	return e.version(version) == scanner.Supported || e.r.Default.Version == version
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
	var noFS []string
	for _, s := range e.r.TLS12Suites {
		if !s.ForwardSecrecy {
			noFS = append(noFS, s.Name)
		}
	}
	switch {
	case len(e.r.TLS12Suites) == 0:
		e.block("forward_secrecy", 0, PointsFS) // nothing observed: coverage() reports it
	case len(noFS) == 0:
		e.block("forward_secrecy", PointsFS, PointsFS)
		e.add("FS_ALL", AxisCurrent, Pass, CatCipherSuites, nil)
	case len(noFS) == len(e.r.TLS12Suites) && !e.supports("TLS 1.3"):
		e.block("forward_secrecy", 0, PointsFS)
		e.add("FS_NONE", AxisCurrent, High, CatCipherSuites, map[string]string{"suites": strings.Join(noFS, ", ")})
		e.capAt("C", "FS_NONE")
	default:
		e.block("forward_secrecy", 0, PointsFS)
		e.add("FS_PARTIAL", AxisCurrent, Medium, CatCipherSuites, map[string]string{"suites": strings.Join(noFS, ", ")})
	}
}

// variety: more than one hybrid group (e.g. SecP256r1MLKEM768 for FIPS clients).
func (e *evaluator) variety() {
	hybrids := e.hybridSupported()
	switch {
	case len(hybrids) > 1:
		e.block("hybrid_variety", PointsVariety, PointsVariety)
		e.add("KEX_MULTIPLE_HYBRIDS", AxisQuantum, Pass, CatKeyExchange, map[string]string{"groups": strings.Join(hybrids, ", ")})
	case len(hybrids) == 1:
		e.block("hybrid_variety", 0, PointsVariety)
		e.add("KEX_SINGLE_HYBRID", AxisQuantum, Low, CatKeyExchange, map[string]string{"group": hybrids[0]})
	default:
		e.block("hybrid_variety", 0, PointsVariety)
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
		if e.version(v) == scanner.Supported {
			legacy = append(legacy, v)
		}
	}
	if len(legacy) > 0 {
		e.add("PROTO_LEGACY_TLS", AxisCurrent, Medium, CatProtocol, map[string]string{"versions": strings.Join(legacy, ", ")})
		e.capAt("B", "PROTO_LEGACY_TLS")
	}
}

func (e *evaluator) suites() {
	var rc4, des, cbc []string
	for _, s := range e.r.TLS12Suites {
		switch {
		case strings.Contains(s.Name, "_RC4_"):
			rc4 = append(rc4, s.Name)
		case strings.Contains(s.Name, "_3DES_"):
			des = append(des, s.Name)
		case !s.AEAD:
			cbc = append(cbc, s.Name)
		}
	}
	if len(rc4) > 0 {
		e.add("SUITE_RC4", AxisCurrent, High, CatCipherSuites, map[string]string{"suites": strings.Join(rc4, ", ")})
		e.capAt("C", "SUITE_RC4")
	}
	if len(des) > 0 {
		e.add("SUITE_3DES", AxisCurrent, High, CatCipherSuites, map[string]string{"suites": strings.Join(des, ", ")})
		e.capAt("C", "SUITE_3DES")
	}
	if len(cbc) > 0 {
		e.add("SUITE_CBC", AxisCurrent, Low, CatCipherSuites, map[string]string{"suites": strings.Join(cbc, ", ")})
		e.capAt("B", "SUITE_CBC")
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
