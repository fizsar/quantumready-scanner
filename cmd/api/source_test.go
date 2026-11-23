package main

import "testing"

func TestSourceConfig(t *testing.T) {
	env := func(v string) func(string) string {
		return func(k string) string {
			if k == "QR_SOURCE_URL" {
				return v
			}
			return ""
		}
	}
	for _, tc := range []struct{ value, want string }{
		{"", ""}, // no link unless it is set
		// The repository itself, never /tree/<commit>: the public repository's commits have other hashes.
		{"https://github.com/fizsar/quantumready-scanner", "https://github.com/fizsar/quantumready-scanner"},
		{"https://github.com/fizsar/quantumready-scanner/", "https://github.com/fizsar/quantumready-scanner"},
		{" https://github.com/fizsar/quantumready-scanner ", "https://github.com/fizsar/quantumready-scanner"},
	} {
		got, err := sourceConfig(env(tc.value))
		if err != nil || got != tc.want {
			t.Errorf("%q: %q, %v; want %q", tc.value, got, err, tc.want)
		}
	}
	for _, bad := range []string{"http://github.com/fizsar/quantumready-scanner", "github.com/fizsar/quantumready-scanner", "https://github.com", "https://github.com/", "https://u:p@github.com/x/y", "https://github.com/x/y?a=1", "https://github.com/x/y#z", "javascript:alert(1)"} {
		if got, err := sourceConfig(env(bad)); err == nil {
			t.Errorf("%q accepted: %q", bad, got)
		}
	}
}
