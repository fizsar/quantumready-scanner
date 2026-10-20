// Proof of work for open public scans (internal/altcha): find n such that
// SHA-256(salt + n) is the challenge. 25 000 hashes on average, with the
// browser's own crypto (hashed in batches: one await per hash is several
// times slower): no third-party script, nothing leaves the page but the
// answer.

export interface AltchaChallenge {
  algorithm: string
  challenge: string
  maxnumber: number
  salt: string
  signature: string
}

const BATCH = 200

const hex = (buf: ArrayBuffer) => Array.from(new Uint8Array(buf), (b) => b.toString(16).padStart(2, '0')).join('')

/** Solves the challenge; returns the payload for the API (base64 JSON). */
export async function solveAltcha(c: AltchaChallenge, signal?: AbortSignal): Promise<string> {
  if (c.algorithm !== 'SHA-256') throw new Error('unsupported algorithm')
  const enc = new TextEncoder()
  for (let start = 0; start <= c.maxnumber; start += BATCH) {
    if (signal?.aborted) throw new DOMException('aborted', 'AbortError')
    const ns = Array.from({ length: Math.min(BATCH, c.maxnumber + 1 - start) }, (_, i) => start + i)
    const hashes = await Promise.all(ns.map((n) => crypto.subtle.digest('SHA-256', enc.encode(c.salt + n))))
    const i = hashes.findIndex((h) => hex(h) === c.challenge)
    if (i >= 0) {
      return btoa(JSON.stringify({ algorithm: c.algorithm, challenge: c.challenge, number: ns[i], salt: c.salt, signature: c.signature }))
    }
  }
  throw new Error('no solution')
}
