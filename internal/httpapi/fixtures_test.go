package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
func (f *fixtureStore) PublicQueueLength(context.Context) (int64, error) { return 0, nil }
func (f *fixtureStore) PublicQueuePosition(context.Context, time.Time) (int64, error) {
	return 0, nil
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
// they get a simulated valid one (labCert): the fixture shows what the TLS
// setup earns, with the key the server really has.
func labScan(t *testing.T, server, host string) (store.Scan, []store.Finding) {
	t.Helper()
	r := readResult(t, filepath.Join("..", "scoring", "testdata", "lab", server+".json"))
	r.Host = host
	r.Certificate = labCert(host, r.Certificate)
	return storedScan(server, host, r)
}

// labCert is the simulated certificate of a lab server. Key type and size
// are those of the server's real self-signed certificate: the one the scan
// captured or, when crypto/tls could not connect, the lab image's (openssl102
// and openssl35 make an RSA 2048 key; kyber an ECDSA P-256 one, and the scan
// captures it). The issuer is plainly the lab's, not an ACME CA, and there are
// no SCTs, as on any private certificate; trusted, so the grade judges the
// TLS setup rather than the lack of a public CA.
func labCert(host string, real *scanner.Certificate) *scanner.Certificate {
	alg, size, curve := "RSA", 2048, ""
	if real != nil {
		alg, size, curve = real.KeyAlgorithm, real.KeySize, real.Curve
	}
	sig := "SHA256-RSA"
	if alg == "ECDSA" {
		sig = "ECDSA-SHA256"
	}
	return &scanner.Certificate{
		Subject: host, SANs: []string{host}, Issuer: "QuantumReady TLS lab CA (simulated)", IssuerOrg: []string{"QuantumReady TLS lab"},
		NotBefore: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), NotAfter: time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC),
		ValidityDays: 90, DaysToExpiry: 46, KeyAlgorithm: alg, KeySize: size, Curve: curve,
		SignatureAlgorithm: sig, Trusted: true, HostnameMatch: true,
	}
}

// TestCertFitsSuites: the check catches the old fixtures' mistake (export's
// RSA suites with an ECDSA certificate) and accepts the lab's real keys.
func TestCertFitsSuites(t *testing.T) {
	r := readResult(t, filepath.Join("..", "scoring", "testdata", "lab", "export.json"))
	r.Certificate = &scanner.Certificate{KeyAlgorithm: "ECDSA", KeySize: 256, Curve: "P-256"}
	if certFitsSuites(r) == nil {
		t.Error("export with an ECDSA certificate passed")
	}
	r.Certificate = labCert("export.tlslab.test", nil)
	if err := certFitsSuites(r); err != nil || r.Certificate.KeyAlgorithm != "RSA" || r.Certificate.KeySize != 2048 {
		t.Errorf("export with the lab's RSA 2048 certificate: %v, %+v", err, r.Certificate)
	}
	k := readResult(t, filepath.Join("..", "scoring", "testdata", "lab", "kyber.json"))
	if c := labCert("kyber.tlslab.test", k.Certificate); c.KeyAlgorithm != "ECDSA" || c.Curve != "P-256" {
		t.Errorf("kyber: %+v, want the ECDSA P-256 key its server really has", c)
	}
}

// certFitsSuites reports a certificate whose key cannot serve the accepted
// suites: *_RSA_* suites need an RSA key, *_ECDSA_* suites an ECDSA one
// (anonymous suites need none).
func certFitsSuites(r *scanner.Result) error {
	if r.Certificate == nil {
		return nil
	}
	var names []string
	for _, list := range [][]scanner.CipherSuite{r.TLS12Suites, r.TLS11Suites, r.TLS10Suites} {
		for _, s := range list {
			names = append(names, s.Name)
		}
	}
	if r.Raw != nil {
		for _, s := range r.Raw.Suites {
			names = append(names, s.Name)
		}
	}
	for _, n := range names {
		switch {
		case strings.Contains(n, "_RSA_") && r.Certificate.KeyAlgorithm != "RSA":
			return fmt.Errorf("%s needs an RSA certificate, the fixture has %s", n, r.Certificate.KeyAlgorithm)
		case strings.Contains(n, "_ECDSA_") && r.Certificate.KeyAlgorithm != "ECDSA":
			return fmt.Errorf("%s needs an ECDSA certificate, the fixture has %s", n, r.Certificate.KeyAlgorithm)
		}
	}
	return nil
}

// realScan is a scan of one of our own domains, kept as the scanner wrote it
// (cmd/qrscan -json, the "scan" part), certificate included.
func realScan(t *testing.T, host string) (store.Scan, []store.Finding) {
	t.Helper()
	return storedScan(host, host, readResult(t, filepath.Join("testdata", host+".json")))
}

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

func storedScan(server, host string, r *scanner.Result) (store.Scan, []store.Finding) {
	r.Raw.Normalize() // the form the scanner writes today (these scans predate it)
	s := scoring.Evaluate(r)
	result, _ := json.Marshal(r)
	scoringJSON, _ := json.Marshal(s)
	// The real times of the scan: /examples shows the date.
	created := r.StartedAt.UTC().Truncate(time.Second)
	finished := created.Add(time.Duration(r.DurationMS) * time.Millisecond).Truncate(time.Second)
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

// TestLabFixtures checks the API view of two lab servers and of a real scan of
// fizsar.com against web/src/test/lab/<server>.<lang>.json, which the web
// tests and /examples use: the fixtures must be what the API returns today,
// scored with the current model. A model change that moves a score fails here
// (pinned below) and leaves the files to regenerate with QR_WRITE_WEB_FIXTURES=1.
func TestLabFixtures(t *testing.T) {
	// Grade and score shown on /examples; its texts explain these numbers.
	pinned := map[string]struct {
		grade string
		score int
	}{"fizsar": {"A", 95}, "modern": {"D", 40}, "kyber": {"D", 40}, "export": {"F", 15}}
	st := &fixtureStore{scans: map[uuid.UUID]store.Scan{}, findings: map[uuid.UUID][]store.Finding{}}
	servers := map[string]string{"export": "export.tlslab.test", "kyber": "kyber.tlslab.test", "modern": "modern.tlslab.test"}
	ids := map[string]uuid.UUID{}
	for server, host := range servers {
		sc, fs := labScan(t, server, host)
		st.scans[sc.ID], st.findings[sc.ID], ids[server] = sc, fs, sc.ID
	}
	sc, fs := realScan(t, "fizsar.com")
	st.scans[sc.ID], st.findings[sc.ID], ids["fizsar"] = sc, fs, sc.ID
	allow, err := ParseAllowlist("export.tlslab.test kyber.tlslab.test modern.tlslab.test fizsar.com")
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
			want := map[string][]string{"export": {"SUITE_NULL", "SUITE_EXPORT", "DH_WEAK"}, "kyber": {"KEX_EXPERIMENTAL_PQC_ONLY"},
				"fizsar": {"KEX_HYBRID_PREFERRED", "CERT_VALID"}, "modern": {"KEX_NO_HYBRID", "FS_ALL", "TLS13_SUPPORTED"}}[server]
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
			var res scanner.Result
			if err := json.Unmarshal(v.Report.Result, &res); err != nil {
				t.Fatal(err)
			}
			// The classical-signature advice follows the renewal finding:
			// fizsar.com renews automatically, the lab certificates do not.
			for _, f := range v.Report.Findings {
				if f.Code != "CERT_CLASSICAL_SIGNATURE" {
					continue
				}
				keep := map[string]string{"es": "Mantén la renovación", "en": "Keep renewal"}[lang]
				automate := map[string]string{"es": "Automatiza la renovación", "en": "Automate renewal"}[lang]
				if want := map[bool]string{true: keep, false: automate}[server == "fizsar"]; !strings.Contains(f.Remediation, want) {
					t.Errorf("%s/%s: classical-signature advice %q, want %q", server, lang, f.Remediation, want)
				}
			}
			if err := certFitsSuites(&res); err != nil {
				t.Errorf("%s/%s: %v", server, lang, err)
			}
			if v.Report.ScoringVersion != scoring.Version || v.Lang != lang {
				t.Errorf("%s/%s: version %s, lang %s", server, lang, v.Report.ScoringVersion, v.Lang)
			}
			if p := pinned[server]; v.Report.Grade != p.grade || v.Report.Score != p.score {
				t.Errorf("%s/%s: %s · %d with the current model, /examples explains %s · %d", server, lang,
					v.Report.Grade, v.Report.Score, p.grade, p.score)
			}
			var pretty json.RawMessage = rec.Body.Bytes()
			out, _ := json.MarshalIndent(pretty, "", "  ")
			out = append(out, '\n')
			path := filepath.Join("..", "..", "web", "src", "test", "lab", server+"."+lang+".json")
			if os.Getenv("QR_WRITE_WEB_FIXTURES") == "1" {
				if err := os.WriteFile(path, out, 0o600); err != nil {
					t.Fatal(err)
				}
				continue
			}
			if old, err := os.ReadFile(path); err != nil || !bytes.Equal(old, out) {
				t.Errorf("%s differs from what the API returns now (%v): regenerate with QR_WRITE_WEB_FIXTURES=1", path, err)
			}
		}
	}
}
