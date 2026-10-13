package httpapi

import (
	"net/http"
	"net/netip"
	"strings"

	"golang.org/x/text/language"
)

// ParsePrefixes reads a comma-separated list of CIDRs or single IPs
// (TRUSTED_PROXIES).
func ParsePrefixes(s string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, f := range strings.Split(s, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if p, err := netip.ParsePrefix(f); err == nil {
			out = append(out, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(f)
		if err != nil {
			return nil, err
		}
		out = append(out, netip.PrefixFrom(a, a.BitLen()))
	}
	return out, nil
}

func contains(ps []netip.Prefix, a netip.Addr) bool {
	for _, p := range ps {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// clientAddr returns the client's address. X-Forwarded-For is only believed
// when the connection comes from a trusted proxy (the Caddy network): then the
// rightmost address that is not itself a trusted proxy is the client.
// Otherwise anyone could set the header and dodge the rate limit.
func clientAddr(r *http.Request, trusted []netip.Prefix) netip.Addr {
	ap, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}
	}
	remote := ap.Addr().Unmap()
	if !contains(trusted, remote) {
		return remote
	}
	parts := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(parts) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.TrimSpace(parts[i]))
		if err != nil {
			return remote
		}
		a = a.Unmap()
		if !contains(trusted, a) {
			return a
		}
	}
	return remote
}

// rateKey groups IPv6 clients by /64, the usual allocation to one customer.
func rateKey(a netip.Addr) string {
	if a.Is6() {
		p, _ := a.Prefix(64)
		return p.String()
	}
	return a.String()
}

var langMatcher = language.NewMatcher([]language.Tag{language.Spanish, language.English})

// pickLang chooses the response language: ?lang=es|en, then Accept-Language,
// then Spanish.
func pickLang(r *http.Request) string {
	if l := r.URL.Query().Get("lang"); l == "es" || l == "en" {
		return l
	}
	tags, _, err := language.ParseAcceptLanguage(r.Header.Get("Accept-Language"))
	if err != nil || len(tags) == 0 {
		return "es"
	}
	_, idx, conf := langMatcher.Match(tags...)
	if conf == language.No {
		return "es"
	}
	return []string{"es", "en"}[idx]
}
