/// <reference types="node" />
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import { findProbe } from './probe'

describe('the checks’ probe (frames drawn by the 3D engine)', () => {
  it('is nothing on a real visit: no object, nothing exposed', () => {
    expect(findProbe({})).toBeNull()
    expect(findProbe(window)).toBeNull()
    expect('__qrProbe' in window).toBe(false)
  })

  it('is the very object a check installed (the engine writes into it)', () => {
    const scope = { __qrProbe: { frames: 0, running: false } }
    const p = findProbe(scope)!
    p.frames++
    p.running = true
    expect(scope.__qrProbe).toEqual({ frames: 1, running: true })
  })

  it('ignores anything that is not a probe', () => {
    for (const bad of [1, 'x', null, {}, { frames: '0', running: false }, { frames: 0 }]) expect(findProbe({ __qrProbe: bad })).toBeNull()
  })

  it('the engine counts each frame it draws and reports its loop, only through the probe', () => {
    const engine = readFileSync(resolve('src/lab/story/engine.ts'), 'utf8')
    expect(engine).toContain('const probe = findProbe()')
    // Every use is guarded: without a probe the engine writes nothing.
    const uses = [...engine.matchAll(/probe\.(frames|running)/g)]
    expect(uses.length).toBeGreaterThanOrEqual(4)
    for (const m of engine.matchAll(/^.*probe\.(?:frames|running).*$/gm)) expect(m[0]).toMatch(/if \(probe\)/)
    expect(engine).not.toContain('__qrProbe') // only probe.ts knows the name
  })

  it('both checks install it before the page loads and refuse to judge a frame that was not drawn after the move', () => {
    for (const f of ['scripts/frame-check.mjs', 'scripts/overlap-check.mjs']) {
      const src = readFileSync(resolve(f), 'utf8')
      expect(src, f).toContain('window.__qrProbe = { frames: 0, running: false }')
      expect(src, f).toContain('p.frames >= f0 + 2')
      expect(src, f).toContain('invalid measurement (render stopped)')
      expect(src, f).toContain('process.exit(3)')
    }
  })
})
