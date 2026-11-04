import { describe, expect, it } from 'vitest'
import { density, ORBITALS, sample } from '../hydrogen'
import { BURST, burstDir, covers, densityWeights, fieldAt, fillBase, frameAt, stageView, trackShare, N_REF, progressAt, STEPS, stepAt, storyAlpha, storySize, termState, TERMS, WAVE, wavePoint, waveSeeds } from './model'
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
    // Fast moves (a wide shot) are fine; a jump is not: 1/1000 of a step never moves it more than 0.05.
    for (let p = 0; p < STEPS.length - 0.001; p += 0.001) {
      const a = frameAt(p).cam
      const b = frameAt(p + 0.001).cam
      expect(Math.abs(a.az - b.az) + Math.abs(a.el - b.el) + Math.abs(a.dist - b.dist)).toBeLessThan(0.05)
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
  it('each particle belongs to a sample of ψ: on the helix (65 %) or in the |ψ|² fill below', () => {
    let fan = 0
    for (let i = 0; i < 5000; i++) {
      expect(seeds[4 * i] >= 0 && seeds[4 * i] <= SAMPLES - 1).toBe(true)
      if (seeds[4 * i + 2] === 0) fan++
    }
    expect(fan / 5000).toBeCloseTo(0.65, 1)
  })

  it('position, colour and opacity follow ψ: the phase sets the hue, |ψ| the opacity', () => {
    const t = 4
    const f = field(t)
    const PEAK = Math.abs(psi(-12, 0)[0]) // field()'s scale: |ψ| at a packet's centre at t = 0
    for (let i = 0; i < 300; i++) {
      const w = wavePoint(seeds, i, f)
      const col = seeds[4 * i]
      const [re, im] = psi(sampleX(col), t)
      const [fr, fi] = fieldAt(f, col)
      // Interpolating between samples stays within 0.5 % of the peak of the exact ψ.
      expect(Math.hypot(fr - re / PEAK, fi - im / PEAK)).toBeLessThan(0.005)
      const amp = Math.hypot(fr, fi)
      if (seeds[4 * i + 2] === 0 && amp > 1e-6) {
        // On the helix: Re and Im of ψ in the same ratio.
        if (Math.abs(w.pos[1]) > 1e-6) expect((w.pos[2] - seeds[4 * i + 3]) / w.pos[1]).toBeCloseTo(fi / fr, 3)
        expect(w.hue).toBeCloseTo(0.5 + (0.5 * fr) / amp, 5) // ½ + ½ cos(arg ψ)
      }
      if (seeds[4 * i + 2] === 1) {
        // The lower particles fill the area under |ψ|² (field units): height = s · |ψ|² · dens.
        const d = fr ** 2 + fi ** 2
        expect(w.pos[1] - fillBase(f)).toBeCloseTo(seeds[4 * i + 1] * d * WAVE.dens, 6)
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

describe('light', () => {
  it('the same coverage per pixel for any particle count: n · size² · opacity is constant', () => {
    const cover = (n: number) => n * storySize(n) ** 2 * storyAlpha(n)
    for (const n of [25_000, 40_000, 50_000, 80_000]) expect(cover(n) / cover(N_REF)).toBeCloseTo(1, 9)
    expect(storyAlpha(25_000)).toBeLessThanOrEqual(0.35)
  })

  for (const o of ORBITALS)
    it(`${o.name}${o.sub}: weights have mean 1 and dim where points are dense (the nucleus keeps detail)`, () => {
      const s = sample(o, 4000, 3)
      const w = densityWeights(o, s.positions)
      expect(w.reduce((a, v) => a + v, 0) / 4000).toBeCloseTo(1, 6)
      const at = (i: number) => density(o, s.positions[3 * i], s.positions[3 * i + 1], s.positions[3 * i + 2])
      // Monotonic: denser → lighter weight.
      const order = [...w.keys()].sort((i, j) => at(i) - at(j))
      for (let k = 1; k < order.length; k++) expect(w[order[k]]).toBeLessThanOrEqual(w[order[k - 1]] + 1e-6)
      expect(Math.max(...w) / Math.min(...w)).toBeGreaterThan(1.5)
    })
})

describe('spectacle', () => {
  it('particles burst apart when the scene changes, stir between orbitals, and stay put on the wave', () => {
    expect(frameAt(3.75).burst).toBe(BURST.scene) // 4f → the wave
    expect(frameAt(1.75).burst).toBe(BURST.orbital)
    expect(frameAt(5.5).burst).toBe(0)
    expect(BURST.scene).toBeGreaterThan(5 * BURST.orbital)
    for (let i = 0; i < 500; i++) {
      const l = Math.hypot(...burstDir(i))
      expect(l).toBeGreaterThanOrEqual(0.5 - 1e-9)
      expect(l).toBeLessThanOrEqual(1.5 + 1e-9)
    }
  })

  it('the camera dives towards the nucleus in 1s, keeps the 2p nodal plane edge-on, and opens wide into scene 2', () => {
    expect(frameAt(0.5).cam.dist).toBeLessThan(3)
    for (const p of [1, 1.25, 1.5]) expect(Math.abs(frameAt(p).cam.el)).toBeLessThan(0.05)
    expect(frameAt(3.5).cam.dist).toBeGreaterThan(10)
  })
})

describe('the helix and its density', () => {
  const seeds = waveSeeds(6000, 2)
  it('ψ is a thin helix of radius |ψ| around the axis, with |ψ|² filled right below it', () => {
    for (const t of [0, 3, MEET]) {
      const f = field(t)
      let helixLow = 0 // −(widest radius): the bottom of the cylinder the helix turns in
      let fillTop = -Infinity
      for (let i = 0; i < 6000; i++) {
        const w = wavePoint(seeds, i, f)
        const col = seeds[4 * i]
        const amp = Math.hypot(...fieldAt(f, col))
        if (seeds[4 * i + 2] === 0) {
          const r = Math.hypot(w.pos[1], w.pos[2] - seeds[4 * i + 3])
          expect(r / WAVE.amp).toBeGreaterThanOrEqual(0.95 * amp - 1e-6)
          expect(r / WAVE.amp).toBeLessThanOrEqual(amp + 1e-6)
          helixLow = Math.min(helixLow, -r)
        } else fillTop = Math.max(fillTop, w.pos[1])
      }
      // The fill never reaches the helix, and its peak sits just under it (0.15, plus sampling).
      expect(fillTop).toBeLessThan(helixLow)
      expect(helixLow - fillTop).toBeLessThan(0.4)
    }
  })
})

describe('what is on screen', () => {
  const vh = 800
  const stage = { top: 0, bottom: vh }
  it('the stage is clipped to the part of the track on screen', () => {
    expect(stageView({ top: 600, bottom: 9000 }, stage, vh)).toMatchObject({ clipTop: 600, clipBottom: 0 })
    expect(stageView({ top: -50, bottom: 9000 }, stage, vh)).toMatchObject({ clipTop: 0, clipBottom: 0, opacity: 1 })
    expect(stageView({ top: -9000, bottom: 300 }, stage, vh)).toMatchObject({ clipTop: 0, clipBottom: 500 })
    // A phone's stage is the top part of the screen (above the sheet).
    expect(stageView({ top: 100, bottom: 9000 }, { top: 0, bottom: 450 }, vh)).toMatchObject({ clipTop: 100, clipBottom: 0 })
  })

  it('fades in progressively as the track comes in, never all or nothing', () => {
    const op = (top: number) => stageView({ top, bottom: 9000 }, stage, vh).opacity
    expect(op(vh + 10)).toBe(0)
    expect(stageView({ top: vh + 10, bottom: 9000 }, stage, vh).on).toBe(false)
    let prev = 0
    for (let top = vh; top >= 0; top -= 10) {
      expect(op(top)).toBeGreaterThanOrEqual(prev)
      expect(op(top) - prev).toBeLessThan(0.1) // no jump
      prev = op(top)
    }
    expect(op(vh - 40)).toBeGreaterThan(0)
    expect(op(vh * 0.6)).toBe(1)
    expect(trackShare({ top: 200, bottom: 9000 }, vh)).toBeCloseTo(0.75, 9)
  })

  it('a panel shows only while the track lies behind all of it', () => {
    const sheet = { top: 450, bottom: vh }
    expect(covers({ top: 500, bottom: 9000 }, sheet)).toBe(false)
    expect(covers({ top: 440, bottom: 9000 }, sheet)).toBe(true)
    expect(covers({ top: -9000, bottom: vh - 20 }, sheet)).toBe(false)
    expect(covers({ top: 0, bottom: 9000 }, { top: 0, bottom: 0 })).toBe(false) // not laid out (display: none)
  })
})
