import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { StoryOptions } from '../story/engine'
import { ENTRY, ENTRY_SHAPES, entryFrame, type Phase } from './entry'

// The engine (three.js, WebGL) replaced by a recorder: what the hero asks of it.
const engine = { opts: null as StoryOptions | null, progress: 0, running: false, starts: 0, stops: 0, disposed: false }
vi.mock('../story/engine', () => ({
  createStory: (_canvas: HTMLCanvasElement, opts: StoryOptions) => {
    engine.opts = opts
    return {
      setProgress: (p: number) => (engine.progress = p),
      setFrame: () => {},
      start: () => {
        if (!engine.running) engine.starts++
        engine.running = true
      },
      stop: () => {
        if (engine.running) engine.stops++
        engine.running = false
      },
      dispose: () => (engine.disposed = true),
    }
  },
}))
const { createHero, HERO_POINTS } = await import('./hero')

/** One frame of the engine's loop, dt ms after the last (only while it runs, as the real one). */
const frame = (dt = 16) => engine.running && engine.opts!.onFrame!(dt)
const frames = (n: number, dt = 16) => {
  for (let i = 0; i < n; i++) frame(dt)
}

describe("the home's hero scene", () => {
  let canvas: HTMLCanvasElement
  let phases: Phase[]
  const make = (restStill = false) => createHero(canvas, { points: 120_000, restStill, onReady: () => {}, onError: () => {}, onPhase: (p) => phases.push(p) })
  beforeEach(() => {
    Object.assign(engine, { opts: null, progress: 0, running: false, starts: 0, stops: 0, disposed: false })
    canvas = document.createElement('canvas')
    phases = []
  })

  it('plays the entry on the story engine, with only its two shapes and a point count fit for a card', () => {
    make()
    expect(engine.opts!.script).toBe(entryFrame)
    expect(engine.opts!.only).toEqual(ENTRY_SHAPES)
    expect(engine.opts!.points).toBe(HERO_POINTS)
    expect(HERO_POINTS).toBeLessThan(120_000)
  })

  it('draws nothing until the card is on screen', () => {
    const hero = make()
    expect(engine.running).toBe(false)
    hero.setVisible(true, false)
    expect(engine.running).toBe(true)
  })

  it('the entry waits for the first 3D frame and for enough of the card to be on screen', () => {
    const whole = make()
    whole.setVisible(true, true)
    frames(30) // drawing attempts while the shapes are on their way: no frame yet
    expect(engine.progress).toBe(0)
    engine.opts!.onReady()
    frames(10)
    expect(engine.progress).toBe(160)

    Object.assign(engine, { running: false, progress: 0 })
    const hero = make()
    hero.setVisible(true, false)
    engine.opts!.onReady()
    frames(30) // drawn, but only a sliver of the card is visible
    expect(engine.progress).toBe(0)
    hero.setVisible(true, true)
    frames(10)
    expect(engine.progress).toBe(160)
    // Once started it goes on, even if less of the card is visible again.
    hero.setVisible(true, false)
    frames(10)
    expect(engine.progress).toBe(320)
  })

  it('off screen it stops drawing, and picks up exactly where it was: no jump, whatever time has passed', () => {
    const hero = make()
    hero.setVisible(true, true)
    engine.opts!.onReady()
    frames(50)
    const before = engine.progress
    expect(before).toBe(800)
    hero.setVisible(false, false)
    expect(engine.running).toBe(false)
    expect(engine.stops).toBe(1)
    frames(500) // nothing runs while stopped
    expect(engine.progress).toBe(before)
    hero.setVisible(true, true)
    expect(engine.running).toBe(true)
    frame(16)
    expect(engine.progress).toBe(before + 16)
  })

  it('reports each phase once, in order, and mirrors it on the canvas (for the checks)', () => {
    const hero = make()
    hero.setVisible(true, true)
    engine.opts!.onReady()
    frames(Math.ceil((ENTRY.collapse + 500) / 16))
    expect(phases).toEqual(['inhale', 'condense', 'hold', 'collapse', 'rest'])
    expect(canvas.dataset.phase).toBe('rest')
    expect(Number(canvas.dataset.entry)).toBeGreaterThan(ENTRY.collapse)
  })

  it('on a desktop it keeps breathing at rest; on a phone it stops for good on the Q̂ at rest (no idle loop on battery)', async () => {
    const desktop = make(false)
    desktop.setVisible(true, true)
    engine.opts!.onReady()
    frames(Math.ceil((ENTRY.collapse + 2000) / 16))
    await Promise.resolve()
    expect(engine.running).toBe(true)
    expect(engine.progress).toBeGreaterThan(ENTRY.collapse + 1900)

    Object.assign(engine, { running: false, starts: 0, stops: 0, progress: 0 })
    const phone = make(true)
    phone.setVisible(true, true)
    engine.opts!.onReady()
    frames(Math.ceil((ENTRY.collapse + 2000) / 16))
    await Promise.resolve() // the stop comes right after the frame that drew the logo
    expect(engine.running).toBe(false)
    expect(engine.progress).toBe(ENTRY.collapse) // the last frame drawn: the logo, not loosened
    // Scrolling away and back does not start it again.
    phone.setVisible(false, false)
    phone.setVisible(true, true)
    expect(engine.running).toBe(false)
  })

  it('disposes of the engine', () => {
    make().dispose()
    expect(engine.disposed).toBe(true)
  })
})
