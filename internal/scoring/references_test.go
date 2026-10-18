package scoring

import (
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// notRules report that the scan could not check something; they judge
// nothing, never score, and so cite no source.
var notRules = []string{"SCAN_PARTIAL", "KEX_HYBRID_UNKNOWN", "CERT_MISSING"}

var (
	sourceRe = regexp.MustCompile(`RFC \d+|CVE-\d{4}-\d+|draft-[a-z0-9-]+|NIST (SP|IR) [0-9A-Z-]+|FIPS \d+|CA/Browser Forum`)
	linkRe   = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)
	codeRe   = regexp.MustCompile("`([A-Z0-9_]+)`")
)

// officialHosts are where each kind of source lives.
var officialHosts = []string{"www.rfc-editor.org", "nvd.nist.gov", "csrc.nist.gov", "datatracker.ietf.org", "cabforum.org"}

func officialURL(t *testing.T, raw string) bool {
	t.Helper()
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && slices.Contains(officialHosts, u.Host)
}

// TestEveryFindingHasAReference backs the claim that every rule cites its
// source: each catalog code has a row in the "Referencias por hallazgo" table
// of docs/scoring.md whose sources are all links to their official site, each
// listed with its title under "Fuentes"; and the table has no stale rows.
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
	table, sources, ok := strings.Cut(section, "#### Fuentes")
	if !ok {
		t.Fatal("docs/scoring.md: no «Fuentes» table")
	}

	// Fuentes: source | official title | URL.
	titled := map[string]string{} // URL -> source
	for _, line := range strings.Split(sources, "\n") {
		cells := strings.Split(line, "|")
		if len(cells) < 5 || !strings.HasPrefix(strings.TrimSpace(cells[3]), "https://") {
			continue
		}
		src, title, link := strings.TrimSpace(cells[1]), strings.TrimSpace(cells[2]), strings.TrimSpace(cells[3])
		if !sourceRe.MatchString(src) || title == "" || !officialURL(t, link) {
			t.Errorf("Fuentes: %q needs a source id, an official title and an official https URL (got %q, %q)", src, title, link)
		}
		titled[link] = src
	}

	refs := map[string]string{}
	for _, line := range strings.Split(table, "\n") {
		cells := strings.Split(line, "|")
		if len(cells) < 4 || !strings.Contains(cells[1], "`") {
			continue
		}
		cell := cells[2]
		for _, m := range codeRe.FindAllStringSubmatch(cells[1], -1) {
			if _, dup := refs[m[1]]; dup {
				t.Errorf("%s: more than one row", m[1])
			}
			refs[m[1]] = cell
		}
		// Every source is a link to its official page, listed in Fuentes.
		if rest := linkRe.ReplaceAllString(cell, ""); sourceRe.MatchString(rest) {
			t.Errorf("%q: source %q without a link", strings.TrimSpace(cells[1]), sourceRe.FindString(rest))
		}
		for _, m := range linkRe.FindAllStringSubmatch(cell, -1) {
			switch {
			case !sourceRe.MatchString(m[1]):
				t.Errorf("link %q is not a source", m[1])
			case !officialURL(t, m[2]):
				t.Errorf("%s: %s is not on an official site", m[1], m[2])
			case titled[m[2]] == "":
				t.Errorf("%s: %s is not listed with its title under «Fuentes»", m[1], m[2])
			}
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
		case !linkRe.MatchString(ref):
			t.Errorf("%s: %q links no source", code, strings.TrimSpace(ref))
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
