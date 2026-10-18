package scoring

import (
	"maps"
	"slices"
	"strings"
	"time"
)

// Context is what a finding's text may depend on besides its own params:
// when the scan ran and which other findings it has. It is applied when the
// text is rendered, so stored findings (code and params only) keep reading
// right, old scans included.
type Context struct {
	ScanTime time.Time // zero: now
	Codes    []string  // every finding code of the scan
}

// params returns p plus the placeholders computed from the context:
//   - {renewal_advice}: keep automated renewal, or automate it, depending on
//     whether the scan found automated renewal (CERT_AUTOMATED_*).
func (c Context) params(p map[string]string, lang string) map[string]string {
	out := maps.Clone(p)
	if out == nil {
		out = map[string]string{}
	}
	automated := slices.ContainsFunc(c.Codes, func(code string) bool { return strings.HasPrefix(code, "CERT_AUTOMATED_") })
	out["renewal_advice"] = renewalAdvice[lang][automated]
	return out
}

var renewalAdvice = map[string]map[bool]string{
	"es": {
		true:  "Mantén la renovación automatizada para poder cambiar a certificados ML-DSA o híbridos en cuanto las autoridades los emitan.",
		false: "Automatiza la renovación (ACME) para poder cambiar a certificados post-cuánticos (ML-DSA o híbridos) en cuanto las autoridades los emitan: hoy este certificado no da señales de renovarse solo.",
	},
	"en": {
		true:  "Keep renewal automated so you can switch to ML-DSA or hybrid certificates as soon as authorities issue them.",
		false: "Automate renewal (ACME) so you can switch to post-quantum (ML-DSA or hybrid) certificates as soon as authorities issue them: today this certificate shows no sign of renewing itself.",
	},
}
