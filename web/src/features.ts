import { useEffect, useState } from 'react'
import { getTargets } from './api'

// Server-side switches the web follows, read from GET /api/targets (one
// request per page load, shared): one build, whatever the server's settings.
// Unknown (no answer, an older API) means off.

let flags: Promise<{ demo: boolean; accounts: boolean; source: string | null }> | null = null

// Only an https URL becomes a link: anything else from the server is ignored.
const httpsURL = (v: unknown) => (typeof v === 'string' && /^https:\/\/[^\s"<>]+$/.test(v) ? v : null)

function features() {
  flags ??= getTargets()
    .then((r) => ({ demo: r.demo === true, accounts: r.accounts === true, source: httpsURL(r.source) }))
    .catch(() => ({ demo: false, accounts: false, source: null }))
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

/**
 * Where this code is published, at the deployed commit (QR_SOURCE_URL on the
 * server; the AGPL's offer of the source), or null: no link.
 */
export function sourceURL(): Promise<string | null> {
  return features().then((f) => f.source)
}

/** sourceURL() for components: null while unknown, or with no link. */
export function useSourceURL(): string | null {
  const [url, setURL] = useState<string | null>(null)
  useEffect(() => {
    let cancelled = false
    void sourceURL().then((v) => !cancelled && setURL(v))
    return () => {
      cancelled = true
    }
  }, [])
  return url
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
