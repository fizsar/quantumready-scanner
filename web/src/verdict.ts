import { ApiError, type Report } from './api'
import type { MessageKey } from './i18n'

export type Tone = 'good' | 'warn' | 'bad'

/**
 * The plain-language sentence at the top of a result, for someone without a
 * technical background. Order matters: a broken certificate outweighs
 * everything, then post-quantum status, then today's weaknesses.
 */
export function headline(r: Report): { key: MessageKey; tone: Tone } {
  const has = (code: string) => r.findings.some((f) => f.code === code)
  const weakToday = r.current_risk.code === 'high' || r.current_risk.code === 'critical'
  if (r.cap?.grade === 'F') return { key: 'headline.broken', tone: 'bad' }
  if (r.pqc) return weakToday ? { key: 'headline.pqcWeak', tone: 'warn' } : { key: 'headline.pqc', tone: 'good' }
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
