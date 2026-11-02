import { describe, expect, it } from 'vitest'
import { dict } from './i18n'
import { showcaseDict } from './i18n.showcase'

// No promises of a beta or of what "will come" (decision of 31/10/2026: no
// accounts in production, and the scanner opens to any domain). Only the
// account pages may mention the beta: they are shown with ACCOUNTS_ENABLED=true,
// where invitation codes and the closed beta are real.
const accountOnly = new Set(['auth.codeHelp', 'org.createClosed'])
const promise = /\bbeta\b|llegar[áa] con|will come with|cuando (se )?abra|once (it )?opens/i

describe('no outdated promises in the texts', () => {
  for (const lang of ['es', 'en'] as const) {
    it(`core and showcase strings (${lang})`, () => {
      const all: [string, string][] = [...Object.entries(dict[lang]), ...Object.entries(showcaseDict[lang])]
      const bad = all.filter(([k, v]) => !accountOnly.has(k) && promise.test(v)).map(([k]) => k)
      expect(bad).toEqual([])
      // The exceptions still exist (a rename would leave them unchecked).
      for (const k of accountOnly) expect(dict[lang][k as keyof (typeof dict)[typeof lang]], k).toMatch(/beta/i)
    })
  }
})
