// For the checks only (web/scripts/frame-check.mjs, overlap-check.mjs): they
// must know that the frame they measure was drawn after they moved the page,
// not a stale one from a renderer that has stopped (a covered window, a
// starved machine). A check puts `window.__qrProbe = { frames: 0, running:
// false }` in the page before any script runs; the 3D engine then counts in
// it every frame it draws, says whether its loop is running and keeps a short
// log of what paused or resumed it, which the check prints when a measure is
// not valid. Without that object (every real visit) nothing is exposed and
// nothing is written.

export interface Probe {
  /** Frames the engine has drawn. */
  frames: number
  /** Whether the engine's loop is running (it stops off screen, at rest on phones, with the tab hidden). */
  running: boolean
  /** When the last frame was drawn (ms, the page's clock: performance.now()). */
  lastFrame?: number
  /** The last things that started, stopped or would stop the loop: [when (ms), what]. */
  log?: [number, string][]
}

/** How many entries of the log are kept. */
export const LOG_SIZE = 40

/** The page's probe, if a check installed one before the engine was created. */
export function findProbe(scope: object = window): Probe | null {
  const p = (scope as { __qrProbe?: unknown }).__qrProbe
  if (!p || typeof p !== 'object') return null
  const { frames, running } = p as Partial<Probe>
  return typeof frames === 'number' && typeof running === 'boolean' ? (p as Probe) : null
}

/** A frame was drawn. */
export function drew(p: Probe | null, now = performance.now()) {
  if (!p) return
  p.frames++
  p.lastFrame = now
}

/** The loop started or stopped: the state the probe shows is the loop's own, set in the same step. */
export function ran(p: Probe | null, running: boolean, now = performance.now()) {
  if (!p) return
  p.running = running
  note(p, running ? 'loop started' : 'loop stopped', now)
}

/** Something that pauses or resumes the loop happened (the card on or off screen, the tab hidden or shown). */
export function note(p: Probe | null, what: string, now = performance.now()) {
  if (!p) return
  const log = (p.log ??= [])
  log.push([Math.round(now), what])
  if (log.length > LOG_SIZE) log.splice(0, log.length - LOG_SIZE)
}
