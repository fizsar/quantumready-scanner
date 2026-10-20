package main

import (
	"strings"
	"testing"
)

func TestDemoConfig(t *testing.T) {
	key := strings.Repeat("ab", 32)
	for _, tc := range []struct {
		env     map[string]string
		proof   bool
		wantErr string
	}{
		{map[string]string{}, false, ""},
		{map[string]string{"DEMO_NOTIFY": "false"}, false, ""},
		{map[string]string{"ALTCHA_HMAC_KEY": key}, true, ""},
		{map[string]string{"DEMO_NOTIFY": "true"}, false, "needs the mailer"},
		{map[string]string{"DEMO_NOTIFY": "yes"}, false, "true or false"},
		{map[string]string{"ALTCHA_HMAC_KEY": "zz"}, false, "must be hex"},
		{map[string]string{"ALTCHA_HMAC_KEY": "abcd"}, false, "ALTCHA_HMAC_KEY"},
	} {
		v, err := demoConfig(func(k string) string { return tc.env[k] })
		if tc.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("%v: err %v, want %q", tc.env, err, tc.wantErr)
			}
			continue
		}
		if err != nil || (v != nil) != tc.proof {
			t.Errorf("%v: verifier %v, err %v", tc.env, v != nil, err)
		}
	}
}
