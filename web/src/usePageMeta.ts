import { useContext, useEffect } from 'react'
import { LangContext } from './i18n'

/** The public origin: canonical URLs always point here (also from previews). */
export const SITE = 'https://quantumready.fizsar.com'

/** The two languages; a page's URL in each is the same path with ?lang=es or ?lang=en. */
const LANGS = ['es', 'en'] as const
type Lang = (typeof LANGS)[number]

/** The share image for a language (1200 × 630): English is og.png (index.html's own), Spanish og-es.png. */
export const ogImage = (lang: Lang) => SITE + (lang === 'es' ? '/og-es.png' : '/og.png')

/** The page's language: the app's (LangContext), else the one on <html>, else English. */
const pageLang = (ctx: Lang | undefined): Lang => ctx ?? (document.documentElement.lang === 'es' ? 'es' : 'en')

/** A path's URL in a language (canonical and hreflang). */
export const langURL = (path: string, lang: Lang) => `${SITE}${path}?lang=${lang}`

/** Finds or creates a <link>/<meta> in <head> and sets one attribute; returns the element. */
function headTag<K extends 'link' | 'meta'>(tag: K, selector: string, init: (el: HTMLElementTagNameMap[K]) => void): HTMLElementTagNameMap[K] {
  let el = document.head.querySelector<HTMLElementTagNameMap[K]>(`${tag}${selector}`)
  if (!el) {
    el = document.createElement(tag)
    init(el)
    document.head.appendChild(el)
  }
  return el
}

/**
 * Per page: the document title, its description, the canonical link (the page in its
 * language: ?lang=es or ?lang=en), hreflang alternates for both languages plus
 * x-default (the path alone: the app picks the visitor's language), and the Open
 * Graph / Twitter tags (title, description, URL, locale, image in the page's
 * language). Private pages (a scan's result, accounts) add a robots noindex meta tag
 * (nginx also sends X-Robots-Tag for /scans/ and the account routes); every other
 * page is indexable. DOM APIs only: compatible with the CSP (no inline scripts).
 */
export function usePageMeta(title: string, noindex = false, description?: string) {
  // Read here, not from <html>: the provider sets <html lang> in its own effect, after its children's.
  const ctxLang = useContext(LangContext)?.lang
  useEffect(() => {
    document.title = title
    for (const [attr, key] of [
      ['property', 'og:title'],
      ['name', 'twitter:title'],
    ] as const) {
      const m = document.head.querySelector<HTMLMetaElement>(`meta[${attr}="${key}"]`)
      if (m) m.content = title
    }
  }, [title])

  useEffect(() => {
    if (!description) return
    const metas = (['meta[name="description"]', 'meta[property="og:description"]', 'meta[name="twitter:description"]'] as const)
      .map((s) => document.head.querySelector<HTMLMetaElement>(s))
      .filter((m): m is HTMLMetaElement => !!m)
    const before = metas.map((m) => m.content)
    for (const m of metas) m.content = description
    return () => metas.forEach((m, i) => (m.content = before[i]))
  }, [description])

  // Every render: the path or the language may have changed.
  useEffect(() => {
    const path = window.location.pathname
    const lang = pageLang(ctxLang)
    const canonical = document.head.querySelector<HTMLLinkElement>('link[rel="canonical"]')
    if (canonical) canonical.href = langURL(path, lang)
    for (const l of LANGS) {
      headTag('link', `[rel="alternate"][hreflang="${l}"]`, (el) => {
        el.rel = 'alternate'
        el.hreflang = l
      }).href = langURL(path, l)
    }
    headTag('link', '[rel="alternate"][hreflang="x-default"]', (el) => {
      el.rel = 'alternate'
      el.hreflang = 'x-default'
    }).href = SITE + path
    const set = (selector: string, value: string) => {
      const m = document.head.querySelector<HTMLMetaElement>(selector)
      if (m) m.content = value
    }
    set('meta[property="og:url"]', langURL(path, lang))
    set('meta[property="og:locale"]', lang === 'es' ? 'es_ES' : 'en_GB')
    set('meta[property="og:locale:alternate"]', lang === 'es' ? 'en_GB' : 'es_ES')
    set('meta[property="og:image"]', ogImage(lang))
    set('meta[name="twitter:image"]', ogImage(lang))
  })

  useEffect(() => {
    if (!noindex) return
    const meta = document.createElement('meta')
    meta.name = 'robots'
    meta.content = 'noindex, nofollow'
    document.head.appendChild(meta)
    return () => meta.remove()
  }, [noindex])
}
