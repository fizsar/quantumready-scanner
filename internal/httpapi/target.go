package httpapi

import (
	"errors"
	"net"
	"net/netip"
	"slices"
	"strings"

	"golang.org/x/net/idna"
)

var (
	errInvalidHost = errors.New("invalid host")
	// errPortNotAllowed: an explicit port other than 443 (only 443 is scanned
	// in this phase; it is refused rather than silently replaced).
	errPortNotAllowed = errors.New("port not allowed")
)

// NormalizeHost turns user input into the bare host that is scanned: trimmed,
// lower case, without scheme, user info, path, query or trailing dot, and
// internationalized names converted to punycode (bücher.de →
// xn--bcher-kva.de). An explicit port is accepted only if it is 443. IP
// literals are returned as such (the allowlist decides).
func NormalizeHost(in string) (string, error) {
	s := strings.TrimSpace(in)
	if s == "" || len(s) > 1024 {
		return "", errInvalidHost
	}
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndex(s, "@"); i >= 0 {
		s = s[i+1:]
	}
	if h, port, err := net.SplitHostPort(s); err == nil {
		if port != "443" {
			return "", errPortNotAllowed
		}
		s = h
	}
	s = strings.TrimSuffix(strings.Trim(s, "[]"), ".")
	if a, err := netip.ParseAddr(s); err == nil {
		return a.String(), nil
	}

	ascii, err := idna.Lookup.ToASCII(s) // also lower-cases and validates
	if err != nil {
		return "", errInvalidHost
	}
	if len(ascii) > 253 || !strings.Contains(ascii, ".") {
		return "", errInvalidHost
	}
	for _, label := range strings.Split(ascii, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", errInvalidHost
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				return "", errInvalidHost
			}
		}
	}
	return ascii, nil
}

// Allowlist holds the hosts that may be scanned in this phase (exact match).
type Allowlist struct {
	hosts map[string]bool
}

// ParseAllowlist reads a comma- or space-separated host list (SCAN_ALLOWLIST),
// normalizing each entry like user input.
func ParseAllowlist(s string) (*Allowlist, error) {
	a := &Allowlist{hosts: map[string]bool{}}
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\t' }) {
		h, err := NormalizeHost(f)
		if err != nil {
			return nil, errors.New("SCAN_ALLOWLIST: invalid host " + f)
		}
		a.hosts[h] = true
	}
	return a, nil
}

// Allows reports whether host (already normalized) is listed.
func (a *Allowlist) Allows(host string) bool { return a != nil && a.hosts[host] }

// Hosts returns the allowed hosts, sorted.
func (a *Allowlist) Hosts() []string {
	out := []string{}
	if a == nil {
		return out
	}
	for h := range a.hosts {
		out = append(out, h)
	}
	slices.Sort(out)
	return out
}

// Len is the number of allowed hosts.
func (a *Allowlist) Len() int {
	if a == nil {
		return 0
	}
	return len(a.hosts)
}
