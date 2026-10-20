// Client for the QuantumReady API (docs/api.md). Same origin: in production
// Caddy routes /api/*, in development Vite proxies it.

import type { AltchaChallenge } from './altcha'

export type Lang = 'es' | 'en'

export type ScanStatus = 'queued' | 'running' | 'done' | 'failed'

export interface Labeled {
  code: string
  label: string
}

export interface Finding {
  code: string
  axis: 'quantum' | 'current'
  axis_label: string
  severity: 'critical' | 'high' | 'medium' | 'low' | 'info' | 'pass'
  severity_label: string
  category: string
  title: string
  detail: string
  remediation?: string
}

export interface Block {
  id: string
  label: string
  points: number
  max: number
  items?: { id: string; label: string; points: number; max: number }[]
}

export interface Report {
  scoring_version: string
  score: number
  grade: 'A' | 'B' | 'C' | 'D' | 'F'
  score_grade: string
  floor_points?: number
  cap?: { grade: string; findings: Labeled[] } | null
  pqc: boolean
  selected_group?: string
  tls_version?: string
  quantum_risk: Labeled
  current_risk: Labeled
  blocks: Block[]
  findings: Finding[]
  not_evaluated: Labeled[]
}

export interface ScanView {
  id: string
  host: string
  port: number
  status: ScanStatus
  retries: number
  created_at: string
  started_at?: string
  finished_at?: string
  error?: { code: string; message: string }
  reused?: boolean
  lang: Lang
  report?: Report
}

/** An error answered by the API ({"error": {...}}) or a network failure (status 0). */
export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly retryAfter?: number // seconds, from Retry-After

  constructor(status: number, code: string, message: string, retryAfter?: number) {
    super(message)
    this.status = status
    this.code = code
    this.retryAfter = retryAfter
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response
  try {
    res = await fetch(path, { ...init, headers: { Accept: 'application/json', ...init?.headers } })
  } catch {
    throw new ApiError(0, 'network', 'network error')
  }
  let body: unknown = null
  try {
    body = await res.json()
  } catch {
    // non-JSON answer (e.g. a proxy error page)
  }
  if (!res.ok) {
    const err = (body as { error?: { code?: string; message?: string } } | null)?.error
    const ra = Number(res.headers.get('Retry-After'))
    throw new ApiError(res.status, err?.code ?? 'internal', err?.message ?? res.statusText, Number.isFinite(ra) && ra > 0 ? ra : undefined)
  }
  return body as T
}

export function getTargets(): Promise<{ hosts: string[]; port: number }> {
  return request('/api/targets')
}

export function createScan(host: string, lang: Lang): Promise<ScanView> {
  return request(`/api/scans?lang=${lang}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ host }),
  })
}

export function getScan(id: string, lang: Lang): Promise<ScanView> {
  return request(`/api/scans/${encodeURIComponent(id)}?lang=${lang}`)
}

export const isFinished = (s: ScanView) => s.status === 'done' || s.status === 'failed'

/** Polling delay before attempt n (0-based): 1 s, 1.5 s, 2.25 s… up to 8 s. */
export function pollDelay(n: number): number {
  return Math.min(1000 * 1.5 ** n, 8000)
}

/** What the "Request a free demo" form sends (POST /api/demo). */
export interface DemoRequest {
  name: string
  email: string
  company: string
  domain: string
  message: string
  privacy: boolean
  lang: Lang
  website: string // honeypot: always empty for people
  altcha?: string
}

/** The proof-of-work challenge of the demo form, or null when the server asks for none. */
export async function getDemoChallenge(): Promise<AltchaChallenge | null> {
  try {
    return await request<AltchaChallenge>('/api/demo/challenge')
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) return null
    throw e
  }
}

export function createDemo(req: DemoRequest): Promise<{ status: string }> {
  return request(`/api/demo?lang=${req.lang}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
}
