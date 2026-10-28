package scoring

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/fizsar/quantumready-saas/internal/scanner"
)

// labResults loads the real scans of the TLS lab servers (intensity full),
// copied from test/tlslab/results in milestone 2.7.4 (modern in showcase S4b). The lab certificates
// are self-signed (or not captured at all where crypto/tls cannot connect),
// and their F cap would hide every protocol and suite rule, so each result
// gets the valid certificate of ideal(): what is judged is the TLS setup.
func labResults(t *testing.T) map[string]*scanner.Result {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("testdata", "lab", "*.json"))
	if err != nil || len(files) != 10 {
		t.Fatalf("lab results: %v, %d files", err, len(files))
	}
	out := map[string]*scanner.Result{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var r scanner.Result
		if err := json.Unmarshal(b, &r); err != nil {
			t.Fatal(err)
		}
		r.Certificate = ideal().Certificate
		out[strings.TrimSuffix(filepath.Base(f), ".json")] = &r
	}
	return out
}

// rawExtras are synthetic intensity-full results for the rules no lab
// server triggers.
func rawExtras() map[string]*scanner.Result {
	tls13 := func(r *scanner.Result, suites ...scanner.RawSuite) {
		r.Intensity = scanner.IntensityFull
		r.Raw = &scanner.RawResult{SuitesComplete: true, TLS13SuitesComplete: true, TLS13GroupsComplete: true, TLS12CurvesComplete: true,
			SSLv2: scanner.RawProtocol{Status: scanner.NotSupported}, SSLv3: scanner.RawProtocol{Status: scanner.NotSupported}, TLS13Suites: suites}
	}
	aes := scanner.RawSuite{Version: "TLS 1.3", Name: "TLS_AES_128_GCM_SHA256", ID: 0x1301, AEAD: true, Mode: "gcm", Bits: 128, ForwardSecrecy: true}
	integrity := apply(func(r *scanner.Result) {
		tls13(r, aes, scanner.RawSuite{Version: "TLS 1.3", Name: "TLS_SHA256_SHA256", ID: 0xC0B4, Mode: "none", ForwardSecrecy: true})
	})
	modern := apply(func(r *scanner.Result) {
		tls13(r, aes, scanner.RawSuite{Version: "TLS 1.3", Name: "TLS_AEGIS_256_SHA512", ID: 0x1306, AEAD: true, Mode: "aead", Bits: 256, ForwardSecrecy: true})
	})
	extra := apply(func(r *scanner.Result) {
		tls13(r, aes)
		r.Raw.TLS13Groups = []scanner.RawGroup{{Name: "X25519Kyber768Draft00", ID: 0x6399, Kind: "kyber_draft"}}
		r.Raw.DH = []scanner.DHParams{{Version: "TLS 1.2", Suite: "TLS_DHE_RSA_WITH_AES_128_GCM_SHA256", PrimeBits: 3072}}
	})
	chacha := scanner.RawSuite{Version: "TLS 1.3", Name: "TLS_CHACHA20_POLY1305_SHA256", ID: 0x1303, AEAD: true, Mode: "aead", Bits: 256, ForwardSecrecy: true}
	order := func(o, a, b string) *scanner.Result { // milestone 2.8.7
		return apply(func(r *scanner.Result) {
			tls13(r, aes, chacha)
			r.Raw.TLS13SuiteOrder = &scanner.SuiteOrder{Order: o, Offer: []string{aes.Name, chacha.Name}, Picks: []string{a, b}}
		})
	}
	return map[string]*scanner.Result{"integrity_only": integrity, "modern_aead": modern, "kyber_and_mlkem": extra,
		"order_server":  order(scanner.SuiteOrderServer, aes.Name, aes.Name),
		"order_client":  order(scanner.SuiteOrderClient, aes.Name, chacha.Name),
		"order_unknown": order("", aes.Name, "")}
}

// The TLS 1.3 suite order (2.8.7) is evaluated and informational: either
// answer leaves score, grade and readiness as they were; an undecided one or
// a scan without it (stored before) adds nothing.
func TestTLS13SuiteOrder(t *testing.T) {
	x := rawExtras()
	base := Evaluate(apply(func(r *scanner.Result) {
		r.Intensity = scanner.IntensityFull
		r.Raw = &scanner.RawResult{SuitesComplete: true, TLS13SuitesComplete: true, TLS13GroupsComplete: true, TLS12CurvesComplete: true,
			SSLv2: scanner.RawProtocol{Status: scanner.NotSupported}, SSLv3: scanner.RawProtocol{Status: scanner.NotSupported},
			TLS13Suites: x["order_server"].Raw.TLS13Suites}
	}))
	for name, want := range map[string]string{"order_server": "TLS13_SUITE_ORDER_SERVER", "order_client": "TLS13_SUITE_ORDER_CLIENT", "order_unknown": ""} {
		res := Evaluate(x[name])
		var got []Finding
		for _, f := range res.Findings {
			if strings.HasPrefix(f.Code, "TLS13_SUITE_ORDER") {
				got = append(got, f)
			}
		}
		switch {
		case want == "" && len(got) != 0, want != "" && (len(got) != 1 || got[0].Code != want || got[0].Severity != Info || got[0].Axis != AxisCurrent):
			t.Errorf("%s: %+v, want %q (informational)", name, got, want)
		}
		if res.Score != base.Score || res.Grade != base.Grade || res.Readiness.Score != base.Readiness.Score || len(res.Findings) != len(base.Findings)+len(got) {
			t.Errorf("%s changed the judgement: %d %s %d", name, res.Score, res.Grade, res.Readiness.Score)
		}
	}
	if got := Evaluate(x["order_client"]).Findings; !slices.ContainsFunc(got, func(f Finding) bool {
		return f.Code == "TLS13_SUITE_ORDER_CLIENT" && f.Params["first"] == "TLS_AES_128_GCM_SHA256" && f.Params["second"] == "TLS_CHACHA20_POLY1305_SHA256"
	}) {
		t.Errorf("client order params: %+v", got)
	}
}

func TestLabServers(t *testing.T) {
	type want struct {
		score   int
		grade   string
		cap     string   // cap grade, "" for none
		capBy   []string // codes imposing it
		include []string // findings that must be there
		ready   string   // quantum readiness: "<score> <level>"
	}
	cases := map[string]want{
		"ssl2":    {45, "F", "F", []string{"PROTO_SSL2"}, []string{"PROTO_SSL2", "KEX_NO_HYBRID", "TLS13_MISSING"}, "15 not_ready"},
		"ssl3":    {45, "F", "F", []string{"SUITE_EXPORT", "SUITE_DES"}, []string{"PROTO_SSL3", "SUITE_EXPORT", "SUITE_DES", "SUITE_RC4", "SUITE_3DES", "DH_SMALL", "SCAN_PARTIAL"}, "15 not_ready"},
		"export":  {45, "F", "F", []string{"SUITE_NULL", "SUITE_EXPORT", "SUITE_ANON", "SUITE_DES", "DH_WEAK"}, []string{"SUITE_NULL", "SUITE_EXPORT", "SUITE_ANON", "SUITE_DES", "DH_WEAK", "PROTO_LEGACY_TLS", "FS_PARTIAL"}, "15 not_ready"},
		"dhe1024": {70, "C", "C", []string{"SUITE_3DES", "DH_SMALL"}, []string{"DH_SMALL", "SUITE_3DES", "SUITE_CBC", "FS_ALL"}, "15 not_ready"},
		"tls10":   {45, "D", "C", []string{"FS_NONE", "SUITE_RC4", "SUITE_3DES"}, []string{"FS_NONE", "SUITE_RC4", "SUITE_3DES", "PROTO_LEGACY_TLS", "SUITE_CBC"}, "15 not_ready"},
		"secp224": {70, "C", "C", []string{"SUITE_RC4", "SUITE_3DES"}, []string{"GROUP_WEAK_CURVES", "SUITE_RC4", "SUITE_3DES"}, "15 not_ready"},
		"tls12":   {45, "D", "B", []string{"SUITE_CBC"}, []string{"DH_OK", "SUITE_CCM8", "SUITE_CBC", "GROUPS_EXTRA", "FS_PARTIAL"}, "15 not_ready"},
		"tls13":   {100, "A", "", nil, []string{"KEX_HYBRID_PREFERRED", "KEX_MULTIPLE_PQC", "KEX_PURE_MLKEM", "SUITE_CCM8", "GROUPS_EXTRA"}, "85 ready"},
		"kyber":   {100, "A", "", nil, []string{"KEX_NO_HYBRID", "KEX_EXPERIMENTAL_PQC_ONLY", "TLS13_SUITES_AEAD", "FS_ALL"}, "10 not_ready"},
		// Well configured today, not quantum-ready: A today, not ready. Kyber
		// stays below it in readiness (the obsolete codepoint costs 15).
		"modern": {100, "A", "", nil, []string{"KEX_NO_HYBRID", "TLS13_SUPPORTED", "TLS13_SUITES_AEAD", "FS_ALL", "CERT_VALID"}, "25 not_ready"},
	}
	for name, r := range labResults(t) {
		res := Evaluate(r)
		capGrade, capBy := "", []string(nil)
		if res.Cap != nil {
			capGrade, capBy = res.Cap.Grade, res.Cap.Codes
		}
		t.Logf("| `%s` | %d | %s | %s %s | %s |", name, res.Score, res.Grade, capGrade, strings.Join(capBy, ", "), strings.Join(codes(res), ", "))
		w, ok := cases[name]
		if !ok {
			t.Errorf("no expectation for %s", name)
			continue
		}
		if got := fmt.Sprintf("%d %s", res.Readiness.Score, res.Readiness.Level); got != w.ready {
			t.Errorf("%s: readiness %s, want %s", name, got, w.ready)
		}
		slices.Sort(capBy)
		slices.Sort(w.capBy)
		if res.Score != w.score || res.Grade != w.grade || capGrade != w.cap || !slices.Equal(capBy, w.capBy) {
			t.Errorf("%s: %d %s, cap %s %v; want %d %s, cap %s %v", name, res.Score, res.Grade, capGrade, capBy, w.score, w.grade, w.cap, w.capBy)
		}
		for _, c := range w.include {
			if !slices.Contains(codes(res), c) {
				t.Errorf("%s: finding %s missing from %v", name, c, codes(res))
			}
		}
	}
}

func TestRawRulesSynthetic(t *testing.T) {
	x := rawExtras()
	if res := Evaluate(x["integrity_only"]); res.Grade != "F" || res.Cap == nil || !slices.Contains(res.Cap.Codes, "SUITE_INTEGRITY_ONLY") {
		t.Errorf("integrity-only TLS 1.3 suite: %s %+v", res.Grade, res.Cap)
	}
	// AEGIS and Ascon are modern AEADs: informational, never a penalty.
	base, modern := Evaluate(apply(func(*scanner.Result) {})), Evaluate(x["modern_aead"])
	if modern.Score != base.Score || modern.Grade != base.Grade || modern.Cap != nil || !slices.Contains(codes(modern), "SUITE_MODERN_AEAD") {
		t.Errorf("AEGIS changed the result: %d %s %+v (base %d %s)", modern.Score, modern.Grade, modern.Cap, base.Score, base.Grade)
	}
	res := Evaluate(x["kyber_and_mlkem"])
	if !slices.Contains(codes(res), "KEX_EXPERIMENTAL_PQC_EXTRA") || slices.Contains(codes(res), "KEX_EXPERIMENTAL_PQC_ONLY") ||
		!slices.Contains(codes(res), "DH_OK") || res.Score != base.Score {
		t.Errorf("Kyber draft next to ML-KEM: %d %v", res.Score, codes(res))
	}
}

// Without raw results (intensity basic, or scans stored before 2.7.3) the
// 2026.2 model gives exactly the 2026.1 score, grade and findings.
func TestBasicScansScoreAsBefore(t *testing.T) {
	for _, r := range []*scanner.Result{ideal(), apply(noPQC), apply(singleHybrid), apply(commercialRSA)} {
		res := Evaluate(r)
		for _, c := range codes(res) {
			if slices.Contains(newIn2026_2, c) {
				t.Errorf("finding %s without raw results", c)
			}
		}
	}
}

// newIn2026_2 are the findings added by model 2026.2 (all need raw probes).
var newIn2026_2 = []string{"PROTO_SSL2", "PROTO_SSL3", "SUITE_NULL", "SUITE_INTEGRITY_ONLY", "SUITE_EXPORT", "SUITE_ANON",
	"SUITE_DES", "SUITE_CCM8", "SUITE_MODERN_AEAD", "TLS13_SUITES_AEAD", "DH_WEAK", "DH_SMALL", "DH_OK", "GROUP_WEAK_CURVES",
	"GROUPS_EXTRA", "KEX_PURE_MLKEM", "KEX_MULTIPLE_PQC", "KEX_EXPERIMENTAL_PQC_ONLY", "KEX_EXPERIMENTAL_PQC_EXTRA"}

// codes returns the finding codes of res, in order.
func codes(res Result) []string {
	var out []string
	for _, f := range res.Findings {
		out = append(out, f.Code)
	}
	return out
}
