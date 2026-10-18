// Command qrscan runs one scan from the command line and prints its scoring
// as a user would read it (development tool; not shipped in the images).
//
//	go run ./cmd/qrscan quantumready.fizsar.com
//	go run ./cmd/qrscan -lang en -facts example.com
//	go run ./cmd/qrscan -json -port 8443 example.com
//	go run ./cmd/qrscan -intensity basic example.com
//
// -lab is for the TLS test lab only (test/tlslab, make tlslab-scan): it
// replaces the anti-SSRF policy with one that accepts ONLY loopback and RFC
// 1918 addresses. It exists only here, never in the production binaries.
//
// The godebug settings let crypto/tls complete handshakes with very old
// servers (SHA-1 signatures in TLS 1.2, RSA keys under 1024 bits) so they can
// be analyzed. The scanner only observes these connections and never trusts
// them; cmd/worker uses the same settings.
//
//go:debug tlssha1=1
//go:debug rsa1024min=0
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/fizsar/quantumready-saas/internal/scanner"
	"github.com/fizsar/quantumready-saas/internal/scoring"
)

func main() {
	port := flag.Int("port", 443, "TCP port")
	lang := flag.String("lang", "es", "language of the report: es or en")
	facts := flag.Bool("facts", false, "also print the raw scan facts")
	asJSON := flag.Bool("json", false, "print scan and scoring as JSON")
	intensity := flag.String("intensity", "full", "basic (crypto/tls only) or full (also raw ClientHello probes)")
	lab := flag.Bool("lab", false, "TLS test lab only: scan loopback and RFC 1918 addresses, nothing else")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: qrscan [-port N] [-lang es|en] [-facts] [-json] [-intensity basic|full] [-lab] host")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	in, err := scanner.ParseIntensity(*intensity)
	if err != nil {
		fmt.Fprintln(os.Stderr, "qrscan:", err)
		os.Exit(2)
	}
	opts := scanner.Options{Intensity: in}
	if *lab {
		opts.AddrPolicy = labPolicy
	}
	r, err := scanner.New(opts).Scan(context.Background(), flag.Arg(0), *port)
	if err != nil {
		fmt.Fprintln(os.Stderr, "qrscan:", err)
		os.Exit(1)
	}
	s := scoring.Evaluate(r)
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(struct {
			Scan    *scanner.Result `json:"scan"`
			Scoring scoring.Result  `json:"scoring"`
		}{r, s})
		return
	}
	if *facts {
		printFacts(r)
		fmt.Println()
	}
	printReport(r, s, *lang)
}

var ui = map[string]map[string]string{
	"es": {"score": "Nota", "model": "modelo", "yes": "sí", "no": "no", "cap": "Tope por riesgo actual", "breakdown": "Desglose", "findings": "Hallazgos", "fix": "Qué hacer", "noteval": "No evaluado (no suma ni resta)"},
	"en": {"score": "Score", "model": "model", "yes": "yes", "no": "no", "cap": "Cap from current risk", "breakdown": "Breakdown", "findings": "Findings", "fix": "What to do", "noteval": "Not evaluated (never scored)"},
}

func printReport(r *scanner.Result, s scoring.Result, lang string) {
	t, ok := ui[lang]
	if !ok {
		lang, t = "es", ui["es"]
	}
	fmt.Printf("%s — %s %d/100 · %s   (%s %s)\n", r.Host, t["score"], s.Score, s.Grade, t["model"], s.ScoringVersion)
	pqc := t["no"]
	if s.PQC {
		pqc = t["yes"] + " (" + r.KeyExchange.Selected + ")"
	}
	fmt.Printf("PQC: %s · %s: %s · %s: %s\n", pqc,
		scoring.Label("axis.quantum", lang), scoring.Label("severity."+s.QuantumRisk, lang),
		scoring.Label("axis.current", lang), scoring.Label("severity."+s.CurrentRisk, lang))
	if s.Cap != nil {
		fmt.Printf("%s: %s (%s)\n", t["cap"], s.Cap.Grade, strings.Join(s.Cap.Codes, ", "))
	}

	fmt.Printf("\n%s\n", t["breakdown"])
	for _, b := range s.Blocks {
		fmt.Printf("  %-40s %3d/%d\n", scoring.Label("block."+b.ID, lang), b.Points, b.Max)
		for _, it := range b.Items {
			fmt.Printf("    %-38s %3d/%d\n", scoring.Label("item."+it.ID, lang), it.Points, it.Max)
		}
	}
	if s.FloorPoints > 0 {
		fmt.Printf("  %-40s +%d\n", scoring.Label("adjust.floor", lang), s.FloorPoints)
	}

	fmt.Printf("\n%s\n", t["findings"])
	tctx := scoring.Context{ScanTime: r.StartedAt}
	for _, f := range s.Findings {
		tctx.Codes = append(tctx.Codes, f.Code)
	}
	for _, f := range s.Findings {
		txt := scoring.DescribeIn(f.Code, f.Params, lang, tctx)
		fmt.Printf("  [%s · %s] %s\n", scoring.Label("severity."+string(f.Severity), lang), scoring.Label("axis."+string(f.Axis), lang), txt.Title)
		fmt.Printf("      %s\n", txt.Detail)
		if txt.Remediation != "" {
			fmt.Printf("      → %s: %s\n", t["fix"], txt.Remediation)
		}
	}

	var ne []string
	for _, c := range s.NotEvaluated {
		ne = append(ne, scoring.Label("noteval."+c, lang))
	}
	fmt.Printf("\n%s: %s\n", t["noteval"], strings.Join(ne, "; "))
}

func printFacts(r *scanner.Result) {
	fmt.Printf("%s:%d (%s) in %d ms\n", r.Host, r.Port, r.IP, r.DurationMS)
	fmt.Printf("default handshake: %s %s %s group=%s\n", r.Default.Status, r.Default.Version, r.Default.CipherSuite, r.Default.Group)
	fmt.Printf("key exchange: selected=%q hybrid=%v\n", r.KeyExchange.Selected, r.KeyExchange.SelectedHybrid)
	for _, g := range r.KeyExchange.Groups {
		fmt.Printf("  %-20s %s\n", g.Name, g.Status)
	}
	fmt.Println("versions:")
	for _, v := range r.Versions {
		fmt.Printf("  %-8s %s\n", v.Version, v.Status)
	}
	fmt.Printf("TLS 1.3 suite: %s\n", r.TLS13Suite)
	fmt.Printf("TLS 1.2 suites (complete=%v):\n", r.TLS12SuitesComplete)
	for _, s := range r.TLS12Suites {
		fmt.Printf("  %-48s fs=%v aead=%v insecure=%v\n", s.Name, s.ForwardSecrecy, s.AEAD, s.Insecure)
	}
	if raw := r.Raw; raw != nil {
		fmt.Printf("raw probes: %d (budget exhausted %v), connections in total: %d\n", raw.Probes, raw.BudgetExhausted, r.Connections)
		fmt.Printf("  SSLv2 %s %v · SSLv3 %s %v\n", raw.SSLv2.Status, raw.SSLv2.Ciphers, raw.SSLv3.Status, raw.SSLv3.Ciphers)
		fmt.Printf("  suites beyond crypto/tls (complete=%v):\n", raw.SuitesComplete)
		for _, s := range raw.Suites {
			fmt.Printf("    %-8s %-48s fs=%v mode=%s bits=%d export=%v anon=%v\n", s.Version, s.Name, s.ForwardSecrecy, s.Mode, s.Bits, s.Export, s.Anonymous)
		}
		fmt.Printf("  TLS 1.3 suites (complete=%v):", raw.TLS13SuitesComplete)
		for _, s := range raw.TLS13Suites {
			fmt.Printf(" %s", s.Name)
		}
		fmt.Println()
		for _, d := range raw.DH {
			fmt.Printf("  DH %d bits (%s, %s)\n", d.PrimeBits, d.Version, d.Suite)
		}
		fmt.Printf("  TLS 1.3 groups (complete=%v):", raw.TLS13GroupsComplete)
		for _, g := range raw.TLS13Groups {
			fmt.Printf(" %s[%s]", g.Name, g.Kind)
		}
		fmt.Printf("\n  TLS 1.2 curves (complete=%v, explicit=%v):", raw.TLS12CurvesComplete, raw.ExplicitCurve)
		for _, g := range raw.TLS12Curves {
			fmt.Printf(" %s", g.Name)
		}
		fmt.Println()
	}
	if c := r.Certificate; c != nil {
		fmt.Printf("certificate: %s, issuer %q %v, %s %d %s, sig %s\n", c.Subject, c.Issuer, c.IssuerOrg, c.KeyAlgorithm, c.KeySize, c.Curve, c.SignatureAlgorithm)
		fmt.Printf("  trusted=%v hostname=%v expired=%v validity=%dd expires_in=%dd ocsp=%v scts=%d embedded/%d handshake chain=%d\n",
			c.Trusted, c.HostnameMatch, c.Expired, c.ValidityDays, c.DaysToExpiry, c.OCSPStapled, c.SCTsEmbedded, c.SCTsHandshake, len(c.Chain))
		if c.TrustError != "" {
			fmt.Printf("  trust error: %s\n", c.TrustError)
		}
	}
}
