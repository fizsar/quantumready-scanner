// Framing: where the camera goes so that a shape is seen whole, centred in
// the free part of the screen (right of the text panel on wide screens, the
// stage above the sheet on phones), with a margin. Pure: the engine, the
// build-time still images and the tests use the same function.

export type V3 = [number, number, number]

/**
 * What the camera must keep in frame, in world units: an axis-aligned box
 * (its centre is where the camera looks) and, optionally, a tighter hull of
 * points around the shape (a sphere, a cylinder): if given, the hull is what
 * must fit, not the box's corners.
 */
export interface Box {
  min: V3
  max: V3
  hull?: V3[]
}

/** The drawing area (width, height) and its free rectangle, in the same pixels. */
export interface Frame {
  width: number
  height: number
  left: number
  top: number
  right: number
  bottom: number
}

export const union = (a: Box, b: Box): Box => ({
  min: [0, 1, 2].map((k) => Math.min(a.min[k], b.min[k])) as V3,
  max: [0, 1, 2].map((k) => Math.max(a.max[k], b.max[k])) as V3,
})

const sub = (a: V3, b: V3): V3 => [a[0] - b[0], a[1] - b[1], a[2] - b[2]]
const dot = (a: V3, b: V3) => a[0] * b[0] + a[1] * b[1] + a[2] * b[2]
const cross = (a: V3, b: V3): V3 => [a[1] * b[2] - a[2] * b[1], a[2] * b[0] - a[0] * b[2], a[0] * b[1] - a[1] * b[0]]
const norm = (a: V3): V3 => {
  const l = Math.hypot(...a)
  return [a[0] / l, a[1] / l, a[2] / l]
}

/** Unit vector from the target towards the camera (y up). */
export const direction = (az: number, el: number): V3 => [Math.cos(el) * Math.sin(az), Math.sin(el), Math.cos(el) * Math.cos(az)]

/** Free space kept around the shape, as a share of the free rectangle's half-size. */
export const MARGIN = 0.1

export interface Shot {
  position: V3
  target: V3
  /** Distance from the camera to the target (the focus of the depth of field). */
  distance: number
  /** Where the target lands, relative to the drawing area's centre, in pixels (the view offset). */
  shift: [number, number]
  /** Pixels per world unit at distance 1 (focal length in pixels). */
  focal: number
}

/**
 * The camera on the sphere (az, el) around the box's centre, at the smallest
 * distance at which every corner of the box projects inside the free
 * rectangle less the margin, times `factor` (≥ 1: farther out). The target
 * projects at the centre of the free rectangle.
 */
export function fit(box: Box, az: number, el: number, factor: number, fovDeg: number, frame: Frame, margin = MARGIN): Shot {
  const target: V3 = [0, 1, 2].map((k) => (box.min[k] + box.max[k]) / 2) as V3
  const dir = direction(az, el)
  const fwd: V3 = [-dir[0], -dir[1], -dir[2]]
  const right = norm(cross(fwd, Math.abs(dir[1]) > 0.999 ? [0, 0, -1] : [0, 1, 0]))
  const up = cross(right, fwd)
  const focal = frame.height / 2 / Math.tan(((fovDeg / 2) * Math.PI) / 180)
  const halfW = ((frame.right - frame.left) / 2) * (1 - margin)
  const halfH = ((frame.bottom - frame.top) / 2) * (1 - margin)
  const corners: V3[] = (box.hull ?? boxCorners(box)).map((p) => sub(p, target))
  const fits = (d: number) =>
    corners.every((v) => {
      const depth = d - dot(v, dir)
      if (depth <= 0.05) return false
      return Math.abs((dot(v, right) * focal) / depth) <= halfW && Math.abs((dot(v, up) * focal) / depth) <= halfH
    })
  const radius = Math.max(...corners.map((v) => Math.hypot(...v)), 1e-3)
  let lo = radius * 0.2
  let hi = radius * 200
  for (let k = 0; k < 40; k++) {
    const mid = (lo + hi) / 2
    if (fits(mid)) hi = mid
    else lo = mid
  }
  const distance = hi * Math.max(1, factor)
  const position: V3 = [target[0] + dir[0] * distance, target[1] + dir[1] * distance, target[2] + dir[2] * distance]
  const shift: [number, number] = [(frame.left + frame.right) / 2 - frame.width / 2, (frame.top + frame.bottom) / 2 - frame.height / 2]
  return { position, target, distance, shift, focal }
}

/** The eight corners of a box. */
export function boxCorners(box: Box): V3[] {
  const out: V3[] = []
  for (const x of [box.min[0], box.max[0]]) for (const y of [box.min[1], box.max[1]]) for (const z of [box.min[2], box.max[2]]) out.push([x, y, z])
  return out
}

/** Points on a sphere of radius r, every 22.5°: in any direction their hull reaches at least cos²(11.25°) ≈ 96 % of r. */
export function sphereHull(r: number): V3[] {
  const out: V3[] = []
  for (let i = 0; i <= 8; i++) {
    const el = -Math.PI / 2 + (i * Math.PI) / 8
    for (let j = 0; j < 16; j++) {
      const az = (j * Math.PI) / 8
      out.push([r * Math.cos(el) * Math.cos(az), r * Math.sin(el), r * Math.cos(el) * Math.sin(az)])
    }
  }
  return out
}

/** Projects a world point through a shot into the drawing area's pixels (x right, y down), or null behind the camera. */
export function project(p: V3, shot: Shot, frame: Frame): [number, number] | null {
  const dir = norm(sub(shot.position, shot.target))
  const fwd: V3 = [-dir[0], -dir[1], -dir[2]]
  const right = norm(cross(fwd, Math.abs(dir[1]) > 0.999 ? [0, 0, -1] : [0, 1, 0]))
  const up = cross(right, fwd)
  const v = sub(p, shot.position)
  const depth = dot(v, fwd)
  if (depth <= 0) return null
  return [frame.width / 2 + shot.shift[0] + (dot(v, right) * shot.focal) / depth, frame.height / 2 + shot.shift[1] - (dot(v, up) * shot.focal) / depth]
}
