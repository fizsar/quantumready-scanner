// Scenes 3–5: the shapes the particles form for qubits, circuits and Shor's
// algorithm. Every number drawn comes from quantum.ts (Bloch vectors, Bell
// probabilities, amplitudes, f(x), the QFT distribution, the factors) or from
// the curve equation below; this file only lays them out in space. Pure: the
// worker builds the buffers with it, the still images and the tests too.
//
// World units, three.js axes (y up), centred near the origin. Each figure is a
// list of parts (lines, rings, shells, dots); particles are shared among the
// parts by their area, so the light per area is the same everywhere and nothing
// burns, and each part has an emphasis (its light weight). Particles left over
// stay inside the figure, with weight 0 (invisible), ready for the next morph.
import { rng } from '../hydrogen.ts'
import { boxCorners, type Box, type V3 } from './camera.ts'
import { A, B_ERR, BAD, babai, closest, E, GOOD, LWE, pointsIn, S, TARGET, WINDOW, type Basis, type P2 } from './lattice.ts'
import { HYBRID, ML_KEM, N as KEM_N, sizes, X25519 } from './mlkem.ts'
import { GRI_2025 } from './mosca.ts'
import { apply, bell, bloch, blochOf, collapse, f, H, probabilities, Q, qftDistribution, qubit, reduced, SHOR, uniform, zeros } from './quantum.ts'

export type FigureId =
  | 'bloch0'
  | 'blochH'
  | 'blochM'
  | 'bell'
  | 'circuit'
  | 'reg3'
  | 'reg6'
  | 'reg10'
  | 'shorF'
  | 'shorQFT'
  | 'key'
  | 'factors'
  | 'curve'
  | 'curveBroken'
  | 'harvest'
  | 'stored'
  | 'decrypted'
  | 'mosca'
  | 'lattice'
  | 'latticeBad'
  | 'lwe'
  | 'mlkem'
  | 'hybrid'

export const FIGURES: readonly FigureId[] = [
  'bloch0',
  'blochH',
  'blochM',
  'bell',
  'circuit',
  'reg3',
  'reg6',
  'reg10',
  'shorF',
  'shorQFT',
  'key',
  'factors',
  'curve',
  'curveBroken',
  'harvest',
  'stored',
  'decrypted',
  'mosca',
  'lattice',
  'latticeBad',
  'lwe',
  'mlkem',
  'hybrid',
]

/** Share of all particles per unit of area (world units²): about the light of an average orbital. */
export const DENSITY = 1 / 18
/** Thickness of the lines (world units). */
export const LINE = 0.05

/** The circuit's wires: particles flow along x between these ends (the shader moves them). */
export const WIRE = { x0: -3, x1: 3, y: 0.8 } as const

interface Part {
  /** Area it covers, world units² (sets its share of the particles). */
  area: number
  /** Particles per area relative to DENSITY (default 1): sparse scaffolding < 1. */
  dens?: number
  /** Light weight of its particles. */
  emph: number
  /** 0 = indigo (NEGATIVE) … 1 = green (POSITIVE). */
  hue: number
  /** A random point of the part. */
  at: (r: () => number, g: () => number) => V3
  /** Particles flow along the wire (circuit). */
  flow?: boolean
}

// ---- Parts ----

const seg = (a: V3, b: V3, emph: number, hue: number, t = LINE): Part => {
  const len = Math.hypot(b[0] - a[0], b[1] - a[1], b[2] - a[2])
  return {
    area: Math.max(len, t) * t,
    emph,
    hue,
    at: (r, g) => {
      const u = r()
      return [a[0] + (b[0] - a[0]) * u + (g() * t) / 3, a[1] + (b[1] - a[1]) * u + (g() * t) / 3, a[2] + (b[2] - a[2]) * u + (g() * t) / 3]
    },
  }
}
/** A circle of radius rad around c, in the plane spanned by unit vectors u and v. */
const ring = (c: V3, rad: number, u: V3, v: V3, emph: number, hue: number): Part => ({
  area: 2 * Math.PI * rad * LINE,
  emph,
  hue,
  at: (r, g) => {
    const a = 2 * Math.PI * r()
    const k = rad + (g() * LINE) / 3
    return [0, 1, 2].map((i) => c[i] + k * (Math.cos(a) * u[i] + Math.sin(a) * v[i]) + (g() * LINE) / 3) as V3
  },
})
/** A sphere's surface: sparse (a tenth of the density), as a faint scaffold. */
export const SHELL_DENS = 0.1
const shell = (c: V3, rad: number, emph: number, hue: number): Part => ({
  area: 4 * Math.PI * rad * rad,
  dens: SHELL_DENS,
  emph,
  hue,
  at: (r) => {
    const z = 2 * r() - 1
    const a = 2 * Math.PI * r()
    const s = Math.sqrt(1 - z * z)
    return [c[0] + rad * s * Math.cos(a), c[1] + rad * z, c[2] + rad * s * Math.sin(a)]
  },
})
const dot = (c: V3, sigma: number, emph: number, hue: number): Part => ({
  area: Math.PI * (2 * sigma) ** 2,
  emph,
  hue,
  at: (_r, g) => [c[0] + g() * sigma, c[1] + g() * sigma, c[2] + g() * sigma],
})
/** The 12 edges of a cube of side s centred at c. */
const cube = (c: V3, s: number, emph: number, hue: number): Part[] => {
  const h = s / 2
  const out: Part[] = []
  for (const [i, j] of [
    [0, 1],
    [0, 2],
    [1, 2],
  ])
    for (const a of [-h, h])
      for (const b of [-h, h]) {
        const k = 3 - i - j
        const p: V3 = [0, 0, 0]
        const q: V3 = [0, 0, 0]
        p[i] = a
        p[j] = b
        p[k] = -h
        q[i] = a
        q[j] = b
        q[k] = h
        out.push(seg(add(c, p), add(c, q), emph, hue))
      }
  return out
}
const add = (a: V3, b: V3): V3 => [a[0] + b[0], a[1] + b[1], a[2] + b[2]]
const scale = (a: V3, k: number): V3 => [a[0] * k, a[1] * k, a[2] * k]
const X: V3 = [1, 0, 0]
const Y: V3 = [0, 1, 0]
const Z: V3 = [0, 0, 1]

/** Physics (x, y, z), z up, to three's axes (y up): the same mapping as the orbitals. */
export const toThree = (v: [number, number, number]): V3 => [v[0], v[2], -v[1]]

// ---- Figures ----

export const BLOCH_R = 1.5

/** A Bloch sphere at c with the (possibly shorter, mixed) Bloch vector v. */
function blochParts(c: V3, rad: number, v: [number, number, number]): Part[] {
  const w = toThree(v)
  const tip = add(c, scale(w, rad))
  const len = Math.hypot(...v)
  const parts: Part[] = [
    shell(c, rad, 0.9, 0.1),
    ring(c, rad, X, Z, 0.55, 0.25), // equator
    ring(c, rad, X, Y, 0.4, 0.25), // a meridian
    seg(add(c, [0, -1.18 * rad, 0]), add(c, [0, 1.18 * rad, 0]), 0.45, 0.25), // z axis, |0⟩ up
    dot(add(c, [0, rad, 0]), 0.05, 1, 0.6), // |0⟩
    dot(add(c, [0, -rad, 0]), 0.05, 1, 0.6), // |1⟩
    dot(tip, 0.09, 2, 1),
  ]
  if (len > 1e-6) parts.push(seg(c, tip, 1.6, 1, 0.08))
  return parts
}

/** Arc from a to b bulging by `bulge` along up. */
const arc = (a: V3, b: V3, bulge: V3, emph: number, hue: number, share = 1): Part => {
  const len = Math.hypot(b[0] - a[0], b[1] - a[1], b[2] - a[2]) * 1.15
  return {
    area: len * LINE * share,
    emph,
    hue,
    at: (r, g) => {
      const u = r()
      const k = 4 * u * (1 - u)
      return [0, 1, 2].map((i) => a[i] + (b[i] - a[i]) * u + bulge[i] * k + (g() * LINE) / 3) as V3
    },
  }
}

export const BELL_X = 2.1
export const BELL_R = 1.2

function bellParts(): Part[] {
  const s = bell()
  const A: V3 = [-BELL_X, 0, 0]
  const B: V3 = [BELL_X, 0, 0]
  const parts = [...blochParts(A, BELL_R, blochOf(reduced(s, 0))), ...blochParts(B, BELL_R, blochOf(reduced(s, 1)))]
  // The correlation: a link between the poles of equal results, as thick as
  // their joint probability (|00⟩: north–north, |11⟩: south–south; 01 and 10: none).
  const p = probabilities(s)
  for (let i = 0; i < 4; i++) {
    if (p[i] < 1e-9) continue
    const ya = i >> 1 ? -BELL_R : BELL_R
    const yb = i & 1 ? -BELL_R : BELL_R
    const out = ya === yb ? Math.sign(ya) * 0.7 : 0
    parts.push(arc(add(A, [0, ya, 0]), add(B, [0, yb, 0]), [0, out, 0], 1.4, 1, 2 * p[i]))
  }
  return parts
}

function circuitParts(): Part[] {
  const { x0, x1, y } = WIRE
  const parts: Part[] = []
  for (const wy of [y, -y]) {
    parts.push(seg([x0, wy, 0], [x1, wy, 0], 0.35, 0.25))
    parts.push({ ...seg([x0, wy, 0], [x1, wy, 0], 1.3, 1, 0.07), flow: true })
  }
  // H on qubit 0.
  const hx = -1.5
  const hs = 0.36
  for (const [a, b] of [
    [
      [hx - hs, y - hs],
      [hx + hs, y - hs],
    ],
    [
      [hx + hs, y - hs],
      [hx + hs, y + hs],
    ],
    [
      [hx + hs, y + hs],
      [hx - hs, y + hs],
    ],
    [
      [hx - hs, y + hs],
      [hx - hs, y - hs],
    ],
  ])
    parts.push(seg([a[0], a[1], 0], [b[0], b[1], 0], 1.1, 0.6))
  // CNOT: control on qubit 0, target (⊕) on qubit 1.
  const cx = 0.6
  const tr = 0.28
  parts.push(dot([cx, y, 0], 0.07, 1.6, 0.6))
  parts.push(seg([cx, y, 0], [cx, -y - tr, 0], 1.1, 0.6))
  parts.push(ring([cx, -y, 0], tr, X, Y, 1.1, 0.6))
  parts.push(seg([cx - tr, -y, 0], [cx + tr, -y, 0], 1.1, 0.6))
  // The outcome: P(00), P(01), P(10), P(11) of CNOT (H ⊗ I) |00⟩, as bars.
  const p = probabilities(bell())
  for (let i = 0; i < 4; i++) {
    const bx = 3.6 + i * 0.42
    parts.push(seg([bx - 0.12, -1.2, 0], [bx + 0.12, -1.2, 0], 0.5, 0.25))
    if (p[i] > 1e-9) parts.push(seg([bx, -1.2, 0], [bx, -1.2 + 2.4 * p[i], 0], 1.5, 1, 0.1))
  }
  return parts
}

/** Height of an amplitude of 1, in world units. */
export const AMP_H = 3

/** Width of the register's row or grid (world units). */
export const REG_W = 4.8

/** n qubits after H on each: 2^n bars, each as tall as its amplitude (2^(−n/2)). */
function registerParts(n: number): Part[] {
  const s = uniform(n)
  const parts: Part[] = []
  const count = s.length
  const side = n <= 3 ? count : 2 ** Math.ceil(n / 2)
  const rows = Math.ceil(count / side)
  const W = REG_W
  const step = W / Math.max(1, side - 1)
  const t = Math.min(0.08, step * 0.35)
  for (let i = 0; i < count; i++) {
    const cx = -W / 2 + (i % side) * step
    const cz = rows > 1 ? -W / 2 + Math.floor(i / side) * (W / (rows - 1)) : 0
    const h = Math.hypot(...s[i]) * AMP_H
    parts.push(seg([cx, 0, cz], [cx, h, cz], 1.4, 1, t))
    parts.push(dot([cx, 0, cz], t * 0.3, 0.8, 0.35))
  }
  return parts
}

/** f(x) = 7^x mod 15 for x = 0 … 31: the height of each dot is f(x). */
export const F_XS = 32
function shorFParts(): Part[] {
  const parts: Part[] = [seg([-3.2, 0, 0], [3.2, 0, 0], 0.35, 0.25)]
  for (let x = 0; x < F_XS; x++) {
    const px = -3.1 + (x * 6.2) / (F_XS - 1)
    const h = (f(x) / SHOR.N) * 2.8
    parts.push(seg([px, 0, 0], [px, h, 0], 0.45, 0.35, 0.03))
    parts.push(dot([px, h, 0], 0.045, 1.8, 1))
  }
  return parts
}

/** P(y) after the QFT, y = 0 … 255, as bars on a baseline: four peaks of ¼. */
export const QFT_H = 10
function shorQFTParts(): Part[] {
  const p = qftDistribution()
  const parts: Part[] = [seg([-3.25, 0, 0], [3.25, 0, 0], 0.4, 0.25)]
  for (let y = 0; y < Q; y++) {
    if (p[y] < 1e-9) continue
    const px = -3.2 + (y * 6.4) / Q
    parts.push(seg([px, 0, 0], [px, p[y] * QFT_H, 0], 1.6, 1, 0.09))
    parts.push(dot([px, p[y] * QFT_H, 0], 0.08, 1.8, 1))
  }
  return parts
}

/** N = 15 as 15 unit cubes in a row: one number. */
const CUBE = 0.38
function keyParts(): Part[] {
  const parts: Part[] = []
  for (let i = 0; i < SHOR.N; i++) parts.push(...cube([-((SHOR.N - 1) / 2) * CUBE * 1.02 + i * CUBE * 1.02, 0, 0], CUBE, 0.9, 1))
  return parts
}
/** The same 15 cubes rearranged as p rows of q (3 × 5), the rows pulled apart: the factors. */
export const FACTORS: [number, number] = [3, 5]
function factorsParts(): Part[] {
  const [p, q] = FACTORS
  const parts: Part[] = []
  for (let r = 0; r < p; r++)
    for (let c = 0; c < q; c++) parts.push(...cube([(c - (q - 1) / 2) * CUBE * 1.3, ((p - 1) / 2 - r) * CUBE * 2.2, (r - 1) * 0.25], CUBE, 0.9, r === 1 ? 0.6 : 1))
  return parts
}

/**
 * X25519's curve (RFC 7748): y² = x³ + 486662x² + x, the same equation over a
 * field of CURVE_P elements instead of 2^255 − 19: its points are a scatter.
 */
export const CURVE_A = 486662
export const CURVE_P = 101
const curves = new Map<string, [number, number][]>()
export function curvePoints(p = CURVE_P, A = CURVE_A): [number, number][] {
  const known = curves.get(`${p},${A}`)
  if (known) return known
  const a = A % p
  const out: [number, number][] = []
  for (let x = 0; x < p; x++) {
    const rhs = (((x * x) % p) * x + ((a * x) % p) * x + x) % p
    for (let y = 0; y < p; y++) if ((y * y) % p === rhs) out.push([x, y])
  }
  curves.set(`${p},${A}`, out)
  return out
}
const CURVE_W = 5.6
const curveAt = (x: number, y: number): V3 => [-CURVE_W / 2 + (x * CURVE_W) / (CURVE_P - 1), -CURVE_W / 2 + (y * CURVE_W) / (CURVE_P - 1), 0]
function curveParts(broken: boolean): Part[] {
  // Broken: the four quadrants pulled apart and tilted.
  const move = (p: V3, qx: number, qy: number): V3 => {
    if (!broken) return p
    const d: V3 = [qx * 0.55, qy * 0.55, qx * qy * 0.35]
    const t = 0.12 * qx * qy
    return [p[0] * Math.cos(t) - p[1] * Math.sin(t) + d[0], p[0] * Math.sin(t) + p[1] * Math.cos(t) + d[1], d[2]]
  }
  const half = (v: number) => (v < (CURVE_P - 1) / 2 ? -1 : 1)
  const parts: Part[] = []
  for (const [x, y] of curvePoints()) parts.push(dot(move(curveAt(x, y), half(x), half(y)), 0.045, 1.6, 1))
  // The frame of the field's grid, in four corners.
  const h = CURVE_W / 2 + 0.15
  for (const qx of [-1, 1])
    for (const qy of [-1, 1]) {
      const c: V3 = [qx * h, qy * h, 0]
      parts.push(seg(move(c, qx, qy), move([0.15 * qx, qy * h, 0], qx, qy), 0.35, 0.25))
      parts.push(seg(move(c, qx, qy), move([qx * h, 0.15 * qy, 0], qx, qy), 0.35, 0.25))
    }
  return parts
}

// ---- Scene 6: harvest now, decrypt later; Mosca's inequality ----

/** A filled rectangle in the xy plane (a bar, a matrix cell). */
const rect = (x: number, y: number, w: number, h: number, emph: number, hue: number): Part => ({
  area: Math.abs(w * h),
  emph,
  hue,
  at: (r, g) => [x + w * r(), y + h * r(), (g() * LINE) / 3],
})

/** Captured packets in a store: a stack of disks (rings), the packets inside, or flying out once decrypted. */
function storeParts(c: V3, rad: number, open: boolean): Part[] {
  const parts: Part[] = []
  const h = 0.6 * rad
  for (let k = 0; k <= 4; k++) parts.push(ring(add(c, [0, -h + (k * 2 * h) / 4, 0]), rad, X, Z, 0.55, 0.25))
  for (let k = 0; k < 4; k++) {
    const a = (k * Math.PI) / 2 + Math.PI / 4
    const v: V3 = [rad * Math.cos(a), 0, rad * Math.sin(a)]
    parts.push(seg(add(c, [v[0], -h, v[2]]), add(c, [v[0], h, v[2]]), 0.4, 0.25))
  }
  // 12 packets: inside, in three layers; once decrypted, out above the store, each split in two.
  for (let i = 0; i < 12; i++) {
    const a = (i * 2 * Math.PI) / 4 + Math.floor(i / 4) * 0.6
    if (!open) parts.push(dot(add(c, [0.5 * rad * Math.cos(a), -h * 0.6 + Math.floor(i / 4) * h * 0.6, 0.5 * rad * Math.sin(a)]), 0.06, 1.4, 1))
    else {
      const out: V3 = add(c, [1.3 * rad * Math.cos(a), h + 0.35 + 0.25 * Math.floor(i / 4), 1.3 * rad * Math.sin(a)])
      parts.push(dot(add(out, [-0.06, 0, 0]), 0.045, 1.6, 1), dot(add(out, [0.06, 0.04, 0]), 0.045, 1.6, 0.7))
    }
  }
  return parts
}

/** Today's traffic: packets flowing from a client to a server; a tap copies them into a store. */
function harvestParts(): Part[] {
  const y = WIRE.y
  const store: V3 = [0, -1.3, 0]
  return [
    dot([WIRE.x0, y, 0], 0.07, 1.8, 0.6),
    dot([WIRE.x1, y, 0], 0.07, 1.8, 0.6),
    seg([WIRE.x0, y, 0], [WIRE.x1, y, 0], 0.35, 0.25),
    { ...seg([WIRE.x0, y, 0], [WIRE.x1, y, 0], 1.3, 1, 0.08), flow: true },
    seg([0, y, 0], [0, store[1] + 0.6 * 0.8, 0], 0.9, 1, 0.04),
    ...storeParts(store, 0.8, false),
  ]
}

/** Years of the timeline: 0 … 30, along x. */
export const YEARS = 30
const yearX = (t: number, x0 = -3, x1 = 3) => x0 + ((x1 - x0) * t) / YEARS

/** The store kept for years: a timeline beside it, a tick every 5 years. */
function storedParts(): Part[] {
  const x0 = -0.4
  const x1 = 3.4
  const parts = storeParts([-1.8, 0, 0], 0.9, false)
  parts.push(seg([x0, -0.7, 0], [x1, -0.7, 0], 0.5, 0.3))
  for (let t = 0; t <= YEARS; t += 5) parts.push(seg([yearX(t, x0, x1), -0.7, 0], [yearX(t, x0, x1), -0.5, 0], 0.6, 0.3))
  return parts
}

/** Default slider values of the inequality (years): the bars drawn under the timeline. */
export const MOSCA_DEFAULT = { x: 10, y: 5 } as const
/** Height of a likelihood of 1 above the axis (world units). */
export const MOSCA_H = 2.4

/**
 * The inequality on a 0–30-year axis: below it, x then y (the default values);
 * above it, z as the survey gives it: at each horizon the range between the
 * lower- and upper-end averages (an error bar), or a single dot where the
 * report states only one end.
 */
function moscaParts(): Part[] {
  const parts: Part[] = [seg([yearX(0), 0, 0], [yearX(YEARS), 0, 0], 0.5, 0.3)]
  for (let t = 5; t <= YEARS; t += 5) parts.push(seg([yearX(t), 0, 0], [yearX(t), -0.12, 0], 0.6, 0.3))
  for (const a of GRI_2025) {
    const x = yearX(a.years)
    if (a.low !== null && a.high !== null) {
      parts.push(seg([x, a.low * MOSCA_H, 0], [x, a.high * MOSCA_H, 0], 1.5, 1, 0.1))
      parts.push(seg([x - 0.1, a.low * MOSCA_H, 0], [x + 0.1, a.low * MOSCA_H, 0], 1.3, 1), seg([x - 0.1, a.high * MOSCA_H, 0], [x + 0.1, a.high * MOSCA_H, 0], 1.3, 1))
    } else {
      const v = (a.low ?? a.high)!
      parts.push(dot([x, v * MOSCA_H, 0], 0.07, 1.6, 0.8))
    }
  }
  const { x, y } = MOSCA_DEFAULT
  parts.push(seg([yearX(0), -0.5, 0], [yearX(x), -0.5, 0], 1.4, 1, 0.12))
  parts.push(seg([yearX(x), -0.5, 0], [yearX(x + y), -0.5, 0], 1.4, 0.15, 0.12))
  return parts
}

// ---- Scene 7: lattices, LWE, ML-KEM, the hybrid ----

/** World units per lattice unit; the window's centre goes to the origin. */
export const LAT_SCALE = 0.24
const latAt = (p: P2): V3 => [(p[0] - (WINDOW.x0 + WINDOW.x1) / 2) * LAT_SCALE, (p[1] - (WINDOW.y0 + WINDOW.y1) / 2) * LAT_SCALE, 0]

/** The lattice, one basis drawn from the origin, the target and the closest point; with the bad basis, Babai's answer too. */
function latticeParts(basis: Basis): Part[] {
  const parts: Part[] = pointsIn(GOOD, WINDOW.x0, WINDOW.x1, WINDOW.y0, WINDOW.y1).map((p) => dot(latAt(p), 0.04, 1.1, 0.35))
  const o = latAt([0, 0])
  for (const b of basis) {
    parts.push(seg(o, latAt(b), 1.5, 1, 0.06))
    parts.push(dot(latAt(b), 0.07, 1.8, 1))
  }
  const t = latAt(TARGET)
  const c = latAt(closest(GOOD, TARGET))
  parts.push(dot(t, 0.07, 2, 0.6))
  parts.push(ring(c, 0.12, X, Y, 1.4, 1))
  parts.push(seg(t, c, 1.2, 1, 0.03))
  if (basis === BAD) {
    const w = latAt(babai(BAD, TARGET))
    parts.push(ring(w, 0.12, X, Y, 1.4, 0))
    parts.push(seg(t, w, 0.9, 0.1, 0.03))
  }
  return parts
}

/** LWE as cells: A (m×n), s (n), e (m) and b = A·s + e (m), each cell lit by its value (mod q; e from −1 to 1). */
export const CELL = 0.36
export interface Cell {
  x: number
  y: number
  size: number
  value: number
  hue: number
}
export function lweCells(): Cell[] {
  const cells: Cell[] = []
  const gap = CELL * 0.12
  const col = (x: number, values: number[], hue: (v: number) => number) =>
    values.forEach((v, i) => cells.push({ x, y: ((LWE.m - 1) / 2 - i) * CELL - CELL / 2 + gap / 2, size: CELL - gap, value: v, hue: hue(v) }))
  const byValue = (v: number) => v / (LWE.q - 1)
  const w = (LWE.n + 5) * CELL
  let x = -w / 2 - CELL
  for (let j = 0; j < LWE.n; j++, x += CELL) col(x, A.map((row) => row[j]), byValue)
  x += CELL
  col(x, S, byValue)
  x += 2 * CELL
  col(x, E, (v) => (v + 1) / 2)
  x += 2 * CELL
  col(x, B_ERR, byValue)
  return cells
}
const lweParts = (): Part[] => lweCells().map((c) => rect(c.x, c.y, c.size, c.size, 1.1, c.hue))

/** ML-KEM-768: k = 3 polynomials of n = 256 coefficients (768 dots on three rings), and its sizes beside X25519's. */
export const BYTES_W = 3.6
function mlkemParts(): Part[] {
  const p = ML_KEM['768']
  const parts: Part[] = []
  const c: V3 = [-1.9, 0, 0]
  for (let k = 0; k < p.k; k++)
    for (let i = 0; i < KEM_N; i++) {
      const a = (i * 2 * Math.PI) / KEM_N
      const r = 0.55 + 0.32 * k
      parts.push(dot(add(c, [r * Math.cos(a), r * Math.sin(a), 0]), 0.018, 1.3, 1 - 0.25 * k))
    }
  const s = sizes(p)
  const bar = (y: number, bytes: number, hue: number) => parts.push(rect(-0.5, y, (BYTES_W * bytes) / s.ek, 0.24, 1.2, hue))
  bar(0.55, s.ek, 1)
  bar(0.05, s.ct, 0.8)
  bar(-0.45, X25519.pub, 0.3)
  return parts
}

/** X25519MLKEM768 on the wire: client share, server share and shared secret, ML-KEM part first, at one scale. */
export const HYBRID_W = 5.2
function hybridParts(): Part[] {
  const parts: Part[] = []
  const scale = HYBRID_W / (HYBRID.client[0].bytes + HYBRID.client[1].bytes)
  const row = (y: number, ps: readonly { part: string; bytes: number }[]) => {
    let x = -HYBRID_W / 2
    for (const q of ps) {
      const w = Math.max(q.bytes * scale, 0.05)
      parts.push(rect(x, y, w, 0.28, 1.2, q.part.startsWith('ML-KEM') ? 1 : 0.3))
      x += w + 0.05
    }
  }
  row(0.7, HYBRID.client)
  row(0.05, HYBRID.server)
  row(-0.6, HYBRID.secret)
  return parts
}

function parts(id: FigureId): Part[] {
  switch (id) {
    case 'bloch0':
      return blochParts([0, 0, 0], BLOCH_R, bloch(zeros(1)))
    case 'blochH':
      return blochParts([0, 0, 0], BLOCH_R, bloch(apply(H, zeros(1))))
    case 'blochM':
      return blochParts([0, 0, 0], BLOCH_R, bloch(collapse(apply(H, zeros(1)), MEASURED)))
    case 'bell':
      return bellParts()
    case 'circuit':
      return circuitParts()
    case 'reg3':
      return registerParts(3)
    case 'reg6':
      return registerParts(6)
    case 'reg10':
      return registerParts(10)
    case 'shorF':
      return shorFParts()
    case 'shorQFT':
      return shorQFTParts()
    case 'key':
      return keyParts()
    case 'factors':
      return factorsParts()
    case 'curve':
      return curveParts(false)
    case 'curveBroken':
      return curveParts(true)
    case 'harvest':
      return harvestParts()
    case 'stored':
      return storedParts()
    case 'decrypted':
      return storeParts([-0.2, -0.3, 0], 0.9, true)
    case 'mosca':
      return moscaParts()
    case 'lattice':
      return latticeParts(GOOD)
    case 'latticeBad':
      return latticeParts(BAD)
    case 'lwe':
      return lweParts()
    case 'mlkem':
      return mlkemParts()
    case 'hybrid':
      return hybridParts()
  }
}
/** The outcome shown when the qubit (H|0⟩) is measured: 1, the south pole (half the time it is 0). */
export const MEASURED = 1

/** Every point that bounds a figure (its parts' extremes, sampled): its framing hull. */
const hulls = new Map<FigureId, V3[]>()
function hullOf(id: FigureId): V3[] {
  let h = hulls.get(id)
  if (h) return h
  const r = rng(7)
  const pts: V3[] = []
  // The parts' own points, without the lines' thickness (the box adds a pad).
  for (const p of parts(id)) for (let k = 0; k < 24; k++) pts.push(p.at(r, () => 0))
  h = convexEnough(pts)
  hulls.set(id, h)
  return h
}
/** Keeps the points on the outside (the extremes along 26 directions): enough for framing. */
function convexEnough(pts: V3[]): V3[] {
  const dirs: V3[] = []
  for (const x of [-1, 0, 1]) for (const y of [-1, 0, 1]) for (const z of [-1, 0, 1]) if (x || y || z) dirs.push([x, y, z])
  const out = new Set<V3>()
  for (const d of dirs) {
    let best = pts[0]
    for (const p of pts) if (p[0] * d[0] + p[1] * d[1] + p[2] * d[2] > best[0] * d[0] + best[1] * d[1] + best[2] * d[2]) best = p
    out.add(best)
  }
  return [...out]
}

/** The figure's box (its centre is where the camera looks) and hull, padded by the lines' and dots' size. */
const boxes = new Map<FigureId, Box>()
export function figureBox(id: FigureId): Box {
  const known = boxes.get(id)
  if (known) return known
  const pts = hullOf(id)
  const pad = 0.15
  const min: V3 = [0, 1, 2].map((k) => Math.min(...pts.map((p) => p[k])) - pad) as V3
  const max: V3 = [0, 1, 2].map((k) => Math.max(...pts.map((p) => p[k])) + pad) as V3
  // Hull: the box's corners (a tight enough bound for flat figures).
  const box = { min, max, hull: boxCorners({ min, max }) }
  boxes.set(id, box)
  return box
}

const gauss = (r: () => number) => () => {
  const u = Math.max(1e-12, r())
  return Math.sqrt(-2 * Math.log(u)) * Math.cos(2 * Math.PI * r())
}

export interface Built {
  /** x, y, z per particle, world units (three's axes). */
  positions: Float32Array
  /** Per particle: hue, light weight (negative: flows along the wire). */
  looks: Float32Array
}

/**
 * `count` particles for the figure: each part gets particles in proportion to
 * its area (DENSITY of all particles per unit²), the rest stay invisible
 * inside the figure's box.
 */
export function buildFigure(id: FigureId, count: number, seed: number): Built {
  const r = rng(seed * 7919 + FIGURES.indexOf(id) * 104729 + 1)
  const g = gauss(r)
  const ps = parts(id)
  const positions = new Float32Array(count * 3)
  const looks = new Float32Array(count * 2)
  const want = ps.map((p) => p.area * (p.dens ?? 1) * DENSITY * count)
  const total = want.reduce((a, b) => a + b, 0)
  // Never more than all the particles: shrink the shares together if needed.
  const k = total > count ? count / total : 1
  let i = 0
  ps.forEach((p, j) => {
    const n = Math.round(want[j] * k)
    for (let m = 0; m < n && i < count; m++, i++) {
      const q = p.at(r, g)
      positions.set(q, 3 * i)
      looks[2 * i] = p.hue
      looks[2 * i + 1] = p.flow ? -p.emph : p.emph
    }
  })
  // The rest: invisible, spread inside the box.
  const { min, max } = figureBox(id)
  for (; i < count; i++) {
    for (let a = 0; a < 3; a++) positions[3 * i + a] = min[a] + (max[a] - min[a]) * r()
    looks[2 * i] = 0.5
    looks[2 * i + 1] = 0
  }
  // Interleave: a particle's index should not decide its part (the cloud is
  // drawn in index order and may be halved on slow devices: drawRange).
  shuffle(positions, looks, r)
  return { positions, looks }
}

function shuffle(pos: Float32Array, look: Float32Array, r: () => number) {
  const n = look.length / 2
  for (let i = n - 1; i > 0; i--) {
    const j = Math.floor(r() * (i + 1))
    for (let a = 0; a < 3; a++) [pos[3 * i + a], pos[3 * j + a]] = [pos[3 * j + a], pos[3 * i + a]]
    for (let a = 0; a < 2; a++) [look[2 * i + a], look[2 * j + a]] = [look[2 * j + a], look[2 * i + a]]
  }
}

/** For the tests and the panel: the qubit the steps of scene 3 draw. */
export const SCENE3 = {
  zero: bloch(zeros(1)),
  plus: bloch(apply(H, zeros(1))),
  plusProbabilities: probabilities(apply(H, zeros(1))),
  measured: bloch(collapse(apply(H, zeros(1)), MEASURED)),
  general: (t: number, p: number) => bloch(qubit(t, p)),
}
