// Vite plugin: the still image of each orbital for /lab/orbital, rendered at
// build time from the same sampler, seed and view as the 3D scene (the first
// frame, without rotation). Emitted as dist/lab/orbital-<id>.png and served
// by the dev server at the same path. Indexed PNG written by hand (zlib from
// Node): no image library.
import { deflateSync } from 'node:zlib'
import type { Plugin } from 'vite'
import { ORBITALS, sample, type Orbital } from '../src/lab/hydrogen.ts'
import { alphaFor, BACKGROUND, CAMERA, NEGATIVE, POSITIVE, RADIUS, SEED, SIZE } from '../src/lab/view.ts'

export const STILL_PX = 640
export const STILL_POINTS = 40_000

/**
 * The orbital's cloud as orbitalScene.ts draws it (additive points): the
 * summed point weight per pixel for each phase of ψ.
 */
export function renderStill(o: Orbital, px = STILL_PX, points = STILL_POINTS): { pos: Float32Array; neg: Float32Array } {
  const s = sample(o, points, SEED)
  const scale = RADIUS / s.r95
  const pos = new Float32Array(px * px)
  const neg = new Float32Array(px * px)
  const alpha = alphaFor(points)
  const f = 1 / Math.tan(((CAMERA.fov / 2) * Math.PI) / 180)
  const [cx, sx] = [Math.cos(CAMERA.tilt), Math.sin(CAMERA.tilt)]
  for (let k = 0; k < points; k++) {
    // physics (x, y, z) → three (x, z, −y), then the scene's tilt about x.
    const x = s.positions[3 * k] * scale
    const y = s.positions[3 * k + 2] * scale
    const z = -s.positions[3 * k + 1] * scale
    const ry = y * cx - z * sx
    const rz = y * sx + z * cx
    const depth = CAMERA.z - rz
    if (depth <= 0.1) continue
    const u = ((x * f) / depth + 1) * 0.5 * px
    const v = (1 - ((ry * f) / depth + 1) * 0.5) * px
    const radius = (SIZE * px) / depth / 2
    const acc = s.signs[k] > 0 ? pos : neg
    const x0 = Math.max(0, Math.floor(u - radius))
    const x1 = Math.min(px - 1, Math.ceil(u + radius))
    const y0 = Math.max(0, Math.floor(v - radius))
    const y1 = Math.min(px - 1, Math.ceil(v + radius))
    for (let j = y0; j <= y1; j++)
      for (let i = x0; i <= x1; i++) {
        const d = ((i + 0.5 - u) ** 2 + (j + 0.5 - v) ** 2) / (radius * radius)
        if (d <= 1) acc[j * px + i] += (1 - d) * alpha
      }
  }
  return { pos, neg }
}

/** Palette: the background, then LEVELS shades of each phase (additive, clamped, as on screen). */
const LEVELS = 127
/** Weight at which a pixel turns (nearly) white; levels are spaced quadratically below it. */
const CAP = 3
const level = (k: number) => CAP * (k / LEVELS) ** 2

export function palette(): Uint8Array {
  const p = new Uint8Array(3 * (1 + 2 * LEVELS))
  p.set(BACKGROUND, 0)
  for (let k = 1; k <= LEVELS; k++)
    for (let c = 0; c < 3; c++) {
      p[3 * k + c] = Math.min(255, Math.round(BACKGROUND[c] + POSITIVE[c] * level(k)))
      p[3 * (LEVELS + k) + c] = Math.min(255, Math.round(BACKGROUND[c] + NEGATIVE[c] * level(k)))
    }
  return p
}

/**
 * Palette index per pixel: the dominant phase with the pixel's total weight
 * (where both phases overlap, at the nodal surfaces, the minor one is dropped).
 */
export function quantize({ pos, neg }: { pos: Float32Array; neg: Float32Array }): Uint8Array {
  const out = new Uint8Array(pos.length)
  for (let p = 0; p < pos.length; p++) {
    const w = Math.min(CAP, pos[p] + neg[p])
    const k = Math.round(Math.sqrt(w / CAP) * LEVELS)
    out[p] = k === 0 ? 0 : pos[p] >= neg[p] ? k : LEVELS + k
  }
  return out
}

const crcTable = Array.from({ length: 256 }, (_, n) => {
  let c = n
  for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1
  return c >>> 0
})
function crc32(buf: Uint8Array): number {
  let c = 0xffffffff
  for (const b of buf) c = crcTable[(c ^ b) & 0xff] ^ (c >>> 8)
  return (c ^ 0xffffffff) >>> 0
}
function chunk(type: string, data: Uint8Array): Buffer {
  const out = Buffer.alloc(12 + data.length)
  out.writeUInt32BE(data.length, 0)
  out.write(type, 4, 'ascii')
  out.set(data, 8)
  out.writeUInt32BE(crc32(out.subarray(4, 8 + data.length)), 8 + data.length)
  return out
}

/** 8-bit indexed PNG, Sub filter on every row (compresses the dark background well). */
export function encodePNG(indices: Uint8Array, plte: Uint8Array, width: number, height: number): Buffer {
  const raw = Buffer.alloc((width + 1) * height)
  for (let y = 0; y < height; y++) {
    raw[y * (width + 1)] = 1
    for (let x = 0; x < width; x++) {
      const cur = indices[y * width + x]
      const left = x > 0 ? indices[y * width + x - 1] : 0
      raw[y * (width + 1) + 1 + x] = (cur - left) & 0xff
    }
  }
  const ihdr = Buffer.alloc(13)
  ihdr.writeUInt32BE(width, 0)
  ihdr.writeUInt32BE(height, 4)
  ihdr.set([8, 3, 0, 0, 0], 8) // 8 bits, indexed colour, deflate, adaptive filtering, no interlace
  return Buffer.concat([
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    chunk('IHDR', ihdr),
    chunk('PLTE', plte),
    chunk('IDAT', deflateSync(raw, { level: 9 })),
    chunk('IEND', new Uint8Array()),
  ])
}

export function orbitalImages(): Plugin {
  const cache = new Map<string, Buffer>()
  const png = (o: Orbital) => {
    let b = cache.get(o.id)
    if (!b) cache.set(o.id, (b = encodePNG(quantize(renderStill(o)), palette(), STILL_PX, STILL_PX)))
    return b
  }
  return {
    name: 'orbital-images',
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        const o = ORBITALS.find((x) => req.url === `/lab/orbital-${x.id}.png`)
        if (!o) return next()
        res.setHeader('Content-Type', 'image/png')
        res.end(png(o))
      })
    },
    generateBundle() {
      for (const o of ORBITALS) this.emitFile({ type: 'asset', fileName: `lab/orbital-${o.id}.png`, source: png(o) })
    },
  }
}
