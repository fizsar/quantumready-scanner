/// <reference types="node" />
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import { bezier, EASE } from './model'

const tokens = readFileSync(resolve('src/tokens.css'), 'utf8')

describe('motion: the engine eases with the named curves of tokens.css', () => {
  const names = { out: 'ease-out', inOut: 'ease-in-out', standard: 'ease-standard', in: 'ease-in' } as const

  it('EASE has the same control points as the CSS tokens', () => {
    for (const [key, token] of Object.entries(names)) {
      const m = tokens.match(new RegExp(`--${token}:\\s*cubic-bezier\\(([^)]*)\\)`))
      expect(m, token).not.toBeNull()
      expect(m![1].split(',').map(Number), token).toEqual([...EASE[key as keyof typeof EASE]])
    }
  })

  it('the tokens are the ones of the art direction (4.4), durations included', () => {
    expect(tokens).toContain('--ease-out: cubic-bezier(0.16, 1, 0.3, 1);')
    expect(tokens).toContain('--ease-in-out: cubic-bezier(0.76, 0, 0.24, 1);')
    expect(tokens).toContain('--ease-standard: cubic-bezier(0.2, 0, 0, 1);')
    expect(tokens).toContain('--ease-in: cubic-bezier(0.4, 0, 1, 1);')
    const dur = Object.fromEntries([...tokens.matchAll(/--(dur-\w+|stagger):\s*(\d+)ms/g)].slice(0, 6).map((m) => [m[1], Number(m[2])]))
    expect(dur).toEqual({ 'dur-instant': 100, 'dur-fast': 200, 'dur-base': 400, 'dur-slow': 800, 'dur-scene': 1200, stagger: 80 })
  })

  it('with reduced motion every duration is 0', () => {
    const block = tokens.match(/@media \(prefers-reduced-motion: reduce\) \{\s*:root \{([^}]*)\}/)
    expect(block).not.toBeNull()
    const zeroed = [...block![1].matchAll(/--([\w-]+):\s*0ms;/g)].map((m) => m[1]).sort()
    expect(zeroed).toEqual(['dur-base', 'dur-fast', 'dur-instant', 'dur-scene', 'dur-slow', 'stagger'])
  })

  it('bezier() is CSS cubic-bezier: exact at the ends, the identity for a straight line, monotonic', () => {
    for (const c of Object.values(EASE)) {
      expect(bezier(c, 0)).toBe(0)
      expect(bezier(c, 1)).toBe(1)
      expect(bezier(c, -1)).toBe(0)
      expect(bezier(c, 2)).toBe(1)
      for (let x = 0.01; x <= 1; x += 0.01) expect(bezier(c, x)).toBeGreaterThanOrEqual(bezier(c, x - 0.01) - 1e-9)
    }
    for (const x of [0.1, 0.37, 0.5, 0.9]) expect(bezier([0.25, 0.25, 0.75, 0.75], x)).toBeCloseTo(x, 6)
    // A point computed by hand: cubic-bezier(0.4, 0, 1, 1) at t = 0.5 is x = 0.65, y = 0.5.
    expect(bezier(EASE.in, 0.65)).toBeCloseTo(0.5, 6)
    // The symmetric curve passes through the centre.
    expect(bezier(EASE.inOut, 0.5)).toBeCloseTo(0.5, 6)
  })

  it('each curve has its character: out is fast first, in is slow first', () => {
    expect(bezier(EASE.out, 0.25)).toBeGreaterThan(0.7)
    expect(bezier(EASE.in, 0.25)).toBeLessThan(0.15)
    expect(bezier(EASE.inOut, 0.2)).toBeLessThan(0.1)
    expect(bezier(EASE.inOut, 0.8)).toBeGreaterThan(0.9)
  })
})
