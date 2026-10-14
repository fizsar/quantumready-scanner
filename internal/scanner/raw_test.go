package scanner

import (
	"crypto/tls"
	"slices"
	"testing"
)

func TestFullIntensityAgainstGo(t *testing.T) {
	pki := newTestPKI(t)
	cert := pki.leaf(t, leafOpts{})
	port := startTLS(t, &tls.Config{Certificates: []tls.Certificate{cert},
		CurvePreferences: []tls.CurveID{tls.X25519MLKEM768, tls.X25519, tls.CurveP256}})
	res := resolverFor(testHost, "127.0.0.1")
	r := scan(t, newTestScanner(pki, Options{Intensity: IntensityFull, Resolver: res}), port)

	if r.Intensity != IntensityFull || r.Raw == nil {
		t.Fatalf("intensity %q, raw %v", r.Intensity, r.Raw)
	}
	raw := r.Raw
	if raw.SSLv2.Status != NotSupported || raw.SSLv3.Status != NotSupported {
		t.Errorf("SSLv2 %+v, SSLv3 %+v: Go speaks neither", raw.SSLv2, raw.SSLv3)
	}
	if len(raw.Suites) != 0 || !raw.SuitesComplete {
		t.Errorf("raw suites %+v (complete %v): Go has nothing beyond what crypto/tls offers", raw.Suites, raw.SuitesComplete)
	}
	var tls13 []string
	for _, s := range raw.TLS13Suites {
		tls13 = append(tls13, s.Name)
	}
	slices.Sort(tls13)
	if want := []string{"TLS_AES_128_GCM_SHA256", "TLS_AES_256_GCM_SHA384", "TLS_CHACHA20_POLY1305_SHA256"}; !slices.Equal(tls13, want) || !raw.TLS13SuitesComplete {
		t.Errorf("TLS 1.3 suites %v (complete %v), want %v", tls13, raw.TLS13SuitesComplete, want)
	}
	if len(raw.TLS13Groups) != 0 || !raw.TLS13GroupsComplete || len(raw.TLS12Curves) != 0 || !raw.TLS12CurvesComplete {
		t.Errorf("groups %+v / curves %+v: Go supports none beyond those crypto/tls probes", raw.TLS13Groups, raw.TLS12Curves)
	}
	if raw.BudgetExhausted || raw.Probes < 6 || raw.Probes > 15 {
		t.Errorf("%d raw probes (budget exhausted %v)", raw.Probes, raw.BudgetExhausted)
	}
	if r.Connections <= raw.Probes {
		t.Errorf("connections %d, raw probes %d", r.Connections, raw.Probes)
	}
	want := []string{NotEvalServerPreference, NotEvalCredentialSuites, NotEvalImplementationBug}
	if !slices.Equal(r.NotEvaluated, want) {
		t.Errorf("not evaluated = %v, want %v", r.NotEvaluated, want)
	}
	if n := res.calls.Load(); n != 1 {
		t.Errorf("%d DNS lookups, want 1 (raw probes must reuse the pinned address)", n)
	}
}

func TestRawProbeBudget(t *testing.T) {
	pki := newTestPKI(t)
	cert := pki.leaf(t, leafOpts{})
	port := startTLS(t, &tls.Config{Certificates: []tls.Certificate{cert}})
	r := scan(t, newTestScanner(pki, Options{Intensity: IntensityFull, RawProbeBudget: 2}), port)
	if !r.Raw.BudgetExhausted || r.Raw.Probes != 2 {
		t.Fatalf("probes %d, exhausted %v", r.Raw.Probes, r.Raw.BudgetExhausted)
	}
	// What could not run stays "not evaluated", never "not supported".
	incomplete := !r.Raw.SuitesComplete || !r.Raw.TLS13SuitesComplete || !r.Raw.TLS13GroupsComplete || !r.Raw.TLS12CurvesComplete
	if !incomplete {
		t.Errorf("budget of 2 yet every enumeration complete: %+v", r.Raw)
	}
	for _, code := range []string{NotEvalServerPreference, NotEvalImplementationBug} {
		if !slices.Contains(r.NotEvaluated, code) {
			t.Errorf("%s missing from %v", code, r.NotEvaluated)
		}
	}
	if len(r.NotEvaluated) <= 3 {
		t.Errorf("not evaluated = %v: incomplete probes must stay listed", r.NotEvaluated)
	}
}

func TestBasicIntensityHasNoRawProbes(t *testing.T) {
	pki := newTestPKI(t)
	cert := pki.leaf(t, leafOpts{})
	port := startTLS(t, &tls.Config{Certificates: []tls.Certificate{cert}})
	r := scan(t, newTestScanner(pki, Options{}), port)
	if r.Intensity != IntensityBasic || r.Raw != nil || !slices.Equal(r.NotEvaluated, notEvaluated) {
		t.Errorf("intensity %q, raw %v, not evaluated %v", r.Intensity, r.Raw, r.NotEvaluated)
	}
}

func TestParseIntensity(t *testing.T) {
	for _, s := range []string{"basic", "full"} {
		if got, err := ParseIntensity(s); err != nil || string(got) != s {
			t.Errorf("ParseIntensity(%q) = %q, %v", s, got, err)
		}
	}
	for _, s := range []string{"", "FULL", "lab"} {
		if _, err := ParseIntensity(s); err == nil {
			t.Errorf("ParseIntensity(%q) accepted", s)
		}
	}
}

func TestClassifyRaw(t *testing.T) {
	if o := classifyRaw(nil, errBudget); o.status != ProbeFailed {
		t.Errorf("budget error classified %s", o.status)
	}
}
