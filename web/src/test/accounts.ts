import { json, stubFetch as stubAll } from './fixtures'

/**
 * stubFetch for the tests of accounts, organizations and verified domains:
 * the server has them on (ACCOUNTS_ENABLED), so GET /api/targets says
 * "accounts": true on top of whatever the handler answers. Production has
 * them off; accountsOff.test.tsx covers that.
 */
export function stubFetch(handler: (url: string, init?: RequestInit) => Response | Promise<Response>) {
  return stubAll(async (url, init) => {
    const res = await handler(url, init)
    if (url !== '/api/targets') return res
    const body = res.ok ? await res.clone().json().catch(() => ({})) : {}
    return json({ hosts: [], port: 443, ...body, accounts: true })
  })
}
