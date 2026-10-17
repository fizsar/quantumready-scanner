import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { describe, expect, it } from 'vitest'
import type { Lang } from './api'
import App from './App'
import { showcaseDict } from './i18n.showcase'
import { LangProvider } from './LangProvider'
import { json, scanView, stubFetch } from './test/fixtures'
import { forbiddenProse, strip } from './test/forbidden'

// What GET /api/targets answers in production (SCAN_ALLOWLIST): the web has
// no list of its own.
const DEMO = ['fizsar.com', 'quantumready.fizsar.com']
const label = { es: 'Dominio', en: 'Domain' }
const submit = { es: 'Escanear', en: 'Scan' }

function api() {
  return stubFetch(async (url, init) => {
    if (url === '/api/targets') return json({ hosts: DEMO, port: 443 })
    if (url.startsWith('/api/scans?') && init?.method === 'POST') {
      const { host } = JSON.parse(String(init.body)) as { host: string }
      return DEMO.includes(host)
        ? json(scanView('queued', { host }), 202)
        : json({ error: { code: 'host_not_allowed', message: 'Scanning any domain will come with the beta.' } }, 403)
    }
    return json(scanView('queued'))
  })
}

function renderAt(path: string, lang: Lang) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <LangProvider initial={lang}>
        <App />
      </LangProvider>
    </MemoryRouter>,
  )
}

async function tryOtherDomain(lang: Lang) {
  const fetch = api()
  renderAt('/', lang)
  await userEvent.type(await screen.findByLabelText(label[lang]), 'example.com')
  await userEvent.click(screen.getByRole('button', { name: submit[lang] }))
  const block = await screen.findByRole('region', { name: showcaseDict[lang]['hna.title'] })
  return { fetch, block }
}

describe('403 host_not_allowed', () => {
  for (const lang of ['es', 'en'] as const) {
    it(`explains instead of a bare error (${lang})`, async () => {
      const { block } = await tryOtherDomain(lang)
      expect(within(block).getByText(showcaseDict[lang]['hna.why'])).toBeTruthy()
      // The bare error line stays empty: the block replaces it.
      expect(screen.getByRole('alert').textContent).toBe('')
      // One button per demo domain, from GET /api/targets.
      const buttons = within(block).getAllByRole('button')
      expect(buttons.map((b) => b.textContent)).toEqual(DEMO.map((h) => showcaseDict[lang]['hna.scan'].replace('{host}', h)))
      // Links: examples, architecture, contact.
      expect(within(block).getByRole('link', { name: showcaseDict[lang]['hna.examples'] }).getAttribute('href')).toBe('/examples')
      expect(within(block).getByRole('link', { name: showcaseDict[lang]['hna.architecture'] }).getAttribute('href')).toBe('/architecture')
      expect(within(block).getByRole('link', { name: 'soporte@fizsar.com' }).getAttribute('href')).toBe('mailto:soporte@fizsar.com')
    })
  }

  it('its buttons scan the right domain', async () => {
    const { fetch, block } = await tryOtherDomain('en')
    await userEvent.click(within(block).getByRole('button', { name: 'Scan quantumready.fizsar.com' }))
    const posts = fetch.mock.calls.filter(([, init]) => init?.method === 'POST').map(([, init]) => JSON.parse(String(init?.body)))
    expect(posts).toEqual([{ host: 'example.com' }, { host: 'quantumready.fizsar.com' }])
  })

  it('its links lead to pages that exist', async () => {
    const { block } = await tryOtherDomain('es')
    const internal = within(block)
      .getAllByRole('link')
      .map((a) => a.getAttribute('href')!)
      .filter((h) => h.startsWith('/'))
    expect(internal).toEqual(['/examples', '/architecture'])
    for (const h of internal) {
      cleanup()
      renderAt(h, 'es')
      const h1 = await screen.findByRole('heading', { level: 1 })
      expect(h1.textContent, h).not.toBe('Esta página no existe')
    }
  })

  for (const lang of ['es', 'en'] as const) {
    it(`promises no dates and says nothing it must not (${lang})`, async () => {
      const { block } = await tryOtherDomain(lang)
      const texts = [
        ...Object.entries(showcaseDict[lang])
          .filter(([k]) => k.startsWith('hna.'))
          .map(([, v]) => v),
        block.textContent ?? '',
      ]
        .map(strip)
        .join('\n')
      for (const re of forbiddenProse) expect(texts, String(re)).not.toMatch(re)
      expect(texts).not.toMatch(/\b20\d\d\b|pronto|próxim|semana|\bmes(es)?\b|soon|next (week|month)|weeks?\b|months?\b|Q[1-4]\b/i)
    })
  }
})
