import { describe, expect, it } from 'vitest'
import { frameAt, progressAt, STEPS, stepAt, termState, TERMS, WAVE, wavePoint, waveSeeds } from './model'
import { field, MEET, psi, SAMPLES, sampleX } from './wave'

describe('the story timeline', () => {
  it('scene 1 is the four orbitals, scene 2 reveals the five terms in order', () => {
    expect(STEPS.map((s) => s.id)).toEqual(['1s', '2pz', '3dz2', '4fxyz', 'psi', 'dt', 'kin', 'pot', 'sup'])
    expect(STEPS.filter((s) => s.scene === 2).map((s) => s.term)).toEqual([...TERMS])
  })

  it('a change of shape holds for the first half of the step and morphs in the second', () => {
    expect(frameAt(1.2)).toMatchObject({ a: { kind: 'orbital', index: 1 }, b: { kind: 'orbital', index: 2 }, mix: 0 })
    expect(frameAt(1.75).mix).toBeCloseTo(0.5, 6)
    expect(frameAt(1.999).mix).toBeCloseTo(1, 3)
    // 4f → the wave, which starts at its first step's time.
    expect(frameAt(3.9)).toMatchObject({ a: { kind: 'orbital', index: 3 }, b: { kind: 'wave' }, t: 0 })
  })

  it('the wave runs each step’s time range continuously, and the last step reaches the meeting time', () => {
    expect(frameAt(5).t).toBe(0)
    expect(frameAt(5.5).t).toBeCloseTo(1.5, 9)
    expect(frameAt(6).t).toBe(3)
    expect(frameAt(STEPS.length).t).toBeCloseTo(MEET, 4)
    let prev = -1
    for (let p = 4; p < STEPS.length; p += 0.01) {
      const t = frameAt(p).t
      expect(t).toBeGreaterThanOrEqual(prev - 1e-9)
      prev = t
    }
  })

  it('the camera moves continuously with the scroll (no jumps)', () => {
    for (let p = 0; p < STEPS.length - 0.01; p += 0.01) {
      const a = frameAt(p).cam
      const b = frameAt(p + 0.01).cam
      expect(Math.abs(a.az - b.az) + Math.abs(a.el - b.el) + Math.abs(a.dist - b.dist)).toBeLessThan(0.1)
    }
  })

  it('scroll → progress → the step whose text is shown', () => {
    expect(progressAt(-1)).toBe(0)
    expect(progressAt(0)).toBeCloseTo(0.35, 9)
    expect(progressAt(100)).toBeLessThan(STEPS.length)
    expect(stepAt(1.7)).toBe(1)
    expect(stepAt(1.8)).toBe(2)
    expect(stepAt(STEPS.length - 0.01)).toBe(STEPS.length - 1)
  })

  it('terms: hidden until their step, then shown; only the step’s own is lit', () => {
    const at = (id: string) => STEPS.find((s) => s.id === id)!
    expect(TERMS.map((t) => termState(at('1s'), t))).toEqual(['hidden', 'hidden', 'hidden', 'hidden', 'hidden'])
    expect(TERMS.map((t) => termState(at('dt'), t))).toEqual(['shown', 'active', 'hidden', 'hidden', 'hidden'])
    expect(TERMS.map((t) => termState(at('sup'), t))).toEqual(['shown', 'shown', 'shown', 'shown', 'active'])
  })
})

describe('the wave as particles', () => {
  const seeds = waveSeeds(5000, 1)
  it('each particle belongs to a sample of ψ: ψ in the complex plane (70 %) or |ψ|² below', () => {
    let fan = 0
    for (let i = 0; i < 5000; i++) {
      expect(Number.isInteger(seeds[4 * i]) && seeds[4 * i] >= 0 && seeds[4 * i] < SAMPLES).toBe(true)
      if (seeds[4 * i + 2] === 0) fan++
    }
    expect(fan / 5000).toBeCloseTo(0.7, 1)
  })

  it('position, colour and opacity follow ψ: the phase sets the hue, |ψ| the opacity', () => {
    const t = 4
    const f = field(t)
    for (let i = 0; i < 300; i++) {
      const w = wavePoint(seeds, i, f)
      const col = seeds[4 * i]
      const [re, im] = psi(sampleX(col), t)
      const amp = Math.hypot(re, im)
      if (seeds[4 * i + 2] === 0 && amp > 1e-6) {
        // On the segment from the axis to ψ (scaled): Re and Im in the same ratio.
        if (Math.abs(w.pos[1]) > 1e-6) expect((w.pos[2] - seeds[4 * i + 3]) / w.pos[1]).toBeCloseTo(im / re, 3)
        expect(w.hue).toBeCloseTo(0.5 + (0.5 * re) / amp, 5) // ½ + ½ cos(arg ψ)
      }
      if (seeds[4 * i + 2] === 1) {
        // The lower particles fill the area under |ψ|² (field units): height = s · |ψ|² · dens.
        const d = f[2 * col] ** 2 + f[2 * col + 1] ** 2
        expect(w.pos[1] - WAVE.base).toBeCloseTo(seeds[4 * i + 1] * d * WAVE.dens, 6)
      }
      expect(w.alpha).toBeGreaterThanOrEqual(0)
      expect(w.alpha).toBeLessThanOrEqual(1)
    }
  })

  it('the |ψ|² particles draw the interference fringes at the meeting time', () => {
    const f = field(MEET)
    // Heights of the |ψ|² particles with s = 1 trace the density.
    const top = (col: number) => wavePoint(new Float32Array([col, 1, 1, 0]), 0, f).pos[1]
    const centre = SAMPLES / 2
    // ±50 samples = ±3.9 in x: several fringes π/2 apart.
    const values = Array.from({ length: 100 }, (_, k) => top(centre - 50 + k))
    const minima = values.filter((v, k) => k > 0 && k < 99 && v < values[k - 1] && v < values[k + 1])
    expect(minima.length).toBeGreaterThanOrEqual(3)
  })
})
