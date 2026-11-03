import { screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { json, stubFetch } from '../test/fixtures'
import { renderAt } from '../test/render'
import { chooseMode, pointBudget } from './capability'

// The 3D module (the only importer of three.js) is replaced by a spy: if the
// page imports it, `imported` turns true.
const scene = vi.hoisted(() => ({ imported: false, created: 0, fail: false }))
vi.mock('./orbitalScene', () => {
  scene.imported = true
  return {
    createOrbitalScene: (_canvas: HTMLCanvasElement, opts: { onReady: () => void; onError: () => void }) => {
      scene.created++
      if (scene.fail) opts.onError()
      else opts.onReady()
      return { setProgress() {}, start() {}, stop() {}, dispose() {} }
    },
  }
})

const media = (matches: (q: string) => boolean) => vi.stubGlobal('matchMedia', (q: string) => ({ matches: matches(q), media: q }))
/** A fast WebGL2 context, as a desktop browser would give. */
const webgl2 = () =>
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockImplementation(((kind: string) =>
    kind === 'webgl2' ? { getExtension: () => null } : null) as unknown as HTMLCanvasElement['getContext'])
/** Long enough for the idle callback (a 200 ms timeout in jsdom) and the import. */
const settle = () => new Promise((r) => setTimeout(r, 600))

beforeEach(() => {
  scene.imported = false
  scene.created = 0
  scene.fail = false
  stubFetch(() => json({ hosts: ['fizsar.com'], port: 443 }))
})
afterEach(() => vi.restoreAllMocks())

describe('/lab/orbital', () => {
  it('with prefers-reduced-motion: still images, and three.js is never loaded', async () => {
    media((q) => q === '(prefers-reduced-motion: reduce)')
    webgl2() // even with WebGL2 available
    renderAt('/lab/orbital')
    expect(await screen.findByRole('heading', { level: 1, name: /Orbitales del hidrógeno/ })).toBeTruthy()
    expect(screen.getByText(/tu sistema pide reducir el movimiento/)).toBeTruthy()
    const imgs = screen.getAllByRole('img', { name: /Nube de puntos del orbital/ })
    expect(imgs.map((i) => i.getAttribute('src'))).toEqual(['/lab/orbital-1s.png', '/lab/orbital-2pz.png', '/lab/orbital-3dz2.png', '/lab/orbital-4fxyz.png'])
    expect(document.querySelector('canvas')).toBeNull()
    await settle()
    expect(scene.imported).toBe(false)
  })

  it('without WebGL2: still images, no three.js', async () => {
    media(() => false)
    renderAt('/lab/orbital', 'en')
    expect(await screen.findByText(/this browser has no accelerated WebGL2/)).toBeTruthy()
    await settle()
    expect(scene.imported).toBe(false)
  })

  it('with WebGL2 and motion allowed: the 3D module is loaded after first paint (control for the tests above)', async () => {
    media(() => false)
    webgl2()
    renderAt('/lab/orbital')
    await screen.findByRole('heading', { level: 1 })
    expect(scene.imported).toBe(false) // not during the first render
    await waitFor(() => expect(scene.created).toBe(1))
    expect(document.querySelector('canvas.lab-canvas')?.getAttribute('data-ready')).toBe('true')
    expect(screen.queryByText(/Imágenes fijas/)).toBeNull()
  })

  it('a 3D scene that fails (a shader that does not compile) falls back to the still images', async () => {
    media(() => false)
    webgl2()
    scene.fail = true
    renderAt('/lab/orbital')
    await waitFor(() => expect(scene.created).toBe(1))
    expect(await screen.findByText(/este navegador no ofrece WebGL2 acelerado/)).toBeTruthy()
    expect(document.querySelector('canvas')).toBeNull()
    expect(screen.getAllByRole('img', { name: /Nube de puntos del orbital/ })).toHaveLength(4)
  })

  it('hidden: noindex, the label with quantum numbers and the MathML equation, cited sources', async () => {
    media(() => true)
    renderAt('/lab/orbital')
    await screen.findByRole('heading', { level: 1 })
    expect(document.head.querySelector('meta[name="robots"]')?.getAttribute('content')).toBe('noindex, nofollow')
    expect(screen.getByText(/1s · \(n, l, m\) = \(1, 0, 0\) · energía E/)).toBeTruthy()
    expect(screen.getByText(/−13,61 eV/)).toBeTruthy()
    const eq = document.querySelector('math[display="block"]')
    expect(eq?.namespaceURI).toBe('http://www.w3.org/1998/Math/MathML')
    expect(eq?.textContent).toContain('∇')
    const links = screen.getAllByRole('link').map((a) => a.getAttribute('href'))
    expect(links).toEqual(expect.arrayContaining(['https://dlmf.nist.gov/18.9', 'https://dlmf.nist.gov/14.30', 'https://physics.nist.gov/cgi-bin/cuu/Value?rydhcev']))
    expect(screen.getByText(/Introduction to Quantum Mechanics/)).toBeTruthy()
  })

  it('is not linked from the other pages', async () => {
    renderAt('/')
    await screen.findByRole('heading', { level: 1 })
    expect(document.querySelector('a[href*="/lab"]')).toBeNull()
  })
})

describe('chooseMode', () => {
  const env = (o: { reduce?: boolean; coarse?: boolean; gl?: boolean; cores?: number; mem?: number; saveData?: boolean }) => ({
    matchMedia: (q: string) => ({ matches: (q.includes('reduced-motion') && !!o.reduce) || (q.includes('coarse') && !!o.coarse) }),
    navigator: { hardwareConcurrency: o.cores ?? 8, deviceMemory: o.mem, connection: { saveData: o.saveData } } as unknown as Navigator,
    webgl2: () => o.gl ?? true,
  })
  it('falls back to still images on reduced motion, data saver, very low-end devices and without WebGL2', () => {
    expect(chooseMode(env({ reduce: true }))).toEqual({ kind: 'still', reason: 'reduced-motion' })
    expect(chooseMode(env({ saveData: true }))).toEqual({ kind: 'still', reason: 'save-data' })
    expect(chooseMode(env({ mem: 1 }))).toEqual({ kind: 'still', reason: 'low-end' })
    expect(chooseMode(env({ cores: 1 }))).toEqual({ kind: 'still', reason: 'low-end' })
    expect(chooseMode(env({ gl: false }))).toEqual({ kind: 'still', reason: 'no-webgl2' })
  })
  it('sizes the cloud to the device', () => {
    expect(chooseMode(env({ cores: 8, mem: 8 }))).toEqual({ kind: '3d', points: 120_000 })
    expect(pointBudget(8, 8, true)).toBe(50_000) // a fast phone
    expect(pointBudget(4, 4, true)).toBe(25_000)
    expect(pointBudget(4, undefined, false)).toBe(40_000)
  })
})
