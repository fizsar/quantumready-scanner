// The scanner facts in report.result (internal/scanner/result.go), typed only
// as far as the "Under the hood" panel reads them, and how a probe's outcome
// reads as a server reply. Nothing here adds data: every value shown comes
// from the API response.
import type { Report, ScanView } from './api'

export type ProbeStatus = 'supported' | 'not_supported' | 'error'

export interface VersionProbe {
  version: string
  id: number
  status: ProbeStatus
  error?: string
}

export interface GroupProbe {
  name: string
  id: number
  hybrid: boolean
  status: ProbeStatus
  error?: string
}

export interface CipherSuite {
  name: string
  id: number
  forward_secrecy: boolean
  aead: boolean
}

export interface RawSuite extends CipherSuite {
  version: string
  mode: string
  bits: number
  export?: boolean
  anonymous?: boolean
}

export interface RawProtocol {
  status: ProbeStatus
  ciphers?: string[]
  error?: string
}

export interface ScanResult {
  host: string
  port: number
  ip: string
  resolved_ips: string[] | null
  duration_ms: number
  default: { status: ProbeStatus; version?: string; cipher_suite?: string; group?: string; group_id?: number; error?: string }
  key_exchange: { offered: string[] | null; selected?: string; selected_hybrid: boolean; groups: GroupProbe[] | null }
  versions: VersionProbe[] | null
  tls12_cipher_suites: CipherSuite[] | null
  tls12_cipher_suites_complete: boolean
  tls10_cipher_suites?: CipherSuite[]
  tls11_cipher_suites?: CipherSuite[]
  legacy_cipher_suites_complete: boolean
  tls13_cipher_suite?: string
  intensity?: 'basic' | 'full'
  connections?: number
  raw?: {
    probes: number
    budget_exhausted?: boolean
    // suites, tls13_groups and tls12_curves hold only what crypto/tls did not
    // already find; scans before S6d lack this field and may hold null.
    lists_exclude_crypto_tls?: boolean
    sslv2: RawProtocol
    sslv3: RawProtocol
    versions?: VersionProbe[]
    suites: RawSuite[] | null
    suites_complete: boolean
    tls13_suites: RawSuite[] | null
    tls13_suites_complete: boolean
    dh?: { version: string; suite: string; prime_bits: number }[]
    tls13_groups: { name: string; id: number }[] | null
    tls13_groups_complete: boolean
    tls12_curves: { name: string; id: number }[] | null
    tls12_curves_complete: boolean
  }
}

/** The scanner facts of a report, when the API sent them. */
export function resultOf(report: Report): ScanResult | undefined {
  const r = (report as Report & { result?: unknown }).result
  return r && typeof r === 'object' ? (r as ScanResult) : undefined
}

// TLS alert descriptions (IANA registry): code, registry name and the text Go's
// crypto/tls prints after "remote error: tls: ".
const ALERTS: [number, string, string][] = [
  [0, 'close_notify', 'close notify'],
  [10, 'unexpected_message', 'unexpected message'],
  [20, 'bad_record_mac', 'bad record MAC'],
  [21, 'decryption_failed', 'decryption failed'],
  [22, 'record_overflow', 'record overflow'],
  [30, 'decompression_failure', 'decompression failure'],
  [40, 'handshake_failure', 'handshake failure'],
  [42, 'bad_certificate', 'bad certificate'],
  [43, 'unsupported_certificate', 'unsupported certificate'],
  [44, 'certificate_revoked', 'revoked certificate'],
  [45, 'certificate_expired', 'expired certificate'],
  [46, 'certificate_unknown', 'unknown certificate'],
  [47, 'illegal_parameter', 'illegal parameter'],
  [48, 'unknown_ca', 'unknown certificate authority'],
  [49, 'access_denied', 'access denied'],
  [50, 'decode_error', 'error decoding message'],
  [51, 'decrypt_error', 'error decrypting message'],
  [60, 'export_restriction', 'export restriction'],
  [70, 'protocol_version', 'protocol version not supported'],
  [71, 'insufficient_security', 'insufficient security level'],
  [80, 'internal_error', 'internal error'],
  [86, 'inappropriate_fallback', 'inappropriate fallback'],
  [90, 'user_canceled', 'user canceled'],
  [100, 'no_renegotiation', 'no renegotiation'],
  [109, 'missing_extension', 'missing extension'],
  [110, 'unsupported_extension', 'unsupported extension'],
  [111, 'certificate_unobtainable', 'certificate unobtainable'],
  [112, 'unrecognized_name', 'unrecognized name'],
  [113, 'bad_certificate_status_response', 'bad certificate status response'],
  [114, 'bad_certificate_hash_value', 'bad certificate hash value'],
  [115, 'unknown_psk_identity', 'unknown PSK identity'],
  [116, 'certificate_required', 'certificate required'],
  [120, 'no_application_protocol', 'no application protocol'],
  [121, 'ech_required', 'encrypted client hello required'],
]

/** What the server answered to one probe. */
export type Reply =
  | { kind: 'hello'; version?: string; suite?: string; group?: string; ciphers?: number }
  // byName: the response only names the alert; the code is the IANA one for that name.
  | { kind: 'alert'; code?: number; name: string; byName?: boolean }
  | { kind: 'closed' }
  | { kind: 'timeout' }
  | { kind: 'refused' }
  | { kind: 'other'; text: string }

/** Reads a probe's error string (as the scanner wrote it) as a server reply. */
export function parseError(error: string | undefined): Reply {
  if (!error) return { kind: 'refused' }
  const goText = /remote error: tls: (.+)$/.exec(error)?.[1]
  if (goText) {
    const a = ALERTS.find(([, , text]) => text === goText)
    return a ? { kind: 'alert', code: a[0], name: a[1], byName: true } : { kind: 'alert', name: goText }
  }
  const code = /^alert\((\d+)\)$/.exec(error)?.[1]
  if (code) {
    const a = ALERTS.find(([c]) => c === Number(code))
    return { kind: 'alert', code: Number(code), name: a?.[1] ?? 'unknown' }
  }
  const name = /^alert ([a-z_]+)$/.exec(error)?.[1]
  if (name) {
    const a = ALERTS.find(([, n]) => n === name)
    return a ? { kind: 'alert', code: a[0], name, byName: true } : { kind: 'alert', name }
  }
  if (/timeout|timed out|deadline exceeded/i.test(error)) return { kind: 'timeout' }
  if (/^(unexpected )?EOF$|connection reset|broken pipe|closed/i.test(error)) return { kind: 'closed' }
  return { kind: 'other', text: error }
}

export type ProbeSource = 'cryptotls' | 'raw'

export type Offer =
  | { kind: 'default'; groups: string[] }
  | { kind: 'version'; version: string }
  | { kind: 'group'; group: string; id: number }

export interface ProbeRow {
  source: ProbeSource
  offer: Offer
  reply: Reply
}

const protoReply = (p: { status: ProbeStatus; error?: string }, hello: Extract<Reply, { kind: 'hello' }>): Reply =>
  p.status === 'supported' ? hello : parseError(p.error)

/**
 * The probes the response records one by one, in the order the scanner runs
 * them: the browser-like handshake, one per version, one per group, then the
 * raw SSL and TLS version probes. (The raw probes that enumerate suites and
 * groups are only kept as their findings.)
 */
export function probeRows(r: ScanResult): ProbeRow[] {
  const rows: ProbeRow[] = [
    {
      source: 'cryptotls',
      offer: { kind: 'default', groups: r.key_exchange.offered ?? [] },
      reply: protoReply(r.default, { kind: 'hello', version: r.default.version, suite: r.default.cipher_suite, group: r.default.group }),
    },
  ]
  for (const v of r.versions ?? []) {
    rows.push({ source: 'cryptotls', offer: { kind: 'version', version: v.version }, reply: protoReply(v, { kind: 'hello', version: v.version }) })
  }
  for (const g of r.key_exchange.groups ?? []) {
    rows.push({ source: 'cryptotls', offer: { kind: 'group', group: g.name, id: g.id }, reply: protoReply(g, { kind: 'hello', group: g.name }) })
  }
  if (r.raw) {
    const raw: ProbeRow[] = []
    for (const [version, p] of [['SSLv2', r.raw.sslv2], ['SSLv3', r.raw.sslv3]] as const) {
      raw.push({ source: 'raw', offer: { kind: 'version', version }, reply: protoReply(p, { kind: 'hello', version, ciphers: p.ciphers?.length }) })
    }
    for (const v of r.raw.versions ?? []) {
      raw.push({ source: 'raw', offer: { kind: 'version', version: v.version }, reply: protoReply(v, { kind: 'hello', version: v.version }) })
    }
    // Oldest version first: SSLv2, SSLv3, TLS 1.0, TLS 1.1…
    const rank = (row: ProbeRow) => VERSION_ORDER.indexOf(row.offer.kind === 'version' ? row.offer.version : '')
    rows.push(...raw.sort((a, b) => rank(a) - rank(b)))
  }
  return rows
}

const VERSION_ORDER = ['SSLv2', 'SSLv3', 'TLS 1.0', 'TLS 1.1', 'TLS 1.2', 'TLS 1.3']

/** The scanner's error when the raw probe budget refused a connection (internal/scanner/raw.go). */
const BUDGET_ERROR = 'raw probe budget exhausted'

/**
 * How many raw probes the panel does not list one by one: raw.probes counts
 * every raw connection made (internal/scanner/raw.go, exchange), and each
 * listed raw row (SSLv2, SSLv3, every raw.versions entry) is exactly one
 * connection, unless the budget refused it (then it was not made). Undefined
 * when the numbers do not add up, so the panel says it without a number.
 */
export function otherRawProbes(r: ScanResult): number | undefined {
  if (!r.raw) return undefined
  const listed = [r.raw.sslv2, r.raw.sslv3, ...(r.raw.versions ?? [])].filter((p) => !p.error?.includes(BUDGET_ERROR)).length
  const others = r.raw.probes - listed
  return others >= 0 ? others : undefined
}

/** Every version tried, oldest first, with whether any probe got it accepted. */
export function versionsTried(r: ScanResult): { version: string; accepted: boolean }[] {
  const order = ['SSLv2', 'SSLv3', 'TLS 1.0', 'TLS 1.1', 'TLS 1.2', 'TLS 1.3']
  const seen = new Map<string, boolean>()
  const add = (v: string, ok: boolean) => seen.set(v, (seen.get(v) ?? false) || ok)
  for (const v of r.versions ?? []) add(v.version, v.status === 'supported')
  if (r.default.status === 'supported' && r.default.version) add(r.default.version, true)
  if (r.raw) {
    add('SSLv2', r.raw.sslv2.status === 'supported')
    add('SSLv3', r.raw.sslv3.status === 'supported')
    for (const v of r.raw.versions ?? []) add(v.version, v.status === 'supported')
  }
  return [...seen].map(([version, accepted]) => ({ version, accepted })).sort((a, b) => order.indexOf(a.version) - order.indexOf(b.version))
}

export interface SuiteRow {
  name: string
  id: number
  forward_secrecy: boolean
  aead: boolean
  bits?: number
  export?: boolean
  anonymous?: boolean
  source: ProbeSource
}

/** Accepted suites by version, newest first, crypto/tls findings before raw ones. */
export function suitesByVersion(r: ScanResult): { version: string; suites: SuiteRow[]; complete: boolean }[] {
  const raw = (version: string) =>
    (r.raw?.suites ?? []).filter((s) => s.version === version).map((s): SuiteRow => ({ ...s, source: 'raw' }))
  const std = (list: CipherSuite[] | null | undefined) => (list ?? []).map((s): SuiteRow => ({ ...s, source: 'cryptotls' }))
  const out: { version: string; suites: SuiteRow[]; complete: boolean }[] = []
  const tls13 = (r.raw?.tls13_suites ?? []).map((s): SuiteRow => ({ ...s, source: 'raw' }))
  if (!tls13.length && r.tls13_cipher_suite) tls13.push({ name: r.tls13_cipher_suite, id: 0, forward_secrecy: true, aead: true, source: 'cryptotls' })
  out.push({ version: 'TLS 1.3', suites: tls13, complete: r.raw ? r.raw.tls13_suites_complete : false })
  out.push({ version: 'TLS 1.2', suites: [...std(r.tls12_cipher_suites), ...raw('TLS 1.2')], complete: r.tls12_cipher_suites_complete && (r.raw?.suites_complete ?? true) })
  const legacy = r.legacy_cipher_suites_complete && (r.raw?.suites_complete ?? true)
  out.push({ version: 'TLS 1.1', suites: [...std(r.tls11_cipher_suites), ...raw('TLS 1.1')], complete: legacy })
  out.push({ version: 'TLS 1.0', suites: [...std(r.tls10_cipher_suites), ...raw('TLS 1.0')], complete: legacy })
  out.push({ version: 'SSLv3', suites: raw('SSLv3'), complete: legacy })
  return out.filter((v) => v.suites.length > 0)
}

/** Saves the API response as a file, from memory (no request, no inline script). */
export function downloadJSON(scan: ScanView) {
  const blob = new Blob([JSON.stringify(scan, null, 2) + '\n'], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `quantumready-${scan.host}-${scan.id.slice(0, 8)}.json`
  document.body.append(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 0)
}

/** 0x11ec style, as the IANA registries write code points. */
export const hex = (id: number) => '0x' + id.toString(16).padStart(4, '0')
