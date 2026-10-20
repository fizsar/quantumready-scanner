package main

import (
	"strings"
	"testing"
)

func TestDemoConfig(t *testing.T) {
	key := strings.Repeat("ab", 32)
	for _, tc := range []struct {
		env     map[string]string
		enabled bool
		proof   bool
		wantErr string
	}{
		{map[string]string{}, false, false, ""}, // off by default
		{map[string]string{"DEMO_ENABLED": "false"}, false, false, ""},
		{map[string]string{"DEMO_ENABLED": "true"}, true, false, ""},
		{map[string]string{"DEMO_ENABLED": "true", "ALTCHA_HMAC_KEY": key}, true, true, ""},
		{map[string]string{"DEMO_NOTIFY": "false"}, false, false, ""},
		{map[string]string{"DEMO_ENABLED": "yes"}, false, false, "DEMO_ENABLED must be true or false"},
		{map[string]string{"DEMO_NOTIFY": "true"}, false, false, "needs the mailer"},
		{map[string]string{"DEMO_NOTIFY": "yes"}, false, false, "true or false"},
		{map[string]string{"ALTCHA_HMAC_KEY": "zz"}, false, false, "must be hex"},
		{map[string]string{"ALTCHA_HMAC_KEY": "abcd"}, false, false, "ALTCHA_HMAC_KEY"},
	} {
		on, v, err := demoConfig(func(k string) string { return tc.env[k] })
		if tc.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("%v: err %v, want %q", tc.env, err, tc.wantErr)
			}
			continue
		}
		if err != nil || on != tc.enabled || (v != nil) != tc.proof {
			t.Errorf("%v: enabled %v, verifier %v, err %v", tc.env, on, v != nil, err)
		}
	}
}
