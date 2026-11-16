// Scenes 3–5: qubits, circuits and Shor's algorithm, computed exactly (small
// state vectors, no shortcuts). Pure functions, no DOM: the shapes the
// particles form, the captions and the tests all come from here.
//
// Sources (also cited on the page): Nielsen & Chuang, Quantum Computation and
// Quantum Information, 10th anniversary ed. (2010): the Bloch sphere (§1.2,
// §4.2), Bell states (§1.3.6), partial trace and reduced states (§2.4.3), the
// quantum Fourier transform (§5.1) and order finding / Shor (§5.3); P. W. Shor,
// SIAM J. Comput. 26(5) 1484–1509 (1997).

export type C = [number, number]
const add = (a: C, b: C): C => [a[0] + b[0], a[1] + b[1]]
const mul = (a: C, b: C): C => [a[0] * b[0] - a[1] * b[1], a[0] * b[1] + a[1] * b[0]]
const conj = (a: C): C => [a[0], -a[1]]
export const abs2 = (a: C) => a[0] * a[0] + a[1] * a[1]

/** A state vector over 2^n basis states |b⟩, b read as a binary number (qubit 0 = most significant). */
export type State = C[]

export const norm2 = (s: State) => s.reduce((t, a) => t + abs2(a), 0)

/** |ψ⟩ = cos(θ/2)|0⟩ + e^{iφ} sin(θ/2)|1⟩. */
export const qubit = (theta: number, phi: number): State => [
  [Math.cos(theta / 2), 0],
  [Math.cos(phi) * Math.sin(theta / 2), Math.sin(phi) * Math.sin(theta / 2)],
]

/** The Bloch vector (⟨X⟩, ⟨Y⟩, ⟨Z⟩) of a one-qubit pure state: (sin θ cos φ, sin θ sin φ, cos θ). */
export function bloch(s: State): [number, number, number] {
  const [a, b] = s
  const ab = mul(conj(a), b) // ā·b
  return [2 * ab[0], 2 * ab[1], abs2(a) - abs2(b)]
}

const R2 = Math.SQRT1_2
/** The Hadamard gate: |0⟩ → (|0⟩ + |1⟩)/√2, |1⟩ → (|0⟩ − |1⟩)/√2. */
export const H: C[][] = [
  [[R2, 0], [R2, 0]],
  [[R2, 0], [-R2, 0]],
]
export const I2: C[][] = [
  [[1, 0], [0, 0]],
  [[0, 0], [1, 0]],
]
/** CNOT on two qubits, control = qubit 0, target = qubit 1 (basis |00⟩, |01⟩, |10⟩, |11⟩). */
export const CNOT: C[][] = [0, 1, 3, 2].map((col) => [0, 1, 2, 3].map((row): C => [row === col ? 1 : 0, 0]))

export function apply(m: C[][], s: State): State {
  return m.map((row) => row.reduce((t, a, j) => add(t, mul(a, s[j])), [0, 0] as C))
}
export function kron(a: C[][], b: C[][]): C[][] {
  const out: C[][] = []
  for (let i = 0; i < a.length; i++)
    for (let k = 0; k < b.length; k++) out.push(a[i].flatMap((x) => b[k].map((y) => mul(x, y))))
  return out
}
/** |0…0⟩ on n qubits. */
export const zeros = (n: number): State => Array.from({ length: 2 ** n }, (_, i): C => [i === 0 ? 1 : 0, 0])

/** Probabilities of each basis outcome. */
export const probabilities = (s: State) => s.map(abs2)

/** After measuring and getting `outcome`, the state collapses to that basis state. */
export const collapse = (s: State, outcome: number): State => s.map((_, i): C => [i === outcome ? 1 : 0, 0])

/** The Bell state (|00⟩ + |11⟩)/√2, built as the circuit does: CNOT (H ⊗ I) |00⟩. */
export const bell = (): State => apply(CNOT, apply(kron(H, I2), zeros(2)))

/** Reduced density matrix of qubit q (0 or 1) of a two-qubit state: the partial trace over the other. */
export function reduced(s: State, q: 0 | 1): C[][] {
  const rho: C[][] = [
    [[0, 0], [0, 0]],
    [[0, 0], [0, 0]],
  ]
  for (let i = 0; i < 2; i++)
    for (let j = 0; j < 2; j++)
      for (let k = 0; k < 2; k++) {
        const a = q === 0 ? 2 * i + k : 2 * k + i
        const b = q === 0 ? 2 * j + k : 2 * k + j
        rho[i][j] = add(rho[i][j], mul(s[a], conj(s[b])))
      }
  return rho
}
/** Bloch vector of a (possibly mixed) one-qubit density matrix: (2 Re ρ₀₁, −2 Im ρ₀₁, ρ₀₀ − ρ₁₁). Length 1 if pure, 0 if maximally mixed. */
export const blochOf = (rho: C[][]): [number, number, number] => [2 * rho[0][1][0], -2 * rho[0][1][1], rho[0][0][0] - rho[1][1][0]]
/** Tr(ρ²): 1 for a pure state, ½ for a maximally mixed qubit. */
export const purity = (rho: C[][]) => abs2(rho[0][0]) + abs2(rho[1][1]) + 2 * abs2(rho[0][1])

/** ⟨Z⊗Z⟩: +1 means both qubits always give the same result in the 0/1 basis. */
export const zz = (s: State) => probabilities(s).reduce((t, p, i) => t + p * (((i >> 1) ^ i) & 1 ? -1 : 1), 0)

/** H on every qubit of |0…0⟩: 2^n basis states, each with amplitude 2^(−n/2). */
export function uniform(n: number): State {
  const plus = apply(H, zeros(1))
  let s = plus
  for (let k = 1; k < n; k++) s = kronState(s, plus)
  return s
}
/** |a⟩ ⊗ |b⟩. */
export const kronState = (a: State, b: State): State => a.flatMap((x) => b.map((y) => mul(x, y)))

/**
 * Resources to factor RSA-2048, as stated by C. Gidney, "How to factor 2048 bit
 * RSA integers with less than a million noisy qubits", arXiv:2505.15917 (2025):
 * under a million noisy qubits and under a week, assuming a 0.1 % gate error,
 * a 1 µs surface-code cycle and a 10 µs control reaction time.
 */
export const GIDNEY_2025 = { qubits: 1_000_000, days: 7, gateError: 0.001, cycleMicros: 1, reactionMicros: 10 } as const

// ---- Shor: N = 15, a = 7 ----

export const SHOR = { N: 15, a: 7, n: 8 } as const
/** 2^n points of the first register (n = 8 ≥ log₂ N², as Shor requires). */
export const Q = 2 ** SHOR.n

export function modpow(base: number, exp: number, mod: number): number {
  let r = 1
  let b = base % mod
  for (let e = exp; e > 0; e >>= 1) {
    if (e & 1) r = (r * b) % mod
    b = (b * b) % mod
  }
  return r
}
export const f = (x: number) => modpow(SHOR.a, x, SHOR.N)

/** The order of a mod N: the smallest r > 0 with a^r ≡ 1 (mod N). */
export function order(a: number, N: number): number {
  for (let r = 1; r <= N; r++) if (modpow(a, r, N) === 1) return r
  throw new Error('no order')
}

/**
 * The distribution of the first register after f is computed into the second
 * register, the second is measured (any value), and the QFT is applied to the
 * first: P(y) = Σ_v |(1/Q) Σ_{x: f(x)=v} e^{2πi xy/Q}|². Computed exactly.
 */
let qft: number[] | null = null
export function qftDistribution(): number[] {
  // O(Q²) terms and always the same: computed once (the panels ask on every render).
  if (qft) return qft
  const byValue = new Map<number, number[]>()
  for (let x = 0; x < Q; x++) {
    const v = f(x)
    if (!byValue.has(v)) byValue.set(v, [])
    byValue.get(v)!.push(x)
  }
  const out = new Array<number>(Q).fill(0)
  for (let y = 0; y < Q; y++)
    for (const xs of byValue.values()) {
      let re = 0
      let im = 0
      for (const x of xs) {
        const ang = (2 * Math.PI * x * y) / Q
        re += Math.cos(ang)
        im += Math.sin(ang)
      }
      out[y] += (re * re + im * im) / (Q * Q)
    }
  qft = out
  return out
}

/** Best rational approximation p/q of y/Q with q < N (continued fractions): the period candidate is the denominator. */
export function periodFrom(y: number, Q_: number, N: number): number {
  // Convergents of y/Q.
  let [h0, h1, k0, k1] = [0, 1, 1, 0]
  let [num, den] = [y, Q_]
  let best = 1
  while (den !== 0) {
    const t = Math.floor(num / den)
    ;[h0, h1] = [h1, t * h1 + h0]
    ;[k0, k1] = [k1, t * k1 + k0]
    if (k1 >= N) break
    best = k1
    ;[num, den] = [den, num - t * den]
  }
  return best
}

export const gcd = (a: number, b: number): number => (b === 0 ? Math.abs(a) : gcd(b, a % b))

/** From an even period r: gcd(a^{r/2} ± 1, N). */
export function factorsFrom(r: number): [number, number] {
  const h = modpow(SHOR.a, r / 2, SHOR.N)
  return [gcd(h - 1, SHOR.N), gcd(h + 1, SHOR.N)]
}
