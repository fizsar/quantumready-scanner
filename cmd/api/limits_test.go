package main

import (
	"testing"
	"time"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestLoadLimits(t *testing.T) {
	l, err := loadLimits(env(nil))
	if err != nil || l.perMinute != 5 || l.perDay != 30 || l.cooldown != 2*time.Minute {
		t.Fatalf("defaults = %+v, %v; want 5/min, 30/day, 2m", l, err)
	}
	l, err = loadLimits(env(map[string]string{"RATE_LIMIT_PER_MINUTE": "1", "RATE_LIMIT_PER_DAY": "10", "SCAN_TARGET_COOLDOWN": "30s"}))
	if err != nil || l.perMinute != 1 || l.perDay != 10 || l.cooldown != 30*time.Second {
		t.Fatalf("custom = %+v, %v", l, err)
	}
	for _, bad := range []map[string]string{
		{"RATE_LIMIT_PER_MINUTE": "0"},
		{"RATE_LIMIT_PER_MINUTE": "five"},
		{"RATE_LIMIT_PER_DAY": "-1"},
		{"RATE_LIMIT_PER_MINUTE": "10", "RATE_LIMIT_PER_DAY": "5"},
		{"SCAN_TARGET_COOLDOWN": "120"}, // missing unit
		{"SCAN_TARGET_COOLDOWN": "0s"},
		{"SCAN_TARGET_COOLDOWN": "48h"},
	} {
		if _, err := loadLimits(env(bad)); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
}
