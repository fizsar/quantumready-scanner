package main

import (
	"fmt"
	"net/url"
	"strings"
)

// sourceConfig reads QR_SOURCE_URL, the public repository of this code
// ("https://github.com/fizsar/quantumready-scanner"): the AGPL asks a network
// service to offer its source to its users. Empty (the default) shows no link.
// Set, GET /api/targets returns the repository itself, which the web shows in
// its footer. Not the commit this binary was built from: it is built in the
// private repository, and the public one carries a filtered history whose
// commits have other hashes (a link to /tree/<commit> there was a 404).
func sourceConfig(getenv func(string) string) (string, error) {
	v := strings.TrimSpace(getenv("QR_SOURCE_URL"))
	if v == "" {
		return "", nil
	}
	u, err := url.Parse(v)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.Trim(u.Path, "/") == "" {
		return "", fmt.Errorf("QR_SOURCE_URL must be an https URL of the repository (https://github.com/owner/repo), got %q", v)
	}
	return strings.TrimRight(u.String(), "/"), nil
}
