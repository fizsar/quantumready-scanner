import { useEffect } from 'react'

/** The public origin: canonical URLs always point here (also from previews). */
export const SITE = 'https://quantumready.fizsar.com'

/**
 * Sets the document title, points the canonical link (index.html) at the
 * current page and, for private pages such as scan results, adds a robots
 * noindex meta tag (nginx also sends X-Robots-Tag for /scans/).
 * DOM APIs only: compatible with the CSP (no inline scripts).
 */
export function usePageMeta(title: string, noindex = false) {
  useEffect(() => {
    document.title = title
  }, [title])

  useEffect(() => {
    const link = document.head.querySelector<HTMLLinkElement>('link[rel="canonical"]')
    if (link) link.href = SITE + window.location.pathname
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
