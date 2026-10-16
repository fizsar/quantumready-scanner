package main

import (
	"strings"
	"testing"
)

func TestQueueConfig(t *testing.T) {
	if m, p, err := queueConfig(func(string) string { return "" }); err != nil || m != 100 || p != 20 {
		t.Errorf("defaults: %d %d %v", m, p, err)
	}
	for _, env := range []map[string]string{{"QR_PUBLIC_QUEUE_MAX": "0"}, {"QR_PUBLIC_QUEUE_MAX": "x"}, {"WORKER_MAX_SCANS_PER_MINUTE": "601"}} {
		if _, _, err := queueConfig(func(k string) string { return env[k] }); err == nil {
			t.Errorf("%v accepted", env)
		}
	}
}

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
