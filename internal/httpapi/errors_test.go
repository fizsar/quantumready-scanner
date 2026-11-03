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

// The domain errors answer both the public scan form and adding a domain to
// an organization (accounts, off in production): no words that only fit the
// latter ("verify", "add").
func TestDomainMessagesFitBothForms(t *testing.T) {
	accountOnly := regexp.MustCompile(`(?i)verific|verif(y|ied)|añad|\badd\b`)
	for _, code := range []string{"invalid_domain", "domain_ip", "domain_reserved", "domain_unknown_tld", "domain_public_suffix"} {
		byLang, ok := messages[code]
		if !ok {
			t.Fatalf("%s: no message", code)
		}
		for lang, msg := range byLang {
			if accountOnly.MatchString(msg) {
				t.Errorf("%s (%s): %q", code, lang, msg)
			}
		}
	}
}
