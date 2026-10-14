//go:build tlslab

// Integration tests against the TLS test lab (test/tlslab). They only run
// inside the lab network: make tlslab-test. Each lab server is scanned with
// intensity full and basic, and the full result is compared with the
// testssl.sh reference (test/tlslab/reference). Results, measurements and the
// comparison table go to $QR_TLSLAB_OUT.

package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fizsar/quantumready-saas/internal/scanner/rawtls"
)

var labServers = []string{"ssl2", "ssl3", "export", "dhe1024", "tls10", "secp224", "tls12", "tls13", "kyber"}

// knownDifferences are expected mismatches with testssl.sh, explained in
// docs/validation.md. Key: server/aspect.
var knownDifferences = map[string]string{
	"ssl3/grupos": "curvas ECDHE que solo existen en SSLv3: no se sondean (un cliente SSLv3 no envía supported_groups); se marca como no evaluado y SSLv3 ya limita la nota a C",
}

// labNetwork is the address policy of these tests: the lab subnet only.
func labNetwork(a netip.Addr) error {
	if netip.MustParsePrefix("172.31.213.0/24").Contains(a.Unmap()) {
		return nil
	}
	return fmt.Errorf("%w: %s is outside the lab network", ErrBlockedAddress, a)
}

type labRun struct {
	Result   *Result `json:"result"`
	Error    string  `json:"error,omitempty"`
	Duration int64   `json:"duration_ms"`
}

func labScan(host string, in Intensity, rtt time.Duration) labRun {
	s := New(Options{Intensity: in, AddrPolicy: labNetwork})
	s.simulatedRTT = rtt
	start := time.Now()
	r, err := s.Scan(context.Background(), host, 443)
	run := labRun{Result: r, Duration: time.Since(start).Milliseconds()}
	if err != nil {
		run.Error = err.Error()
	}
	return run
}

// view is what both tools say about a server, in comparable form.
type view struct {
	Protocols   []string `json:"protocols"`
	Suites      []string `json:"suites"` // TLS 1.0–1.2, union over versions
	SSLv3Suites []string `json:"sslv3_suites"`
	SSLv2       []string `json:"sslv2_ciphers"`
	TLS13Suites []string `json:"tls13_suites"`
	DHBits      []string `json:"dh_bits"`
	Groups      []string `json:"groups"`
}

func sortedSet(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func ourView(r *Result) view {
	protos, suites, ssl3, tls13, dh, groups := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	var v view
	for _, p := range r.Versions {
		if p.Status == Supported {
			protos[p.Version] = true
		}
	}
	for _, list := range [][]CipherSuite{r.TLS12Suites, r.TLS11Suites, r.TLS10Suites} {
		for _, s := range list {
			suites[s.Name] = true
		}
	}
	for _, g := range r.KeyExchange.Groups {
		if g.Status == Supported {
			groups[g.Name] = true
		}
	}
	if raw := r.Raw; raw != nil {
		if raw.SSLv2.Status == Supported {
			protos["SSLv2"] = true
			v.SSLv2 = slices.Sorted(slices.Values(raw.SSLv2.Ciphers))
		}
		if raw.SSLv3.Status == Supported {
			protos["SSLv3"] = true
		}
		for _, p := range raw.Versions {
			if p.Status == Supported {
				protos[p.Version] = true
			}
		}
		for _, s := range raw.Suites {
			if s.Version == "SSLv3" {
				ssl3[s.Name] = true
				continue
			}
			protos[s.Version] = true
			suites[s.Name] = true
		}
		for _, s := range raw.TLS13Suites {
			protos["TLS 1.3"] = true
			tls13[s.Name] = true
		}
		for _, d := range raw.DH {
			dh[strconv.Itoa(d.PrimeBits)] = true
		}
		for _, g := range append(append([]RawGroup{}, raw.TLS13Groups...), raw.TLS12Curves...) {
			groups[g.Name] = true
		}
	}
	v.Protocols, v.Suites, v.SSLv3Suites, v.TLS13Suites, v.DHBits, v.Groups =
		sortedSet(protos), sortedSet(suites), sortedSet(ssl3), sortedSet(tls13), sortedSet(dh), sortedSet(groups)
	return v
}

var (
	testsslCipherID = regexp.MustCompile(`^cipher-(ssl2|ssl3|tls1|tls1_1|tls1_2|tls1_3)_x[0-9a-f]+$`)
	testsslDHBits   = regexp.MustCompile(`\bDH (\d+)\b`)
	testsslProtos   = map[string]string{"SSLv2": "SSLv2", "SSLv3": "SSLv3", "TLS1": "TLS 1.0", "TLS1_1": "TLS 1.1", "TLS1_2": "TLS 1.2", "TLS1_3": "TLS 1.3"}
	// OpenSSL group names → IANA names.
	opensslGroups = map[string]string{"prime256v1": "secp256r1", "X25519": "x25519", "X448": "x448", "secp384r1": "secp384r1", "secp521r1": "secp521r1"}
)

func testsslView(t *testing.T, server string) view {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "test", "tlslab", "reference", server+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		ScanResult []map[string]json.RawMessage `json:"scanResult"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	type entry struct{ ID, Finding string }
	var entries []entry
	for _, sr := range doc.ScanResult {
		for _, raw := range sr {
			var list []entry
			if json.Unmarshal(raw, &list) == nil {
				entries = append(entries, list...)
			}
		}
	}
	protos, suites, ssl3, ssl2, tls13, dh, groups := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, e := range entries {
		// "offered …", or "vulnerable with 4 ciphers" for SSLv2; not "TLSv1.1 is not
		// offered, and downgraded…" nor "connection failed rather than downgrading…".
		if name, ok := testsslProtos[e.ID]; ok && (strings.HasPrefix(e.Finding, "offered") || strings.HasPrefix(e.Finding, "vulnerable")) {
			protos[name] = true
		}
		if m := testsslCipherID.FindStringSubmatch(e.ID); m != nil {
			cols := strings.Fields(e.Finding)
			name := cols[len(cols)-1]
			switch m[1] {
			case "ssl2":
				ssl2[name] = true
			case "ssl3":
				ssl3[name] = true
			case "tls1_3":
				tls13[name] = true
			default:
				suites[name] = true
			}
			if d := testsslDHBits.FindStringSubmatch(e.Finding); d != nil {
				dh[d[1]] = true
			}
		}
		if e.ID == "FS_ECDHE_curves" || e.ID == "FS_KEMs" || e.ID == "DH_groups" {
			for _, g := range strings.Fields(e.Finding) {
				if n, ok := opensslGroups[g]; ok {
					g = n
				}
				if _, ok := rawtls.GroupByID(groupID(g)); ok {
					groups[g] = true
				}
			}
		}
	}
	// testssl.sh lists the SSLv3 suites in full; we list those not found in a
	// TLS version. Compare like with like.
	for s := range ssl3 {
		if suites[s] {
			delete(ssl3, s)
		}
	}
	return view{Protocols: sortedSet(protos), Suites: sortedSet(suites), SSLv3Suites: sortedSet(ssl3), SSLv2: sortedSet(ssl2),
		TLS13Suites: sortedSet(tls13), DHBits: sortedSet(dh), Groups: sortedSet(groups)}
}

func groupID(name string) uint16 {
	for _, g := range rawtls.Groups() {
		if g.Name == name {
			return g.ID
		}
	}
	return 0xffff
}

// diff returns what only a has and what only b has.
func diff(a, b []string) (onlyA, onlyB []string) {
	for _, x := range a {
		if !slices.Contains(b, x) {
			onlyA = append(onlyA, x)
		}
	}
	for _, x := range b {
		if !slices.Contains(a, x) {
			onlyB = append(onlyB, x)
		}
	}
	return onlyA, onlyB
}

func TestLab(t *testing.T) {
	out := os.Getenv("QR_TLSLAB_OUT")
	var cmp, meas strings.Builder
	cmp.WriteString("| Servidor | Aspecto | testssl.sh | QuantumReady | |\n|---|---|---|---|---|\n")
	meas.WriteString("| Servidor | full: conexiones | full: sondas en crudo | full: duración (local) | full: duración (RTT 100 ms) | basic: conexiones | basic: duración (local) | basic: duración (RTT 100 ms) |\n|---|---|---|---|---|---|---|---|\n")
	for _, server := range labServers {
		t.Run(server, func(t *testing.T) {
			full := labScan(server, IntensityFull, 0)
			basic := labScan(server, IntensityBasic, 0)
			slowFull := labScan(server, IntensityFull, 100*time.Millisecond)
			slowBasic := labScan(server, IntensityBasic, 100*time.Millisecond)
			if out != "" {
				b, _ := json.MarshalIndent(map[string]labRun{"full": full, "basic": basic, "full_rtt100ms": slowFull, "basic_rtt100ms": slowBasic}, "", "  ")
				_ = os.WriteFile(filepath.Join(out, server+".json"), b, 0o644)
			}
			if full.Error != "" {
				t.Fatalf("full scan: %s", full.Error)
			}
			raw := full.Result.Raw
			conns := func(r labRun) string {
				if r.Error != "" {
					return "—"
				}
				return strconv.Itoa(r.Result.Connections)
			}
			dur := func(r labRun) string {
				if r.Error != "" {
					return "error: " + r.Error
				}
				return fmt.Sprintf("%d ms", r.Duration)
			}
			fmt.Fprintf(&meas, "| `%s` | %s | %d%s | %s | %s | %s | %s | %s |\n", server, conns(full), raw.Probes,
				map[bool]string{true: " (presupuesto agotado)", false: ""}[raw.BudgetExhausted],
				dur(full), dur(slowFull), conns(basic), dur(basic), dur(slowBasic))

			ours, ref := ourView(full.Result), testsslView(t, server)
			for _, a := range []struct {
				name     string
				ref, our []string
			}{
				{"protocolos", ref.Protocols, ours.Protocols},
				{"cifrados SSLv2", ref.SSLv2, ours.SSLv2},
				{"suites SSLv3 (no vistas en TLS)", ref.SSLv3Suites, ours.SSLv3Suites},
				{"suites TLS 1.0–1.2", ref.Suites, ours.Suites},
				{"suites TLS 1.3", ref.TLS13Suites, ours.TLS13Suites},
				{"DH (bits)", ref.DHBits, ours.DHBits},
				{"grupos", ref.Groups, ours.Groups},
			} {
				onlyRef, onlyOurs := diff(a.ref, a.our)
				verdict := "="
				switch {
				case len(onlyRef) > 0 && len(onlyOurs) > 0:
					verdict = fmt.Sprintf("distinto: solo testssl %v; solo nosotros %v", onlyRef, onlyOurs)
				case len(onlyRef) > 0:
					verdict = fmt.Sprintf("**vemos menos**: %v", onlyRef)
				case len(onlyOurs) > 0:
					verdict = fmt.Sprintf("**vemos más**: %v", onlyOurs)
				}
				if len(a.ref)+len(a.our) > 0 {
					fmt.Fprintf(&cmp, "| `%s` | %s | %d | %d | %s |\n", server, a.name, len(a.ref), len(a.our), verdict)
				}
				if len(onlyRef) > 0 {
					key := server + "/" + a.name
					if why, ok := knownDifferences[key]; ok {
						t.Logf("known difference %s: %s", key, why)
					} else {
						t.Errorf("%s: testssl.sh sees %v and we do not", a.name, onlyRef)
					}
				}
				if len(onlyOurs) > 0 {
					t.Logf("%s: we see more than testssl.sh: %v", a.name, onlyOurs)
				}
			}
		})
	}
	t.Log("\n" + meas.String() + "\n" + cmp.String())
	if out != "" {
		_ = os.WriteFile(filepath.Join(out, "COMPARISON.md"), []byte("## Mediciones (intensidad full y basic)\n\n"+meas.String()+"\n## Comparación con testssl.sh\n\n"+cmp.String()), 0o644)
	}
}
