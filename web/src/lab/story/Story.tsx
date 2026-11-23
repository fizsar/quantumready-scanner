import { memo, useEffect, useRef, useState, type ReactNode } from 'react'
import type { Lang } from '../../api'
import { useLang } from '../../i18n'
import { usePageMeta } from '../../usePageMeta'
import { chooseMode, type Mode } from '../capability'
import { energy, r95 } from '../hydrogen'
import { SEED } from '../view'
import type { Story as Engine } from './engine'
import { FigureCaption } from './captions'
import { MOSCA_DEFAULT } from './figures'
import { loadStory, storyStillURL } from './load'
import { EDGE, orbitalOf, panelsShown, progressAt, stageMask, stageView, STEPS, stepAt, termState, type Step, type Term } from './model'
import { MoscaWidget, type MoscaValue } from './MoscaWidget'
import { TEXT } from './texts'
import { typeset } from './typeset'
import './Story.css'

// The step texts, their captions and the sources depend only on the step and the language:
// memoised, so changing step (every few hundred pixels of scroll) re-renders none of the 34
// panels (twice: the track and the phone sheet) nor their formulas. It used to cost a 50–90 ms
// stall at every step change.
const MemoStepText = memo(StepText)
const MemoCaption = memo(Caption)
const MemoSourcesBody = memo(function SourcesBody({ lang }: { lang: Lang }) {
  const tx = TEXT[lang]
  return (
    <>
      <h2 id="story-sources-title">{tx.sources}</h2>
      <p>{typeset(tx.method)}</p>
      <p>{typeset(tx.units)}</p>
      <p>{typeset(tx.methodQuantum)}</p>
      <p>{typeset(tx.methodPq)}</p>
      <Sources lang={lang} />
    </>
  )
})

/** /historia: the story, from quantum mechanics to your website (linked from the home). */
export default function Story() {
  const { lang } = useLang()
  const tx = TEXT[lang]
  usePageMeta(`${tx.title} · QuantumReady`, false, tx.lead)
  const [chosen] = useState<Mode>(() => chooseMode())
  const [failed, setFailed] = useState(false)
  const mode: Mode = failed ? { kind: 'still', reason: 'no-webgl2' } : chosen
  const [index, setIndex] = useState(0)
  const [ready, setReady] = useState(false)
  // Scene 6: the inequality's sliders (one state for the panel's and the sheet's widget).
  const [mosca, setMosca] = useState<MoscaValue>({ ...MOSCA_DEFAULT })
  const moscaNow = useRef<MoscaValue>(mosca)
  moscaNow.current = mosca
  useEffect(() => {
    engine.current?.setMosca(mosca.x, mosca.y)
  }, [mosca])
  const phone = usePhone()
  const track = useRef<HTMLDivElement>(null)
  const article = useRef<HTMLElement>(null)
  const sheet = useRef<HTMLDivElement>(null)
  const stage = useRef<HTMLDivElement>(null)
  const canvas = useRef<HTMLCanvasElement>(null)
  const engine = useRef<Engine | null>(null)
  const visible = useRef(false)
  // The last progress and free area, for an engine that arrives after them
  // (three.js loads after the first paint; at the top of the page no scroll follows).
  const latest = useRef<{
    p: number
    free: { left: number; top: number; right: number; bottom: number } | null
  }>({ p: 0, free: null })

  useEffect(() => {
    let raf = 0
    const update = () => {
      raf = 0
      const el = track.current
      if (!el) return
      // The visible height (iOS toolbars change it while scrolling), not 100vh.
      const vh = window.visualViewport?.height ?? window.innerHeight
      const r = el.getBoundingClientRect()
      const p = progressAt(-r.top / vh)
      latest.current.p = p
      engine.current?.setProgress(p)
      setIndex(stepAt(p))
      // The stage: limited to the track on screen, with soft edges, and fading
      // in with it (model.ts stageView, stageMask).
      const st = stage.current
      if (st) {
        const box = st.getBoundingClientRect()
        const v = stageView(r, box, vh)
        const m = stageMask(v, box.bottom - box.top)
        st.style.setProperty('--vis-top', `${m.top}px`)
        st.style.setProperty('--vis-bottom', `${m.bottom}px`)
        st.style.maskImage = m.image
        st.style.setProperty('-webkit-mask-image', m.image)
        st.style.opacity = String(v.opacity)
        st.style.visibility = v.on ? 'visible' : 'hidden'
        if (v.on !== visible.current) {
          visible.current = v.on
          if (v.on) engine.current?.start()
          else engine.current?.stop()
        }
        // The free part of the stage: right of the text panels on wide screens,
        // all of it on phones (the sheet is below). The shape is framed in it.
        const sh = sheet.current
        const phone = !!sh && getComputedStyle(sh).display !== 'none'
        const panel = el.querySelector('.story-panel')?.getBoundingClientRect()
        const left = phone || !panel ? 0 : Math.min(box.width * 0.6, panel.right - box.left + FREE_GAP)
        // Vertically: the band of the stage that is visible (the rest is masked, behind the header
        // or the sources), inside its soft edges, so the shape is never half hidden.
        const bandTop = m.top + (v.clipTop > 0 ? EDGE : 0)
        const bandBottom = m.bottom - (v.clipBottom > 0 ? EDGE : 0)
        const band = bandBottom - bandTop > box.height * 0.25
        const free = {
          left,
          top: band ? bandTop : m.top,
          right: box.width - (phone ? 0 : FREE_GAP),
          bottom: band ? bandBottom : m.bottom,
        }
        latest.current.free = free
        engine.current?.setFrame(free)
        // Text panels only with the scene in place: entering, the intro leaves, the
        // scene fills its zone, then the panel comes in; leaving, the other way round.
        // The sheet (phones) follows in the same frame, written to the DOM like
        // data-panels: through React state it lagged a render behind, and for a
        // moment it showed over no scene (WebKit, a toolbar appearing at the exit).
        const on = panelsShown(r, box, vh, phone ? sh!.getBoundingClientRect() : undefined)
        article.current?.setAttribute('data-panels', on ? 'on' : 'off')
        sh?.setAttribute('data-active', String(phone && on))
        st.style.setProperty('--free-left', `${free.left}px`)
        st.style.setProperty('--free-right', `${box.width - free.right}px`)
      }
    }
    const onScroll = () => {
      if (!raf) raf = requestAnimationFrame(update)
    }
    update()
    window.addEventListener('scroll', onScroll, { passive: true })
    window.addEventListener('resize', onScroll)
    window.visualViewport?.addEventListener('resize', onScroll)
    window.visualViewport?.addEventListener('scroll', onScroll)
    return () => {
      window.removeEventListener('scroll', onScroll)
      window.removeEventListener('resize', onScroll)
      window.visualViewport?.removeEventListener('resize', onScroll)
      window.visualViewport?.removeEventListener('scroll', onScroll)
      cancelAnimationFrame(raf)
    }
  }, [])

  // Phones: the sheet is as tall as its tallest step (every step is stacked in
  // it), never scrolls inside, and the scene takes the rest of the screen above.
  useEffect(() => {
    const el = sheet.current
    if (!el || typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(() => article.current?.style.setProperty('--sheet-h', `${el.offsetHeight}px`))
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  // 3D only: three.js loads once the page has painted and the main thread is idle.
  useEffect(() => {
    if (chosen.kind !== '3d') return
    let cancelled = false
    const idle = window.requestIdleCallback ?? ((cb: () => void) => window.setTimeout(cb, 200))
    const handle = idle(() => {
      loadStory().then(({ createStory }) => {
        if (cancelled || !canvas.current) return
        engine.current = createStory(canvas.current, {
          points: chosen.points,
          seed: SEED,
          smooth: true,
          onReady: () => setReady(true),
          onError: () => {
            const e = engine.current
            engine.current = null
            setTimeout(() => e?.dispose()) // not from inside its own render
            setFailed(true)
          },
        })
        engine.current.setProgress(latest.current.p)
        engine.current.setMosca(moscaNow.current.x, moscaNow.current.y)
        if (latest.current.free) engine.current.setFrame(latest.current.free)
        if (visible.current) engine.current.start()
      })
    })
    return () => {
      cancelled = true
      if (typeof handle === 'number') window.cancelIdleCallback?.(handle)
      engine.current?.dispose()
      engine.current = null
    }
  }, [chosen])

  const step = STEPS[index]
  const sceneStart = (i: number) => i === 0 || STEPS[i - 1].scene !== STEPS[i].scene

  return (
    <article ref={article} className="story" aria-labelledby="story-title" data-mode={mode.kind}>
      <header className="story-intro">
        <p className="prompt">
          <span className="user">guest@quantumready</span>:<span className="path">~/lab</span>$ ./historia
        </p>
        <h1 id="story-title" className="h-title">
          {tx.title}
        </h1>
        <p className="story-lead">{tx.lead}</p>
        {mode.kind === 'still' && <p className="story-note">{tx.still[mode.reason]}</p>}
      </header>

      {/* Never an empty scene: the step's still image until the 3D has drawn. */}
      <div ref={stage} className="story-stage" data-backdrop aria-hidden="true">
        {(mode.kind === 'still' || !ready) && <img className="story-still" src={storyStillURL(step.id)} alt="" />}
        {mode.kind === '3d' && <canvas ref={canvas} className="story-canvas" data-ready={ready} />}
      </div>

      {/* Phones: text and 3D in separate zones, the current step in a panel below the scene. */}
      {/* Its text is a copy of the step panels (which screen readers read), so it is
          hidden from them; only an interactive control (scene 6) is not, on phones. */}
      {/* data-active is set by the scroll handler, in the frame data-panels changes. */}
      <div ref={sheet} className="story-sheet">
        {STEPS.map((s, i) => (
          <div key={s.id} className="story-layer" data-current={i === index} data-step={s.id}>
            <div className="story-layer-copy" aria-hidden="true">
              <MemoStepText step={s} first={sceneStart(i)} lang={lang} />
              <MemoCaption step={s} lang={lang} />
            </div>
            {phone && s.id === 'mosca' && <MoscaWidget value={mosca} onChange={setMosca} lang={lang} active />}
          </div>
        ))}
      </div>

      <div className="story-track" ref={track}>
        {STEPS.map((s, i) => (
          <section key={s.id} className="story-step" aria-labelledby={`story-step-${s.id}`}>
            <div className="story-panel">
              <MemoStepText step={s} first={sceneStart(i)} lang={lang} headingId={`story-step-${s.id}`} />
              {/* The label (quantum numbers, the equation) inside the panel: nothing loose over the scene. */}
              <MemoCaption step={s} lang={lang} />
              {s.id === 'mosca' && <MoscaWidget value={mosca} onChange={setMosca} lang={lang} active={!phone} />}
            </div>
          </section>
        ))}
        <div className="story-tail" />
      </div>

      <section className="story-sources" aria-labelledby="story-sources-title">
        <MemoSourcesBody lang={lang} />
      </section>
    </article>
  )
}

function StepText({ step, first, lang, headingId }: { step: Step; first: boolean; lang: Lang; headingId?: string }) {
  const tx = TEXT[lang]
  const t = tx.steps[step.id]
  const o = orbitalOf(step)
  return (
    <>
      {first && <p className="story-kicker">{tx.scenes[step.scene]}</p>}
      <h2 id={headingId} className="story-h">
        {o ? (
          <>
            {o.name}
            {o.sub && <sub>{o.sub}</sub>} ·{' '}
            <span className="story-nowrap">
              (n, l, m) = ({o.n}, {o.l}, {num(o.m)})
            </span>
          </>
        ) : (
          <Title text={t.title} />
        )}
      </h2>
      <p className="story-body">{typeset(t.body)}</p>
      <p className="story-legend">
        {tx.legend[step.scene].map((l, k) => (
          <span key={l} className={k === 0 ? 'story-pos' : 'story-neg'}>
            ● {l}{' '}
          </span>
        ))}
      </p>
    </>
  )
}

/** Phones: the layout with the sheet below the scene (Story.css, the same breakpoint). */
const PHONE = '(max-width: 40rem)'
function usePhone() {
  const query = () => (typeof window.matchMedia === 'function' ? window.matchMedia(PHONE) : null)
  const [phone, setPhone] = useState(() => !!query()?.matches)
  useEffect(() => {
    const m = query()
    if (!m || typeof m.addEventListener !== 'function') return
    const on = () => setPhone(m.matches)
    m.addEventListener('change', on)
    return () => m.removeEventListener('change', on)
  }, [])
  return phone
}

/** Space between the text panels and the scene on wide screens (CSS px). */
const FREE_GAP = 24

/** A step title; an expression before ': ' never breaks inside (e.g. "iℏ ∂ψ/∂t: how it changes"). */
function Title({ text }: { text: string }) {
  const i = text.indexOf(': ')
  if (i < 0) return <>{typeset(text)}</>
  return (
    <>
      <span className="story-nowrap">{typeset(text.slice(0, i + 1))}</span> {typeset(text.slice(i + 2))}
    </>
  )
}

/** A typographic minus for negative numbers. */
const num = (v: number) => String(v).replace('-', '−')

function Caption({ step, lang }: { step: Step; lang: Lang }) {
  const tx = TEXT[lang]
  const o = orbitalOf(step)
  if (step.shape.kind === 'figure') return <FigureCaption step={step} lang={lang} />
  if (o) {
    const e = new Intl.NumberFormat(lang, {
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    })
      .format(energy(o.n))
      .replace('-', '−')
    const r = new Intl.NumberFormat(lang, { maximumFractionDigits: 1 }).format(r95(o))
    return (
      <div className="story-cap">
        <p className="story-qn">
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
          {tx.energy} E<sub>{o.n}</sub> = {e} eV
        </p>
        <p className="story-scale">{typeset(tx.scale(r))}</p>
      </div>
    )
  }
  return (
    <div className="story-cap story-eq">
      <SchrodingerTerms step={step} />
    </div>
  )
}

/** One term of the equation, with its reveal state (Story.css). */
function T({ term, state, children }: { term: Term; state: string; children: ReactNode }) {
  return (
    <mrow data-term={term} data-state={state}>
      {children}
    </mrow>
  )
}

/** iℏ ∂ψ/∂t = −(ℏ²/2m) ∂²ψ/∂x² + V(x)ψ and ψ = (ψ₁ + ψ₂)/√2, revealed and lit term by term. */
function SchrodingerTerms({ step }: { step: Step }) {
  const st = (t: Term) => termState(step, t)
  const d = (top: ReactNode, bottom: ReactNode) => (
    <mfrac>
      <mrow>{top}</mrow>
      <mrow>{bottom}</mrow>
    </mfrac>
  )
  return (
    <>
      <math display="block" aria-label="ψ(x, t)">
        <T term="psi" state={st('psi')}>
          <mi>ψ</mi>
          <mo>(</mo>
          <mi>x</mi>
          <mo>,</mo>
          <mi>t</mi>
          <mo>)</mo>
        </T>
        <T term="sup" state={st('sup')}>
          <mo>=</mo>
          {d(
            <>
              <msub>
                <mi>ψ</mi>
                <mn>1</mn>
              </msub>
              <mo>+</mo>
              <msub>
                <mi>ψ</mi>
                <mn>2</mn>
              </msub>
            </>,
            <msqrt>
              <mn>2</mn>
            </msqrt>,
          )}
        </T>
      </math>
      <math display="block" aria-label="Schrödinger">
        <T term="dt" state={st('dt')}>
          <mi>i</mi>
          <mi>ℏ</mi>
          {d(
            <>
              <mo>∂</mo>
              <mi>ψ</mi>
            </>,
            <>
              <mo>∂</mo>
              <mi>t</mi>
            </>,
          )}
        </T>
        <T term="kin" state={st('kin')}>
          <mo>=</mo>
          <mo>−</mo>
          {d(
            <msup>
              <mi>ℏ</mi>
              <mn>2</mn>
            </msup>,
            <>
              <mn>2</mn>
              <mi>m</mi>
            </>,
          )}
          {d(
            <>
              <msup>
                <mo>∂</mo>
                <mn>2</mn>
              </msup>
              <mi>ψ</mi>
            </>,
            <>
              <mo>∂</mo>
              <msup>
                <mi>x</mi>
                <mn>2</mn>
              </msup>
            </>,
          )}
        </T>
        <T term="pot" state={st('pot')}>
          <mo>+</mo>
          <mi>V</mi>
          <mo>(</mo>
          <mi>x</mi>
          <mo>)</mo>
          <mi>ψ</mi>
        </T>
      </math>
    </>
  )
}

function Sources({ lang }: { lang: Lang }) {
  const es = lang === 'es'
  return (
    <ul>
      <li>
        D. J. Griffiths, D. F. Schroeter, <cite>Introduction to Quantum Mechanics</cite>, {es ? '3.ª ed.' : '3rd ed.'}, Cambridge University Press, 2018: §2.4 ({es ? 'la partícula libre y los paquetes de ondas' : 'the free particle and wave packets'}) {es ? 'y' : 'and'} §4.2 ({es ? 'el átomo de hidrógeno' : 'the hydrogen atom'}).
      </li>
      <li>
        C. Cohen-Tannoudji, B. Diu, F. Laloë, <cite>Quantum Mechanics</cite>, vol. 1, {es ? 'complemento' : 'complement'} G<sub>I</sub> ({es ? 'el paquete de ondas gaussiano' : 'the Gaussian wave packet'}).
      </li>
      <li>
        NIST <cite>Digital Library of Mathematical Functions</cite>:{' '}
        <a href="https://dlmf.nist.gov/18.9" rel="noopener">
          §18.9
        </a>
        ,{' '}
        <a href="https://dlmf.nist.gov/14.10" rel="noopener">
          §14.10
        </a>
        ,{' '}
        <a href="https://dlmf.nist.gov/14.30" rel="noopener">
          §14.30
        </a>{' '}
        ({es ? 'Laguerre, Legendre y armónicos esféricos' : 'Laguerre, Legendre and spherical harmonics'}
        ).
      </li>
      <li>
        M. A. Nielsen, I. L. Chuang, <cite>Quantum Computation and Quantum Information</cite>, {es ? 'ed. del 10.º aniversario' : '10th anniversary ed.'}, Cambridge
        University Press, 2010: §1.2 ({es ? 'la esfera de Bloch' : 'the Bloch sphere'}), §1.3.6 ({es ? 'estados de Bell' : 'Bell states'}), §2.4.3 (
        {es ? 'traza parcial' : 'partial trace'}), §5.1–5.3 ({es ? 'QFT, búsqueda del orden y Shor' : 'QFT, order finding and Shor'}).
      </li>
      <li>
        P. W. Shor, «Polynomial-Time Algorithms for Prime Factorization and Discrete Logarithms on a Quantum Computer», <cite>SIAM J. Comput.</cite> 26(5), 1484–1509
        (1997).
      </li>
      <li>
        R. L. Rivest, A. Shamir, L. Adleman, «A Method for Obtaining Digital Signatures and Public-Key Cryptosystems», <cite>Commun. ACM</cite> 21(2), 120–126 (1978).
      </li>
      <li>
        A. Langley, M. Hamburg, S. Turner,{' '}
        <a href="https://www.rfc-editor.org/rfc/rfc7748" rel="noopener">
          RFC 7748
        </a>{' '}
        ({es ? 'X25519: la curva' : 'X25519: the curve'} y² = x³ + 486662x² + x {es ? 'sobre' : 'over'} 2²⁵⁵ − 19), 2016.
      </li>
      <li>
        C. Gidney, «How to factor 2048 bit RSA integers with less than a million noisy qubits»,{' '}
        <a href="https://arxiv.org/abs/2505.15917" rel="noopener">
          arXiv:2505.15917
        </a>{' '}
        (2025):{' '}
        {es
          ? 'menos de un millón de cúbits ruidosos y menos de una semana, suponiendo un 0,1\u00a0% de error por puerta, un ciclo de 1 µs y 10 µs de tiempo de reacción. Antes, C. Gidney y M. Ekerå, '
          : 'under a million noisy qubits and under a week, assuming 0.1% gate error, a 1 µs cycle and 10 µs reaction time. Earlier, C. Gidney and M. Ekerå, '}
        <cite>Quantum</cite> 5, 433 (2021): {es ? '20 millones de cúbits y 8 horas.' : '20 million qubits and 8 hours.'}
      </li>
      <li>
        M. Roetteler, M. Naehrig, K. M. Svore, K. Lauter, «Quantum Resource Estimates for Computing Elliptic Curve Discrete Logarithms», ASIACRYPT 2017:{' '}
        {es ? '2330 cúbits lógicos para una curva de 256 bits.' : '2330 logical qubits for a 256-bit curve.'}
      </li>
      <li>
        M. Mosca, «Cybersecurity in an Era with Quantum Computers: Will We Be Ready?», <cite>IEEE Security &amp; Privacy</cite> 16(5), 38–41 (2018){' '}
        {es ? '(la desigualdad x + y > z).' : '(the inequality x + y > z).'}
      </li>
      <li>
        M. Mosca, M. Piani (evolutionQ),{' '}
        <a href="https://globalriskinstitute.org/publication/quantum-threat-timeline-report-2025b/" rel="noopener">
          <cite>Quantum Threat Timeline Report 2025</cite>
        </a>
        , Global Risk Institute, {es ? 'marzo de 2026' : 'March 2026'}:{' '}
        {es
          ? '26 expertos estiman la probabilidad de un ordenador cuántico capaz de factorizar un número de 2048 bits en menos de 24 horas; medias entre la interpretación «pesimista» y la «optimista» de sus respuestas: 28–\u206049\u00a0% en 10 años, 51–\u206070\u00a0% en 15 (y ~15\u00a0% en 5 años en la optimista, ~69\u00a0% en 20 en la pesimista).'
          : '26 experts estimate the likelihood of a quantum computer able to factor a 2048-bit number in less than 24 hours; averages between the “pessimistic” and “optimistic” readings of their answers: 28–\u206049% within 10 years, 51–\u206070% within 15 (and ~15% within 5 years in the optimistic one, ~69% within 20 in the pessimistic one).'}
      </li>
      <li>
        L. Babai, «On Lovász’ lattice reduction and the nearest lattice point problem», <cite>Combinatorica</cite> 6(1), 1–13 (1986).
      </li>
      <li>
        O. Regev, «On lattices, learning with errors, random linear codes, and cryptography», <cite>J. ACM</cite> 56(6), art. 34 (2009).
      </li>
      <li>
        NIST,{' '}
        <a href="https://doi.org/10.6028/NIST.FIPS.203" rel="noopener">
          FIPS 203
        </a>
        , <cite>Module-Lattice-Based Key-Encapsulation Mechanism Standard</cite>, {es ? 'agosto de 2024' : 'August 2024'} ({es ? 'parámetros y tamaños, tablas 2 y 3' : 'parameters and sizes, Tables 2 and 3'}).
      </li>
      <li>
        K. Kwiatkowski, P. Kampanakis, B. E. Westerbaan, D. Stebila,{' '}
        <a href="https://www.rfc-editor.org/rfc/rfc10024" rel="noopener">
          RFC 10024
        </a>
        , <cite>Post-Quantum Traditional (PQ/T) Hybrid Key Agreement Mechanisms for TLS 1.3</cite>, {es ? 'agosto de 2026' : 'August 2026'}{' '}
        {es ? '(antes draft-ietf-tls-ecdhe-mlkem-05): X25519MLKEM768, §4.' : '(formerly draft-ietf-tls-ecdhe-mlkem-05): X25519MLKEM768, §4.'}
      </li>
      <li>
        IANA,{' '}
        <a href="https://www.iana.org/assignments/tls-parameters/tls-parameters.xhtml#tls-parameters-8" rel="noopener">
          TLS Supported Groups
        </a>
        : X25519MLKEM768 = 4588 (0x11EC), Recommended: Y ({es ? 'consultado el 7 de noviembre de 2026' : 'checked on 7 November 2026'}).
      </li>
      <li>
        CODATA 2022 (NIST), R∞hc:{' '}
        <a href="https://physics.nist.gov/cgi-bin/cuu/Value?rydhcev" rel="noopener">
          physics.nist.gov
        </a>
        .
      </li>
    </ul>
  )
}
