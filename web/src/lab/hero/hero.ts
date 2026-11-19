// The home's hero scene: the story engine (three.js) playing the entry
// (entry.ts) once, then resting on the Q̂. Loaded lazily with the engine:
// nothing here is in the initial bundle.
import { findProbe, note } from '../probe'
import { SEED } from '../view'
import { createStory } from '../story/engine'
import { ENTRY, ENTRY_SHAPES, entryFrame, phaseAt, type Phase } from './entry'

export interface Hero {
  /**
   * Whether the card is on screen (any of it) and whether enough of it is to
   * play the entry. Off screen nothing is drawn; the entry starts the first
   * time `enough` is true and never restarts.
   */
  setVisible(on: boolean, enough: boolean): void
  dispose(): void
}

export interface HeroOptions {
  points: number
  /** Stop drawing once the entry is over (phones: no idle loop on battery). */
  restStill: boolean
  onReady: () => void
  onError: () => void
  onPhase: (p: Phase) => void
}

/** The card is small: more points than this add light the eye cannot tell apart. */
export const HERO_POINTS = 60_000

export function createHero(canvas: HTMLCanvasElement, opts: HeroOptions): Hero {
  let elapsed = 0
  let armed = false
  let drawn = false
  let phase: Phase = 'handover'
  let done = false
  // For the checks' log only (lab/probe.ts): what the page last said about the card.
  const probe = findProbe()
  let seen: [boolean | null, boolean | null] = [null, null]
  const story = createStory(canvas, {
    points: Math.min(opts.points, HERO_POINTS),
    seed: SEED,
    script: entryFrame,
    waveGain: 0,
    jitter: 0.012,
    pointScale: 2.25,
    only: ENTRY_SHAPES,
    onReady: () => {
      drawn = true
      opts.onReady()
    },
    onError: opts.onError,
    onFrame: (dt) => {
      // The entry's clock starts with the first frame drawn (the still image's
      // own frame) and runs only while frames are drawn: a pause never skips a part of it.
      if (armed && drawn && !done) elapsed += dt
      story.setProgress(elapsed)
      canvas.dataset.entry = String(Math.round(elapsed))
      const now = phaseAt(elapsed)
      if (now !== phase) {
        phase = now
        canvas.dataset.phase = now
        opts.onPhase(now)
      }
      if (opts.restStill && !done && elapsed >= ENTRY.collapse) {
        // This frame is the Q̂ at rest (the breath starts from 0): it stays on the canvas.
        done = true
        note(probe, 'entry over on a phone: the loop stops for good')
        elapsed = ENTRY.collapse
        story.setProgress(elapsed)
        queueMicrotask(() => story.stop())
      }
    },
  })
  canvas.dataset.phase = phase
  return {
    setVisible(on, enough) {
      if (on !== seen[0] || enough !== seen[1]) note(probe, `card on screen: ${on}, enough of it for the entry: ${enough}`)
      seen = [on, enough]
      if (enough) armed = true
      if (on && !done) story.start()
      else story.stop()
    },
    dispose: () => story.dispose(),
  }
}
