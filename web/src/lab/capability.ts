// Whether this device gets the 3D scene, and how many points. No three.js
// here: the page decides before importing anything heavy, and devices that
// get still images never download the 3D code.

export type Mode = { kind: '3d'; points: number } | { kind: 'still'; reason: 'reduced-motion' | 'no-webgl2' | 'save-data' | 'low-end' }

interface Env {
  matchMedia?: (q: string) => { matches: boolean }
  navigator?: Navigator & { deviceMemory?: number; connection?: { saveData?: boolean } }
  /** Probes for a WebGL2 context that is not software-rendered. */
  webgl2?: () => boolean
}

/** WebGL2 without a major performance caveat (a software renderer such as SwiftShader fails it). */
export function hasFastWebGL2(): boolean {
  try {
    const canvas = document.createElement('canvas')
    const gl = canvas.getContext('webgl2', { failIfMajorPerformanceCaveat: true })
    if (!gl) return false
    gl.getExtension('WEBGL_lose_context')?.loseContext()
    return true
  } catch {
    return false
  }
}

export function chooseMode(env: Env = { matchMedia: window.matchMedia?.bind(window), navigator, webgl2: hasFastWebGL2 }): Mode {
  const mq = (q: string) => env.matchMedia?.(q).matches ?? false
  if (mq('(prefers-reduced-motion: reduce)')) return { kind: 'still', reason: 'reduced-motion' }
  const nav = env.navigator
  if (nav?.connection?.saveData) return { kind: 'still', reason: 'save-data' }
  const mem = nav?.deviceMemory // Chromium only; rounded down, ≤ 8
  const cores = nav?.hardwareConcurrency ?? 4
  if ((mem !== undefined && mem < 2) || cores < 2) return { kind: 'still', reason: 'low-end' }
  if (!env.webgl2?.()) return { kind: 'still', reason: 'no-webgl2' }
  return { kind: '3d', points: pointBudget(cores, mem, mq('(pointer: coarse)')) }
}

/**
 * Points per orbital. The scene halves it again at run time if the first
 * frames are slow (OrbitalScene: adaptive draw range).
 */
export function pointBudget(cores: number, memory: number | undefined, coarse: boolean): number {
  const mem = memory ?? 4
  if (!coarse && cores >= 8 && mem >= 8) return 120_000
  if (cores >= 6 && mem >= 4) return coarse ? 50_000 : 80_000
  return coarse ? 25_000 : 40_000
}

/** Device pixel ratio for the canvas: at most 1.5 (points do not need more). */
export const pixelRatio = (dpr: number) => Math.min(dpr || 1, 2)
