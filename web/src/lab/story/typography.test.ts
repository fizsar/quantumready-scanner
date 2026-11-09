/// <reference types="node" />
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

// Spanish writes a space before % («51–70 %»). With a normal space a line can
// start with "%" (it did on phones with the narrower sans): the story's visible
// texts use a no-break space ( ) there.
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
