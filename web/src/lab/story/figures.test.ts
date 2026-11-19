import { describe, expect, it } from 'vitest'
import { AMP_H, BELL_R, BELL_X, BLOCH_R, buildFigure, curvePoints, CURVE_A, CURVE_P, DENSITY, FACTORS, F_XS, figureBox, FIGURES, QFT_H, REG_W, toThree, WIRE, type FigureId } from './figures'
import { apply, bloch, collapse, f, H, zeros } from './quantum'

const N = 60_000
const built = new Map<FigureId, ReturnType<typeof buildFigure>>()
const get = (id: FigureId) => {
  if (!built.has(id)) built.set(id, buildFigure(id, N, 1))
  return built.get(id)!
}
/** Particles whose light weight is w (each role of a figure has its own weight). */
const withWeight = (id: FigureId, w: number) => {
  const { positions, looks } = get(id)
  const out: [number, number, number][] = []
  for (let i = 0; i < N; i++) if (Math.abs(looks[2 * i + 1] - w) < 1e-6) out.push([positions[3 * i], positions[3 * i + 1], positions[3 * i + 2]])
  return out
}
const mean = (ps: [number, number, number][]) => [0, 1, 2].map((k) => ps.reduce((t, p) => t + p[k], 0) / ps.length)
const median = (v: number[]) => {
  const s = [...v].sort((a, b) => a - b)
  return s[Math.floor(s.length / 2)]
}

describe('figures: what the particles draw comes from quantum.ts', () => {
  it('every figure: deterministic, every particle inside its box, the visible share never squeezed (same light per area everywhere)', () => {
    for (const id of FIGURES) {
      const a = get(id)
      const b = buildFigure(id, N, 1)
      expect(a.positions.every((v, i) => v === b.positions[i]), id).toBe(true)
      const { min, max } = figureBox(id)
      let visible = 0
      let outside = 0
      let badHue = 0
      for (let i = 0; i < N; i++) {
        for (let k = 0; k < 3; k++) if (a.positions[3 * i + k] < min[k] - 0.2 || a.positions[3 * i + k] > max[k] + 0.2) outside++
        if (a.looks[2 * i + 1] !== 0) visible++
        if (!(a.looks[2 * i] >= 0 && a.looks[2 * i] <= 1)) badHue++
      }
      expect(outside, `${id} outside its box`).toBe(0)
      expect(badHue, id).toBe(0)
      // Some light, and never all particles (then the shares would have been squeezed: brighter per area).
      expect(visible, id).toBeGreaterThan(0.02 * N)
      expect(visible, id).toBeLessThan(N)
    }
  })

  it('only the circuit and the captured traffic have flowing particles, and they lie on the wire(s)', () => {
    for (const id of FIGURES) {
      const { positions, looks } = get(id)
      for (let i = 0; i < N; i++)
        if (looks[2 * i + 1] < 0) {
          expect(['circuit', 'harvest']).toContain(id)
          // (the captured traffic's packets have a size: a few sigmas of their dot)
          expect(Math.abs(Math.abs(positions[3 * i + 1]) - WIRE.y)).toBeLessThan(0.3)
          expect(positions[3 * i]).toBeGreaterThanOrEqual(WIRE.x0 - 0.3)
          expect(positions[3 * i]).toBeLessThanOrEqual(WIRE.x1 + 0.3)
        }
    }
    expect(withWeight('circuit', -1.3).length).toBeGreaterThan(100)
  })

  it('scene 3: the vector’s tip at |0⟩ (north), after H on the equator (+x), after measuring 1 at the south pole', () => {
    const cases: [FigureId, [number, number, number]][] = [
      ['bloch0', bloch(zeros(1))],
      ['blochH', bloch(apply(H, zeros(1)))],
      ['blochM', bloch(collapse(apply(H, zeros(1)), 1))],
    ]
    const expected: Record<string, number[]> = { bloch0: [0, BLOCH_R, 0], blochH: [BLOCH_R, 0, 0], blochM: [0, -BLOCH_R, 0] }
    for (const [id, v] of cases) {
      const tip = mean(withWeight(id, 2))
      const want = toThree(v).map((x) => x * BLOCH_R)
      tip.forEach((x, k) => expect(x, `${id} tip`).toBeCloseTo(want[k], 1))
      tip.forEach((x, k) => expect(x, `${id} tip`).toBeCloseTo(expected[id][k], 1))
    }
  })

  it('scene 3, Bell: both vectors shrink to their sphere’s centre; links only between equal results (00 and 11), equally thick', () => {
    const tips = withWeight('bell', 2)
    const left = mean(tips.filter((p) => p[0] < 0))
    const right = mean(tips.filter((p) => p[0] > 0))
    ;[-BELL_X, 0, 0].forEach((x, k) => expect(left[k]).toBeCloseTo(x, 1))
    ;[BELL_X, 0, 0].forEach((x, k) => expect(right[k]).toBeCloseTo(x, 1))
    // No vector line (weight 1.6) in the Bell figure: the reduced states have no direction.
    expect(withWeight('bell', 1.6)).toHaveLength(0)
    const link = withWeight('bell', 1.4)
    const top = link.filter((p) => p[1] > 0)
    const bottom = link.filter((p) => p[1] < 0)
    expect(top.length).toBeGreaterThan(200)
    expect(Math.abs(top.length - bottom.length) / link.length).toBeLessThan(0.05)
    // Mid-way the links run above the north poles and below the south poles: none crosses.
    for (const p of link) if (Math.abs(p[0]) < 0.3) expect(Math.abs(p[1])).toBeGreaterThan(BELL_R)
  })

  it('scene 4: the circuit’s outcome bars are P(00) = P(11) = ½ (and nothing for 01, 10)', () => {
    const bars = withWeight('circuit', 1.5)
    const xs = [...new Set(bars.map((p) => Math.round(p[0] / 0.42 - 3.6 / 0.42)))].sort()
    expect(xs).toEqual([0, 3])
    const h = Math.max(...bars.map((p) => p[1])) + 1.2
    expect(h).toBeCloseTo(2.4 * 0.5, 1)
  })

  it('scene 4: n qubits after H give 2^n bars, each as tall as its amplitude 2^(−n/2)', () => {
    for (const n of [3, 6, 10]) {
      const id = `reg${n}` as FigureId
      const bars = withWeight(id, 1.4)
      // The top of the bars (99th percentile: the lines have a thickness) at the amplitude, within a line's width.
      const ys = bars.map((p) => p[1]).sort((a, b) => a - b)
      expect(Math.abs(ys[Math.floor(0.99 * ys.length)] - AMP_H * 2 ** (-n / 2)), id).toBeLessThan(0.06)
      // Every node of the row (n = 3) or grid has a foot: 2^n of them.
      const side = n <= 3 ? 2 ** n : 2 ** Math.ceil(n / 2)
      const rows = 2 ** n / side
      const step = REG_W / (side - 1)
      const zstep = rows > 1 ? REG_W / (rows - 1) : 1
      const nodes = new Map<string, number>()
      for (const p of withWeight(id, 0.8)) {
        const key = `${Math.round((p[0] + REG_W / 2) / step)},${rows > 1 ? Math.round((p[2] + REG_W / 2) / zstep) : 0}`
        nodes.set(key, (nodes.get(key) ?? 0) + 1)
      }
      // (a straggler of the jitter may fall nearer a neighbour: count the nodes with a real foot)
      expect([...nodes.values()].filter((c) => c >= 3).length, id).toBe(2 ** n)
    }
  })

  it('scene 5: f(x) = 7^x mod 15 drawn with period 4; after the QFT, light only at y = 0, 64, 128, 192, each ¼', () => {
    // Each column x = 0 … 31: the dot's (median) height is f(x).
    const dots = withWeight('shorF', 1.8)
    const dx = 6.2 / (F_XS - 1)
    for (let x = 0; x < F_XS; x++) {
      const col = dots.filter((p) => Math.round((p[0] + 3.1) / dx) === x).map((p) => p[1])
      expect(median(col) * (15 / 2.8), `x = ${x}`).toBeCloseTo(f(x), 0)
    }
    expect([0, 1, 2, 3, 4, 5, 6, 7].map(f)).toEqual([1, 7, 4, 13, 1, 7, 4, 13])
    // After the QFT: every lit bar at one of the four peaks, and all four lit.
    const peaks = withWeight('shorQFT', 1.6)
    const at = [0, 64, 128, 192].map((y) => -3.2 + (y * 6.4) / 256)
    expect(peaks.filter((p) => Math.min(...at.map((x) => Math.abs(p[0] - x))) > 0.15)).toHaveLength(0)
    for (const x of at) expect(peaks.filter((p) => Math.abs(p[0] - x) < 0.1).length).toBeGreaterThan(20)
    const tops = peaks.map((p) => p[1]).sort((a, b) => a - b)
    expect(Math.abs(tops[Math.floor(0.99 * tops.length)] - 0.25 * QFT_H)).toBeLessThan(0.06)
  })

  it('scene 5: 15 cubes in a row, then 3 rows of 5', () => {
    expect(FACTORS[0] * FACTORS[1]).toBe(15)
    const key = figureBox('key')
    const fac = figureBox('factors')
    expect(key.max[0] - key.min[0]).toBeGreaterThan(5 * (key.max[1] - key.min[1]))
    expect(fac.max[1] - fac.min[1]).toBeGreaterThan(0.5 * (fac.max[0] - fac.min[0]))
  })

  it('scene 5: X25519’s equation y² = x³ + 486662x² + x over a small field: every point on it, the group order within Hasse’s bound and a multiple of 4', () => {
    const pts = curvePoints()
    const a = CURVE_A % CURVE_P
    for (const [x, y] of pts) expect((y * y) % CURVE_P).toBe((x * x * x + a * x * x + x) % CURVE_P)
    expect(pts).toContainEqual([0, 0]) // the point of order 2
    const order = pts.length + 1 // plus the point at infinity
    expect(Math.abs(order - (CURVE_P + 1))).toBeLessThanOrEqual(2 * Math.sqrt(CURVE_P))
    expect(order % 4).toBe(0)
    // Control: brute force over all pairs agrees on the count.
    let count = 0
    for (let x = 0; x < CURVE_P; x++) for (let y = 0; y < CURVE_P; y++) if ((y * y - (x * x * x + a * x * x + x)) % CURVE_P === 0) count++
    expect(count).toBe(pts.length)
    expect(withWeight('curve', 1.6).length).toBeGreaterThan(0)
  })

  it('light per area: each part gets its particles by area (DENSITY), so no part is denser than the others', () => {
    expect(DENSITY).toBeGreaterThan(0)
    // The vector line of |0⟩: length R, thickness 0.08 → R·0.08·DENSITY·N particles.
    const line = withWeight('bloch0', 1.6).length
    expect(line / (BLOCH_R * 0.08 * DENSITY * N)).toBeCloseTo(1, 1)
  })
})
