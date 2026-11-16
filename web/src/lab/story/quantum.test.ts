import { describe, expect, it } from 'vitest'
import {
  apply,
  bell,
  bloch,
  blochOf,
  collapse,
  CNOT,
  f,
  factorsFrom,
  gcd,
  H,
  I2,
  kron,
  kronState,
  modpow,
  norm2,
  order,
  periodFrom,
  probabilities,
  purity,
  Q,
  qftDistribution,
  qubit,
  reduced,
  SHOR,
  uniform,
  zeros,
  zz,
  type State,
} from './quantum'

const close = (a: number[], b: number[], digits = 12) => {
  expect(a).toHaveLength(b.length)
  a.forEach((x, i) => expect(x, `component ${i}`).toBeCloseTo(b[i], digits))
}
const flat = (s: State) => s.flatMap((c) => c)

describe('one qubit: the Bloch sphere (Nielsen & Chuang §1.2)', () => {
  it('|ψ⟩ = cos(θ/2)|0⟩ + e^{iφ} sin(θ/2)|1⟩ is normalised and points at (sin θ cos φ, sin θ sin φ, cos θ)', () => {
    for (const [t, p] of [
      [0, 0],
      [Math.PI / 2, 0],
      [1.1, 2.3],
      [Math.PI, 0.7],
      [2.5, -1.2],
    ]) {
      const s = qubit(t, p)
      expect(norm2(s)).toBeCloseTo(1, 12)
      close(bloch(s), [Math.sin(t) * Math.cos(p), Math.sin(t) * Math.sin(p), Math.cos(t)])
    }
  })

  it('|0⟩ at the north pole; H takes it to the equator (+x) with P(0) = P(1) = ½; measuring collapses it to a pole', () => {
    const z = zeros(1)
    close(bloch(z), [0, 0, 1])
    const plus = apply(H, z)
    close(flat(plus), [Math.SQRT1_2, 0, Math.SQRT1_2, 0])
    close(bloch(plus), [1, 0, 0])
    close(probabilities(plus), [0.5, 0.5])
    close(bloch(collapse(plus, 0)), [0, 0, 1])
    close(bloch(collapse(plus, 1)), [0, 0, -1])
  })

  it('H is its own inverse; H|1⟩ = (|0⟩ − |1⟩)/√2 (−x)', () => {
    close(flat(apply(H, apply(H, qubit(1.3, 0.4)))), flat(qubit(1.3, 0.4)))
    close(bloch(apply(H, [[0, 0], [1, 0]])), [-1, 0, 0])
  })
})

describe('two qubits: the Bell state (Nielsen & Chuang §1.3.6, §2.4.3)', () => {
  it('CNOT (H ⊗ I) |00⟩ = (|00⟩ + |11⟩)/√2', () => {
    close(flat(bell()), [Math.SQRT1_2, 0, 0, 0, 0, 0, Math.SQRT1_2, 0])
    // CNOT flips the target only when the control is 1.
    close(flat(apply(CNOT, [[0, 0], [0, 0], [1, 0], [0, 0]])), [0, 0, 0, 0, 0, 0, 1, 0])
    close(flat(apply(CNOT, [[0, 0], [1, 0], [0, 0], [0, 0]])), [0, 0, 1, 0, 0, 0, 0, 0])
  })

  it('each qubit on its own is maximally mixed: ρ = I/2, Bloch vector 0 (the centre), purity ½', () => {
    for (const q of [0, 1] as const) {
      const rho = reduced(bell(), q)
      close(rho.flatMap((r) => r.flatMap((c) => c)), [0.5, 0, 0, 0, 0, 0, 0.5, 0])
      close(blochOf(rho), [0, 0, 0])
      expect(purity(rho)).toBeCloseTo(0.5, 12)
    }
  })

  it('yet the results always agree: ⟨Z⊗Z⟩ = +1, only 00 and 11, each with ½', () => {
    expect(zz(bell())).toBeCloseTo(1, 12)
    close(probabilities(bell()), [0.5, 0, 0, 0.5])
  })

  it('control: a product state keeps each qubit pure, with its own Bloch vector, and no correlation', () => {
    const s = kronState(qubit(0.9, 0.3), qubit(2.1, -1))
    for (const [q, one] of [
      [0, qubit(0.9, 0.3)],
      [1, qubit(2.1, -1)],
    ] as const) {
      const rho = reduced(s, q)
      expect(purity(rho)).toBeCloseTo(1, 12)
      close(blochOf(rho), bloch(one))
    }
    expect(zz(s)).toBeCloseTo(Math.cos(0.9) * Math.cos(2.1), 12)
    // kron of gates acts like the gates on each qubit.
    close(flat(apply(kron(H, I2), zeros(2))), flat(kronState(apply(H, zeros(1)), zeros(1))))
  })
})

describe('n qubits: H on each one gives 2^n states at once', () => {
  it('2^n amplitudes, each 2^(−n/2)', () => {
    for (let n = 1; n <= 6; n++) {
      const s = uniform(n)
      expect(s).toHaveLength(2 ** n)
      for (const a of s) {
        expect(a[0]).toBeCloseTo(2 ** (-n / 2), 12)
        expect(a[1]).toBe(0)
      }
      expect(norm2(s)).toBeCloseTo(1, 12)
    }
  })
})

describe('Shor with N = 15, a = 7 (Shor 1997; Nielsen & Chuang §5.3)', () => {
  it('f(x) = 7^x mod 15 = 1, 7, 4, 13, 1, …: period r = 4', () => {
    expect([0, 1, 2, 3, 4, 5, 6, 7].map(f)).toEqual([1, 7, 4, 13, 1, 7, 4, 13])
    expect(order(SHOR.a, SHOR.N)).toBe(4)
    expect(gcd(SHOR.a, SHOR.N)).toBe(1) // a must share no factor with N
    expect(modpow(7, 2, 15)).toBe(4)
    expect(modpow(3, 200, 7)).toBe(2) // 3^6 ≡ 1 (mod 7), 200 = 6·33 + 2: 3² = 9 ≡ 2
  })

  it('2^n with n = 8: N² ≤ 2^n < 2N² as Shor asks', () => {
    expect(Q).toBe(256)
    expect(SHOR.N ** 2).toBeLessThanOrEqual(Q)
    expect(Q).toBeLessThan(2 * SHOR.N ** 2)
  })

  it('after the QFT: four peaks of ¼ at the multiples of 2^n/r = 64, and nothing elsewhere', () => {
    const p = qftDistribution()
    expect(p.reduce((a, b) => a + b, 0)).toBeCloseTo(1, 12)
    for (let y = 0; y < Q; y++) expect(p[y], `y = ${y}`).toBeCloseTo(y % 64 === 0 ? 0.25 : 0, 12)
  })

  it('continued fractions recover r from a peak: 64/256 = 1/4 and 192/256 = 3/4 give 4; 128/256 = 1/2 gives only 2 (repeat); 0 says nothing', () => {
    expect(periodFrom(64, Q, SHOR.N)).toBe(4)
    expect(periodFrom(192, Q, SHOR.N)).toBe(4)
    expect(periodFrom(128, Q, SHOR.N)).toBe(2)
    expect(periodFrom(0, Q, SHOR.N)).toBe(1)
    // Off the peaks too: 85/256 ≈ 1/3 gives 3. The order is below N, so a
    // denominator of N or more is never a candidate: 17/256 ≈ 1/15 gives 1, not 15.
    expect(periodFrom(85, Q, SHOR.N)).toBe(3)
    expect(periodFrom(17, Q, SHOR.N)).toBe(1)
  })

  it('r = 4 is even and 7^2 ≢ −1 (mod 15): gcd(7² − 1, 15) = 3 and gcd(7² + 1, 15) = 5, and 3 · 5 = 15', () => {
    const [p, q] = factorsFrom(4)
    expect([p, q]).toEqual([3, 5])
    expect(p * q).toBe(SHOR.N)
    expect(modpow(SHOR.a, 2, SHOR.N)).not.toBe(SHOR.N - 1)
  })
})
