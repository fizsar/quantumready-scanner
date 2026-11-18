/// <reference types="node" />
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import { shapeKey, STEPS, type FrameState } from '../story/model'
import { ATOM, BREATH, BREATH_MS, COLLAPSE_BURST, ENTRY, ENTRY_SHAPES, entryFrame, FLASH, LOGO, LOOSEN, phaseAt } from './entry'

const tokens = readFileSync(resolve('src/tokens.css'), 'utf8')
const ms = (name: string) => Number(tokens.match(new RegExp(`--${name}:\\s*(\\d+)ms`))![1])

/** How much of each shape a frame shows, and how loose its particles are (the shader's burst term). */
const state = (f: FrameState) => {
  const atom = (shapeKey(f.a) === shapeKey(ATOM) ? 1 - f.mix : 0) + (shapeKey(f.b) === shapeKey(ATOM) ? f.mix : 0)
  const logo = (shapeKey(f.a) === shapeKey(LOGO) ? 1 - f.mix : 0) + (shapeKey(f.b) === shapeKey(LOGO) ? f.mix : 0)
  return { atom, logo, loose: f.burst * Math.sin(Math.PI * f.mix), az: f.cam.az, el: f.cam.el, frame: f.cam.frame, gain: f.gain ?? 1 }
}

describe("the home's entry", () => {
  it('starts on the very frame of the still image: the 1s orbital, the story’s first camera, nothing moved', () => {
    const f = entryFrame(0)
    expect(f).toMatchObject({ a: ATOM, b: ATOM, mix: 0, burst: 0, gain: 1 })
    expect(f.cam).toEqual(STEPS[0].cam)
    expect(STEPS[0].id).toBe('1s') // HeroScene shows story-1s.png
    // And stays there while the canvas fades in over the image.
    expect(state(entryFrame(ENTRY.handover - 1))).toEqual(state(f))
  })

  it('runs its phases in order and is over within 2.5 s of the first 3D frame', () => {
    const order = ['handover', 'inhale', 'condense', 'hold', 'collapse', 'rest']
    const seen: string[] = []
    for (let t = 0; t <= ENTRY.collapse + 10; t++) if (seen[seen.length - 1] !== phaseAt(t)) seen.push(phaseAt(t))
    expect(seen).toEqual(order)
    expect(ENTRY.collapse).toBeLessThanOrEqual(2500)
  })

  it('the collapse lasts --dur-scene and the canvas has faded in (--dur-base) before the collapse starts', () => {
    expect(ENTRY.collapse - ENTRY.hold).toBe(ms('dur-scene'))
    expect(ENTRY.hold).toBeGreaterThan(ms('dur-base'))
  })

  it('never jumps: shape, looseness, camera and light are continuous through every phase and across their boundaries', () => {
    let prev = state(entryFrame(0))
    for (let t = 1; t <= ENTRY.collapse + 2 * BREATH_MS; t++) {
      const now = state(entryFrame(t))
      for (const k of Object.keys(now) as (keyof typeof now)[]) expect(Math.abs(now[k] - prev[k]), `${k} at ${t} ms`).toBeLessThan(0.02)
      prev = now
    }
  })

  it('breathes in, condenses into the orbital (a little brighter on the way), then holds it still', () => {
    const loosest = state(entryFrame(ENTRY.inhale))
    expect(loosest.atom).toBeCloseTo(1, 9)
    expect(loosest.loose).toBeCloseTo(LOOSEN, 6)
    let peak = 0
    for (let t = ENTRY.inhale; t <= ENTRY.condense; t++) {
      const s = state(entryFrame(t))
      expect(s.logo).toBe(0)
      peak = Math.max(peak, s.gain)
    }
    expect(peak).toBeGreaterThan(1.1)
    expect(peak).toBeLessThanOrEqual(1 + FLASH)
    const held = state(entryFrame(ENTRY.hold - 1))
    expect(held).toMatchObject({ atom: 1, logo: 0, loose: 0, gain: 1 })
    expect(held.frame).toBeLessThan(STEPS[0].cam.frame) // closer than the still
    expect(held.frame).toBeGreaterThanOrEqual(1) // never cut by the card
  })

  it('collapses into the Q̂: the orbital stops spinning as it goes, and the logo is head-on at rest', () => {
    expect(entryFrame(ENTRY.hold).spin).toBe(1)
    const mid = entryFrame((ENTRY.hold + ENTRY.collapse) / 2)
    expect(mid).toMatchObject({ a: ATOM, b: LOGO, burst: COLLAPSE_BURST })
    expect(mid.mix).toBeCloseTo(0.5, 2)
    const rest = entryFrame(ENTRY.collapse)
    expect(state(rest)).toMatchObject({ atom: 0, logo: 1, loose: 0 })
    expect(rest.spin).toBe(0)
    expect(Math.abs(rest.cam.az)).toBeLessThan(0.01)
    expect(rest.cam.frame).toBeGreaterThanOrEqual(1)
  })

  it('at rest the Q̂ only breathes: a small, slow loosening that comes back to nothing', () => {
    let most = 0
    for (let t = 0; t <= BREATH_MS; t += 10) {
      const s = state(entryFrame(ENTRY.collapse + t))
      expect(s.logo).toBe(1)
      most = Math.max(most, s.loose)
    }
    expect(most).toBeGreaterThan(0.02)
    expect(most).toBeLessThanOrEqual(COLLAPSE_BURST * Math.sin(Math.PI * BREATH) + 1e-9)
    expect(state(entryFrame(ENTRY.collapse + BREATH_MS)).loose).toBeCloseTo(0, 9)
  })

  it('needs only the two shapes it draws (the worker builds nothing else)', () => {
    expect(ENTRY_SHAPES).toEqual({ orbitals: [0], figures: ['qhat'] })
    for (let t = 0; t < ENTRY.collapse + BREATH_MS; t += 7) {
      const f = entryFrame(t)
      for (const s of [f.a, f.b]) expect([shapeKey(ATOM), shapeKey(LOGO)]).toContain(shapeKey(s))
    }
  })
})
