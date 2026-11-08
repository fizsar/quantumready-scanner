import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { cssFiles, read } from './cssFiles'

// JetBrains Mono, self-hosted from @fontsource: the two weights in use, woff2 with
// font-display: swap, and only the face of the first screen (Latin 400) preloaded,
// from the very file its @font-face loads (else the browser downloads it twice).
describe('self-hosted font', () => {
  const mainTsx = read('src/main.tsx')
  const fontCss = (w: number) => read(`node_modules/@fontsource/jetbrains-mono/${w}.css`)
  const preloads = [...read('index.html').matchAll(/<link rel="preload"[^>]*>/g)].map((m) => m[0])

  it('main.tsx imports the 400 and the 700, and nothing else declares the family', () => {
    expect([...mainTsx.matchAll(/import '@fontsource\/jetbrains-mono\/(\w+)\.css'/g)].map((m) => m[1])).toEqual(['400', '700'])
    for (const f of cssFiles) expect(readFileSync(f, 'utf8')).not.toMatch(/@font-face/)
  })

  it('the only weights used are those two', () => {
    const used = new Set(cssFiles.flatMap((f) => [...readFileSync(f, 'utf8').matchAll(/font-weight:\s*(\w+)/g)].map((m) => m[1])))
    for (const w of used) expect(['400', '700', 'normal', 'bold']).toContain(w)
  })

  it('every face is woff2 with font-display: swap', () => {
    for (const w of [400, 700]) {
      const faces = [...fontCss(w).matchAll(/@font-face \{([^}]*)\}/g)].map((m) => m[1])
      expect(faces.length).toBeGreaterThan(0)
      for (const f of faces) {
        expect(f).toMatch(/font-display: swap;/)
        expect(f).toMatch(/url\([^)]+\.woff2\) format\('woff2'\)/)
      }
    }
  })

  it('index.html preloads exactly one font, in CORS mode, and it is the Latin 400 face', () => {
    expect(preloads).toHaveLength(1)
    expect(preloads[0]).toMatch(/as="font" type="font\/woff2" crossorigin/)
    const href = preloads[0].match(/href="\/node_modules\/@fontsource\/jetbrains-mono\/files\/([^"]+)"/)?.[1]
    const latin400 = fontCss(400).match(/\/\* jetbrains-mono-latin-400-normal \*\/\s*@font-face \{[^}]*url\(\.\/files\/([^)]+\.woff2)\)/)?.[1]
    expect(latin400).toBe('jetbrains-mono-latin-400-normal.woff2')
    expect(href).toBe(latin400)
  })
})
