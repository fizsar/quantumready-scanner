/// <reference types="node" />
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
// The checks' side of the probe: plain JavaScript run by Node and by Playwright's image.
// @ts-expect-error no types for the script
import * as checks from '../../scripts/painted.mjs'
import { drew, findProbe, LOG_SIZE, note, ran, type Probe } from './probe'

const fresh = (): Probe => ({ frames: 0, running: false })

describe('the checks’ probe (frames drawn by the 3D engine)', () => {
  it('is nothing on a real visit: no object, nothing exposed, nothing written', () => {
    expect(findProbe({})).toBeNull()
    expect(findProbe(window)).toBeNull()
    expect('__qrProbe' in window).toBe(false)
    // Every helper is a no-op without a probe.
    drew(null)
    ran(null, true)
    note(null, 'x')
    expect('__qrProbe' in window).toBe(false)
  })

  it('is the very object a check installed (the engine writes into it), as the checks install it', () => {
    const scope: { __qrProbe?: Probe } = {}
    new Function('window', checks.PROBE)(scope)
    expect(scope.__qrProbe).toEqual({ frames: 0, running: false })
    const p = findProbe(scope)!
    drew(p, 1234)
    expect(scope.__qrProbe).toMatchObject({ frames: 1, lastFrame: 1234 })
  })

  it('ignores anything that is not a probe', () => {
    for (const bad of [1, 'x', null, {}, { frames: '0', running: false }, { frames: 0 }]) expect(findProbe({ __qrProbe: bad })).toBeNull()
  })

  it('counts frames with their time, and the running state changes together with its log entry', () => {
    const p = fresh()
    ran(p, true, 100)
    expect(p).toMatchObject({ running: true, log: [[100, 'loop started']] })
    drew(p, 116)
    drew(p, 133)
    expect(p).toMatchObject({ frames: 2, lastFrame: 133 })
    ran(p, false, 150)
    expect(p.running).toBe(false)
    expect(p.log![1]).toEqual([150, 'loop stopped'])
  })

  it('keeps only the last entries of the log', () => {
    const p = fresh()
    for (let i = 0; i < LOG_SIZE + 25; i++) note(p, `n${i}`, i)
    expect(p.log).toHaveLength(LOG_SIZE)
    expect(p.log![LOG_SIZE - 1]).toEqual([LOG_SIZE + 24, `n${LOG_SIZE + 24}`])
  })

  it('the engine and the hero touch the probe only through these helpers (so its state is the loop’s own)', () => {
    for (const f of ['src/lab/story/engine.ts', 'src/lab/hero/hero.ts']) {
      const src = readFileSync(resolve(f), 'utf8')
      expect(src, f).toContain('const probe = findProbe()')
      expect(src, f).not.toMatch(/probe\.(frames|running|lastFrame|log)/)
      expect(src, f).not.toContain('__qrProbe') // only probe.ts knows the name
    }
    const engine = readFileSync(resolve('src/lab/story/engine.ts'), 'utf8')
    // Started and stopped in the same step as the loop's own flag; a frame counted where it is drawn.
    expect(engine).toMatch(/running = true\n\s+ran\(probe, true\)/)
    expect(engine).toMatch(/if \(running\) ran\(probe, false\)\n\s+running = false/)
    expect(engine.match(/drew\(probe\)/g)).toHaveLength(2)
    expect(engine).toContain('note(probe, `visibilitychange: ${document.visibilityState}`)')
  })
})

describe('the checks refuse a frame that was not drawn after the move', () => {
  it('both checks use the same rule, install the probe before the page loads and end with their own exit code', () => {
    expect(checks.INVALID).toBe('invalid measurement (render stopped)')
    expect(checks.INVALID_EXIT).toBe(3)
    expect(checks.PAINTED).toContain('p.frames >= f0 + 2')
    expect(checks.PAINTED).toContain('n >= 2')
    for (const f of ['scripts/frame-check.mjs', 'scripts/overlap-check.mjs']) {
      const src = readFileSync(resolve(f), 'utf8')
      expect(src, f).toMatch(/import \{[^}]*PAINTED[^}]*\} from '\.\/painted\.mjs'/)
      expect(src, f).toMatch(/addScriptToEvaluateOnNewDocument', \{ source: PROBE \}|addInitScript\(PROBE\)/)
      expect(src, f).toContain('process.exit(INVALID_EXIT)')
      expect(src, f).toContain('report(')
    }
  })

  /** What the page answers (DUMP), with a log relative to now = 20 000 ms. */
  const dump = (over: object) =>
    JSON.stringify({
      now: 20_000,
      running: true,
      frames: 480,
      lastFrameAgo: 5210,
      log: [
        [2_000, 'asked to run'],
        [2_001, 'loop started'],
        [12_500, 'card on screen: false, enough of it for the entry: false'],
        [12_501, 'loop stopped'],
        [16_000, 'visibilitychange: hidden'],
      ],
      visibility: 'visible',
      focus: true,
      rafAlive: true,
      scene: { top: 150, bottom: 580, viewport: 900, onScreen: true },
      canvas: { ready: 'true', phase: 'rest', entry: '4460' },
      ...over,
    })

  it('the report says what the engine claims, its last frame, the page, the scene and what paused it, and when', () => {
    const lines: string[] = checks.report('hero 1440×900, rest+2s', dump({}))
    expect(lines[0]).toBe(`hero 1440×900, rest+2s: invalid measurement (render stopped): no new frame in ${checks.PAINT_MS} ms`)
    const text = lines.join('\n')
    expect(text).toContain('engine: says it is RUNNING, 480 frames drawn, last one 5210 ms ago')
    expect(text).toContain('page: visibility visible, focus true, animation frames alive')
    expect(text).toContain('scene: on screen (top 150, bottom 580, viewport 900); canvas: ready true, phase rest, entry 4460 ms')
    // While waiting (the last PAINT_MS) and in the 5 s before that, each with its own events.
    expect(text).toContain('paused or resumed while waiting: -4000 ms: visibilitychange: hidden')
    expect(text).toContain('in the 5 s before: -7500 ms: card on screen: false, enough of it for the entry: false; -7499 ms: loop stopped')
    expect(text).not.toContain('asked to run') // older than that
  })

  it('names the cause: a loop that says it runs and draws nothing is a bug; a page without frames is the renderer', () => {
    const verdict = (over: object) => (checks.report('x', dump(over)) as string[]).at(-1)
    expect(verdict({ running: true, rafAlive: true })).toContain('a bug in its loop')
    expect(verdict({ running: true, rafAlive: false })).toContain('the renderer stopped')
    expect(verdict({ running: false, rafAlive: false })).toContain('the renderer stopped')
    expect(verdict({ running: true, rafAlive: false })).not.toContain('bug')
  })

  it('a page that does not answer is reported as such, still invalid', () => {
    const lines: string[] = checks.report('x', null)
    expect(lines[0]).toContain('invalid measurement (render stopped)')
    expect(lines[1]).toContain('did not answer')
  })
})
