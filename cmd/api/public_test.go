package main

import (
	"strings"
	"testing"
)

func TestPublicScanConfig(t *testing.T) {
	key := strings.Repeat("ab", 32)
	cases := []struct {
		env        map[string]string
		open, ok   bool
		hasChecker bool
	}{
		{map[string]string{}, false, true, false},
		{map[string]string{"PUBLIC_SCAN_OPEN": "false"}, false, true, false},
		{map[string]string{"PUBLIC_SCAN_OPEN": "true"}, false, false, false}, // no key: refused
		{map[string]string{"PUBLIC_SCAN_OPEN": "true", "ALTCHA_HMAC_KEY": key}, true, true, true},
		{map[string]string{"PUBLIC_SCAN_OPEN": "yes", "ALTCHA_HMAC_KEY": key}, false, false, false},
		{map[string]string{"PUBLIC_SCAN_OPEN": "true", "ALTCHA_HMAC_KEY": "abcd"}, false, false, false}, // too short
		{map[string]string{"PUBLIC_SCAN_OPEN": "true", "ALTCHA_HMAC_KEY": strings.Repeat("zz", 32)}, false, false, false},
	}
	for _, c := range cases {
		open, v, err := publicScanConfig(func(k string) string { return c.env[k] })
		if (err == nil) != c.ok || open != c.open || (v != nil) != c.hasChecker {
			t.Errorf("%v: open=%v checker=%v err=%v", c.env, open, v != nil, err)
		}
	}
}
