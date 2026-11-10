import { fireEvent, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { json, stubFetch } from '../../test/fixtures'
import { renderAt } from '../../test/render'
import { curvePoints } from './figures'

// The engine (the only importer of three.js here) is replaced by a spy.
const engine = vi.hoisted(() => ({
  imported: false,
  created: 0,
  fail: false,
  progress: [] as number[],
  frames: [] as unknown[],
}))
vi.mock('./engine', () => {
  engine.imported = true
  return {
    createStory: (_canvas: HTMLCanvasElement, opts: { onReady: () => void; onError: () => void }) => {
      engine.created++
      if (engine.fail) opts.onError()
      else opts.onReady()
      return {
        setProgress: (p: number) => engine.progress.push(p),
        setFrame: (f: unknown) => engine.frames.push(f),
        start() {},
        stop() {},
        dispose() {},
      }
    },
  }
})

const media = (matches: (q: string) => boolean) =>
  vi.stubGlobal('matchMedia', (q: string) => ({
    matches: matches(q),
    media: q,
  }))
const webgl2 = () =>
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockImplementation(((kind: string) =>
    kind === 'webgl2' ? { getExtension: () => null } : null) as unknown as HTMLCanvasElement['getContext'])
/** As if the visitor had scrolled `viewports` screens into the story track. */
const scrolled = (viewports: number) =>
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
    const top = this.classList.contains('story-track') ? -viewports * window.innerHeight : 0
    return {
      top,
      bottom: top + 1000,
      left: 0,
      right: 0,
      width: 0,
      height: 1000,
      x: 0,
      y: top,
      toJSON() {},
    } as DOMRect
  })
const settle = () => new Promise((r) => setTimeout(r, 600))

beforeEach(() => {
  Object.assign(engine, {
    imported: false,
    created: 0,
    fail: false,
    progress: [],
    frames: [],
  })
  stubFetch(() => json({ hosts: ['fizsar.com'], port: 443 }))
})
afterEach(() => vi.restoreAllMocks())

describe('/lab/historia', () => {
  it('with prefers-reduced-motion: still images per step, and three.js is never loaded', async () => {
    media((q) => q === '(prefers-reduced-motion: reduce)')
    webgl2()
    renderAt('/lab/historia')
    expect(
      await screen.findByRole('heading', {
        level: 1,
        name: /De la mecánica cuántica a tu web/,
      }),
    ).toBeTruthy()
    expect(screen.getByText(/tu sistema pide reducir el movimiento/)).toBeTruthy()
    expect(document.querySelector('.story-still')?.getAttribute('src')).toBe('/lab/story-1s.png')
    expect(document.querySelector('canvas')).toBeNull()
    await settle()
    expect(engine.imported).toBe(false)
  })

  it('without WebGL2: still images, no three.js', async () => {
    media(() => false)
    renderAt('/lab/historia', 'en')
    expect(await screen.findByText(/this browser has no accelerated WebGL2/)).toBeTruthy()
    await settle()
    expect(engine.imported).toBe(false)
  })

  it('with WebGL2: the engine loads after first paint and follows the scroll (control)', async () => {
    media(() => false)
    webgl2()
    scrolled(2)
    // The page waits for an idle moment before loading three.js: hold it until we say so.
    let idle: (() => void) | null = null
    vi.stubGlobal('requestIdleCallback', (cb: () => void) => ((idle = cb), 1))
    renderAt('/lab/historia')
    await screen.findByRole('heading', { level: 1 })
    await waitFor(() => expect(idle).not.toBeNull())
    await new Promise((r) => setTimeout(r, 300))
    expect(engine.created).toBe(0) // painted, not loaded
    idle!()
    await waitFor(() => expect(engine.created).toBe(1))
    expect(document.querySelector('canvas.story-canvas')?.getAttribute('data-ready')).toBe('true')
    // It arrives after the page measured the scroll and the free area: it gets
    // both at once, with no scroll event (at the top of the page none follows).
    expect(engine.progress.at(-1)).toBeCloseTo(2.35, 6)
    expect(engine.frames).toHaveLength(1)
    window.dispatchEvent(new Event('scroll'))
    await waitFor(() => expect(engine.progress.at(-1)).toBeCloseTo(2.35, 6))
  })

  it('a failing engine (shader error) falls back to the still images', async () => {
    media(() => false)
    webgl2()
    engine.fail = true
    renderAt('/lab/historia')
    await waitFor(() => expect(engine.created).toBe(1))
    expect(await screen.findByText(/este navegador no ofrece WebGL2 acelerado/)).toBeTruthy()
    expect(document.querySelector('canvas')).toBeNull()
  })

  it('scene 2: the equation appears term by term and the step’s term is lit', async () => {
    media(() => true)
    scrolled(5) // progress 5.35: the "∂ψ/∂t" step
    renderAt('/lab/historia')
    await screen.findByRole('heading', { level: 1 })
    const panel = () => document.getElementById('story-step-dt')!.closest('.story-panel')!
    const state = (term: string) => panel().querySelector(`[data-term="${term}"]`)?.getAttribute('data-state')
    await waitFor(() => expect(state('dt')).toBe('active'))
    expect(state('psi')).toBe('shown')
    expect(state('kin')).toBe('hidden')
    expect(panel().querySelector('math')?.namespaceURI).toBe('http://www.w3.org/1998/Math/MathML')
    expect(document.querySelector('.story-still')?.getAttribute('src')).toBe('/lab/story-dt.png')
  })

  it('scene 1: the label with the quantum numbers; hidden page; cited sources', async () => {
    media(() => true)
    renderAt('/lab/historia')
    await screen.findByRole('heading', { level: 1 })
    expect(document.head.querySelector('meta[name="robots"]')?.getAttribute('content')).toBe('noindex, nofollow')
    expect(document.querySelector('.story-caption')).toBeNull()
    const panel = document.getElementById('story-step-1s')!.closest('.story-panel')!
    expect(panel.querySelector('.story-cap')?.textContent).toMatch(/energía E1 = −13,61 eV/)
    // The expression in the title stays together.
    expect(panel.querySelector('.story-h .story-nowrap')?.textContent).toBe('(n, l, m) = (1, 0, 0)')
    expect(screen.getAllByText(/Cohen-Tannoudji/).length).toBeGreaterThan(0)
    expect(screen.getAllByRole('heading', { level: 2 }).map((h) => h.textContent)).toContain('Superposición e interferencia')
  })

  it('scenes 3–5: each panel shows the numbers computed by quantum.ts (Spanish)', async () => {
    media(() => true)
    renderAt('/lab/historia')
    await screen.findByRole('heading', { level: 1 })
    const cap = (id: string) => document.getElementById(`story-step-${id}`)!.closest('.story-panel')!.querySelector('.story-cap')!
    const line = (id: string) => cap(id).querySelector('.story-scale')!.textContent
    expect(line('q0')).toBe('θ = 0: |0⟩, vector (0; 0; 1)')
    expect(line('qH')).toBe('θ = π/2, φ = 0: P(0) = 0,5 · P(1) = 0,5')
    expect(line('qM')).toBe('resultado 1: |1⟩, vector (0; 0; −1)')
    expect(line('bell')).toBe('cada cúbit: vector (0; 0; 0), pureza 0,5 · ⟨Z⊗Z⟩ = 1')
    expect(line('circ')).toBe('P(00) = 0,5 · P(01) = 0 · P(10) = 0 · P(11) = 0,5')
    expect(line('reg3')).toBe('2³ = 8 amplitudes de 0,354')
    expect(line('reg10')).toBe('2¹⁰ = 1024 amplitudes de 0,031')
    expect(line('shorF')).toBe('f = 1, 7, 4, 13, 1, 7, … → r = 4')
    expect(line('qft')).toBe('picos en y = 0, 64, 128, 192, P = 0,25 · r = 4')
    expect(line('factors')).toBe('mcd(48, 15) = 3 · mcd(50, 15) = 5 · 3 × 5 = 15')
    expect(line('curve')).toBe(`sobre 𝔽₁₀₁: ${curvePoints().length + 1} puntos (con el del infinito)`)
    expect(line('broken')).toBe('RSA-2048: < 1.000.000 cúbits ruidosos, < 7 días, con 0,1\u00a0% de error por puerta, ciclo de 1 µs y 10 µs de reacción')
    // The formula of each step in MathML.
    for (const id of ['q0', 'bell', 'circ', 'reg6', 'shorF', 'qft', 'key', 'factors', 'curve'])
      expect(cap(id).querySelector('math')?.namespaceURI, id).toBe('http://www.w3.org/1998/Math/MathML')
    // Scene headings and the sources of scenes 3–5.
    for (const s of ['Escena 3 · El cúbit', 'Escena 4 · El ordenador cuántico', 'Escena 5 · El algoritmo de Shor']) expect(screen.getAllByText(s).length).toBeGreaterThan(0)
    const sources = document.querySelector('.story-sources')!.textContent!
    for (const s of ['Nielsen', 'SIAM J. Comput. 26(5), 1484–1509', 'Commun. ACM 21(2)', 'RFC 7748', 'arXiv:2505.15917', 'Quantum 5, 433', 'ASIACRYPT 2017', '2330 cúbits lógicos'])
      expect(sources, s).toContain(s)
  })

  it('scenes 3–5 in English, numbers in English format', async () => {
    media(() => true)
    renderAt('/lab/historia', 'en')
    await screen.findByRole('heading', { level: 1 })
    const line = (id: string) => document.getElementById(`story-step-${id}`)!.closest('.story-panel')!.querySelector('.story-scale')!.textContent
    expect(line('qH')).toBe('θ = π/2, φ = 0: P(0) = 0.5 · P(1) = 0.5')
    expect(line('factors')).toBe('gcd(48, 15) = 3 · gcd(50, 15) = 5 · 3 × 5 = 15')
    expect(line('broken')).toBe('RSA-2048: < 1,000,000 noisy qubits, < 7 days, at 0.1% gate error, 1 µs cycle and 10 µs reaction')
  })

  it('scene 6: the panels’ formulas, the note on z, the scene heading and the sources (Spanish)', async () => {
    media(() => true)
    renderAt('/lab/historia')
    await screen.findByRole('heading', { level: 1 })
    const cap = (id: string) => document.getElementById(`story-step-${id}`)!.closest('.story-panel')!.querySelector('.story-cap')!
    const line = (id: string) => cap(id).querySelector('.story-scale')!.textContent
    expect(line('mosca')).toBe('z no es una fecha: es incierto (encuesta del GRI, 2025)')
    for (const id of ['harvest', 'stored', 'decrypted', 'mosca']) expect(cap(id).querySelector('math')?.namespaceURI, id).toBe('http://www.w3.org/1998/Math/MathML')
    expect(screen.getAllByText('Escena 6 · Cosechar ahora, descifrar después').length).toBeGreaterThan(0)
    const sources = document.querySelector('.story-sources')!.textContent!
    for (const s of ['IEEE Security & Privacy 16(5), 38–41 (2018)', 'Quantum Threat Timeline Report 2025', '26 expertos', 'menos de 24 horas']) expect(sources, s).toContain(s)
  })

  it('scene 7: each panel shows the numbers computed by lattice.ts and mlkem.ts, and the sources (Spanish)', async () => {
    media(() => true)
    renderAt('/lab/historia')
    await screen.findByRole('heading', { level: 1 })
    const cap = (id: string) => document.getElementById(`story-step-${id}`)!.closest('.story-panel')!.querySelector('.story-cap')!
    const line = (id: string) => cap(id).querySelector('.story-scale')!.textContent
    expect(line('lattice')).toBe('b₁ = (3; 1), b₂ = (1; 2), det = 5 · t = (3,6; 2,4) → más cercano (4; 3), a 0,72')
    expect(line('cvp')).toBe('(17; 9), (7; 4), det = 5: la misma red · redondeo → (14; 8), a 11,81 (16 veces más lejos)')
    expect(line('lwe')).toBe('n = 4, m = 6 · sin e, Gauss da (23; 71; 8; 54) = s · con e, (7; 42; 39; 86) · a la fuerza, 88.529.281 candidatos')
    expect(line('mlkem')).toBe('clave pública 1184 B, cifrado 1088 B · X25519: 32 B · secretos posibles: como máximo un número de 537 cifras')
    expect(line('hybrid')).toBe('cliente 1184 + 32 = 1216 B · servidor 1088 + 32 = 1120 B · secreto 64 B · grupo 4588 (0x11EC)')
    for (const id of ['lattice', 'cvp', 'lwe', 'mlkem', 'hybrid']) expect(cap(id).querySelector('math')?.namespaceURI, id).toBe('http://www.w3.org/1998/Math/MathML')
    expect(screen.getAllByText('Escena 7 · Retículos y ML-KEM').length).toBeGreaterThan(0)
    const sources = document.querySelector('.story-sources')!.textContent!
    for (const s of ['FIPS 203', 'RFC 10024', 'draft-ietf-tls-ecdhe-mlkem-05', '4588 (0x11EC)', 'Combinatorica 6(1)', 'J. ACM 56(6)']) expect(sources, s).toContain(s)
  })

  it('scene 6: the sliders have labels and spoken values, and the verdict is announced as they move (desktop: one active widget)', async () => {
    media(() => false)
    renderAt('/lab/historia')
    await screen.findByRole('heading', { level: 1 })
    const widgets = document.querySelectorAll('.story-mosca')
    expect(widgets).toHaveLength(1) // no copy in the sheet on wide screens
    const w = widgets[0] as HTMLElement
    expect(w.hasAttribute('inert')).toBe(false)
    expect(w.getAttribute('aria-hidden')).toBeNull()
    const x = screen.getByRole('slider', { name: /x secreto/ })
    const y = screen.getByRole('slider', { name: /y migración/ })
    expect([x.getAttribute('min'), x.getAttribute('max'), y.getAttribute('max')]).toEqual(['0', '30', '20'])
    expect(x.getAttribute('aria-valuetext')).toBe('10 años')
    const out = w.querySelector('.story-mosca-out')!
    expect(out.getAttribute('aria-live')).toBe('polite')
    expect(out.textContent).toBe('x + y = 15 años: en 15 años, los expertos ven un 51–\u206070\u00a0% de probabilidad de un ordenador cuántico capaz (GRI 2025). Riesgo.')
    fireEvent.change(x, { target: { value: '2' } })
    fireEvent.change(y, { target: { value: '2' } })
    expect(x.getAttribute('aria-valuetext')).toBe('2 años')
    expect(out.textContent).toBe('x + y = 4 años: en 5 años la media no pasa del 15\u00a0%; en 10, 28–\u206049\u00a0% (GRI 2025).')
    fireEvent.change(x, { target: { value: '20' } })
    expect(out.textContent).toBe('x + y = 22 años: en 15 años, los expertos ven un 51–\u206070\u00a0% de probabilidad de un ordenador cuántico capaz (GRI 2025). Riesgo. A 20, al menos el 69\u00a0%.')
  })

  it('scene 6 on phones: the active widget is the sheet’s (not hidden from assistive technology); the panel’s copy is inert and hidden', async () => {
    media((q) => q === '(max-width: 40rem)')
    renderAt('/lab/historia', 'en')
    await screen.findByRole('heading', { level: 1 })
    const sheetW = document.querySelector('.story-sheet .story-mosca') as HTMLElement
    const panelW = document.querySelector('.story-track .story-mosca') as HTMLElement
    expect(sheetW.hasAttribute('inert')).toBe(false)
    expect(sheetW.closest('[aria-hidden="true"]')).toBeNull()
    expect(sheetW.querySelector('.story-mosca-out')!.getAttribute('aria-live')).toBe('polite')
    expect(panelW.hasAttribute('inert')).toBe(true)
    expect(panelW.getAttribute('aria-hidden')).toBe('true')
    expect(panelW.querySelector('.story-mosca-out')!.getAttribute('aria-live')).toBeNull()
    // Every other piece of the sheet stays hidden from assistive technology (a copy of the panels).
    for (const copy of document.querySelectorAll('.story-sheet .story-layer-copy')) expect(copy.getAttribute('aria-hidden')).toBe('true')
    // One state: moving the sheet's slider moves the panel's too.
    fireEvent.change(sheetW.querySelector('input')!, { target: { value: '25' } })
    expect((panelW.querySelector('input') as HTMLInputElement).value).toBe('25')
    expect(sheetW.querySelector('.story-mosca-out')!.textContent).toBe('x + y = 30 years: within 15 years, experts put a capable quantum computer at 51–\u206070% (GRI 2025). At risk. At 20, at least 69%.')
  })

  it('the stage shows only the track on screen, with soft edges, and fades in with it; the sheet shows once the track is behind it', async () => {
    media(() => true)
    const vh = window.innerHeight
    let track = { top: vh - 200, bottom: 9000 } // the intro fills most of the screen; the track comes in from below
    const boxes: Record<string, { top: number; bottom: number }> = {
      'story-stage': { top: 0, bottom: vh },
      'story-sheet': { top: vh - 160, bottom: vh },
    }
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
      const key = [...this.classList].find((c) => c in boxes || c === 'story-track')
      const r = key === 'story-track' ? track : key ? boxes[key] : { top: 0, bottom: 0 }
      return {
        ...r,
        left: 0,
        right: 100,
        width: 100,
        height: r.bottom - r.top,
        x: 0,
        y: r.top,
        toJSON() {},
      } as DOMRect
    })
    renderAt('/lab/historia')
    await screen.findByRole('heading', { level: 1 })
    const st = () => document.querySelector<HTMLElement>('.story-stage')!.style
    const band = () => [st().getPropertyValue('--vis-top'), st().getPropertyValue('--vis-bottom')]
    const sheetOn = () => document.querySelector('.story-sheet')?.getAttribute('data-active')
    const scroll = async (r: { top: number; bottom: number }) => {
      track = r
      window.dispatchEvent(new Event('scroll'))
      await new Promise((ok) => requestAnimationFrame(() => setTimeout(ok)))
    }
    // Entering: only the bottom 200 px are the track; the stage shows there, faintly, fading in at its top edge.
    await waitFor(() => expect(band()).toEqual([`${vh - 200}px`, `${vh}px`]))
    // The browser normalizes the gradient's text; the stops are what matter (exact form: model.test.ts).
    const stops = () => [...st().maskImage.matchAll(/(-?[\d.]+)px/g)].map((m) => Number(m[1]))
    expect(stops()).toEqual([vh - 200, vh - 200 + 72, vh, vh])
    expect(Number(st().opacity)).toBeGreaterThan(0)
    expect(Number(st().opacity)).toBeLessThan(1)
    expect(st().visibility).toBe('visible')
    // The sheet is not shown until the track is behind it.
    await scroll({ top: vh - 100, bottom: 9000 })
    expect(sheetOn()).toBe('false')
    // Inside: the whole screen, fully opaque, no fade at the screen's own edges; sheet on.
    await scroll({ top: -500, bottom: 8000 })
    await waitFor(() => expect(band()).toEqual(['0px', `${vh}px`]))
    expect(stops()).toEqual([0, 0, vh, vh])
    expect(st().opacity).toBe('1')
    expect(sheetOn()).toBe('true')
    // Leaving: the sources come in from below; the stage fades out where the track ends.
    await scroll({ top: -9000, bottom: vh - 300 })
    await waitFor(() => expect(band()).toEqual(['0px', `${vh - 300}px`]))
    expect(stops()).toEqual([0, 0, vh - 300 - 72, vh - 300])
    expect(sheetOn()).toBe('false')
    // Past it: hidden.
    await scroll({ top: -9600, bottom: -10 })
    await waitFor(() => expect(st().visibility).toBe('hidden'))
  })

  it('the sheet changes in the same frame as data-panels: never shown over no scene, not even for a render', async () => {
    media(() => true)
    const vh = window.innerHeight
    let track = { top: vh - 200, bottom: 9000 }
    const boxes: Record<string, { top: number; bottom: number }> = {
      'story-stage': { top: 0, bottom: vh },
      'story-sheet': { top: vh - 160, bottom: vh },
    }
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
      const key = [...this.classList].find((c) => c in boxes || c === 'story-track')
      const r = key === 'story-track' ? track : key ? boxes[key] : { top: 0, bottom: 0 }
      return { ...r, left: 0, right: 100, width: 100, height: r.bottom - r.top, x: 0, y: r.top, toJSON() {} } as DOMRect
    })
    renderAt('/lab/historia')
    await screen.findByRole('heading', { level: 1 })
    // Read both attributes in the animation frame right after the page's update,
    // before React could render anything.
    const sameFrame = (r: { top: number; bottom: number }) =>
      new Promise<[string | null, string | null]>((ok) => {
        track = r
        window.dispatchEvent(new Event('scroll'))
        requestAnimationFrame(() =>
          ok([document.querySelector('.story')!.getAttribute('data-panels'), document.querySelector('.story-sheet')!.getAttribute('data-active')]),
        )
      })
    // In, out at the end (as when Safari's toolbar shortens the track), in again, entering.
    for (const r of [{ top: -500, bottom: 8000 }, { top: -9000, bottom: vh - 300 }, { top: -500, bottom: 8000 }, { top: vh - 100, bottom: 9000 }]) {
      const [panels, active] = await sameFrame(r)
      expect(active, `track ${r.top}..${r.bottom}`).toBe(panels === 'on' ? 'true' : 'false')
    }
  })

  it('progress uses the visible height (visualViewport: iOS toolbars), not the layout viewport', async () => {
    media(() => false)
    webgl2()
    const listeners: string[] = []
    vi.stubGlobal('visualViewport', {
      height: 400,
      addEventListener: (t: string) => listeners.push(t),
      removeEventListener() {},
    })
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
      const r = this.classList.contains('story-track') ? { top: -400, bottom: 9000 } : { top: 0, bottom: 0 }
      return {
        ...r,
        left: 0,
        right: 0,
        width: 0,
        height: r.bottom - r.top,
        x: 0,
        y: r.top,
        toJSON() {},
      } as DOMRect
    })
    renderAt('/lab/historia')
    await waitFor(() => expect(engine.created).toBe(1))
    window.dispatchEvent(new Event('scroll'))
    // One visible screen (400 px) into the track: step 1, +0.35.
    await waitFor(() => expect(engine.progress.at(-1)).toBeCloseTo(1.35, 6))
    expect(listeners).toEqual(expect.arrayContaining(['resize', 'scroll']))
  })

  it('never an empty scene: the current step’s still image until the 3D has drawn', async () => {
    media(() => false)
    webgl2()
    engine.fail = false
    renderAt('/lab/historia')
    await screen.findByRole('heading', { level: 1 })
    expect(document.querySelector('.story-still')?.getAttribute('src')).toBe('/lab/story-1s.png')
    await waitFor(() => expect(document.querySelector('canvas.story-canvas')?.getAttribute('data-ready')).toBe('true'))
    expect(document.querySelector('.story-still')).toBeNull()
  })

  it('is not linked from the other pages', async () => {
    renderAt('/')
    await screen.findByRole('heading', { level: 1 })
    expect(document.querySelector('a[href*="/lab"]')).toBeNull()
  })
})
