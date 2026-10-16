package main

import (
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/fizsar/quantumready-saas/internal/altcha"
)

// publicScanConfig reads PUBLIC_SCAN_OPEN and ALTCHA_HMAC_KEY. Opening
// public scanning (any domain) requires the proof-of-work key: at least 32
// random bytes in hex (openssl rand -hex 32, generated on the server).
func publicScanConfig(getenv func(string) string) (open bool, v *altcha.Verifier, err error) {
	switch getenv("PUBLIC_SCAN_OPEN") {
	case "", "false":
	case "true":
		open = true
	default:
		return false, nil, fmt.Errorf("PUBLIC_SCAN_OPEN must be true or false, got %q", getenv("PUBLIC_SCAN_OPEN"))
	}
	k := getenv("ALTCHA_HMAC_KEY")
	if k == "" {
		if open {
			return false, nil, errors.New("PUBLIC_SCAN_OPEN=true needs ALTCHA_HMAC_KEY (openssl rand -hex 32)")
		}
		return false, nil, nil
	}
	key, err := hex.DecodeString(k)
	if err != nil {
		return false, nil, errors.New("ALTCHA_HMAC_KEY must be hex (openssl rand -hex 32)")
	}
	v, err = altcha.New(key, 0, 0)
	if err != nil {
		return false, nil, fmt.Errorf("ALTCHA_HMAC_KEY: %w", err)
	}
	return open, v, nil
}
