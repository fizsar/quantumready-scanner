import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { contrast, hexToken, over, rgbaToken, type RGB, type RGBA } from './contrast'
import { cssFiles, read } from './cssFiles'

const indexCss = read('src/index.css')
const rule = (css: string, selector: string) => {
  const m = css.match(new RegExp(`(?:^|\\n)${selector.replace('.', '\\.')} \\{([^}]*)\\}`))
  if (!m) throw new Error(`no ${selector} rule`)
  return m[1]
}

// WCAG 2.2 AA 1.4.11 (non-text contrast): the boundary that shows where a form
// field is must reach 3:1 against what is next to it. Inside the field is --bg;
// outside, the page (--bg) or a panel (--panel over the page). The page's top
// carries a faint indigo glow (body background): the panel over that glow at
// full strength is the lightest neighbour.
describe('form field border (WCAG 1.4.11)', () => {
  const border = hexToken(indexCss, 'border-control')
  const bg = hexToken(indexCss, 'bg')
  const panel = rgbaToken(indexCss, 'panel')
  const glow = (() => {
    const indigo = indexCss.match(/--indigo:\s*(\d+),\s*(\d+),\s*(\d+);/)
    const alpha = rule(indexCss, 'body').match(/radial-gradient\([^,]+,\s*rgba\(var\(--indigo\),\s*([\d.]+)\)/)
    if (!indigo || !alpha) throw new Error('no indigo glow on body')
    return over([...indigo.slice(1).map(Number), Number(alpha[1])] as RGBA, bg)
  })()
  const neighbours: Record<string, RGB> = {
    '--bg (inside the field, and the page)': bg,
    'a panel over the page': over(panel, bg),
    'a panel over the brightest point of the glow': over(panel, glow),
    'the glow itself': glow,
  }
  for (const [name, colour] of Object.entries(neighbours))
    it(`--border-control against ${name}: ≥ 3:1`, () => {
      expect(contrast(border, colour)).toBeGreaterThanOrEqual(3)
    })

  it('the decorative --line would not pass (why the field has its own token)', () => {
    expect(contrast(over(rgbaToken(indexCss, 'line'), bg), bg)).toBeLessThan(3)
  })

  it('.input (the domain field and every other field) draws its border with --border-control', () => {
    expect(rule(indexCss, '.input')).toMatch(/border:\s*1px solid var\(--border-control\);/)
  })

  it('no stylesheet changes the border of a field elsewhere', () => {
    for (const f of cssFiles) {
      const css = readFileSync(f, 'utf8')
      for (const m of css.matchAll(/([^{}]*)\{([^}]*)\}/g)) {
        const sel = m[1].trim()
        if (/(\.input|textarea|select|input)(?![-\w])/.test(sel) && !/\.input::placeholder/.test(sel) && sel !== '.input')
          expect(`${f}: ${sel} { ${m[2].trim()} }`).not.toMatch(/\bborder(-color)?\s*:/)
      }
    }
  })
})
