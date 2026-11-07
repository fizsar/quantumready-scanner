// The label of each step of scenes 3–7, inside its panel: the formula in
// MathML and one line of numbers, every one of them computed here from
// quantum.ts, lattice.ts, mlkem.ts and figures.ts (nothing typed by hand but
// the sourced estimates).
import { memo, type ReactNode } from 'react'
import type { Lang } from '../../api'
import { curvePoints, CURVE_P, MEASURED } from './figures'
import { A, B_ERR, B_EXACT, BAD, babai, closest, det, dist, gauss, GOOD, LWE, TARGET, TOY_SPACE } from './lattice'
import { HYBRID, hex, ML_KEM, N as KEM_N, Q as KEM_Q, sizes, spaceDigits, total, X25519 } from './mlkem'
import type { Step } from './model'
import { apply, bell, bloch, blochOf, collapse, f, factorsFrom, GIDNEY_2025, H, order, periodFrom, probabilities, purity, Q, qftDistribution, reduced, SHOR, uniform, zeros, zz } from './quantum'

const fmt = (lang: Lang, v: number, digits = 3) => new Intl.NumberFormat(lang, { maximumFractionDigits: digits }).format(Math.abs(v) < 1e-12 ? 0 : v).replace('-', '−')
const vec = (lang: Lang, v: readonly number[]) => `(${v.map((x) => fmt(lang, x)).join('; ')})`

const ket = (s: string) => (
  <mrow>
    <mo stretchy="false">|</mo>
    <mn>{s}</mn>
    <mo stretchy="false">⟩</mo>
  </mrow>
)
const frac = (a: ReactNode, b: ReactNode) => (
  <mfrac>
    <mrow>{a}</mrow>
    <mrow>{b}</mrow>
  </mfrac>
)

/** |ψ⟩ = cos(θ/2)|0⟩ + e^{iφ} sin(θ/2)|1⟩. */
const QubitFormula = () => (
  <math display="block" aria-label="|ψ⟩ = cos(θ/2)|0⟩ + e^(iφ) sin(θ/2)|1⟩">
    <mo stretchy="false">|</mo>
    <mi>ψ</mi>
    <mo stretchy="false">⟩</mo>
    <mo>=</mo>
    <mi>cos</mi>
    <mo>⁡</mo>
    <mo stretchy="false">(</mo>
    <mi>θ</mi>
    <mo>/</mo>
    <mn>2</mn>
    <mo stretchy="false">)</mo>
    {ket('0')}
    <mo>+</mo>
    <msup>
      <mi>e</mi>
      <mrow>
        <mi>i</mi>
        <mi>φ</mi>
      </mrow>
    </msup>
    <mi>sin</mi>
    <mo>⁡</mo>
    <mo stretchy="false">(</mo>
    <mi>θ</mi>
    <mo>/</mo>
    <mn>2</mn>
    <mo stretchy="false">)</mo>
    {ket('1')}
  </math>
)

/** (|00⟩ + |11⟩)/√2, optionally preceded by what produces it. */
const BellFormula = ({ lead }: { lead?: ReactNode }) => (
  <math display="block" aria-label="(|00⟩ + |11⟩)/√2">
    {lead}
    {frac(
      <>
        {ket('00')}
        <mo>+</mo>
        {ket('11')}
      </>,
      <msqrt>
        <mn>2</mn>
      </msqrt>,
    )}
  </math>
)

/** Memoized: its props never change, and the page re-renders every panel when the step changes. */
export const FigureCaption = memo(function FigureCaption({ step, lang }: { step: Step; lang: Lang }) {
  if (step.shape.kind !== 'figure') return null
  const es = lang === 'es'
  const n = (v: number, d = 3) => fmt(lang, v, d)
  const id = step.shape.id
  let formula: ReactNode = null
  let line = ''
  switch (id) {
    case 'bloch0':
      formula = <QubitFormula />
      line = `θ = 0: |0⟩, vector ${vec(lang, bloch(zeros(1)))}`
      break
    case 'blochH': {
      const p = probabilities(apply(H, zeros(1)))
      formula = <QubitFormula />
      line = `θ = π/2, φ = 0: P(0) = ${n(p[0])} · P(1) = ${n(p[1])}`
      break
    }
    case 'blochM':
      formula = <QubitFormula />
      line = `${es ? 'resultado' : 'outcome'} ${MEASURED}: |${MEASURED}⟩, vector ${vec(lang, bloch(collapse(apply(H, zeros(1)), MEASURED)))}`
      break
    case 'bell': {
      const rho = reduced(bell(), 0)
      formula = <BellFormula />
      line = `${es ? 'cada cúbit' : 'each qubit'}: vector ${vec(lang, blochOf(rho))}, ${es ? 'pureza' : 'purity'} ${n(purity(rho))} · ⟨Z⊗Z⟩ = ${n(zz(bell()))}`
      break
    }
    case 'circuit': {
      const p = probabilities(bell())
      formula = (
        <BellFormula
          lead={
            <>
              <mi>CNOT</mi>
              <mo stretchy="false">(</mo>
              <mi>H</mi>
              <mo>⊗</mo>
              <mi>I</mi>
              <mo stretchy="false">)</mo>
              {ket('00')}
              <mo>=</mo>
            </>
          }
        />
      )
      line = `P(00) = ${n(p[0])} · P(01) = ${n(p[1])} · P(10) = ${n(p[2])} · P(11) = ${n(p[3])}`
      break
    }
    case 'reg3':
    case 'reg6':
    case 'reg10': {
      const q = Number(id.slice(3))
      const s = uniform(q)
      formula = (
        <math display="block" aria-label={`H^⊗${q}|0…0⟩ = 2^(−${q}/2) Σ|x⟩`}>
          <msup>
            <mi>H</mi>
            <mrow>
              <mo>⊗</mo>
              <mn>{q}</mn>
            </mrow>
          </msup>
          {ket('0…0')}
          <mo>=</mo>
          <msup>
            <mn>2</mn>
            <mrow>
              <mo>−</mo>
              <mn>{q}</mn>
              <mo>/</mo>
              <mn>2</mn>
            </mrow>
          </msup>
          <munder>
            <mo>∑</mo>
            <mi>x</mi>
          </munder>
          <mo stretchy="false">|</mo>
          <mi>x</mi>
          <mo stretchy="false">⟩</mo>
        </math>
      )
      line = `2${sup(q)} = ${s.length} ${es ? 'amplitudes de' : 'amplitudes of'} ${n(s[0][0])}`
      break
    }
    case 'shorF': {
      const vals = Array.from({ length: 6 }, (_, x) => f(x))
      formula = (
        <math display="block" aria-label={`f(x) = ${SHOR.a}^x mod ${SHOR.N}`}>
          <mi>f</mi>
          <mo stretchy="false">(</mo>
          <mi>x</mi>
          <mo stretchy="false">)</mo>
          <mo>=</mo>
          <msup>
            <mn>{SHOR.a}</mn>
            <mi>x</mi>
          </msup>
          <mo lspace="0.3em">mod</mo>
          <mn>{SHOR.N}</mn>
        </math>
      )
      line = `f = ${vals.join(', ')}, … → r = ${order(SHOR.a, SHOR.N)}`
      break
    }
    case 'shorQFT': {
      const p = qftDistribution()
      const peaks = p.flatMap((v, y) => (v > 1e-9 ? [y] : []))
      const y = peaks[1]
      formula = (
        <math display="block" aria-label={`y/2^${SHOR.n} = ${y}/${Q} → r = ${periodFrom(y, Q, SHOR.N)}`}>
          <mfrac>
            <mi>y</mi>
            <msup>
              <mn>2</mn>
              <mn>{SHOR.n}</mn>
            </msup>
          </mfrac>
          <mo>=</mo>
          <mfrac>
            <mn>{y}</mn>
            <mn>{Q}</mn>
          </mfrac>
          <mo>=</mo>
          <mfrac>
            <mn>1</mn>
            <mn>{periodFrom(y, Q, SHOR.N)}</mn>
          </mfrac>
        </math>
      )
      line = `${es ? 'picos en' : 'peaks at'} y = ${peaks.join(', ')}, P = ${n(p[y])} · r = ${periodFrom(y, Q, SHOR.N)}`
      break
    }
    case 'key':
      formula = (
        <math display="block" aria-label={`N = p · q = ${SHOR.N}`}>
          <mi>N</mi>
          <mo>=</mo>
          <mi>p</mi>
          <mo>·</mo>
          <mi>q</mi>
          <mo>=</mo>
          <mn>{SHOR.N}</mn>
        </math>
      )
      line = es ? `${SHOR.N} cubos: un solo número` : `${SHOR.N} cubes: one number`
      break
    case 'factors': {
      const r = order(SHOR.a, SHOR.N)
      const h = SHOR.a ** (r / 2)
      const [p, q] = factorsFrom(r)
      formula = (
        <math display="block" aria-label={`gcd(${SHOR.a}^${r / 2} ± 1, ${SHOR.N}) = ${p}, ${q}`}>
          <mi>{es ? 'mcd' : 'gcd'}</mi>
          <mo stretchy="false">(</mo>
          <msup>
            <mn>{SHOR.a}</mn>
            <mn>{r / 2}</mn>
          </msup>
          <mo>±</mo>
          <mn>1</mn>
          <mo>,</mo>
          <mn>{SHOR.N}</mn>
          <mo stretchy="false">)</mo>
        </math>
      )
      line = `${es ? 'mcd' : 'gcd'}(${h - 1}, ${SHOR.N}) = ${p} · ${es ? 'mcd' : 'gcd'}(${h + 1}, ${SHOR.N}) = ${q} · ${p} × ${q} = ${p * q}`
      break
    }
    case 'curve':
    case 'curveBroken': {
      formula = (
        <math display="block" aria-label="y² = x³ + 486662x² + x">
          <msup>
            <mi>y</mi>
            <mn>2</mn>
          </msup>
          <mo>=</mo>
          <msup>
            <mi>x</mi>
            <mn>3</mn>
          </msup>
          <mo>+</mo>
          <mn>486662</mn>
          <msup>
            <mi>x</mi>
            <mn>2</mn>
          </msup>
          <mo>+</mo>
          <mi>x</mi>
        </math>
      )
      if (id === 'curve') line = es ? `sobre 𝔽${sub(CURVE_P)}: ${curvePoints().length + 1} puntos (con el del infinito)` : `over 𝔽${sub(CURVE_P)}: ${curvePoints().length + 1} points (with the one at infinity)`
      else {
        const g = GIDNEY_2025
        line = es
          ? `RSA-2048: < ${n(g.qubits, 0)} cúbits ruidosos, < ${g.days} días, con ${n(g.gateError * 100, 1)} % de error por puerta, ciclo de ${g.cycleMicros} µs y ${g.reactionMicros} µs de reacción`
          : `RSA-2048: < ${n(g.qubits, 0)} noisy qubits, < ${g.days} days, at ${n(g.gateError * 100, 1)}% gate error, ${g.cycleMicros} µs cycle and ${g.reactionMicros} µs reaction`
      }
      break
    }
    case 'harvest':
      formula = <Formula label="c = Enc(k, m)">{enc('c', 'Enc', 'k', 'm')}</Formula>
      line = es ? 'se copia c; sin la clave k no se puede leer' : 'c is copied; without the key k it cannot be read'
      break
    case 'stored':
      formula = <Formula label="k ← X25519">{assign('k', 'X25519')}</Formula>
      line = es ? 'k se acordó con X25519 (o RSA): Shor la recupera (escena 5)' : 'k was agreed with X25519 (or RSA): Shor recovers it (scene 5)'
      break
    case 'decrypted':
      formula = <Formula label="m = Dec(k, c)">{enc('m', 'Dec', 'k', 'c')}</Formula>
      line = es ? 'con k, m se lee años después de enviarse' : 'with k, m is read years after it was sent'
      break
    case 'mosca':
      formula = (
        <Formula label="x + y > z">
          <mi>x</mi>
          <mo>+</mo>
          <mi>y</mi>
          <mo>&gt;</mo>
          <mi>z</mi>
          <mo>⇒</mo>
          <mtext>{es ? 'riesgo' : 'risk'}</mtext>
        </Formula>
      )
      line = es ? 'z no es una fecha: es incierto (encuesta del GRI, 2025)' : 'z is not a date: it is uncertain (GRI survey, 2025)'
      break
    case 'lattice': {
      const c = closest(GOOD, TARGET)
      formula = (
        <Formula label="L = {a b1 + c b2 : a, c integers}">
          <mi>L</mi>
          <mo>=</mo>
          <mo>{'{'}</mo>
          <mi>a</mi>
          <msub>
            <mi>b</mi>
            <mn>1</mn>
          </msub>
          <mo>+</mo>
          <mi>c</mi>
          <msub>
            <mi>b</mi>
            <mn>2</mn>
          </msub>
          <mo>:</mo>
          <mi>a</mi>
          <mo>,</mo>
          <mi>c</mi>
          <mo>∈</mo>
          <mi mathvariant="normal">ℤ</mi>
          <mo>{'}'}</mo>
        </Formula>
      )
      line = es
        ? `b₁ = ${vec(lang, GOOD[0])}, b₂ = ${vec(lang, GOOD[1])}, det = ${det(GOOD)} · t = ${vec(lang, TARGET)} → más cercano ${vec(lang, c)}, a ${n(dist(c, TARGET), 2)}`
        : `b₁ = ${vec(lang, GOOD[0])}, b₂ = ${vec(lang, GOOD[1])}, det = ${det(GOOD)} · t = ${vec(lang, TARGET)} → closest ${vec(lang, c)}, at ${n(dist(c, TARGET), 2)}`
      break
    }
    case 'latticeBad': {
      const c = closest(GOOD, TARGET)
      const w = babai(BAD, TARGET)
      formula = (
        <Formula label="b1' = 5 b1 + 2 b2, b2' = 2 b1 + b2">
          <msubsup>
            <mi>b</mi>
            <mn>1</mn>
            <mo>′</mo>
          </msubsup>
          <mo>=</mo>
          <mn>5</mn>
          <msub>
            <mi>b</mi>
            <mn>1</mn>
          </msub>
          <mo>+</mo>
          <mn>2</mn>
          <msub>
            <mi>b</mi>
            <mn>2</mn>
          </msub>
          <mo>,</mo>
          <msubsup>
            <mi>b</mi>
            <mn>2</mn>
            <mo>′</mo>
          </msubsup>
          <mo>=</mo>
          <mn>2</mn>
          <msub>
            <mi>b</mi>
            <mn>1</mn>
          </msub>
          <mo>+</mo>
          <msub>
            <mi>b</mi>
            <mn>2</mn>
          </msub>
        </Formula>
      )
      line = es
        ? `${vec(lang, BAD[0])}, ${vec(lang, BAD[1])}, det = ${det(BAD)}: la misma red · redondeo → ${vec(lang, w)}, a ${n(dist(w, TARGET), 2)} (${Math.floor(dist(w, TARGET) / dist(c, TARGET))} veces más lejos)`
        : `${vec(lang, BAD[0])}, ${vec(lang, BAD[1])}, det = ${det(BAD)}: the same lattice · rounding → ${vec(lang, w)}, at ${n(dist(w, TARGET), 2)} (${Math.floor(dist(w, TARGET) / dist(c, TARGET))} times farther)`
      break
    }
    case 'lwe': {
      const exact = gauss(A, B_EXACT, LWE.q)
      const wrong = gauss(A, B_ERR, LWE.q)
      formula = (
        <Formula label="b = A s + e (mod q)">
          <mi mathvariant="bold">b</mi>
          <mo>=</mo>
          <mi mathvariant="bold">A</mi>
          <mi mathvariant="bold">s</mi>
          <mo>+</mo>
          <mi mathvariant="bold">e</mi>
          <mspace width="0.4em" />
          <mo stretchy="false">(</mo>
          <mi>mod</mi>
          <mspace width="0.2em" />
          <mn>{LWE.q}</mn>
          <mo stretchy="false">)</mo>
        </Formula>
      )
      line = es
        ? `n = ${LWE.n}, m = ${LWE.m} · sin e, Gauss da ${vec(lang, exact)} = s · con e, ${vec(lang, wrong)} · a la fuerza, ${n(TOY_SPACE, 0)} candidatos`
        : `n = ${LWE.n}, m = ${LWE.m} · without e, Gauss gives ${vec(lang, exact)} = s · with e, ${vec(lang, wrong)} · by brute force, ${n(TOY_SPACE, 0)} candidates`
      break
    }
    case 'mlkem': {
      const p = ML_KEM['768']
      const z = sizes(p)
      formula = (
        <Formula label={`n = ${KEM_N}, k = ${p.k}, q = ${KEM_Q}`}>
          <mi>n</mi>
          <mo>=</mo>
          <mn>{KEM_N}</mn>
          <mo>,</mo>
          <mi>k</mi>
          <mo>=</mo>
          <mn>{p.k}</mn>
          <mo>,</mo>
          <mi>q</mi>
          <mo>=</mo>
          <mn>{KEM_Q}</mn>
        </Formula>
      )
      line = es
        ? `clave pública ${n(z.ek, 0)} B, cifrado ${n(z.ct, 0)} B · X25519: ${X25519.pub} B · secretos posibles: un número de ${spaceDigits(p)} cifras`
        : `public key ${n(z.ek, 0)} B, ciphertext ${n(z.ct, 0)} B · X25519: ${X25519.pub} B · possible secrets: a ${spaceDigits(p)}-digit number`
      break
    }
    case 'hybrid': {
      const [ek, xc] = HYBRID.client
      const [ct, xs] = HYBRID.server
      formula = (
        <Formula label="key_share = ek ‖ X25519">
          <mtext>key_share</mtext>
          <mo>=</mo>
          <mtext>ek</mtext>
          <mo>‖</mo>
          <mtext>X25519</mtext>
        </Formula>
      )
      line = es
        ? `cliente ${ek.bytes} + ${xc.bytes} = ${total(HYBRID.client)} B · servidor ${ct.bytes} + ${xs.bytes} = ${total(HYBRID.server)} B · secreto ${total(HYBRID.secret)} B · grupo ${HYBRID.codepoint} (${hex(HYBRID.codepoint)})`
        : `client ${ek.bytes} + ${xc.bytes} = ${total(HYBRID.client)} B · server ${ct.bytes} + ${xs.bytes} = ${total(HYBRID.server)} B · secret ${total(HYBRID.secret)} B · group ${HYBRID.codepoint} (${hex(HYBRID.codepoint)})`
      break
    }
  }
  return (
    <div className="story-cap story-eq">
      {formula}
      <p className="story-scale">{line}</p>
    </div>
  )
})

/** A display formula with its spoken label. */
const Formula = ({ label, children }: { label: string; children: ReactNode }) => (
  <math display="block" aria-label={label}>
    {children}
  </math>
)
/** out = Fn(key, in). */
const enc = (out: string, fn: string, key: string, input: string) => (
  <>
    <mi>{out}</mi>
    <mo>=</mo>
    <mi mathvariant="normal">{fn}</mi>
    <mo stretchy="false">(</mo>
    <mi>{key}</mi>
    <mo>,</mo>
    <mi>{input}</mi>
    <mo stretchy="false">)</mo>
  </>
)
const assign = (v: string, from: string) => (
  <>
    <mi>{v}</mi>
    <mo>←</mo>
    <mtext>{from}</mtext>
  </>
)

const SUP = '⁰¹²³⁴⁵⁶⁷⁸⁹'
const SUB = '₀₁₂₃₄₅₆₇₈₉'
const sup = (v: number) => [...String(v)].map((d) => SUP[Number(d)]).join('')
const sub = (v: number) => [...String(v)].map((d) => SUB[Number(d)]).join('')
