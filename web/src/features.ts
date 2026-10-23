import { useEffect, useState } from 'react'
import { getTargets } from './api'

// Server-side switches the web follows, read from GET /api/targets (one
// request per page load, shared): one build, whatever the server's settings.

let demo: Promise<boolean> | null = null

/** Whether the "Request a free demo" form is on (DEMO_ENABLED). Off if unknown. */
export function demoEnabled(): Promise<boolean> {
  demo ??= getTargets()
    .then((r) => r.demo === true)
    .catch(() => false)
  return demo
}

/** demoEnabled() for components: null while it is not known yet. */
export function useDemoEnabled(): boolean | null {
  const [on, setOn] = useState<boolean | null>(null)
  useEffect(() => {
    let cancelled = false
    void demoEnabled().then((v) => !cancelled && setOn(v))
    return () => {
      cancelled = true
    }
  }, [])
  return on
}

export function resetFeaturesForTests() {
  demo = null
}
