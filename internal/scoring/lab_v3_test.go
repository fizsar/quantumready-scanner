package scoring

import (
	"path/filepath"
	"slices"
	"strconv"
	"testing"
)

// The scans in testdata/lab_v3 were written on 29/10/2026 by the scanner of
// milestone 2.8.7 (certificate from the raw probes, Chrome 156's offer, TLS
// 1.3 suite order), for the twelve lab servers (intensity full, as in
// TestLab) and a real scan of fizsar.com. Against the scans from before
// (lab_v2, lab_pq), with the same model, the judgement changes only where
// 2.8.7 says it does.
func TestLabV3AgainstBefore(t *testing.T) {
	cert := []string{"CERT_HOSTNAME_MISMATCH", "CERT_SELF_SIGNED", "CERT_CLASSICAL_SIGNATURE", "CERT_NOT_AUTOMATED", "CERT_CT_MISSING"}
	order := []string{"TLS13_SUITE_ORDER_CLIENT"}
	for server, want := range map[string]struct {
		before, after  string // "<grade> <score> <readiness>"
		added, removed []string
	}{
		// The certificate, now read from the raw probes: self-signed and for
		// another name, so F stays; key and signature add 20.
		"ssl2":    {"F 0 0", "F 20 0", cert, []string{"CERT_MISSING"}},
		"ssl3":    {"F 0 0", "F 20 0", cert, []string{"CERT_MISSING"}},
		"export":  {"F 0 0", "F 20 0", cert, []string{"CERT_MISSING"}},
		"dhe1024": {"F 25 0", "F 45 0", cert, []string{"CERT_MISSING"}},
		"secp224": {"F 25 0", "F 45 0", cert, []string{"CERT_MISSING"}},
		// Nothing new: crypto/tls already saw the certificate, one TLS 1.3 suite at most.
		"tls10": {"F 20 0", "F 20 0", nil, nil},
		"tls12": {"F 20 0", "F 20 0", nil, nil},
		// The TLS 1.3 suite order, informational: every one follows the client.
		"tls13":      {"F 75 70", "F 75 70", order, nil},
		"modern":     {"F 75 10", "F 75 10", order, nil},
		"mldsa-dual": {"F 75 75", "F 75 75", order, nil},
		"mldsa-only": {"F 75 75", "F 75 75", order, nil},
		"fizsar.com": {"A 100 75", "A 100 75", order, nil},
		// A self-signed leaf without the CA flag is now self-signed, not just untrusted.
		"kyber": {"F 75 0", "F 75 0", []string{"CERT_SELF_SIGNED", "TLS13_SUITE_ORDER_CLIENT"}, []string{"CERT_UNTRUSTED"}},
	} {
		t.Run(server, func(t *testing.T) {
			dir := "lab_v2"
			if server == "mldsa-dual" || server == "mldsa-only" {
				dir = "lab_pq"
			}
			b, a := Evaluate(readResult(t, filepath.Join("testdata", dir, server+".json"))), Evaluate(readResult(t, filepath.Join("testdata", "lab_v3", server+".json")))
			line := func(r Result) string {
				return r.Grade + " " + strconv.Itoa(r.Score) + " " + strconv.Itoa(r.Readiness.Score)
			}
			if line(b) != want.before || line(a) != want.after {
				t.Errorf("before %s, after %s; want %s, %s", line(b), line(a), want.before, want.after)
			}
			var added, removed []string
			for _, c := range codes(a) {
				if !slices.Contains(codes(b), c) {
					added = append(added, c)
				}
			}
			for _, c := range codes(b) {
				if !slices.Contains(codes(a), c) {
					removed = append(removed, c)
				}
			}
			if !slices.Equal(added, want.added) || !slices.Equal(removed, want.removed) {
				t.Errorf("added %v, removed %v; want %v, %v", added, removed, want.added, want.removed)
			}
		})
	}
}
