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
	for _, tc := range []struct{ value, commit, want string }{
		{"", "5637c0d", ""}, // off while the repository is private
		{"https://github.com/fizsar/quantumready-saas", "5637c0d", "https://github.com/fizsar/quantumready-saas/tree/5637c0d"},
		{"https://github.com/fizsar/quantumready-saas/", "5637c0da1b2c3d4e5f60718293a4b5c6d7e8f901", "https://github.com/fizsar/quantumready-saas/tree/5637c0da1b2c3d4e5f60718293a4b5c6d7e8f901"},
		{" https://github.com/fizsar/quantumready-saas ", "unknown", "https://github.com/fizsar/quantumready-saas"}, // a dev build: the repository
	} {
		got, err := sourceConfig(env(tc.value), tc.commit)
		if err != nil || got != tc.want {
			t.Errorf("%q, %q: %q, %v; want %q", tc.value, tc.commit, got, err, tc.want)
		}
	}
	for _, bad := range []string{"http://github.com/fizsar/quantumready-saas", "github.com/fizsar/quantumready-saas", "https://github.com", "https://github.com/", "https://u:p@github.com/x/y", "https://github.com/x/y?a=1", "https://github.com/x/y#z", "javascript:alert(1)"} {
		if got, err := sourceConfig(env(bad), "5637c0d"); err == nil {
			t.Errorf("%q accepted: %q", bad, got)
		}
	}
}
