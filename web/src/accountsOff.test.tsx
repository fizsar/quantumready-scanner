import { screen, waitFor } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { json, stubFetch } from './test/fixtures'
import { renderAt } from './test/render'

// Production (Apertura): accounts, organizations and verified domains are off
// (ACCOUNTS_ENABLED=false). The server says so in GET /api/targets, or does
// not say it (an older API, or no answer): either way the web shows none of
// them and never calls /api/auth.

const accountPaths = ['/login', '/register', '/verify', '/forgot', '/reset', '/account', '/org', '/org/scans/abc', '/invite']

const answers = {
  'accounts: false': { hosts: ['fizsar.com'], port: 443, accounts: false },
  'accounts unknown': { hosts: ['fizsar.com'], port: 443 },
}

describe('accounts off', () => {
  for (const [name, targets] of Object.entries(answers)) {
    describe(name, () => {
      for (const path of accountPaths) {
        it(`${path} is the 404, with no request to /api/auth`, async () => {
          const fetch = stubFetch((url) => (url === '/api/targets' ? json(targets) : json({ error: { code: 'not_found', message: 'x' } }, 404)))
          renderAt(path)
          expect(await screen.findByRole('heading', { name: 'Esta página no existe' })).toBeTruthy()
          expect(fetch.mock.calls.some(([u]) => String(u).startsWith('/api/auth') || String(u).startsWith('/api/org'))).toBe(false)
        })
      }

      it('the header offers no sign in nor account', async () => {
        const fetch = stubFetch(() => json(targets))
        renderAt('/')
        await waitFor(() => expect(fetch.mock.calls.some(([u]) => String(u) === '/api/targets')).toBe(true))
        await new Promise((r) => setTimeout(r, 50)) // the flag has been read
        for (const label of ['Entrar', 'Mi cuenta', 'Organización']) {
          expect(screen.queryByRole('link', { name: label })).toBeNull()
        }
        expect(fetch.mock.calls.some(([u]) => String(u).startsWith('/api/auth'))).toBe(false)
      })
    })
  }

  // /about-scans: the owner's opt-out from /org needs an account; without
  // accounts the only way out is the email to soporte.
  for (const accounts of [false, true]) {
    it(`/about-scans offers the opt-out from an account only with accounts on (${accounts})`, async () => {
      stubFetch((url) =>
        url === '/api/targets' ? json({ hosts: [], port: 443, accounts }) : json({ error: { code: 'unauthenticated', message: 'x' } }, 401),
      )
      renderAt('/about-scans')
      expect(await screen.findByText(/Escríbenos a soporte@fizsar\.com/)).toBeTruthy()
      const optout = /Si tienes cuenta, verifica el dominio/
      if (accounts) expect(await screen.findByText(optout)).toBeTruthy()
      else {
        await new Promise((r) => setTimeout(r, 50)) // the flag has been read
        expect(screen.queryByText(optout)).toBeNull()
      }
    })
  }

  it('with accounts on, the same pages and the sign-in link are there', async () => {
    stubFetch((url) =>
      url === '/api/targets' ? json({ hosts: [], port: 443, accounts: true }) : json({ error: { code: 'unauthenticated', message: 'x' } }, 401),
    )
    renderAt('/login')
    expect(await screen.findByRole('link', { name: 'Entrar' })).toBeTruthy()
    expect(screen.queryByRole('heading', { name: 'Esta página no existe' })).toBeNull()
  })
})
