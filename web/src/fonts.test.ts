import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { cssFiles, read } from './cssFiles'

// Two self-hosted families: Bricolage Grotesque (tokens.css; headings and text)
// and JetBrains Mono (@fontsource; labels, data, code). Both woff2 with
// font-display: swap, and only the face of the first screen and its LCP (the
// sans) preloaded, from the very file its @font-face loads (else it downloads twice).
const tokensCss = read('src/tokens.css')
const faces = (css: string) => [...css.matchAll(/@font-face \{([^}]*)\}/g)].map((m) => m[1])

describe('Bricolage Grotesque (sans)', () => {
  const real = faces(tokensCss).find((f) => /font-family: 'Bricolage Grotesque';/.test(f))
  const fallback = faces(tokensCss).find((f) => /font-family: 'Bricolage Fallback';/.test(f))

  it('one variable woff2, 400–700, swap, from the assets next to its licence', () => {
    expect(real).toMatch(/font-weight: 400 700;/)
    expect(real).toMatch(/font-display: swap;/)
    expect(real).toMatch(/url\('\.\/assets\/fonts\/bricolage-grotesque\.woff2'\) format\('woff2'\)/)
    expect(read('src/assets/fonts/OFL-BricolageGrotesque.txt')).toContain('SIL OPEN FONT LICENSE Version 1.1')
  })

  it('a metric-matched local fallback sits between it and the system font', () => {
    expect(fallback).toMatch(/src: local\('Arial'\)/)
    for (const p of ['size-adjust', 'ascent-override', 'descent-override', 'line-gap-override']) expect(fallback).toMatch(new RegExp(`${p}: [\\d.]+%;`))
    expect(tokensCss).toMatch(/--font-sans: 'Bricolage Grotesque', 'Bricolage Fallback', system-ui, sans-serif;/)
  })

  it('no other stylesheet declares a font face', () => {
    for (const f of cssFiles) if (!f.endsWith('tokens.css')) expect(readFileSync(f, 'utf8')).not.toMatch(/@font-face/)
  })
})

describe('JetBrains Mono (mono)', () => {
  const fontCss = (w: number) => read(`node_modules/@fontsource/jetbrains-mono/${w}.css`)

  it('main.tsx imports the 400 and the 700, before the tokens and the page styles', () => {
    const main = read('src/main.tsx')
    expect([...main.matchAll(/import '@fontsource\/jetbrains-mono\/(\w+)\.css'/g)].map((m) => m[1])).toEqual(['400', '700'])
    expect(main.indexOf("import './tokens.css'")).toBeLessThan(main.indexOf("import './index.css'"))
  })

  it('every face is woff2 with font-display: swap', () => {
    for (const w of [400, 700]) {
      const fs = faces(fontCss(w))
      expect(fs.length).toBeGreaterThan(0)
      for (const f of fs) {
        expect(f).toMatch(/font-display: swap;/)
        expect(f).toMatch(/url\([^)]+\.woff2\) format\('woff2'\)/)
      }
    }
  })
})

describe('which text is mono', () => {
  const indexCss = read('src/index.css')
  const mono = indexCss.match(/((?:[.\w-]+(?: [\w-]+)?,\n)+[.\w-]+(?: [\w-]+)?) \{\n  font-family: var\(--font-mono\);\n\}/)

  it('labels, data, code, fields and the domain pills use the mono', () => {
    const sel = mono?.[1].split(',\n') ?? []
    for (const s of ['.crumbs', '.prompt', 'code', '.label', '.input', '.btn-target', '.lang-btn', '.badge', '.sev', '.host'])
      expect(sel).toContain(s)
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

  it('index.html preloads exactly one font, in CORS mode: the sans file of its @font-face', () => {
    const preloads = [...read('index.html').matchAll(/<link rel="preload"[^>]*>/g)].map((m) => m[0])
    expect(preloads).toHaveLength(1)
    expect(preloads[0]).toMatch(/as="font" type="font\/woff2" crossorigin/)
    expect(preloads[0]).toContain('href="/src/assets/fonts/bricolage-grotesque.woff2"')
  })
})
