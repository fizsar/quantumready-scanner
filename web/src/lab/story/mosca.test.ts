import { describe, expect, it } from 'vitest'
import { GRI_2025, GRI_EXPERTS, pct, verdict } from './mosca'

describe('scene 6: Mosca’s inequality with the GRI 2025 survey', () => {
  it('the survey values are the ones the report states (26 experts; 28–49 % in 10 years, 51–70 % in 15)', () => {
    expect(GRI_EXPERTS).toBe(26)
    const at = (y: number) => GRI_2025.find((a) => a.years === y)!
    expect([at(10).low, at(10).high]).toEqual([0.28, 0.49])
    expect([at(15).low, at(15).high]).toEqual([0.51, 0.7])
    expect([at(5).low, at(5).high]).toEqual([null, 0.15])
    expect([at(20).low, at(20).high]).toEqual([0.69, null])
    // A cumulative likelihood never decreases with the horizon, at either end.
    const lows = GRI_2025.map((a) => a.low).filter((v): v is number => v !== null)
    const highs = GRI_2025.map((a) => a.high).filter((v): v is number => v !== null)
    expect([...lows].sort((a, b) => a - b)).toEqual(lows)
    expect([...highs].sort((a, b) => a - b)).toEqual(highs)
  })

  it('x + y below every horizon: nothing to say from the survey', () => {
    expect(verdict(2, 2)).toEqual({ kind: 'below', total: 4 })
  })

  it('x + y from 5 to 9 years: only the 5-year upper end is stated', () => {
    const v = verdict(3, 4)
    expect(v.kind).toBe('partial')
    if (v.kind === 'partial') expect(v.anchor.years).toBe(5)
    expect(verdict(5, 4).kind).toBe('partial')
  })

  it('x + y of 10 or more: at least the 10-year range (a cumulative likelihood: any shorter horizon is a lower bound)', () => {
    const v = verdict(6, 4)
    expect(v).toMatchObject({ kind: 'range', total: 10, anchor: { years: 10, low: 0.28, high: 0.49 } })
    expect(verdict(9, 5)).toMatchObject({ anchor: { years: 10 } })
    expect(verdict(10, 5)).toMatchObject({ anchor: { years: 15, low: 0.51, high: 0.7 } })
    expect(verdict(10, 5)).not.toHaveProperty('also')
  })

  it('x + y of 20 or more: the 15-year range, plus the 20-year lower end (69 %)', () => {
    const v = verdict(15, 5)
    expect(v).toMatchObject({ kind: 'range', anchor: { years: 15 }, also: { years: 20, low: 0.69 } })
    expect(verdict(30, 20)).toMatchObject({ total: 50, anchor: { years: 15 }, also: { years: 20 } })
    expect(pct(0.69)).toBe(69)
  })
})
