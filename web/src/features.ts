import { useEffect, useState } from 'react'
import { getTargets } from './api'

// Server-side switches the web follows, read from GET /api/targets (one
// request per page load, shared): one build, whatever the server's settings.
// Unknown (no answer, an older API) means off.

let flags: Promise<{ demo: boolean; accounts: boolean }> | null = null

function features() {
  flags ??= getTargets()
    .then((r) => ({ demo: r.demo === true, accounts: r.accounts === true }))
    .catch(() => ({ demo: false, accounts: false }))
  return flags
}

/** Whether the "Request a free demo" form is on (DEMO_ENABLED). */
export function demoEnabled(): Promise<boolean> {
  return features().then((f) => f.demo)
}

/**
 * Whether accounts, organizations and verified domains are on
 * (ACCOUNTS_ENABLED). Off in production: no login, no account pages and no
 * call to /api/auth.
 */
export function accountsEnabled(): Promise<boolean> {
  return features().then((f) => f.accounts)
}

function useFlag(get: () => Promise<boolean>): boolean | null {
  const [on, setOn] = useState<boolean | null>(null)
  useEffect(() => {
    let cancelled = false
    void get().then((v) => !cancelled && setOn(v))
    return () => {
      cancelled = true
    }
  }, [get])
  return on
}

/** demoEnabled() for components: null while it is not known yet. */
export function useDemoEnabled(): boolean | null {
  return useFlag(demoEnabled)
}

/** accountsEnabled() for components: null while it is not known yet. */
export function useAccountsEnabled(): boolean | null {
  return useFlag(accountsEnabled)
}

export function resetFeaturesForTests() {
  flags = null
}
