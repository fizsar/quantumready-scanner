import { describe, expect, it } from 'vitest'
import { chooseMode, pointBudget } from './capability'

describe('chooseMode', () => {
  const env = (o: { reduce?: boolean; coarse?: boolean; gl?: boolean; cores?: number; mem?: number; saveData?: boolean }) => ({
    matchMedia: (q: string) => ({ matches: (q.includes('reduced-motion') && !!o.reduce) || (q.includes('coarse') && !!o.coarse) }),
    navigator: { hardwareConcurrency: o.cores ?? 8, deviceMemory: o.mem, connection: { saveData: o.saveData } } as unknown as Navigator,
    webgl2: () => o.gl ?? true,
  })
  it('falls back to still images on reduced motion, data saver, very low-end devices and without WebGL2', () => {
    expect(chooseMode(env({ reduce: true }))).toEqual({ kind: 'still', reason: 'reduced-motion' })
    expect(chooseMode(env({ saveData: true }))).toEqual({ kind: 'still', reason: 'save-data' })
    expect(chooseMode(env({ mem: 1 }))).toEqual({ kind: 'still', reason: 'low-end' })
    expect(chooseMode(env({ cores: 1 }))).toEqual({ kind: 'still', reason: 'low-end' })
    expect(chooseMode(env({ gl: false }))).toEqual({ kind: 'still', reason: 'no-webgl2' })
  })
  it('sizes the cloud to the device', () => {
    expect(chooseMode(env({ cores: 8, mem: 8 }))).toEqual({ kind: '3d', points: 120_000 })
    expect(pointBudget(8, 8, true)).toBe(50_000) // a fast phone
    expect(pointBudget(4, 4, true)).toBe(25_000)
    expect(pointBudget(4, undefined, false)).toBe(40_000)
  })
})
