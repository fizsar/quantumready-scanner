import { readFileSync, statSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import { cssFiles, read } from './cssFiles'

// Two self-hosted families in tokens.css: Bricolage Grotesque (headings and
// text) and JetBrains Mono (labels, data, code), split by unicode-range, woff2
// with font-display: swap, behind metric-matched fallbacks, and one preload: the
// face that paints the LCP. Which characters each face covers:
// fontCoverage.test.tsx.
const tokensCss = read('src/tokens.css')
const faces = [...tokensCss.matchAll(/@font-face \{([^}]*)\}/g)].map((m) => m[1])
const family = (f: string) => f.match(/font-family: '([^']+)'/)?.[1]
const url = (f: string) => f.match(/url\('\.\/([^']+\.woff2)'\)/)?.[1]
const real = faces.filter((f) => /^(Bricolage Grotesque|JetBrains Mono)$/.test(family(f) ?? ''))
const fallbacks = faces.filter((f) => /Fallback/.test(family(f) ?? ''))
const KB = (f: string) => statSync(resolve('src', url(f)!)).size / 1024

describe('self-hosted faces', () => {
  it('each family has 400 and 700, every face a woff2 with swap and a unicode-range, next to its licence', () => {
    for (const fam of ['Bricolage Grotesque', 'JetBrains Mono']) {
      const fs = real.filter((f) => family(f) === fam)
      expect(new Set(fs.map((f) => f.match(/font-weight: (\d+);/)?.[1]))).toEqual(new Set(['400', '700']))
      for (const f of fs) {
        expect(f).toMatch(/font-display: swap;/)
        expect(url(f)).toMatch(/^assets\/fonts\/[\w-]+\.woff2$/)
        expect(f).toMatch(/unicode-range: U\+/)
      }
    }
    expect(read('src/assets/fonts/OFL-BricolageGrotesque.txt')).toContain('SIL OPEN FONT LICENSE Version 1.1')
    expect(read('src/assets/fonts/OFL-JetBrainsMono.txt')).toContain('SIL OPEN FONT LICENSE Version 1.1')
  })

  it('no face is heavier than 45 KB; the preloaded one stays under 25 KB', () => {
    for (const f of real) expect(KB(f), url(f)).toBeLessThan(45)
    expect(KB(real.find((f) => url(f) === 'assets/fonts/bricolage-400-latin.woff2')!)).toBeLessThan(25)
  })

  it('nothing else declares a font face, and @fontsource is no longer loaded by the app', () => {
    for (const f of cssFiles) if (!f.endsWith('tokens.css')) expect(readFileSync(f, 'utf8')).not.toMatch(/@font-face/)
    expect(read('src/main.tsx')).not.toMatch(/@fontsource/)
    expect(read('src/main.tsx').indexOf("import './tokens.css'")).toBeLessThan(read('src/main.tsx').indexOf("import './index.css'"))
  })
})

describe('fallbacks', () => {
  it('every fallback is a local font with the four metric overrides', () => {
    expect(fallbacks.length).toBeGreaterThanOrEqual(5)
    for (const f of fallbacks) {
      expect(f).toMatch(/src: local\('/)
      for (const p of ['size-adjust', 'ascent-override', 'descent-override', 'line-gap-override']) expect(f).toMatch(new RegExp(`${p}: [\\d.]+%;`))
    }
  })

  it('each stack puts its fallbacks between the real face and the generic one', () => {
    const stack = (v: string) => tokensCss.match(new RegExp(`--${v}: ([^;]+);`))![1].split(/,\s*/).map((s) => s.replace(/'/g, ''))
    const sans = stack('font-sans')
    const mono = stack('font-mono')
    expect(sans[0]).toBe('Bricolage Grotesque')
    expect(mono[0]).toBe('JetBrains Mono')
    for (const f of fallbacks) expect([...sans, ...mono]).toContain(family(f))
    expect(sans.at(-1)).toBe('sans-serif')
    expect(mono.at(-1)).toBe('monospace')
  })
})

describe('which text is mono', () => {
  const indexCss = read('src/index.css')
  const mono = indexCss.match(/((?:[.\w-]+(?: [\w-]+)?,\n)+[.\w-]+(?: [\w-]+)?) \{\n  font-family: var\(--font-mono\);\n\}/)

  it('labels, data, code, fields, the domain pills and the .mono mark use the mono', () => {
    const sel = mono?.[1].split(',\n') ?? []
    for (const s of ['.mono', '.crumbs', '.prompt', 'code', '.label', '.input', '.btn-target', '.lang-btn', '.badge', '.sev', '.host']) expect(sel).toContain(s)
  })

  it('that rule comes after every `font: inherit` (same specificity: the later one wins)', () => {
    expect(mono).not.toBeNull()
    expect(indexCss.indexOf(mono![0])).toBeGreaterThan(indexCss.lastIndexOf('font: inherit'))
  })
})

describe('weights and preload', () => {
  it('the only weights used are 400 and 700 (both families have them)', () => {
    const used = new Set(cssFiles.flatMap((f) => [...readFileSync(f, 'utf8').matchAll(/font-weight:\s*(\w+)/g)].map((m) => m[1])))
    for (const w of used) expect(['400', '700', 'normal', 'bold']).toContain(w)
  })

  it('index.html preloads exactly one font, in CORS mode: Bricolage 400 latin, the file of its @font-face', () => {
    const preloads = [...read('index.html').matchAll(/<link rel="preload"[^>]*>/g)].map((m) => m[0])
    expect(preloads).toHaveLength(1)
    expect(preloads[0]).toMatch(/as="font" type="font\/woff2" crossorigin/)
    expect(preloads[0]).toContain('href="/src/assets/fonts/bricolage-400-latin.woff2"')
    expect(real.some((f) => family(f) === 'Bricolage Grotesque' && /font-weight: 400;/.test(f) && url(f) === 'assets/fonts/bricolage-400-latin.woff2')).toBe(true)
  })
})
