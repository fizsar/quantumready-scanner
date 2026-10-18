import { cleanup, render, screen, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { describe, expect, it } from 'vitest'
import type { Lang } from './api'
import App from './App'
import { uthAnchor } from './exampleAnchors'
import { EXAMPLES } from './examples'
import { showcaseDict } from './i18n.showcase'
import { LangProvider } from './LangProvider'
import { json, stubFetch } from './test/fixtures'
import { forbiddenEverywhere, forbiddenProse, strip, stripData } from './test/forbidden'

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

// The grades the page explains. The Go test TestLabFixtures pins the same
// numbers against the current scoring model and checks the fixtures are what
// the API returns now, so a model change fails there and here.
const shown = { fizsar: ['A', 95], modern: ['D', 40], kyber: ['D', 40], export: ['F', 15] } as const
const labLabel = { es: 'Servidor de laboratorio aislado; no es un sitio real', en: 'Isolated lab server; not a real website' }

describe('/examples', () => {
  it('has the four examples, best grade first', () => {
    expect(EXAMPLES.map((e) => e.key)).toEqual(['fizsar', 'modern', 'kyber', 'export'])
    expect(EXAMPLES.map((e) => e.lab)).toEqual([false, true, true, true])
  })

  for (const lang of ['es', 'en'] as const) {
    it(`renders every example with its grade (${lang})`, async () => {
      renderAt('/examples', lang)
      expect(await screen.findByRole('heading', { level: 1, name: showcaseDict[lang]['examples.title'] })).toBeTruthy()
      expect(document.title).toBe(showcaseDict[lang]['examples.meta.title'])
      for (const { key, lab, scans } of EXAMPLES) {
        const scan = scans[lang]
        const [grade, score] = shown[key]
        expect([scan.report!.grade, scan.report!.score], key).toEqual([grade, score])
        const section = document.getElementById(key)!
        expect(section, key).not.toBeNull()
        expect(within(section).getByRole('heading', { level: 2, name: `${scan.host} · ${grade} · ${score}` })).toBeTruthy()
        // The report itself, with the same component as a scan.
        expect(within(section).getByText(lang === 'es' ? `Nota: ${grade}` : `Grade: ${grade}`)).toBeTruthy()
        expect(within(section).getByText(lang === 'es' ? `${score} de 100 puntos` : `${score} out of 100 points`)).toBeTruthy()
        for (const part of ['config', 'why', 'fix'] as const) {
          expect(within(section).getByText(showcaseDict[lang][`examples.${key}.${part}`]), `${key}.${part}`).toBeTruthy()
        }
        // Lab servers carry the label; the real scan does not.
        expect(within(section).queryByText(labLabel[lang]) !== null, key).toBe(lab)
        // A lab certificate is no CA's: no Let's Encrypt, no automated renewal claimed.
        if (lab) expect(section.textContent, key).not.toMatch(/Let.s Encrypt/)
        // Each panel has its own anchor.
        expect(section.querySelector(`#${uthAnchor(key)}`), key).not.toBeNull()
      }
    })
  }

  // kyber finished at 22:04:37 UTC and fizsar.com at 11:47:58 UTC: near
  // midnight in some zone, so header and footer must use the same one (the
  // visitor's). TZ changes at runtime reach Date and Intl in Node.
  for (const [zone, kyberDay, fizsarDay] of [
    ['UTC', '14 October 2026', '17 October 2026'],
    ['Pacific/Kiritimati', '15 October 2026', '18 October 2026'], // UTC+14
    ['America/Los_Angeles', '14 October 2026', '17 October 2026'],
  ] as const) {
    it(`header and report footer give the same day (${zone})`, async () => {
      const before = process.env.TZ
      process.env.TZ = zone
      try {
        renderAt('/examples', 'en')
        await screen.findByRole('heading', { level: 1, name: 'Examples' })
        for (const [key, day, header] of [
          ['kyber', kyberDay, `Scanned in the lab on ${kyberDay}`],
          ['fizsar', fizsarDay, `Real scan of ${fizsarDay}`],
        ] as const) {
          const section = document.getElementById(key)!
          expect(within(section).getByText(header), key).toBeTruthy()
          const footer = section.querySelector('.report-footer')!.textContent ?? ''
          expect(footer, `${key} footer`).toContain(`Scanned on ${day} at `)
        }
      } finally {
        process.env.TZ = before
      }
    })
  }

  it('the lab hosts are reserved names that never resolve (RFC 6761 .test)', () => {
    for (const { lab, scans } of EXAMPLES.filter((e) => e.lab)) {
      expect(lab).toBe(true)
      expect(scans.es.host).toMatch(/^[a-z0-9-]+\.tlslab\.test$/)
    }
  })

  it('#<example>-under-the-hood opens that panel only', async () => {
    renderAt(`/examples#${uthAnchor('fizsar')}`, 'en')
    await screen.findByRole('heading', { level: 1, name: 'Examples' })
    const toggles = screen.getAllByRole('button', { name: 'Under the hood' })
    expect(toggles.map((b) => b.getAttribute('aria-expanded'))).toEqual(['true', 'false', 'false', 'false'])
    expect(toggles[0].closest('section')?.id).toBe('fizsar-under-the-hood')
  })

  it('every internal link leads to a page or anchor that exists', async () => {
    const { container } = renderAt('/examples', 'es')
    await screen.findByRole('heading', { level: 1, name: 'Ejemplos' })
    const hrefs = [...container.querySelectorAll('a[href]')].map((a) => a.getAttribute('href')!)
    for (const h of hrefs.filter((h) => h.startsWith('#'))) expect(document.getElementById(h.slice(1)), h).not.toBeNull()
    expect(hrefs).toEqual(expect.arrayContaining(['#fizsar', '#modern', '#kyber', '#export', '#fizsar-under-the-hood', '#modern-under-the-hood']))
    for (const h of hrefs.filter((h) => !h.startsWith('/') && !h.startsWith('#'))) expect(h, h).toMatch(/^(https:\/\/fizsar\.com\/?|mailto:soporte@fizsar\.com)$/)
    for (const h of [...new Set(hrefs.filter((h) => h.startsWith('/')))]) {
      cleanup()
      renderAt(h, 'es')
      const h1 = await screen.findByRole('heading', { level: 1 })
      expect(h1.textContent, h).not.toBe('Esta página no existe')
    }
  })

  for (const lang of ['es', 'en'] as const) {
    it(`says nothing it must not (${lang})`, async () => {
      renderAt('/examples', lang)
      await screen.findByRole('heading', { level: 1 })
      // The page's own words: every string and the intro panels as rendered.
      const prose = [
        ...Object.entries(showcaseDict[lang])
          .filter(([k]) => k.startsWith('examples.') || k.startsWith('links.'))
          .map(([, v]) => v),
        ...[...document.querySelectorAll('.examples > .panel, .example-intro')].map((e) => e.textContent ?? ''),
      ]
        .map(strip)
        .join('\n')
      for (const re of forbiddenProse) expect(prose, String(re)).not.toMatch(re)
      // The reports (API data, panels included even if closed).
      const data = stripData([...document.querySelectorAll('.example .report')].map((e) => e.textContent ?? '').join('\n'))
      for (const re of forbiddenEverywhere) expect(data, String(re)).not.toMatch(re)
    })
  }

  it('is linked from the home page, discreetly, next to Architecture', async () => {
    renderAt('/', 'es')
    const nav = await screen.findByRole('navigation', { name: 'Más sobre QuantumReady' })
    expect(within(nav).getByRole('link', { name: 'Ejemplos' }).getAttribute('href')).toBe('/examples')
    expect(within(nav).getByRole('link', { name: 'Arquitectura' }).getAttribute('href')).toBe('/architecture')
  })
})
