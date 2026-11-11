// Shared by frame-check.mjs and overlap-check.mjs: a measure only counts if the
// frame it looks at was drawn after the page got to its position.
//
// A renderer can stop (a covered or minimized window, a starved machine); what
// is on screen is then a stale frame, which reads as a cut shape, or as fine.
// The checks put a probe in the page before any script runs (PROBE; see
// src/lab/probe.ts): the 3D engine counts in it the frames it draws, says
// whether its loop is running and logs what pauses or resumes it. Before each
// measure (PAINTED), two animation frames must go by and, if the engine's loop
// is running, it must have drawn two more frames. If that does not happen
// within PAINT_MS, the result is INVALID ("invalid measurement (render
// stopped)"), never "ok" nor a failure; the check prints the engine's state at
// that moment (DUMP, report) and ends with exit code 3 (a failed check is 1).
//   PAUSE_RENDER=1  self-test: the check stops the rendering before its third measure and must end as invalid.

export const PROBE = 'window.__qrProbe = { frames: 0, running: false }'
export const PAINT_MS = Number(process.env.PAINT_MS ?? 5000)
export const INVALID = 'invalid measurement (render stopped)'
export const INVALID_EXIT = 3

/** In the page: resolves once a fresh frame is on screen. */
export const PAINTED = `new Promise((ok) => {
  const p = window.__qrProbe ?? { frames: 0, running: false }
  const f0 = p.frames
  let n = 0
  const tick = () => { n++; if (n >= 2 && (!p.running || p.frames >= f0 + 2)) ok(true); else requestAnimationFrame(tick) }
  requestAnimationFrame(tick)
})`

/** In the page: the state of the engine and of what pauses it, as JSON. A frozen page never answers: race it. */
export const DUMP = `(async () => {
  const p = window.__qrProbe ?? {}
  const now = performance.now()
  const rafAlive = await new Promise((r) => { requestAnimationFrame(() => r(true)); setTimeout(() => r(false), 800) })
  const scene = document.querySelector('.hero-scene, .story-stage')
  const b = scene?.getBoundingClientRect()
  const canvas = document.querySelector('canvas')
  return JSON.stringify({
    now: Math.round(now),
    running: p.running ?? null,
    frames: p.frames ?? null,
    lastFrameAgo: p.lastFrame === undefined ? null : Math.round(now - p.lastFrame),
    log: p.log ?? [],
    visibility: document.visibilityState,
    focus: document.hasFocus(),
    rafAlive,
    scene: b ? { top: Math.round(b.top), bottom: Math.round(b.bottom), viewport: innerHeight, onScreen: b.bottom > 0 && b.top < innerHeight && getComputedStyle(scene).visibility !== 'hidden' } : null,
    canvas: canvas ? { ready: canvas.dataset.ready ?? null, phase: canvas.dataset.phase ?? null, entry: canvas.dataset.entry ?? null } : null,
  })
})()`

/**
 * The lines to print for an invalid measure, from DUMP's answer (null: the page did not answer).
 * They say whether the engine claims to be running without drawing (a bug of its loop) or
 * the whole page stopped making frames (the environment).
 */
export function report(where, json) {
  const out = [`${where ? `${where}: ` : ''}${INVALID}: no new frame in ${PAINT_MS} ms`]
  if (!json) return [...out, '  the page did not answer (frozen or hung): no state to show']
  const d = JSON.parse(json)
  const since = d.now - PAINT_MS
  out.push(
    `  engine: ${d.running === null ? 'no probe' : d.running ? 'says it is RUNNING' : 'stopped'}, ${d.frames} frames drawn, last one ${d.lastFrameAgo === null ? 'never' : `${d.lastFrameAgo} ms ago`}`,
    `  page: visibility ${d.visibility}, focus ${d.focus}, animation frames ${d.rafAlive ? 'alive' : 'STOPPED'}`,
    `  scene: ${d.scene ? `${d.scene.onScreen ? 'on screen' : 'OFF screen'} (top ${d.scene.top}, bottom ${d.scene.bottom}, viewport ${d.scene.viewport})` : 'none'}; canvas: ${d.canvas ? `ready ${d.canvas.ready}, phase ${d.canvas.phase}, entry ${d.canvas.entry} ms` : 'none'}`,
  )
  const during = d.log.filter(([t]) => t >= since)
  const before = d.log.filter(([t]) => t >= since - 5000 && t < since)
  const show = (l) => (l.length ? l.map(([t, what]) => `${t - d.now} ms: ${what}`).join('; ') : 'nothing')
  out.push(`  paused or resumed while waiting: ${show(during)}`, `  in the 5 s before: ${show(before)}`)
  out.push(
    d.running && d.rafAlive
      ? '  => the engine says it is running and the page makes frames, but it draws none: a bug in its loop'
      : d.running
        ? '  => the engine was running and the page stopped making frames: the renderer stopped (window covered, machine starved)'
        : '  => the engine is stopped and the page makes no frames: the renderer stopped',
  )
  return out
}
