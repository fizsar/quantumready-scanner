import { cleanup } from '@testing-library/react'
import { afterEach, vi } from 'vitest'

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
