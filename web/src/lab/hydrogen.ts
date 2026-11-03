// Hydrogen-atom eigenfunctions ψ_nlm = R_nl(r) · Y_lm(θ, φ), in atomic units
// (Bohr radius a₀ = 1, infinitely heavy nucleus), and a sampler of |ψ|².
// Pure functions, no DOM: the 3D scene (in a worker), the build-time still
// images (vite.config.ts) and the tests use this same code.
//
// Sources (also cited on the page):
// - Radial part: Griffiths & Schroeter, Introduction to Quantum Mechanics,
//   3rd ed. (2018), §4.2, with the modern (DLMF) convention of the associated
//   Laguerre polynomials:
//     R_nl(r) = √((2/n)³ (n−l−1)! / (2n (n+l)!)) e^(−ρ/2) ρ^l L_(n−l−1)^(2l+1)(ρ),  ρ = 2r/n
// - L_k^(α): three-term recurrence, NIST DLMF §18.9.
// - Associated Legendre functions: recurrences of NIST DLMF §14.10, without
//   the Condon–Shortley phase (−1)^m (DLMF's Ferrers functions include it).
// - Real spherical harmonics: the real combinations of Y_l^m and Y_l^−m
//   (DLMF §14.30): m > 0 → √2 N P_l^m(cos θ) cos(mφ), m < 0 → √2 N P_l^|m| sin(|m|φ).
// - Energies: E_n = −R∞hc / n², R∞hc from CODATA 2022 (NIST).

export interface Orbital {
  /** Short id, also the still image's file name. */
  id: string
  n: number
  l: number
  m: number
  /** Chemists' name, e.g. 2p, and its subscript, e.g. z: 2p_z. */
  name: string
  sub: string
}

/** The prototype's sequence: one orbital per value of l. */
export const ORBITALS: readonly Orbital[] = [
  { id: '1s', n: 1, l: 0, m: 0, name: '1s', sub: '' },
  { id: '2pz', n: 2, l: 1, m: 0, name: '2p', sub: 'z' },
  { id: '3dz2', n: 3, l: 2, m: 0, name: '3d', sub: 'z²' },
  { id: '4fxyz', n: 4, l: 3, m: -2, name: '4f', sub: 'xyz' },
]

/** Rydberg energy R∞hc in eV (CODATA 2022). */
export const RYDBERG_EV = 13.605693122990

/** E_n in eV for a fixed nucleus. */
export const energy = (n: number) => -RYDBERG_EV / (n * n)

function factorial(k: number): number {
  let f = 1
  for (let i = 2; i <= k; i++) f *= i
  return f
}

/** Associated Laguerre polynomial L_k^(α)(x) (DLMF §18.9 recurrence). */
export function laguerre(k: number, alpha: number, x: number): number {
  if (k === 0) return 1
  let prev = 1
  let cur = 1 + alpha - x
  for (let j = 1; j < k; j++) {
    const next = ((2 * j + alpha + 1 - x) * cur - (j + alpha) * prev) / (j + 1)
    prev = cur
    cur = next
  }
  return cur
}

/** Associated Legendre function P_l^m(x), m ≥ 0, without the Condon–Shortley phase. */
export function legendre(l: number, m: number, x: number): number {
  // P_m^m = (2m−1)!! (1−x²)^(m/2)
  let pmm = 1
  const s = Math.sqrt(Math.max(0, 1 - x * x))
  for (let i = 1; i <= m; i++) pmm *= (2 * i - 1) * s
  if (l === m) return pmm
  let pm1 = x * (2 * m + 1) * pmm // P_(m+1)^m
  if (l === m + 1) return pm1
  let pl = 0
  for (let ll = m + 2; ll <= l; ll++) {
    pl = (x * (2 * ll - 1) * pm1 - (ll + m - 1) * pmm) / (ll - m)
    pmm = pm1
    pm1 = pl
  }
  return pl
}

/** Radial function R_nl(r), r in units of a₀. */
export function radial(n: number, l: number, r: number): number {
  const rho = (2 * r) / n
  const norm = Math.sqrt((2 / n) ** 3 * (factorial(n - l - 1) / (2 * n * factorial(n + l))))
  return norm * Math.exp(-rho / 2) * rho ** l * laguerre(n - l - 1, 2 * l + 1, rho)
}

/** Real spherical harmonic Y_lm(θ, φ). */
export function realY(l: number, m: number, theta: number, phi: number): number {
  const am = Math.abs(m)
  const norm = Math.sqrt(((2 * l + 1) / (4 * Math.PI)) * (factorial(l - am) / factorial(l + am)))
  const p = legendre(l, am, Math.cos(theta))
  if (m === 0) return norm * p
  return Math.SQRT2 * norm * p * (m > 0 ? Math.cos(am * phi) : Math.sin(am * phi))
}

/** ψ_nlm at a Cartesian point (units of a₀). */
export function psi(o: Pick<Orbital, 'n' | 'l' | 'm'>, x: number, y: number, z: number): number {
  const r = Math.hypot(x, y, z)
  if (r === 0) return o.l === 0 ? radial(o.n, 0, 0) * realY(0, 0, 0, 0) : 0
  return radial(o.n, o.l, r) * realY(o.l, o.m, Math.acos(z / r), Math.atan2(y, x))
}

/** Probability density |ψ|² in a₀⁻³. */
export const density = (o: Pick<Orbital, 'n' | 'l' | 'm'>, x: number, y: number, z: number) => psi(o, x, y, z) ** 2

/** Deterministic PRNG (mulberry32): the same seed gives the same cloud in 3D and in the still image. */
export function rng(seed: number): () => number {
  let a = seed >>> 0
  return () => {
    a = (a + 0x6d2b79f5) >>> 0
    let t = a
    t = Math.imul(t ^ (t >>> 15), t | 1)
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61)
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

export interface Sample {
  /** x, y, z per point, in a₀. */
  positions: Float32Array
  /** Sign of ψ at each point: +1 or −1. */
  signs: Float32Array
  /** Radius (a₀) inside which the electron is found with 95 % probability. */
  r95: number
}

/** Upper end of the radial grid: the probability beyond it is below 1e-12 for n ≤ 4. */
const rMax = (n: number) => 8 * n * n + 20
const GRID = 4096

/**
 * count points distributed as |ψ_nlm|²: r by inverting the cumulative radial
 * distribution r² R_nl², the direction by rejection against |Y_lm|² (uniform
 * on the sphere). ψ is separable, so the joint density is exactly |ψ|².
 */
export function sample(o: Orbital, count: number, seed = 1): Sample {
  const rand = rng(seed + o.n * 1000 + o.l * 100 + (o.m + 10))
  const radiusAt = radialQuantile(o)
  // max |Y|² on a fine grid (+2 %), for the rejection step.
  let yMax = 0
  for (let i = 0; i <= 180; i++)
    for (let j = 0; j < 360; j++) yMax = Math.max(yMax, realY(o.l, o.m, (i * Math.PI) / 180, (j * Math.PI) / 180) ** 2)
  yMax *= 1.02

  const positions = new Float32Array(count * 3)
  const signs = new Float32Array(count)
  for (let k = 0; k < count; k++) {
    let theta = 0
    let phi = 0
    let y = 0
    for (;;) {
      const c = 2 * rand() - 1
      theta = Math.acos(c)
      phi = 2 * Math.PI * rand()
      y = realY(o.l, o.m, theta, phi)
      if (rand() * yMax <= y * y) break
    }
    const r = radiusAt(rand())
    const st = Math.sin(theta)
    positions[3 * k] = r * st * Math.cos(phi)
    positions[3 * k + 1] = r * st * Math.sin(phi)
    positions[3 * k + 2] = r * Math.cos(theta)
    signs[k] = radial(o.n, o.l, r) * y >= 0 ? 1 : -1
  }
  return { positions, signs, r95: radiusAt(0.95) }
}

/** Radius (a₀) inside which the electron is found with 95 % probability. */
export const r95 = (o: Pick<Orbital, 'n' | 'l'>) => radialQuantile(o)(0.95)

/** u ↦ r with P(radius < r) = u, from the cumulative distribution of r² R_nl² (trapezoids). */
function radialQuantile(o: Pick<Orbital, 'n' | 'l'>): (u: number) => number {
  const top = rMax(o.n)
  const h = top / GRID
  const cdf = new Float64Array(GRID + 1)
  let prev = 0
  for (let i = 1; i <= GRID; i++) {
    const r = i * h
    const p = r * r * radial(o.n, o.l, r) ** 2
    cdf[i] = cdf[i - 1] + ((prev + p) / 2) * h
    prev = p
  }
  const total = cdf[GRID]
  const radiusAt = (u: number) => {
    const target = u * total
    let lo = 0
    let hi = GRID
    while (hi - lo > 1) {
      const mid = (lo + hi) >> 1
      if (cdf[mid] < target) lo = mid
      else hi = mid
    }
    const span = cdf[hi] - cdf[lo]
    return (lo + (span > 0 ? (target - cdf[lo]) / span : 0)) * h
  }
  return radiusAt
}
