package main

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var commitRe = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

// sourceConfig reads QR_SOURCE_URL, the public repository of this code
// ("https://github.com/fizsar/quantumready-saas"): the AGPL asks a network
// service to offer its source to its users. Empty (the default, and while the
// repository is private) shows no link. Set, GET /api/targets returns it
// pointing at the commit this binary was built from, which the web shows in
// its footer; without a known commit, the repository itself.
func sourceConfig(getenv func(string) string, commit string) (string, error) {
	v := strings.TrimSpace(getenv("QR_SOURCE_URL"))
	if v == "" {
		return "", nil
	}
	u, err := url.Parse(v)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.Trim(u.Path, "/") == "" {
		return "", fmt.Errorf("QR_SOURCE_URL must be an https URL of the repository (https://github.com/owner/repo), got %q", v)
	}
	repo := strings.TrimRight(u.String(), "/")
	if commitRe.MatchString(commit) {
		return repo + "/tree/" + commit, nil
	}
	return repo, nil
}
