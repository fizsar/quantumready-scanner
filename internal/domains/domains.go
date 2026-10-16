// Package domains validates the domains an organization may verify: real,
// registrable names (or subdomains of them), never a public suffix (com,
// co.uk, github.io…), an IP address or a special-use name. The Public
// Suffix List is embedded (public_suffix_list.dat, pinned in
// psl_version.go; make psl updates it).
package domains

import (
	_ "embed"
	"errors"
	"net/netip"
	"strings"

	"golang.org/x/net/idna"
)

//go:embed public_suffix_list.dat
var pslData string

// Errors of Normalize.
var (
	ErrInvalid      = errors.New("domains: not a valid domain name")
	ErrIP           = errors.New("domains: an IP address is not a domain")
	ErrSpecialUse   = errors.New("domains: special-use or reserved name")
	ErrUnknownTLD   = errors.New("domains: unknown top-level domain")
	ErrPublicSuffix = errors.New("domains: a public suffix cannot be verified")
)

type rules struct {
	exact, wildcard, exception map[string]bool
}

var psl = func() *rules {
	r := &rules{exact: map[string]bool{}, wildcard: map[string]bool{}, exception: map[string]bool{}}
	for _, line := range strings.Split(pslData, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		if i := strings.IndexAny(line, " \t"); i >= 0 {
			line = line[:i]
		}
		set := r.exact
		switch {
		case strings.HasPrefix(line, "!"):
			set, line = r.exception, line[1:]
		case strings.HasPrefix(line, "*."):
			set, line = r.wildcard, line[2:]
		}
		ascii, err := idna.Lookup.ToASCII(line)
		if err != nil {
			continue // a rule we could never match in punycode
		}
		set[strings.ToLower(ascii)] = true
	}
	return r
}()

// PublicSuffix returns the public suffix of an ASCII, lower-case name and
// whether a rule of the list matched (false: only the default "*" rule, an
// unknown top-level domain). Algorithm of publicsuffix.org/list.
func PublicSuffix(name string) (suffix string, known bool) {
	labels := strings.Split(name, ".")
	for i := range labels {
		s := strings.Join(labels[i:], ".")
		if psl.exception[s] {
			return strings.Join(labels[i+1:], "."), true
		}
		if psl.exact[s] || (i+1 < len(labels) && psl.wildcard[strings.Join(labels[i+1:], ".")]) {
			return s, true
		}
	}
	return labels[len(labels)-1], false
}

// Special-use names (RFC 6761 and later) and reverse-DNS zones: never verified.
var specialTLDs = map[string]bool{
	"local": true, "localhost": true, "test": true, "example": true, "invalid": true,
	"onion": true, "internal": true, "alt": true, "arpa": true, "lan": true, "home": true, "corp": true,
}

// Normalize returns the canonical form of a domain an organization wants to
// verify (lower case, punycode, no trailing dot) or why it cannot be one.
func Normalize(in string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(in))
	s = strings.TrimSuffix(s, ".")
	if s == "" || len(s) > 253 || strings.ContainsAny(s, "/:@?#[] \t") {
		if a, err := netip.ParseAddr(strings.Trim(s, "[]")); err == nil && a.IsValid() {
			return "", ErrIP
		}
		return "", ErrInvalid
	}
	if _, err := netip.ParseAddr(s); err == nil {
		return "", ErrIP
	}
	ascii, err := idna.Lookup.ToASCII(s)
	if err != nil {
		return "", ErrInvalid
	}
	labels := strings.Split(ascii, ".")
	if len(labels) < 2 || len(ascii) > 253 {
		return "", ErrInvalid
	}
	for _, l := range labels {
		if !validLabel(l) {
			return "", ErrInvalid
		}
	}
	tld := labels[len(labels)-1]
	if specialTLDs[tld] || allDigits(tld) {
		return "", ErrSpecialUse
	}
	suffix, known := PublicSuffix(ascii)
	if !known {
		return "", ErrUnknownTLD
	}
	if suffix == ascii {
		return "", ErrPublicSuffix
	}
	return ascii, nil
}

// validLabel: letters, digits and inner hyphens, 1 to 63 characters.
func validLabel(l string) bool {
	if l == "" || len(l) > 63 || l[0] == '-' || l[len(l)-1] == '-' {
		return false
	}
	for _, c := range l {
		letter, digit := c >= 'a' && c <= 'z', c >= '0' && c <= '9'
		if !letter && !digit && c != '-' {
			return false
		}
	}
	return true
}

func allDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return s != ""
}

// Registrable returns the registrable domain of a normalized host (its
// public suffix plus one label: www.shop.example.co.uk → example.co.uk), or
// the host itself if it has none.
func Registrable(host string) string {
	suffix, _ := PublicSuffix(host)
	if host == suffix || !strings.HasSuffix(host, "."+suffix) {
		return host
	}
	rest := strings.TrimSuffix(host, "."+suffix)
	return rest[strings.LastIndexByte(rest, '.')+1:] + "." + suffix
}

// Covers reports whether a verified domain covers host: the domain itself
// or any subdomain of it (both normalized).
func Covers(domain, host string) bool {
	return host == domain || strings.HasSuffix(host, "."+domain)
}

// RecordName is where the verification TXT record goes.
func RecordName(domain string) string { return "_quantumready." + domain }

// RecordValue is the TXT value that proves control of a domain.
func RecordValue(token string) string { return "qr-verify=" + token }
