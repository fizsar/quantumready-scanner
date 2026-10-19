import { cleanup, configure } from '@testing-library/react'
import { afterEach, vi } from 'vitest'

// findBy*/waitFor: 5 s instead of 1 s. Lazily loaded pages (/examples) can
// take longer than a second when the whole suite runs in parallel.
configure({ asyncUtilTimeout: 5000 })

afterEach(() => {
  cleanup()
  vi.useRealTimers()
  try {
    window.localStorage.clear()
  } catch {
    // some tests make storage throw on purpose
  }
  document.head.querySelectorAll('meta[name="robots"]').forEach((m) => m.remove())
  window.history.replaceState(null, '', '/')
})
