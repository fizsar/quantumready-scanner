import { json, stubFetch as stubAll } from './fixtures'

/**
 * stubFetch for pages seen by a visitor without a session: GET /api/auth/me
 * (AuthProvider) answers 401, everything else goes to handler. The showcase
 * pages (/architecture, /examples, the scan panel, the 403 block) are public.
 */
export function stubFetch(handler: (url: string, init?: RequestInit) => Response | Promise<Response>) {
  return stubAll((url, init) =>
    url.startsWith('/api/auth/') ? json({ error: { code: 'unauthorized', message: 'no session' } }, 401) : handler(url, init),
  )
}
