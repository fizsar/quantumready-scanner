package main

import "testing"

func TestAccountsConfig(t *testing.T) {
	const origin = "https://quantumready.fizsar.com"
	for _, c := range []struct {
		value, origin string
		on, ok        bool
	}{
		{"", origin, false, true}, // off by default, even with PUBLIC_ORIGIN
		{"false", origin, false, true},
		{"true", origin, true, true},
		{"true", "", false, false}, // on needs PUBLIC_ORIGIN
		{"yes", origin, false, false},
		{"TRUE", origin, false, false},
	} {
		on, err := accountsConfig(func(k string) string {
			if k == "ACCOUNTS_ENABLED" {
				return c.value
			}
			return ""
		}, c.origin)
		if on != c.on || (err == nil) != c.ok {
			t.Errorf("ACCOUNTS_ENABLED=%q origin=%q: on=%v err=%v", c.value, c.origin, on, err)
		}
	}
}
