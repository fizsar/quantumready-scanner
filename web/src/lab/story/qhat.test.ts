import { describe, expect, it } from 'vitest'
import { buildFigure, figureBox, FIGURES, QHAT } from './figures'

const N = 60_000
const { positions, looks } = buildFigure('qhat', N, 1)
const strokes: [number, number, number, number][] = []
let dust = 0
for (let i = 0; i < N; i++) {
  const w = looks[2 * i + 1]
  if (w > 1) strokes.push([positions[3 * i], positions[3 * i + 1], positions[3 * i + 2], looks[2 * i]])
  else if (w > 0) dust++
}
/** Distance from (x, y) to the segment a–b. */
const toSeg = (x: number, y: number, a: number[], b: number[]) => {
  const t = Math.min(1, Math.max(0, ((x - a[0]) * (b[0] - a[0]) + (y - a[1]) * (b[1] - a[1])) / ((b[0] - a[0]) ** 2 + (b[1] - a[1]) ** 2)))
  return Math.hypot(x - a[0] - t * (b[0] - a[0]), y - a[1] - t * (b[1] - a[1]))
}

describe('the Q̂ of the brand as a figure of particles', () => {
  it('is the last figure, so adding it changed no other figure’s particles (their seed is their index)', () => {
    expect(FIGURES[FIGURES.length - 1]).toBe('qhat')
  })

  it('every bright particle lies on a stroke of the glyph: the bowl, its tail or the hat', () => {
    const { bowl, tail, hat, stroke } = QHAT
    let off = 0
    for (const [x, y, z] of strokes) {
      // On the oval: its implicit equation is 1 there, and the distance is about (value − 1) × the radius.
      const k = Math.hypot((x - bowl.c[0]) / bowl.rx, (y - bowl.c[1]) / bowl.ry)
      const d = Math.min(Math.abs(k - 1) * Math.min(bowl.rx, bowl.ry), toSeg(x, y, tail[0], tail[1]), toSeg(x, y, hat[0], hat[1]), toSeg(x, y, hat[1], hat[2]))
      if (d > stroke || Math.abs(z) > stroke) off++
    }
    expect(strokes.length).toBeGreaterThan(2000)
    // Each stroke is a tube of dots around its line, narrower than the stroke: none strays.
    expect(off).toBe(0)
  })

  it('all three parts are there, in proportion to their length', () => {
    const { tail, hat } = QHAT
    const near = (a: number[], b: number[]) => strokes.filter(([x, y]) => toSeg(x, y, a, b) < QHAT.stroke / 2).length
    const hatN = near(hat[0], hat[1]) + near(hat[1], hat[2])
    const tailN = strokes.filter(([x, y]) => toSeg(x, y, tail[0], tail[1]) < QHAT.stroke / 2 && y < -1.25).length // its end, clear of the bowl
    expect(hatN).toBeGreaterThan(strokes.length * 0.08)
    expect(tailN).toBeGreaterThan(strokes.length * 0.02)
    // The hat sits clear above the bowl: an empty band between them.
    const top = QHAT.bowl.c[1] + QHAT.bowl.ry
    expect(strokes.filter(([, y]) => y > top + 0.12 && y < hat[0][1] - 0.12).length).toBeLessThan(strokes.length * 0.002)
  })

  it('goes from indigo on the left to green on the right', () => {
    const left = strokes.filter(([x]) => x < -0.8).map((s) => s[3])
    const right = strokes.filter(([x]) => x > 0.8).map((s) => s[3])
    const mean = (v: number[]) => v.reduce((a, b) => a + b, 0) / v.length
    expect(mean(left)).toBeLessThan(0.25)
    expect(mean(right)).toBeGreaterThan(0.85)
    for (const s of strokes) expect(s[3] >= QHAT.hue[0] - 1e-6 && s[3] <= QHAT.hue[1] + 1e-6).toBe(true)
  })

  it('reads as distinct dots, not a fill: several particles stacked on each site, plus a few faint loose ones', () => {
    const sites = new Set(strokes.map(([x, y, z]) => `${x.toFixed(5)},${y.toFixed(5)},${z.toFixed(5)}`))
    expect(sites.size).toBeGreaterThan(500)
    expect(sites.size).toBeLessThan(strokes.length / 2)
    expect(dust).toBeGreaterThan(0)
    expect(dust).toBeLessThan(strokes.length / 5)
  })

  it('its box is the glyph’s, centred between the hat and the tail', () => {
    const { min, max } = figureBox('qhat')
    expect(min[0]).toBeLessThan(-QHAT.bowl.rx)
    expect(max[0]).toBeGreaterThan(QHAT.tail[1][0])
    expect(max[1]).toBeGreaterThan(QHAT.hat[1][1])
    expect(min[1]).toBeLessThan(QHAT.tail[1][1])
    expect(max[1] - QHAT.hat[1][1]).toBeLessThan(0.3)
  })
})
