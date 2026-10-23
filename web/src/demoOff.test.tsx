import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { describe, expect, it } from 'vitest'
import type { Lang } from './api'
import App from './App'
import { showcaseDict } from './i18n.showcase'
import { AuthProvider } from './AuthProvider'
import { LangProvider } from './LangProvider'
import { json } from './test/fixtures'
import { stubFetch } from './test/visitor'
import fizsarEn from './test/lab/fizsar.en.json'

// DEMO_ENABLED off (the production default): the server says "demo": false
// (or nothing, or cannot be reached) and the web shows no trace of the form.
type Targets = Record<string, unknown> | 'down'

function renderAt(path: string, targets: Targets, lang: Lang = 'es') {
  const fetch = stubFetch(async (url, init) => {
    if (url === '/api/targets') return targets === 'down' ? Promise.reject(new Error('offline')) : json(targets)
    if (init?.method === 'POST') return json({ error: { code: 'host_not_allowed', message: 'x' } }, 403)
    return json(fizsarEn)
  })
  render(
    <MemoryRouter initialEntries={[path]}>
      <LangProvider initial={lang}>
        <AuthProvider>
          <App />
        </AuthProvider>
      </LangProvider>
    </MemoryRouter>,
  )
  return fetch
}

const off: Targets[] = [
  { hosts: ['fizsar.com'], port: 443, demo: false },
  { hosts: ['fizsar.com'], port: 443 }, // an older API that does not say
]

describe('demo form off', () => {
  for (const targets of off) {
    it(`no button on the home page, in the 403 block or on /examples (${JSON.stringify(targets)})`, async () => {
      renderAt('/', targets)
      await screen.findByRole('button', { name: 'Escanear fizsar.com' }) // targets loaded
      await screen.findByRole('region', { name: showcaseDict.es['how.title'] })
      expect(screen.queryByRole('link', { name: showcaseDict.es['demo.button'] })).toBeNull()
      expect(document.body.textContent).not.toContain(showcaseDict.es['demo.cta'])

      await userEvent.type(screen.getByRole('textbox'), 'example.org')
      await userEvent.click(screen.getByRole('button', { name: 'Escanear' }))
      const block = await screen.findByRole('heading', { name: showcaseDict.es['hna.title'] })
      expect(within(block.parentElement!).queryByRole('link', { name: showcaseDict.es['demo.button'] })).toBeNull()
    })

    it(`/demo is the SPA's not-found page (${JSON.stringify(targets)})`, async () => {
      renderAt('/demo', targets)
      expect(await screen.findByRole('heading', { level: 1, name: 'Esta página no existe' })).toBeTruthy()
      expect(screen.queryByRole('textbox')).toBeNull()
    })
  }

  it('none on /examples', async () => {
    renderAt('/examples', off[0], 'en')
    await screen.findByRole('heading', { level: 1, name: 'Examples' }, { timeout: 5000 })
    await new Promise((r) => setTimeout(r, 0))
    expect(screen.queryByRole('link', { name: showcaseDict.en['demo.button'] })).toBeNull()
  })

  it('/privacy keeps where the data is, without the demo section or line', async () => {
    renderAt('/privacy', off[0])
    const where = await screen.findByRole('region', { name: 'Dónde están los datos' })
    expect(where.textContent).toContain('OVH, en Beauharnois (Canadá)')
    expect(screen.queryByRole('region', { name: 'Si pides una demo' })).toBeNull()
    expect(document.body.textContent).not.toContain(showcaseDict.es['privacy.counted.demo'])
    expect(document.body.textContent).not.toContain('12 meses')
  })

  it('an API that cannot be reached counts as off', async () => {
    renderAt('/demo', 'down')
    expect(await screen.findByRole('heading', { level: 1, name: 'Esta página no existe' })).toBeTruthy()
  })

  it('on: /privacy has the demo section and line (the same page, one build)', async () => {
    renderAt('/privacy', { hosts: ['fizsar.com'], port: 443, demo: true })
    expect(await screen.findByRole('region', { name: 'Si pides una demo' })).toBeTruthy()
    expect(document.body.textContent).toContain(showcaseDict.es['privacy.counted.demo'])
  })

  it('/api/targets is read once per page load, however many components ask', async () => {
    const fetch = renderAt('/', { hosts: ['fizsar.com'], port: 443, demo: true })
    await screen.findByRole('link', { name: showcaseDict.es['demo.button'] })
    await userEvent.click(screen.getByRole('link', { name: 'Privacidad' }))
    await screen.findByRole('region', { name: 'Si pides una demo' })
    // Home's own list of demo domains is one more read; the switch is one, shared.
    expect(fetch.mock.calls.filter(([u]) => u === '/api/targets').length).toBeLessThanOrEqual(2)
  })
})
