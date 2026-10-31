package main

import (
	"errors"
	"fmt"
)

// accountsConfig reads ACCOUNTS_ENABLED: accounts, organizations, invitations
// and verified domains (/api/auth, /api/org, /api/organizations,
// /api/invitations). Off by default and in production (decision of
// 31/10/2026: a portfolio with open public scanning, no sign-up). On needs
// PUBLIC_ORIGIN, which the session cookies and the email links depend on.
// PUBLIC_ORIGIN alone no longer turns accounts on: production keeps it set
// for the Origin check of every POST.
func accountsConfig(getenv func(string) string, origin string) (bool, error) {
	switch getenv("ACCOUNTS_ENABLED") {
	case "", "false":
		return false, nil
	case "true":
		if origin == "" {
			return false, errors.New("ACCOUNTS_ENABLED=true needs PUBLIC_ORIGIN")
		}
		return true, nil
	}
	return false, fmt.Errorf("ACCOUNTS_ENABLED must be true or false, got %q", getenv("ACCOUNTS_ENABLED"))
}
