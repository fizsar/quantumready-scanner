// Builds the story's static shapes off the main thread and hands the buffers
// over (transferred, not copied): the first orbital first, so the scene starts
// early, then the wave's particle seeds and the other orbitals.
import { ORBITALS, sample } from '../hydrogen'
import { orbitalHues, waveSeeds } from './model'

export interface StoryRequest {
  count: number
  seed: number
}

export type StoryReply =
  | { kind: 'orbital'; index: number; positions: Float32Array; hues: Float32Array; r95: number }
  | { kind: 'seeds'; seeds: Float32Array }

const scope = self as unknown as { onmessage: (e: MessageEvent<StoryRequest>) => void; postMessage: (m: StoryReply, t: Transferable[]) => void }

scope.onmessage = (e) => {
  const { count, seed } = e.data
  const orbital = (index: number) => {
    const s = sample(ORBITALS[index], count, seed)
    const hues = orbitalHues(s.signs)
    scope.postMessage({ kind: 'orbital', index, positions: s.positions, hues, r95: s.r95 }, [s.positions.buffer, hues.buffer])
  }
  orbital(0)
  const seeds = waveSeeds(count, seed)
  scope.postMessage({ kind: 'seeds', seeds }, [seeds.buffer])
  for (let i = 1; i < ORBITALS.length; i++) orbital(i)
}
