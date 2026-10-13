import { useEffect } from 'react'

/**
 * Sets the document title and, for private pages such as scan results, a
 * robots noindex meta tag (nginx also sends X-Robots-Tag for /scans/).
 * DOM APIs only: compatible with the CSP (no inline scripts).
 */
export function usePageMeta(title: string, noindex = false) {
  useEffect(() => {
    document.title = title
  }, [title])

  useEffect(() => {
    if (!noindex) return
    const meta = document.createElement('meta')
    meta.name = 'robots'
    meta.content = 'noindex, nofollow'
    document.head.appendChild(meta)
    return () => meta.remove()
  }, [noindex])
}
