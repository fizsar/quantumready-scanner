// Scene 2: a free particle in a superposition of two Gaussian wave packets
// moving towards each other. Units ℏ = m = 1. Pure functions, no DOM: the
// 3D scene, the build-time still images and the tests use this code.
//
// Each packet is the exact solution of the free Schrödinger equation
//   i ∂ψ/∂t = −½ ∂²ψ/∂x²
// with initial value ψ(x, 0) = (2πσ²)^(−1/4) exp(−(x−x₀)²/(4σ²) + i k₀ (x−x₀)):
//   ψ(x, t) = (2πσ²)^(−1/4) α^(−1/2) exp([−(x−x₀)²/(4σ²) + i k₀ (x−x₀) − i k₀² t/2] / α),
//   α = 1 + i t/(2σ²)
// (Griffiths & Schroeter, Introduction to Quantum Mechanics, 3rd ed., §2.4,
// the free particle; Cohen-Tannoudji, Diu, Laloë, Quantum Mechanics, vol. 1,
// complement G_I, the Gaussian wave packet). The equation is linear, so the
// sum of two packets is a solution too: ψ = (ψ₁ + ψ₂)/√2, normalized while
// they start far apart (overlap e^(−32) at t = 0).

export type Complex = [number, number]

export interface Packet {
  x0: number
  k0: number
  sigma: number
}

/** Two packets 24 apart, at speed ±2: they meet at x = 0 at t = 6. */
export const PACKETS: readonly Packet[] = [
  { x0: -12, k0: 2, sigma: 1.5 },
  { x0: 12, k0: -2, sigma: 1.5 },
]
/** When the packets overlap the most. */
export const MEET = 6

const div = (a: Complex, b: Complex): Complex => {
  const d = b[0] * b[0] + b[1] * b[1]
  return [(a[0] * b[0] + a[1] * b[1]) / d, (a[1] * b[0] - a[0] * b[1]) / d]
}
const exp = (a: Complex): Complex => {
  const m = Math.exp(a[0])
  return [m * Math.cos(a[1]), m * Math.sin(a[1])]
}
/** Principal square root (Re α = 1 > 0, so it is continuous in t). */
const sqrt = (a: Complex): Complex => {
  const r = Math.hypot(a[0], a[1])
  const re = Math.sqrt((r + a[0]) / 2)
  const im = Math.sign(a[1]) * Math.sqrt(Math.max(0, (r - a[0]) / 2))
  return [re, im]
}

/** One packet at (x, t). */
export function packet(p: Packet, x: number, t: number): Complex {
  const s2 = p.sigma * p.sigma
  const alpha: Complex = [1, t / (2 * s2)]
  const u = x - p.x0
  const exponent: Complex = [-(u * u) / (4 * s2), p.k0 * u - (p.k0 * p.k0 * t) / 2]
  const pref = (2 * Math.PI * s2) ** -0.25
  const v = div(exp(div(exponent, alpha)), sqrt(alpha))
  return [pref * v[0], pref * v[1]]
}

/** ψ = (ψ₁ + ψ₂)/√2. */
export function psi(x: number, t: number, packets: readonly Packet[] = PACKETS): Complex {
  let re = 0
  let im = 0
  for (const p of packets) {
    const v = packet(p, x, t)
    re += v[0]
    im += v[1]
  }
  const n = 1 / Math.sqrt(packets.length)
  return [re * n, im * n]
}

/** Width of a packet at time t: σ(t) = σ √(1 + (t / 2σ²)²). */
export const width = (p: Packet, t: number) => p.sigma * Math.sqrt(1 + (t / (2 * p.sigma * p.sigma)) ** 2)

/** The x range drawn, and how many samples the field texture holds. */
export const X_RANGE = 20
export const SAMPLES = 512
export const sampleX = (i: number) => -X_RANGE + (2 * X_RANGE * (i + 0.5)) / SAMPLES

/**
 * ψ at the SAMPLES points of the drawn range, scaled so that the peak of
 * |ψ| at t = 0 is 1 (what the 3D scene and the still images draw).
 */
export function field(t: number, out = new Float32Array(SAMPLES * 2)): Float32Array {
  for (let i = 0; i < SAMPLES; i++) {
    const v = psi(sampleX(i), t)
    out[2 * i] = v[0] / PEAK
    out[2 * i + 1] = v[1] / PEAK
  }
  return out
}
/** |ψ| at the centre of a packet at t = 0. */
const PEAK = Math.abs(psi(PACKETS[0].x0, 0)[0])

