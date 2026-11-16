import { describe, expect, it } from 'vitest'
import { field, MEET, packet, PACKETS, psi, SAMPLES, sampleX, width, X_RANGE, type Complex } from './wave'

// Numerical checks of the closed form, independent of how it is written:
// the PDE itself, conservation of probability, the group velocity, the
// spreading law and the interference fringes.
const norm = (f: (x: number) => Complex, a = -60, b = 60, n = 24_000) => {
  const h = (b - a) / n
  let s = 0
  for (let i = 0; i <= n; i++) {
    const [re, im] = f(a + i * h)
    s += (i === 0 || i === n ? 0.5 : 1) * (re * re + im * im)
  }
  return s * h
}

describe('free Gaussian wave packets (ℏ = m = 1)', () => {
  it('t = 0 is the initial Gaussian (2πσ²)^(−1/4) exp(−(x−x₀)²/4σ² + i k₀(x−x₀))', () => {
    const p = PACKETS[0]
    for (const x of [-14, -12.5, -12, -10, -8]) {
      const u = x - p.x0
      const m = (2 * Math.PI * p.sigma ** 2) ** -0.25 * Math.exp(-(u * u) / (4 * p.sigma ** 2))
      const [re, im] = packet(p, x, 0)
      expect(re).toBeCloseTo(m * Math.cos(p.k0 * u), 12)
      expect(im).toBeCloseTo(m * Math.sin(p.k0 * u), 12)
    }
  })

  it('solves i ∂ψ/∂t = −½ ∂²ψ/∂x² (finite differences), for each packet and for the superposition', () => {
    const h = 1e-3
    const residual = (f: (x: number, t: number) => Complex, x: number, t: number) => {
      const dt = [(f(x, t + h)[0] - f(x, t - h)[0]) / (2 * h), (f(x, t + h)[1] - f(x, t - h)[1]) / (2 * h)]
      const d2 = [0, 1].map((c) => (f(x + h, t)[c] - 2 * f(x, t)[c] + f(x - h, t)[c]) / (h * h))
      // i ∂ψ/∂t = (−Im ∂tψ, Re ∂tψ); right side −½ ∂²ψ
      return Math.hypot(-dt[1] + 0.5 * d2[0], dt[0] + 0.5 * d2[1])
    }
    // Negative times too: the principal branch of α^(−1/2) must stay continuous through t = 0.
    for (const t of [-3, -0.5, 0.5, 3, MEET, 9])
      for (const x of [-11, -6, -1, 0, 2.5, 7]) {
        expect(residual((x, t) => packet(PACKETS[0], x, t), x, t)).toBeLessThan(2e-5)
        expect(residual(psi, x, t)).toBeLessThan(2e-5)
      }
  })

  it('probability is conserved: ∫|ψ|² dx = 1 at every time', () => {
    for (const t of [-4, 0, 2, MEET, 10]) expect(norm((x) => psi(x, t))).toBeCloseTo(1, 6)
  })

  it('each packet moves at the group velocity k₀ and spreads as σ√(1 + (t/2σ²)²)', () => {
    const p = PACKETS[0]
    for (const t of [0, 3, 8]) {
      const f = (x: number) => packet(p, x, t)
      const mean = (g: (x: number) => number) => {
        let s = 0
        for (let i = 0; i <= 24_000; i++) {
          const x = -60 + i * 0.005
          const [re, im] = f(x)
          s += g(x) * (re * re + im * im) * 0.005
        }
        return s
      }
      const m = mean((x) => x)
      expect(m).toBeCloseTo(p.x0 + p.k0 * t, 4)
      expect(Math.sqrt(mean((x) => (x - m) ** 2))).toBeCloseTo(width(p, t), 4)
    }
  })

  it('where they overlap, |ψ|² has fringes 2π/(k₁−k₂) = π/2 apart; the separate densities have none', () => {
    const dens = (x: number) => {
      const [re, im] = psi(x, MEET)
      return re * re + im * im
    }
    const minima: number[] = []
    for (let x = -3; x <= 3; x += 0.001) if (dens(x) < dens(x - 0.001) && dens(x) < dens(x + 0.001)) minima.push(x)
    expect(minima.length).toBeGreaterThanOrEqual(3)
    for (let i = 1; i < minima.length; i++) expect(minima[i] - minima[i - 1]).toBeCloseTo(Math.PI / 2, 2)
    // Without interference (|ψ₁|² + |ψ₂|²)/2 is smooth: no minimum near the centre.
    const classical = (x: number) => PACKETS.reduce((s, p) => s + Math.hypot(...packet(p, x, MEET)) ** 2, 0) / 2
    let mins = 0
    for (let x = -3; x <= 3; x += 0.001) if (classical(x) < classical(x - 0.001) && classical(x) < classical(x + 0.001)) mins++
    expect(mins).toBe(0)
  })

  it('field(): ψ on the drawn grid, scaled so the packet peak at t = 0 is 1', () => {
    const f = field(0)
    expect(f).toHaveLength(SAMPLES * 2)
    expect(sampleX(0)).toBeCloseTo(-X_RANGE + X_RANGE / SAMPLES, 12)
    let peak = 0
    for (let i = 0; i < SAMPLES; i++) peak = Math.max(peak, Math.hypot(f[2 * i], f[2 * i + 1]))
    expect(peak).toBeCloseTo(1, 2)
    const i = 300
    const [re, im] = psi(sampleX(i), 4)
    const g = field(4)
    expect(g[2 * i] / g[2 * i + 1]).toBeCloseTo(re / im, 5)
  })
})
