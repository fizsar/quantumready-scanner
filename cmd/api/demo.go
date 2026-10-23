package main

import (
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/fizsar/quantumready-saas/internal/altcha"
)

// demoConfig reads the settings of the demo request form:
//
//   - DEMO_ENABLED (default false): serve the form at all. Off, POST
//     /api/demo and its challenge are 404 and the web hides /demo and its
//     buttons; stored requests are still purged and qradmin demos works.
//   - ALTCHA_HMAC_KEY (optional): with it, POST /api/demo requires a proof of
//     work. At least 32 random bytes in hex, generated on the server
//     (openssl rand -hex 32). Without it, the honeypot and the per-client
//     limit still apply.
//   - DEMO_NOTIFY (default false): email support about each request. There is
//     no mailer on this branch yet, so true is refused at startup instead of
//     silently telling nobody; main wires it to its email outbox.
func demoConfig(getenv func(string) string) (enabled bool, proof *altcha.Verifier, err error) {
	switch getenv("DEMO_ENABLED") {
	case "", "false":
	case "true":
		enabled = true
	default:
		return false, nil, fmt.Errorf("DEMO_ENABLED must be true or false, got %q", getenv("DEMO_ENABLED"))
	}
	switch getenv("DEMO_NOTIFY") {
	case "", "false":
	case "true":
		return false, nil, errors.New("DEMO_NOTIFY=true needs the mailer, which this version does not have yet")
	default:
		return false, nil, fmt.Errorf("DEMO_NOTIFY must be true or false, got %q", getenv("DEMO_NOTIFY"))
	}
	k := getenv("ALTCHA_HMAC_KEY")
	if k == "" {
		return enabled, nil, nil
	}
	key, err := hex.DecodeString(k)
	if err != nil {
		return false, nil, errors.New("ALTCHA_HMAC_KEY must be hex (openssl rand -hex 32)")
	}
	proof, err = altcha.New(key, 0, 0)
	if err != nil {
		return false, nil, fmt.Errorf("ALTCHA_HMAC_KEY: %w", err)
	}
	return enabled, proof, nil
}
