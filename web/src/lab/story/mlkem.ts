// Scene 7: ML-KEM (NIST FIPS 203, "Module-Lattice-Based Key-Encapsulation
// Mechanism Standard", August 2024) and the hybrid X25519MLKEM768 of TLS 1.3
// (RFC 10024, August 2026, formerly draft-ietf-tls-ecdhe-mlkem-05). Sizes are
// computed from the parameters with FIPS 203's formulas, not typed in.

/** FIPS 203 §8, Table 2: n = 256 coefficients per polynomial, modulus q = 3329. */
export const N = 256
export const Q = 3329

export interface MlKem {
  name: string
  k: number
  eta1: number
  eta2: number
  du: number
  dv: number
}
/** FIPS 203 Table 2. */
export const ML_KEM: Record<'512' | '768' | '1024', MlKem> = {
  '512': { name: 'ML-KEM-512', k: 2, eta1: 3, eta2: 2, du: 10, dv: 4 },
  '768': { name: 'ML-KEM-768', k: 3, eta1: 2, eta2: 2, du: 10, dv: 4 },
  '1024': { name: 'ML-KEM-1024', k: 4, eta1: 2, eta2: 2, du: 11, dv: 5 },
}

/**
 * Sizes in bytes (FIPS 203 §7 and Table 3): the encapsulation (public) key is
 * k polynomials of 256 12-bit coefficients plus the 32-byte seed ρ; the
 * ciphertext is k polynomials compressed to d_u bits and one to d_v bits; the
 * shared secret is 32 bytes.
 */
export const sizes = (p: MlKem) => ({
  ek: (N * 12 * p.k) / 8 + 32,
  dk: (N * 12 * p.k) / 8 + (N * 12 * p.k) / 8 + 32 + 32 + 32,
  ct: (N * (p.du * p.k + p.dv)) / 8,
  ss: 32,
})

/** X25519 (RFC 7748): 32-byte public values and a 32-byte shared secret. */
export const X25519 = { pub: 32, ss: 32 } as const

/**
 * X25519MLKEM768 (RFC 10024 §4): the client's key_exchange is the ML-KEM-768
 * encapsulation key followed by its X25519 share; the server's, the ML-KEM
 * ciphertext followed by its X25519 share; the shared secret, the ML-KEM secret
 * followed by the X25519 one (ML-KEM first in this group, for historical
 * reasons, unlike SecP256r1MLKEM768). Breaking it means breaking both.
 */
export const HYBRID = (() => {
  const m = sizes(ML_KEM['768'])
  return {
    name: 'X25519MLKEM768',
    /** IANA TLS Supported Groups, value 4588 (0x11EC), Recommended: Y, reference RFC 10024 (checked 2026-11-07). */
    codepoint: 4588,
    client: [
      { part: 'ML-KEM-768 ek', bytes: m.ek },
      { part: 'X25519', bytes: X25519.pub },
    ],
    server: [
      { part: 'ML-KEM-768 ct', bytes: m.ct },
      { part: 'X25519', bytes: X25519.pub },
    ],
    secret: [
      { part: 'ML-KEM-768', bytes: m.ss },
      { part: 'X25519', bytes: X25519.ss },
    ],
  }
})()
export const total = (parts: readonly { bytes: number }[]) => parts.reduce((t, p) => t + p.bytes, 0)

/**
 * Decimal digits of (2η₁ + 1)^(k·n): how many secret vectors there are when each
 * of the k·n coefficients is small, in {−η₁, …, η₁} (FIPS 203 samples them
 * from the centred binomial distribution CBD_η₁). Not a security level (the best
 * attacks are lattice reduction, far below brute force): only a sense of why
 * trying them all is out of the question.
 */
export const spaceDigits = (p: MlKem) => Math.floor(p.k * N * Math.log10(2 * p.eta1 + 1)) + 1
export const hex = (v: number) => `0x${v.toString(16).toUpperCase().padStart(4, '0')}`
