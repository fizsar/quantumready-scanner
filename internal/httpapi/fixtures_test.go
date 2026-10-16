package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/fizsar/quantumready-saas/internal/scanner"
	"github.com/fizsar/quantumready-saas/internal/scoring"
	"github.com/fizsar/quantumready-saas/internal/store"
)

// fixtureStore serves fixed scans, as the database would.
type fixtureStore struct {
	scans    map[uuid.UUID]store.Scan
	findings map[uuid.UUID][]store.Finding
}

func (f *fixtureStore) EnqueueOrReuse(context.Context, store.Tenant, store.ScanRequest) (store.Scan, bool, error) {
	panic("not used")
}
func (f *fixtureStore) ScanFor(_ context.Context, _ store.Tenant, id uuid.UUID) (store.Scan, error) {
	return f.scans[id], nil
}
func (f *fixtureStore) RecentPublicScan(context.Context, string, int32, time.Duration, time.Duration) (store.Scan, error) {
	panic("not used")
}
func (f *fixtureStore) PublicScanBlocked(context.Context, string) (bool, error) {
	panic("not used")
}
func (f *fixtureStore) OrgScans(context.Context, store.Tenant, int32) ([]store.Scan, error) {
	panic("not used")
}
func (f *fixtureStore) FindingsFor(_ context.Context, _ store.Tenant, id uuid.UUID) ([]store.Finding, error) {
	return f.findings[id], nil
}

// labScan turns a real TLS lab scan into a stored scan, scored with the
// current model. The lab certificates are self-signed (or not captured), so
// they get a valid one: the fixture shows what the TLS setup alone earns.
func labScan(t *testing.T, server, host string) (store.Scan, []store.Finding) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "scoring", "testdata", "lab", server+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var r scanner.Result
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	r.Host = host
	r.Certificate = &scanner.Certificate{
		Subject: host, SANs: []string{host}, Issuer: "YE1", IssuerOrg: []string{"Let's Encrypt"},
		NotBefore: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), NotAfter: time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC),
		ValidityDays: 90, DaysToExpiry: 46, KeyAlgorithm: "ECDSA", KeySize: 256, Curve: "P-256",
		SignatureAlgorithm: "ECDSA-SHA384", Trusted: true, HostnameMatch: true, SCTsEmbedded: 2,
	}
	s := scoring.Evaluate(&r)
	result, _ := json.Marshal(&r)
	scoringJSON, _ := json.Marshal(s)
	created := time.Date(2026, 10, 15, 9, 30, 0, 0, time.UTC)
	finished := created.Add(9 * time.Second)
	sc := store.Scan{ID: uuid.NewSHA1(uuid.NameSpaceURL, []byte(server)), Host: host, Port: 443, Status: "done",
		CreatedAt: created, StartedAt: &created, FinishedAt: &finished, Result: result, Scoring: scoringJSON}
	var fs []store.Finding
	for i, f := range s.Findings {
		params, _ := json.Marshal(f.Params)
		fs = append(fs, store.Finding{ScanID: sc.ID, Position: int32(i), Code: f.Code, Axis: string(f.Axis),
			Severity: string(f.Severity), Category: string(f.Category), Params: params})
	}
	return sc, fs
}

// TestLabFixtures checks the API view of two lab servers and, with
// QR_WRITE_WEB_FIXTURES=1, writes it for the web tests and screenshots
// (web/src/test/lab/<server>.<lang>.json).
func TestLabFixtures(t *testing.T) {
	st := &fixtureStore{scans: map[uuid.UUID]store.Scan{}, findings: map[uuid.UUID][]store.Finding{}}
	servers := map[string]string{"export": "export.tlslab.test", "kyber": "kyber.tlslab.test"}
	ids := map[string]uuid.UUID{}
	for server, host := range servers {
		sc, fs := labScan(t, server, host)
		st.scans[sc.ID], st.findings[sc.ID], ids[server] = sc, fs, sc.ID
	}
	allow, err := ParseAllowlist("export.tlslab.test kyber.tlslab.test")
	if err != nil {
		t.Fatal(err)
	}
	h := NewRouter(Config{Scans: st, Allowlist: allow})
	for server, id := range ids {
		for _, lang := range []string{"es", "en"} {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/scans/"+id.String()+"?lang="+lang, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("%s/%s: %d %s", server, lang, rec.Code, rec.Body)
			}
			var v scanView
			if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, f := range v.Report.Findings {
				got = append(got, f.Code)
			}
			want := map[string][]string{"export": {"SUITE_NULL", "SUITE_EXPORT", "DH_WEAK"}, "kyber": {"KEX_EXPERIMENTAL_PQC_ONLY"}}[server]
			for _, c := range want {
				if !slices.Contains(got, c) {
					t.Errorf("%s/%s: %s missing from %v", server, lang, c, got)
				}
			}
			if v.Report.Cap != nil {
				for _, c := range v.Report.Cap.Codes {
					if strings.Contains(c.Label, "{") {
						t.Errorf("%s/%s: unfilled placeholder in the cap label %q", server, lang, c.Label)
					}
				}
			}
			if v.Report.ScoringVersion != scoring.Version || v.Lang != lang {
				t.Errorf("%s/%s: version %s, lang %s", server, lang, v.Report.ScoringVersion, v.Lang)
			}
			if os.Getenv("QR_WRITE_WEB_FIXTURES") == "1" {
				dir := filepath.Join("..", "..", "web", "src", "test", "lab")
				if err := os.MkdirAll(dir, 0o750); err != nil {
					t.Fatal(err)
				}
				var pretty json.RawMessage = rec.Body.Bytes()
				out, _ := json.MarshalIndent(pretty, "", "  ")
				if err := os.WriteFile(filepath.Join(dir, server+"."+lang+".json"), append(out, '\n'), 0o600); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
