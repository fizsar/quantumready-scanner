import { screen, waitFor } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { json, stubFetch } from './test/fixtures'
import { renderAt } from './test/render'

// The AGPL's "Source code" link in the footer: only when the server sends
// an https URL (QR_SOURCE_URL), pointing at the deployed commit.
const base = { hosts: ['fizsar.com'], port: 443 }
const commitURL = 'https://github.com/fizsar/quantumready-saas/tree/5637c0d'

async function footerAfterTargets(targets: object, lang?: 'en') {
  const fetch = stubFetch((url) => (url === '/api/targets' ? json(targets) : json({ error: { code: 'not_found', message: 'x' } }, 404)))
  renderAt('/', lang)
  await waitFor(() => expect(fetch.mock.calls.some(([u]) => String(u) === '/api/targets')).toBe(true))
  await new Promise((r) => setTimeout(r, 50)) // the answer has been read
  return screen.getByRole('contentinfo')
}

describe('source code link', () => {
  it('links to the deployed commit when the server says where (es)', async () => {
    const footer = await footerAfterTargets({ ...base, source: commitURL })
    const link = await screen.findByRole('link', { name: 'Código fuente' })
    expect(footer.contains(link)).toBe(true)
    expect(link.getAttribute('href')).toBe(commitURL)
  })

  it('in English', async () => {
    await footerAfterTargets({ ...base, source: commitURL }, 'en')
    expect((await screen.findByRole('link', { name: 'Source code' })).getAttribute('href')).toBe(commitURL)
  })

  for (const [name, targets] of [
    ['no source (the repository is private)', base],
    ['not https', { ...base, source: 'http://github.com/fizsar/quantumready-saas' }],
    ['a script URL', { ...base, source: 'javascript:alert(1)' }],
    ['not a string', { ...base, source: 42 }],
  ] as const) {
    it(`no link: ${name}`, async () => {
      await footerAfterTargets(targets)
      expect(screen.queryByRole('link', { name: 'Código fuente' })).toBeNull()
    })
  }
})
