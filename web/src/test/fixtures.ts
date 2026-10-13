import { vi } from 'vitest'
import type { Report, ScanView } from '../api'

const blocks = (kex: number, tls: number, fs: number, variety: number, cert: number): Report['blocks'] => [
  { id: 'key_exchange', label: 'Intercambio de claves post-cuántico', points: kex, max: 50 },
  { id: 'tls13', label: 'TLS 1.3', points: tls, max: 15 },
  { id: 'forward_secrecy', label: 'Secreto hacia adelante (forward secrecy)', points: fs, max: 10 },
  { id: 'hybrid_variety', label: 'Variedad de grupos híbridos', points: variety, max: 5 },
  {
    id: 'certificate', label: 'Certificado y agilidad', points: cert, max: 20,
    items: [
      { id: 'valid', label: 'Válido (cadena, nombre, fechas)', points: 5, max: 5 },
      { id: 'key', label: 'Clave', points: 5, max: 5 },
      { id: 'signature', label: 'Firma', points: 5, max: 5 },
      { id: 'automation', label: 'Renovación automatizada', points: cert - 15, max: 5 },
    ],
  },
]

const notEvaluated = [
  { code: 'ssl2_ssl3', label: 'SSLv2 y SSLv3' },
  { code: 'tls12_dhe_suites', label: 'Suites DHE de TLS 1.2' },
]

/** fizsar.com today: PQC preferred, one hybrid group → 95 A. */
export const reportA: Report = {
  scoring_version: '2026.1', score: 95, grade: 'A', score_grade: 'A', floor_points: 0, cap: null, pqc: true,
  selected_group: 'X25519MLKEM768', tls_version: 'TLS 1.3',
  quantum_risk: { code: 'low', label: 'Bajo' }, current_risk: { code: 'none', label: 'Ninguno' },
  blocks: blocks(50, 15, 10, 0, 20),
  findings: [
    { code: 'CERT_CLASSICAL_SIGNATURE', axis: 'quantum', axis_label: 'Riesgo cuántico', severity: 'low', severity_label: 'Bajo', category: 'certificate', title: 'Firma clásica (ECDSA P-256): riesgo cuántico futuro', detail: 'La identidad del servidor…', remediation: 'Nada que hacer todavía.' },
    { code: 'KEX_SINGLE_HYBRID', axis: 'quantum', axis_label: 'Riesgo cuántico', severity: 'low', severity_label: 'Bajo', category: 'key_exchange', title: 'Un solo grupo híbrido', detail: 'Solo se acepta X25519MLKEM768.', remediation: 'Añade SecP256r1MLKEM768.' },
    { code: 'KEX_HYBRID_PREFERRED', axis: 'quantum', axis_label: 'Riesgo cuántico', severity: 'pass', severity_label: 'Correcto', category: 'key_exchange', title: 'Intercambio de claves post-cuántico: X25519MLKEM768', detail: 'Con la oferta de un navegador…' },
    { code: 'CERT_VALID', axis: 'current', axis_label: 'Riesgo actual', severity: 'pass', severity_label: 'Correcto', category: 'certificate', title: 'Certificado válido', detail: 'La cadena es de confianza…' },
  ],
  not_evaluated: notEvaluated,
}

/** A well-configured TLS 1.2-only server without PQC → floor → 40 D. */
export const reportD: Report = {
  scoring_version: '2026.1', score: 40, grade: 'D', score_grade: 'D', floor_points: 10, cap: null, pqc: false,
  selected_group: 'x25519', tls_version: 'TLS 1.2',
  quantum_risk: { code: 'critical', label: 'Crítico' }, current_risk: { code: 'none', label: 'Ninguno' },
  blocks: blocks(0, 0, 10, 0, 20),
  findings: [
    { code: 'KEX_NO_HYBRID', axis: 'quantum', axis_label: 'Riesgo cuántico', severity: 'critical', severity_label: 'Crítico', category: 'key_exchange', title: 'Sin intercambio de claves post-cuántico', detail: 'El servidor no acepta ningún grupo híbrido ML-KEM.', remediation: 'Activa X25519MLKEM768.' },
    { code: 'TLS13_MISSING', axis: 'quantum', axis_label: 'Riesgo cuántico', severity: 'high', severity_label: 'Alto', category: 'protocol', title: 'Sin TLS 1.3 no es posible adoptar intercambio de claves híbrido ML-KEM', detail: '…', remediation: 'Activa TLS 1.3.' },
    { code: 'FS_ALL', axis: 'current', axis_label: 'Riesgo actual', severity: 'pass', severity_label: 'Correcto', category: 'cipher_suites', title: 'Forward secrecy', detail: '…' },
  ],
  not_evaluated: notEvaluated,
}

export function scanView(status: ScanView['status'], extra: Partial<ScanView> = {}): ScanView {
  return {
    id: '11111111-2222-3333-4444-555555555555', host: 'fizsar.com', port: 443, status, retries: 0,
    created_at: '2026-10-13T17:11:13Z', lang: 'es', ...extra,
  }
}

export function json(body: unknown, status = 200, headers: Record<string, string> = {}): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json', ...headers } })
}

/** Stubs fetch with a handler; returns the mock to inspect calls. */
export function stubFetch(handler: (url: string, init?: RequestInit) => Response | Promise<Response>) {
  const fn = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => handler(String(input), init))
  vi.stubGlobal('fetch', fn)
  return fn
}

export function setNavigatorLanguages(langs: string[]) {
  vi.spyOn(window.navigator, 'languages', 'get').mockReturnValue(langs)
  vi.spyOn(window.navigator, 'language', 'get').mockReturnValue(langs[0])
}
