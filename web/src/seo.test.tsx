/// <reference types="vite/client" />
import { render } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import ogPng from '../public/og.png?inline'
import security from '../public/.well-known/security.txt?raw'
import robots from '../public/robots.txt?raw'
import sitemap from '../public/sitemap.xml?raw'
import indexHtml from '../index.html?raw'
import { SITE, usePageMeta } from './usePageMeta'

const page = new DOMParser().parseFromString(indexHtml, 'text/html')
const meta = (attr: 'name' | 'property', key: string) => page.head.querySelector(`meta[${attr}="${key}"]`)?.getAttribute('content') ?? ''

describe('index.html', () => {
  it('has a real title, description and canonical', () => {
    expect(page.title).toBe('QuantumReady · Post-quantum audit of your website')
    expect(meta('name', 'description').length).toBeGreaterThan(80)
    expect(page.head.querySelector('link[rel="canonical"]')?.getAttribute('href')).toBe(SITE + '/')
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
    expect(data.creator).toEqual({ '@type': 'Person', name: 'fizsar', url: 'https://fizsar.com/' })
  })

  it('keeps the CSP: the only inline script is the JSON-LD data block', () => {
    const inline = [...page.querySelectorAll('script:not([src])')]
    expect(inline.map((s) => s.getAttribute('type'))).toEqual(['application/ld+json'])
    expect(page.querySelectorAll('[style], [onload], [onclick]')).toHaveLength(0)
  })
})

describe('og.png', () => {
  it('is a 1200×630 PNG', () => {
    const bytes = Uint8Array.from(atob(ogPng.split(',')[1]), (c) => c.charCodeAt(0))
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
    expect([...doc.getElementsByTagName('loc')].map((l) => l.textContent)).toEqual([SITE + '/', SITE + '/architecture', SITE + '/examples', SITE + '/privacy'])
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

  it('follows the current page', () => {
    const link = document.createElement('link')
    link.rel = 'canonical'
    link.href = SITE + '/'
    document.head.appendChild(link)
    window.history.replaceState(null, '', '/architecture?lang=en#x')
    render(<Page />)
    expect(link.href).toBe(SITE + '/architecture')
  })
})
