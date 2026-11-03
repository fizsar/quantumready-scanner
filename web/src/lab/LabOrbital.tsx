import { useEffect, useRef, useState } from 'react'
import type { Lang } from '../api'
import { useLang } from '../i18n'
import { usePageMeta } from '../usePageMeta'
import { chooseMode, type Mode } from './capability'
import { energy, ORBITALS, r95 as radius95, type Orbital } from './hydrogen'
import { loadScene, stillURL } from './load'
import type { OrbitalScene } from './orbitalScene'
import { SEED } from './view'
import './LabOrbital.css'

const R95 = ORBITALS.map(radius95)

const text: Record<Lang, {
  title: string
  lead: string
  legend: [string, string, string]
  phase: string
  scale: (r: string) => string
  orbitals: { body: string }[]
  energy: string
  sourcesTitle: string
  stillNote: Record<Exclude<Mode, { kind: '3d' }>['reason'], string>
  alt: (name: string) => string
}> = {
  es: {
    title: 'Orbitales del hidrógeno · laboratorio',
    lead: 'Nubes de puntos muestreadas de |ψ|², la densidad de probabilidad del electrón, con las funciones de onda exactas del átomo de hidrógeno. Baja para pasar de 1s a 2p, 3d y 4f.',
    legend: ['verde: ψ > 0', 'índigo: ψ < 0', 'el signo global es una convención: lo físico es dónde cambia'],
    phase: 'Fase de ψ',
    scale: (r) => `escala: el 95 % de la probabilidad está dentro de r = ${r} a₀`,
    orbitals: [
      { body: 'El estado fundamental. Con l = 0 no hay nodos angulares: la nube es esférica y de una sola fase. La densidad es máxima en el núcleo, pero la distancia más probable es un radio de Bohr, a₀ ≈ 52,9 pm.' },
      { body: 'Con l = 1 aparece un plano nodal, z = 0, donde ψ vale cero. Separa dos lóbulos de fase opuesta.' },
      { body: 'Con l = 2 hay dos superficies nodales: los conos cos²θ = 1/3. Separan los lóbulos del eje z del anillo del plano xy, que tiene la fase contraria.' },
      { body: 'Con l = 3 hay tres planos nodales, x = 0, y = 0 y z = 0: ψ es proporcional a xyz. Ocho lóbulos, uno por octante, que alternan de fase.' },
    ],
    energy: 'energía',
    sourcesTitle: 'Fuentes y método',
    stillNote: {
      'reduced-motion': 'Imágenes fijas: tu sistema pide reducir el movimiento.',
      'no-webgl2': 'Imágenes fijas: este navegador no ofrece WebGL2 acelerado.',
      'save-data': 'Imágenes fijas: el modo de ahorro de datos está activado.',
      'low-end': 'Imágenes fijas para este dispositivo.',
    },
    alt: (name) => `Nube de puntos del orbital ${name} del hidrógeno`,
  },
  en: {
    title: 'Hydrogen orbitals · lab',
    lead: 'Point clouds sampled from |ψ|², the electron’s probability density, using the exact wave functions of the hydrogen atom. Scroll to go from 1s to 2p, 3d and 4f.',
    legend: ['green: ψ > 0', 'indigo: ψ < 0', 'the overall sign is a convention: what is physical is where it changes'],
    phase: 'Phase of ψ',
    scale: (r) => `scale: 95% of the probability lies within r = ${r} a₀`,
    orbitals: [
      { body: 'The ground state. With l = 0 there are no angular nodes: the cloud is spherical and has a single phase. The density peaks at the nucleus, but the most likely distance is one Bohr radius, a₀ ≈ 52.9 pm.' },
      { body: 'With l = 1 a nodal plane appears, z = 0, where ψ is zero. It separates two lobes of opposite phase.' },
      { body: 'With l = 2 there are two nodal surfaces: the cones cos²θ = 1/3. They separate the lobes along z from the ring in the xy plane, which has the opposite phase.' },
      { body: 'With l = 3 there are three nodal planes, x = 0, y = 0 and z = 0: ψ is proportional to xyz. Eight lobes, one per octant, alternating in phase.' },
    ],
    energy: 'energy',
    sourcesTitle: 'Sources and method',
    stillNote: {
      'reduced-motion': 'Still images: your system asks for reduced motion.',
      'no-webgl2': 'Still images: this browser has no accelerated WebGL2.',
      'save-data': 'Still images: data saver is on.',
      'low-end': 'Still images for this device.',
    },
    alt: (name) => `Point cloud of the hydrogen ${name} orbital`,
  },
}

/** /lab/orbital: hidden prototype of the first scene of the redesign (noindex, not linked). */
export default function LabOrbital() {
  const { lang } = useLang()
  const tx = text[lang]
  usePageMeta(`${tx.title} · QuantumReady`, true)
  const [chosen] = useState<Mode>(() => chooseMode())
  // A 3D scene that fails (shader, lost WebGL) falls back to the still images.
  const [failed, setFailed] = useState(false)
  const mode: Mode = failed ? { kind: 'still', reason: 'no-webgl2' } : chosen
  const [index, setIndex] = useState(0)
  const [ready, setReady] = useState(false)
  const [active, setActive] = useState(false)
  const track = useRef<HTMLDivElement>(null)
  const canvas = useRef<HTMLCanvasElement>(null)
  const scene = useRef<OrbitalScene | null>(null)
  const visible = useRef(false)

  // Scroll → progress (0 … 3) through the track; the stage shows only while the track is on screen.
  useEffect(() => {
    let raf = 0
    const update = () => {
      raf = 0
      const el = track.current
      if (!el) return
      const r = el.getBoundingClientRect()
      const span = Math.max(1, r.height - window.innerHeight)
      // +0.35: each orbital morphs into the next while the next text panel comes in.
      const p = Math.min(ORBITALS.length - 1, Math.max(0, (-r.top / span) * (ORBITALS.length - 1) + 0.35))
      scene.current?.setProgress(p)
      const i = Math.floor(p)
      setIndex(Math.min(ORBITALS.length - 1, p - i > 0.75 ? i + 1 : i))
    }
    const onScroll = () => {
      if (!raf) raf = requestAnimationFrame(update)
    }
    update()
    window.addEventListener('scroll', onScroll, { passive: true })
    window.addEventListener('resize', onScroll)
    const show = (on: boolean) => {
      visible.current = on
      setActive(on)
      if (on) scene.current?.start()
      else scene.current?.stop()
    }
    // Without IntersectionObserver (old browsers, jsdom): always on.
    const io = typeof IntersectionObserver === 'undefined' ? null : new IntersectionObserver(([e]) => show(e.isIntersecting))
    if (io && track.current) io.observe(track.current)
    else show(true)
    return () => {
      window.removeEventListener('scroll', onScroll)
      window.removeEventListener('resize', onScroll)
      io?.disconnect()
      cancelAnimationFrame(raf)
    }
  }, [])

  // 3D only: load three.js once the page has painted and the main thread is idle.
  useEffect(() => {
    if (chosen.kind !== '3d') return
    let cancelled = false
    const idle = window.requestIdleCallback ?? ((cb: () => void) => window.setTimeout(cb, 200))
    const handle = idle(() => {
      loadScene().then(({ createOrbitalScene }) => {
        if (cancelled || !canvas.current) return
        scene.current = createOrbitalScene(canvas.current, {
          points: chosen.points,
          seed: SEED,
          onReady: () => setReady(true),
          onError: () => {
            const s = scene.current
            scene.current = null
            setTimeout(() => s?.dispose()) // not from inside its own render
            setFailed(true)
          },
        })
        if (visible.current) scene.current.start()
      })
    })
    return () => {
      cancelled = true
      if (typeof handle === 'number') window.cancelIdleCallback?.(handle)
      scene.current?.dispose()
      scene.current = null
    }
  }, [chosen])

  const o = ORBITALS[index]
  const fmt = new Intl.NumberFormat(lang, { maximumFractionDigits: 2, minimumFractionDigits: 2 })
  const r95 = new Intl.NumberFormat(lang, { maximumFractionDigits: 1 }).format(R95[index])

  return (
    <article className="lab" aria-labelledby="lab-title" data-mode={mode.kind}>
      <header className="lab-intro">
        <p className="prompt">
          <span className="user">guest@quantumready</span>:<span className="path">~/lab</span>$ ./orbital
        </p>
        <h1 id="lab-title" className="h-title">
          {tx.title}
        </h1>
        <p className="muted">{tx.lead}</p>
        <p className="lab-legend" aria-label={tx.phase}>
          <span className="lab-pos">● {tx.legend[0]}</span> <span className="lab-neg">● {tx.legend[1]}</span>{' '}
          <span className="muted">({tx.legend[2]})</span>
        </p>
        {mode.kind === 'still' && <p className="muted lab-note">{tx.stillNote[mode.reason]}</p>}
      </header>

      <div className="lab-stage" data-active={active} aria-hidden="true">
        <img className="lab-still" src={stillURL(mode.kind === 'still' ? o.id : ORBITALS[0].id)} alt="" data-hidden={mode.kind === '3d' && ready} />
        {mode.kind === '3d' && <canvas ref={canvas} className="lab-canvas" data-ready={ready} />}
      </div>

      <div className="lab-caption" data-active={active}>
        <p className="lab-qn">
          <math>
            <msub>
              <mi>ψ</mi>
              <mrow>
                <mn>{o.n}</mn>
                <mo>,</mo>
                <mn>{o.l}</mn>
                <mo>,</mo>
                <mn>{num(o.m)}</mn>
              </mrow>
            </msub>
          </math>{' '}
          <span>
            <Name o={o} /> · (n, l, m) = ({o.n}, {o.l}, {num(o.m)}) · {tx.energy} E<sub>{o.n}</sub> = {fmt.format(energy(o.n)).replace('-', '−')} eV
          </span>
        </p>
        <p className="lab-scale muted">{tx.scale(r95)}</p>
        <div className="lab-eq">
          <Schrodinger />
        </div>
      </div>

      <div className="lab-track" ref={track}>
        {tx.orbitals.map((s, i) => (
          <section key={ORBITALS[i].id} className="lab-step" aria-labelledby={`lab-step-${i}`}>
            <div className="lab-panel">
              <h2 id={`lab-step-${i}`}>
                <Name o={ORBITALS[i]} /> · (n, l, m) = ({ORBITALS[i].n}, {ORBITALS[i].l}, {num(ORBITALS[i].m)})
              </h2>
              <p>{s.body}</p>
              {mode.kind === 'still' && <img className="lab-step-img" src={stillURL(ORBITALS[i].id)} alt={tx.alt(ORBITALS[i].name + ORBITALS[i].sub)} loading="lazy" width={640} height={640} />}
            </div>
          </section>
        ))}
      </div>

      <Sources lang={lang} title={tx.sourcesTitle} />
    </article>
  )
}

/** A typographic minus for negative numbers. */
const num = (v: number) => String(v).replace('-', '−')

/** 2p_z with a real subscript. */
function Name({ o }: { o: Orbital }) {
  return (
    <>
      {o.name}
      {o.sub && <sub>{o.sub}</sub>}
    </>
  )
}

/** Time-independent Schrödinger equation of hydrogen (fixed nucleus), MathML Core. */
function Schrodinger() {
  return (
    <math display="block" aria-label="Schrödinger">
      <mrow>
        <mo>−</mo>
        <mfrac>
          <msup>
            <mi>ħ</mi>
            <mn>2</mn>
          </msup>
          <mrow>
            <mn>2</mn>
            <msub>
              <mi>m</mi>
              <mi mathvariant="normal">e</mi>
            </msub>
          </mrow>
        </mfrac>
        <msup>
          <mo>∇</mo>
          <mn>2</mn>
        </msup>
        <mi>ψ</mi>
        <mo>−</mo>
        <mfrac>
          <msup>
            <mi>e</mi>
            <mn>2</mn>
          </msup>
          <mrow>
            <mn>4</mn>
            <mi>π</mi>
            <msub>
              <mi>ε</mi>
              <mn>0</mn>
            </msub>
            <mi>r</mi>
          </mrow>
        </mfrac>
        <mi>ψ</mi>
        <mo>=</mo>
        <msub>
          <mi>E</mi>
          <mi>n</mi>
        </msub>
        <mi>ψ</mi>
        <mo>,</mo>
        <mspace width="1em" />
        <msub>
          <mi>E</mi>
          <mi>n</mi>
        </msub>
        <mo>=</mo>
        <mo>−</mo>
        <mfrac>
          <mrow>
            <msub>
              <mi>R</mi>
              <mi>∞</mi>
            </msub>
            <mi>h</mi>
            <mi>c</mi>
          </mrow>
          <msup>
            <mi>n</mi>
            <mn>2</mn>
          </msup>
        </mfrac>
      </mrow>
    </math>
  )
}

function Sources({ lang, title }: { lang: Lang; title: string }) {
  const es = lang === 'es'
  return (
    <section className="lab-sources" aria-labelledby="lab-sources-title">
      <h2 id="lab-sources-title">{title}</h2>
      <p>
        {es
          ? 'Cada punto se muestrea de |ψ_nlm|² = |R_nl(r)|² |Y_lm(θ, φ)|² (unidades atómicas, a₀ = 1, núcleo fijo): el radio invirtiendo la distribución acumulada de r²R_nl², la dirección por rechazo contra |Y_lm|². Las imágenes fijas se generan al compilar con el mismo código y la misma semilla. Los tests comparan ψ con las formas cerradas tabuladas, comprueban que ∫|ψ|² d³r = 1 y que el radio medio de la nube es ⟨r⟩ = a₀[3n² − l(l+1)]/2.'
          : 'Each point is sampled from |ψ_nlm|² = |R_nl(r)|² |Y_lm(θ, φ)|² (atomic units, a₀ = 1, fixed nucleus): the radius by inverting the cumulative distribution of r²R_nl², the direction by rejection against |Y_lm|². The still images are generated at build time with the same code and seed. The tests compare ψ with the tabulated closed forms and check that ∫|ψ|² d³r = 1 and that the cloud’s mean radius is ⟨r⟩ = a₀[3n² − l(l+1)]/2.'}
      </p>
      <ul>
        <li>
          D. J. Griffiths, D. F. Schroeter, <cite>Introduction to Quantum Mechanics</cite>, {es ? '3.ª ed.' : '3rd ed.'}, Cambridge University Press, 2018, §4.2 ({es ? 'el átomo de hidrógeno: ecuación radial, funciones de onda y energías' : 'the hydrogen atom: radial equation, wave functions and energies'}).
        </li>
        <li>
          NIST <cite>Digital Library of Mathematical Functions</cite>:{' '}
          <a href="https://dlmf.nist.gov/18.9" rel="noopener">§18.9</a> ({es ? 'recurrencia de los polinomios de Laguerre asociados' : 'recurrence of the associated Laguerre polynomials'}),{' '}
          <a href="https://dlmf.nist.gov/14.10" rel="noopener">§14.10</a> ({es ? 'recurrencias de las funciones de Legendre asociadas; aquí sin la fase de Condon–Shortley' : 'recurrences of the associated Legendre functions; here without the Condon–Shortley phase'}),{' '}
          <a href="https://dlmf.nist.gov/14.30" rel="noopener">§14.30</a> ({es ? 'armónicos esféricos; los reales son combinaciones de Y_l^m e Y_l^−m' : 'spherical harmonics; the real ones are combinations of Y_l^m and Y_l^−m'}).
        </li>
        <li>
          CODATA 2022 (NIST), {es ? 'energía de Rydberg' : 'Rydberg energy'} R∞hc = {es ? '13,605 693 122 990' : '13.605 693 122 990'} eV:{' '}
          <a href="https://physics.nist.gov/cgi-bin/cuu/Value?rydhcev" rel="noopener">physics.nist.gov</a>.
        </li>
        <li>
          H. A. Bethe, E. E. Salpeter, <cite>Quantum Mechanics of One- and Two-Electron Atoms</cite>, Springer, 1957, §3 ({es ? 'valores esperados de r, usados en los tests' : 'expectation values of r, used in the tests'}).
        </li>
      </ul>
    </section>
  )
}
