import { ApiError, type Report } from './api'
import type { MessageKey } from './i18n'

export type Tone = 'good' | 'warn' | 'bad'

/**
 * The plain-language sentence at the top of a result, for someone without a
 * technical background: what is wrong, without alarm, and what to do. Order
 * matters: a broken certificate (browsers warn) outweighs everything, then
 * the configuration problems found by the raw probes (model 2026.2), then
 * post-quantum status, then today's weaknesses.
 */
export function headline(r: Report): { key: MessageKey; tone: Tone } {
  const has = (...codes: string[]) => r.findings.some((f) => codes.includes(f.code))
  const weakToday = r.current_risk.code === 'high' || r.current_risk.code === 'critical'
  const certBroken = r.cap?.grade === 'F' && r.cap.findings.some((f) => f.code.startsWith('CERT_'))
  if (certBroken) return { key: 'headline.broken', tone: 'bad' }
  if (has('PROTO_SSL2', 'PROTO_SSL3')) return { key: 'headline.ssl', tone: 'bad' }
  if (has('SUITE_NULL', 'SUITE_INTEGRITY_ONLY')) return { key: 'headline.noEncryption', tone: 'bad' }
  if (has('SUITE_EXPORT', 'SUITE_ANON', 'SUITE_DES')) return { key: 'headline.brokenCiphers', tone: 'bad' }
  if (has('DH_WEAK', 'DH_SMALL')) return { key: 'headline.logjam', tone: has('DH_WEAK') ? 'bad' : 'warn' }
  if (r.pqc) return weakToday ? { key: 'headline.pqcWeak', tone: 'warn' } : { key: 'headline.pqc', tone: 'good' }
  if (has('KEX_EXPERIMENTAL_PQC_ONLY')) return { key: 'headline.pqcExperimental', tone: 'warn' }
  if (has('KEX_HYBRID_NOT_PREFERRED')) return { key: 'headline.notPreferred', tone: 'warn' }
  if (weakToday) return { key: 'headline.weak', tone: 'bad' }
  return { key: 'headline.classical', tone: 'warn' }
}

const friendly: Record<string, MessageKey> = {
  invalid_host: 'err.invalid_host',
  port_not_allowed: 'err.port_not_allowed',
  host_not_allowed: 'err.host_not_allowed',
  body_too_large: 'err.body_too_large',
  network: 'err.network',
  not_found: 'err.not_found',
}

/** A friendly message for an API error; falls back to the API's own text. */
export function errorMessage(err: ApiError, t: (k: MessageKey, v?: Record<string, string | number>) => string): string {
  if (err.code === 'rate_limited') {
    return err.retryAfter ? t('err.rate_limited', { seconds: err.retryAfter }) : t('err.rate_limited_nowait')
  }
  if (friendly[err.code]) return t(friendly[err.code])
  if (err.status >= 500) return t('err.internal')
  return err.message || t('err.internal')
}
