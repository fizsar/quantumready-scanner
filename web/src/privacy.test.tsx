import { render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { describe, expect, it } from 'vitest'
import type { Lang } from './api'
import App from './App'
import { showcaseDict } from './i18n.showcase'
import { LangProvider } from './LangProvider'
import { routeOf } from './metrics'
import { json, stubFetch } from './test/fixtures'
import { forbiddenProse, strip } from './test/forbidden'

function renderAt(path: string, lang: Lang) {
  stubFetch(() => json({ hosts: ['fizsar.com'], port: 443 }))
  return render(
    <MemoryRouter initialEntries={[path]}>
      <LangProvider initial={lang}>
        <App />
      </LangProvider>
    </MemoryRouter>,
  )
}

const heading = { es: 'Privacidad', en: 'Privacy' }
const sections = {
  es: ['Qué contamos', 'Qué no hacemos', 'Control Global de Privacidad (GPC)', 'Dominios que todavía no se pueden escanear', 'Informes de escaneo', 'Contacto'],
  en: ['What we count', 'What we do not do', 'Global Privacy Control (GPC)', 'Domains that cannot be scanned yet', 'Scan reports', 'Contact'],
}

describe('/privacy', () => {
  for (const lang of ['es', 'en'] as const) {
    it(`renders every section, with the contact address (${lang})`, async () => {
      renderAt('/privacy', lang)
      expect(await screen.findByRole('heading', { level: 1, name: heading[lang] })).toBeTruthy()
      for (const s of sections[lang]) expect(screen.getByRole('region', { name: s }), s).toBeTruthy()
      await waitFor(() => expect(document.title).toBe(showcaseDict[lang]['privacy.meta.title']))
      expect(screen.getByRole('link', { name: 'soporte@fizsar.com' }).getAttribute('href')).toBe('mailto:soporte@fizsar.com')
    })

    it(`says what the code does (${lang})`, async () => {
      renderAt('/privacy', lang)
      const page = (await screen.findByRole('article')).textContent ?? ''
      // The promises it makes are the ones M1 keeps: GPC, no cookies, 30 days and no backups, beta.
      const facts = {
        es: [/Global Privacy Control/, /No usamos cookies/, /30 días/, /no entran en las copias de seguridad/, /llegará con la beta/, /también con GPC/],
        en: [/Global Privacy Control/, /We use no cookies/, /30 days/, /left out of the backups/, /will come with the beta/, /with GPC too/],
      }[lang]
      for (const re of facts) expect(page, String(re)).toMatch(re)
    })

    it(`says nothing it must not (${lang})`, async () => {
      renderAt('/privacy', lang)
      const page = await screen.findByRole('article')
      const text = strip(page.textContent ?? '')
      for (const re of forbiddenProse) expect(text, String(re)).not.toMatch(re)
      const all = Object.entries(showcaseDict[lang])
        .filter(([k]) => k.startsWith('privacy.'))
        .map(([, v]) => strip(v))
        .join('\n')
      for (const re of forbiddenProse) expect(all, String(re)).not.toMatch(re)
      // Neither a full policy nor promises of what is not built yet.
      expect(all).not.toMatch(/anonimizad|anonymi[sz]ed|RGPD|GDPR|cumpl|complian|nunca registra|never log/i)
    })
  }

  it('is linked from the footer of every page and counted as its own route', async () => {
    renderAt('/', 'es')
    const footer = await screen.findByRole('contentinfo')
    expect(within(footer).getByRole('link', { name: 'Privacidad' }).getAttribute('href')).toBe('/privacy')
    expect(routeOf('/privacy')).toBe('/privacy')
  })

  it('has no inline styles, scripts or handlers (CSP)', async () => {
    const { container } = renderAt('/privacy', 'en')
    await screen.findByRole('heading', { level: 1, name: 'Privacy' })
    expect(container.querySelectorAll('[style], style, script')).toHaveLength(0)
  })
})
