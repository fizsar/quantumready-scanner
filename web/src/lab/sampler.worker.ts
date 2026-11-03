// Samples the orbitals off the main thread and hands the buffers over
// (transferred, not copied), first orbital first so the scene starts early.
import { ORBITALS, sample } from './hydrogen'

export interface SampleRequest {
  count: number
  seed: number
}

export interface SampleReply {
  index: number
  positions: Float32Array
  signs: Float32Array
  r95: number
}

const scope = self as unknown as { onmessage: (e: MessageEvent<SampleRequest>) => void; postMessage: (m: SampleReply, t: Transferable[]) => void }

scope.onmessage = (e) => {
  ORBITALS.forEach((o, index) => {
    const s = sample(o, e.data.count, e.data.seed)
    scope.postMessage({ index, ...s }, [s.positions.buffer, s.signs.buffer])
  })
}
