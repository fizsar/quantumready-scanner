// The label of each step of scenes 3–6, inside its panel: the formula in
// MathML and one line of numbers, every one of them computed here from
// quantum.ts and figures.ts (nothing typed by hand but the sourced estimates).
import { memo, type ReactNode } from 'react'
import type { Lang } from '../../api'
import { curvePoints, CURVE_P, MEASURED } from './figures'
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
