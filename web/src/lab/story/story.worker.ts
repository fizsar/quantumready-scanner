// Builds the story's static shapes off the main thread and hands the buffers
// over (transferred, not copied): the first orbital first, so the scene starts
// early, then the wave's particle seeds and the other orbitals.
import { ORBITALS, sample } from '../hydrogen'
import { densityWeights, orbitalHues, waveSeeds } from './model'

export interface StoryRequest {
  count: number
  seed: number
}

export type StoryReply =
  /** looks: per particle (hue, light weight). */
  | { kind: 'orbital'; index: number; positions: Float32Array; looks: Float32Array; r95: number }
  | { kind: 'seeds'; seeds: Float32Array }

const scope = self as unknown as { onmessage: (e: MessageEvent<StoryRequest>) => void; postMessage: (m: StoryReply, t: Transferable[]) => void }

scope.onmessage = (e) => {
  const { count, seed } = e.data
  const orbital = (index: number) => {
    const o = ORBITALS[index]
    const s = sample(o, count, seed)
    const hues = orbitalHues(s.signs)
    const weights = densityWeights(o, s.positions)
    const looks = new Float32Array(count * 2)
    for (let i = 0; i < count; i++) {
      looks[2 * i] = hues[i]
      looks[2 * i + 1] = weights[i]
    }
    scope.postMessage({ kind: 'orbital', index, positions: s.positions, looks, r95: s.r95 }, [s.positions.buffer, looks.buffer])
  }
  orbital(0)
  const seeds = waveSeeds(count, seed)
  scope.postMessage({ kind: 'seeds', seeds }, [seeds.buffer])
  for (let i = 1; i < ORBITALS.length; i++) orbital(i)
}
