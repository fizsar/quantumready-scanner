package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/fizsar/quantumready-saas/internal/store"
)

// TestAPIJSONGolden pins, byte for byte, what GET /api/scans/{id} returns
// for the ten TLS lab servers, the real scan of fizsar.com and a queued and a
// failed scan, in Spanish and English: testdata/apijson/<name>.<lang>.json.
// The files were written before milestone 2.8.3 moved the report out of this
// package (internal/report), so that refactor provably changed nothing; on
// main they were written with main's code before the merge (eb16990), whose
// queued scans also carry queue_position and queue_eta_seconds. A
// deliberate change of the API output regenerates them with
// QR_WRITE_API_GOLDEN=1, and the diff shows what changed.
func TestAPIJSONGolden(t *testing.T) {
	st := &fixtureStore{scans: map[uuid.UUID]store.Scan{}, findings: map[uuid.UUID][]store.Finding{}}
	ids := map[string]uuid.UUID{}
	labs, err := filepath.Glob(filepath.Join("..", "scoring", "testdata", "lab", "*.json"))
	if err != nil || len(labs) != 10 {
		t.Fatalf("lab results: %v, %d files", err, len(labs))
	}
	for _, f := range labs {
		server := filepath.Base(f[:len(f)-len(".json")])
		host := server + ".tlslab.test"
		sc, fs := labScan(t, server, host)
		st.scans[sc.ID], st.findings[sc.ID], ids[server] = sc, fs, sc.ID
	}
	sc, fs := realScan(t, "fizsar.com")
	st.scans[sc.ID], st.findings[sc.ID], ids["fizsar.com"] = sc, fs, sc.ID
	created := time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)
	timeout := "timeout"
	for name, s := range map[string]store.Scan{
		"queued": {Host: "queued.example", Port: 443, Status: "queued", CreatedAt: created},
		"failed": {Host: "failed.example", Port: 443, Status: "failed", Retries: 1, Error: &timeout, CreatedAt: created,
			StartedAt: &created, FinishedAt: &created},
	} {
		s.ID = uuid.NewSHA1(uuid.NameSpaceURL, []byte(name))
		st.scans[s.ID], ids[name] = s, s.ID
	}
	allow, err := ParseAllowlist("fizsar.com")
	if err != nil {
		t.Fatal(err)
	}
	h := NewRouter(Config{Scans: st, Allowlist: allow})
	names := make([]string, 0, len(ids))
	for n := range ids {
		names = append(names, n)
	}
	sort.Strings(names)
	write := os.Getenv("QR_WRITE_API_GOLDEN") == "1"
	for _, name := range names {
		for _, lang := range []string{"es", "en"} {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/scans/"+ids[name].String()+"?lang="+lang, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("%s/%s: %d %s", name, lang, rec.Code, rec.Body)
			}
			path := filepath.Join("testdata", "apijson", name+"."+lang+".json")
			if write {
				if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, rec.Body.Bytes(), 0o600); err != nil {
					t.Fatal(err)
				}
				continue
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(rec.Body.Bytes(), want) {
				t.Errorf("%s: the API output changed (%d bytes, golden %d); if deliberate, regenerate with QR_WRITE_API_GOLDEN=1", path, rec.Body.Len(), len(want))
			}
		}
	}
	if len(names) != 13 {
		t.Errorf("%d scans, want 13 (10 lab servers, fizsar.com, queued, failed)", len(names))
	}
}
