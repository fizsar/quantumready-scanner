import { cleanup, render, screen, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { describe, expect, it } from 'vitest'
import type { Lang } from './api'
import App from './App'
import { showcaseDict } from './i18n.showcase'
import { LangProvider } from './LangProvider'
import { stubFetch, json } from './test/fixtures'

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

const heading = { es: 'Arquitectura', en: 'Architecture' }
const sections = {
  es: ['El flujo de un escaneo', 'Motor de escaneo', 'Modelo de puntuación (2026.2)', 'Seguridad del propio servicio', 'Calidad', 'Operación', /En desarrollo/, 'Código'],
  en: ['The path of a scan', 'Scanning engine', 'Scoring model (2026.2)', 'Security of the service itself', 'Quality', 'Operations', /In development/, 'Code'],
}

// What the page must never say: exact software versions, concrete limits,
// proxy settings, bucket names, the server's location or address.
const forbidden: RegExp[] = [
  /\b(Go|Golang|PostgreSQL|Postgres|Caddy|nginx|React|Node|Vite|Trivy|Alpine|Debian|distroless|restic|testssl\.sh)\s*v?\d/i,
  /\bv?\d+\.\d+\.\d+\b/, // any x.y.z
  /TRUSTED_PROXIES|X-Forwarded-For|trusted prox|proxies de confianza/i,
  /fizsar-qr|backblaze|\bb2\b|bucket|eu-central|s3\./i,
  /\bOVH\b|canad|beauharnois|149\.56|\bVPS\b/i,
  /\d+\s*(\/|por |per )\s*(min|minuto|minute|día|day|hora|hour)/i,
  /\b\d+\s*(s|ms|seg|segundos|seconds)\b/i, // timeouts
  /\b(48|60)\b/, // raw probe budget, scan timeout
  /RATE_LIMIT|SCAN_ALLOWLIST|SCAN_INTENSITY|QR_[A-Z]/,
]
// Allowed numbers with dots: protocol versions and the scoring model.
const strip = (s: string) => s.replace(/\b(TLS|SSL) ?v?\d(\.\d)?/g, '').replace(/2026\.2/g, '')

describe('/architecture', () => {
  for (const lang of ['es', 'en'] as const) {
    it(`renders every section (${lang})`, async () => {
      renderAt('/architecture', lang)
      expect(await screen.findByRole('heading', { level: 1, name: heading[lang] })).toBeTruthy()
      for (const s of sections[lang]) expect(screen.getByRole('region', { name: s }), String(s)).toBeTruthy()
      expect(document.title).toBe(showcaseDict[lang]['arch.meta.title'])
      const link = screen.getByRole('link', { name: 'soporte@fizsar.com' })
      expect(link.getAttribute('href')).toBe('mailto:soporte@fizsar.com')
      // The in-development part says so and links nowhere.
      const dev = screen.getByRole('region', { name: sections[lang][6] })
      expect(within(dev).getByText(showcaseDict[lang]['arch.dev.badge'])).toBeTruthy()
      expect(within(dev).queryAllByRole('link')).toHaveLength(0)
    })

    it(`says nothing it must not (${lang})`, async () => {
      renderAt('/architecture', lang)
      const page = await screen.findByRole('article')
      const text = strip(page.textContent ?? '')
      for (const re of forbidden) expect(text, String(re)).not.toMatch(re)
      // Also the meta description and every string of the page.
      const all = Object.entries(showcaseDict[lang])
        .filter(([k]) => k.startsWith('arch.'))
        .map(([, v]) => strip(v))
        .join('\n')
      for (const re of forbidden) expect(all, String(re)).not.toMatch(re)
    })
  }

  it('sets its own description and restores the default one on leaving', async () => {
    const meta = document.createElement('meta')
    meta.name = 'description'
    meta.content = 'default'
    document.head.appendChild(meta)
    try {
      const { unmount } = renderAt('/architecture', 'en')
      await screen.findByRole('heading', { level: 1, name: 'Architecture' })
      expect(meta.content).toBe(showcaseDict.en['arch.meta.description'])
      unmount()
      expect(meta.content).toBe('default')
    } finally {
      meta.remove()
    }
  })

  it('the diagram is an accessible SVG with title and desc', async () => {
    renderAt('/architecture', 'es')
    const svg = await screen.findByRole('img', { name: showcaseDict.es['arch.flow.svgTitle'] })
    const title = svg.querySelector(':scope > title')
    const desc = svg.querySelector(':scope > desc')
    expect(title?.textContent).toBe(showcaseDict.es['arch.flow.svgTitle'])
    expect(desc?.textContent).toBe(showcaseDict.es['arch.flow.svgDesc'])
    expect(svg.getAttribute('aria-labelledby')).toBe(title?.id)
    expect(svg.getAttribute('aria-describedby')).toBe(desc?.id)
    for (const k of ['arch.flow.browser', 'arch.flow.caddy', 'arch.flow.web', 'arch.flow.api', 'arch.flow.db', 'arch.flow.worker', 'arch.flow.scanner', 'arch.flow.target'] as const) {
      expect(within(svg as unknown as HTMLElement).getByText(showcaseDict.es[k]), k).toBeTruthy()
    }
  })

  it('keeps the CSP: no style attributes or elements, no inline handlers', async () => {
    const { container } = renderAt('/architecture', 'en')
    await screen.findByRole('heading', { level: 1, name: 'Architecture' })
    expect(container.querySelectorAll('[style], style, script')).toHaveLength(0)
    const handlers = [...container.querySelectorAll('*')].flatMap((el) => [...el.attributes].filter((a) => a.name.startsWith('on')))
    expect(handlers).toHaveLength(0)
  })

  it('every internal link leads to a page that exists', async () => {
    const { container } = renderAt('/architecture', 'es')
    await screen.findByRole('heading', { level: 1, name: 'Arquitectura' })
    const hrefs = [...container.querySelectorAll('a[href]')].map((a) => a.getAttribute('href')!)
    expect(hrefs).toContain('/architecture') // footer link
    const internal = [...new Set(hrefs.filter((h) => h.startsWith('/')))]
    for (const h of hrefs.filter((h) => !h.startsWith('/') && !h.startsWith('#'))) expect(h, h).toMatch(/^(https:\/\/fizsar\.com\/?|mailto:soporte@fizsar\.com)$/)
    for (const h of internal) {
      cleanup()
      renderAt(h, 'es')
      expect(screen.queryByRole('heading', { name: 'Esta página no existe' }), h).toBeNull()
    }
  })

  it('is linked from the footer of every page', async () => {
    renderAt('/', 'en')
    const footer = await screen.findByRole('contentinfo')
    expect(within(footer).getByRole('link', { name: 'Architecture' }).getAttribute('href')).toBe('/architecture')
  })
})
