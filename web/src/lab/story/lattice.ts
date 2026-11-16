// Scene 7: lattices and learning with errors (LWE), the hard problems behind
// ML-KEM. Pure and exact (integers), no DOM: the figures, the panels and the
// tests all use these numbers.
//
// A 2D lattice is every integer combination a·b₁ + c·b₂ of two basis vectors.
// The same lattice has many bases: a "good" one (short, nearly orthogonal) and
// "bad" ones (long, nearly parallel), related by an integer matrix of
// determinant ±1. Finding the lattice point closest to a target (CVP) is easy
// with a good basis (Babai's rounding: write the target in the basis, round the
// coefficients) and fails with a bad one; in hundreds of dimensions no good
// basis can be found from a bad one in practical time. L. Babai, "On Lovász'
// lattice reduction and the nearest lattice point problem", Combinatorica 6(1),
// 1–13 (1986). LWE: O. Regev, "On lattices, learning with errors, random linear
// codes, and cryptography", J. ACM 56(6), art. 34 (2009).
import { rng } from '../hydrogen.ts'

export type P2 = [number, number]
/** A basis: two column vectors. */
export type Basis = [P2, P2]

export const GOOD: Basis = [
  [3, 1],
  [1, 2],
]
/** An integer matrix with determinant 1: GOOD·U spans the same lattice. */
export const U: [[number, number], [number, number]] = [
  [5, 2],
  [2, 1],
]
/** b₁' = 5b₁ + 2b₂, b₂' = 2b₁ + b₂. */
export const BAD: Basis = [
  [U[0][0] * GOOD[0][0] + U[1][0] * GOOD[1][0], U[0][0] * GOOD[0][1] + U[1][0] * GOOD[1][1]],
  [U[0][1] * GOOD[0][0] + U[1][1] * GOOD[1][0], U[0][1] * GOOD[0][1] + U[1][1] * GOOD[1][1]],
]
/** The target point (not on the lattice). */
export const TARGET: P2 = [3.6, 2.4]

export const det = (b: Basis) => b[0][0] * b[1][1] - b[1][0] * b[0][1]
export const point = (b: Basis, a: number, c: number): P2 => [a * b[0][0] + c * b[1][0], a * b[0][1] + c * b[1][1]]
export const dist = (p: P2, q: P2) => Math.hypot(p[0] - q[0], p[1] - q[1])

/** The target's coordinates in the basis (real), from the inverse of [b₁ b₂]. */
export function coords(b: Basis, t: P2): P2 {
  const d = det(b)
  return [(b[1][1] * t[0] - b[1][0] * t[1]) / d, (-b[0][1] * t[0] + b[0][0] * t[1]) / d]
}

/** Babai's rounding: round the coordinates in the basis to the nearest integers. */
export function babai(b: Basis, t: P2): P2 {
  const [c1, c2] = coords(b, t)
  return point(b, Math.round(c1), Math.round(c2))
}

/** The truly closest lattice point, by exhaustive search (fine in 2D). */
export function closest(b: Basis, t: P2, reach = 40): P2 {
  let best: P2 = [0, 0]
  let bd = Infinity
  for (let a = -reach; a <= reach; a++)
    for (let c = -reach; c <= reach; c++) {
      const p = point(b, a, c)
      const d = dist(p, t)
      if (d < bd) [best, bd] = [p, d]
    }
  return best
}

/** Lattice points in a window [x0, x1] × [y0, y1]. */
export function pointsIn(b: Basis, x0: number, x1: number, y0: number, y1: number, reach = 40): P2[] {
  const out: P2[] = []
  for (let a = -reach; a <= reach; a++)
    for (let c = -reach; c <= reach; c++) {
      const p = point(b, a, c)
      if (p[0] >= x0 && p[0] <= x1 && p[1] >= y0 && p[1] <= y1) out.push(p)
    }
  return out.sort((p, q) => p[1] - q[1] || p[0] - q[0])
}
/** The window the scene shows: wide enough for the target, its closest point and Babai's answer with the bad basis. */
export const WINDOW = { x0: -5, x1: 16, y0: -3, y1: 10 } as const

// ---- LWE, a toy version: b = A·s + e (mod q) ----

/** Toy sizes: n unknowns, m equations, modulus q (ML-KEM: n = 256 per polynomial, q = 3329). */
export const LWE = { n: 4, m: 6, q: 97 } as const
export const mod = (a: number, q: number) => ((a % q) + q) % q

/** A: m×n, uniform mod q, from a fixed seed (the same on every build). */
export const A: number[][] = (() => {
  const r = rng(11)
  return Array.from({ length: LWE.m }, () => Array.from({ length: LWE.n }, () => Math.floor(r() * LWE.q)))
})()
/** The secret s and the small errors e ∈ {−1, 0, 1}. */
export const S: number[] = [23, 71, 8, 54]
export const E: number[] = [1, -1, 0, 1, -1, 1]

export const mulMod = (m: number[][], v: number[], q: number) => m.map((row) => mod(row.reduce((t, a, j) => t + a * v[j], 0), q))
/** b = A·s + e (mod q), and the same without the errors. */
export const B_ERR: number[] = mulMod(A, S, LWE.q).map((v, i) => mod(v + E[i], LWE.q))
export const B_EXACT: number[] = mulMod(A, S, LWE.q)

/** a⁻¹ mod q (q prime), by the extended Euclidean algorithm. */
export function inv(a: number, q: number): number {
  let [r0, r1, t0, t1] = [q, mod(a, q), 0, 1]
  while (r1 !== 0) {
    const k = Math.floor(r0 / r1)
    ;[r0, r1] = [r1, r0 - k * r1]
    ;[t0, t1] = [t1, t0 - k * t1]
  }
  if (r0 !== 1) throw new Error(`${a} has no inverse mod ${q}`)
  return mod(t0, q)
}

/** Gaussian elimination mod q on the first n equations: the unique s with A_n·s = b_n (mod q). */
export function gauss(a: number[][], b: number[], q: number): number[] {
  const n = a[0].length
  const m = a.slice(0, n).map((row, i) => [...row.map((v) => mod(v, q)), mod(b[i], q)])
  for (let c = 0; c < n; c++) {
    const p = m.findIndex((row, i) => i >= c && row[c] !== 0)
    if (p < 0) throw new Error('singular mod q')
    ;[m[c], m[p]] = [m[p], m[c]]
    const k = inv(m[c][c], q)
    m[c] = m[c].map((v) => mod(v * k, q))
    for (let i = 0; i < n; i++)
      if (i !== c && m[i][c] !== 0) {
        const f = m[i][c]
        m[i] = m[i].map((v, j) => mod(v - f * m[c][j], q))
      }
  }
  return m.map((row) => row[n])
}

/** Candidates for s by brute force: q^n (toy: 97⁴). */
export const TOY_SPACE = LWE.q ** LWE.n

/** Residual b − A·x (mod q), centred in (−q/2, q/2]: small (|e|) for the true s, large for a wrong one. */
export const residual = (a: number[][], x: number[], b: number[], q: number) =>
  mulMod(a, x, q).map((v, i) => {
    const r = mod(b[i] - v, q)
    return r > q / 2 ? r - q : r
  })
