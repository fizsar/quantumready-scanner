// For the checks only (web/scripts/frame-check.mjs, overlap-check.mjs): they
// must know that the frame they measure was drawn after they moved the page,
// not a stale one from a renderer that has stopped (a covered window, a
// starved machine). A check puts `window.__qrProbe = { frames: 0, running:
// false }` in the page before any script runs; the 3D engine then counts in
// it every frame it draws and says whether its loop is running. Without that
// object (every real visit) nothing is exposed and nothing is written.

export interface Probe {
  /** Frames the engine has drawn. */
  frames: number
  /** Whether the engine's loop is running (it stops off screen, at rest on phones, with the tab hidden). */
  running: boolean
}

/** The page's probe, if a check installed one before the engine was created. */
export function findProbe(scope: object = window): Probe | null {
  const p = (scope as { __qrProbe?: unknown }).__qrProbe
  if (!p || typeof p !== 'object') return null
  const { frames, running } = p as Partial<Probe>
  return typeof frames === 'number' && typeof running === 'boolean' ? (p as Probe) : null
}
