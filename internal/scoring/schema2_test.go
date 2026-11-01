package scoring

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fizsar/quantumready-saas/internal/scanner"
)

// The v2 results in testdata/lab_v2 were written on 27/10/2026 by the scanner
// of milestone 2.8.4 (schema 2: probe log, error codes, chain OIDs), for the
// ten servers of the TLS lab (intensity full, as in TestLab) and a real scan
// of fizsar.com. The v1 results are the ones scoring has been tested with
// (testdata/lab, from 14/10/2026) and internal/httpapi/testdata/fizsar.com.json.

func readResult(t *testing.T, path string) *scanner.Result {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var r scanner.Result
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	return &r
}

func judgement(t *testing.T, r *scanner.Result) string {
	t.Helper()
	b, err := json.Marshal(Evaluate(r))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Evaluate(v1) == Evaluate(v2) for every lab server and fizsar.com: the same
// server, scanned before and after schema 2, gets the same judgement, down to
// the findings and their parameters.
func TestSchema2SameJudgement(t *testing.T) {
	v2, _ := filepath.Glob(filepath.Join("testdata", "lab_v2", "*.json"))
	if len(v2) != 11 {
		t.Fatalf("%d v2 results, want 11 (10 lab servers and fizsar.com)", len(v2))
	}
	for _, f := range v2 {
		name := strings.TrimSuffix(filepath.Base(f), ".json")
		t.Run(name, func(t *testing.T) {
			v1path := filepath.Join("testdata", "lab", name+".json")
			if name == "fizsar.com" {
				v1path = filepath.Join("..", "httpapi", "testdata", "fizsar.com.json")
			}
			old, cur := readResult(t, v1path), readResult(t, f)
			if old.SchemaVersion != 1 || cur.SchemaVersion != 2 || cur.ProbeLog == nil || len(cur.ProbeLog.Probes) == 0 {
				t.Fatalf("schema %d → %d, log %v", old.SchemaVersion, cur.SchemaVersion, cur.ProbeLog != nil)
			}
			// fizsar.com was scanned ten days apart: the same certificate
			// (same notAfter) then had ten days less to expiry, the one fact
			// that depends on when the scan ran. Align it, and only it.
			if o, c := old.Certificate, cur.Certificate; o != nil && c != nil && o.DaysToExpiry != c.DaysToExpiry {
				if !o.NotAfter.Equal(c.NotAfter) {
					t.Fatalf("another certificate (%s, %s): not comparable", o.NotAfter, c.NotAfter)
				}
				c.DaysToExpiry = o.DaysToExpiry
			}
			if a, b := judgement(t, old), judgement(t, cur); a != b {
				t.Errorf("v1 and v2 judged differently\nv1 %s\nv2 %s", a, b)
			}
		})
	}
}

// Scoring reads none of the schema 2 fields: a v2 result judged with and
// without them gives the same judgement.
func TestSchema2FieldsAreNotScored(t *testing.T) {
	v2, _ := filepath.Glob(filepath.Join("testdata", "lab_v2", "*.json"))
	for _, f := range v2 {
		r := readResult(t, f)
		with := judgement(t, r)
		r.ProbeLog = nil
		r.Default.ErrorCode = ""
		for i := range r.KeyExchange.Groups {
			r.KeyExchange.Groups[i].ErrorCode = ""
		}
		for i := range r.Versions {
			r.Versions[i].ErrorCode = ""
		}
		if r.Raw != nil {
			r.Raw.SSLv2.ErrorCode, r.Raw.SSLv3.ErrorCode = "", ""
			for i := range r.Raw.Versions {
				r.Raw.Versions[i].ErrorCode = ""
			}
		}
		if c := r.Certificate; c != nil {
			c.TrustErrorCode = ""
			for i := range c.Chain {
				c.Chain[i].SignatureOID, c.Chain[i].PublicKeyOID = "", ""
			}
		}
		if without := judgement(t, r); with != without {
			t.Errorf("%s: the schema 2 fields change the judgement", filepath.Base(f))
		}
	}
}
