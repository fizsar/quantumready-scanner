import { cleanup, configure } from '@testing-library/react'
import { afterEach, beforeEach, vi } from 'vitest'
import { resetFeaturesForTests } from '../features'

// M1 metrics go through navigator.sendBeacon (jsdom has none): a stub keeps
// them away from the fetch mocks of other tests; metrics tests read it.
beforeEach(() => {
  resetFeaturesForTests() // GET /api/targets is read once per page load
  Object.defineProperty(navigator, 'sendBeacon', { value: vi.fn(() => true), configurable: true, writable: true })
})

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
