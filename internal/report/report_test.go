package report

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/fizsar/quantumready-saas/internal/scoring"
)

func build(t *testing.T, in Input) *Document {
	t.Helper()
	if in.Result == nil {
		in.Result = json.RawMessage(`{}`)
	}
	doc, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// Lists the web iterates are [] rather than null, even when empty; the API
// has always sent them so (testdata/apijson in internal/httpapi).
func TestEmptyListsAreArrays(t *testing.T) {
	b, err := json.Marshal(build(t, Input{Lang: "es"}))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"findings":[]`, `"not_evaluated":[]`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("%s missing from %s", want, b)
		}
	}
}

// Texts that depend on when the scan ran use the scan's date, not today's:
// the CA/Browser Forum validity schedule reads differently before and after
// its steps.
func TestTextsUseTheScanDate(t *testing.T) {
	f := []Finding{{Code: "CERT_NOT_AUTOMATED", Axis: "quantum", Severity: "medium", Category: "certificate",
		Params: map[string]string{"days": "398", "issuer": "X"}}}
	at := func(y int, m time.Month) string {
		return build(t, Input{Lang: "en", Findings: f, ScanTime: time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)}).Findings[0].Detail
	}
	before, after := at(2026, 1), at(2027, 6)
	if before == after || strings.Contains(before, "{") || strings.Contains(after, "{") {
		t.Errorf("the scan date does not reach the text:\n%s\n%s", before, after)
	}
}

// Every text and label is resolved in the requested language.
func TestBuildResolvesTexts(t *testing.T) {
	in := Input{
		Scoring: scoring.Result{ScoringVersion: scoring.Version, Score: 45, Grade: "D", QuantumRisk: "high", CurrentRisk: "none",
			Blocks:       []scoring.Block{{ID: "tls13", Points: 15, Max: 15}},
			Cap:          &scoring.Cap{Grade: "C", Codes: []string{"SUITE_RC4"}},
			NotEvaluated: []string{"ssl2_ssl3"}},
		Result:   json.RawMessage(`{"default":{"version":"TLS 1.3"},"key_exchange":{"selected":"x25519"}}`),
		Findings: []Finding{{Code: "SUITE_RC4", Axis: "current", Severity: "high", Category: "cipher_suites", Params: map[string]string{"suites": "RC4-SHA"}}},
	}
	for _, lang := range []string{"es", "en"} {
		in.Lang = lang
		d := build(t, in)
		if d.TLSVersion != "TLS 1.3" || d.SelectedGroup != "x25519" || d.Grade != "D" {
			t.Errorf("%s: facts %q %q %q", lang, d.TLSVersion, d.SelectedGroup, d.Grade)
		}
		for name, s := range map[string]string{"block": d.Blocks[0].Label, "cap": d.Cap.Codes[0].Label, "title": d.Findings[0].Title,
			"remediation": d.Findings[0].Remediation, "severity": d.Findings[0].SeverityLabel, "risk": d.QuantumRisk.Label, "noteval": d.NotEvaluated[0].Label} {
			if s == "" || strings.Contains(s, "{") || s == "SUITE_RC4" || strings.Contains(s, ".") && !strings.Contains(s, " ") {
				t.Errorf("%s: %s not resolved: %q", lang, name, s)
			}
		}
	}
}

func TestBadResult(t *testing.T) {
	if _, err := Build(Input{Result: json.RawMessage(`{`)}); err == nil {
		t.Error("a broken stored result was accepted")
	}
}
