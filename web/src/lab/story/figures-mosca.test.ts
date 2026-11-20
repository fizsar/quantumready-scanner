import { describe, expect, it } from 'vitest'
import { buildFigure, MOSCA_DEFAULT, MOSCA_H, WIRE, YEARS, type FigureId } from './figures'
import { GRI_2025 } from './mosca'

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
const yearX = (t: number) => -3 + (6 * t) / YEARS

describe('scene 6 figures: what is drawn comes from mosca.ts', () => {
  it('the captured traffic flows on the wire as packets; the store holds 12 packets', () => {
    const flow = withWeight('harvest', -1.9)
    expect(flow.length).toBeGreaterThan(100)
    // Packets have a size (a gaussian dot): on the wire within a few of its sigmas.
    for (const p of flow) expect(Math.abs(p[1] - WIRE.y)).toBeLessThan(0.3)
    expect(withWeight('harvest', 1.4).length).toBeGreaterThan(0) // the packets inside
  })

  it('z as the survey states it: error bars at 10 and 15 years from the lower- to the upper-end averages; single dots at 5 and 20', () => {
    const bars = withWeight('mosca', 1.5)
    for (const a of GRI_2025.filter((a) => a.low !== null && a.high !== null)) {
      const ys = bars.filter((p) => Math.abs(p[0] - yearX(a.years)) < 0.08).map((p) => p[1])
      expect(ys.length, `${a.years} years`).toBeGreaterThan(20)
      expect(q(ys, 0.01) / MOSCA_H, `${a.years} low`).toBeCloseTo(a.low!, 1)
      expect(q(ys, 0.99) / MOSCA_H, `${a.years} high`).toBeCloseTo(a.high!, 1)
    }
    const dots = withWeight('mosca', 1.6)
    for (const a of GRI_2025.filter((a) => a.low === null || a.high === null)) {
      const near = dots.filter((p) => Math.abs(p[0] - yearX(a.years)) < 0.3)
      expect(near.length, `${a.years} years`).toBeGreaterThan(5)
      const ys = near.map((p) => p[1])
      expect(q(ys, 0.5) / MOSCA_H).toBeCloseTo((a.low ?? a.high)!, 1)
    }
    // No error bar where the report gives only one end.
    for (const t of [5, 20]) expect(bars.filter((p) => Math.abs(p[0] - yearX(t)) < 0.08)).toHaveLength(0)
  })

  it('x then y under the axis, at the sliders’ default values', () => {
    const xs = withWeight('mosca', 1.4).map((p) => p[0])
    expect(q(xs, 0.002)).toBeCloseTo(yearX(0), 1)
    expect(q(xs, 0.998)).toBeCloseTo(yearX(MOSCA_DEFAULT.x + MOSCA_DEFAULT.y), 1)
  })
})
