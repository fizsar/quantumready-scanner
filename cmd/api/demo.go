package main

import (
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/fizsar/quantumready-saas/internal/altcha"
)

// demoConfig reads the settings of the demo request form:
//
//   - ALTCHA_HMAC_KEY (optional): with it, POST /api/demo requires a proof of
//     work. At least 32 random bytes in hex, generated on the server
//     (openssl rand -hex 32). Without it, the honeypot and the per-client
//     limit still apply.
//   - DEMO_NOTIFY (default false): email support about each request. There is
//     no mailer on this branch yet, so true is refused at startup instead of
//     silently telling nobody; main wires it to its email outbox.
func demoConfig(getenv func(string) string) (*altcha.Verifier, error) {
	switch getenv("DEMO_NOTIFY") {
	case "", "false":
	case "true":
		return nil, errors.New("DEMO_NOTIFY=true needs the mailer, which this version does not have yet")
	default:
		return nil, fmt.Errorf("DEMO_NOTIFY must be true or false, got %q", getenv("DEMO_NOTIFY"))
	}
	k := getenv("ALTCHA_HMAC_KEY")
	if k == "" {
		return nil, nil
	}
	key, err := hex.DecodeString(k)
	if err != nil {
		return nil, errors.New("ALTCHA_HMAC_KEY must be hex (openssl rand -hex 32)")
	}
	v, err := altcha.New(key, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("ALTCHA_HMAC_KEY: %w", err)
	}
	return v, nil
}
