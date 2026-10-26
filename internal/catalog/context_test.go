package catalog

import (
	"strings"
	"testing"
	"time"
)

// The SC-081 schedule as of the scan's date: steps in force in the past
// tense, upcoming ones in the future; a step applies from its day on.
func TestValidityScheduleFollowsTheScanDate(t *testing.T) {
	day := func(y int, m time.Month, d, h int) time.Time { return time.Date(y, m, d, h, 0, 0, 0, time.UTC) }
	es := map[string]string{
		"before": "La validez máxima de los certificados bajará a 200 días el 15 de marzo de 2026, a 100 el 15 de marzo de 2027 y a 47 el 15 de marzo de 2029 (votación SC-081 del CA/Browser Forum)",
		"200":    "La validez máxima de los certificados ya es de 200 días (desde el 15 de marzo de 2026) y bajará a 100 días el 15 de marzo de 2027 y a 47 el 15 de marzo de 2029 (votación SC-081 del CA/Browser Forum)",
		"100":    "La validez máxima de los certificados ya es de 100 días (desde el 15 de marzo de 2027) y bajará a 47 días el 15 de marzo de 2029 (votación SC-081 del CA/Browser Forum)",
		"47":     "La validez máxima de los certificados ya es de 47 días (desde el 15 de marzo de 2029; votación SC-081 del CA/Browser Forum)",
	}
	en := map[string]string{
		"before": "Maximum certificate validity will drop to 200 days on 15 March 2026, to 100 on 15 March 2027 and to 47 on 15 March 2029 (CA/Browser Forum ballot SC-081)",
		"200":    "Maximum certificate validity is already 200 days (since 15 March 2026) and will drop to 100 days on 15 March 2027 and to 47 on 15 March 2029 (CA/Browser Forum ballot SC-081)",
		"100":    "Maximum certificate validity is already 100 days (since 15 March 2027) and will drop to 47 days on 15 March 2029 (CA/Browser Forum ballot SC-081)",
		"47":     "Maximum certificate validity is already 47 days (since 15 March 2029; CA/Browser Forum ballot SC-081)",
	}
	for _, tt := range []struct {
		at   time.Time
		want string
	}{
		{day(2026, 3, 14, 23), "before"}, // the evening before the first step
		{day(2026, 3, 15, 0), "200"},     // the day it applies
		{day(2026, 10, 18, 12), "200"},   // the lab scans and fizsar.com
		{day(2027, 3, 14, 23), "200"},
		{day(2027, 3, 15, 0), "100"},
		{day(2029, 3, 14, 23), "100"},
		{day(2029, 3, 15, 0), "47"},
		{day(2031, 1, 1, 0), "47"},
	} {
		if got := scheduleText(tt.at, "es"); got != es[tt.want] {
			t.Errorf("es %v:\n got %q\nwant %q", tt.at, got, es[tt.want])
		}
		if got := scheduleText(tt.at, "en"); got != en[tt.want] {
			t.Errorf("en %v:\n got %q\nwant %q", tt.at, got, en[tt.want])
		}
	}
	// In the finding's text, as the API renders it.
	d := DescribeIn("CERT_NOT_AUTOMATED", map[string]string{"days": "398", "issuer": "X"}, "es", Context{ScanTime: day(2026, 10, 18, 12)}).Detail
	if !strings.Contains(d, es["200"]+". El cambio a certificados post-cuánticos") || strings.Contains(d, "{") {
		t.Errorf("CERT_NOT_AUTOMATED detail: %q", d)
	}
}

// The classical-signature advice depends on whether the scan found automated
// renewal: keep it, or automate it to be able to move to post-quantum
// certificates.
func TestClassicalSignatureAdviceFollowsRenewal(t *testing.T) {
	params := map[string]string{"key": "RSA 2048"}
	for _, tt := range []struct {
		codes []string
		lang  string
		want  string
		not   string
	}{
		{[]string{"CERT_CLASSICAL_SIGNATURE", "CERT_AUTOMATED_ACME"}, "es", "Mantén la renovación automatizada", "Automatiza"},
		{[]string{"CERT_CLASSICAL_SIGNATURE", "CERT_AUTOMATED_SHORT"}, "en", "Keep renewal automated", "Automate"},
		{[]string{"CERT_CLASSICAL_SIGNATURE", "CERT_NOT_AUTOMATED"}, "es", "Automatiza la renovación (ACME)", "Mantén"},
		{[]string{"CERT_CLASSICAL_SIGNATURE", "CERT_NOT_AUTOMATED"}, "en", "Automate renewal (ACME)", "Keep"},
	} {
		r := DescribeIn("CERT_CLASSICAL_SIGNATURE", params, tt.lang, Context{Codes: tt.codes}).Remediation
		if !strings.Contains(r, tt.want) || strings.Contains(r, tt.not) || strings.Contains(r, "{") {
			t.Errorf("%v %s: %q, want %q and no %q", tt.codes, tt.lang, r, tt.want, tt.not)
		}
	}
}
