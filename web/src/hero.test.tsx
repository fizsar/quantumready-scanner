import { screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { contrast, hexToken, over, type RGB, type RGBA } from './contrast'
import { read } from './cssFiles'
import { dict } from './i18n'
import { stubFetch } from './test/accounts'
import { json } from './test/fixtures'
import { renderAt } from './test/render'

const heroCss = read('src/Hero.css')
const tokensCss = read('src/tokens.css')
const indexCss = read('src/index.css')

const open = () =>
  stubFetch((url) => (url === '/api/targets' ? json({ hosts: ['fizsar.com'], port: 443, open: true, proof: true }) : json({ error: { code: 'unauthenticated', message: 'x' } }, 401)))

describe("the home's entry: everything is there in the first render", () => {
  it('title, form and the still image of the scene, before anything has loaded (not even the targets)', () => {
    // A request that never answers: the first render is all there is.
    stubFetch(() => new Promise<Response>(() => {}))
    renderAt('/')
    expect(screen.getByRole('heading', { level: 1 }).textContent).toBe(dict.es['home.title'])
    const input = screen.getByLabelText('Dominio') as HTMLInputElement
    expect(input.disabled).toBe(false)
    expect(screen.getByRole('button', { name: 'Escanear' })).toBeTruthy()
    const still = document.querySelector<HTMLImageElement>('.hero-scene img.hero-still')!
    expect(still.getAttribute('src')).toBe('/lab/story-1s.png')
    // Its box is reserved before it loads: no layout shift.
    expect(still.getAttribute('width')).toBe('640')
    expect(still.getAttribute('height')).toBe('640')
    // No 3D code and no canvas until the page has painted and decided.
    expect(document.querySelector('canvas')).toBeNull()
  })

  it('the title is one heading with the same words; the accent only wraps part of it', () => {
    open()
    for (const lang of ['es', 'en'] as const) {
      expect(dict[lang]['home.title']).toContain(dict[lang]['home.titleAccent'])
    }
    renderAt('/')
    const h1 = screen.getByRole('heading', { level: 1 })
    expect(h1.querySelector('.hero-accent')?.textContent).toBe(dict.es['home.titleAccent'])
    expect(screen.getAllByRole('heading', { level: 1 })).toHaveLength(1)
  })

  it('the scene is decorative: hidden from assistive technology, nothing focusable in it, and after the form in the document', async () => {
    open()
    renderAt('/')
    await screen.findByRole('button', { name: 'Escanear fizsar.com' })
    const scene = document.querySelector('.hero-scene')!
    expect(scene.getAttribute('aria-hidden')).toBe('true')
    expect(scene.querySelector('a, button, input, [tabindex]')).toBeNull()
    expect(scene.querySelector('img')!.getAttribute('alt')).toBe('')
    // No link to the story from the card (the story stays hidden until it leaves /lab).
    expect(document.querySelector('.hero a[href*="historia"]')).toBeNull()
    // Keyboard order: the field, its button, the example domains, the link; the scene comes last.
    const order = [...document.querySelectorAll('.hero input, .hero button, .hero a, .hero-scene')].map((e) => (e.matches('.hero-scene') ? 'scene' : e.tagName))
    expect(order).toEqual(['INPUT', 'BUTTON', 'BUTTON', 'A', 'scene'])
  })

  it('the label says what the still image is, in both languages', () => {
    open()
    renderAt('/')
    const label = document.querySelector('.hero-label')!
    expect(label.getAttribute('data-on')).toBe('true')
    expect(label.textContent).toContain(dict.es['home.sceneLabel'])
    expect(dict.en['home.sceneLabel']).toBe('computed, not drawn')
  })
})

describe("the home's entry: styles", () => {
  const rule = (selector: string) => {
    const m = [...heroCss.matchAll(new RegExp(`(?:^|\\n)${selector.replace(/[.[\]='-]/g, '\\$&')} \\{([^}]*)\\}`, 'g'))]
    if (!m.length) throw new Error(`no ${selector} rule`)
    return m.map((x) => x[1]).join('\n')
  }
  const bg = hexToken(tokensCss, 'bg')
  const triplet = (name: string) => tokensCss.match(new RegExp(`--${name}:\\s*(\\d+),\\s*(\\d+),\\s*(\\d+);`))!.slice(1).map(Number) as RGB
  // The page's top carries a faint indigo glow (body): the lightest thing behind the title.
  const glow = over([...triplet('indigo'), Number(indexCss.match(/radial-gradient\([^,]+,\s*rgba\(var\(--indigo\),\s*([\d.]+)\)/)![1])] as RGBA, bg)

  it('the gradient words are large text and every stop of the gradient reaches 3:1 (WCAG 1.4.3, large text)', () => {
    expect(rule('.hero-accent')).toContain('background: var(--grad);')
    const stops = [...tokensCss.match(/--grad:\s*([^;]*);/)![1].matchAll(/var\(--(\w+)\)/g)].map((m) => m[1])
    expect(stops).toEqual(['cyan', 'indigo', 'violet'])
    for (const s of stops) for (const back of [bg, glow]) expect(contrast(triplet(s), back), s).toBeGreaterThanOrEqual(3)
    // Large: at least 24 px at its smallest (2.488rem), and bold.
    expect(rule('.hero-title')).toMatch(/font-size:\s*clamp\(2\.488rem,/)
    expect(rule('.hero-title')).toContain('font-weight: 700;')
  })

  it('every text colour of the entry is a checked one, at 4.5:1 on what is behind it', () => {
    const colours = [...heroCss.matchAll(/(?<![-\w])color:\s*([^;]+);/g)].map((m) => m[1].trim())
    for (const c of colours) expect(['var(--text)', 'var(--muted)', 'var(--green)', 'transparent', 'CanvasText']).toContain(c)
    const surface = hexToken(tokensCss, 'surface-1')
    for (const name of ['text', 'muted', 'green']) for (const back of [bg, glow, surface]) expect(contrast(hexToken(tokensCss, name), back), name).toBeGreaterThanOrEqual(4.5)
    // The label sits on its own opaque pill, never straight over the scene.
    expect(rule('.hero-label')).toContain('background: var(--surface-1);')
    expect(rule('.hero-label')).toContain('color: var(--text);')
  })

  it('the scene can never cover the form: no pointer events, clipped to its own box, in its own cell', () => {
    const scene = rule('.hero-scene')
    expect(scene).toContain('pointer-events: none;')
    expect(scene).toContain('overflow: hidden;')
    expect(scene).toContain('grid-area: scene;')
    expect(scene).toContain('aspect-ratio: 1;') // the still image is square: the 3D frames the same square
    expect(rule('.hero-copy')).toContain('grid-area: copy;')
    expect(heroCss).not.toMatch(/z-index/)
  })

  it('the entrance never hides anything (it starts visible) and only runs without reduced motion', () => {
    const from = heroCss.match(/@keyframes hero-enter \{\s*from \{([^}]*)\}/)![1]
    expect(Number(from.match(/opacity:\s*([\d.]+)/)![1])).toBeGreaterThanOrEqual(0.4)
    const anim = heroCss.indexOf('animation: hero-enter')
    const media = heroCss.lastIndexOf('@media', anim)
    expect(heroCss.slice(media, anim)).toContain('(prefers-reduced-motion: no-preference)')
    expect(heroCss.match(/animation:/g)).toHaveLength(1)
    // Stagger and curve from the tokens.
    expect(heroCss).toContain('animation: hero-enter var(--dur-slow) var(--ease-out) backwards;')
    expect(heroCss).toContain('animation-delay: var(--stagger);')
  })

  it('the canvas fades in over the image with --dur-base, and the label fades out slowly, with a token too', () => {
    expect(rule('.hero-canvas')).toContain('transition: opacity var(--dur-base) linear;')
    expect(rule('.hero-label')).toMatch(/opacity var\(--dur-slow\) var\(--ease-in\)/)
  })
})
