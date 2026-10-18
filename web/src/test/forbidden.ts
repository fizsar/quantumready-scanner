// What the showcase pages must never say (/architecture, /examples): exact
// software versions, proxy settings, bucket names, the server's location,
// configuration variables and, in the pages' own prose, concrete limits,
// timeouts or the server's address.

/**
 * Never, not even inside the reports shown on a page. (Reports may name
 * third-party minimum versions in their advice, e.g. "OpenSSL 3.5+": those come
 * from the scoring catalog and say nothing about our own stack.)
 */
export const forbiddenEverywhere: RegExp[] = [
  /TRUSTED_PROXIES|X-Forwarded-For|trusted prox|proxies de confianza/i,
  /fizsar-qr|backblaze|\bb2\b|bucket|eu-central|s3\./i,
  /\bOVH\b|canad|beauharnois|\bVPS\b/i,
  /RATE_LIMIT|SCAN_ALLOWLIST|SCAN_INTENSITY|QR_[A-Z]/,
]

/** Also never in the pages' own words. */
export const forbiddenProse: RegExp[] = [
  ...forbiddenEverywhere,
  /\b(Go|Golang|PostgreSQL|Postgres|Caddy|nginx|React|Node|Vite|Trivy|Alpine|Debian|distroless|restic|testssl\.sh|OpenSSL)\s*v?\d/i,
  /\bv?\d+\.\d+\.\d+\b/, // any x.y.z
  /149\.56/,
  /\d+\s*(\/|por |per )\s*(min|minuto|minute|día|day|hora|hour)/i,
  /\b\d+\s*(s|ms|seg|segundos|seconds)\b/i, // timeouts
  /\b(48|60)\b/, // raw probe budget, scan timeout
]

/** Protocol versions and the scoring model are fine. */
export const strip = (s: string) => s.replace(/\b(TLS|SSL) ?v?\d(\.\d)?/g, '').replace(/2026\.2/g, '')

/** Scan data may show the scanned and resolved IPs (public DNS answers). */
export const stripData = (s: string) => strip(s).replace(/\b\d{1,3}(\.\d{1,3}){3}\b/g, '')
