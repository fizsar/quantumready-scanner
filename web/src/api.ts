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
  intensity?: 'basic' | 'full'
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
  /** False for accounts created by invitation during the beta. */
  can_create_organizations: boolean
}

const CSRF_HEADER = 'X-QR-CSRF'

function send<T>(method: string, path: string, lang: Lang, body?: unknown, csrf?: string): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (csrf) headers[CSRF_HEADER] = csrf
  return request(`${path}?lang=${lang}`, { method, headers, body: JSON.stringify(body ?? {}), credentials: 'same-origin' })
}

const post = <T,>(path: string, lang: Lang, body?: unknown, csrf?: string) => send<T>('POST', path, lang, body, csrf)

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

// --- Organizations (/api/org: always the session's active organization).

export type Role = 'owner' | 'admin' | 'viewer'
export type OrgAction = 'view' | 'rename' | 'invite' | 'revoke_invitation' | 'change_role' | 'remove_member' | 'manage_domains'

export interface Org {
  id: string
  name: string
  role: Role
  permissions: OrgAction[]
}

export interface Member {
  user_id: string
  email: string
  name?: string
  role: Role
  joined_at: string
  you: boolean
}

export interface Invitation {
  id: string
  email: string
  role: Role
  inviter_email?: string
  created_at: string
  expires_at: string
  expired: boolean
}

export interface InvitationPreview {
  organization: string
  email: string
  role: Role
  inviter_email?: string
  expires_at: string
}

const get = <T,>(path: string, lang: Lang): Promise<T> => request(`${path}?lang=${lang}`, { credentials: 'same-origin' })

export const getOrg = (lang: Lang) => get<Org>('/api/org', lang)
export const renameOrg = (name: string, csrf: string, lang: Lang) => send<null>('PATCH', '/api/org', lang, { name }, csrf)
export const listMembers = (lang: Lang) => get<{ members: Member[] }>('/api/org/members', lang).then((r) => r.members)
export const changeRole = (userId: string, role: Role, csrf: string, lang: Lang) =>
  send<null>('PATCH', `/api/org/members/${encodeURIComponent(userId)}`, lang, { role }, csrf)
export const removeMember = (userId: string, csrf: string, lang: Lang) =>
  send<null>('DELETE', `/api/org/members/${encodeURIComponent(userId)}`, lang, undefined, csrf)
export const leaveOrg = (csrf: string, lang: Lang) => post<null>('/api/org/leave', lang, undefined, csrf)
export const listInvitations = (lang: Lang) => get<{ invitations: Invitation[] }>('/api/org/invitations', lang).then((r) => r.invitations)
export const createInvitation = (email: string, role: Role, csrf: string, lang: Lang) =>
  post<Invitation>('/api/org/invitations', lang, { email, role, lang }, csrf)
export const revokeInvitation = (id: string, csrf: string, lang: Lang) =>
  send<null>('DELETE', `/api/org/invitations/${encodeURIComponent(id)}`, lang, undefined, csrf)
export const previewInvitation = (token: string, lang: Lang) => post<InvitationPreview>('/api/invitations/preview', lang, { token })
export const acceptInvitation = (token: string, csrf: string, lang: Lang) => post<Org>('/api/invitations/accept', lang, { token }, csrf)
export const registerInvited = (email: string, password: string, invitationToken: string, lang: Lang) =>
  post<{ status: string }>('/api/auth/register', lang, { email, password, invitation_token: invitationToken, lang })
export const createOrganization = (name: string, csrf: string, lang: Lang) => post<Org>('/api/organizations', lang, { name }, csrf)
export const switchOrganization = (id: string, csrf: string, lang: Lang) =>
  post<Org>(`/api/organizations/${encodeURIComponent(id)}/switch`, lang, undefined, csrf)

// --- Domains and full scans of the active organization.

export interface Domain {
  id: string
  name: string
  status: 'pending' | 'verified' | 'lost'
  verified: boolean
  verified_at?: string
  last_checked_at?: string
  lost_at?: string
  grace_until?: string
  check_pending: boolean
  last_check_error?: { code: string; message: string }
  record: { type: 'TXT'; name: string; value: string }
}

export interface ScanSummary {
  id: string
  host: string
  status: ScanStatus
  intensity: 'basic' | 'full'
  created_at: string
  finished_at?: string
  score?: number
  grade?: string
}

export const listDomains = (lang: Lang) => get<{ domains: Domain[] }>('/api/org/domains', lang).then((r) => r.domains)
export const getDomain = (id: string, lang: Lang) => get<Domain>(`/api/org/domains/${encodeURIComponent(id)}`, lang)
export const addDomain = (name: string, csrf: string, lang: Lang) => post<Domain>('/api/org/domains', lang, { name }, csrf)
export const deleteDomain = (id: string, csrf: string, lang: Lang) =>
  send<null>('DELETE', `/api/org/domains/${encodeURIComponent(id)}`, lang, undefined, csrf)
export const verifyDomain = (id: string, csrf: string, lang: Lang) =>
  post<Domain>(`/api/org/domains/${encodeURIComponent(id)}/verify`, lang, undefined, csrf)
export const listOrgScans = (lang: Lang) => get<{ scans: ScanSummary[] }>('/api/org/scans', lang).then((r) => r.scans)
export const createOrgScan = (host: string, csrf: string, lang: Lang) => post<ScanView>('/api/org/scans', lang, { host }, csrf)
export const getOrgScan = (id: string, lang: Lang) => get<ScanView>(`/api/org/scans/${encodeURIComponent(id)}`, lang)

export const isFinished = (s: ScanView) => s.status === 'done' || s.status === 'failed'

/** Polling delay before attempt n (0-based): 1 s, 1.5 s, 2.25 s… up to 8 s. */
export function pollDelay(n: number): number {
  return Math.min(1000 * 1.5 ** n, 8000)
}
