package scoring

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// notRules report that the scan could not check something; they judge
// nothing, never score, and so cite no source.
var notRules = []string{"SCAN_PARTIAL", "KEX_HYBRID_UNKNOWN", "CERT_MISSING"}

var sourceRe = regexp.MustCompile(`RFC \d+|CVE-\d{4}-\d+|draft-[a-z0-9-]+|NIST (SP|IR) [0-9A-Z-]+|FIPS \d+|CA/Browser Forum`)

// TestEveryFindingHasAReference backs the claim that every rule cites its
// source: each catalog code has a row with a reference in the "Referencias
// por hallazgo" table of docs/scoring.md, and the table has no stale rows.
func TestEveryFindingHasAReference(t *testing.T) {
	b, err := os.ReadFile("../../docs/scoring.md")
	if err != nil {
		t.Fatal(err)
	}
	_, section, ok := strings.Cut(string(b), "### Referencias por hallazgo")
	if !ok {
		t.Fatal("docs/scoring.md: no «Referencias por hallazgo» section")
	}
	section, _, _ = strings.Cut(section, "\n## ")

	refs := map[string]string{}
	codeRe := regexp.MustCompile("`([A-Z0-9_]+)`")
	for _, line := range strings.Split(section, "\n") {
		cells := strings.Split(line, "|")
		if len(cells) < 4 || !strings.Contains(cells[1], "`") {
			continue
		}
		for _, m := range codeRe.FindAllStringSubmatch(cells[1], -1) {
			if _, dup := refs[m[1]]; dup {
				t.Errorf("%s: more than one row", m[1])
			}
			refs[m[1]] = cells[2]
		}
	}

	for code := range findingTexts {
		if slices.Contains(notRules, code) {
			if _, ok := refs[code]; ok {
				t.Errorf("%s is listed as not a rule but has a row", code)
			}
			continue
		}
		ref, ok := refs[code]
		switch {
		case !ok:
			t.Errorf("%s: no row in the references table", code)
		case !sourceRe.MatchString(ref):
			t.Errorf("%s: %q cites no RFC, CVE, draft, NIST, FIPS or CA/Browser Forum source", code, strings.TrimSpace(ref))
		}
	}
	for code := range refs {
		if _, ok := findingTexts[code]; !ok {
			t.Errorf("%s: row for a code that is not in the catalog", code)
		}
	}
	for _, code := range notRules {
		if _, ok := findingTexts[code]; !ok {
			t.Errorf("%s: listed as not a rule but not in the catalog", code)
		}
	}
}
