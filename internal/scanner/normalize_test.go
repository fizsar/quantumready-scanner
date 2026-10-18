package scanner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func loadRaw(t *testing.T, path string) *Result {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var r Result
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	if r.Raw == nil {
		t.Fatalf("%s: no raw result", path)
	}
	return &r
}

// Results written before S6d (here the real scan of fizsar.com and the lab's
// export) hold null for "nothing beyond what crypto/tls already found" and
// list raw versions newest first; Normalize makes both explicit.
func TestNormalizeRawResult(t *testing.T) {
	for _, path := range []string{
		filepath.Join("..", "httpapi", "testdata", "fizsar.com.json"),
		filepath.Join("..", "scoring", "testdata", "lab", "export.json"),
	} {
		r := loadRaw(t, path)
		before, _ := json.Marshal(r.Raw)
		if strings.Contains(path, "fizsar") {
			if !strings.Contains(string(before), `"tls13_groups":null`) || !strings.Contains(string(before), `"tls12_curves":null`) {
				t.Fatalf("fizsar.com: expected the old null lists, got %s", before)
			}
		}
		r.Raw.Normalize()
		after, _ := json.Marshal(r.Raw)
		for _, want := range []string{`"lists_exclude_crypto_tls":true`, `"tls13_groups":[`, `"tls12_curves":[`, `"suites":[`, `"tls13_suites":[`} {
			if !strings.Contains(string(after), want) {
				t.Errorf("%s: %s missing in %s", path, want, after)
			}
		}
		if strings.Contains(string(after), "null") {
			t.Errorf("%s: null left in %s", path, after)
		}
		ids := make([]uint16, len(r.Raw.Versions))
		for i, v := range r.Raw.Versions {
			ids[i] = v.ID
		}
		if !slices.IsSorted(ids) {
			t.Errorf("%s: raw versions not oldest first: %v", path, ids)
		}
		// Nothing else changes, and doing it twice changes nothing.
		var old, now RawResult
		_ = json.Unmarshal(before, &old)
		_ = json.Unmarshal(after, &now)
		if len(old.Suites) != len(now.Suites) || len(old.Versions) != len(now.Versions) || old.Probes != now.Probes || old.TLS13SuitesComplete != now.TLS13SuitesComplete {
			t.Errorf("%s: normalizing changed the content", path)
		}
		r.Raw.Normalize()
		if again, _ := json.Marshal(r.Raw); string(again) != string(after) {
			t.Errorf("%s: not idempotent", path)
		}
	}
	// fizsar.com: crypto/tls found x25519, secp256r1 and X25519MLKEM768; the
	// raw lists are empty because nothing else is accepted, and complete.
	r := loadRaw(t, filepath.Join("..", "httpapi", "testdata", "fizsar.com.json"))
	r.Raw.Normalize()
	var std []string
	for _, g := range r.KeyExchange.Groups {
		if g.Status == Supported {
			std = append(std, g.Name)
		}
	}
	if !slices.Equal(std, []string{"X25519MLKEM768", "x25519", "secp256r1"}) || len(r.Raw.TLS13Groups) != 0 || !r.Raw.TLS13GroupsComplete || len(r.Raw.TLS12Curves) != 0 || !r.Raw.TLS12CurvesComplete {
		t.Errorf("fizsar.com: groups %v, raw tls13 %v (complete %v), raw tls12 %v (complete %v)", std, r.Raw.TLS13Groups, r.Raw.TLS13GroupsComplete, r.Raw.TLS12Curves, r.Raw.TLS12CurvesComplete)
	}
}

// Every new raw result is normalized: the raw run ends with Normalize.
func TestRawRunNormalizes(t *testing.T) {
	b, err := os.ReadFile("raw.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "rs.res.Normalize()\n\tres.Raw = &rs.res") {
		t.Error("runRaw no longer normalizes its result before publishing it")
	}
}
