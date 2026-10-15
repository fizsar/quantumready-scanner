// Client for the QuantumReady API (docs/api.md). Same origin: in production
// Caddy routes /api/*, in development Vite proxies it.

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

// --- Accounts (/api/auth). The session lives in an HttpOnly cookie; the
// state-changing calls of a signed-in user carry its CSRF token in a header.

export interface Me {
  user: { id: string; email: string; name?: string; lang: Lang }
  organizations: { id: string; name: string; role: 'owner' | 'admin' | 'viewer' }[]
  active_org_id: string | null
  csrf_token: string
}

const CSRF_HEADER = 'X-QR-CSRF'

function post<T>(path: string, lang: Lang, body?: unknown, csrf?: string): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (csrf) headers[CSRF_HEADER] = csrf
  return request(`${path}?lang=${lang}`, { method: 'POST', headers, body: JSON.stringify(body ?? {}), credentials: 'same-origin' })
}

export const getMe = (lang: Lang): Promise<Me> => request(`/api/auth/me?lang=${lang}`, { credentials: 'same-origin' })

export interface RegisterInput {
  email: string
  password: string
  name?: string
  organization?: string
  signup_code: string
}

export const register = (input: RegisterInput, lang: Lang) => post<{ status: string }>('/api/auth/register', lang, { ...input, lang })
export const login = (email: string, password: string, lang: Lang) => post<Me>('/api/auth/login', lang, { email, password })
export const verifyEmail = (token: string, lang: Lang) => post<{ status: string }>('/api/auth/verify-email', lang, { token })
export const resendVerification = (email: string, lang: Lang) => post<{ status: string }>('/api/auth/resend-verification', lang, { email, lang })
export const forgotPassword = (email: string, lang: Lang) => post<{ status: string }>('/api/auth/forgot-password', lang, { email, lang })
export const resetPassword = (token: string, password: string, lang: Lang) =>
  post<{ status: string }>('/api/auth/reset-password', lang, { token, password })
export const logout = (csrf: string, lang: Lang) => post<null>('/api/auth/logout', lang, undefined, csrf)
export const changePassword = (current: string, next: string, csrf: string, lang: Lang) =>
  post<null>('/api/auth/change-password', lang, { current_password: current, new_password: next }, csrf)

export const isFinished = (s: ScanView) => s.status === 'done' || s.status === 'failed'

/** Polling delay before attempt n (0-based): 1 s, 1.5 s, 2.25 s… up to 8 s. */
export function pollDelay(n: number): number {
  return Math.min(1000 * 1.5 ** n, 8000)
}
