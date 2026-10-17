// The /examples reports: the API's own answers (internal/httpapi
// TestLabFixtures checks they match the current scoring model and pins their
// grades). A real scan of fizsar.com and three servers of the isolated TLS lab
// (test/tlslab), whose .test names never resolve on the Internet (RFC 6761).
import type { Lang, ScanView } from './api'
import type { ExampleKey } from './exampleAnchors'
import exportEn from './test/lab/export.en.json'
import exportEs from './test/lab/export.es.json'
import fizsarEn from './test/lab/fizsar.en.json'
import fizsarEs from './test/lab/fizsar.es.json'
import kyberEn from './test/lab/kyber.en.json'
import kyberEs from './test/lab/kyber.es.json'
import modernEn from './test/lab/modern.en.json'
import modernEs from './test/lab/modern.es.json'

export const EXAMPLES = [
  { key: 'fizsar', lab: false, scans: { es: fizsarEs, en: fizsarEn } },
  { key: 'modern', lab: true, scans: { es: modernEs, en: modernEn } },
  { key: 'kyber', lab: true, scans: { es: kyberEs, en: kyberEn } },
  { key: 'export', lab: true, scans: { es: exportEs, en: exportEn } },
] as unknown as { key: ExampleKey; lab: boolean; scans: Record<Lang, ScanView> }[]
