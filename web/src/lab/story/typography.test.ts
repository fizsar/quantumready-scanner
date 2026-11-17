/// <reference types="node" />
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

// Spanish writes a space before % («51–70 %»). With a normal space a line can
// start with "%" (it did on phones with the narrower sans): the story's visible
// texts use a no-break space (U+00A0, written \u00a0) there.
describe('no line starts with %', () => {
  for (const f of ['mosca.ts', 'captions.tsx', 'Story.tsx', 'texts.ts'])
    it(`${f}: a number before % is joined by a no-break space`, () => {
      const code = readFileSync(resolve('src/lab/story', f), 'utf8')
        .split('\n')
        .filter((l) => !/^\s*(\/\/|\*|\/\*)/.test(l))
        .join('\n')
      expect(code.match(/[\d}] %/g) ?? []).toEqual([])
    })
})

// A range («51–70 %») never breaks after its dash (it did on phones, «51–» at
// the end of a line): a word joiner (⁠) follows the dash in the texts that
// carry the survey's ranges, the slider's result and the source note.
describe('a numeric range stays on one line', () => {
  const unjoined = (s: string) => s.match(/[\d}]–(?:\d|\$\{)/g) ?? []

  it('mosca.ts: every range in its sentences (code, not comments)', () => {
    const code = readFileSync(resolve('src/lab/story/mosca.ts'), 'utf8')
      .split('\n')
      .filter((l) => !/^\s*(\/\/|\*|\/\*)/.test(l))
      .join('\n')
    expect(unjoined(code)).toEqual([])
  })

  it('Story.tsx: the GRI note (ES and EN)', () => {
    const note = readFileSync(resolve('src/lab/story/Story.tsx'), 'utf8')
      .split('\n')
      .filter((l) => /^\s*[?:] '26 (expertos|experts)/.test(l))
    expect(note).toHaveLength(2)
    expect(unjoined(note.join('\n'))).toEqual([])
  })
})
