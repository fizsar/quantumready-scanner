import { describe, expect, it } from 'vitest'
import { buildFigure, BYTES_W, HYBRID_W, LAT_SCALE, lweCells, type FigureId } from './figures'
import { A, B_ERR, BAD, babai, E, GOOD, LWE, pointsIn, S, TARGET, WINDOW } from './lattice'
import { HYBRID, ML_KEM, sizes, X25519 } from './mlkem'

const N = 60_000
const built = new Map<FigureId, ReturnType<typeof buildFigure>>()
const get = (id: FigureId) => {
  if (!built.has(id)) built.set(id, buildFigure(id, N, 1))
  return built.get(id)!
}
type P = [number, number, number]
const withWeight = (id: FigureId, w: number) => {
  const { positions, looks } = get(id)
  const out: P[] = []
  for (let i = 0; i < N; i++) if (Math.abs(looks[2 * i + 1] - w) < 1e-6) out.push([positions[3 * i], positions[3 * i + 1], positions[3 * i + 2]])
  return out
}
const q = (v: number[], f: number) => [...v].sort((a, b) => a - b)[Math.floor(f * (v.length - 1))]

describe('scene 7 figures: what is drawn comes from lattice.ts and mlkem.ts', () => {
  const back = (p: P): [number, number] => [p[0] / LAT_SCALE + (WINDOW.x0 + WINDOW.x1) / 2, p[1] / LAT_SCALE + (WINDOW.y0 + WINDOW.y1) / 2]

  /** Each particle goes to the nearest expected point; all must be near one, and every point must get some. */
  const near = (ps: [number, number][], want: [number, number][], reach: number) => {
    const hits = new Map<string, number>()
    for (const p of ps) {
      let best = want[0]
      for (const w of want) if (Math.hypot(p[0] - w[0], p[1] - w[1]) < Math.hypot(p[0] - best[0], p[1] - best[1])) best = w
      expect(Math.hypot(p[0] - best[0], p[1] - best[1])).toBeLessThan(reach)
      hits.set(best.join(','), (hits.get(best.join(',')) ?? 0) + 1)
    }
    for (const w of want) expect(hits.get(w.join(',')) ?? 0, w.join(',')).toBeGreaterThan(2)
  }

  it('every lattice point of the window, and only those, is lit', () => {
    // Points are √5 apart at least: a particle within 0.8 belongs to exactly one.
    near(withWeight('lattice', 1.6).map(back), pointsIn(GOOD, WINDOW.x0, WINDOW.x1, WINDOW.y0, WINDOW.y1), 0.8)
  })

  it('the basis tips (where they end inside the window) are the good basis, then the bad one; with the bad one, a ring at Babai’s answer (14, 8)', () => {
    const tips = (id: FigureId) => {
      const { positions, looks } = get(id)
      const out: [number, number][] = []
      for (let i = 0; i < N; i++) if (Math.abs(looks[2 * i + 1] - 1.8) < 1e-6 && Math.abs(looks[2 * i] - 1) < 1e-6) out.push(back([positions[3 * i], positions[3 * i + 1], 0]))
      return out
    }
    const inWin = (p: readonly number[]) => p[0] >= WINDOW.x0 && p[0] <= WINDOW.x1 && p[1] >= WINDOW.y0 && p[1] <= WINDOW.y1
    near(tips('lattice'), GOOD.filter(inWin).map((p) => [p[0], p[1]] as [number, number]), 1.2)
    const badIn = BAD.filter(inWin).map((p) => [p[0], p[1]] as [number, number])
    if (badIn.length) near(tips('latticeBad'), badIn, 1.2)
    else expect(tips('latticeBad')).toHaveLength(0) // both leave the window: cut at its edge, no head
    const { positions, looks } = get('latticeBad')
    const ring: [number, number][] = []
    for (let i = 0; i < N; i++) if (Math.abs(looks[2 * i + 1] - 1.8) < 1e-6 && Math.abs(looks[2 * i]) < 1e-6) ring.push(back([positions[3 * i], positions[3 * i + 1], 0]))
    const c = [ring.reduce((t, p) => t + p[0], 0) / ring.length, ring.reduce((t, p) => t + p[1], 0) / ring.length]
    expect(c.map(Math.round)).toEqual(babai(BAD, TARGET))
  })

  it('LWE: one cell per number of A, s, e and b, lit by its value; every particle in a cell', () => {
    const cells = lweCells()
    expect(cells).toHaveLength(LWE.m * LWE.n + LWE.n + LWE.m + LWE.m)
    expect(cells.map((c) => c.value)).toEqual([...[0, 1, 2, 3].flatMap((j) => A.map((r) => r[j])), ...S, ...E, ...B_ERR])
    // The light of a cell is its value: v/(q − 1) for numbers mod q, (e + 1)/2 for the errors in {−1, 0, 1}.
    const isE = (k: number) => k >= LWE.m * LWE.n + LWE.n && k < LWE.m * LWE.n + LWE.n + LWE.m
    cells.forEach((c, k) => expect(c.hue, `cell ${k}`).toBeCloseTo(isE(k) ? (c.value + 1) / 2 : c.value / (LWE.q - 1), 9))
    const { positions, looks } = get('lwe')
    const count = new Array(cells.length).fill(0)
    for (let i = 0; i < N; i++) {
      if (Math.abs(looks[2 * i + 1] - 1.1) > 1e-6) continue
      const [x, y] = [positions[3 * i], positions[3 * i + 1]]
      const k = cells.findIndex((c) => x >= c.x - 1e-6 && x <= c.x + c.size + 1e-6 && y >= c.y - 1e-6 && y <= c.y + c.size + 1e-6)
      expect(k).toBeGreaterThanOrEqual(0)
      expect(looks[2 * i]).toBeCloseTo(cells[k].hue, 5)
      count[k]++
    }
    for (const n of count) expect(n).toBeGreaterThan(5)
  })

  it('ML-KEM-768: the bars are as long as the sizes (1184 : 1088 : 32 B)', () => {
    const rows = withWeight('mlkem', 1.2)
    const len = (y: number) => {
      const xs = rows.filter((p) => Math.abs(p[1] - (y + 0.12)) < 0.13).map((p) => p[0])
      return q(xs, 0.999) - q(xs, 0.001)
    }
    const s = sizes(ML_KEM['768'])
    expect(len(0.55)).toBeCloseTo(BYTES_W, 1)
    expect(len(0.05) / len(0.55)).toBeCloseTo(s.ct / s.ek, 1)
    expect(len(-0.45) / len(0.55)).toBeCloseTo(X25519.pub / s.ek, 1)
  })

  it('the hybrid: the client share is ML-KEM then X25519, 1184 : 32, at one scale with the server share and the secret', () => {
    const rows = withWeight('hybrid', 1.2)
    const parts = (y: number, hue: number) => {
      const { positions, looks } = get('hybrid')
      const xs: number[] = []
      for (let i = 0; i < N; i++)
        if (Math.abs(looks[2 * i + 1] - 1.2) < 1e-6 && Math.abs(looks[2 * i] - hue) < 1e-6 && Math.abs(positions[3 * i + 1] - (y + 0.14)) < 0.15) xs.push(positions[3 * i])
      return [q(xs, 0.001), q(xs, 0.999)]
    }
    expect(rows.length).toBeGreaterThan(100)
    const [k0, k1] = parts(0.7, 1)
    const [x0] = parts(0.7, 0.3)
    expect(x0).toBeGreaterThan(k1) // X25519 after ML-KEM
    const scale = HYBRID_W / (HYBRID.client[0].bytes + HYBRID.client[1].bytes)
    expect(k1 - k0).toBeCloseTo(HYBRID.client[0].bytes * scale, 1)
    const [s0, s1] = parts(0.05, 1)
    expect((s1 - s0) / (k1 - k0)).toBeCloseTo(HYBRID.server[0].bytes / HYBRID.client[0].bytes, 1)
  })
})
