package httpapi

import (
	"testing"

	"github.com/fizsar/quantumready-saas/internal/catalog"
)

// The API's own messages make none of the claims no report may make
// (catalog.ForbiddenClaims: not official, no ENS certification or
// compliance, NIS2 does not require post-quantum cryptography).
func TestMessagesMakeNoForbiddenClaims(t *testing.T) {
	for code, byLang := range messages {
		for lang, s := range byLang {
			for _, re := range catalog.ForbiddenClaims {
				if re.MatchString(s) {
					t.Errorf("%s (%s) matches %s: %q", code, lang, re, s)
				}
			}
		}
	}
}
