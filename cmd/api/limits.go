package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/fizsar/quantumready-saas/internal/httpapi"
)

// limits are the scan request limits, configurable from the environment
// (see .env.example). Defaults are the values used since milestone 2.4.
type limits struct {
	perMinute, perDay int
	cooldown          time.Duration
}

func loadLimits(getenv func(string) string) (limits, error) {
	l := limits{perMinute: 5, perDay: 30, cooldown: httpapi.DefaultTargetCooldown}
	var err error
	if l.perMinute, err = intVar(getenv, "RATE_LIMIT_PER_MINUTE", l.perMinute, 1, 1000); err != nil {
		return l, err
	}
	if l.perDay, err = intVar(getenv, "RATE_LIMIT_PER_DAY", l.perDay, 1, 100000); err != nil {
		return l, err
	}
	if l.perDay < l.perMinute {
		return l, fmt.Errorf("RATE_LIMIT_PER_DAY (%d) must be >= RATE_LIMIT_PER_MINUTE (%d)", l.perDay, l.perMinute)
	}
	if v := getenv("SCAN_TARGET_COOLDOWN"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < time.Second || d > 24*time.Hour {
			return l, fmt.Errorf("SCAN_TARGET_COOLDOWN must be a duration between 1s and 24h (e.g. 2m), got %q", v)
		}
		l.cooldown = d
	}
	return l, nil
}

func intVar(getenv func(string) string, key string, def, lo, hi int) (int, error) {
	v := getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < lo || n > hi {
		return 0, fmt.Errorf("%s must be an integer between %d and %d, got %q", key, lo, hi, v)
	}
	return n, nil
}
