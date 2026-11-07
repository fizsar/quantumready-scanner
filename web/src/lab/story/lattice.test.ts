import { describe, expect, it } from 'vitest'
import { A, B_ERR, B_EXACT, BAD, babai, closest, coords, det, dist, E, gauss, GOOD, inv, LWE, mod, mulMod, pointsIn, residual, S, TARGET, TOY_SPACE, U, WINDOW } from './lattice'
import { HYBRID, hex, ML_KEM, N, Q, sizes, spaceDigits, total, X25519 } from './mlkem'

describe('scene 7: a 2D lattice, a good and a bad basis', () => {
  it('both bases span the same lattice: U is an integer matrix of determinant 1, and |det| = 5 for both', () => {
    expect(U[0][0] * U[1][1] - U[0][1] * U[1][0]).toBe(1)
    expect(det(GOOD)).toBe(5)
    expect(det(BAD)).toBe(5)
    expect(BAD).toEqual([
      [17, 9],
      [7, 4],
    ])
    // Each good basis vector has integer coordinates in the bad basis, and vice versa.
    for (const v of GOOD) coords(BAD, v).forEach((c) => expect(Math.abs(c - Math.round(c))).toBeLessThan(1e-9))
    for (const v of BAD) coords(GOOD, v).forEach((c) => expect(Math.abs(c - Math.round(c))).toBeLessThan(1e-9))
  })

  it('the closest lattice point to the target is (4, 3), at 0.72; Babai with the good basis finds it', () => {
    const c = closest(GOOD, TARGET)
    expect(c).toEqual([4, 3])
    expect(closest(BAD, TARGET)).toEqual([4, 3]) // the same lattice
    expect(dist(c, TARGET)).toBeCloseTo(Math.hypot(0.4, 0.6), 12)
    expect(babai(GOOD, TARGET)).toEqual([4, 3])
  })

  it('with the bad basis, Babai’s rounding lands on (14, 8): more than 16 times farther', () => {
    const b = babai(BAD, TARGET)
    expect(b).toEqual([14, 8])
    expect(dist(b, TARGET) / dist([4, 3], TARGET)).toBeGreaterThan(16)
  })

  it('the window shows the target, both answers and the bases’ tips', () => {
    const pts = pointsIn(GOOD, WINDOW.x0, WINDOW.x1, WINDOW.y0, WINDOW.y1)
    for (const p of [
      [4, 3],
      [14, 8],
      [3, 1],
      [1, 2],
      [0, 0],
    ])
      expect(pts).toContainEqual(p)
    // Lattice points: area / det of the window, roughly.
    const area = (WINDOW.x1 - WINDOW.x0) * (WINDOW.y1 - WINDOW.y0)
    expect(pts.length).toBeGreaterThan(area / 5 - 15)
    expect(pts.length).toBeLessThan(area / 5 + 15)
  })
})

describe('scene 7: LWE, b = A·s + e (mod q)', () => {
  it('inverses mod q', () => {
    for (let a = 1; a < LWE.q; a++) expect(mod(a * inv(a, LWE.q), LWE.q)).toBe(1)
  })

  it('without the errors, Gaussian elimination recovers s from n equations', () => {
    expect(gauss(A, B_EXACT, LWE.q)).toEqual(S)
    expect(B_EXACT).toEqual(mulMod(A, S, LWE.q))
  })

  it('with the errors, the same elimination gives a wrong answer, and the true s leaves only the tiny errors', () => {
    const wrong = gauss(A, B_ERR, LWE.q)
    expect(wrong).not.toEqual(S)
    expect(residual(A, S, B_ERR, LWE.q)).toEqual(E)
    // The wrong answer fits its n equations exactly but not the others: the small errors got amplified.
    const r = residual(A, wrong, B_ERR, LWE.q)
    expect(r.slice(0, LWE.n)).toEqual(new Array(LWE.n).fill(0))
    expect(Math.min(...r.slice(LWE.n).map(Math.abs))).toBeGreaterThan(20)
    for (const e of E) expect(Math.abs(e)).toBeLessThanOrEqual(1)
  })

  it('brute force: 97⁴ = 88 529 281 candidates in the toy; ML-KEM-768’s small secrets, 5^768: a 537-digit number', () => {
    expect(TOY_SPACE).toBe(88_529_281)
    expect(2 * ML_KEM['768'].eta1 + 1).toBe(5)
    expect(spaceDigits(ML_KEM['768'])).toBe(537)
    expect(String(5n ** 768n).length).toBe(537) // the same, with exact integers
  })
})

describe('scene 7: ML-KEM (FIPS 203) and X25519MLKEM768 (RFC 10024)', () => {
  it('parameters: n = 256, q = 3329; ML-KEM-768 has k = 3 (768 = k·n coefficients per vector)', () => {
    expect(N).toBe(256)
    expect(Q).toBe(3329)
    expect(ML_KEM['768'].k * N).toBe(768)
  })

  it('sizes from the formulas match FIPS 203 Table 3 for all three parameter sets', () => {
    expect(sizes(ML_KEM['512'])).toEqual({ ek: 800, dk: 1632, ct: 768, ss: 32 })
    expect(sizes(ML_KEM['768'])).toEqual({ ek: 1184, dk: 2400, ct: 1088, ss: 32 })
    expect(sizes(ML_KEM['1024'])).toEqual({ ek: 1568, dk: 3168, ct: 1568, ss: 32 })
  })

  it('X25519MLKEM768: client share 1216 B = 1184 + 32 (ML-KEM first), server 1120 B = 1088 + 32, secret 64 B; group 4588 = 0x11EC', () => {
    expect(HYBRID.client.map((p) => p.bytes)).toEqual([1184, 32])
    expect(total(HYBRID.client)).toBe(1216)
    expect(total(HYBRID.server)).toBe(1120)
    expect(total(HYBRID.secret)).toBe(64)
    expect(HYBRID.client[0].part.startsWith('ML-KEM')).toBe(true)
    expect(HYBRID.secret[0].part.startsWith('ML-KEM')).toBe(true)
    expect(HYBRID.codepoint).toBe(0x11ec)
    expect(hex(HYBRID.codepoint)).toBe('0x11EC')
    expect(X25519.pub).toBe(32)
    // The key share is 38 times X25519's alone.
    expect(Math.round(total(HYBRID.client) / X25519.pub)).toBe(38)
  })
})
