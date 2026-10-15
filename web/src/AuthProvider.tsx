import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { ApiError, getMe, logout as apiLogout, type Me } from './api'
import { AuthContext, type AuthState } from './auth'
import { useLang } from './i18n'

/** Holds the session: asks the API once (GET /api/auth/me), then follows login and logout. */
export function AuthProvider({ children }: { children: ReactNode }) {
  const { lang } = useLang()
  const [state, setState] = useState<AuthState>({ status: 'loading' })

  useEffect(() => {
    let cancelled = false
    getMe(lang)
      .then((me) => !cancelled && setState({ status: 'user', me }))
      // 401 (no session), 404 (accounts disabled) or offline: anonymous.
      .catch(() => !cancelled && setState({ status: 'anon' }))
    return () => {
      cancelled = true
    }
    // The session does not depend on the language: ask once.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const signedIn = useCallback((me: Me) => setState({ status: 'user', me }), [])
  const signedOut = useCallback(() => setState({ status: 'anon' }), [])
  const logout = useCallback(async () => {
    if (state.status !== 'user') return
    try {
      await apiLogout(state.me.csrf_token, lang)
    } catch (e) {
      // Already gone on the server: forget it here too.
      if (!(e instanceof ApiError && e.status === 401)) throw e
    }
    setState({ status: 'anon' })
  }, [state, lang])

  const value = useMemo(() => ({ state, signedIn, signedOut, logout }), [state, signedIn, signedOut, logout])
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}
