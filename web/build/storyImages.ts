// Vite plugin: one still image per step of /lab/historia, rendered at build
// time from the story model (model.ts, wave.ts, hydrogen.ts) with the step's
// camera at its start (the wave at the end of its time range), as the engine
// draws it: additive soft points with the same light weights, sizes and depth
// of field, hue from indigo to green. Emitted as
// dist/lab/story-<step>.png and served by the dev server at the same path.
import type { Plugin } from 'vite'
import { ORBITALS, sample } from '../src/lab/hydrogen.ts'
import { camPos, densityWeights, DOF, orbitalHues, STEPS, storyAlpha, storySize, WAVE_GAIN, WAVE_SIZE, waveSeeds, wavePoint, type Step } from '../src/lab/story/model.ts'
import { field } from '../src/lab/story/wave.ts'
import { BACKGROUND, NEGATIVE, POSITIVE, RADIUS, SEED, SIZE } from '../src/lab/view.ts'
import { encodePNG } from './orbitalImages.ts'

export const STORY_PX = 640
export const STORY_POINTS = 40_000

type V3 = [number, number, number]
const sub = (a: V3, b: V3): V3 => [a[0] - b[0], a[1] - b[1], a[2] - b[2]]
const dot = (a: V3, b: V3) => a[0] * b[0] + a[1] * b[1] + a[2] * b[2]
const cross = (a: V3, b: V3): V3 => [a[1] * b[2] - a[2] * b[1], a[2] * b[0] - a[0] * b[2], a[0] * b[1] - a[1] * b[0]]
const norm = (a: V3): V3 => {
  const l = Math.hypot(...a)
  return [a[0] / l, a[1] / l, a[2] / l]
}

/** The particles of a step at its end: world position, hue and opacity. */
export function stepParticles(step: Step, n = STORY_POINTS): { pos: Float32Array; hue: Float32Array; alpha: Float32Array } {
  const pos = new Float32Array(n * 3)
  const hue = new Float32Array(n)
  const alpha = new Float32Array(n)
  if (step.shape.kind === 'orbital') {
    const s = sample(ORBITALS[step.shape.index], n, SEED)
    const scale = RADIUS / s.r95
    const h = orbitalHues(s.signs)
    const w = densityWeights(ORBITALS[step.shape.index], s.positions)
    for (let i = 0; i < n; i++) {
      // physics (x, y, z) → three (x, z, −y), as the engine's orbital().
      pos[3 * i] = s.positions[3 * i] * scale
      pos[3 * i + 1] = s.positions[3 * i + 2] * scale
      pos[3 * i + 2] = -s.positions[3 * i + 1] * scale
      hue[i] = h[i]
      alpha[i] = w[i]
    }
  } else {
    const seeds = waveSeeds(n, SEED)
    const f = field(step.shape.t1)
    for (let i = 0; i < n; i++) {
      const w = wavePoint(seeds, i, f)
      pos.set(w.pos, 3 * i)
      hue[i] = w.hue
      alpha[i] = w.alpha
    }
  }
  return { pos, hue, alpha }
}

const HUES = 16
const LEVELS = 15
const CAP = 3

/** Background, then HUES × LEVELS colours: indigo → green, dark → (nearly) white, clamped as additive blending is. */
export function storyPalette(): Uint8Array {
  const p = new Uint8Array(3 * (1 + HUES * LEVELS))
  p.set(BACKGROUND, 0)
  for (let h = 0; h < HUES; h++)
    for (let k = 1; k <= LEVELS; k++) {
      const w = CAP * (k / LEVELS) ** 2
      const at = 3 * (1 + h * LEVELS + (k - 1))
      for (let c = 0; c < 3; c++) {
        const col = NEGATIVE[c] + ((POSITIVE[c] - NEGATIVE[c]) * h) / (HUES - 1)
        p[at + c] = Math.min(255, Math.round(BACKGROUND[c] + col * w))
      }
    }
  return p
}

/** Palette indices of the step's still image (square, the step's camera, no view offset). */
export function renderStep(step: Step, px = STORY_PX, n = STORY_POINTS): Uint8Array {
  const { pos, hue, alpha } = stepParticles(step, n)
  const cam = step.cam
  const eye = camPos(cam)
  const fwd = norm(sub(cam.target, eye))
  const right = norm(cross(fwd, [0, 1, 0]))
  const up = cross(right, fwd)
  const tan = Math.tan(((cam.fov / 2) * Math.PI) / 180)
  const weight = new Float32Array(px * px)
  const hueSum = new Float32Array(px * px)
  const a0 = storyAlpha(n)
  const wave = step.shape.kind === 'wave'
  const sizeK = storySize(n) * (wave ? WAVE_SIZE : 1)
  const gain = wave ? WAVE_GAIN : 1
  for (let i = 0; i < n; i++) {
    if (alpha[i] <= 0) continue
    const d = sub([pos[3 * i], pos[3 * i + 1], pos[3 * i + 2]], eye)
    const z = dot(d, fwd)
    if (z <= 0.1) continue
    const u = ((dot(d, right) / (z * tan) + 1) / 2) * px
    const v = ((1 - dot(d, up) / (z * tan)) / 2) * px
    const blur = 1 + (wave ? 0 : DOF) * Math.abs(z - cam.dist)
    const radius = (SIZE * px * 5 * sizeK * blur) / z / 2
    const a = Math.min(1, ((alpha[i] * gain) / (blur * blur)) * a0)
    const x0 = Math.max(0, Math.floor(u - radius))
    const x1 = Math.min(px - 1, Math.ceil(u + radius))
    const y0 = Math.max(0, Math.floor(v - radius))
    const y1 = Math.min(px - 1, Math.ceil(v + radius))
    for (let y = y0; y <= y1; y++)
      for (let x = x0; x <= x1; x++) {
        const q = ((x + 0.5 - u) ** 2 + (y + 0.5 - v) ** 2) / (radius * radius)
        if (q > 1) continue
        const w = Math.exp(-q * 3.5) * a
        weight[y * px + x] += w
        hueSum[y * px + x] += w * hue[i]
      }
  }
  const out = new Uint8Array(px * px)
  for (let p = 0; p < px * px; p++) {
    const w = Math.min(CAP, weight[p])
    const k = Math.round(Math.sqrt(w / CAP) * LEVELS)
    if (k === 0) continue
    const h = Math.round((hueSum[p] / weight[p]) * (HUES - 1))
    out[p] = 1 + h * LEVELS + (k - 1)
  }
  return out
}

export function storyImages(): Plugin {
  const cache = new Map<string, Buffer>()
  const png = (s: Step) => {
    let b = cache.get(s.id)
    if (!b) cache.set(s.id, (b = encodePNG(renderStep(s), storyPalette(), STORY_PX, STORY_PX)))
    return b
  }
  return {
    name: 'story-images',
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        const s = STEPS.find((x) => req.url === `/lab/story-${x.id}.png`)
        if (!s) return next()
        res.setHeader('Content-Type', 'image/png')
        res.end(png(s))
      })
    },
    generateBundle() {
      for (const s of STEPS) this.emitFile({ type: 'asset', fileName: `lab/story-${s.id}.png`, source: png(s) })
    },
  }
}
