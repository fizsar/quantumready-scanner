package scoring

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/fizsar/quantumready-saas/internal/scanner"
)

// Old stored results (null lists, raw versions newest first) and their
// normalized form score exactly the same: no stored grade changes meaning.
func TestNormalizedRawScoresTheSame(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("testdata", "lab", "*.json"))
	files = append(files, filepath.Join("..", "httpapi", "testdata", "fizsar.com.json"))
	for _, f := range files {
		r := labResult(t, f)
		n := labResult(t, f)
		n.Raw.Normalize()
		a, b := Evaluate(r), Evaluate(n)
		if !reflect.DeepEqual(a, b) {
			t.Errorf("%s: %d %s %v vs normalized %d %s %v", f, a.Score, a.Grade, codes(a), b.Score, b.Grade, codes(b))
		}
	}
}

func labResult(t *testing.T, path string) *scanner.Result {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var r scanner.Result
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	if r.Raw == nil {
		t.Fatalf("%s: no raw result", path)
	}
	if r.Certificate == nil {
		r.Certificate = ideal().Certificate
	}
	return &r
}
