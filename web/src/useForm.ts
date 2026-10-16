import { useId, useState } from 'react'
import { ApiError } from './api'

export const asError = (e: unknown) => (e instanceof ApiError ? e : new ApiError(0, 'network', ''))

/** Form state: busy flag, last error and the id of its alert. */
export function useForm() {
  const errorId = useId()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<ApiError | null>(null)
  const run = async (f: () => Promise<void>) => {
    setBusy(true)
    setError(null)
    try {
      await f()
    } catch (e) {
      setError(asError(e))
    } finally {
      setBusy(false)
    }
  }
  return { busy, error, setError, run, errorId }
}
