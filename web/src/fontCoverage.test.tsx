/// <reference types="node" />
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { screen, waitFor } from '@testing-library/react'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import type { Lang } from './api'
import { cssFiles } from './cssFiles'
import { sentence, verdict, X_MAX, Y_MAX } from './lab/story/mosca'
import { familyCoverage } from './test/fontCoverage'
import { json, stubFetch } from './test/fixtures'
import { renderAt } from './test/render'

// Every character of the visible texts must be painted by the font of its context,
// never by a system font picked as a fallback (a change of style mid-line):
// - inside <math>: MathML, whose math font is the system's (STIX Two Math on Apple,
//   Cambria Math on Windows…): allowed there and only there;
// - where a mono rule of the stylesheets applies (`.mono` marks it on purpose):
//   JetBrains Mono, as self-hosted (tokens.css);
// - anywhere else: Bricolage Grotesque, as self-hosted (tokens.css).
// A ψ in running text is a failure until it is marked (<math> or .mono).

// Not drawn (default-ignorable or spaces the layout handles): no glyph needed.
const IGNORABLE = new Set([0x09, 0x0a, 0x0d, 0x20, 0xad, 0x200b, 0x200c, 0x200d, 0x200e, 0x200f, 0x2060, 0x2061, 0x2062, 0x2063, 0x2064, 0xfe0f])

const monoSelectors = cssFiles
  .flatMap((f) => [...readFileSync(f, 'utf8').replace(/\/\*[\s\S]*?\*\//g, '').matchAll(/([^{}@]+)\{([^{}]*)\}/g)])
  .filter(([, , body]) => /font-family:\s*var\(--(font-mono|mono-font)\)/.test(body))
  .map(([, sel]) => sel.trim())
  .join(', ')

let sans400: (cp: number) => boolean, sans700: (cp: number) => boolean, mono400: (cp: number) => boolean, mono700: (cp: number) => boolean
beforeAll(() => {
  sans400 = familyCoverage(resolve('src/tokens.css'), 'Bricolage Grotesque', 400)
  sans700 = familyCoverage(resolve('src/tokens.css'), 'Bricolage Grotesque', 700)
  mono400 = familyCoverage(resolve('src/tokens.css'), 'JetBrains Mono', 400)
  mono700 = familyCoverage(resolve('src/tokens.css'), 'JetBrains Mono', 700)
})

type Miss = { ch: string; cp: string; font: string; where: string }
const hex = (cp: number) => 'U+' + cp.toString(16).toUpperCase().padStart(4, '0')

/** Every character of the rendered page that its context's font does not cover. */
function misses(root: Element): Miss[] {
  const out = new Map<string, Miss>()
  const walk = document.createTreeWalker(root, NodeFilter.SHOW_TEXT)
  for (let n = walk.nextNode(); n; n = walk.nextNode()) {
    const el = n.parentElement
    if (!el || el.closest('script, style, math')) continue
    const mono = !!monoSelectors && !!el.closest(monoSelectors)
    for (const ch of n.textContent ?? '') {
      const cp = ch.codePointAt(0)!
      if (IGNORABLE.has(cp)) continue
      const ok = mono ? mono400(cp) && mono700(cp) : sans400(cp) && sans700(cp)
      if (!ok) {
        const key = `${cp}:${mono}`
        if (!out.has(key)) out.set(key, { ch, cp: hex(cp), font: mono ? 'JetBrains Mono' : 'Bricolage Grotesque', where: `${el.tagName.toLowerCase()}.${el.className}: "${(n.textContent ?? '').trim().slice(0, 50)}"` })
      }
    }
  }
  return [...out.values()]
}

afterEach(() => vi.restoreAllMocks())

describe('the fonts read correctly', () => {
  it('Bricolage covers Latin text and the operators of the texts, not Greek; the mono covers Greek', () => {
    for (const c of 'Áéíóúñ¿¡«»—–’€→≤≥×') expect(sans400(c.codePointAt(0)!), c).toBe(true)
    expect(sans400('ψ'.codePointAt(0)!)).toBe(false)
    expect(mono400('ψ'.codePointAt(0)!)).toBe(true)
  })
  it('the mono rules were found in the stylesheets (code, the labels, the story data)', () => {
    for (const s of ['code', '.label', '.story-kicker', '.mono']) expect(monoSelectors.split(/,\s*/)).toContain(s)
  })
})

const PAGES = ['/', '/examples', '/architecture', '/privacy', '/about-scans', '/lab/historia']

describe.each(['es', 'en'] as Lang[])('every character has its font (%s)', (lang) => {
  // Whole pages (/examples renders every report): more than the default 5 s when the suite runs in parallel.
  for (const path of PAGES)
    it(path, { timeout: 20_000 }, async () => {
      // Still images, no 3D (the engine is not needed to read the texts).
      vi.stubGlobal('matchMedia', (q: string) => ({ matches: q === '(prefers-reduced-motion: reduce)', media: q }))
      stubFetch(() => json({ hosts: ['fizsar.com', 'quantumready.fizsar.com'], port: 443, open: true, proof: true }))
      renderAt(path, lang)
      await screen.findAllByRole('heading', { level: 1 })
      await waitFor(() => expect(document.querySelector('main')!.textContent!.length).toBeGreaterThan(200))
      const m = misses(document.body)
      if (process.env.COV_DUMP) for (const x of m) console.log(`MISS ${x.cp} ${x.ch} [${x.font}] ${x.where}`)
      expect(m).toEqual([])
    })

  it("the slider's every sentence (scene 6), in running text", () => {
    const sentences = new Set<string>()
    for (let x = 0; x <= X_MAX; x++) for (let y = 0; y <= Y_MAX; y++) sentences.add(sentence(verdict(x, y), lang))
    const p = document.createElement('p')
    p.textContent = [...sentences].join(' ')
    document.body.append(p)
    expect(misses(p)).toEqual([])
    p.remove()
  })
})
