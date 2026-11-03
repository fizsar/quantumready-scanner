import { screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { json, stubFetch } from '../../test/fixtures'
import { renderAt } from '../../test/render'

// The engine (the only importer of three.js here) is replaced by a spy.
const engine = vi.hoisted(() => ({ imported: false, created: 0, fail: false, progress: [] as number[] }))
vi.mock('./engine', () => {
  engine.imported = true
  return {
    createStory: (_canvas: HTMLCanvasElement, opts: { onReady: () => void; onError: () => void }) => {
      engine.created++
      if (engine.fail) opts.onError()
      else opts.onReady()
      return { setProgress: (p: number) => engine.progress.push(p), start() {}, stop() {}, dispose() {} }
    },
  }
})

const media = (matches: (q: string) => boolean) => vi.stubGlobal('matchMedia', (q: string) => ({ matches: matches(q), media: q }))
const webgl2 = () =>
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockImplementation(((kind: string) =>
    kind === 'webgl2' ? { getExtension: () => null } : null) as unknown as HTMLCanvasElement['getContext'])
/** As if the visitor had scrolled `viewports` screens into the story track. */
const scrolled = (viewports: number) =>
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
    const top = this.classList.contains('story-track') ? -viewports * window.innerHeight : 0
    return { top, bottom: top + 1000, left: 0, right: 0, width: 0, height: 1000, x: 0, y: top, toJSON() {} } as DOMRect
  })
const settle = () => new Promise((r) => setTimeout(r, 600))

beforeEach(() => {
  Object.assign(engine, { imported: false, created: 0, fail: false, progress: [] })
  stubFetch(() => json({ hosts: ['fizsar.com'], port: 443 }))
})
afterEach(() => vi.restoreAllMocks())

describe('/lab/historia', () => {
  it('with prefers-reduced-motion: still images per step, and three.js is never loaded', async () => {
    media((q) => q === '(prefers-reduced-motion: reduce)')
    webgl2()
    renderAt('/lab/historia')
    expect(await screen.findByRole('heading', { level: 1, name: /De la mecánica cuántica a tu web/ })).toBeTruthy()
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
    renderAt('/lab/historia')
    await screen.findByRole('heading', { level: 1 })
    expect(engine.imported).toBe(false)
    await waitFor(() => expect(engine.created).toBe(1))
    expect(document.querySelector('canvas.story-canvas')?.getAttribute('data-ready')).toBe('true')
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
    const state = (term: string) => document.querySelector(`.story-caption [data-current="true"] [data-term="${term}"]`)?.getAttribute('data-state')
    await waitFor(() => expect(state('dt')).toBe('active'))
    expect(state('psi')).toBe('shown')
    expect(state('kin')).toBe('hidden')
    expect(document.querySelector('.story-caption [data-current="true"] math')?.namespaceURI).toBe('http://www.w3.org/1998/Math/MathML')
    expect(document.querySelector('.story-still')?.getAttribute('src')).toBe('/lab/story-dt.png')
  })

  it('scene 1: the label with the quantum numbers; hidden page; cited sources', async () => {
    media(() => true)
    renderAt('/lab/historia')
    await screen.findByRole('heading', { level: 1 })
    expect(document.head.querySelector('meta[name="robots"]')?.getAttribute('content')).toBe('noindex, nofollow')
    expect(document.querySelectorAll('.story-caption [data-current="true"]')).toHaveLength(1)
    expect(document.querySelector('.story-caption [data-current="true"]')?.textContent).toMatch(/energía E1 = −13,61 eV/)
    expect(screen.getAllByText(/Cohen-Tannoudji/).length).toBeGreaterThan(0)
    expect(screen.getAllByRole('heading', { level: 2 }).map((h) => h.textContent)).toContain('Superposición e interferencia')
  })

  it('is not linked from the other pages', async () => {
    renderAt('/')
    await screen.findByRole('heading', { level: 1 })
    expect(document.querySelector('a[href*="/lab"]')).toBeNull()
  })
})
