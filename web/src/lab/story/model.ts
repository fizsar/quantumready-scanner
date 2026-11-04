// The story as data: its steps, what the particles form at each one and
// where the camera is. No three.js and no DOM: the engine (engine.ts), the
// build-time still images (build/storyImages.ts) and the tests share it, so
// a still image is exactly the 3D frame at the start of its step.
//
// One cloud of N particles. Every shape gives each particle a position, a
// "hue" (0 = indigo, 1 = green) and a weight (its share of the light); the
// engine morphs particle i of one shape into particle i of the next on the GPU.
import { density, ORBITALS, rng, type Orbital } from '../hydrogen.ts'
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

/** Depth of field: point size grows and opacity falls with the distance from the focus (orbitals only). */
export const DOF = 0.2

// ---- light ----
//
// Additive points saturate where many overlap. Three things keep the dense
// parts from burning to white, on any screen:
// 1. Coverage per pixel is the same for any particle count: with fewer points
//    (phones) each point grows by (N_REF/n)^¼ and its opacity by (N_REF/n)^½,
//    so n · size² · opacity is constant. Sizes are in drawing-buffer pixels
//    (devicePixelRatio, capped at 1.5, included), and so is the cloud: the
//    ratio, and the light per pixel, do not depend on the DPR.
// 2. Each orbital particle is weighted by the local density of points:
//    w = 1/(1 + ρ/(K⟨ρ⟩)), normalized to mean 1, so the brightest regions are
//    compressed (tone mapping before the sum) and the faint ones keep their light.
// 3. A per-point opacity cap.

/** Particle count the light is tuned for (a desktop). */
export const N_REF = 120_000
const ALPHA_REF = 0.05
const KNEE = 1.2
/** Per-point opacity for n particles (see above). */
export const storyAlpha = (n: number) => Math.min(0.35, ALPHA_REF * Math.sqrt(N_REF / n))
/** Point size factor for n particles (see above). */
export const storySize = (n: number) => (N_REF / n) ** 0.25

/** Light weight of each orbital particle from the local density |ψ|² at it, normalized to mean 1. */
export function densityWeights(o: Pick<Orbital, 'n' | 'l' | 'm'>, positions: Float32Array): Float32Array {
  const n = positions.length / 3
  const rho = new Float64Array(n)
  let mean = 0
  for (let i = 0; i < n; i++) {
    rho[i] = density(o, positions[3 * i], positions[3 * i + 1], positions[3 * i + 2])
    mean += rho[i] / n
  }
  const w = new Float32Array(n)
  let wm = 0
  for (let i = 0; i < n; i++) {
    w[i] = 1 / (1 + rho[i] / (KNEE * mean))
    wm += w[i] / n
  }
  for (let i = 0; i < n; i++) w[i] /= wm
  return w
}

// ---- the steps ----

/** Terms of the Schrödinger equation that scene 2 reveals one by one. */
export type Term = 'psi' | 'dt' | 'kin' | 'pot' | 'sup'

export interface Step {
  id: string
  scene: 1 | 2
  shape: Shape
  /** Where the camera is when the step starts (and its still image). */
  cam: Cam
  /** Where it passes halfway through the step, on its way to the next step's cam. */
  via?: Cam
  /** Scene 2: the term this step explains (lit), and the ones already shown. */
  term?: Term
}

const cam = (az: number, el: number, dist: number, target: [number, number, number] = [0, 0, 0]): Cam => ({ az, el, dist, target, fov: 38 })
/** Scene 2 looks at the wave from three quarters (az ≈ 2π − 0.6, after the turn from the atom). */
const TAU = 2 * Math.PI
const WAVE_T: [number, number, number] = [0, -1.1, 0]

export const STEPS: readonly Step[] = [
  // Into the nucleus and back out.
  { id: '1s', scene: 1, shape: { kind: 'orbital', index: 0 }, cam: cam(0, 0.35, 5), via: cam(0.5, 0.15, 2.6) },
  // The nodal plane edge-on (elevation ~0), turning around it.
  { id: '2pz', scene: 1, shape: { kind: 'orbital', index: 1 }, cam: cam(0.8, 0.03, 5), via: cam(2.0, 0.03, 4.6) },
  // Side view: the two lobes, the ring and, between them, the nodal cones.
  { id: '3dz2', scene: 1, shape: { kind: 'orbital', index: 2 }, cam: cam(2.6, 0.04, 5.2), via: cam(3.3, 0.12, 4.8) },
  // From above at an angle, the eight octants; then a wide shot for the change of scene.
  { id: '4fxyz', scene: 1, shape: { kind: 'orbital', index: 3 }, cam: cam(3.9, 0.62, 5.4), via: cam(4.7, 0.95, 11) },
  { id: 'psi', scene: 2, shape: { kind: 'wave', t0: 0, t1: 0 }, cam: cam(TAU - 0.62, 0.32, 6.8, WAVE_T), term: 'psi' },
  { id: 'dt', scene: 2, shape: { kind: 'wave', t0: 0, t1: 3 }, cam: cam(TAU - 0.78, 0.28, 6.4, WAVE_T), term: 'dt' },
  { id: 'kin', scene: 2, shape: { kind: 'wave', t0: 3, t1: 5 }, cam: cam(TAU - 0.52, 0.22, 6.2, WAVE_T), term: 'kin' },
  { id: 'pot', scene: 2, shape: { kind: 'wave', t0: 5, t1: 5.4 }, cam: cam(TAU - 0.7, 0.42, 6.6, WAVE_T), term: 'pot' },
  { id: 'sup', scene: 2, shape: { kind: 'wave', t0: 5.4, t1: 6 }, cam: cam(TAU - 0.5, 0.26, 6.2, WAVE_T), term: 'sup' },
]

export const TERMS: readonly Term[] = ['psi', 'dt', 'kin', 'pot', 'sup']

/** Order in which terms appear: everything up to the step's own term is shown. */
export function termState(step: Step, term: Term): 'hidden' | 'shown' | 'active' {
  if (!step.term) return 'hidden'
  const a = TERMS.indexOf(step.term)
  const b = TERMS.indexOf(term)
  return b < a ? 'shown' : b === a ? 'active' : 'hidden'
}

type Span = { top: number; bottom: number }

/** How much of the screen (height vh) the story track fills: 0 … 1. */
export const trackShare = (track: Span, vh: number) => Math.max(0, Math.min(vh, track.bottom) - Math.max(0, track.top)) / vh

/**
 * The stage (fixed behind the page) is clipped to the part of the track that
 * is on screen, so it is never behind the intro, the header or the sources,
 * and it fades in as the track comes in: from 0 at its first pixel to full
 * once it fills 40 % of the screen. No all-or-nothing switch, so no empty
 * screen while the track enters or leaves (whatever the browser's toolbars do
 * to the viewport height).
 */
export function stageView(track: Span, stage: Span, vh: number): { clipTop: number; clipBottom: number; opacity: number; on: boolean } {
  const clipTop = Math.max(0, Math.min(stage.bottom, track.top) - stage.top)
  const clipBottom = Math.max(0, stage.bottom - Math.max(stage.top, track.bottom))
  const shown = stage.bottom - stage.top - clipTop - clipBottom
  const f = Math.min(1, trackShare(track, vh) / 0.4)
  const opacity = shown > 0 ? f * f * (3 - 2 * f) : 0
  return { clipTop, clipBottom, opacity, on: opacity > 0 }
}

/** A fixed panel (the phone sheet, the label) shows while the track lies behind all of it. */
export const covers = (track: Span, box: Span) => box.bottom > box.top && track.top <= box.top + 1 && track.bottom >= box.bottom - 1

/** Progress through the track: scrolled viewports, +0.35 so a shape morphs while the next panel comes in. */
export const progressAt = (scrolled: number) => Math.min(STEPS.length - 1e-6, Math.max(0, scrolled + 0.35))

/** The step whose text is shown: the next one from the middle of a morph. */
export function stepAt(p: number) {
  const i = Math.floor(p)
  return Math.min(STEPS.length - 1, p - i > 0.75 ? i + 1 : i)
}

const smooth = (f: number) => f * f * (3 - 2 * f)
const lerp = (a: number, b: number, f: number) => a + (b - a) * f

/** How far particles fly apart in the middle of a morph (world units): a gentle stir between orbitals, a burst from the atom to the wave. */
export const BURST = { orbital: 0.3, scene: 2.4 }

/**
 * What to draw at progress p (0 … STEPS.length): shape A, shape B and the mix
 * between them, the burst of the morph, the wave time if a wave is involved,
 * and the camera. Within a step the camera goes from its cam (through its via)
 * to the next step's cam; a change of shape holds for the first half of the
 * step and morphs in the second; a wave runs its time range.
 */
export function frameAt(p: number): { a: Shape; b: Shape; mix: number; burst: number; t: number; cam: Cam; step: number } {
  const last = STEPS.length - 1
  const q = Math.min(Math.max(p, 0), STEPS.length - 1e-6)
  const i = Math.floor(q)
  const f = q - i
  const cur = STEPS[i]
  const next = STEPS[Math.min(i + 1, last)]
  const cam = camAlong(cur, i === last ? cur.cam : next.cam, f)
  if (cur.shape.kind === 'wave') {
    const t = lerp(cur.shape.t0, cur.shape.t1, f)
    return { a: cur.shape, b: cur.shape, mix: 0, burst: 0, t, cam, step: i }
  }
  if (i === last || shapeKey(next.shape) === shapeKey(cur.shape))
    return { a: cur.shape, b: cur.shape, mix: 0, burst: 0, t: 0, cam, step: i }
  const m = smooth(Math.min(1, Math.max(0, (f - 0.5) * 2)))
  const t = next.shape.kind === 'wave' ? next.shape.t0 : 0
  const burst = next.scene !== cur.scene ? BURST.scene : BURST.orbital
  return { a: cur.shape, b: next.shape, mix: m, burst, t, cam, step: i }
}

/** The camera f of the way through a step: cam → via → end, or cam → end. */
function camAlong(s: Step, end: Cam, f: number): Cam {
  if (!s.via) return camLerp(s.cam, end, smooth(f))
  return f < 0.5 ? camLerp(s.cam, s.via, smooth(f * 2)) : camLerp(s.via, end, smooth((f - 0.5) * 2))
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

/**
 * Direction and strength of particle i's flight in a burst: a unit vector
 * times 0.5 … 1.5, from a hash of i. The engine's vertex shader computes the
 * same from gl_VertexID (engine.ts).
 */
export function burstDir(i: number): [number, number, number] {
  const h = (k: number) => {
    const v = Math.sin((i + 1) * k) * 43758.5453
    return v - Math.floor(v)
  }
  const z = h(12.9898) * 2 - 1
  const a = h(78.233) * 2 * Math.PI
  const r = Math.sqrt(1 - z * z)
  const s = 0.5 + h(37.719)
  return [r * Math.cos(a) * s, z * s, r * Math.sin(a) * s]
}

// ---- the wave as particles (scene 2) ----

/**
 * World mapping of the wave: x ∈ [−20, 20] → [−2.4, 2.4]; ψ as a helix in the
 * (y, z) plane around the x axis, radius |ψ|; |ψ|² as a filled curve right
 * below it (fillBase).
 */
export const WAVE = { xScale: 0.12, amp: 0.9, dens: 0.6, gap: 0.15 }

/**
 * Where the |ψ|² fill starts for this field: its highest point (the peak of
 * |ψ|²) sits `gap` below the lowest point of the helix, at every time.
 */
export function fillBase(field: Float32Array): number {
  let amp = 0
  for (let i = 0; i < field.length; i += 2) amp = Math.max(amp, Math.hypot(field[i], field[i + 1]))
  return -WAVE.amp * amp - WAVE.gap - amp * amp * WAVE.dens
}
/** The wave is drawn with finer points (a crisp helix) and more light per point to make up for it. */
export const WAVE_SIZE = 0.45
export const WAVE_GAIN = 2

/** The helix is a thin tube: particles between 0.95 and 1 of the way from the axis to ψ. */
const TUBE = 0.05

/**
 * ψ at a fractional sample position u ∈ [0, SAMPLES − 1], linearly
 * interpolated between the two nearest samples: the helix is a continuous
 * line, not a dot per sample. The engine's shader does the same.
 */
export function fieldAt(field: Float32Array, u: number): [number, number] {
  const i = Math.min(SAMPLES - 2, Math.max(0, Math.floor(u)))
  const t = u - i
  return [field[2 * i] * (1 - t) + field[2 * i + 2] * t, field[2 * i + 1] * (1 - t) + field[2 * i + 3] * t]
}

/**
 * Per-particle parameters for the wave: where it sits along x (a fractional sample position), a number
 * in 0 … 1 (place across the tube or up the fill), its role (0: on the helix,
 * 1: in the |ψ|² fill) and a small depth jitter.
 */
export function waveSeeds(n: number, seed: number): Float32Array {
  const rand = rng(seed * 7919)
  const out = new Float32Array(n * 4)
  for (let i = 0; i < n; i++) {
    out[4 * i] = rand() * (SAMPLES - 1)
    out[4 * i + 1] = rand()
    out[4 * i + 2] = rand() < 0.65 ? 0 : 1
    out[4 * i + 3] = (rand() - 0.5) * 0.03
  }
  return out
}

/**
 * One wave particle from the field (ψ scaled by its peak at t = 0): position,
 * hue and opacity. The engine's vertex shader does exactly this (engine.ts).
 * On the helix the hue follows the phase: green where ψ is real and positive,
 * indigo where real and negative; opacity follows |ψ| (|ψ|² in the fill), so
 * empty space stays dark.
 */
export function wavePoint(seed: Float32Array, i: number, field: Float32Array, base = fillBase(field)): { pos: [number, number, number]; hue: number; alpha: number } {
  const col = seed[4 * i]
  const s = seed[4 * i + 1]
  const role = seed[4 * i + 2]
  const jit = seed[4 * i + 3]
  const [re, im] = fieldAt(field, col)
  const x = sampleX(col) * WAVE.xScale
  if (role < 0.5) {
    const amp = Math.hypot(re, im)
    const r = 1 - TUBE * s
    return { pos: [x, r * re * WAVE.amp, r * im * WAVE.amp + jit], hue: amp > 0 ? 0.5 + (0.5 * re) / amp : 0.5, alpha: Math.min(1, amp) }
  }
  const d = re * re + im * im
  return { pos: [x + jit, base + s * d * WAVE.dens, jit], hue: 0.85, alpha: Math.min(1, d * 1.2) }
}

/** The orbital a step shows, if any (captions, still images). */
export const orbitalOf = (s: Step) => (s.shape.kind === 'orbital' ? ORBITALS[s.shape.index] : null)

/** Hue of each orbital particle from the sign of ψ: green where ψ > 0, indigo where ψ < 0. */
export const orbitalHues = (signs: Float32Array) => signs.map((v) => (v > 0 ? 1 : 0))
