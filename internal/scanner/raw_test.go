package scanner

import (
	"crypto/tls"
	"errors"
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
	if r.Certificate == nil || r.Certificate.Source != "" {
		t.Errorf("certificate %+v: crypto/tls saw it, so it is not from the raw probes", r.Certificate)
	}
}

// A TLS 1.2 server that refuses every ClientHello carrying
// extended_master_secret, which crypto/tls always sends and the raw probes
// never do: no crypto/tls handshake gets to the certificate (as with the
// lab's EXPORT, DHE-only or secp224 servers), the raw probes read it in clear.
func TestCertificateFromRawProbes(t *testing.T) {
	pki := newTestPKI(t)
	cert := pki.leaf(t, leafOpts{})
	port := startTLS(t, &tls.Config{Certificates: []tls.Certificate{cert}, MaxVersion: tls.VersionTLS12,
		GetConfigForClient: func(hi *tls.ClientHelloInfo) (*tls.Config, error) {
			if slices.Contains(hi.Extensions, 23) {
				return nil, errors.New("no crypto/tls clients here")
			}
			return nil, nil
		}})

	full := scan(t, newTestScanner(pki, Options{Intensity: IntensityFull}), port)
	c := full.Certificate
	if c == nil {
		t.Fatal("no certificate: the raw probes saw it in clear")
	}
	if c.Source != CertSourceRaw || !c.Trusted || !c.HostnameMatch || c.KeyAlgorithm != "ECDSA" || c.OCSPStapled || c.SCTsHandshake != 0 || len(c.Chain) != 2 {
		t.Errorf("certificate %+v: want the leaf and its CA, trusted, from the raw probes", c)
	}
	sel := full.CertificateSelection
	if sel.WithMLDSA == nil || sel.WithMLDSA.PostQuantum || sel.WithMLDSA.SHA256 == "" {
		t.Errorf("selection %+v: without TLS 1.3, the certificate seen is also what a client offering ML-DSA gets", sel.WithMLDSA)
	}
	if slices.Contains(full.NotEvaluated, NotEvalPQCertificate) {
		t.Errorf("not evaluated %v: the certificate was seen", full.NotEvaluated)
	}
	for _, p := range full.ProbeLog.Probes {
		if p.Client == clientCryptoTLS && p.Status == Supported {
			t.Fatalf("a crypto/tls probe completed (%+v): the server must refuse them all", p)
		}
	}

	// Intensity basic has no raw probes: not seen, as before.
	basic := scan(t, newTestScanner(pki, Options{Intensity: IntensityBasic}), port)
	if basic.Certificate != nil || !slices.Contains(basic.NotEvaluated, NotEvalPQCertificate) {
		t.Errorf("basic: certificate %+v, not evaluated %v", basic.Certificate, basic.NotEvaluated)
	}
}

// Chrome 156's offer (2.8.7): what a crypto/tls server picks for it, in TLS
// 1.3 and 1.2, logged with the offer as sent.
func TestChromeOffer(t *testing.T) {
	pki := newTestPKI(t)
	cert := pki.leaf(t, leafOpts{})
	for _, tc := range []struct {
		name, version, suite, group string
		max                         uint16
		curves                      []tls.CurveID
		retry                       bool
	}{
		{"TLS 1.3", "TLS 1.3", "", "X25519MLKEM768", 0, nil, false},
		{"TLS 1.2", "TLS 1.2", "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256", "x25519", tls.VersionTLS12, nil, false},
		{"HelloRetryRequest", "TLS 1.3", "", "secp256r1", 0, []tls.CurveID{tls.CurveP256}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			port := startTLS(t, &tls.Config{Certificates: []tls.Certificate{cert}, MaxVersion: tc.max, CurvePreferences: tc.curves})
			r := scan(t, newTestScanner(pki, Options{Intensity: IntensityFull}), port)
			c := r.ChromeOffer
			if c == nil || c.Status != Supported || c.Version != tc.version || c.Group != tc.group || tc.suite != "" && c.CipherSuite != tc.suite || r.ChromeHelloRetry != tc.retry {
				t.Fatalf("chrome offer %+v (retry %v)", c, r.ChromeHelloRetry)
			}
			if c.Group != r.Default.Group || c.Version != r.Default.Version {
				t.Errorf("Chrome got %s/%s, the browser-like crypto/tls offer %s/%s: same server, same choice expected", c.Version, c.Group, r.Default.Version, r.Default.Group)
			}
			var logged []ProbeRecord
			for _, p := range r.ProbeLog.Probes {
				if p.Purpose == PurposeChrome {
					logged = append(logged, p)
				}
			}
			if len(logged) != 1 || logged[0].Client != clientRaw || logged[0].Offer == nil ||
				!slices.Equal(r.ProbeLog.Offers[*logged[0].Offer].ALPN, []string{"h2", "http/1.1"}) {
				t.Errorf("logged %+v", logged)
			}
		})
	}
	basic := scan(t, newTestScanner(pki, Options{Intensity: IntensityBasic}), startTLS(t, &tls.Config{Certificates: []tls.Certificate{cert}}))
	if basic.ChromeOffer != nil {
		t.Errorf("basic: chrome offer %+v (raw probes are intensity full only)", basic.ChromeOffer)
	}
}

func TestNoteChainKeepsTheHighestVersion(t *testing.T) {
	a, b, c := [][]byte{{1}}, [][]byte{{2}}, [][]byte{{3}}
	for _, order := range [][]int{{0, 1, 2}, {2, 1, 0}, {1, 2, 0}} {
		rs := &rawScan{}
		seen := []struct {
			chain [][]byte
			v     uint16
		}{{a, 0x0002}, {b, 0x0303}, {c, 0x0300}}
		for _, i := range order {
			rs.noteChain(seen[i].chain, seen[i].v)
		}
		rs.noteChain(nil, 0x0304) // a probe without a certificate changes nothing
		if !slices.Equal(rs.chain[0], b[0]) || rs.chainVersion != 0x0303 {
			t.Errorf("order %v: kept %v (0x%04x), want the TLS 1.2 chain", order, rs.chain, rs.chainVersion)
		}
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
