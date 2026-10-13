import { useEffect, useState } from 'react'
import { ApiError, getScan, isFinished, pollDelay, type Lang, type ScanView } from './api'

/** Give up polling after this many consecutive network errors. */
const MAX_NETWORK_ERRORS = 10

export interface ScanState {
  scan?: ScanView
  error?: ApiError
  /** The last poll failed but polling continues. */
  reconnecting: boolean
}

/**
 * Loads a scan and polls it with a growing delay (see pollDelay) until it is
 * done or failed. A 4xx answer (e.g. 404) stops polling; network errors are
 * retried a few times. Changing lang reloads the texts.
 */
export function useScan(id: string, lang: Lang): ScanState {
  const [state, setState] = useState<ScanState>({ reconnecting: false })

  useEffect(() => {
    let cancelled = false
    let timer: ReturnType<typeof setTimeout> | undefined
    let attempt = 0
    let networkErrors = 0

    const tick = async () => {
      try {
        const scan = await getScan(id, lang)
        if (cancelled) return
        networkErrors = 0
        setState({ scan, reconnecting: false })
        if (isFinished(scan)) return
      } catch (e) {
        if (cancelled) return
        const err = e instanceof ApiError ? e : new ApiError(0, 'network', String(e))
        if ((err.status >= 400 && err.status < 500) || ++networkErrors > MAX_NETWORK_ERRORS) {
          setState((s) => ({ ...s, error: err, reconnecting: false }))
          return
        }
        setState((s) => ({ ...s, reconnecting: true }))
      }
      timer = setTimeout(tick, pollDelay(attempt++))
    }

    void tick()
    return () => {
      cancelled = true
      clearTimeout(timer)
    }
  }, [id, lang])

  return state
}
