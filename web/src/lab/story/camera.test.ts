import { describe, expect, it } from 'vitest'
import { stepParticles } from '../../../build/storyImages'
import { boxCorners, direction, fit, MARGIN, project, sphereHull, type Frame } from './camera'
import { shapeBox, STEPS } from './model'
import { field } from './wave'

// Frames as the page computes them: the free area right of the text panel on
// wide screens, the whole stage above the sheet on phones, the square of a
// still image.
const FRAMES: Record<string, Frame> = {
  'desktop 1280×800': { width: 1280, height: 800, left: 560, top: 0, right: 1256, bottom: 800 },
  'desktop 1634×916': { width: 1634, height: 916, left: 820, top: 0, right: 1610, bottom: 916 },
  'desktop 1920×1080': { width: 1920, height: 1080, left: 960, top: 0, right: 1896, bottom: 1080 },
  'iPhone 393×852 (stage above the sheet)': { width: 393, height: 500, left: 0, top: 0, right: 393, bottom: 500 },
  'iPhone SE 375×667': { width: 375, height: 315, left: 0, top: 0, right: 375, bottom: 315 },
  'still 640×640': { width: 640, height: 640, left: 0, top: 0, right: 640, bottom: 640 },
}

describe('framing: every step whole, centred in the free area', () => {
  for (const [name, frame] of Object.entries(FRAMES))
    it(name, () => {
      for (const step of STEPS) {
        const f = step.shape.kind === 'wave' ? field(step.shape.t1) : undefined
        const box = shapeBox(step.shape, f)
        const shot = fit(box, step.cam.az, step.cam.el, step.cam.frame, step.cam.fov, frame)
        // The shape's centre lands at the centre of the free area.
        const c = project(shot.target, shot, frame)!
        expect(c[0]).toBeCloseTo((frame.left + frame.right) / 2, 6)
        expect(c[1]).toBeCloseTo((frame.top + frame.bottom) / 2, 6)
        // Every point of its hull (or its box's corners) inside the free area, with the margin.
        const mx = ((frame.right - frame.left) / 2) * MARGIN - 0.5
        const my = ((frame.bottom - frame.top) / 2) * MARGIN - 0.5
        for (const p of box.hull ?? boxCorners(box)) {
          const [u, v] = project(p, shot, frame)!
          expect(u, `${step.id} hull`).toBeGreaterThanOrEqual(frame.left + mx)
          expect(u, `${step.id} hull`).toBeLessThanOrEqual(frame.right - mx)
          expect(v, `${step.id} hull`).toBeGreaterThanOrEqual(frame.top + my)
          expect(v, `${step.id} hull`).toBeLessThanOrEqual(frame.bottom - my)
        }
        // And the particles themselves: at least 99.5 % inside the free area.
        const { pos } = stepParticles(step, 6000)
        let inside = 0
        for (let i = 0; i < 6000; i++) {
          const q = project([pos[3 * i], pos[3 * i + 1], pos[3 * i + 2]], shot, frame)
          if (q && q[0] >= frame.left && q[0] <= frame.right && q[1] >= frame.top && q[1] <= frame.bottom) inside++
        }
        expect(inside / 6000, `${step.id} particles inside`).toBeGreaterThanOrEqual(0.995)
      }
    })

  it('the sphere hull reaches at least 96 % of the radius in every direction', () => {
    const hull = sphereHull(2)
    let worst = Infinity
    for (let i = 0; i <= 60; i++)
      for (let j = 0; j < 120; j++) {
        const d = direction((j * Math.PI) / 60, -Math.PI / 2 + (i * Math.PI) / 60)
        worst = Math.min(worst, Math.max(...hull.map((p) => p[0] * d[0] + p[1] * d[1] + p[2] * d[2])))
      }
    expect(worst / 2).toBeGreaterThanOrEqual(Math.cos(Math.PI / 16) ** 2 - 1e-9)
  })

  it('closer than the tight fit is never allowed; wider shots only move back', () => {
    const frame = FRAMES['desktop 1440×900'] ?? FRAMES['desktop 1634×916']
    const box = shapeBox(STEPS[0].shape)
    const tight = fit(box, 0.3, 0.2, 1, 38, frame)
    expect(fit(box, 0.3, 0.2, 0.5, 38, frame).distance).toBeCloseTo(tight.distance, 9)
    expect(fit(box, 0.3, 0.2, 2, 38, frame).distance).toBeCloseTo(2 * tight.distance, 9)
    const d = direction(0.3, 0.2)
    expect(Math.hypot(...d)).toBeCloseTo(1, 12)
  })
})
