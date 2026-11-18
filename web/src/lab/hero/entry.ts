// The home's entry, as data: what the cloud in the hero card draws at each
// moment after the 3D takes over from the still image. Pure (no three.js, no
// DOM): the engine plays it (hero.ts) and the tests check it.
//
// The still image is the story's first step (the 1s orbital, its camera): the
// first frame here is that same frame, so the canvas appears over the image
// without a jump. Then the cloud breathes in (loosens), condenses into the 1s
// orbital, holds, collapses into the brand's Q̂ and rests there, breathing.
import type { FigureId } from '../story/figures.ts'
import { bezier, camLerp, EASE, STEPS, type Cam, type FrameState, type Shape } from '../story/model.ts'

/** Where each phase ends, in ms since the 3D took over. */
export const ENTRY = {
  /** The canvas fades in over the still image: nothing moves. */
  handover: 200,
  /** Breathing in: the cloud loosens (--ease-in). */
  inhale: 400,
  /** Condensing into the 1s orbital (--ease-out). */
  condense: 1000,
  /** The orbital, turning slowly. */
  hold: 1250,
  /** The collapse into Q̂ (--dur-scene, --ease-in-out). */
  collapse: 2450,
} as const

export type Phase = 'handover' | 'inhale' | 'condense' | 'hold' | 'collapse' | 'rest'

export const phaseAt = (ms: number): Phase =>
  ms < ENTRY.handover ? 'handover' : ms < ENTRY.inhale ? 'inhale' : ms < ENTRY.condense ? 'condense' : ms < ENTRY.hold ? 'hold' : ms < ENTRY.collapse ? 'collapse' : 'rest'

export const ATOM: Shape = { kind: 'orbital', index: 0 }
export const QHAT_ID: FigureId = 'qhat'
export const LOGO: Shape = { kind: 'figure', id: QHAT_ID }
/** The shapes the worker builds for the entry. */
export const ENTRY_SHAPES = { orbitals: [0], figures: [QHAT_ID] }

/** How far the particles loosen when the cloud breathes in, and in the collapse (world units; model.ts BURST). */
export const LOOSEN = 0.45
export const COLLAPSE_BURST = 0.6
/** The nucleus lights up as the cloud condenses: at most this much more light. */
export const FLASH = 0.15
/** At rest the Q̂ breathes: one breath every BREATH_MS, its particles loosening by at most BREATH of the burst. */
export const BREATH_MS = 5000
export const BREATH = 0.04

/** The still image's camera (the story's first step), a little closer once condensed, and head-on for the logo. */
const START: Cam = STEPS[0].cam
const NEAR: Cam = { ...START, frame: 1.35 }
const FRONT: Cam = { az: 0, el: 0.04, frame: 1, fov: START.fov }

const span = (ms: number, from: number, to: number) => Math.min(1, Math.max(0, (ms - from) / (to - from)))

/** What to draw ms after the 3D took over. */
export function entryFrame(ms: number): FrameState {
  const base = { a: ATOM, b: ATOM, mix: 0, burst: 0, t: 0, cam: START, step: 0, spin: 1, gain: 1 }
  switch (phaseAt(ms)) {
    case 'handover':
      return base
    case 'inhale':
      // Half-way through the "morph" of the orbital into itself, the particles are at their loosest.
      return { ...base, burst: LOOSEN, mix: 0.5 * bezier(EASE.in, span(ms, ENTRY.handover, ENTRY.inhale)) }
    case 'condense': {
      const u = bezier(EASE.out, span(ms, ENTRY.inhale, ENTRY.condense))
      return { ...base, burst: LOOSEN, mix: 0.5 + 0.5 * u, cam: camLerp(START, NEAR, u), gain: 1 + FLASH * Math.sin(Math.PI * u) }
    }
    case 'hold':
      return { ...base, cam: NEAR }
    case 'collapse': {
      const u = bezier(EASE.inOut, span(ms, ENTRY.hold, ENTRY.collapse))
      return { ...base, b: LOGO, mix: u, burst: COLLAPSE_BURST, cam: camLerp(NEAR, FRONT, u), spin: 1 - u }
    }
    case 'rest': {
      const breath = (BREATH / 2) * (1 - Math.cos((2 * Math.PI * (ms - ENTRY.collapse)) / BREATH_MS))
      return { ...base, a: LOGO, b: LOGO, mix: breath, burst: COLLAPSE_BURST, cam: FRONT, spin: 0 }
    }
  }
}
