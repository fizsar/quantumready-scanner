// The view shared by the 3D scene (orbitalScene.ts) and the build-time still
// images (build/orbitalImages.ts), so both show the same cloud from the same
// place. No three.js and no DOM.

/** Seed of the samples: the still image is the first points of the 3D cloud. */
export const SEED = 1
/** World radius of each cloud's r95 sphere: every orbital fills the same space. */
export const RADIUS = 1.5
export const CAMERA = { fov: 38, z: 5, tilt: 0.35 }
/** Point diameter in pixels = SIZE · canvas height (device px) / distance. */
export const SIZE = 0.022
/** Per-point opacity: fewer points, each brighter, so the total brightness stays similar. */
export const alphaFor = (points: number) => Math.min(0.9, Math.max(0.12, 0.3 * Math.sqrt(40_000 / points)))
/** ψ > 0 and ψ < 0: --green and --indigo (index.css). */
export const POSITIVE: [number, number, number] = [0x3d, 0xff, 0xa2]
export const NEGATIVE: [number, number, number] = [125, 122, 255]
export const BACKGROUND: [number, number, number] = [5, 6, 11]
