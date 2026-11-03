// The story as data: its steps, what the particles form at each one and
// where the camera is. No three.js and no DOM: the engine (engine.ts), the
// build-time still images (build/storyImages.ts) and the tests share it, so
// a still image is exactly the 3D frame at the end of its step.
//
// One cloud of N particles. Every shape gives each particle a position and a
// "hue" (0 = indigo, 1 = green) and an opacity; the engine morphs particle i
// of one shape into particle i of the next on the GPU.
import { ORBITALS, rng } from '../hydrogen.ts'
import { SAMPLES, sampleX } from './wave.ts'

export type Shape = { kind: 'orbital'; index: number } | { kind: 'wave'; t0: number; t1: number }

/** Camera on a sphere around a target: azimuth and elevation in radians. */
export interface Cam {
  az: number
  el: number
  dist: number
  target: [number, number, number]
  fov: number
}

/** Depth of field: point size grows and opacity falls with the distance from the focus. */
export const DOF = 0.35
/** Overall particle brightness: below 1 so dense regions glow without burning to white. */
export const GLOW = 0.55
/**
 * Per-particle opacity for n particles: inversely proportional to n, so the
 * cloud is equally bright with 25,000 points on a phone and 120,000 on a desktop.
 */
export const storyAlpha = (n: number) => Math.min(0.9, (0.3 * GLOW * 40_000) / n)

/** Terms of the Schrödinger equation that scene 2 reveals one by one. */
export type Term = 'psi' | 'dt' | 'kin' | 'pot' | 'sup'

export interface Step {
  id: string
  scene: 1 | 2
  shape: Shape
  cam: Cam
  /** Scene 2: the term this step explains (lit), and the ones already shown. */
  term?: Term
}

const atom = (az: number): Cam => ({ az, el: 0.35, dist: 5, target: [0, 0, 0], fov: 38 })

export const STEPS: readonly Step[] = [
  { id: '1s', scene: 1, shape: { kind: 'orbital', index: 0 }, cam: atom(0) },
  { id: '2pz', scene: 1, shape: { kind: 'orbital', index: 1 }, cam: atom(0.9) },
  { id: '3dz2', scene: 1, shape: { kind: 'orbital', index: 2 }, cam: atom(1.8) },
  { id: '4fxyz', scene: 1, shape: { kind: 'orbital', index: 3 }, cam: atom(2.7) },
  { id: 'psi', scene: 2, shape: { kind: 'wave', t0: 0, t1: 0 }, cam: { az: -0.35, el: 0.3, dist: 7, target: [0, -0.3, 0], fov: 38 }, term: 'psi' },
  { id: 'dt', scene: 2, shape: { kind: 'wave', t0: 0, t1: 3 }, cam: { az: -0.75, el: 0.22, dist: 6.6, target: [0, -0.3, 0], fov: 38 }, term: 'dt' },
  { id: 'kin', scene: 2, shape: { kind: 'wave', t0: 3, t1: 5 }, cam: { az: -1.05, el: 0.12, dist: 6.2, target: [0, -0.3, 0], fov: 38 }, term: 'kin' },
  { id: 'pot', scene: 2, shape: { kind: 'wave', t0: 5, t1: 5.4 }, cam: { az: -0.4, el: 0.45, dist: 6.4, target: [0, -0.3, 0], fov: 38 }, term: 'pot' },
  { id: 'sup', scene: 2, shape: { kind: 'wave', t0: 5.4, t1: 6 }, cam: { az: -0.12, el: 0.18, dist: 4.6, target: [0, -0.5, 0], fov: 38 }, term: 'sup' },
]

export const TERMS: readonly Term[] = ['psi', 'dt', 'kin', 'pot', 'sup']

/** Order in which terms appear: everything up to the step's own term is shown. */
export function termState(step: Step, term: Term): 'hidden' | 'shown' | 'active' {
  if (!step.term) return 'hidden'
  const a = TERMS.indexOf(step.term)
  const b = TERMS.indexOf(term)
  return b < a ? 'shown' : b === a ? 'active' : 'hidden'
}

/** Progress through the track: scrolled viewports, +0.35 so a shape morphs while the next panel comes in. */
export const progressAt = (scrolled: number) => Math.min(STEPS.length - 1e-6, Math.max(0, scrolled + 0.35))

/** The step whose text is shown: the next one from the middle of a morph. */
export function stepAt(p: number) {
  const i = Math.floor(p)
  return Math.min(STEPS.length - 1, p - i > 0.75 ? i + 1 : i)
}

const smooth = (f: number) => f * f * (3 - 2 * f)
const lerp = (a: number, b: number, f: number) => a + (b - a) * f

/**
 * What to draw at progress p (0 … STEPS.length): shape A, shape B and the mix
 * between them, the wave time if a wave is involved, and the camera. Within
 * a step the camera glides to the next one; a change of shape holds for the
 * first half of the step and morphs in the second; a wave runs its time range.
 */
export function frameAt(p: number): { a: Shape; b: Shape; mix: number; t: number; cam: Cam; step: number } {
  const last = STEPS.length - 1
  const q = Math.min(Math.max(p, 0), STEPS.length - 1e-6)
  const i = Math.floor(q)
  const f = q - i
  const cur = STEPS[i]
  const next = STEPS[Math.min(i + 1, last)]
  const cam = i === last ? cur.cam : camLerp(cur.cam, next.cam, smooth(f))
  if (cur.shape.kind === 'wave') {
    const t = lerp(cur.shape.t0, cur.shape.t1, f)
    return { a: cur.shape, b: cur.shape, mix: 0, t, cam, step: i }
  }
  if (i === last || shapeKey(next.shape) === shapeKey(cur.shape))
    return { a: cur.shape, b: cur.shape, mix: 0, t: 0, cam, step: i }
  const m = smooth(Math.min(1, Math.max(0, (f - 0.5) * 2)))
  const t = next.shape.kind === 'wave' ? next.shape.t0 : 0
  return { a: cur.shape, b: next.shape, mix: m, t, cam, step: i }
}

/** A short key per shape: equal keys draw the same particles (the wave's time aside). */
export const shapeKey = (s: Shape) => (s.kind === 'orbital' ? `o${s.index}` : 'w')

export function camLerp(a: Cam, b: Cam, f: number): Cam {
  return {
    az: lerp(a.az, b.az, f),
    el: lerp(a.el, b.el, f),
    dist: lerp(a.dist, b.dist, f),
    target: [0, 1, 2].map((k) => lerp(a.target[k], b.target[k], f)) as [number, number, number],
    fov: lerp(a.fov, b.fov, f),
  }
}

/** Camera position for a Cam (y up). */
export function camPos(c: Cam): [number, number, number] {
  return [
    c.target[0] + c.dist * Math.cos(c.el) * Math.sin(c.az),
    c.target[1] + c.dist * Math.sin(c.el),
    c.target[2] + c.dist * Math.cos(c.el) * Math.cos(c.az),
  ]
}

// ---- the wave as particles (scene 2) ----

/** World mapping of the wave: x ∈ [−20, 20] → [−2.4, 2.4]; ψ in the (y, z) plane; |ψ|² below. */
export const WAVE = { xScale: 0.12, amp: 0.9, base: -1.6, dens: 0.55 }

/**
 * Per-particle parameters for the wave: the x sample it belongs to, how far
 * from the axis (0 … 1), its role (0: ψ in the complex plane, 1: |ψ|² below)
 * and a small depth jitter.
 */
export function waveSeeds(n: number, seed: number): Float32Array {
  const rand = rng(seed * 7919)
  const out = new Float32Array(n * 4)
  for (let i = 0; i < n; i++) {
    out[4 * i] = Math.floor(rand() * SAMPLES)
    out[4 * i + 1] = rand()
    out[4 * i + 2] = rand() < 0.7 ? 0 : 1
    out[4 * i + 3] = (rand() - 0.5) * 0.06
  }
  return out
}

/**
 * One wave particle from the field (ψ scaled by its peak at t = 0): position,
 * hue and opacity. The engine's vertex shader does exactly this (engine.ts).
 * Hue follows the phase: green where ψ is real and positive, indigo where
 * real and negative. Opacity follows |ψ| (or |ψ|²), so empty space stays dark.
 */
export function wavePoint(seed: Float32Array, i: number, field: Float32Array): { pos: [number, number, number]; hue: number; alpha: number } {
  const col = seed[4 * i]
  const s = seed[4 * i + 1]
  const role = seed[4 * i + 2]
  const jit = seed[4 * i + 3]
  const re = field[2 * col]
  const im = field[2 * col + 1]
  const x = sampleX(col) * WAVE.xScale
  if (role < 0.5) {
    const amp = Math.hypot(re, im)
    return { pos: [x, s * re * WAVE.amp, s * im * WAVE.amp + jit], hue: amp > 0 ? 0.5 + (0.5 * re) / amp : 0.5, alpha: Math.min(1, amp) }
  }
  const d = re * re + im * im
  return { pos: [x + jit, WAVE.base + s * d * WAVE.dens, jit], hue: 0.85, alpha: Math.min(1, d * 1.2) }
}

/** The orbital a step shows, if any (captions, still images). */
export const orbitalOf = (s: Step) => (s.shape.kind === 'orbital' ? ORBITALS[s.shape.index] : null)

/** Hue of each orbital particle from the sign of ψ: green where ψ > 0, indigo where ψ < 0. */
export const orbitalHues = (signs: Float32Array) => signs.map((v) => (v > 0 ? 1 : 0))
