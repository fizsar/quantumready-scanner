/// <reference types="vite/client" />
import { render } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import ogPng from '../public/og.png?inline'
import ogEsPng from '../public/og-es.png?inline'
import security from '../public/.well-known/security.txt?raw'
import robots from '../public/robots.txt?raw'
import sitemap from '../public/sitemap.xml?raw'
import indexHtml from '../index.html?raw'
import { LangContext, translate } from './i18n'
import { ogImage, SITE, usePageMeta } from './usePageMeta'

const page = new DOMParser().parseFromString(indexHtml, 'text/html')
const meta = (attr: 'name' | 'property', key: string) => page.head.querySelector(`meta[${attr}="${key}"]`)?.getAttribute('content') ?? ''

describe('index.html', () => {
  it('has a real title, description and canonical', () => {
    expect(page.title).toBe('QuantumReady · Post-quantum TLS scanner (ML-KEM, X25519MLKEM768)')
    expect(meta('name', 'description').length).toBeGreaterThan(80)
    expect(page.head.querySelector('link[rel="canonical"]')?.getAttribute('href')).toBe(SITE + '/')
    const alt = (l: string) => page.head.querySelector(`link[rel="alternate"][hreflang="${l}"]`)?.getAttribute('href')
    expect([alt('es'), alt('en'), alt('x-default')]).toEqual([SITE + '/?lang=es', SITE + '/?lang=en', SITE + '/'])
    expect(page.documentElement.lang).toBe('en') // the metadata language; the app switches it on load
  })

  it('has Open Graph and Twitter Card tags pointing at the 1200×630 image', () => {
    for (const [key, want] of [
      ['og:type', 'website'],
      ['og:url', SITE + '/'],
      ['og:image', SITE + '/og.png'],
      ['og:image:width', '1200'],
      ['og:image:height', '630'],
      ['og:locale', 'en_GB'],
      ['og:locale:alternate', 'es_ES'],
    ]) {
      expect(meta('property', key), key).toBe(want)
    }
    for (const key of ['og:title', 'og:description', 'og:image:alt', 'og:site_name']) expect(meta('property', key), key).not.toBe('')
    expect(meta('name', 'twitter:card')).toBe('summary_large_image')
    expect(meta('name', 'twitter:image')).toBe(SITE + '/og.png')
    for (const key of ['twitter:title', 'twitter:description', 'twitter:image:alt']) expect(meta('name', key), key).not.toBe('')
  })

  it('claims nothing it cannot back', () => {
    // "certificate" only ever as the website's TLS certificate, never as a seal or a promise.
    expect(indexHtml).not.toMatch(/certified|certifies|guarantee|warrant|certificad|garantiz/i)
    expect(indexHtml).not.toMatch(/(?<!TLS )certificate/i)
  })

  it('has valid JSON-LD for a WebApplication', () => {
    const blocks = page.querySelectorAll('script[type="application/ld+json"]')
    expect(blocks).toHaveLength(1)
    const data = JSON.parse(blocks[0].textContent ?? '')
    expect(data['@context']).toBe('https://schema.org')
    expect(data['@type']).toBe('WebApplication')
    expect(data.name).toBe('QuantumReady')
    expect(data.url).toBe(SITE + '/')
    expect(data.inLanguage).toEqual(['en', 'es'])
    expect(data.applicationCategory).toBe('SecurityApplication')
    expect(data.isAccessibleForFree).toBe(true)
    expect(data.offers).toEqual({ '@type': 'Offer', price: '0', priceCurrency: 'EUR' })
    expect(data.author).toEqual({ '@type': 'Person', name: 'Fiz Sar López', url: 'https://fizsar.com/' })
    expect(data.isBasedOn.codeRepository).toBe('https://github.com/fizsar/quantumready-scanner')
  })

  it('keeps the CSP: the only inline script is the JSON-LD data block', () => {
    const inline = [...page.querySelectorAll('script:not([src])')]
    expect(inline.map((s) => s.getAttribute('type'))).toEqual(['application/ld+json'])
    expect(page.querySelectorAll('[style], [onload], [onclick]')).toHaveLength(0)
  })
})

describe('og.png and og-es.png', () => {
  it.each([
    ['og.png (English)', ogPng],
    ['og-es.png (Spanish)', ogEsPng],
  ])('%s is a 1200×630 PNG', (_name, png) => {
    const bytes = Uint8Array.from(atob(png.split(',')[1]), (c) => c.charCodeAt(0))
    expect(Array.from(bytes.slice(1, 4), (b) => String.fromCharCode(b)).join('')).toBe('PNG')
    const view = new DataView(bytes.buffer)
    expect([view.getUint32(16), view.getUint32(20)]).toEqual([1200, 630])
    expect(bytes.length).toBeLessThan(300_000) // light enough for link previews
  })
})

describe('robots.txt and sitemap.xml', () => {
  it('robots.txt only keeps crawlers out of the API, and names the sitemap', () => {
    expect(robots).toBe(`User-agent: *\nDisallow: /api/\n\nSitemap: ${SITE}/sitemap.xml\n`)
    expect(robots).not.toMatch(/scans/) // scan results carry noindex, which crawlers must be able to read
  })

  it('sitemap.xml lists the public pages', () => {
    const doc = new DOMParser().parseFromString(sitemap, 'application/xml')
    expect(doc.querySelector('parsererror')).toBeNull()
    const pages = ['/', '/historia', '/architecture', '/examples', '/about-scans', '/privacy']
    // Each public page in both languages, with its hreflang alternates and x-default.
    expect([...doc.getElementsByTagName('loc')].map((l) => l.textContent)).toEqual(pages.flatMap((p) => [`${SITE}${p}?lang=es`, `${SITE}${p}?lang=en`]))
    for (const url of doc.getElementsByTagName('url')) {
      const links = [...url.getElementsByTagNameNS('http://www.w3.org/1999/xhtml', 'link')].map((l) => `${l.getAttribute('hreflang')} ${l.getAttribute('href')}`)
      const path = new URL(url.getElementsByTagName('loc')[0].textContent!).pathname
      expect(links).toEqual([`es ${SITE}${path}?lang=es`, `en ${SITE}${path}?lang=en`, `x-default ${SITE}${path}`])
    }
  })
})

describe('security.txt', () => {
  const field = (name: string) =>
    security
      .split('\n')
      .filter((l: string) => l.startsWith(name + ': '))
      .map((l: string) => l.slice(name.length + 2))

  it('has the required fields once', () => {
    expect(field('Contact')).toEqual(['mailto:soporte@fizsar.com'])
    expect(field('Expires')).toHaveLength(1)
    expect(field('Preferred-Languages')).toEqual(['es, en'])
    expect(field('Canonical')).toEqual([SITE + '/.well-known/security.txt'])
  })

  it('does not expire within 30 days, nor more than a year ahead', () => {
    const left = Date.parse(field('Expires')[0]) - Date.now()
    const day = 24 * 3600 * 1000
    expect(left).toBeGreaterThan(30 * day)
    expect(left).toBeLessThan(366 * day)
  })
})

describe('canonical link', () => {
  function Page() {
    usePageMeta('x')
    return null
  }
  afterEach(() => {
    document.head.querySelector('link[rel="canonical"]')?.remove()
    window.history.replaceState(null, '', '/')
  })

  it('follows the current page, in its language, with hreflang alternates', () => {
    const link = document.createElement('link')
    link.rel = 'canonical'
    link.href = SITE + '/'
    document.head.appendChild(link)
    window.history.replaceState(null, '', '/architecture?lang=en#x')
    render(<Page />)
    expect(link.href).toBe(SITE + '/architecture?lang=en')
    const alt = (l: string) => document.head.querySelector<HTMLLinkElement>(`link[rel="alternate"][hreflang="${l}"]`)?.href
    expect([alt('es'), alt('en'), alt('x-default')]).toEqual([SITE + '/architecture?lang=es', SITE + '/architecture?lang=en', SITE + '/architecture'])
  })

  it('in Spanish: the Spanish URL as canonical and the Spanish share image', () => {
    const link = document.createElement('link')
    link.rel = 'canonical'
    document.head.appendChild(link)
    const og = document.createElement('meta')
    og.setAttribute('property', 'og:image')
    document.head.appendChild(og)
    window.history.replaceState(null, '', '/historia')
    const t = (k: Parameters<typeof translate>[1]) => translate('es', k)
    render(
      <LangContext.Provider value={{ lang: 'es', setLang: () => {}, t }}>
        <Page />
      </LangContext.Provider>,
    )
    expect(link.href).toBe(SITE + '/historia?lang=es')
    expect(og.content).toBe(ogImage('es'))
    expect(ogImage('es')).toBe(SITE + '/og-es.png')
    og.remove()
  })
})
