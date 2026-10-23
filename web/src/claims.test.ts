import { describe, expect, it } from 'vitest'
import { dict } from './i18n'
import { showcaseDict } from './i18n.showcase'
import { forbiddenClaims } from './test/forbidden'

// Every string the web can show, in both languages: never "official", never an
// ENS certification or compliance, never "NIS2 requires post-quantum".
const all = (d: Record<string, Record<string, string>>) => Object.values(d).flatMap((byKey) => Object.entries(byKey))

describe('claims the web never makes', () => {
  for (const [key, text] of [...all(dict), ...all(showcaseDict)]) {
    it(key, () => {
      for (const re of forbiddenClaims) expect(text, String(re)).not.toMatch(re)
    })
  }

  it('the patterns catch what they must (and not ordinary words)', () => {
    const caught = (s: string) => forbiddenClaims.some((re) => re.test(s))
    for (const s of [
      'Herramienta oficial del CCN',
      'Official report',
      'Informe certificado ENS',
      'certificado conforme al ENS',
      'ENS-certified scan',
      'Tu web cumple el ENS',
      'cumple con el ENS',
      'Your site complies with the ENS',
      'ENS compliant',
      'NIS2 exige criptografía post-cuántica',
      'NIS 2 requires post-quantum key exchange',
      'La criptografía post-cuántica es obligatoria según NIS2',
      'PQC is mandatory under NIS2',
    ])
      expect(caught(s), s).toBe(true)
    for (const s of [
      'Mapeo orientativo frente a NIS2 y DORA',
      'Tokens de sesión',
      'Lens',
      'El ENS (Esquema Nacional de Seguridad) es una norma española.',
      'NIS2 pide gestionar los riesgos; la criptografía post-cuántica es una buena práctica.',
    ])
      expect(caught(s), s).toBe(false)
  })
})
