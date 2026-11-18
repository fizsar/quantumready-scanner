// The story as data: its steps, what the particles form at each one and
// where the camera is. No three.js and no DOM: the engine (engine.ts), the
// build-time still images (build/storyImages.ts) and the tests share it, so
// a still image is exactly the 3D frame at the start of its step.
//
// One cloud of N particles. Every shape gives each particle a position, a
// "hue" (0 = indigo, 1 = green) and a weight (its share of the light); the
// engine morphs particle i of one shape into particle i of the next on the GPU.
import { density, ORBITALS, rng, rQuantile, type Orbital } from '../hydrogen.ts'
import { RADIUS } from '../view.ts'
import { sphereHull, type Box, type V3 } from './camera.ts'
import { figureBox, type FigureId } from './figures.ts'
import { SAMPLES, sampleX, X_RANGE } from './wave.ts'

/** What the particles form: an orbital, the wave, or a figure of scenes 3–5 (figures.ts). */
export type Shape = { kind: 'orbital'; index: number } | { kind: 'wave'; t0: number; t1: number } | { kind: 'figure'; id: FigureId }

/**
 * Camera on a sphere around the shape's centre: azimuth and elevation in
 * radians, and how far out it frames the shape: 1 = the whole shape just fits
 * the free part of the screen (with its margin, camera.ts fit), more = wider.
 * It never goes closer than 1, so no shape is ever cut by the screen's edges.
 */
export interface Cam {
  az: number
  el: number
  frame: number
  fov: number
}

/** Depth of field: point size grows and opacity falls with the distance from the focus (orbitals only). */
export const DOF = 0.2
/** The blur grows a point to at most this many times its size (its light still falls as 1/blur²). */
export const MAX_BLUR = 1.6
/** No point is ever wider than this share of the drawing area's height (a particle near the camera). */
export const MAX_POINT = 0.03

/**
 * A point's diameter in drawing-buffer pixels: its world size (base = POINT ×
 * focal length × storySize) at its depth, grown by the blur up to MAX_BLUR, and
 * never more than MAX_POINT of the height. The engine's vertex shader does the same.
 */
export const pointPx = (base: number, blur: number, depth: number, bufferHeight: number) =>
  Math.min((base * Math.min(blur, MAX_BLUR)) / depth, MAX_POINT * bufferHeight)

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

/** Diameter of a point in world units (the engine and the stills draw it at that size, whatever the screen). */
export const POINT = 0.05

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

export type Scene = 1 | 2 | 3 | 4 | 5 | 6 | 7
export const SCENES: readonly Scene[] = [1, 2, 3, 4, 5, 6, 7]

export interface Step {
  id: string
  scene: Scene
  shape: Shape
  /** Where the camera is when the step starts (and its still image). */
  cam: Cam
  /** Where it passes halfway through the step, on its way to the next step's cam. */
  via?: Cam
  /** Scene 2: the term this step explains (lit), and the ones already shown. */
  term?: Term
}

const cam = (az: number, el: number, frame: number): Cam => ({ az, el, frame, fov: 38 })
/** Scene 2: a longer lens (less perspective: the helix stays regular from end to end). */
const waveCam = (az: number, el: number): Cam => ({ az: TAU + az, el, frame: 1.08, fov: 16 })
const fig = (id: FigureId): Shape => ({ kind: 'figure', id })
/** Scene 2 looks at the wave from three quarters (az ≈ 2π − 0.6, after the turn from the atom). */
const TAU = 2 * Math.PI

export const STEPS: readonly Step[] = [
  // In towards the nucleus (as close as the whole cloud allows) and back out.
  { id: '1s', scene: 1, shape: { kind: 'orbital', index: 0 }, cam: cam(0, 0.35, 1.6), via: cam(0.5, 0.15, 1) },
  // The nodal plane edge-on (elevation ~0), turning around it.
  { id: '2pz', scene: 1, shape: { kind: 'orbital', index: 1 }, cam: cam(0.8, 0.03, 1.15), via: cam(2.0, 0.03, 1.05) },
  // Side view: the two lobes, the ring and, between them, the nodal cones.
  { id: '3dz2', scene: 1, shape: { kind: 'orbital', index: 2 }, cam: cam(2.6, 0.04, 1.15), via: cam(3.3, 0.12, 1.05) },
  // From above at an angle, the eight octants; then a wide shot for the change of scene.
  { id: '4fxyz', scene: 1, shape: { kind: 'orbital', index: 3 }, cam: cam(3.9, 0.62, 1.15), via: cam(4.7, 0.95, 2.6) },
  // The wave from three quarters: from above (elevation) and nearly side-on, so the
  // helix's turns never overlap on screen and |ψ|² hangs right below it (camera.test.ts).
  { id: 'psi', scene: 2, shape: { kind: 'wave', t0: 0, t1: 0 }, cam: waveCam(-0.18, 0.26), term: 'psi' },
  { id: 'dt', scene: 2, shape: { kind: 'wave', t0: 0, t1: 3 }, cam: waveCam(-0.17, 0.26), term: 'dt' },
  { id: 'kin', scene: 2, shape: { kind: 'wave', t0: 3, t1: 5 }, cam: waveCam(-0.12, 0.24), term: 'kin' },
  { id: 'pot', scene: 2, shape: { kind: 'wave', t0: 5, t1: 5.4 }, cam: waveCam(-0.11, 0.22), term: 'pot' },
  { id: 'sup', scene: 2, shape: { kind: 'wave', t0: 5.4, t1: 6 }, cam: waveCam(-0.1, 0.26), via: waveCam(-0.1, 0.26), term: 'sup' },
  // Scene 3, the qubit: a Bloch sphere seen from above the equator, a little to the side.
  { id: 'q0', scene: 3, shape: fig('bloch0'), cam: cam(0.55, 0.3, 1.1), via: cam(0.75, 0.26, 1.05) },
  { id: 'qH', scene: 3, shape: fig('blochH'), cam: cam(0.9, 0.24, 1.1), via: cam(0.7, 0.3, 1.05) },
  { id: 'qM', scene: 3, shape: fig('blochM'), cam: cam(0.55, 0.22, 1.1) },
  { id: 'bell', scene: 3, shape: fig('bell'), cam: cam(0.18, 0.2, 1.05), via: cam(0.3, 0.24, 1.05) },
  // Scene 4, the computer: the circuit head-on, then the register from above at an angle.
  { id: 'circ', scene: 4, shape: fig('circuit'), cam: cam(0.08, 0.06, 1.05), via: cam(0.18, 0.1, 1.05) },
  { id: 'reg3', scene: 4, shape: fig('reg3'), cam: cam(0.25, 0.3, 1.05) },
  { id: 'reg6', scene: 4, shape: fig('reg6'), cam: cam(0.45, 0.55, 1.05) },
  { id: 'reg10', scene: 4, shape: fig('reg10'), cam: cam(0.6, 0.7, 1.05) },
  // Scene 5, Shor: the plots head-on, the cubes and the curve slightly from the side.
  { id: 'shorF', scene: 5, shape: fig('shorF'), cam: cam(0.06, 0.05, 1.05) },
  { id: 'qft', scene: 5, shape: fig('shorQFT'), cam: cam(-0.06, 0.06, 1.05) },
  { id: 'key', scene: 5, shape: fig('key'), cam: cam(0.3, 0.3, 1.05) },
  { id: 'factors', scene: 5, shape: fig('factors'), cam: cam(0.4, 0.25, 1.05) },
  { id: 'curve', scene: 5, shape: fig('curve'), cam: cam(0.12, 0.08, 1.05) },
  { id: 'broken', scene: 5, shape: fig('curveBroken'), cam: cam(0.3, 0.15, 1.1) },
  // Scene 6, harvest now, decrypt later: the traffic and the store from slightly above, then the inequality head-on.
  { id: 'harvest', scene: 6, shape: fig('harvest'), cam: cam(0.15, 0.22, 1.05) },
  { id: 'stored', scene: 6, shape: fig('stored'), cam: cam(0.3, 0.25, 1.05) },
  { id: 'decrypted', scene: 6, shape: fig('decrypted'), cam: cam(0.35, 0.35, 1.05) },
  { id: 'mosca', scene: 6, shape: fig('mosca'), cam: cam(0.04, 0.05, 1.05) },
  // Scene 7, lattices and ML-KEM: flat figures, nearly head-on.
  { id: 'lattice', scene: 7, shape: fig('lattice'), cam: cam(0.06, 0.08, 1.05) },
  { id: 'cvp', scene: 7, shape: fig('latticeBad'), cam: cam(-0.06, 0.08, 1.05) },
  { id: 'lwe', scene: 7, shape: fig('lwe'), cam: cam(0.1, 0.1, 1.05) },
  { id: 'mlkem', scene: 7, shape: fig('mlkem'), cam: cam(0.12, 0.12, 1.05) },
  { id: 'hybrid', scene: 7, shape: fig('hybrid'), cam: cam(0.05, 0.06, 1.05) },
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

/** Length of the soft edge of the stage where the track begins or ends on screen (px). */
export const EDGE = 72

/**
 * The stage's visible band (from the top of the stage, in px) and a CSS mask
 * that fades it out over EDGE px wherever the track begins or ends on screen:
 * never a straight cut across a shape. At the screen's own edges there is no fade.
 */
export function stageMask(v: { clipTop: number; clipBottom: number }, height: number): { top: number; bottom: number; image: string } {
  const top = v.clipTop
  const bottom = height - v.clipBottom
  const span = Math.max(0, bottom - top)
  const ft = top > 0 ? Math.min(EDGE, span / 2) : 0
  const fb = v.clipBottom > 0 ? Math.min(EDGE, span / 2) : 0
  const image = `linear-gradient(to bottom, transparent ${top}px, #000 ${top + ft}px, #000 ${bottom - fb}px, transparent ${bottom}px)`
  return { top, bottom, image }
}

/** A fixed panel (the phone sheet) shows while the track lies behind all of it. */
export const covers = (track: Span, box: Span) => box.bottom > box.top && track.top <= box.top + 1 && track.bottom >= box.bottom - 1

/** No text panel is shown while the visible scene is shorter than this share of the screen. */
export const PANEL_SCENE = 0.4

/**
 * Whether the text panels may show: only with the scene in place. The visible
 * part of the stage (the track on screen) must be at least PANEL_SCENE of the
 * screen; on phones (a sheet below the stage) the track must also cover the
 * stage's whole zone and the sheet, so the intro has left at the top first.
 * Entering: intro out, scene in, then the panel; leaving, the other way round.
 */
export function panelsShown(track: Span, stage: Span, vh: number, sheet?: Span): boolean {
  const v = stageView(track, stage, vh)
  const top = Math.max(0, stage.top + v.clipTop)
  const bottom = Math.min(vh, stage.bottom - v.clipBottom, sheet ? sheet.top : Infinity)
  if (bottom - top < PANEL_SCENE * vh) return false
  return sheet ? covers(track, { top: stage.top, bottom: sheet.bottom }) : true
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

// ---- motion: the named curves of tokens.css, for what the engine animates ----

/** The control points of the motion tokens (tokens.css; model.test.ts checks they are the same). */
export const EASE = {
  out: [0.16, 1, 0.3, 1],
  inOut: [0.76, 0, 0.24, 1],
  standard: [0.2, 0, 0, 1],
  in: [0.4, 0, 1, 1],
} as const satisfies Record<string, readonly [number, number, number, number]>

/** CSS cubic-bezier(x1, y1, x2, y2) at x ∈ [0, 1]: the same curve the browser draws. */
export function bezier([x1, y1, x2, y2]: readonly [number, number, number, number], x: number): number {
  if (x <= 0) return 0
  if (x >= 1) return 1
  const at = (a: number, b: number, t: number) => 3 * a * t * (1 - t) ** 2 + 3 * b * t * t * (1 - t) + t ** 3
  // x(t) is monotonic for control points in [0, 1]: bisection.
  let lo = 0
  let hi = 1
  for (let k = 0; k < 40; k++) {
    const mid = (lo + hi) / 2
    if (at(x1, x2, mid) < x) lo = mid
    else hi = mid
  }
  return at(y1, y2, (lo + hi) / 2)
}

/** What the engine draws at one moment (frameAt for the story; hero/entry.ts for the home's entry). */
export interface FrameState {
  a: Shape
  b: Shape
  mix: number
  burst: number
  t: number
  cam: Cam
  step: number
  /** How much an orbital spins about its axis, 0 … 1 (default: scene 1 spins, less as it morphs away). */
  spin?: number
  /** Light, relative to the usual (default 1). */
  gain?: number
}

/** How far particles fly apart in the middle of a morph (world units): a gentle stir within a scene, a burst from one scene to the next. */
export const BURST = { orbital: 0.3, scene: 2.4 }

/**
 * What to draw at progress p (0 … STEPS.length): shape A, shape B and the mix
 * between them, the burst of the morph, the wave time if a wave is involved,
 * and the camera. Within a step the camera goes from its cam (through its via)
 * to the next step's cam; a change of shape holds for the first half of the
 * step and morphs in the second; a wave runs its time range.
 */
export function frameAt(p: number): FrameState {
  const last = STEPS.length - 1
  const q = Math.min(Math.max(p, 0), STEPS.length - 1e-6)
  const i = Math.floor(q)
  const f = q - i
  const cur = STEPS[i]
  const next = STEPS[Math.min(i + 1, last)]
  const cam = camAlong(cur, i === last ? cur.cam : next.cam, f)
  // A wave runs its time range through the whole step, morphing or not.
  const tCur = cur.shape.kind === 'wave' ? lerp(cur.shape.t0, cur.shape.t1, f) : 0
  if (i === last || shapeKey(next.shape) === shapeKey(cur.shape))
    return { a: cur.shape, b: cur.shape, mix: 0, burst: 0, t: tCur, cam, step: i }
  const m = smooth(Math.min(1, Math.max(0, (f - 0.5) * 2)))
  const t = next.shape.kind === 'wave' ? next.shape.t0 : tCur
  const burst = next.scene !== cur.scene ? BURST.scene : BURST.orbital
  return { a: cur.shape, b: next.shape, mix: m, burst, t, cam, step: i }
}

/** The camera f of the way through a step: cam → via → end, or cam → end. */
function camAlong(s: Step, end: Cam, f: number): Cam {
  if (!s.via) return camLerp(s.cam, end, smooth(f))
  return f < 0.5 ? camLerp(s.cam, s.via, smooth(f * 2)) : camLerp(s.via, end, smooth((f - 0.5) * 2))
}

/** A short key per shape: equal keys draw the same particles (the wave's time aside). */
export const shapeKey = (s: Shape) => (s.kind === 'orbital' ? `o${s.index}` : s.kind === 'figure' ? `f${s.id}` : 'w')

export function camLerp(a: Cam, b: Cam, f: number): Cam {
  // The azimuth turns the short way round (never a whole turn between two steps).
  const d = b.az - a.az
  const short = d - 2 * Math.PI * Math.round(d / (2 * Math.PI))
  return {
    az: a.az + short * f,
    el: lerp(a.el, b.el, f),
    frame: lerp(a.frame, b.frame, f),
    fov: lerp(a.fov, b.fov, f),
  }
}

// ---- boxes: what the camera must keep in frame ----

/** A sphere of radius r (orbitals spin about their axis: a sphere is what they fill). */
const sphere = (r: number): Box => ({ min: [-r, -r, -r], max: [r, r, r], hull: sphereHull(r) })
/** Share of an orbital's probability inside its box: the faint last half-percent may fall outside. */
export const ORBITAL_SHARE = 0.995
const orbitalBoxes = ORBITALS.map((o) => sphere((RADIUS * rQuantile(o, ORBITAL_SHARE)) / rQuantile(o, 0.95)))

/** The box around the wave for a field: the helix's cylinder and the |ψ|² fill below it. */
export function waveBox(field: Float32Array): Box {
  let amp = 0
  for (let i = 0; i < field.length; i += 2) amp = Math.max(amp, Math.hypot(field[i], field[i + 1]))
  const x = X_RANGE * WAVE.xScale
  const a = WAVE.amp * amp + 0.02
  const base = fillBase(field)
  // The helix turns inside a cylinder of radius a around the x axis; the fill hangs below it.
  const hull: V3[] = []
  for (const xx of [-x, x]) {
    for (let k = 0; k < 16; k++) hull.push([xx, a * Math.cos((k * Math.PI) / 8), a * Math.sin((k * Math.PI) / 8)])
    hull.push([xx, base, -0.02], [xx, base, 0.02])
  }
  return { min: [-x, base, -a], max: [x, a, a], hull }
}

/** The box around a shape (the wave's depends on its field at the moment). */
export const shapeBox = (s: Shape, field?: Float32Array): Box =>
  s.kind === 'orbital' ? orbitalBoxes[s.index] : s.kind === 'figure' ? figureBox(s.id) : waveBox(field!)

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
