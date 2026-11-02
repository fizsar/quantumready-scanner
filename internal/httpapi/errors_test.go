package httpapi

import (
	"regexp"
	"testing"
)

// No error message promises a beta or what "will come" (decision of
// 31/10/2026: no accounts in production, the scanner opens to any domain).
// Only new_organizations_closed may: it is an account error, answered with
// ACCOUNTS_ENABLED=true, where the closed beta is real.
func TestErrorMessagesMakeNoPromises(t *testing.T) {
	promise := regexp.MustCompile(`(?i)\bbeta\b|llegará con|will come with`)
	accountOnly := map[string]bool{"new_organizations_closed": true}
	for code, byLang := range messages {
		for lang, msg := range byLang {
			if !accountOnly[code] && promise.MatchString(msg) {
				t.Errorf("%s (%s): %q", code, lang, msg)
			}
		}
	}
	for code := range accountOnly {
		if _, ok := messages[code]; !ok {
			t.Errorf("%s is gone: update accountOnly", code)
		}
	}
}
