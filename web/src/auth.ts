import { createContext, useContext, useEffect, useState } from 'react'
import { ApiError, type Me } from './api'
import type { LangContextValue } from './i18n'

/** loading: asking the API; anon: no session (or accounts are off); user: signed in. */
export type AuthState = { status: 'loading' } | { status: 'anon' } | { status: 'user'; me: Me }

export interface AuthValue {
  state: AuthState
  /** Stores the session returned by login. */
  signedIn: (me: Me) => void
  /** Forgets the session locally (after a 401, a password reset…). */
  signedOut: () => void
  logout: () => Promise<void>
  /** Reloads the session (organizations changed). */
  refresh: () => Promise<void>
}

export const AuthContext = createContext<AuthValue | null>(null)

/** The session; see AuthProvider. */
export function useAuth(): AuthValue {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth outside AuthProvider')
  return ctx
}

/**
 * The #token=… of an email link. Links put it in the fragment, which the
 * browser never sends to the server (no proxy logs, no Referer).
 */
export function useHashToken(): string {
  const [token] = useState(() => /(?:^#|&)token=([A-Za-z0-9_-]+)/.exec(window.location.hash)?.[1] ?? '')
  useEffect(() => {
    // Out of the address bar and the history once read.
    if (window.location.hash) window.history.replaceState(window.history.state, '', window.location.pathname + window.location.search)
  }, [])
  return token
}

/** Text for a failed account call: the API's own (localized) message, or ours. */
export function authErrorMessage(e: unknown, t: LangContextValue['t']): string {
  if (!(e instanceof ApiError) || e.status === 0) return t('err.network')
  if (e.status === 429) return e.retryAfter ? t('auth.tooMany', { seconds: e.retryAfter }) : t('auth.tooManyNoWait')
  if (e.status >= 500 || !e.message) return t('err.internal')
  return e.message
}
