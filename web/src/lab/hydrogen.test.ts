import { describe, expect, it } from 'vitest'
import { density, energy, ORBITALS, psi, radial, realY, sample } from './hydrogen'

// Reference values from the closed forms tabulated in Griffiths & Schroeter,
// Introduction to Quantum Mechanics, 3rd ed., §4.2 (radial functions R_nl and
// spherical harmonics), written out by hand here, independently of the
// Laguerre and Legendre recurrences in hydrogen.ts. a₀ = 1.
const { PI, sqrt, exp, cos, sin } = Math
const ref: Record<string, (r: number, t: number, p: number) => number> = {
  // ψ_100 = e^(−r) / √π
  '1s': (r) => exp(-r) / sqrt(PI),
  // R_21 = r e^(−r/2) / √24, Y_10 = √(3/4π) cos θ
  '2pz': (r, t) => (r * exp(-r / 2)) / sqrt(24) * sqrt(3 / (4 * PI)) * cos(t),
  // R_32 = 4 r² e^(−r/3) / (81 √30), Y_20 = √(5/16π) (3cos²θ − 1)
  '3dz2': (r, t) => ((4 * r * r * exp(-r / 3)) / (81 * sqrt(30))) * sqrt(5 / (16 * PI)) * (3 * cos(t) ** 2 - 1),
  // R_43 = r³ e^(−r/4) / (768 √35); f_xyz = √(105/16π) sin²θ cos θ sin 2φ (= ½√(105/π) xyz/r³)
  '4fxyz': (r, t, p) => ((r ** 3 * exp(-r / 4)) / (768 * sqrt(35))) * sqrt(105 / (16 * PI)) * sin(t) ** 2 * cos(t) * sin(2 * p),
}

const points: [number, number, number][] = [
  [0.5, 0.3, 0.2],
  [1.2, -0.7, 2.1],
  [-3, 2.5, -1.5],
  [4, 6, 3],
  [-9, -4, 7],
  [0, 0, 5],
]

describe('hydrogen eigenfunctions', () => {
  for (const o of ORBITALS) {
    it(`ψ and |ψ|² of ${o.name} match the closed form`, () => {
      for (const [x, y, z] of points) {
        const r = Math.hypot(x, y, z)
        const want = ref[o.id](r, Math.acos(z / r), Math.atan2(y, x))
        expect(psi(o, x, y, z)).toBeCloseTo(want, 12)
        expect(density(o, x, y, z) / want ** 2 || 1).toBeCloseTo(1, 10)
      }
    })
  }

  // The prototype's orbitals all have l = n − 1 (no radial nodes, L_0 = 1):
  // these exercise the Laguerre recurrence (degree 1 to 3) and radial nodes.
  it('radial functions with radial nodes (2s, 3s, 3p, 4s) match the tables too', () => {
    for (const r of [0.3, 1, 2.5, 6, 11, 17]) {
      expect(radial(2, 0, r)).toBeCloseTo((1 / sqrt(2)) * (1 - r / 2) * exp(-r / 2), 12)
      expect(radial(3, 0, r)).toBeCloseTo((2 / (3 * sqrt(3))) * (1 - (2 * r) / 3 + (2 * r * r) / 27) * exp(-r / 3), 12)
      expect(radial(3, 1, r)).toBeCloseTo(((8 / (27 * sqrt(6))) * (1 - r / 6)) * r * exp(-r / 3), 12)
      expect(radial(4, 0, r)).toBeCloseTo((1 / 4) * (1 - (3 * r) / 4 + (r * r) / 8 - r ** 3 / 192) * exp(-r / 4), 12)
    }
  })

  it('the real harmonics follow their Cartesian forms (p_x, d_xy, d_x²−y²)', () => {
    for (const [t, p] of [[0.4, 1.1], [1.3, -2], [2.2, 0.7]]) {
      expect(realY(1, 1, t, p)).toBeCloseTo(sqrt(3 / (4 * PI)) * sin(t) * cos(p), 12)
      expect(realY(2, -2, t, p)).toBeCloseTo(sqrt(15 / (16 * PI)) * sin(t) ** 2 * sin(2 * p), 12)
      expect(realY(2, 2, t, p)).toBeCloseTo(sqrt(15 / (16 * PI)) * sin(t) ** 2 * cos(2 * p), 12)
    }
  })

  // ∫ |ψ|² d³r over all space = 1, integrating the 3D density itself in
  // spherical coordinates (Simpson in r and θ, trapezoid in φ, periodic).
  for (const o of ORBITALS) {
    it(`${o.name}: the density is normalized`, () => {
      const R = 10 * o.n * o.n + 20
      const nr = 600, nt = 90, np = 64
      const simpson = (i: number, n: number) => (i === 0 || i === n ? 1 : i % 2 ? 4 : 2)
      let total = 0
      for (let i = 0; i <= nr; i++) {
        const r = (i * R) / nr
        for (let j = 0; j <= nt; j++) {
          const t = (j * PI) / nt
          let ring = 0
          for (let k = 0; k < np; k++) {
            const p = (2 * PI * k) / np
            ring += density(o, r * sin(t) * cos(p), r * sin(t) * sin(p), r * cos(t))
          }
          total += simpson(i, nr) * simpson(j, nt) * ring * r * r * sin(t)
        }
      }
      total *= (R / nr / 3) * (PI / nt / 3) * ((2 * PI) / np)
      expect(total).toBeCloseTo(1, 4)
    })
  }

  it('energies: E_n = −R∞hc / n² (13.6057 eV, CODATA 2022)', () => {
    expect(energy(1)).toBeCloseTo(-13.6057, 4)
    expect(energy(2)).toBeCloseTo(-3.4014, 4)
    expect(energy(4)).toBeCloseTo(-0.85036, 5)
  })
})

describe('sampling |ψ|²', () => {
  // ⟨r⟩ = (a₀/2) [3n² − l(l+1)] (Bethe & Salpeter, Quantum Mechanics of One-
  // and Two-Electron Atoms, §3): the sampled cloud has the right size.
  for (const o of ORBITALS) {
    it(`${o.name}: mean radius and the sign of ψ at every point`, () => {
      const s = sample(o, 40_000, 7)
      let sum = 0
      for (let k = 0; k < 40_000; k++) {
        const [x, y, z] = [s.positions[3 * k], s.positions[3 * k + 1], s.positions[3 * k + 2]]
        sum += Math.hypot(x, y, z)
        const v = psi(o, x, y, z)
        if (Math.abs(v) > 1e-9) expect(s.signs[k]).toBe(Math.sign(v))
      }
      const want = (3 * o.n * o.n - o.l * (o.l + 1)) / 2
      expect(Math.abs(sum / 40_000 - want) / want).toBeLessThan(0.02)
      // Probability of the negative phase: none for 1s, ½ for the odd 2p_z and
      // 4f_xyz, and for 3d_z² the torus |cos θ| < 1/√3: (5/8) ∫ (3c²−1)² dc = 2/(3√3).
      const neg = [...s.signs].filter((v) => v < 0).length / 40_000
      const wantNeg: Record<string, number> = { '1s': 0, '2pz': 0.5, '3dz2': 2 / (3 * sqrt(3)), '4fxyz': 0.5 }
      expect(Math.abs(neg - wantNeg[o.id])).toBeLessThan(0.01)
    })
  }

  it('3s: the sign follows the radial part too (two radial nodes, ⟨r⟩ = 13.5 a₀)', () => {
    const o = { id: '3s', n: 3, l: 0, m: 0, name: '3s', sub: '' }
    const s = sample(o, 20_000, 5)
    let sum = 0
    for (let k = 0; k < 20_000; k++) {
      const [x, y, z] = [s.positions[3 * k], s.positions[3 * k + 1], s.positions[3 * k + 2]]
      sum += Math.hypot(x, y, z)
      const v = psi(o, x, y, z)
      if (Math.abs(v) > 1e-9) expect(s.signs[k]).toBe(Math.sign(v))
    }
    expect(Math.abs(sum / 20_000 - 13.5) / 13.5).toBeLessThan(0.02)
    expect(s.signs.some((v) => v < 0)).toBe(true)
  })

  it('the same seed gives the same cloud (3D and still image agree)', () => {
    const a = sample(ORBITALS[2], 500, 3)
    const b = sample(ORBITALS[2], 500, 3)
    expect(a.positions).toEqual(b.positions)
    expect(sample(ORBITALS[2], 500, 4).positions).not.toEqual(a.positions)
  })

  it('r95 of 1s: P(r < R) = 1 − e^(−2R)(1 + 2R + 2R²) = 0.95', () => {
    const R = sample(ORBITALS[0], 10, 1).r95
    expect(1 - exp(-2 * R) * (1 + 2 * R + 2 * R * R)).toBeCloseTo(0.95, 4)
  })
})
