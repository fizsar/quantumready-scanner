/// <reference types="node" />
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

// Read from disk: vitest's CSS handling empties `?raw` imports of .css files. Tests run in web/.
const indexCss = readFileSync(resolve('src/index.css'), 'utf8')
const storyCss = readFileSync(resolve('src/lab/story/Story.css'), 'utf8')

// WCAG 2.2 AA (1.4.3): text at least 4.5:1 against what is behind it. Behind
// a panel there can be anything the 3D draws, and additive blending can reach
// white, so the panel colour is composited over white (the worst case) and
// every text colour used on the panels is checked against that.
type RGB = [number, number, number]
const hex = (h: string): RGB => [1, 3, 5].map((i) => parseInt(h.slice(i, i + 2), 16)) as RGB
const token = (name: string) => {
  const m = indexCss.match(new RegExp(`--${name}:\\s*(#[0-9a-fA-F]{6})`))
  if (!m) throw new Error(`no --${name} in index.css`)
  return hex(m[1])
}
const luminance = ([r, g, b]: RGB) => {
  const c = (v: number) => {
    const s = v / 255
    return s <= 0.04045 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4
  }
  return 0.2126 * c(r) + 0.7152 * c(g) + 0.0722 * c(b)
}
const contrast = (a: RGB, b: RGB) => {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x)
  return (hi + 0.05) / (lo + 0.05)
}
const over = (rgba: [number, number, number, number], back: RGB): RGB =>
  [0, 1, 2].map((i) => rgba[i] * rgba[3] + back[i] * (1 - rgba[3])) as RGB

const panel = (() => {
  const m = storyCss.match(/--story-panel:\s*rgba\((\d+),\s*(\d+),\s*(\d+),\s*([\d.]+)\)/)
  if (!m) throw new Error('no --story-panel in Story.css')
  return m.slice(1).map(Number) as [number, number, number, number]
})()

describe('contrast of the story texts (WCAG AA)', () => {
  const WHITE: RGB = [255, 255, 255]
  const texts = { text: token('text'), muted: token('muted'), green: token('green') }

  it('the contrast formula matches known values', () => {
    expect(contrast([0, 0, 0], WHITE)).toBeCloseTo(21, 6)
    expect(contrast(hex('#777777'), WHITE)).toBeCloseTo(4.48, 2)
  })

  for (const [name, colour] of Object.entries(texts))
    it(`--${name} on a panel over the brightest possible backdrop: ≥ 4.5:1`, () => {
      expect(contrast(colour, over(panel, WHITE))).toBeGreaterThanOrEqual(4.5)
    })

  it('the phone sheet is opaque (--bg), so its texts pass too', () => {
    const sheet = storyCss.match(/@media \(max-width: 40rem\)[\s\S]*?\.story-sheet \{[^}]*background:\s*rgb\((\d+), (\d+), (\d+)\)/)
    expect(sheet?.[1]).toBeDefined()
    const bg = sheet!.slice(1, 4).map(Number) as RGB
    for (const c of Object.values(texts)) expect(contrast(c, bg)).toBeGreaterThanOrEqual(4.5)
  })

  it('every story text rule uses one of the checked colours', () => {
    const colours = [...storyCss.matchAll(/(?<![-\w])color:\s*([^;]+);/g)].map((m) => m[1].trim())
    for (const c of colours) expect(['var(--text)', 'var(--muted)', 'var(--green)']).toContain(c)
  })
})
