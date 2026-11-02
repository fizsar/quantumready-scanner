package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/fizsar/quantumready-saas/internal/scanner"
	"github.com/fizsar/quantumready-saas/internal/scoring"
	"github.com/fizsar/quantumready-saas/internal/store"
	"github.com/fizsar/quantumready-saas/internal/testdb"
	"github.com/fizsar/quantumready-saas/internal/worker"
	"github.com/fizsar/quantumready-saas/migrations"
)

type scanAPI struct {
	h  http.Handler
	st *store.Store
}

func newScanAPI(t *testing.T, limiter *Limiter, allow string) *scanAPI {
	t.Helper()
	st := store.NewStore(testdb.Schema(t, func(ctx context.Context, db *sql.DB) error {
		_, err := migrations.Up(ctx, db)
		return err
	}))
	al, err := ParseAllowlist(allow)
	if err != nil {
		t.Fatal(err)
	}
	trusted, _ := ParsePrefixes("172.18.0.0/16")
	return &scanAPI{
		st: st,
		h: NewRouter(Config{
			DB: fakePinger{}, Scans: st, Allowlist: al, Limiter: limiter, TrustedProxies: trusted,
			Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		}),
	}
}

type reqOpts struct {
	remote, xff, lang, contentType string
}

func (a *scanAPI) do(t *testing.T, method, path, body string, o reqOpts) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.RemoteAddr = "203.0.113.10:4000"
	if o.remote != "" {
		r.RemoteAddr = o.remote
	}
	if o.xff != "" {
		r.Header.Set("X-Forwarded-For", o.xff)
	}
	if o.lang != "" {
		r.Header.Set("Accept-Language", o.lang)
	}
	ct := "application/json"
	if o.contentType != "" {
		ct = o.contentType
	}
	if method == http.MethodPost {
		r.Header.Set("Content-Type", ct)
	}
	rec := httptest.NewRecorder()
	a.h.ServeHTTP(rec, r)
	for k := range rec.Header() {
		if strings.HasPrefix(strings.ToLower(k), "access-control-") {
			t.Errorf("%s %s: CORS header %s set", method, path, k)
		}
	}
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return v
}

type errBody struct {
	Error apiError `json:"error"`
}

func wantError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) apiError {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, status, rec.Body.String())
	}
	e := decode[errBody](t, rec).Error
	if e.Code != code || e.Message == "" {
		t.Fatalf("error = %+v, want code %s with a message", e, code)
	}
	return e
}

func TestCreateScan(t *testing.T) {
	a := newScanAPI(t, nil, "fizsar.com,quantumready.fizsar.com")

	rec := a.do(t, "POST", "/api/scans", `{"host":"https://FIZSAR.com/some/path"}`, reqOpts{})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	v := decode[scanView](t, rec)
	if v.Host != "fizsar.com" || v.Port != 443 || v.Status != "queued" || v.Reused || v.Report != nil || v.Lang != "es" {
		t.Fatalf("view = %+v", v)
	}
	if loc := rec.Header().Get("Location"); loc != "/api/scans/"+v.ID.String() {
		t.Errorf("Location = %q", loc)
	}
	if rec.Header().Get("Content-Language") != "es" || rec.Header().Get("X-Content-Type-Options") != "nosniff" ||
		!strings.Contains(rec.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Errorf("headers = %v", rec.Header())
	}

	// Same target within the cooldown: the same scan, 200, reused.
	rec = a.do(t, "POST", "/api/scans", `{"host":"fizsar.com"}`, reqOpts{})
	again := decode[scanView](t, rec)
	if rec.Code != http.StatusOK || !again.Reused || again.ID != v.ID {
		t.Fatalf("second request = %d %+v, want 200 reused %s", rec.Code, again, v.ID)
	}

	got := a.do(t, "GET", "/api/scans/"+v.ID.String(), "", reqOpts{})
	if got.Code != http.StatusOK || decode[scanView](t, got).ID != v.ID {
		t.Fatalf("GET = %d %s", got.Code, got.Body.String())
	}
}

func TestListTargets(t *testing.T) {
	a := newScanAPI(t, nil, "quantumready.fizsar.com, fizsar.com")
	rec := a.do(t, "GET", "/api/targets", "", reqOpts{})
	got := decode[struct {
		Hosts []string `json:"hosts"`
		Port  int      `json:"port"`
	}](t, rec)
	if rec.Code != 200 || strings.Join(got.Hosts, ",") != "fizsar.com,quantumready.fizsar.com" || got.Port != 443 {
		t.Fatalf("GET /api/targets = %d %+v", rec.Code, got)
	}
}

func TestCreateScanRejects(t *testing.T) {
	a := newScanAPI(t, nil, "fizsar.com")
	for _, tt := range []struct {
		name, body, contentType string
		status                  int
		code                    string
	}{
		{"not on the allowlist", `{"host":"example.com"}`, "", 403, "host_not_allowed"},
		{"subdomain is not an exact match", `{"host":"www.fizsar.com"}`, "", 403, "host_not_allowed"},
		{"IP literal", `{"host":"149.56.13.98"}`, "", 403, "host_not_allowed"},
		{"invalid host", `{"host":"not a host"}`, "", 400, "invalid_host"},
		{"port other than 443", `{"host":"fizsar.com:8443"}`, "", 400, "port_not_allowed"},
		{"URL with port 80", `{"host":"http://fizsar.com:80/"}`, "", 400, "port_not_allowed"},
		{"empty host", `{"host":""}`, "", 400, "invalid_host"},
		{"invalid JSON", `{"host":`, "", 400, "invalid_json"},
		{"unknown field", `{"host":"fizsar.com","port":8443}`, "", 400, "invalid_json"},
		{"form post", `host=fizsar.com`, "application/x-www-form-urlencoded", 415, "unsupported_media_type"},
		{"text/plain (cross-site simple request)", `{"host":"fizsar.com"}`, "text/plain", 415, "unsupported_media_type"},
		{"body too large", `{"host":"` + strings.Repeat("a", 2000) + `.com"}`, "", 413, "body_too_large"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			wantError(t, a.do(t, "POST", "/api/scans", tt.body, reqOpts{contentType: tt.contentType}), tt.status, tt.code)
		})
	}

	e := wantError(t, a.do(t, "POST", "/api/scans", `{"host":"example.com"}`, reqOpts{lang: "en-US,en;q=0.9"}), 403, "host_not_allowed")
	if e.Message != "Right now, scanning is limited to the demo domains." {
		t.Errorf("English message = %q", e.Message)
	}
	esMsg := wantError(t, a.do(t, "POST", "/api/scans", `{"host":"example.com"}`, reqOpts{lang: "es"}), 403, "host_not_allowed")
	if esMsg.Message != "Ahora mismo el escaneo está limitado a los dominios de demostración." {
		t.Errorf("Spanish message = %q", esMsg.Message)
	}
	es := wantError(t, a.do(t, "POST", "/api/scans", `{"host":"fizsar.com:8443"}`, reqOpts{lang: "es"}), 400, "port_not_allowed")
	en := wantError(t, a.do(t, "POST", "/api/scans", `{"host":"fizsar.com:8443"}`, reqOpts{lang: "en"}), 400, "port_not_allowed")
	if !strings.Contains(es.Message, "puerto 443") || !strings.Contains(en.Message, "port 443") {
		t.Errorf("port messages = %q / %q", es.Message, en.Message)
	}
	if rec := a.do(t, "POST", "/api/scans", `{"host":"https://fizsar.com:443/"}`, reqOpts{}); rec.Code != 202 {
		t.Errorf("explicit :443 = %d, want 202", rec.Code)
	}
	wantError(t, a.do(t, "OPTIONS", "/api/scans", "", reqOpts{}), 405, "method_not_allowed")
	wantError(t, a.do(t, "DELETE", "/api/scans/"+uuid.NewString(), "", reqOpts{}), 405, "method_not_allowed")
	wantError(t, a.do(t, "GET", "/api/nope", "", reqOpts{}), 404, "not_found")
}

// TestHostNotAllowedRevealsNothing: the 403 body is only the code and a fixed
// message. No allowlisted host, no setting name, no internal detail; the demo
// domains are public through GET /api/targets, and only there.
func TestHostNotAllowedRevealsNothing(t *testing.T) {
	a := newScanAPI(t, nil, "fizsar.com,quantumready.fizsar.com,secret-demo.example.org")
	for _, lang := range []string{"es", "en"} {
		rec := a.do(t, "POST", "/api/scans", `{"host":"example.com"}`, reqOpts{lang: lang})
		if rec.Code != 403 {
			t.Fatalf("%s: %d", lang, rec.Code)
		}
		var body map[string]map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if len(body) != 1 || len(body["error"]) != 2 || body["error"]["code"] != "host_not_allowed" {
			t.Errorf("%s: body %s, want only error.code and error.message", lang, rec.Body)
		}
		raw := strings.ToLower(rec.Body.String())
		for _, leak := range []string{"fizsar", "secret-demo", "example.com", "allowlist", "scan_", "trusted", "proxy", "rate", "limit", "config", ".go", "goroutine", "postgres"} {
			if strings.Contains(raw, leak) {
				t.Errorf("%s: 403 body mentions %q: %s", lang, leak, rec.Body)
			}
		}
	}
}

func TestRateLimit(t *testing.T) {
	a := newScanAPI(t, NewLimiter(2, 30), "a.example.com,b.example.com,c.example.com,d.example.com")
	post := func(host string, o reqOpts) *httptest.ResponseRecorder {
		return a.do(t, "POST", "/api/scans", `{"host":"`+host+`"}`, o)
	}

	client := reqOpts{remote: "203.0.113.20:1000"}
	if rec := post("a.example.com", client); rec.Code != 202 {
		t.Fatalf("1st = %d", rec.Code)
	}
	if rec := post("a.example.com", client); rec.Code != 200 { // reused: does not count
		t.Fatalf("reuse = %d", rec.Code)
	}
	if rec := post("b.example.com", client); rec.Code != 202 {
		t.Fatalf("2nd = %d", rec.Code)
	}
	rec := post("c.example.com", client)
	wantError(t, rec, 429, "rate_limited")
	if ra, err := strconv.Atoi(rec.Header().Get("Retry-After")); err != nil || ra < 1 || ra > 61 {
		t.Errorf("Retry-After = %q, want 1-61 seconds", rec.Header().Get("Retry-After"))
	}

	// A spoofed X-Forwarded-For from a direct client changes nothing.
	wantError(t, post("c.example.com", reqOpts{remote: "203.0.113.20:1001", xff: "198.51.100.77"}), 429, "rate_limited")

	// Behind Caddy, each forwarded client has its own budget.
	if rec := post("c.example.com", reqOpts{remote: "172.18.0.2:5000", xff: "198.51.100.1"}); rec.Code != 202 {
		t.Fatalf("forwarded client A = %d", rec.Code)
	}
	if rec := post("d.example.com", reqOpts{remote: "172.18.0.2:5001", xff: "198.51.100.2"}); rec.Code != 202 {
		t.Fatalf("forwarded client B = %d", rec.Code)
	}
}

func TestGetScanReport(t *testing.T) {
	a := newScanAPI(t, nil, "fizsar.com")
	v := decode[scanView](t, a.do(t, "POST", "/api/scans", `{"host":"fizsar.com"}`, reqOpts{}))

	// A worker completes it with a canned scan result.
	ws := store.NewStore(testdb.AsRole(t, a.st.Pool(), store.RoleWorker)) // production privileges
	w, err := worker.New(ws, cannedScanner{}, worker.Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := w.ProcessOne(context.Background()); !ok || err != nil {
		t.Fatalf("ProcessOne = %v, %v", ok, err)
	}

	es := decode[scanView](t, a.do(t, "GET", "/api/scans/"+v.ID.String(), "", reqOpts{lang: "es-ES"}))
	en := decode[scanView](t, a.do(t, "GET", "/api/scans/"+v.ID.String()+"?lang=en", "", reqOpts{lang: "es-ES"}))
	for _, got := range []scanView{es, en} {
		r := got.Report
		if got.Status != "done" || r == nil {
			t.Fatalf("view = %+v", got)
		}
		// Model 2026.3: security today (3 blocks) and quantum readiness.
		if r.Grade != "A" || r.Score != 100 || !r.PQC || r.SelectedGroup != "X25519MLKEM768" || r.TLSVersion != "TLS 1.3" ||
			r.ScoringVersion != scoring.Version || len(r.Blocks) != 3 || r.Readiness == nil || r.Readiness.Score != 75 || r.OutdatedModel ||
			len(r.Findings) == 0 || len(r.NotEvaluated) == 0 || len(r.Result) == 0 {
			t.Fatalf("report = %+v", r)
		}
		for _, f := range r.Findings {
			if f.Title == "" || f.Detail == "" || f.SeverityLabel == "" || f.AxisLabel == "" || strings.Contains(f.Title+f.Detail, "{") {
				t.Errorf("finding %s not rendered: %+v", f.Code, f)
			}
		}
	}
	if es.Lang != "es" || en.Lang != "en" {
		t.Errorf("langs = %s %s", es.Lang, en.Lang)
	}
	if es.Report.Readiness.Blocks[0].Label != "Intercambio de claves post-cuántico" || en.Report.Readiness.Blocks[0].Label != "Post-quantum key exchange" ||
		es.Report.Blocks[1].Label == en.Report.Blocks[1].Label || es.Report.Readiness.Level.Label != "Preparado" || en.Report.Readiness.Level.Label != "Ready" {
		t.Errorf("labels = %q / %q, %q / %q", es.Report.Readiness.Blocks[0].Label, en.Report.Readiness.Blocks[0].Label, es.Report.Blocks[1].Label, en.Report.Blocks[1].Label)
	}
	if es.Report.Findings[0].Title == en.Report.Findings[0].Title {
		t.Errorf("finding not translated: %q", es.Report.Findings[0].Title)
	}
}

func TestGetScanFailedAndNotFound(t *testing.T) {
	a := newScanAPI(t, nil, "fizsar.com")
	v := decode[scanView](t, a.do(t, "POST", "/api/scans", `{"host":"fizsar.com"}`, reqOpts{}))
	c, err := a.st.Claim(context.Background(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.st.Fail(context.Background(), c.ID, *c.LeaseToken, "connection_refused"); err != nil {
		t.Fatal(err)
	}
	got := decode[scanView](t, a.do(t, "GET", "/api/scans/"+v.ID.String()+"?lang=en", "", reqOpts{}))
	if got.Status != "failed" || got.Error == nil || got.Error.Code != "connection_refused" ||
		got.Error.Message != "The server refused the connection on port 443." || got.Report != nil {
		t.Fatalf("failed view = %+v", got)
	}

	wantError(t, a.do(t, "GET", "/api/scans/"+uuid.NewString(), "", reqOpts{}), 404, "not_found")
	wantError(t, a.do(t, "GET", "/api/scans/not-a-uuid", "", reqOpts{}), 404, "not_found")
}

type cannedScanner struct{}

func (cannedScanner) ScanTimeout() time.Duration { return time.Second }

func (cannedScanner) ScanWith(_ context.Context, host string, port int, _ scanner.Intensity) (*scanner.Result, error) {
	return &scanner.Result{
		SchemaVersion: scanner.SchemaVersion, Host: host, Port: port,
		Default:             scanner.Handshake{Status: scanner.Supported, Version: "TLS 1.3", CipherSuite: "TLS_AES_128_GCM_SHA256", Group: "X25519MLKEM768"},
		KeyExchange:         scanner.KeyExchange{Selected: "X25519MLKEM768", SelectedHybrid: true, Groups: []scanner.GroupProbe{{Name: "X25519MLKEM768", Hybrid: true, Status: scanner.Supported}}},
		Versions:            []scanner.VersionProbe{{Version: "TLS 1.2", Status: scanner.Supported}, {Version: "TLS 1.3", Status: scanner.Supported}},
		TLS12Suites:         []scanner.CipherSuite{{Name: "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256", ForwardSecrecy: true, AEAD: true}},
		TLS12SuitesComplete: true, LegacySuitesComplete: true, TLS13Suite: "TLS_AES_128_GCM_SHA256",
		Certificate: &scanner.Certificate{
			Trusted: true, HostnameMatch: true, KeyAlgorithm: "ECDSA", KeySize: 256, Curve: "P-256",
			SignatureAlgorithm: "ECDSA-SHA384", ValidityDays: 90, DaysToExpiry: 60, IssuerOrg: []string{"Let's Encrypt"}, SCTsEmbedded: 2,
		},
		NotEvaluated: []string{scanner.NotEvalSSL, scanner.NotEvalDHESuites},
	}, nil
}
