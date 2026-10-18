package scoring

import (
	"strings"
	"testing"
)

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
