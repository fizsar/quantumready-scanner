import { act, renderHook } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { pollDelay } from './api'
import { json, reportA, scanView, stubFetch } from './test/fixtures'
import { useScan } from './useScan'

describe('pollDelay', () => {
  it('grows by 1.5x from 1 s up to 8 s', () => {
    expect([0, 1, 2, 3, 10].map(pollDelay)).toEqual([1000, 1500, 2250, 3375, 8000])
  })
})

describe('useScan polling', () => {
  it('polls with a growing delay and stops at done', async () => {
    vi.useFakeTimers()
    const answers = [scanView('queued'), scanView('running'), scanView('running'), scanView('done', { report: reportA })]
    const fetch = stubFetch(() => json(answers.shift() ?? scanView('done', { report: reportA })))

    const { result } = renderHook(() => useScan('abc', 'es'))
    await act(() => vi.advanceTimersByTimeAsync(0))
    expect(fetch).toHaveBeenCalledTimes(1)
    expect(result.current.scan?.status).toBe('queued')

    await act(() => vi.advanceTimersByTimeAsync(999))
    expect(fetch).toHaveBeenCalledTimes(1) // not before 1 s
    await act(() => vi.advanceTimersByTimeAsync(1))
    expect(fetch).toHaveBeenCalledTimes(2)
    expect(result.current.scan?.status).toBe('running')

    await act(() => vi.advanceTimersByTimeAsync(1499))
    expect(fetch).toHaveBeenCalledTimes(2) // second wait is 1.5 s
    await act(() => vi.advanceTimersByTimeAsync(1))
    expect(fetch).toHaveBeenCalledTimes(3)

    await act(() => vi.advanceTimersByTimeAsync(2250))
    expect(fetch).toHaveBeenCalledTimes(4)
    expect(result.current.scan?.status).toBe('done')

    await act(() => vi.advanceTimersByTimeAsync(60_000))
    expect(fetch).toHaveBeenCalledTimes(4) // stopped
    expect(String(fetch.mock.calls[0][0])).toBe('/api/scans/abc?lang=es')
  })

  it('stops at failed', async () => {
    vi.useFakeTimers()
    const fetch = stubFetch(() => json(scanView('failed', { error: { code: 'connection_refused', message: 'x' } })))
    const { result } = renderHook(() => useScan('abc', 'en'))
    await act(() => vi.advanceTimersByTimeAsync(30_000))
    expect(fetch).toHaveBeenCalledTimes(1)
    expect(result.current.scan?.error?.code).toBe('connection_refused')
  })

  it('stops on 404 and reports the error', async () => {
    vi.useFakeTimers()
    const fetch = stubFetch(() => json({ error: { code: 'not_found', message: 'No encontrado.' } }, 404))
    const { result } = renderHook(() => useScan('nope', 'es'))
    await act(() => vi.advanceTimersByTimeAsync(30_000))
    expect(fetch).toHaveBeenCalledTimes(1)
    expect(result.current.error?.code).toBe('not_found')
  })

  it('keeps polling through a network error', async () => {
    vi.useFakeTimers()
    let n = 0
    stubFetch(() => {
      n++
      if (n === 2) throw new TypeError('Failed to fetch')
      return json(n < 3 ? scanView('running') : scanView('done', { report: reportA }))
    })
    const { result } = renderHook(() => useScan('abc', 'es'))
    await act(() => vi.advanceTimersByTimeAsync(1000))
    expect(result.current.reconnecting).toBe(true)
    await act(() => vi.advanceTimersByTimeAsync(1500))
    expect(result.current.scan?.status).toBe('done')
    expect(result.current.reconnecting).toBe(false)
  })
})
