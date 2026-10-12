package scanner

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"
)

func scan(t *testing.T, s *Scanner, port int) *Result {
	t.Helper()
	r, err := s.Scan(context.Background(), testHost, port)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if _, err := json.Marshal(r); err != nil {
		t.Fatalf("result does not marshal: %v", err)
	}
	return r
}

func wantGroups(t *testing.T, r *Result, want map[string]Status) {
	t.Helper()
	for name, st := range want {
		if got := groupStatus(r, name); got != st {
			t.Errorf("group %s = %q, want %q", name, got, st)
		}
	}
}

func wantVersions(t *testing.T, r *Result, want map[string]Status) {
	t.Helper()
	for v, st := range want {
		if got := versionStatus(r, v); got != st {
			t.Errorf("version %s = %q, want %q", v, got, st)
		}
	}
}

func TestKeyExchange(t *testing.T) {
	pki := newTestPKI(t)
	cert := pki.leaf(t, leafOpts{})
	tests := []struct {
		name         string
		server       []tls.CurveID
		classicFirst bool // see classicWhenOffered
		wantSelected string
		wantHybrid   bool
		wantGroups   map[string]Status
	}{
		{
			name:         "ML-KEM preferred",
			server:       []tls.CurveID{tls.X25519MLKEM768, tls.X25519, tls.CurveP256},
			wantSelected: "X25519MLKEM768",
			wantHybrid:   true,
			wantGroups: map[string]Status{
				"X25519MLKEM768": Supported, "SecP256r1MLKEM768": NotSupported, "SecP384r1MLKEM1024": NotSupported,
				"x25519": Supported, "secp256r1": Supported, "secp384r1": NotSupported,
			},
		},
		{
			name:         "ML-KEM supported but not preferred",
			server:       []tls.CurveID{tls.X25519MLKEM768, tls.X25519},
			classicFirst: true,
			wantSelected: "x25519",
			wantGroups:   map[string]Status{"X25519MLKEM768": Supported, "x25519": Supported},
		},
		{
			name:         "no ML-KEM",
			server:       []tls.CurveID{tls.X25519, tls.CurveP256},
			wantSelected: "x25519",
			wantGroups: map[string]Status{
				"X25519MLKEM768": NotSupported, "SecP256r1MLKEM768": NotSupported, "SecP384r1MLKEM1024": NotSupported,
			},
		},
		{
			name:         "only the P-256 hybrid",
			server:       []tls.CurveID{tls.SecP256r1MLKEM768, tls.CurveP256},
			wantSelected: "secp256r1", // not in the browser offer
			wantGroups: map[string]Status{
				"SecP256r1MLKEM768": Supported, "X25519MLKEM768": NotSupported, "x25519": NotSupported,
			},
		},
		{
			name:         "all three hybrids",
			server:       []tls.CurveID{tls.X25519MLKEM768, tls.SecP256r1MLKEM768, tls.SecP384r1MLKEM1024, tls.X25519},
			wantSelected: "X25519MLKEM768",
			wantHybrid:   true,
			wantGroups: map[string]Status{
				"X25519MLKEM768": Supported, "SecP256r1MLKEM768": Supported, "SecP384r1MLKEM1024": Supported,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &tls.Config{Certificates: []tls.Certificate{cert}, CurvePreferences: tt.server}
			if tt.classicFirst {
				cfg.GetConfigForClient = classicWhenOffered(cfg)
			}
			port := startTLS(t, cfg)
			r := scan(t, newTestScanner(pki, Options{}), port)
			if r.KeyExchange.Selected != tt.wantSelected || r.KeyExchange.SelectedHybrid != tt.wantHybrid {
				t.Errorf("selected = %q (hybrid %v), want %q (hybrid %v)",
					r.KeyExchange.Selected, r.KeyExchange.SelectedHybrid, tt.wantSelected, tt.wantHybrid)
			}
			if r.Default.Status != Supported || r.Default.Group != tt.wantSelected {
				t.Errorf("default handshake = %+v", r.Default)
			}
			wantGroups(t, r, tt.wantGroups)
		})
	}
}

// classicWhenOffered simulates a server that supports X25519MLKEM768 but
// prefers X25519 whenever the client offers it. A Go server cannot be set up
// like that with CurvePreferences alone: it ignores their order and always
// prefers the hybrid.
func classicWhenOffered(base *tls.Config) func(*tls.ClientHelloInfo) (*tls.Config, error) {
	return func(chi *tls.ClientHelloInfo) (*tls.Config, error) {
		cfg := base.Clone()
		cfg.GetConfigForClient = nil
		if slices.Contains(chi.SupportedCurves, tls.X25519) {
			cfg.CurvePreferences = []tls.CurveID{tls.X25519}
		}
		return cfg, nil
	}
}

func TestProtocolsAndSuites(t *testing.T) {
	pki := newTestPKI(t)
	ecdsaCert := pki.leaf(t, leafOpts{})
	rsaCert := pki.leaf(t, leafOpts{key: "rsa2048"})

	t.Run("TLS 1.2 only", func(t *testing.T) {
		port := startTLS(t, &tls.Config{Certificates: []tls.Certificate{ecdsaCert}, MaxVersion: tls.VersionTLS12})
		r := scan(t, newTestScanner(pki, Options{}), port)
		wantVersions(t, r, map[string]Status{"TLS 1.0": NotSupported, "TLS 1.1": NotSupported, "TLS 1.2": Supported, "TLS 1.3": NotSupported})
		wantGroups(t, r, map[string]Status{"X25519MLKEM768": NotSupported, "x25519": Supported})
		if r.KeyExchange.Selected != "x25519" || r.Default.Version != "TLS 1.2" {
			t.Errorf("default = %+v", r.Default)
		}
		if r.TLS13Suite != "" {
			t.Errorf("TLS13Suite = %q, want empty", r.TLS13Suite)
		}
		if len(r.TLS12Suites) == 0 || !r.TLS12SuitesComplete {
			t.Fatalf("TLS 1.2 suites = %v (complete %v)", r.TLS12Suites, r.TLS12SuitesComplete)
		}
		for _, s := range r.TLS12Suites {
			if !strings.Contains(s.Name, "ECDSA") || !s.ForwardSecrecy {
				t.Errorf("unexpected suite for an ECDSA server: %+v", s)
			}
		}
	})

	t.Run("TLS 1.3 only", func(t *testing.T) {
		port := startTLS(t, &tls.Config{Certificates: []tls.Certificate{ecdsaCert}, MinVersion: tls.VersionTLS13})
		r := scan(t, newTestScanner(pki, Options{}), port)
		wantVersions(t, r, map[string]Status{"TLS 1.2": NotSupported, "TLS 1.3": Supported})
		if len(r.TLS12Suites) != 0 || !r.TLS12SuitesComplete {
			t.Errorf("TLS 1.2 suites = %v (complete %v), want none", r.TLS12Suites, r.TLS12SuitesComplete)
		}
		if !strings.HasPrefix(r.TLS13Suite, "TLS_") {
			t.Errorf("TLS13Suite = %q", r.TLS13Suite)
		}
	})

	t.Run("legacy: TLS 1.0, RC4, 3DES, CBC, RSA key exchange", func(t *testing.T) {
		port := startTLS(t, &tls.Config{
			Certificates: []tls.Certificate{rsaCert},
			MinVersion:   tls.VersionTLS10,
			MaxVersion:   tls.VersionTLS12,
			CipherSuites: []uint16{
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA,
				tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
				tls.TLS_RSA_WITH_AES_128_CBC_SHA,
			},
		})
		r := scan(t, newTestScanner(pki, Options{}), port)
		wantVersions(t, r, map[string]Status{"TLS 1.0": Supported, "TLS 1.1": Supported, "TLS 1.2": Supported, "TLS 1.3": NotSupported})
		got := suiteNames(r)
		if len(got) != 4 {
			t.Errorf("suites = %v, want the 4 configured", r.TLS12Suites)
		}
		want := map[string]CipherSuite{
			"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256": {ForwardSecrecy: true, AEAD: true},
			"TLS_ECDHE_RSA_WITH_RC4_128_SHA":        {ForwardSecrecy: true, Insecure: true},
			"TLS_RSA_WITH_3DES_EDE_CBC_SHA":         {Insecure: true},
			"TLS_RSA_WITH_AES_128_CBC_SHA":          {Insecure: true},
		}
		for name, w := range want {
			s, ok := got[name]
			if !ok {
				t.Errorf("missing suite %s", name)
				continue
			}
			if s.ForwardSecrecy != w.ForwardSecrecy || s.AEAD != w.AEAD || s.Insecure != w.Insecure {
				t.Errorf("%s = %+v, want fs=%v aead=%v insecure=%v", name, s, w.ForwardSecrecy, w.AEAD, w.Insecure)
			}
		}
	})

	t.Run("RSA key exchange only (no forward secrecy)", func(t *testing.T) {
		port := startTLS(t, &tls.Config{
			Certificates: []tls.Certificate{rsaCert},
			MaxVersion:   tls.VersionTLS12,
			CipherSuites: []uint16{tls.TLS_RSA_WITH_AES_128_GCM_SHA256},
		})
		r := scan(t, newTestScanner(pki, Options{}), port)
		// Browsers (and Go's defaults) no longer offer RSA key exchange.
		if r.Default.Status != NotSupported {
			t.Errorf("default handshake = %+v, want not_supported", r.Default)
		}
		if len(r.TLS12Suites) != 1 || r.TLS12Suites[0].ForwardSecrecy {
			t.Errorf("suites = %+v, want only RSA_WITH_AES_128_GCM without FS", r.TLS12Suites)
		}
		wantGroups(t, r, map[string]Status{"x25519": NotSupported, "X25519MLKEM768": NotSupported})
		if r.Certificate == nil || r.Certificate.KeyAlgorithm != "RSA" {
			t.Errorf("certificate not captured from the other probes: %+v", r.Certificate)
		}
	})
}

func TestCertificate(t *testing.T) {
	pki := newTestPKI(t)
	other := newTestPKI(t)
	tests := []struct {
		name  string
		pki   *testPKI // issuer
		leaf  leafOpts
		check func(t *testing.T, c *Certificate)
	}{
		{"valid ECDSA", pki, leafOpts{}, func(t *testing.T, c *Certificate) {
			if !c.Trusted || !c.HostnameMatch || c.Expired || c.SelfSigned {
				t.Errorf("want trusted, matching, not expired: %+v", c)
			}
			if c.KeyAlgorithm != "ECDSA" || c.KeySize != 256 || c.Curve != "P-256" || c.SignatureAlgorithm != "ECDSA-SHA256" {
				t.Errorf("key/signature = %s %d %s %s", c.KeyAlgorithm, c.KeySize, c.Curve, c.SignatureAlgorithm)
			}
			if c.ValidityDays != 90 || c.DaysToExpiry != 80 || c.Issuer != "QuantumReady Test CA" || len(c.Chain) != 2 {
				t.Errorf("validity %d, expiry %d, issuer %q, chain %d", c.ValidityDays, c.DaysToExpiry, c.Issuer, len(c.Chain))
			}
		}},
		{"expired", pki, leafOpts{notBefore: testNow.AddDate(0, -3, 0), notAfter: testNow.AddDate(0, 0, -1)}, func(t *testing.T, c *Certificate) {
			if !c.Expired || c.Trusted || c.DaysToExpiry >= 0 {
				t.Errorf("want expired and untrusted: expired=%v trusted=%v days=%d", c.Expired, c.Trusted, c.DaysToExpiry)
			}
		}},
		{"RSA-1024", pki, leafOpts{key: "rsa1024"}, func(t *testing.T, c *Certificate) {
			if c.KeyAlgorithm != "RSA" || c.KeySize != 1024 {
				t.Errorf("key = %s %d, want RSA 1024", c.KeyAlgorithm, c.KeySize)
			}
		}},
		{"name mismatch", pki, leafOpts{dns: []string{"other.test"}}, func(t *testing.T, c *Certificate) {
			if c.HostnameMatch || !c.Trusted {
				t.Errorf("want trusted chain but no name match: match=%v trusted=%v", c.HostnameMatch, c.Trusted)
			}
		}},
		{"unknown CA", other, leafOpts{}, func(t *testing.T, c *Certificate) {
			if c.Trusted || !strings.Contains(c.TrustError, "unknown authority") {
				t.Errorf("want untrusted (unknown authority): trusted=%v err=%q", c.Trusted, c.TrustError)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port := startTLS(t, &tls.Config{Certificates: []tls.Certificate{tt.pki.leaf(t, tt.leaf)}})
			r := scan(t, newTestScanner(pki, Options{}), port)
			if r.Certificate == nil {
				t.Fatal("no certificate")
			}
			tt.check(t, r.Certificate)
		})
	}

	t.Run("SCTs embedded and in the handshake", func(t *testing.T) {
		cert := pki.leaf(t, leafOpts{scts: 3})
		cert.SignedCertificateTimestamps = [][]byte{[]byte("sct-one"), []byte("sct-two")}
		port := startTLS(t, &tls.Config{Certificates: []tls.Certificate{cert}})
		c := scan(t, newTestScanner(pki, Options{}), port).Certificate
		if c.SCTsEmbedded != 3 || c.SCTsHandshake != 2 {
			t.Errorf("SCTs embedded=%d handshake=%d, want 3 and 2", c.SCTsEmbedded, c.SCTsHandshake)
		}
	})

	t.Run("no SCTs", func(t *testing.T) {
		port := startTLS(t, &tls.Config{Certificates: []tls.Certificate{pki.leaf(t, leafOpts{})}})
		c := scan(t, newTestScanner(pki, Options{}), port).Certificate
		if c.SCTsEmbedded != 0 || c.SCTsHandshake != 0 {
			t.Errorf("SCTs embedded=%d handshake=%d, want 0 and 0", c.SCTsEmbedded, c.SCTsHandshake)
		}
	})
}

func TestUnreachable(t *testing.T) {
	t.Run("closed port", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := ln.Addr().(*net.TCPAddr).Port
		ln.Close()
		_, err = newTestScanner(nil, Options{}).Scan(context.Background(), testHost, port)
		if !errors.Is(err, ErrUnreachable) {
			t.Fatalf("error = %v, want ErrUnreachable", err)
		}
	})

	// A server that accepts TCP but never answers: bounded by the handshake timeout.
	t.Run("silent server", func(t *testing.T) {
		port := silentServer(t)
		start := time.Now()
		_, err := newTestScanner(nil, Options{HandshakeTimeout: 300 * time.Millisecond}).Scan(context.Background(), testHost, port)
		if !errors.Is(err, ErrUnreachable) {
			t.Fatalf("error = %v, want ErrUnreachable", err)
		}
		if d := time.Since(start); d > 2*time.Second {
			t.Errorf("took %v, want about the handshake timeout", d)
		}
	})

	t.Run("scan timeout", func(t *testing.T) {
		port := silentServer(t)
		start := time.Now()
		_, err := newTestScanner(nil, Options{HandshakeTimeout: 10 * time.Second, ScanTimeout: 300 * time.Millisecond}).Scan(context.Background(), testHost, port)
		if err == nil {
			t.Fatal("want an error")
		}
		if d := time.Since(start); d > 2*time.Second {
			t.Errorf("took %v, want about the scan timeout", d)
		}
	})

	t.Run("canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := newTestScanner(nil, Options{}).Scan(ctx, testHost, 443)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	})

	t.Run("name does not resolve", func(t *testing.T) {
		_, err := newTestScanner(nil, Options{}).Scan(context.Background(), "nowhere.test", 443)
		if !errors.Is(err, ErrResolve) {
			t.Fatalf("error = %v, want ErrResolve", err)
		}
	})

	t.Run("invalid port", func(t *testing.T) {
		_, err := newTestScanner(nil, Options{}).Scan(context.Background(), testHost, 70000)
		if !errors.Is(err, ErrInvalidTarget) {
			t.Fatalf("error = %v, want ErrInvalidTarget", err)
		}
	})
}

func silentServer(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { c.Close() })
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestConcurrencyLimits(t *testing.T) {
	pki := newTestPKI(t)
	port := startTLS(t, &tls.Config{Certificates: []tls.Certificate{pki.leaf(t, leafOpts{})}})
	for _, tt := range []struct {
		name string
		opts Options
		max  int32
	}{
		{"per target", Options{PerTargetConcurrency: 3}, 3},
		{"global", Options{PerTargetConcurrency: 8, GlobalConcurrency: 2}, 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScanner(pki, tt.opts)
			scan(t, s, port)
			if got := s.maxActive.Load(); got > tt.max || got < 2 {
				t.Errorf("max concurrent handshakes = %d, want 2..%d", got, tt.max)
			}
		})
	}
}

// DNS rebinding: the name is resolved once and every connection goes to that
// validated address, even if a later answer would point somewhere internal.
func TestResolvesOnceAndPinsTheAddress(t *testing.T) {
	pki := newTestPKI(t)
	port := startTLS(t, &tls.Config{Certificates: []tls.Certificate{pki.leaf(t, leafOpts{})}})
	res := resolverFor(testHost, "127.0.0.1")
	res.answers[testHost] = append(res.answers[testHost], []netip.Addr{netip.MustParseAddr("10.0.0.1")})
	r := scan(t, newTestScanner(pki, Options{Resolver: res}), port)
	if n := res.calls.Load(); n != 1 {
		t.Errorf("resolver called %d times, want 1", n)
	}
	if r.IP != "127.0.0.1" || r.Default.Status != Supported {
		t.Errorf("ip = %s, default = %+v", r.IP, r.Default)
	}
}
